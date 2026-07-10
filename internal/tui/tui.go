// Package tui is the single-process Bubble Tea prototype front-end.
package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"tokensmith/internal/balance"
	"tokensmith/internal/game"
	"tokensmith/internal/ingest"
	"tokensmith/internal/ledger"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
	"tokensmith/internal/store"
)

// tickDT is how many simulated seconds each real tick advances.
const tickDT = 3600.0

// tickInterval is the real time between ticks.
const tickInterval = 250 * time.Millisecond

// gameSecPerRealSec converts a per-game-second rate to the per-real-second rate
// the player actually perceives (each real second advances several ticks of
// tickDT game-seconds).
const gameSecPerRealSec = tickDT * float64(time.Second) / float64(tickInterval)

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Page identifies the active TUI tab.
type Page int

const (
	PageOverview Page = iota
	PageModels
	PageMarket
	PageCompute
	PageTeam
	PageTech
	numPages
)

var pageNames = [numPages]string{"總覽", "模型", "市場", "算力", "團隊", "科技"}

// Model is the Bubble Tea root model.
type Model struct {
	state          model.GameState
	cfg            balance.Config
	poller         *ingest.Poller
	savePath       string
	ticksSinceSave int
	page           Page
	dialog         *trainDialog   // non-nil while the training modal is open
	publish        *publishDialog // non-nil while the publish/price modal is open
	event          *eventDialog   // non-nil while the event-choice modal is open
	techCursor     int            // selected tech node on the tech page
	procCursor     int            // selected process node on the compute page
	modelCursor    int            // selected index into state.Models on models page
	// Harvest-daemon integration (§10.2).
	ledgerPath     string
	metaPath       string
	daemonMode     bool                          // consume the daemon ledger instead of the built-in poller
	consumed       map[string]model.SourceTotals // per-source ledger tokens already applied
	lastRealUnix   int64
	metaMissing    bool
	offlineSummary *Summary // shown as a banner until dismissed by any key
	// tokensThisTick is true only on the tick that just harvested new tokens
	// (drives the pulse restart). lastTokenRnD is the per-source R&D delta from
	// that tick, kept (not cleared) across the pulse-decay window so the status
	// bar can keep showing it while it fades — see internal/tui/display.go.
	tokensThisTick bool
	lastTokenRnD   map[string]float64
	// streakDays/lastActiveDate mirror store.Meta; kept in memory between saves
	// so every tick can read the current streak multiplier cheaply.
	streakDays     int
	lastActiveDate string
	// lastCampaignUnix mirrors store.Meta.LastCampaignUnix — wall-clock of the
	// last board cycle (or the session that armed the clock). advanceCampaignTo
	// drives board-cycle catch-up against this field on startup and live ticks.
	lastCampaignUnix int64
	notice           string // transient one-line banner, dismissed by any key
	pendingRestart   bool   // armed manual restart; a second X confirms it
	width            int    // terminal width
	height           int    // terminal height
	vp               viewport.Model
	disp             displayState
	dispReady        bool // false until first snap after new game / restart
}

// New returns the game model wired to the real save/ledger/meta locations,
// with offline progress settled if a daemon ledger is present.
func New() Model {
	m := newAtPaths(store.DefaultPath(), ledger.DefaultPath(), store.DefaultMetaPath())
	return m.startup(time.Now().Unix())
}

// newAt is a test helper: derive sibling ledger/meta files from the save dir.
// It does NOT run startup(), so unit tests stay hermetic (no real-log/ledger read).
func newAt(savePath string) Model {
	dir := filepath.Dir(savePath)
	return newAtPaths(savePath, filepath.Join(dir, "ledger.json"), filepath.Join(dir, "meta.json"))
}

func newAtPaths(savePath, ledgerPath, metaPath string) Model {
	state, ok, err := store.Load(savePath)
	if err != nil {
		// Corrupt/unreadable save: preserve it beside the original so a later
		// autosave doesn't silently clobber recoverable data, then start fresh.
		_ = os.Rename(savePath, savePath+".corrupt")
		state = game.NewGame()
	} else if !ok {
		state = game.NewGame()
	}
	if state.Events.RandState == 0 {
		// New game or pre-events save: seed once, outside the pure sim.
		state.Events.RandState = uint64(time.Now().UnixNano())
	}
	if state.Campaign.RandState == 0 {
		// Campaign RNG is independent of event RNG; XOR a golden ratio constant
		// so a shared UnixNano seed cannot leave both streams identical.
		state.Campaign.RandState = uint64(time.Now().UnixNano()) ^ 0x9e3779b97f4a7c15
	}
	meta, metaOK, _ := store.LoadMeta(metaPath)
	m := Model{
		state:            state,
		cfg:              balance.Default(),
		poller:           ingest.NewDefaultPoller(),
		savePath:         savePath,
		ledgerPath:       ledgerPath,
		metaPath:         metaPath,
		consumed:         meta.ConsumedSources,
		lastRealUnix:     meta.LastRealUnix,
		metaMissing:      !metaOK,
		streakDays:       meta.StreakDays,
		lastActiveDate:   meta.LastActiveDate,
		lastCampaignUnix: meta.LastCampaignUnix,
		width:            100,
		height:           40,
		vp:               viewport.New(80, 20),
	}
	m.resize(m.width, m.height)
	m.refreshViewport()
	return m
}

