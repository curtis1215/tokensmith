package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func testModel(t *testing.T) Model {
	m := newAt(filepath.Join(t.TempDir(), "s.json"))
	m.poller = ingestEmptyPoller(t)
	return m
}

func TestTabCyclesPages(t *testing.T) {
	m := testModel(t)
	if m.page != PageOverview {
		t.Fatalf("start page = %v, want overview", m.page)
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if nm.(Model).page != PageWarRoom {
		t.Fatalf("after Tab = %v, want war room", nm.(Model).page)
	}
}

func TestNumberKeyJumpsPage(t *testing.T) {
	m := testModel(t)
	// After renumber: 1總覽 2戰情 3模型 4市場 5算力 6團隊 7科技 8成就
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'6'}})
	if nm.(Model).page != PageTeam {
		t.Fatalf("key 6 = %v, want team", nm.(Model).page)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if nm.(Model).page != PageWarRoom {
		t.Fatalf("key 2 = %v, want war room", nm.(Model).page)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'8'}})
	if nm.(Model).page != PageAchievements {
		t.Fatalf("key 8 = %v, want achievements", nm.(Model).page)
	}
}

func TestNumPagesIsEight(t *testing.T) {
	if numPages != 8 {
		t.Fatalf("numPages=%d want 8", numPages)
	}
	if pageNames[1] != "戰情室" {
		t.Fatalf("pageNames[1]=%q", pageNames[1])
	}
}

func TestViewHasChrome(t *testing.T) {
	m := testModel(t)
	v := m.View()
	if !strings.Contains(v, "Tokensmith") || !strings.Contains(v, "總覽") || !strings.Contains(v, "團隊") {
		t.Fatalf("view missing chrome:\n%s", v)
	}
}

func TestTabBarListsWarRoom(t *testing.T) {
	m := testModel(t)
	v := m.View()
	if !strings.Contains(v, "戰情室") {
		t.Fatalf("tab bar missing 戰情室:\n%s", v)
	}
}

func TestWarRoomPageKeys(t *testing.T) {
	m := testModel(t)
	m.page = PageWarRoom
	if keys := pageKeys(m); !strings.Contains(keys, "[1]總覽") {
		t.Fatalf("war room keys missing [1]總覽: %q", keys)
	}
	m = pendingChipShortage(m)
	m.page = PageWarRoom
	if keys := pageKeys(m); !strings.Contains(keys, "[e]") {
		t.Fatalf("war room pending should show [e]: %q", keys)
	}
}

func TestOverviewPageKeysPending(t *testing.T) {
	m := pendingChipShortage(testModel(t))
	m.page = PageOverview
	keys := pageKeys(m)
	if !strings.Contains(keys, "[e]") {
		t.Fatalf("overview pending should show [e]: %q", keys)
	}
	for _, want := range []string{"[c]公司策略", "[t]訓練"} {
		if !strings.Contains(keys, want) {
			t.Fatalf("overview help missing %q: %q", want, keys)
		}
	}
}

func TestProgressBar(t *testing.T) {
	got := Bar(0.5, 10)
	full := strings.Count(got, "█")
	if full != 5 {
		t.Fatalf("Bar(0.5,10) filled=%d, want 5 (%q)", full, got)
	}
}
