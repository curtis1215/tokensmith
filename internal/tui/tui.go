// Package tui is the single-process Bubble Tea prototype front-end.
package tui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"tokensmith/internal/balance"
	"tokensmith/internal/dailyusage"
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

// ticksPerRealSec is the tick frequency (250ms interval).
const ticksPerRealSec = 4

// snapshotPollEveryTicks keeps standalone SQLite/filesystem snapshots off the
// 250ms render loop. Append-only JSONL polling remains per tick.
const snapshotPollEveryTicks = 5 * ticksPerRealSec

// dailyRefreshEveryTicks reloads daily-usage.json on the same ~5s cadence as
// other mutable sources (not every 250ms render tick).
const dailyRefreshEveryTicks = 5 * ticksPerRealSec

// gameSecPerRealSec converts a per-game-second rate to the per-real-second rate
// the player actually perceives (each real second advances several ticks of
// tickDT game-seconds).
const gameSecPerRealSec = tickDT * float64(time.Second) / float64(tickInterval)

const (
	bannerShowTicks = 12 // 每條 Major 橫幅顯示 ~3s
	maxBanners      = 8  // 佇列上限，超出丟最舊
)

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Page identifies the active TUI tab.
type Page int

const (
	PageOverview Page = iota
	PageWarRoom
	PageModels
	PageMarket
	PageCompute
	PageTeam
	PageTech
	PageAchievements
	numPages
)

var pageNames = [numPages]string{"總覽", "戰情室", "模型", "市場", "算力", "團隊", "科技", "成就"}

// Model is the Bubble Tea root model.
type Model struct {
	state             model.GameState
	cfg               balance.Config
	poller            *ingest.Poller
	snapshotSources   []ingest.SnapshotSource
	snapshotTotals    map[string]model.SourceTotals
	snapshotPollTicks int
	savePath          string
	ticksSinceSave    int
	page              Page
	dialog            *trainDialog          // non-nil while the training modal is open
	publish           *publishDialog        // non-nil while the publish/price modal is open
	event             *eventDialog          // non-nil while the event-choice modal is open
	doctrineDialog    *doctrineDialog       // non-nil while doctrine/perk/secondary/pivot modal is open
	directiveDialog   *directiveDialog      // non-nil while executive-directive modal is open
	campaignEnd       *campaignEndDialog    // non-nil while victory/exit modal is open
	employeeDetail    *employeeDetailDialog // non-nil while team employee/candidate detail is open
	campaignError     string                // last rejected campaign command; survives ticks
	techCursor        int                   // visible tech-entry index on the tech page
	techEra           int                   // selected era (1-based); 0 = current era
	procCursor        int                   // selected process node on the compute page
	modelCursor       int                   // selected index into state.Models on models page
	marketCursor      int                   // focused talent-market candidate on team page
	rosterCursor      int                   // focused roster employee on team page
	teamFocusRoster   bool                  // false = market focus; true = roster focus
	fireConfirmID     string                // pending FireEmployee id; empty = no confirm armed
	// Harvest-daemon integration (§10.2).
	ledgerPath     string
	metaPath       string
	daemonMode     bool                          // consume the daemon ledger instead of the built-in poller
	consumed       map[string]model.SourceTotals // per-source ledger tokens already applied
	lastRealUnix   int64
	metaMissing    bool
	offlineSummary *Summary // shown as a banner until dismissed by any key
	offlineReports []string // 離線期間新增的董事會報告行（最多 4）
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
	achievements   map[string]int64 // mirrors store.Meta.Achievements
	// lastCampaignUnix mirrors store.Meta.LastCampaignUnix — wall-clock of the
	// last board cycle (or the session that armed the clock). advanceCampaignTo
	// drives board-cycle catch-up against this field on startup and live ticks.
	lastCampaignUnix int64
	// lastCampaignCycle mirrors store.Meta.LastCampaignCycle — high-water of
	// Campaign.Cycle after a successful state+meta pair. Startup recovery uses
	// this to detect state-saved/meta-stale half-writes without wall-clock in sim.
	lastCampaignCycle int
	notice            string // transient one-line banner, dismissed by any key
	pendingRestart    bool   // armed manual restart; a second X confirms it
	width             int    // terminal width
	height            int    // terminal height
	vp                viewport.Model
	disp              displayState
	dispReady         bool // false until first snap after new game / restart
	// Display-layer trend history (TUI memory only, never persisted).
	sparkValuation spark
	sparkUsers     spark
	sparkRnD       spark
	sparkTick      int
	cashRate       float64                // smoothed display cash delta, $/real-second
	prevRank       [model.NumSegments]int // 上次取樣名次（0 = 無資料）
	lastRank       [model.NumSegments]int
	rankTick       int
	// Celebration feedback (TUI state, never persisted).
	banners     []Moment
	bannerTicks int
	epic        *Moment
	blink       bool // 每 tick 翻轉；威脅行明暗交替
	// Load/migration failure: block all writes and gameplay; only quit/resize.
	startupErr   error
	saveDisabled bool
	// Daily per-source raw token statistics (independent of ledger/save).
	dailyDoc          dailyusage.Document
	dailyDay          string // current time.Local day key for rendering
	dailyReader       dailyusage.Reader
	dailyWriter       *dailyusage.Buffer
	dailyRefreshTicks int
}