// ledgerFresh reports whether the daemon updated the ledger recently enough to
// treat it as the live token source.
func ledgerFresh(l ledger.Ledger, now int64) bool { return now-l.UpdatedAt <= 30 }

// startup detects daemon mode and settles offline progress. Called by New()
// only, so unit-test constructors stay hermetic.
//
// Economic settlement (tokens/R&D/events) remains daemon-only and conditional,
// but board-cycle catch-up always runs via advanceCampaignTo so standalone
// early-return and first-open paths cannot skip the campaign clock.
//
// Meta persistence separates campaign clock from economic watermark:
// daemon settle/first-open adopt stamps LastRealUnix to startup(now);
// standalone/stale opens preserve the prior LastRealUnix on disk so a later
// daemon session can still settle the full offline economic window.
func (m Model) startup(now int64) Model {
	advanceEconomic := false
	l, ok, _ := ledger.Load(m.ledgerPath)
	if ok && ledgerFresh(l, now) {
		m.daemonMode = true
		if m.metaMissing {
			// First-ever open: adopt the current total so we don't settle a phantom
			// window of everything harvested before the player ever played.
			m.consumed = copySourceTotals(l.Sources)
			advanceEconomic = true
		} else {
			prevIn, prevOut := sumSourceTotals(m.consumed)
			offIn := l.TotalIn() - prevIn
			offOut := l.TotalOut() - prevOut
			elapsed := float64(now - m.lastRealUnix)
			if offIn > 0 || offOut > 0 {
				m.updateStreak(time.Unix(now, 0))
			}
			cfg := m.cfg
			cfg.StreakMult = m.currentStreakMult()
			ns, sum := Settle(m.state, cfg, elapsed, offIn, offOut)
			m.state = ns
			m.consumed = copySourceTotals(l.Sources)
			if sum.RnDGained > 0 || sum.TrainingCompleted || sum.EventsFired > 0 || sum.EventsAutoResolved > 0 {
				m.offlineSummary = &sum
			}
			advanceEconomic = true
		}
	}
	// Board-cycle catch-up is wall-clock TUI work, independent of daemon mode.
	var advanced int
	m, advanced = m.advanceCampaignTo(now)
	if advanced > 0 {
		if m.offlineSummary == nil {
			m.offlineSummary = &Summary{}
		}
		m.offlineSummary.CampaignCycles = advanced
		// Persist advanced campaign state before the campaign meta watermark so
		// a crash cannot drop Cycle/reports while meta claims cycles were taken.
		_ = store.Save(m.savePath, m.state)
	}
	if advanceEconomic {
		m.lastRealUnix = now
	}
	// Always write meta (campaign clock + first-open consumed), but only advance
	// economic LastRealUnix when daemon settle/adopt ran — never burn it on a
	// standalone board-only open.
	m.saveMetaAt(m.lastRealUnix)
	return m
}

