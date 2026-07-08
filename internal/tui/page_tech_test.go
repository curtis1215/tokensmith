package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTechPageListsNodesAndUnlocks(t *testing.T) {
	m := testModel(t)
	m.page = PageTech
	m.state.Resources.RnD = 1e9
	v := renderTech(m)
	if len(m.cfg.TechNodes) > 0 && !strings.Contains(v, m.cfg.TechNodes[0].ID) {
		t.Fatalf("tech page should list node ids:\n%s", v)
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // unlock node under cursor 0 (no prereqs)
	if len(nm.(Model).state.UnlockedTech) == 0 {
		t.Fatalf("Enter should unlock the selected tech node")
	}
}

func TestTechCursorMoves(t *testing.T) {
	m := testModel(t)
	m.page = PageTech
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if nm.(Model).techCursor != 1 {
		t.Fatalf("down should move tech cursor to 1, got %d", nm.(Model).techCursor)
	}
}
