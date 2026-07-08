// Package tui is the single-process Bubble Tea prototype front-end.
package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"tokensmith/internal/balance"
	"tokensmith/internal/game"
	"tokensmith/internal/ingest"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
	"tokensmith/internal/store"
)

// tickDT is how many simulated seconds each real tick advances.
const tickDT = 3600.0

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
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
	lastTokens     int
	savePath       string
	ticksSinceSave int
	page           Page
	dialog         *trainDialog // non-nil while the training modal is open
	techCursor     int          // selected tech node on the tech page
}

// New returns a fresh prototype model.
func New() Model { return newAt(store.DefaultPath()) }

func newAt(savePath string) Model {
	state, ok, err := store.Load(savePath)
	if err != nil {
		// Corrupt/unreadable save: preserve it beside the original so a later
		// autosave doesn't silently clobber recoverable data, then start fresh.
		_ = os.Rename(savePath, savePath+".corrupt")
		state = game.NewGame()
	} else if !ok {
		state = game.NewGame()
	}
	return Model{
		state:    state,
		cfg:      balance.Default(),
		poller:   ingest.NewDefaultPoller(),
		savePath: savePath,
	}
}

func (m Model) Init() tea.Cmd {
	m.poller.Prime() // start at end of logs: harvest new coding, not history
	return tick()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		events := m.poller.Poll()
		m.lastTokens = 0
		for _, e := range events {
			m.lastTokens += e.InputTokens + e.OutputTokens
		}
		m.state = sim.Tick(m.state, tickDT, events, m.cfg)
		m.ticksSinceSave++
		if m.ticksSinceSave >= 40 {
			m.ticksSinceSave = 0
			_ = store.Save(m.savePath, m.state)
		}
		return m, tick()
	case tea.KeyMsg:
		if m.dialog != nil {
			return m.updateDialog(msg)
		}
		switch msg.String() {
		case "tab", "right":
			m.page = (m.page + 1) % numPages
			return m, nil
		case "shift+tab", "left":
			m.page = (m.page + numPages - 1) % numPages
			return m, nil
		case "1", "2", "3", "4", "5", "6":
			m.page = Page(msg.String()[0] - '1')
			return m, nil
		case "up":
			if m.page == PageTech && m.techCursor > 0 {
				m.techCursor--
			}
			return m, nil
		case "down":
			if m.page == PageTech && m.techCursor < len(m.cfg.TechNodes)-1 {
				m.techCursor++
			}
			return m, nil
		case "enter":
			if m.page == PageTech && m.techCursor >= 0 && m.techCursor < len(m.cfg.TechNodes) {
				m.state = applyOK(m.state, model.UnlockTech{NodeID: m.cfg.TechNodes[m.techCursor].ID}, m.cfg)
			}
			return m, nil
		case "q", "ctrl+c":
			_ = store.Save(m.savePath, m.state)
			return m, tea.Quit
		case "t":
			if m.page == PageModels || m.page == PageOverview {
				d := newTrainDialog(m)
				m.dialog = &d
			}
			return m, nil
		case "P":
			if m.page == PageOverview || m.page == PageTech {
				m.state = applyOK(m.state, model.PrestigeReset{}, m.cfg)
			}
			return m, nil
		case "r":
			if m.page == PageCompute {
				m.state = applyOK(m.state, model.RentTrainingCompute{Delta: 1}, m.cfg)
			}
			return m, nil
		case "R":
			if m.page == PageCompute {
				m.state = applyOK(m.state, model.RentTrainingCompute{Delta: -1}, m.cfg)
			}
			return m, nil
		case "i":
			if m.page == PageCompute {
				m.state = applyOK(m.state, model.RentInferenceCompute{Delta: 1}, m.cfg)
			}
			return m, nil
		case "I":
			if m.page == PageCompute {
				m.state = applyOK(m.state, model.RentInferenceCompute{Delta: -1}, m.cfg)
			}
			return m, nil
		case "b":
			if m.page == PageCompute && len(m.cfg.Chips) > 0 {
				m.state = applyOK(m.state, model.BuildServer{ChipName: m.cfg.Chips[0].Name}, m.cfg)
			}
			return m, nil
		case "e":
			if m.page == PageCompute {
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
func (m Model) updateDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

// applyOK applies a command, returning the new state or the old one unchanged
// if the command was rejected (keeps a bad keystroke a harmless no-op).
func applyOK(s model.GameState, cmd model.Command, b balance.Config) model.GameState {
	if ns, err := sim.Apply(s, cmd, b); err == nil {
		return ns
	}
	return s
}

var (
	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	boxStyle       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	helpStyle      = lipgloss.NewStyle().Faint(true)
	tabActiveStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Underline(true)
)

// human formats large numbers compactly (e.g. 1.84M, 340k).
func human(v float64) string {
	switch {
	case v >= 1e9:
		return fmt.Sprintf("%.2fB", v/1e9)
	case v >= 1e6:
		return fmt.Sprintf("%.2fM", v/1e6)
	case v >= 1e3:
		return fmt.Sprintf("%.0fk", v/1e3)
	default:
		return fmt.Sprintf("%.0f", v)
	}
}

// progressBar renders a fixed-width ▓/░ bar for frac∈[0,1].
func progressBar(frac float64, width int) string {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	n := int(frac * float64(width))
	return strings.Repeat("▓", n) + strings.Repeat("░", width-n)
}

func renderResourceBar(m Model) string {
	s := m.state
	trainUtil := 0.0
	if s.HasTraining {
		trainUtil = 1 // a job fully occupies the training pool in v0
	}
	infUtil := 0.0
	if cap := sim.EffectiveInference(s, m.cfg); cap > 0 {
		infUtil = s.Compute.InferenceLoad / cap
	}
	bar := fmt.Sprintf("Day %d   💰 $%s   ⚡R&D %.0f/s   🖥訓練%.0f%% 推理%.0f%%   📈估值 $%s",
		int(s.GameTime/86400), human(s.Resources.Cash), s.Resources.RnD, trainUtil*100, infUtil*100,
		human(sim.Valuation(s, m.cfg)))
	if m.lastTokens > 0 {
		bar += fmt.Sprintf("   ⚡token +%d", m.lastTokens)
	}
	return bar
}

// pressures returns ⚠ attention items surfaced on the overview page. (A real
// coding-streak counter is deferred to a later plan.)
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
	sep := strings.Repeat("─", 66)
	page := m.renderPage()
	if m.dialog != nil {
		page = renderTrainDialog(*m.dialog, m)
	}
	body := lipgloss.JoinVertical(lipgloss.Left,
		renderResourceBar(m),
		sep,
		renderTabBar(m.page),
		sep,
		page,
	)
	return boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, titleStyle.Render("Tokensmith"), body))
}
