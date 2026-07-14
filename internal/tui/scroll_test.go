package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"tokensmith/internal/model"
)

func tallViewport(t *testing.T, page Page) Model {
	t.Helper()
	m := testModel(t)
	m.page = page
	m.resize(80, 20)
	// Ensure content is longer than the viewport so scroll can move.
	var b strings.Builder
	for i := 0; i < 80; i++ {
		b.WriteString("scroll-line\n")
	}
	m.vp.SetContent(b.String())
	return m
}

func TestBrowsePageDownScrolls(t *testing.T) {
	m := tallViewport(t, PageMarket)
	before := m.vp.YOffset
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	g := nm.(Model)
	if g.vp.YOffset <= before {
		t.Fatalf("market down should scroll: before=%d after=%d", before, g.vp.YOffset)
	}
}

func TestBrowsePgDnScrolls(t *testing.T) {
	m := tallViewport(t, PageOverview)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if nm.(Model).vp.YOffset <= 0 {
		t.Fatalf("overview pgdown should increase YOffset, got %d", nm.(Model).vp.YOffset)
	}
}

func TestModelsDownMovesCursorNotOnlyScroll(t *testing.T) {
	m := tallViewport(t, PageModels)
	m.state.Models = []model.Model{
		{Online: false, Gen: 1, Name: "A"},
		{Online: false, Gen: 1, Name: "B"},
		{Online: true, Gen: 1, Name: "C", Users: 10, Price: 12},
	}
	m.modelCursor = 0
	m.refreshViewport()
	beforeOff := m.vp.YOffset
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	g := nm.(Model)
	if g.modelCursor == 0 {
		t.Fatal("models down should move modelCursor")
	}
	// List-cursor pages must not treat plain down as scroll.
	if g.vp.YOffset != beforeOff {
		t.Fatalf("models down should not scroll: before=%d after=%d", beforeOff, g.vp.YOffset)
	}
}

func TestModelsPgDnStillScrolls(t *testing.T) {
	m := tallViewport(t, PageModels)
	// Real page content must be taller than the viewport so offset survives refresh.
	var models []model.Model
	for i := 0; i < 40; i++ {
		models = append(models, model.Model{
			Online: true, Gen: 1, Name: "Model" + string(rune('A'+i%26)),
			Users: float64(i + 1), Price: 12,
		})
	}
	m.state.Models = models
	m.refreshViewport()
	beforeCur := m.modelCursor
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	g := nm.(Model)
	if g.vp.YOffset <= 0 {
		t.Fatalf("models pgdown should scroll, YOffset=%d lines=%d h=%d",
			g.vp.YOffset, g.vp.TotalLineCount(), g.vp.Height)
	}
	if g.modelCursor != beforeCur {
		t.Fatalf("pgdown should not change modelCursor")
	}
}

func TestDialogBlocksScroll(t *testing.T) {
	m := tallViewport(t, PageOverview)
	d := newTrainDialog(m)
	m.dialog = &d
	m.refreshViewport()
	before := m.vp.YOffset
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	g := nm.(Model)
	if g.vp.YOffset != before {
		t.Fatalf("dialog open: scroll must not change offset (%d → %d)", before, g.vp.YOffset)
	}
}

func TestPageChangeGotoTop(t *testing.T) {
	m := tallViewport(t, PageMarket)
	m.vp.PageDown()
	if m.vp.YOffset == 0 {
		t.Fatal("precondition: need non-zero offset")
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	g := nm.(Model)
	if g.vp.YOffset != 0 {
		t.Fatalf("tab page change should GotoTop, YOffset=%d", g.vp.YOffset)
	}
}

func TestTeamKStillHiresNotScroll(t *testing.T) {
	// Team uses k for focus navigation (not viewport scroll).
	m := tallViewport(t, PageTeam)
	beforeOff := m.vp.YOffset
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	g := nm.(Model)
	if g.vp.YOffset != beforeOff {
		t.Fatalf("team k must not scroll")
	}
}

// TestPageBodyFitsViewportWidth guards the blocker where pages laid out with
// full terminal m.width while the viewport is only w-4, clipping cards at 80 cols.
func TestPageBodyFitsViewportWidth(t *testing.T) {
	m := testModel(t)
	m.resize(80, 40)
	// Seed content that historically produced wide team/tech lines.
	m.state.Models = []model.Model{
		{Online: false, Gen: 1, Name: "DraftA", Segment: model.SegConsumer},
		{Online: true, Gen: 2, Name: "LiveModel", Segment: model.SegConsumer, Users: 12345, Price: 12},
	}
	// Representative pages: overview + tech (review report), plus team (long star blurbs).
	pages := []Page{PageOverview, PageDashboard, PageWarRoom, PageTech, PageTeam, PageModels, PageMarket, PageCompute, PageAchievements}
	for _, page := range pages {
		m.page = page
		body := m.contentBody()
		maxW := 0
		var widest string
		for _, line := range strings.Split(body, "\n") {
			w := lipgloss.Width(line)
			if w > maxW {
				maxW = w
				widest = line
			}
		}
		if maxW > m.vp.Width {
			t.Errorf("page %s (%d): body max width %d exceeds vp.Width %d; widest=%q",
				pageNames[page], page, maxW, m.vp.Width, widest)
		}
	}
}
