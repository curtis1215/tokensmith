package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestAchievementsPageRenders(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 110, Height: 40})
	m = mm.(Model)
	m.achievements = map[string]int64{"first-online": 1751500000}
	out := renderAchievements(m)
	for _, want := range []string{"成就進度", "1/29", "🏆 首航", "🔒", "進度", "輪迴"} {
		if !strings.Contains(out, want) {
			t.Fatalf("achievements page missing %q", want)
		}
	}
}

func TestAchievementsPageReachableByKey7(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'7'}})
	m = mm.(Model)
	if m.page != PageAchievements {
		t.Fatalf("key 7 should open achievements page, got %v", m.page)
	}
	if !strings.Contains(m.View(), "成就進度") {
		t.Fatal("View should render achievements page")
	}
}