// pushBanner queues a Major banner, dropping the oldest beyond maxBanners.
func (m *Model) pushBanner(mo Moment) {
	if len(m.banners) >= maxBanners {
		m.banners = m.banners[1:]
	}
	m.banners = append(m.banners, mo)
	if len(m.banners) == 1 {
		m.bannerTicks = bannerShowTicks
	}
}

// New returns the game model wired to the real save/ledger/meta locations,
// with offline progress settled if a daemon ledger is present.
func New() Model {
	m := newAtPaths(store.DefaultPath(), ledger.DefaultPath(), store.DefaultMetaPath())
	m.snapshotSources = ingest.NewDefaultSnapshotSources()
	m.snapshotTotals = make(map[string]model.SourceTotals, len(m.snapshotSources))
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
	var startupErr error
	saveDisabled := false
	if err != nil {
		// Load/migration failure: keep the original file untouched, do not
		// seed a fresh writable run, and block all subsequent saves.
		startupErr = err
		saveDisabled = true
		state = model.GameState{}
	} else if !ok {
		state = game.NewGame()
	}
	if !saveDisabled {
		if state.Events.RandState == 0 {
			// New game or pre-events save: seed once, outside the pure sim.
			state.Events.RandState = uint64(time.Now().UnixNano())
		}
		if state.Campaign.RandState == 0 {
			// Campaign RNG is independent of event RNG; XOR a golden ratio constant
			// so a shared UnixNano seed cannot leave both streams identical.
			state.Campaign.RandState = uint64(time.Now().UnixNano()) ^ 0x9e3779b97f4a7c15
		}
	}
	meta, metaOK, _ := store.LoadMeta(metaPath)

	// Daily usage is sibling to save/ledger; failure never blocks gameplay.
	dailyPath := filepath.Join(filepath.Dir(savePath), "daily-usage.json")
	dailyStore := dailyusage.New(dailyPath)
	dailyDoc := dailyusage.Document{SchemaVersion: dailyusage.SchemaVersion}
	if doc, ok, err := dailyStore.Load(); err == nil && ok {
		dailyDoc = doc
	}

	m := Model{
		state:             state,
		cfg:               balance.Default(),
		poller:            ingest.NewDefaultPoller(),
		savePath:          savePath,
		ledgerPath:        ledgerPath,
		metaPath:          metaPath,
		consumed:          meta.ConsumedSources,
		lastRealUnix:      meta.LastRealUnix,
		metaMissing:       !metaOK,
		streakDays:        meta.StreakDays,
		lastActiveDate:    meta.LastActiveDate,
		achievements:      meta.Achievements,
		lastCampaignUnix:  meta.LastCampaignUnix,
		lastCampaignCycle: meta.LastCampaignCycle,
		width:             100,
		height:            40,
		vp:                viewport.New(80, 20),
		startupErr:        startupErr,
		saveDisabled:      saveDisabled,
		dailyDoc:          dailyDoc,
		dailyDay:          dailyusage.DayKey(time.Now()),
		dailyReader:       dailyStore,
		dailyWriter:       dailyusage.NewBuffer(dailyStore),
	}
	m.sparkValuation = newSpark(60)
	m.sparkUsers = newSpark(60)
	m.sparkRnD = newSpark(60)
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
	// Failed load/migration: no settle, no meta write, no state write.
	if m.saveDisabled {
		return m
	}
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
	// Recovery: saved GameState is ahead of meta high-water (state Save ok,
	// meta write lost). Arm the campaign clock at now and skip replay so
	// rivals/holds/reports are not double-applied.
	var advanced int
	preCampaign := m.state
	if m.state.Campaign.Doctrine != model.DoctrineNone && m.state.Campaign.Cycle > m.lastCampaignCycle {
		m.lastCampaignUnix = now
		m.lastCampaignCycle = m.state.Campaign.Cycle
	} else {
		m, advanced = m.advanceCampaignTo(now)
	}
	if advanced > 0 {
		for _, e := range newReportEntries(preCampaign, m.state) {
			if len(m.offlineReports) >= 4 {
				break
			}
			m.offlineReports = append(m.offlineReports, formatReportEntry(e))
		}
		if m.offlineSummary == nil {
			m.offlineSummary = &Summary{}
		}
		m.offlineSummary.CampaignCycles = advanced
		// Persist advanced campaign state before the campaign meta watermark so
		// a crash cannot drop Cycle/reports while meta claims cycles were taken.
		// Never advance meta after a failed state write.
		if err := store.Save(m.savePath, m.state); err != nil {
			return m
		}
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
		ConsumedSources:   m.consumed,
		LastRealUnix:      lastRealUnix,
		LastActiveDate:    m.lastActiveDate,
		StreakDays:        m.streakDays,
		LastCampaignUnix:  m.lastCampaignUnix,
		LastCampaignCycle: m.state.Campaign.Cycle,
		Achievements:      m.achievements,
	})
}

