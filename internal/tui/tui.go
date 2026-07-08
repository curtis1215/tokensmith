// Package tui is the single-process Bubble Tea prototype front-end.
package tui

import (
	"fmt"
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

// Model is the Bubble Tea root model.
type Model struct {
	state          model.GameState
	cfg            balance.Config
	poller         *ingest.Poller
	lastTokens     int
	savePath       string
	ticksSinceSave int
}

// New returns a fresh prototype model.
func New() Model { return newAt(store.DefaultPath()) }

func newAt(savePath string) Model {
	state, ok, _ := store.Load(savePath)
	if !ok {
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
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	boxStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	helpStyle  = lipgloss.NewStyle().Faint(true)
)

func (m Model) View() string {
	s := m.state
	res := fmt.Sprintf("💰 $%.0f    ⚡ R&D %.0f    🖥 訓練 %.0f · 推理 %.1f/%.0f",
		s.Resources.Cash, s.Resources.RnD,
		s.Compute.TrainingCapacity, s.Compute.InferenceLoad, s.Compute.InferenceCapacity)
	if m.lastTokens > 0 {
		res += fmt.Sprintf("    ⚡token +%d", m.lastTokens)
	}

	var mb strings.Builder
	mb.WriteString("模型:\n")
	if s.HasTraining {
		mb.WriteString(fmt.Sprintf("  訓練中 Gen%d  剩 %.0f GPU·s\n", s.Training.Gen, s.Training.WorkRemaining))
	}
	for _, md := range s.Models {
		mb.WriteString(fmt.Sprintf("  Gen%d  用戶 %.0f  價 $%.0f  能力 %.0f\n",
			md.Gen, md.Users, md.Price, md.Quality[model.DimCapability]))
	}
	if len(s.Models) == 0 && !s.HasTraining {
		mb.WriteString("  (無 — 按 t 訓練第一個模型)\n")
	}

	var cb strings.Builder
	cb.WriteString("對手 (能力):\n")
	for _, c := range s.Competitors {
		cb.WriteString(fmt.Sprintf("  %-10s %.1f\n", c.Name, c.Quality[model.DimCapability]))
	}

	help := helpStyle.Render("[t]訓練  [r]+訓練算力  [i]+推理算力  [q]離開")
	body := lipgloss.JoinVertical(lipgloss.Left, res, "", mb.String(), cb.String(), help)
	return boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, titleStyle.Render("Tokensmith"), body))
}
