package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"tokensmith/internal/balance"
)

func TestTechPageListsNodesAndUnlocks(t *testing.T) {
	m := testModel(t)
	m.page = PageTech
	m.state.Resources.RnD = 1e9
	v := renderTech(m)
	if len(m.cfg.TechNodes) > 0 && !strings.Contains(v, m.cfg.TechNodes[0].ID) {
		t.Fatalf("tech page should list node ids:\n%s", v)
	}
	if !strings.Contains(v, "演算法") || !strings.Contains(v, "能力架構 I") {
		t.Fatalf("tech page missing branch or Chinese name:\n%s", v)
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

// Tech ↑↓ must follow on-screen branch order, not flat catalog indices.
// model-gen-2 is catalog index 8 but appears third in the Algo card (after 0,1).
func TestTechCursorFollowsVisualOrder(t *testing.T) {
	m := testModel(t)
	m.page = PageTech
	// From algo-train-1 (catalog idx 1) → next visual is model-gen-2, not infra-eff-1.
	m.techCursor = 1
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	got := nm.(Model).techCursor
	if m.cfg.TechNodes[got].ID != balance.GenUnlockNodeID(2) {
		t.Fatalf("down from algo-train-1: want model-gen-2, got idx=%d id=%s",
			got, m.cfg.TechNodes[got].ID)
	}
	// Up returns to algo-train-1.
	nm, _ = nm.Update(tea.KeyMsg{Type: tea.KeyUp})
	if nm.(Model).techCursor != 1 {
		t.Fatalf("up should return to algo-train-1 (idx 1), got %d", nm.(Model).techCursor)
	}
}

func TestTechUnlockInsufficientRnDNotice(t *testing.T) {
	m := testModel(t)
	m.page = PageTech
	m.state.Resources.RnD = 0
	// cursor 0 is algo-cap-1 (15k R&D)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := nm.(Model)
	if got.notice != "R&D 不足" {
		t.Fatalf("want R&D notice, got %q", got.notice)
	}
	if len(got.state.UnlockedTech) != 0 {
		t.Fatalf("should not unlock without R&D")
	}
}