func (m Model) Init() tea.Cmd {
	if m.saveDisabled {
		return nil // recovery mode: no tick loop
	}
	if !m.daemonMode {
		m.poller.Prime() // standalone: start at end of logs, harvest new coding
		m.primeSnapshotSources()
	}
	return tick()
}

// recordDailyUsage updates the local-day key, applies standalone harvest
// deltas to the in-memory daily view and retry buffer, and periodically
// refreshes from disk. Daemon-mode never writes (daemon already records).
func (m *Model) recordDailyUsage(now time.Time, events []model.TokenEvent) {
	m.dailyDay = dailyusage.DayKey(now)
	if !m.daemonMode {
		batch := dailyusage.BatchFromEvents(now, events)
		if len(batch.Sources) > 0 {
			dailyusage.Apply(&m.dailyDoc, batch)
		}
		if m.dailyWriter != nil {
			if err := m.dailyWriter.Record(batch); err != nil {
				m.setNotice("⚠ 今日 Token 統計暫存失敗，將自動重試")
			}
		}
	}
	m.dailyRefreshTicks++
	if m.dailyRefreshTicks < dailyRefreshEveryTicks {
		return
	}
	m.dailyRefreshTicks = 0
	m.refreshDailyDocFromDisk()
}

// refreshDailyDocFromDisk replaces the cached document only on a successful
// present load. Standalone mode skips replacement while writes are pending so
// the in-memory overlay is not clobbered by a stale file.
func (m *Model) refreshDailyDocFromDisk() {
	if m.dailyReader == nil {
		return
	}
	if !m.daemonMode && m.dailyWriter != nil && m.dailyWriter.Pending() > 0 {
		return
	}
	doc, ok, err := m.dailyReader.Load()
	if err != nil || !ok {
		// Retain last valid in-memory view.
		return
	}
	m.dailyDoc = doc
}

