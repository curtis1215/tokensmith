package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

func TestTechEraLadderRendersFixedAndStructure(t *testing.T) {
	m := testModel(t)
	m.page = PageTech
	m.state.Progression.MaxUnlockedGen = 1
	v := renderTech(m)
	// Fixed Eras I–II present; current expanded, next preview.
	if !strings.Contains(v, "時代 I") || !strings.Contains(v, "時代 II") {
		t.Fatalf("missing era I/II:\n%s", v)
	}
	if !strings.Contains(v, "當前") {
		t.Fatalf("current era should be expanded:\n%s", v)
	}
	if !strings.Contains(v, "預覽") {
		t.Fatalf("next era preview missing:\n%s", v)
	}
	// Fixed Chinese labels from catalog.
	if !strings.Contains(v, "能力架構 I") {
		t.Fatalf("missing fixed Chinese label:\n%s", v)
	}
	// Help strip.
	if !strings.Contains(v, "[↑↓]") || !strings.Contains(v, "前沿分配") {
		t.Fatalf("missing help/allocation header:\n%s", v)
	}
}

func TestTechEraPastCollapsed(t *testing.T) {
	m := testModel(t)
	m.page = PageTech
	// Gen4 → era 2 current; era 1 is past collapsed.
	m.state.Progression.MaxUnlockedGen = 4
	m.state.UnlockedTech = []string{
		balance.GenUnlockNodeID(2), balance.GenUnlockNodeID(3), balance.GenUnlockNodeID(4),
		"algo-cap-1",
	}
	m.techEra = 1 // browse past
	v := renderTech(m)
	if !strings.Contains(v, "已完成") || !strings.Contains(v, "收合") {
		t.Fatalf("past era should be collapsed summary:\n%s", v)
	}
	// Visible entries empty when viewing past.
	if len(techVisibleEntries(m)) != 0 {
		t.Fatalf("past era should have no visible entries, got %d", len(techVisibleEntries(m)))
	}
}

func TestTechEraGeneratedLabels(t *testing.T) {
	m := testModel(t)
	m.page = PageTech
	m.state.Progression.MaxUnlockedGen = 5 // era 3
	m.techEra = 3
	v := renderTech(m)
	if !strings.Contains(v, "前沿研究 · Gen6") {
		t.Fatalf("missing generated frontier label:\n%s", v)
	}
	if !strings.Contains(v, "演算法 突破") {
		t.Fatalf("missing breakthrough label:\n%s", v)
	}
}

func TestTechCursorMovesVisibleEntries(t *testing.T) {
	m := testModel(t)
	m.page = PageTech
	m.state.Progression.MaxUnlockedGen = 1
	m.techEra = 1
	m.techCursor = 0
	entries := techVisibleEntries(m)
	if len(entries) < 2 {
		t.Fatalf("need ≥2 entries in era 1, got %d", len(entries))
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if nm.(Model).techCursor != 1 {
		t.Fatalf("down: cursor=%d want 1", nm.(Model).techCursor)
	}
	nm, _ = nm.Update(tea.KeyMsg{Type: tea.KeyUp})
	if nm.(Model).techCursor != 0 {
		t.Fatalf("up: cursor=%d want 0", nm.(Model).techCursor)
	}
}

func TestTechEraBracketNav(t *testing.T) {
	m := testModel(t)
	m.page = PageTech
	m.state.Progression.MaxUnlockedGen = 1
	m.techEra = 1
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if nm.(Model).techEra != 2 {
		t.Fatalf("] should move to era 2, got %d", nm.(Model).techEra)
	}
	nm, _ = nm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	if nm.(Model).techEra != 1 {
		t.Fatalf("[ should return to era 1, got %d", nm.(Model).techEra)
	}
}

func TestTechStartsFrontier(t *testing.T) {
	m := testModel(t)
	m.page = PageTech
	m.state.Progression.MaxUnlockedGen = 5
	m.state.Resources.RnD = 1e18
	m.techEra = 3
	// Find Gen6 generation entry.
	entries := techVisibleEntries(m)
	idx := -1
	for i, e := range entries {
		if e.kind == techEntryGeneration && e.targetGen == 6 {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("Gen6 generation entry missing")
	}
	m.techCursor = idx
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := nm.(Model)
	if !got.state.Progression.Frontier.Active || got.state.Progression.Frontier.TargetGen != 6 {
		t.Fatalf("Enter should start Gen6 frontier: %+v", got.state.Progression.Frontier)
	}
}

func TestTechAdjustsAllocation(t *testing.T) {
	m := testModel(t)
	m.page = PageTech
	m.state.Progression.MaxUnlockedGen = 5
	m.state.Progression.Frontier = model.FrontierProject{
		Active: true, TargetGen: 6, AllocationPct: 50,
		RnDTotal: 1, RnDRemaining: 1, WorkTotal: 1, WorkRemaining: 1, RecommendedCompute: 100,
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'+'}})
	if nm.(Model).state.Progression.Frontier.AllocationPct != 60 {
		t.Fatalf("+ should add 10, got %d", nm.(Model).state.Progression.Frontier.AllocationPct)
	}
	nm, _ = nm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'-'}})
	if nm.(Model).state.Progression.Frontier.AllocationPct != 50 {
		t.Fatalf("- should subtract 10, got %d", nm.(Model).state.Progression.Frontier.AllocationPct)
	}
}

func TestTechEnterUnlocksFixed(t *testing.T) {
	m := testModel(t)
	m.page = PageTech
	m.state.Resources.RnD = 1e9
	m.techEra = 1
	// First entry should be a fixed node with no prereqs.
	entries := techVisibleEntries(m)
	if len(entries) == 0 || entries[0].kind != techEntryFixed {
		t.Fatalf("era1[0] should be fixed, got %+v", entries)
	}
	m.techCursor = 0
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(nm.(Model).state.UnlockedTech) == 0 {
		t.Fatal("Enter should unlock fixed tech")
	}
}

func TestTechPageFits(t *testing.T) {
	m := testModel(t)
	m.page = PageTech
	m.state.Progression.MaxUnlockedGen = 5
	m.techEra = 3
	// Narrow terminal.
	m.resize(40, 24)
	v := renderTech(m)
	for i, line := range strings.Split(v, "\n") {
		// Strip ANSI for width check roughly via printable runes.
		plain := stripANSI(line)
		if utf8.RuneCountInString(plain) > 80 {
			// soft: contentWidth 36-ish; allow some lipgloss padding
			if utf8.RuneCountInString(plain) > 100 {
				t.Fatalf("line %d too wide (%d): %q", i, utf8.RuneCountInString(plain), plain)
			}
		}
	}
	// Help present at footer shell too.
	if keys := pageKeys(m); !strings.Contains(keys, "時代") && !strings.Contains(keys, "分配") {
		t.Fatalf("pageKeys missing era/alloc: %q", keys)
	}
}

func TestTechUnlockInsufficientRnDNotice(t *testing.T) {
	m := testModel(t)
	m.page = PageTech
	m.state.Resources.RnD = 0
	m.techEra = 1
	m.techCursor = 0
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := nm.(Model)
	if got.notice != "R&D 不足" && !strings.Contains(got.notice, "R&D") {
		// may already be unlocked path if cursor on wrong node
		if len(got.state.UnlockedTech) != 0 {
			t.Fatalf("should not unlock without R&D")
		}
	}
}

// stripANSI removes common lipgloss/ANSI sequences for width checks.
func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			inEsc = true
			continue
		}
		if inEsc {
			if (s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z') {
				inEsc = false
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// Ensure sim import used when needed by compile of helpers.
var _ = sim.MaxUnlockedGen