// copySourceTotals deep-copies a per-source totals map so callers don't alias
// a ledger snapshot that gets discarded.
func copySourceTotals(src map[string]model.SourceTotals) map[string]model.SourceTotals {
	out := make(map[string]model.SourceTotals, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// sumSourceTotals adds up every source's In/Out.
func sumSourceTotals(m map[string]model.SourceTotals) (in, out int) {
	for _, t := range m {
		in += t.In
		out += t.Out
	}
	return
}

// currentStreakMult returns the token-R&D multiplier for m.streakDays, capped
// at streakCapDays consecutive days.
func (m Model) currentStreakMult() float64 {
	days := m.streakDays
	if days > streakCapDays {
		days = streakCapDays
	}
	return 1 + streakBonusPerDay*float64(days)
}

// updateStreak advances the coding-streak counter from the real calendar
// date. Call only when this tick actually harvested tokens (an idle tick
// must not break or extend the streak).
func (m *Model) updateStreak(now time.Time) {
	today := now.Format("2006-01-02")
	if today == m.lastActiveDate {
		return
	}
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	if m.lastActiveDate == yesterday {
		m.streakDays++
	} else {
		m.streakDays = 1
	}
	m.lastActiveDate = today
}

const (
	streakCapDays     = 10   // multiplier stops growing past this many consecutive days
	streakBonusPerDay = 0.06 // +6%/day, so day 5 = ×1.3 and the day-10 cap = ×1.6
)

// saveMeta persists the consumed watermark, streak state, campaign clock,
// and stamps LastRealUnix to the live wall clock (autosave / quit).
func (m Model) saveMeta() {
	m.saveMetaAt(time.Now().Unix())
}

// saveMetaAt writes meta with an explicit LastRealUnix. Startup uses this so
// daemon settle can stamp the synthetic/session now without burning economic
// elapsed on standalone board-only opens (which pass the preserved watermark).
func (m Model) saveMetaAt(lastRealUnix int64) {
	_ = store.SaveMeta(m.metaPath, store.Meta{
		ConsumedSources:  m.consumed,
		LastRealUnix:     lastRealUnix,
		LastActiveDate:   m.lastActiveDate,
		StreakDays:       m.streakDays,
		LastCampaignUnix: m.lastCampaignUnix,
	})
}

func (m Model) Init() tea.Cmd {
	if !m.daemonMode {
		m.poller.Prime() // standalone: start at end of logs, harvest new coding
	}
	return tick()
}

// pollTokens returns the token events for this tick, either from the daemon
// ledger (advancing the consumed watermark) or the built-in poller.
func (m *Model) pollTokens() []model.TokenEvent {
	if !m.daemonMode {
		return m.poller.Poll()
	}
	l, ok, _ := ledger.Load(m.ledgerPath)
	if !ok {
		return nil
	}
	var events []model.TokenEvent
	for src, tot := range l.Sources {
		prev := m.consumed[src]
		di := tot.In - prev.In
		do := tot.Out - prev.Out
		if di <= 0 && do <= 0 {
			continue
		}
		events = append(events, model.TokenEvent{Source: src, InputTokens: di, OutputTokens: do})
	}
	if len(events) == 0 {
		return nil
	}
	m.consumed = copySourceTotals(l.Sources)
	return events
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m2, cmd := m.handleUpdate(msg)
	m2.refreshViewport()
	return m2, cmd
}

func (m Model) handleUpdate(msg tea.Msg) (Model, tea.Cmd) {
	if m.modelCursor >= len(m.state.Models) && len(m.state.Models) > 0 {
		m.modelCursor = len(m.state.Models) - 1
	}
	if len(m.state.Models) == 0 {
		m.modelCursor = 0
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil
	case tickMsg:
		now := time.Time(msg)
		events := m.pollTokens()
		m.tokensThisTick = len(events) > 0
		if m.tokensThisTick {
			m.updateStreak(now)
		}
		cfgTick := m.cfg
		cfgTick.StreakMult = m.currentStreakMult()
		if m.tokensThisTick {
			// ponytail: soft cap (SoftCapFull/SoftCapMult) is intentionally not
			// reflected here — only matters at extreme burst volumes (~2M weighted
			// tokens in ~6s) normal solo usage never reaches. If that ever becomes
			// needed, have sim.Tick return the actual token R&D applied this tick.
			pe := sim.PrestigeEffects(m.state.Prestige.UnlockedPrestige, cfgTick)
			rnd := make(map[string]float64, len(events))
			for _, e := range events {
				rnd[e.Source] += sim.TokenRawRnD([]model.TokenEvent{e}, cfgTick) * cfgTick.StreakMult * pe.RnDMult
			}
			m.lastTokenRnD = rnd
		}
		prevFired := m.state.Events.FiredCount
		m.state = sim.Tick(m.state, tickDT, events, cfgTick)
		// Board cycles use wall-clock, not game-time: catch up after the pure tick.
		m, _ = m.advanceCampaignTo(now.Unix())
		if m.state.Events.FiredCount > prevFired {
			m.setNotice("📰 產業事件：" + latestEventName(m.state))
		}
		// Mechanism B: auto game-over + restart once debt passes the threshold.
		// Active campaigns use FinancialDistressCycles instead; player recovers
		// operationally or exits after two distressed cycles (Phase C restructuring later).
		if m.state.Campaign.Doctrine == model.DoctrineNone &&
			m.state.Resources.Cash < -m.cfg.BankruptcyDebtRatio*m.cfg.StartingCash {
			m.state = sim.Restart(m.state, m.cfg)
			m.setNotice("💥 破產！公司已重整重來")
			m.snapDisplay()
		}
		m.advanceDisplay()
		m.ticksSinceSave++
		if m.ticksSinceSave >= 40 {
			m.ticksSinceSave = 0
			_ = store.Save(m.savePath, m.state)
			m.saveMeta()
		}
		return m, tick()
	case tea.KeyMsg:
		if m.event != nil {
			return m.updateEventDialog(msg)
		}
		if m.publish != nil {
			return m.updatePublishDialog(msg)
		}
		if m.dialog != nil {
			return m.updateDialog(msg)
		}
		// Mechanism A: a second X confirms a voluntary restart; any other key cancels.
		if m.pendingRestart {
			m.pendingRestart = false
			m.notice = ""
			if msg.String() == "X" {
				m.state = sim.Restart(m.state, m.cfg)
				m.setNotice("🔄 已重來——祝這次順利")
				m.snapDisplay()
			}
			return m, nil
		}
		m.offlineSummary = nil // any key dismisses the transient banners
		m.notice = ""

		// Scroll routing (no dialog): PgUp/Dn always; ↑↓/j/k only on browse pages.
		// Team keeps `k` for hire-marketing, so k does not scroll there.
		if scrolled, next := m.tryScroll(msg); scrolled {
			return next, nil
		}

		switch msg.String() {
		case "X":
			m.pendingRestart = true
			m.setNotice("確認重來？再按一次 X（其他鍵取消）")
			return m, nil
		case "tab", "right":
			m.page = (m.page + 1) % numPages
			m.vp.GotoTop()
			return m, nil
		case "shift+tab", "left":
			m.page = (m.page + numPages - 1) % numPages
			m.vp.GotoTop()
			return m, nil
		case "1", "2", "3", "4", "5", "6":
			m.page = Page(msg.String()[0] - '1')
			m.vp.GotoTop()
			return m, nil
		case "up":
			if m.page == PageTech && len(m.cfg.TechNodes) > 0 {
				vis := techVisualOrder(m.cfg.TechNodes)
				idx := indexOf(vis, m.techCursor)
				if idx > 0 {
					m.techCursor = vis[idx-1]
				}
			}
			if m.page == PageCompute && m.procCursor > 0 {
				m.procCursor--
			}
			if m.page == PageModels && len(m.state.Models) > 0 {
				vis := visualIndices(m.state.Models)
				idx := indexOf(vis, m.modelCursor)
				if idx > 0 {
					m.modelCursor = vis[idx-1]
				}
			}
			return m, nil
		case "down":
			if m.page == PageTech && len(m.cfg.TechNodes) > 0 {
				vis := techVisualOrder(m.cfg.TechNodes)
				idx := indexOf(vis, m.techCursor)
				if idx >= 0 && idx < len(vis)-1 {
					m.techCursor = vis[idx+1]
				}
			}
			if m.page == PageCompute && m.procCursor < len(m.cfg.Processes)-1 {
				m.procCursor++
			}
			if m.page == PageModels && len(m.state.Models) > 0 {
				vis := visualIndices(m.state.Models)
				idx := indexOf(vis, m.modelCursor)
				if idx >= 0 && idx < len(vis)-1 {
					m.modelCursor = vis[idx+1]
				}
			}
			return m, nil
		case "enter":
			if m.page == PageTech && m.techCursor >= 0 && m.techCursor < len(m.cfg.TechNodes) {
				node := m.cfg.TechNodes[m.techCursor]
				ns, err := sim.Apply(m.state, model.UnlockTech{NodeID: node.ID}, m.cfg)
				switch {
				case err == nil:
					m.state = ns
				case errors.Is(err, sim.ErrInsufficientRnD):
					m.setNotice("R&D 不足")
				case errors.Is(err, sim.ErrAlreadyUnlocked):
					m.setNotice("已解鎖")
				case errors.Is(err, sim.ErrPrereqNotMet):
					m.setNotice("前置科技未滿足")
				default:
					m.setNotice("無法解鎖")
				}
			}
			return m, nil
		case "q", "ctrl+c":
			_ = store.Save(m.savePath, m.state)
			m.saveMeta()
			return m, tea.Quit
		case "t":
			if m.page == PageModels || m.page == PageOverview {
				d := newTrainDialog(m)
				m.dialog = &d
			}
			return m, nil
		case "p":
			if m.page == PageModels {
				if d, ok := newPublishDialog(m, m.modelCursor); ok {
					m.publish = &d
				} else {
					m.setNotice("請選取待發佈草稿（先訓練模型）")
				}
			}
			return m, nil
		case "$":
			if m.page == PageModels && m.modelCursor >= 0 && m.modelCursor < len(m.state.Models) {
				md := m.state.Models[m.modelCursor]
				if md.Online {
					d := publishDialog{
						index:     m.modelCursor,
						name:      md.Name,
						price:     md.Price,
						refPrice:  sim.EffectiveRefPrice(m.state, md.Segment, m.cfg),
						gen:       md.Gen,
						segment:   md.Segment,
						quality:   md.Quality,
						priceOnly: true,
					}
					m.publish = &d
				}
			}
			return m, nil
		case "P":
			if m.page == PageOverview || m.page == PageTech {
				m.state = applyOK(m.state, model.PrestigeReset{}, m.cfg)
				m.snapDisplay()
			}
			return m, nil
		case "r", "R", "i", "I":
			if m.page == PageCompute {
				key := msg.String()
				p := m.cfg.Processes[m.procCursor]
				pool := model.PoolInference
				if key == "r" || key == "R" {
					pool = model.PoolTraining
				}
				d := 1
				if key == "R" || key == "I" {
					d = -1
				}
				m.state = applyOK(m.state, model.RentCompute{Process: p.ID, Pool: pool, Delta: d}, m.cfg)
			}
			return m, nil
		case "b", "B":
			if m.page == PageCompute {
				pool := model.PoolTraining
				if msg.String() == "B" {
					pool = model.PoolInference
				}
				m.state = applyOK(m.state, model.BuildServer{Process: m.cfg.Processes[m.procCursor].ID, Pool: pool}, m.cfg)
			}
			return m, nil
		case "e":
			if m.page == PageOverview {
				if d, ok := newEventDialog(m); ok {
					m.event = &d
				} else {
					m.setNotice("目前沒有待決事件")
				}
			} else if m.page == PageCompute {
				m.state = applyOK(m.state, model.ExpandDatacenter{PowerDelta: 100, SlotDelta: 5}, m.cfg)
			} else if m.page == PageTeam {
				m.state = applyOK(m.state, model.HireStaff{Role: model.RoleEngineer, Count: 1}, m.cfg)
			}
			return m, nil
		case "h":
			if m.page == PageTeam {
				m.state = applyOK(m.state, model.HireStaff{Role: model.RoleResearcher, Tier: model.Tier1, Count: 1}, m.cfg)
			}
			return m, nil
		case "o":
			if m.page == PageTeam {
				m.state = applyOK(m.state, model.HireStaff{Role: model.RoleOps, Count: 1}, m.cfg)
			}
			return m, nil
		case "k":
			if m.page == PageTeam {
				m.state = applyOK(m.state, model.HireStaff{Role: model.RoleMarketing, Count: 1}, m.cfg)
			}
			return m, nil
		case "s":
			if m.page == PageTeam {
				if id := firstUnhiredStar(m); id != "" {
					m.state = applyOK(m.state, model.SignStar{StarID: id}, m.cfg)
				}
			}
			return m, nil
		}
	}
	return m, nil
}

// updateDialog routes keys to the open training modal, applying StartTraining
// on confirm and closing on either confirm or cancel.
func (m Model) updateDialog(msg tea.KeyMsg) (Model, tea.Cmd) {
	d, confirm, cancel := m.dialog.update(msg)
	if cancel {
		m.dialog = nil
		return m, nil
	}
	if confirm {
		m.state = applyOK(m.state, d.command(m.cfg), m.cfg)
		m.dialog = nil
		return m, nil
	}
	m.dialog = &d
	return m, nil
}

func (m Model) updatePublishDialog(msg tea.KeyMsg) (Model, tea.Cmd) {
	d, confirm, cancel := m.publish.update(msg)
	if cancel {
		m.publish = nil
		return m, nil
	}
	if confirm {
		if d.priceOnly {
			m.state = applyOK(m.state, model.SetPrice{ModelIndex: d.index, Price: d.price}, m.cfg)
			m.publish = nil
		} else {
			ns, err := sim.Apply(m.state, d.command(), m.cfg)
			if err != nil {
				switch {
				case errors.Is(err, sim.ErrInvalidName):
					m.setNotice("名稱需為 1–24 字")
				case errors.Is(err, sim.ErrInvalidPrice):
					m.setNotice("定價必須大於 0")
				default:
					m.setNotice("發佈失敗")
				}
				m.publish = &d
				return m, nil
			}
			m.state = ns
			m.setNotice(fmt.Sprintf("「%s」已上線", d.name))
			m.publish = nil
		}
		return m, nil
	}
	m.publish = &d
	return m, nil
}

func (m Model) updateEventDialog(msg tea.KeyMsg) (Model, tea.Cmd) {
	d, confirm, cancel := m.event.update(msg)
	if cancel {
		m.event = nil
		return m, nil
	}
	if confirm {
		ns, err := sim.Apply(m.state, model.ResolveEvent{PendingIndex: 0, Choice: d.cursor}, m.cfg)
		switch {
		case err == nil:
			m.state = ns
		case errors.Is(err, sim.ErrInsufficientCash):
			m.setNotice("現金不足，付不起這個選項")
			m.event = &d
			return m, nil
		case errors.Is(err, sim.ErrInsufficientRnD):
			m.setNotice("R&D 不足，付不起這個選項")
			m.event = &d
			return m, nil
		default:
			// e.g. pending was auto-resolved while the dialog was open
			m.setNotice("該事件已逾時自動決議")
			m.event = nil
			return m, nil
		}
		m.event = nil
		return m, nil
	}
	m.event = &d
	return m, nil
}

// resize updates terminal dimensions and viewport content region size.
func (m *Model) resize(w, h int) {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	m.width, m.height = w, h
	ch := h - m.chromeRows()
	if ch < 3 {
		ch = 3
	}
	cw := w - 4 // outer box margin
	if cw < 20 {
		cw = 20
	}
	m.vp.Width = cw
	m.vp.Height = ch
}

// contentWidth is the width available for page body layout inside the viewport.
// Page renderers must use this (not m.width) for ResponsiveRow and line fitting;
// resize() sets vp.Width to terminal width minus outer box chrome.
func (m Model) contentWidth() int {
	if m.vp.Width > 0 {
		return m.vp.Width
	}
	cw := m.width - 4
	if cw < 20 {
		return 20
	}
	return cw
}

// cardInnerWidth is the max display width for text inside a Card (box border + pad).
func (m Model) cardInnerWidth() int {
	inner := m.contentWidth() - cardFrameWidth
	if inner < 20 {
		return 20
	}
	return inner
}

// chromeRows estimates fixed shell lines (header/notice/bar/tabs/footer/border).
func (m Model) chromeRows() int {
	n := 2 // rounded border top+bottom
	n++    // header
	if m.offlineSummary != nil {
		n++
	}
	if m.notice != "" {
		n++
	}
	n++    // resource bar
	n++    // tabs
	n++    // footer
	n += 2 // breathing room / padding
	return n
}

// contentBody is the scrollable region: dialog or active page (no footer).
func (m Model) contentBody() string {
	if m.event != nil {
		return renderEventDialog(*m.event, m)
	}
	if m.publish != nil {
		return renderPublishDialog(*m.publish, m)
	}
	if m.dialog != nil {
		return renderTrainDialog(*m.dialog, m)
	}
	return m.renderPage()
}

func (m *Model) refreshViewport() {
	m.vp.SetContent(m.contentBody())
}

// pageUsesListCursor reports pages where ↑↓ move a selection cursor.
func (m Model) pageUsesListCursor() bool {
	switch m.page {
	case PageModels, PageTech, PageCompute:
		return true
	default:
		return false
	}
}

// tryScroll handles viewport scroll keys. Returns (true, m) if the key was
// consumed as a scroll action. Dialog callers must not invoke this.
func (m Model) tryScroll(msg tea.KeyMsg) (bool, Model) {
	switch msg.String() {
	case "pgdown", "ctrl+d":
		m.vp.HalfViewDown()
		return true, m
	case "pgup", "ctrl+u":
		m.vp.HalfViewUp()
		return true, m
	case "j", "down":
		if !m.pageUsesListCursor() {
			m.vp.LineDown(1)
			return true, m
		}
	case "k", "up":
		// Preserve Team `k` = hire marketing; only pure browse pages scroll with k/up.
		if !m.pageUsesListCursor() {
			if msg.String() == "k" && m.page == PageTeam {
				return false, m
			}
			m.vp.LineUp(1)
			return true, m
		}
	}
	return false, m
}

// pageKeys returns page-specific help text for the fixed shell footer.
func pageKeys(m Model) string {
	if m.publish != nil || m.dialog != nil || m.event != nil {
		return "" // dialogs embed their own help
	}
	switch m.page {
	case PageModels:
		if len(m.state.Models) == 0 {
			return "[t]訓練"
		}
		return "[↑↓]選模型 [p]發佈 [t]訓練 [$]改價"
	case PageMarket:
		return "[↑↓]捲動"
	case PageCompute:
		return "[↑↓]選製程 [r/R]±訓練 [i/I]±推理 [b/B]建訓練/推理伺服器 [e]擴機房"
	case PageTeam:
		return "[h]雇研究員 [e]雇工程 [o]雇營運 [k]雇行銷 [s]簽明星"
	case PageTech:
		return "[↑↓]選節點 [Enter]解鎖"
	default: // overview
		hint := "[t]訓練 [X]重來"
		if m.state.PeakValuation >= m.cfg.PrestigeUnlockValuation {
			hint = "[t]訓練 [P]傳承重開 [X]重來"
		}
		if len(m.state.Events.Pending) > 0 {
			hint = "[e]事件決策 " + hint
		}
		return hint
	}
}

// applyOK applies a command, returning the new state or the old one unchanged
// if the command was rejected (keeps a bad keystroke a harmless no-op).
func applyOK(s model.GameState, cmd model.Command, b balance.Config) model.GameState {
	if ns, err := sim.Apply(s, cmd, b); err == nil {
		return ns
	}
	return s
}

// human formats large numbers compactly (e.g. 1.84M, 340k).
func human(v float64) string {
	switch {
	case v >= 1e9:
		return fmt.Sprintf("%.2fB", v/1e9)
	case v >= 1e6:
		return fmt.Sprintf("%.2fM", v/1e6)
	case v >= 1e3:
		return fmt.Sprintf("%.0fk", v/1e3)
	case v > 0 && v < 1:
		return fmt.Sprintf("%.2f", v)
	default:
		return fmt.Sprintf("%.0f", v)
	}
}

func renderResourceBar(m Model) string {
	s := m.state
	// Prefer approached display values; fall back to truth before first snap.
	cash, rnd, val := s.Resources.Cash, s.Resources.RnD, sim.Valuation(s, m.cfg)
	trainUtil, infUtil := 0.0, 0.0
	if s.HasTraining {
		trainUtil = 1 // a job fully occupies the training pool in v0
	}
	if cap := sim.EffectiveInference(s, m.cfg); cap > 0 {
		infUtil = s.Compute.InferenceLoad / cap
	}
	if m.dispReady {
		cash, rnd, val = m.disp.Cash, m.disp.RnD, m.disp.Valuation
		trainUtil, infUtil = m.disp.TrainUtil, m.disp.InfUtil
	}
	// Show the R&D rate per real second (what the player perceives), not per
	// game-second — the latter is tiny and rounds to 0 in the display.
	rndPerRealSec := sim.RnDRatePerSec(s, m.cfg) * gameSecPerRealSec

	cashStr := fmt.Sprintf("💰 $%s", human(cash))
	if cash < 0 {
		cashStr = styleWarn.Render(cashStr)
	}

	infStr := fmt.Sprintf("推理%.0f%%", infUtil*100)
	if infUtil >= 0.9 {
		infStr = styleWarn.Render(infStr)
	}

	rndSeg := fmt.Sprintf("%s (+%s/s)", human(rnd), human(rndPerRealSec))
	if m.disp.PulseToken > 0 {
		rndSeg = styleAccent.Render(rndSeg)
	}

	bar := fmt.Sprintf("%s   ⚡R&D %s   🖥訓練%.0f%% %s   📈估值 $%s",
		cashStr, rndSeg,
		trainUtil*100, infStr, human(val))

	if m.disp.PulseToken > 0 && len(m.lastTokenRnD) > 0 {
		parts := make([]string, 0, len(m.lastTokenRnD)+1)
		for _, src := range sourceKeysOrdered(m.lastTokenRnD) {
			parts = append(parts, fmt.Sprintf("⚡ %s +%s R&D", sourceLabel(src), human(m.lastTokenRnD[src])))
		}
		if m.streakDays > 0 {
			parts = append(parts, fmt.Sprintf("🔥連續%d天 ×%.2f", m.streakDays, m.currentStreakMult()))
		}
		bar += "   " + strings.Join(parts, "   ")
	}
	return bar
}

// knownSourceOrder fixes the display order of the two known token sources;
// any future/unknown source is appended after them in map-iteration order.
var knownSourceOrder = []string{"claude-code", "codex"}

// sourceKeysOrdered returns m's keys in a stable, deterministic order so the
// status bar doesn't reorder itself between renders.
func sourceKeysOrdered(m map[string]float64) []string {
	var out []string
	seen := make(map[string]bool, len(m))
	for _, k := range knownSourceOrder {
		if _, ok := m[k]; ok {
			out = append(out, k)
			seen[k] = true
		}
	}
	for k := range m {
		if !seen[k] {
			out = append(out, k)
		}
	}
	return out
}

// sourceLabel maps a TokenEvent.Source to its display name.
func sourceLabel(src string) string {
	switch src {
	case "claude-code":
		return "Claude Code"
	case "codex":
		return "Codex"
	default:
		return src
	}
}

// latestEventName names the most recently fired event for the notice line.
func latestEventName(s model.GameState) string {
	if n := len(s.Events.Pending); n > 0 {
		return eventLabel(s.Events.Pending[n-1].EventID).Name + "（總覽頁按 e 決策）"
	}
	if n := len(s.Events.Log); n > 0 {
		return eventLabel(s.Events.Log[n-1].EventID).Name
	}
	return ""
}

// pressures returns ⚠ attention items surfaced on the overview page.
func pressures(m Model) []string {
	s := m.state
	var out []string
	if cap := sim.EffectiveInference(s, m.cfg); cap > 0 && s.Compute.InferenceLoad/cap >= 0.9 {
		out = append(out, "⚠ 推理接近上限——加租或自建推理算力")
	}
	hasOnline := false
	for _, md := range s.Models {
		if md.Online {
			hasOnline = true
			break
		}
	}
	if !hasOnline && !s.HasTraining {
		out = append(out, "⚠ 尚無營運中模型——到模型頁按 t 開訓")
	}
	if sim.EffectiveTraining(s, m.cfg) == 0 {
		out = append(out, "⚠ 無訓練算力——到算力頁按 r 租用才能訓練")
	}
	draftN := 0
	for _, md := range s.Models {
		if sim.IsDraft(md) {
			draftN++
		}
	}
	if draftN > 0 {
		out = append(out, fmt.Sprintf("待發佈模型 %d 個 — 模型頁按 p", draftN))
	}
	return out
}

func renderTabBar(p Page) string {
	var parts []string
	for i, name := range pageNames {
		label := fmt.Sprintf("[%d]%s", i+1, name)
		if Page(i) == p {
			label = tabActiveStyle.Render(label)
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, "  ")
}

func (m Model) renderPage() string {
	switch m.page {
	case PageModels:
		return renderModels(m)
	case PageMarket:
		return renderMarket(m)
	case PageCompute:
		return renderCompute(m)
	case PageTeam:
		return renderTeam(m)
	case PageTech:
		return renderTech(m)
	default:
		return renderOverview(m)
	}
}

func (m Model) View() string {
	// Local content refresh keeps View usable from tests without Update, while
	// preserving the stored YOffset for scroll.
	vp := m.vp
	vp.SetContent(m.contentBody())

	var top []string
	day := int(m.state.GameTime / 86400)
	top = append(top, styleTitle.Render(fmt.Sprintf("Tokensmith  ·  Day %d", day)))
	if m.offlineSummary != nil {
		top = append(top, offlineBanner(*m.offlineSummary))
	}
	if m.notice != "" {
		notice := m.notice
		if m.disp.PulseNotice > 0 {
			notice = styleAccent.Render(notice)
		} else {
			notice = styleMuted.Render(notice)
		}
		top = append(top, notice)
	}
	top = append(top, renderResourceBar(m))
	top = append(top, renderTabBar(m.page))

	mid := vp.View()
	bot := Footer(pageKeys(m))
	return boxStyle.Render(VStack(append(top, mid, bot)...))
}

// offlineBanner summarises what happened while the game was closed.
func offlineBanner(s Summary) string {
	msg := fmt.Sprintf("💤 離開 %.1fh，寫了 %d tokens → +%s R&D",
		s.SecondsSettled/3600, s.TokensIn+s.TokensOut, human(s.RnDGained))
	if s.TrainingCompleted {
		msg += " · 訓練完成 ✓"
	}
	if s.EventsFired > 0 {
		msg += fmt.Sprintf(" · 產業事件 %d 起", s.EventsFired)
		if s.EventsAutoResolved > 0 {
			msg += fmt.Sprintf("（%d 起已自動決議）", s.EventsAutoResolved)
		}
	} else if s.EventsAutoResolved > 0 {
		msg += fmt.Sprintf(" · %d 起待決事件已自動決議", s.EventsAutoResolved)
	}
	if s.CampaignCycles > 0 {
		msg += fmt.Sprintf(" · 董事會週期 %d 次", s.CampaignCycles)
	}
	return tabActiveStyle.Render(msg) + helpStyle.Render("  （按任意鍵關閉）")
}

func visualIndices(models []model.Model) []int {
	var vis []int
	for i, md := range models {
		if sim.IsDraft(md) {
			vis = append(vis, i)
		}
	}
	for i, md := range models {
		if !sim.IsDraft(md) {
			vis = append(vis, i)
		}
	}
	return vis
}

func indexOf(arr []int, val int) int {
	for i, v := range arr {
		if v == val {
			return i
		}
	}
	return -1
}