// pollTokens returns the token events for this tick, either from the daemon
// ledger (advancing the consumed watermark) or the built-in poller.
func (m *Model) pollTokens() []model.TokenEvent {
	if !m.daemonMode {
		events := m.poller.Poll()
		m.snapshotPollTicks++
		if m.snapshotPollTicks < snapshotPollEveryTicks {
			return events
		}
		m.snapshotPollTicks = 0
		return append(events, m.pollSnapshotSources()...)
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

func (m *Model) primeSnapshotSources() {
	m.snapshotPollTicks = 0
	if m.snapshotTotals == nil {
		m.snapshotTotals = make(map[string]model.SourceTotals, len(m.snapshotSources))
	}
	for _, source := range m.snapshotSources {
		if totals, present, err := source.Totals(); err == nil && present {
			m.snapshotTotals[source.Source()] = totals
		}
	}
}

func (m *Model) pollSnapshotSources() []model.TokenEvent {
	if m.snapshotTotals == nil {
		m.snapshotTotals = make(map[string]model.SourceTotals, len(m.snapshotSources))
	}
	var events []model.TokenEvent
	for _, source := range m.snapshotSources {
		current, present, err := source.Totals()
		if err != nil || !present {
			continue
		}
		name := source.Source()
		previous, known := m.snapshotTotals[name]
		m.snapshotTotals[name] = current
		if !known || current.In < previous.In || current.Out < previous.Out {
			continue
		}
		delta := model.TokenEvent{
			Source:       name,
			InputTokens:  current.In - previous.In,
			OutputTokens: current.Out - previous.Out,
		}
		if delta.InputTokens > 0 || delta.OutputTokens > 0 {
			events = append(events, delta)
		}
	}
	return events
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m2, cmd := m.handleUpdate(msg)
	m2.refreshViewport()
	return m2, cmd
}

func (m Model) handleUpdate(msg tea.Msg) (Model, tea.Cmd) {
	// Recovery mode after load failure: only quit and resize; never write.
	if m.saveDisabled {
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			m.resize(msg.Width, msg.Height)
			return m, nil
		case tea.KeyMsg:
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			}
			return m, nil
		default:
			return m, nil
		}
	}

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
		m.recordDailyUsage(now, events)
		m.tokensThisTick = len(events) > 0
		if m.tokensThisTick {
			m.updateStreak(now)
		}
		prevState := m.state
		cfgTick := m.cfg
		cfgTick.StreakMult = m.currentStreakMult()
		if m.tokensThisTick {
			pe := sim.PrestigeEffects(m.state.Prestige.UnlockedPrestige, cfgTick)
			hq := balance.OfficeTokenRnDMultAt(m.state.Office.Level, cfgTick)
			rnd := make(map[string]float64, len(events))
			for _, e := range events {
				rnd[e.Source] += sim.TokenRawRnD([]model.TokenEvent{e}, cfgTick) * cfgTick.StreakMult * pe.RnDMult * hq
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
		for _, mo := range detectMoments(prevState, m.state, m.cfg) {
			switch mo.Level {
			case LevelMinor:
				m.setNotice(mo.Text)
			case LevelMajor:
				m.pushBanner(mo)
			case LevelEpic:
				mo := mo
				m.epic = &mo
			}
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
		m.rankTick++
		if m.rankTick >= 240 { // 每 60 實秒輪替一次名次快照
			m.rankTick = 0
			for seg := 0; seg < model.NumSegments; seg++ {
				r, _ := sim.MarketRank(m.state, m.cfg, model.Segment(seg))
				m.prevRank[seg] = m.lastRank[seg]
				m.lastRank[seg] = r
			}
		}
		m.blink = !m.blink
		m.advanceDisplay()
		if m.sparkTick%8 == 0 {
			m.checkAchievements(now.Unix())
		}
		m.ticksSinceSave++
		if m.ticksSinceSave >= 40 {
			m.ticksSinceSave = 0
			// Meta watermarks only advance after a confirmed state write.
			if !m.saveDisabled {
				if err := store.Save(m.savePath, m.state); err == nil {
					m.saveMeta()
				}
			}
		}
		return m, tick()
	case tea.KeyMsg:
		if m.epic != nil {
			m.epic = nil
			return m, nil
		}
		// Campaign dialogs take priority over event/publish/train.
		if m.doctrineDialog != nil {
			return m.updateDoctrineDialog(msg)
		}
		if m.directiveDialog != nil {
			return m.updateDirectiveDialog(msg)
		}
		if m.campaignEnd != nil {
			return m.updateCampaignEndDialog(msg)
		}
		if m.event != nil {
			return m.updateEventDialog(msg)
		}
		if m.publish != nil {
			return m.updatePublishDialog(msg)
		}
		if m.dialog != nil {
			return m.updateDialog(msg)
		}
		if m.employeeDetail != nil {
			return m.updateEmployeeDetailDialog(msg)
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
		m.offlineReports = nil
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
		case "1", "2", "3", "4", "5", "6", "7", "8":
			m.page = Page(msg.String()[0] - '1')
			m.vp.GotoTop()
			return m, nil
		case "up":
			if m.page == PageTech {
				techMoveCursor(&m, -1)
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
			if m.page == PageTeam {
				teamMoveFocus(&m, -1)
			}
			return m, nil
		case "down":
			if m.page == PageTech {
				techMoveCursor(&m, +1)
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
			if m.page == PageTeam {
				teamMoveFocus(&m, +1)
			}
			return m, nil
		case "[":
			if m.page == PageTech {
				techShiftEra(&m, -1)
			}
			return m, nil
		case "]":
			if m.page == PageTech {
				techShiftEra(&m, +1)
			}
			return m, nil
		case "+", "=":
			if m.page == PageTech {
				techAdjustAllocation(&m, +10)
			}
			return m, nil
		case "-", "_":
			if m.page == PageTech {
				techAdjustAllocation(&m, -10)
			}
			return m, nil
		case "enter":
			if m.page == PageTech {
				techActivate(&m)
			}
			if m.page == PageTeam {
				if d, ok := newEmployeeDetailDialog(m); ok {
					m.fireConfirmID = ""
					m.employeeDetail = &d
				} else {
					m.setNotice("沒有可查看的員工／候選人")
				}
			}
			return m, nil
		case "q", "ctrl+c":
			if !m.saveDisabled {
				if err := store.Save(m.savePath, m.state); err == nil {
					m.saveMeta()
				}
			}
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
			if m.page == PageOverview || m.page == PageWarRoom || m.page == PageTech {
				if d, ok := newCampaignEndDialog(m, campaignEndVictory); ok {
					m.campaignEnd = &d
					m.campaignError = ""
				} else {
					m.state = applyOK(m.state, model.PrestigeReset{}, m.cfg)
					m.snapDisplay()
				}
			}
			return m, nil
		case "E":
			if m.page == PageOverview || m.page == PageWarRoom {
				if d, ok := newCampaignEndDialog(m, campaignEndExit); ok {
					m.campaignEnd = &d
					m.campaignError = ""
				} else {
					m.campaignError = campaignErrorText(sim.ErrStrategyExitLocked)
				}
			}
			return m, nil
		case "c":
			if m.page == PageOverview || m.page == PageWarRoom {
				if d, ok := newDoctrineDialog(m, false); ok {
					m.doctrineDialog = &d
					m.campaignError = ""
				} else {
					m.campaignError = "此策略目前無法執行"
				}
			}
			return m, nil
		case "C":
			if m.page == PageOverview || m.page == PageWarRoom {
				if d, ok := newDoctrineDialog(m, true); ok {
					m.doctrineDialog = &d
					m.campaignError = ""
				} else if m.state.Campaign.Stage == model.CampaignStageShowdown ||
					m.state.Campaign.Stage == model.CampaignStageWon {
					m.campaignError = campaignErrorText(sim.ErrPivotLocked)
				} else {
					m.campaignError = "此策略目前無法執行"
				}
			}
			return m, nil
		case "d":
			if m.page == PageOverview || m.page == PageWarRoom {
				if d, ok := newDirectiveDialog(m); ok {
					m.directiveDialog = &d
					m.campaignError = ""
				} else {
					m.campaignError = "此策略目前無法執行"
				}
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
				m.applyNotice(model.RentCompute{Process: p.ID, Pool: pool, Delta: d}, "")
			} else if m.page == PageTeam && (msg.String() == "r" || msg.String() == "R") {
				applyTeamReroll(&m)
			}
			return m, nil
		case "b", "B":
			if m.page == PageCompute {
				pool := model.PoolTraining
				if msg.String() == "B" {
					pool = model.PoolInference
				}
				m.applyNotice(model.BuildServer{Process: m.cfg.Processes[m.procCursor].ID, Pool: pool}, "🏗 伺服器建造完成")
			}
			return m, nil
		case "e":
			if m.page == PageOverview || m.page == PageWarRoom {
				if d, ok := newEventDialog(m); ok {
					m.event = &d
				} else {
					m.setNotice("目前沒有待決事件")
				}
			} else if m.page == PageCompute {
				m.applyNotice(model.ExpandDatacenter{PowerDelta: 100, SlotDelta: 5}, "🏗 機房擴建完成")
			}
			return m, nil
		case "u":
			if m.page == PageTeam {
				applyTeamUpgrade(&m)
			}
			return m, nil
		case "h":
			if m.page == PageTeam {
				applyTeamHire(&m)
			}
			return m, nil
		case "f":
			if m.page == PageTeam {
				applyTeamFire(&m)
			}
			return m, nil
		case "esc", "escape":
			if m.page == PageTeam && m.fireConfirmID != "" {
				clearTeamFireConfirm(&m)
			}
			return m, nil
		case "j":
			if m.page == PageTeam {
				teamMoveFocus(&m, +1)
			}
			return m, nil
		case "k":
			// Team: k moves focus up (not viewport scroll; see tryScroll).
			if m.page == PageTeam {
				teamMoveFocus(&m, -1)
			}
			return m, nil
		case " ":
			// Space toggles market ↔ roster focus on the team page.
			if m.page == PageTeam {
				teamToggleFocus(&m)
			}
			return m, nil
		}
	}
	return m, nil
}

// updateEmployeeDetailDialog closes the team employee/candidate detail modal.
func (m Model) updateEmployeeDetailDialog(msg tea.KeyMsg) (Model, tea.Cmd) {
	d, cancel := m.employeeDetail.update(msg)
	if cancel {
		m.employeeDetail = nil
		return m, nil
	}
	m.employeeDetail = &d
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
		ns, err := sim.Apply(m.state, d.command(m.cfg), m.cfg)
		if err != nil {
			switch {
			case errors.Is(err, sim.ErrInsufficientCash):
				d.errMsg = "現金不足"
			case errors.Is(err, sim.ErrInsufficientRnD):
				d.errMsg = "R&D 不足"
			case errors.Is(err, sim.ErrTrainingInProgress):
				d.errMsg = "已有訓練進行中"
			default:
				d.errMsg = "無法開始訓練"
			}
			m.dialog = &d
			return m, nil
		}
		m.state = ns
		m.setNotice("🚂 訓練已啟動")
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
			m.setNotice("✓ 事件已決議")
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

func (m Model) updateDoctrineDialog(msg tea.KeyMsg) (Model, tea.Cmd) {
	d, confirm, cancel := m.doctrineDialog.update(msg)
	if cancel {
		m.doctrineDialog = nil
		m.campaignError = ""
		return m, nil
	}
	if confirm {
		cmd := d.command(m)
		ns, err := sim.Apply(m.state, cmd, m.cfg)
		if err != nil {
			m.campaignError = campaignErrorText(err)
			m.doctrineDialog = &d
			return m, nil
		}
		m.state = ns
		m.campaignError = ""
		m.doctrineDialog = nil
		if _, ok := cmd.(model.ChooseDoctrine); ok {
			// Task 8 hard gate: overwrite any pre-armed clock at selection time.
			// Preserve economic LastRealUnix (Task 8 C1) — only campaign clock moves.
			m.lastCampaignUnix = time.Now().Unix()
			m.saveMetaAt(m.lastRealUnix)
		}
		return m, nil
	}
	m.doctrineDialog = &d
	return m, nil
}

func (m Model) updateDirectiveDialog(msg tea.KeyMsg) (Model, tea.Cmd) {
	d, confirm, cancel := m.directiveDialog.update(msg)
	if cancel {
		m.directiveDialog = nil
		m.campaignError = ""
		return m, nil
	}
	if confirm {
		ns, err := sim.Apply(m.state, d.command(m), m.cfg)
		if err != nil {
			m.campaignError = campaignErrorText(err)
			m.directiveDialog = &d
			return m, nil
		}
		m.state = ns
		m.campaignError = ""
		m.directiveDialog = nil
		return m, nil
	}
	m.directiveDialog = &d
	return m, nil
}

func (m Model) updateCampaignEndDialog(msg tea.KeyMsg) (Model, tea.Cmd) {
	d, confirm, cancel := m.campaignEnd.update(msg)
	if cancel {
		m.campaignEnd = nil
		m.campaignError = ""
		return m, nil
	}
	if confirm {
		cmd := d.command()
		ns, err := sim.Apply(m.state, cmd, m.cfg)
		if err != nil {
			m.campaignError = campaignErrorText(err)
			m.campaignEnd = &d
			return m, nil
		}
		m.state = ns
		m.campaignError = ""
		m.campaignEnd = nil
		m.snapDisplay()
		switch cmd.(type) {
		case model.CampaignPrestige, model.CampaignExit:
			m.epic = newRunEpic(m)
		}
		return m, nil
	}
	m.campaignEnd = &d
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
		n += lipgloss.Height(renderOfflineReport(m))
	}
	if m.notice != "" {
		n++
	}
	if len(m.banners) > 0 {
		n++
	}
	// campaignError outside an open dialog is shown as a banner line.
	if m.campaignError != "" && m.doctrineDialog == nil && m.directiveDialog == nil && m.campaignEnd == nil {
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
	if m.epic != nil {
		return renderEpicOverlay(*m.epic, m)
	}
	// Campaign dialogs before event/publish/train.
	if m.doctrineDialog != nil {
		return renderDoctrineDialog(*m.doctrineDialog, m)
	}
	if m.directiveDialog != nil {
		return renderDirectiveDialog(*m.directiveDialog, m)
	}
	if m.campaignEnd != nil {
		return renderCampaignEndDialog(*m.campaignEnd, m)
	}
	if m.event != nil {
		return renderEventDialog(*m.event, m)
	}
	if m.publish != nil {
		return renderPublishDialog(*m.publish, m)
	}
	if m.dialog != nil {
		return renderTrainDialog(*m.dialog, m)
	}
	if m.employeeDetail != nil {
		return renderEmployeeDetailDialog(*m.employeeDetail, m)
	}
	return m.renderPage()
}

func (m *Model) refreshViewport() {
	m.vp.SetContent(m.contentBody())
}

// pageUsesListCursor reports pages where ↑↓/j/k move a selection cursor.
func (m Model) pageUsesListCursor() bool {
	switch m.page {
	case PageModels, PageTech, PageCompute, PageTeam:
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
		// Team/Models/Tech/Compute use k/up for list cursors, not scroll.
		if !m.pageUsesListCursor() {
			m.vp.LineUp(1)
			return true, m
		}
	}
	return false, m
}

// pageKeys returns page-specific help text for the fixed shell footer.
func pageKeys(m Model) string {
	if m.publish != nil || m.dialog != nil || m.event != nil ||
		m.doctrineDialog != nil || m.directiveDialog != nil || m.campaignEnd != nil ||
		m.employeeDetail != nil {
		return "" // dialogs embed their own help
	}
	switch m.page {
	case PageWarRoom:
		hint := "[1]總覽"
		if len(m.state.Events.Pending) > 0 {
			hint = "[e]決策 " + hint
		}
		if m.state.Campaign.PerkTierPending > 0 {
			hint += " [c]能力"
		}
		if m.state.Campaign.Victory != model.DoctrineNone {
			hint += " [P]結算"
		}
		return hint
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
		return "[j/k]選擇 [space]市場/名冊 [Enter]詳情 [h]雇用 [f]解雇(兩次確認) [u]升級 [r]重抽"
	case PageTech:
		return "[↑↓]條目 [ ]時代 [Enter]執行 [+]/[-]前沿分配"
	case PageAchievements:
		return "[↑↓]捲動"
	default: // overview — campaign keys are hints only (dialogs land in Task 10)
		hint := "[t]訓練 [X]重來"
		if m.state.Campaign.Victory != model.DoctrineNone {
			hint = "[t]訓練 [P]勝利結算 [X]重來"
		} else if m.state.PeakValuation >= m.cfg.PrestigeUnlockValuation {
			hint = "[t]訓練 [P]傳承重開 [X]重來"
		}
		if m.state.Campaign.Cycle >= m.cfg.Campaign.StrategyExitCycle ||
			m.state.Campaign.FinancialDistressCycles >= 2 {
			hint += " [E]策略退出"
		}
		hint += " [c]公司策略 [d]高層指令"
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

// applyNotice applies cmd; on success shows okMsg (empty = silent success).
// Rejected commands stay silent no-ops, same as applyOK.
func (m *Model) applyNotice(cmd model.Command, okMsg string) {
	ns, err := sim.Apply(m.state, cmd, m.cfg)
	if err != nil {
		return
	}
	m.state = ns
	if okMsg != "" {
		m.setNotice(okMsg)
	}
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
	switch {
	case cash < 0:
		cashStr = styleLoss.Render(cashStr)
	case m.cashRate > 0.5:
		cashStr += styleGain.Render(fmt.Sprintf(" ▲$%s/s", human(m.cashRate)))
	case m.cashRate < -0.5:
		cashStr += styleLoss.Render(fmt.Sprintf(" ▼$%s/s", human(-m.cashRate)))
	}

	infStr := fmt.Sprintf("推理%.0f%%", infUtil*100)
	if infUtil >= 0.9 {
		infStr = styleWarn.Render(infStr)
	}

	rndSeg := fmt.Sprintf("%s (+%s/s)", human(rnd), human(rndPerRealSec))
	if m.disp.PulseToken > 0 {
		rndSeg = styleAccent.Render(rndSeg)
	}

	valStr := stylePurple.Render(fmt.Sprintf("📈估值 $%s", human(val)))
	sep := styleMuted.Render(" │ ")
	segs := []string{
		cashStr,
		"⚡R&D " + rndSeg,
		fmt.Sprintf("🖥訓練%.0f%% %s", trainUtil*100, infStr),
		valStr,
	}
	if m.streakDays >= 2 {
		streak := fmt.Sprintf("🔥%d天 ×%.2f", m.streakDays, m.currentStreakMult())
		if m.disp.PulseToken > 0 {
			segs = append(segs, styleGold.Bold(true).Render(streak))
		} else {
			segs = append(segs, styleAmber.Render(streak))
		}
	}
	bar := strings.Join(segs, sep)

	if m.disp.PulseToken > 0 && len(m.lastTokenRnD) > 0 {
		parts := make([]string, 0, len(m.lastTokenRnD))
		for _, src := range sourceKeysOrdered(m.lastTokenRnD) {
			chip := fmt.Sprintf(" ⚡%s +%s R&D ", sourceLabel(src), human(m.lastTokenRnD[src]))
			parts = append(parts, lipgloss.NewStyle().Bold(true).Foreground(colorInk).Background(colorCyan).Render(chip))
		}
		hq := balance.OfficeTokenRnDMultAt(m.state.Office.Level, m.cfg)
		hqChip := styleMuted.Render(fmt.Sprintf(" ·  總部 ×%.2f", hq))
		bar += "  " + strings.Join(parts, " ") + hqChip
	}
	return bar
}

// knownSourceOrder fixes the display order of known token sources;
// any future/unknown source is appended after them in map-iteration order.
var knownSourceOrder = []string{"claude-code", "codex", "grok", "opencode"}

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
	case "grok":
		return "Grok（估算）"
	case "opencode":
		return "OpenCode"
	default:
		return src
	}
}

// latestEventName names the most recently fired event for the notice line.
func latestEventName(s model.GameState) string {
	if n := len(s.Events.Pending); n > 0 {
		return eventLabel(s.Events.Pending[n-1].EventID).Name + "（按 e 決策）"
	}
	if n := len(s.Events.Log); n > 0 {
		return eventLabel(s.Events.Log[n-1].EventID).Name
	}
	return ""
}

// pressures returns all ⚠ attention items (operational + campaign).
func pressures(m Model) []string {
	return append(operationalPressures(m), campaignPressures(m)...)
}

// operationalPressures are business/ops warnings for the overview 注意 card.
func operationalPressures(m Model) []string {
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
	// Persistent frontier stall (card/pressure line — never toast spam per tick).
	if fv := sim.FrontierProgressView(s, m.cfg); fv.Active && fv.UnavailableReason != "" {
		out = append(out, "⚠ 前沿研究停滯——"+frontierStallCopy(fv.UnavailableReason))
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

// campaignPressures are war-room warnings (strategy / perk / distress).
func campaignPressures(m Model) []string {
	s := m.state
	var out []string
	hasOnline := false
	for _, md := range s.Models {
		if md.Online {
			hasOnline = true
			break
		}
	}
	if c := s.Campaign.FinancialDistressCycles; c >= 1 {
		out = append(out, styleLoss.Bold(true).Render(
			fmt.Sprintf("🩸 財務危機 第 %d 週期——連續 2 週期可策略退出 [E]", c)))
	}
	if s.Campaign.Doctrine == model.DoctrineNone && hasOnline {
		out = append(out, "⚠ 尚未選擇公司戰略——按 c 選擇")
	}
	if s.Campaign.PerkTierPending > 0 {
		out = append(out, fmt.Sprintf("⚠ 可選第 %d 階路線能力——按 c 選擇", s.Campaign.PerkTierPending))
	}
	return out
}

func renderTabBar(p Page) string {
	var parts []string
	for i, name := range pageNames {
		label := fmt.Sprintf(" %d %s ", i+1, name)
		if Page(i) == p {
			parts = append(parts, lipgloss.NewStyle().Bold(true).Foreground(colorInk).Background(colorCyan).Render(label))
		} else {
			parts = append(parts, styleMuted.Render(label))
		}
	}
	return strings.Join(parts, " ")
}

func (m Model) renderPage() string {
	switch m.page {
	case PageWarRoom:
		return renderWarRoom(m)
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
	case PageAchievements:
		return renderAchievements(m)
	default:
		return renderOverview(m)
	}
}

func (m Model) View() string {
	if m.saveDisabled {
		return renderLoadFailure(m)
	}
	// Local content refresh keeps View usable from tests without Update, while
	// preserving the stored YOffset for scroll.
	vp := m.vp
	vp.SetContent(m.contentBody())

	var top []string
	day := int(m.state.GameTime / 86400)
	top = append(top, styleTitle.Render(fmt.Sprintf("Tokensmith  ·  Day %d", day)))
	if m.offlineSummary != nil {
		top = append(top, renderOfflineReport(m))
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
	if len(m.banners) > 0 {
		top = append(top, styleGold.Bold(true).Render("★ "+m.banners[0].Text))
	}
	// Show campaignError as a shell banner when no campaign dialog is open
	// (in-dialog errors render inside the modal). Tick must not clear it.
	if m.campaignError != "" && m.doctrineDialog == nil && m.directiveDialog == nil && m.campaignEnd == nil {
		top = append(top, styleWarn.Render(m.campaignError))
	}
	top = append(top, renderResourceBar(m))
	top = append(top, renderTabBar(m.page))

	mid := vp.View()
	bot := Footer(pageKeys(m))
	return boxStyle.Render(VStack(append(top, mid, bot)...))
}

// renderLoadFailure is the blocking recovery screen when save load/migration
// fails. Gameplay and writes are disabled; only quit is offered.
func renderLoadFailure(m Model) string {
	errText := "unknown error"
	if m.startupErr != nil {
		errText = m.startupErr.Error()
	}
	body := VStack(
		styleWarn.Render("無法載入存檔"),
		"",
		"路徑："+m.savePath,
		"錯誤："+errText,
		"",
		styleMuted.Render("原始檔案未更動（未改名、未覆寫）。"),
		styleMuted.Render("請手動修復或還原備份後再啟動。"),
		"",
		styleAccent.Render("按 q 結束（不會寫入存檔）"),
	)
	return boxStyle.Render(CardIn(CardThreat, m.contentWidth(), "存檔載入失敗", body))
}

// renderOfflineReport summarises what happened while the game was closed.
func renderOfflineReport(m Model) string {
	s := *m.offlineSummary
	lines := []string{fmt.Sprintf("💤 離開 %.1fh · 寫了 %d tokens → +%s R&D",
		s.SecondsSettled/3600, s.TokensIn+s.TokensOut, human(s.RnDGained))}
	if s.TrainingCompleted {
		lines = append(lines, styleGain.Render("🧪 訓練完成 ✓"))
	}
	if s.EventsFired > 0 {
		ev := fmt.Sprintf("📰 產業事件 %d 起", s.EventsFired)
		if s.EventsAutoResolved > 0 {
			ev += fmt.Sprintf("（%d 起已自動決議）", s.EventsAutoResolved)
		}
		lines = append(lines, ev)
	} else if s.EventsAutoResolved > 0 {
		lines = append(lines, fmt.Sprintf("📰 %d 起待決事件已自動決議", s.EventsAutoResolved))
	}
	if s.CampaignCycles > 0 {
		lines = append(lines, fmt.Sprintf("🏛 董事會週期 %d 次", s.CampaignCycles))
	}
	lines = append(lines, m.offlineReports...)
	lines = append(lines, styleMuted.Render("按任意鍵關閉"))
	return CardIn(CardAccent, 0, "離線戰報", VStack(lines...))
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
