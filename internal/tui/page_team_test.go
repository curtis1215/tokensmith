package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"tokensmith/internal/model"
)

func TestTeamPageShowsRoles(t *testing.T) {
	m := testModel(t)
	m.page = PageTeam
	v := renderTeam(m)
	for _, w := range []string{"研究員", "工程", "營運", "行銷", "明星", "Dr. Aria Chen"} {
		if !strings.Contains(v, w) {
			t.Errorf("team page missing %q:\n%s", w, v)
		}
	}
}

func TestTeamHireResearcher(t *testing.T) {
	m := testModel(t)
	m.page = PageTeam
	m.state.Resources.Cash = 1e6
	before := m.state.Research.Researchers[model.Tier1]
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if nm.(Model).state.Research.Researchers[model.Tier1] != before+1 {
		t.Fatalf("h should hire a T1 researcher")
	}
}

func TestTeamSignStar(t *testing.T) {
	m := testModel(t)
	m.page = PageTeam
	m.state.Resources.Cash = 1e12 // afford any star
	if len(m.cfg.Stars) == 0 {
		t.Skip("no stars configured")
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if len(nm.(Model).state.HiredStars) == 0 {
		t.Fatalf("s should sign the first unhired star")
	}
}
