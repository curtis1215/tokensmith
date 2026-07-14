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
	if nm.(Model).page != PageDashboard {
		t.Fatalf("after Tab = %v, want dashboard", nm.(Model).page)
	}
}

func TestNumberKeyJumpsPage(t *testing.T) {
	m := testModel(t)
	// 1總覽 2儀表板 3戰情 4模型 5市場 6算力 7團隊 8科技 9成就
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'7'}})
	if nm.(Model).page != PageTeam {
		t.Fatalf("key 7 = %v, want team", nm.(Model).page)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if nm.(Model).page != PageDashboard {
		t.Fatalf("key 2 = %v, want dashboard", nm.(Model).page)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	if nm.(Model).page != PageWarRoom {
		t.Fatalf("key 3 = %v, want war room", nm.(Model).page)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'9'}})
	if nm.(Model).page != PageAchievements {
		t.Fatalf("key 9 = %v, want achievements", nm.(Model).page)
	}
}

func TestNumPagesIsNine(t *testing.T) {
	if numPages != 9 {
		t.Fatalf("numPages=%d want 9", numPages)
	}
	if pageNames[1] != "儀表板" {
		t.Fatalf("pageNames[1]=%q", pageNames[1])
	}
	if pageNames[2] != "戰情室" {
		t.Fatalf("pageNames[2]=%q", pageNames[2])
	}
}

func TestDashboardPageRendersTitles(t *testing.T) {
	m := testModel(t)
	m.page = PageDashboard
	m.width, m.height = 120, 40
	m.resize(m.width, m.height)
	v := m.View()
	for _, want := range []string{"儀表板", "用戶增長", "營收增長", "R&D 增長"} {
		if !strings.Contains(v, want) {
			t.Fatalf("missing %q in:\n%s", want, v)
		}
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
