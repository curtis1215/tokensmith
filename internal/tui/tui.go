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
		case "q", "ctrl+c":
			_ = store.Save(m.savePath, m.state)
			return m, tea.Quit
		case "t":
			cmd := model.StartTraining{
				Gen:   1,
				Alloc: [model.NumQualityDims]float64{0.4, 0.2, 0.2, 0.2},
				Price: m.cfg.SegmentRefPrice[model.SegConsumer],
			}
			if ns, err := sim.Apply(m.state, cmd, m.cfg); err == nil {
				m.state = ns
			}
		case "r":
			if ns, err := sim.Apply(m.state, model.RentTrainingCompute{Delta: 1}, m.cfg); err == nil {
				m.state = ns
			}
		case "i":
			if ns, err := sim.Apply(m.state, model.RentInferenceCompute{Delta: 1}, m.cfg); err == nil {
				m.state = ns
			}
		}
	}
	return m, nil
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
	bar := fmt.Sprintf("💰 $%s   ⚡R&D %.0f/s   🖥訓練%.0f%% 推理%.0f%%   📈估值 $%s",
		human(s.Resources.Cash), s.Resources.RnD, trainUtil*100, infUtil*100,
		human(sim.Valuation(s, m.cfg)))
	if m.lastTokens > 0 {
		bar += fmt.Sprintf("   ⚡token +%d", m.lastTokens)
	}
	return bar
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
	body := lipgloss.JoinVertical(lipgloss.Left,
		renderResourceBar(m),
		sep,
		renderTabBar(m.page),
		sep,
		m.renderPage(),
	)
	return boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, titleStyle.Render("Tokensmith"), body))
}
