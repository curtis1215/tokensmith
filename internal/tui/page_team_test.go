package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"tokensmith/internal/model"
)

func TestTeamPageShowsOfficeSeatsAndPayroll(t *testing.T) {
	m := testModel(t)
	m.page = PageTeam
	m.state.Office.Level = 1
	m.state.Employees = []model.Employee{
		{
			ID: "e1", Name: "林明遠", Rank: model.RankStaff,
			PrimaryRole:   model.RoleResearcher,
			Stats:         [model.NumRoles]int{40, 10, 10, 10},
			SkillIDs:      []string{"m-deep-research"},
			MonthlySalary: 2500,
		},
	}
	m.state.Market.Candidates = []model.Employee{
		{
			ID: "c1", Name: "陳雅婷", Rank: model.RankGrunt,
			PrimaryRole: model.RoleEngineer,
			Stats:       [model.NumRoles]int{10, 30, 10, 10},
			HireCost:    800, MonthlySalary: 800,
		},
	}
	v := renderTeam(m)
	for _, w := range []string{
		"辦公室", "車庫", "工位", "月薪",
		"林明遠", "職員", "研發", "陳雅婷", "雜魚", "工程",
		"人才市場", "重抽", "在職",
	} {
		if !strings.Contains(v, w) {
			t.Errorf("team page missing %q:\n%s", w, v)
		}
	}
	// Seat line shape: 工位 a/b
	if !strings.Contains(v, "工位 1/3") && !strings.Contains(v, "工位1/3") {
		// allow spaced or tight; require a/b digits near 工位
		if !strings.Contains(v, "1/3") {
			t.Errorf("expected 工位 seats like 1/3:\n%s", v)
		}
	}
	// Monthly unit label
	if !strings.Contains(v, "/月") {
		t.Errorf("expected /月 salary unit:\n%s", v)
	}
}

func TestTeamPageRankAndRoleZH(t *testing.T) {
	if rankNameZH(model.RankGrunt) != "雜魚" || rankNameZH(model.RankGod) != "大神" {
		t.Fatalf("rank ZH map wrong: grunt=%q god=%q", rankNameZH(model.RankGrunt), rankNameZH(model.RankGod))
	}
	if roleNameZH(model.RoleResearcher) != "研發" || roleNameZH(model.RoleMarketing) != "行銷" {
		t.Fatalf("role ZH wrong")
	}
}

func TestTeamKeyUpgradeOffice(t *testing.T) {
	m := testModel(t)
	m.page = PageTeam
	m.state.Office.Level = 1
	m.state.Resources.Cash = 100_000
	before := m.state.Resources.Cash
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	g := nm.(Model)
	if g.state.Office.Level != 2 {
		t.Fatalf("upgrade u: level=%d want 2", g.state.Office.Level)
	}
	if g.state.Resources.Cash >= before {
		t.Fatalf("upgrade should spend cash: before=%v after=%v", before, g.state.Resources.Cash)
	}
	if g.notice == "" {
		t.Fatalf("upgrade should set notice, got empty")
	}
}

func TestTeamKeyHireFocusedCandidate(t *testing.T) {
	m := testModel(t)
	m.page = PageTeam
	m.teamFocusRoster = false // market pane
	m.state.Office.Level = 1
	m.state.Resources.Cash = 100_000
	m.state.Employees = nil
	m.state.Market.Candidates = []model.Employee{
		{ID: "c1", Name: "甲", HireCost: 100, MonthlySalary: 500, Rank: model.RankGrunt, PrimaryRole: model.RoleOps},
		{ID: "c2", Name: "乙", HireCost: 100, MonthlySalary: 500, Rank: model.RankStaff, PrimaryRole: model.RoleMarketing},
	}
	m.marketCursor = 1
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	g := nm.(Model)
	if len(g.state.Employees) != 1 || g.state.Employees[0].ID != "c2" {
		t.Fatalf("hire focused: emps=%+v", g.state.Employees)
	}
	if len(g.state.Market.Candidates) != 1 || g.state.Market.Candidates[0].ID != "c1" {
		t.Fatalf("remaining candidates=%+v", g.state.Market.Candidates)
	}
}

func TestTeamKeyHireNoOpOnRosterFocus(t *testing.T) {
	m := testModel(t)
	m.page = PageTeam
	m.teamFocusRoster = true
	m.state.Resources.Cash = 100_000
	m.state.Market.Candidates = []model.Employee{
		{ID: "c1", Name: "甲", HireCost: 100, MonthlySalary: 500},
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	g := nm.(Model)
	if len(g.state.Employees) != 0 {
		t.Fatalf("hire on roster focus should no-op, emps=%d", len(g.state.Employees))
	}
	if !strings.Contains(g.notice, "人才市場") {
		t.Fatalf("notice=%q", g.notice)
	}
}

func TestTeamKeyFireFocusedEmployee(t *testing.T) {
	m := testModel(t)
	m.page = PageTeam
	m.teamFocusRoster = true // roster pane required
	m.state.Resources.Cash = 100_000
	m.state.Employees = []model.Employee{
		{ID: "e1", Name: "甲", MonthlySalary: 1000, Rank: model.RankStaff},
		{ID: "e2", Name: "乙", MonthlySalary: 1000, Rank: model.RankLead},
	}
	m.rosterCursor = 1
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	g := nm.(Model)
	if len(g.state.Employees) != 1 || g.state.Employees[0].ID != "e1" {
		t.Fatalf("fire focused: emps=%+v", g.state.Employees)
	}
	if !strings.Contains(g.notice, "遣散") {
		t.Fatalf("fire notice should quote severance: %q", g.notice)
	}
}

func TestTeamKeyFireNoOpOnMarketFocus(t *testing.T) {
	m := testModel(t)
	m.page = PageTeam
	m.teamFocusRoster = false
	m.state.Resources.Cash = 100_000
	m.state.Employees = []model.Employee{
		{ID: "e1", Name: "甲", MonthlySalary: 1000},
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	g := nm.(Model)
	if len(g.state.Employees) != 1 {
		t.Fatalf("fire on market focus should no-op")
	}
	if !strings.Contains(g.notice, "名冊") {
		t.Fatalf("notice=%q", g.notice)
	}
}

func TestTeamKeyRerollMarket(t *testing.T) {
	m := testModel(t)
	m.page = PageTeam
	m.state.Resources.Cash = 100_000
	m.state.Market = model.TalentMarket{
		RandState:     7,
		NextRefreshAt: 999,
		Candidates: []model.Employee{
			{ID: "old", Name: "舊人", HireCost: 1, MonthlySalary: 1},
		},
	}
	// Ensure enough cash for base reroll ($5k)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	g := nm.(Model)
	if g.state.Market.RerollCount != 1 {
		t.Fatalf("reroll count=%d want 1", g.state.Market.RerollCount)
	}
	if len(g.state.Market.Candidates) == 0 {
		t.Fatalf("reroll should refill candidates")
	}
	for _, c := range g.state.Market.Candidates {
		if c.ID == "old" {
			t.Fatalf("old candidate should be gone")
		}
	}
}

func TestTeamJKMovesMarketCursor(t *testing.T) {
	m := testModel(t)
	m.page = PageTeam
	m.teamFocusRoster = false
	m.state.Market.Candidates = []model.Employee{
		{ID: "c1", Name: "一"},
		{ID: "c2", Name: "二"},
		{ID: "c3", Name: "三"},
	}
	m.marketCursor = 0
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	g := nm.(Model)
	if g.marketCursor != 1 {
		t.Fatalf("j should advance marketCursor, got %d", g.marketCursor)
	}
	nm, _ = g.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	g = nm.(Model)
	if g.marketCursor != 0 {
		t.Fatalf("k should retreat marketCursor, got %d", g.marketCursor)
	}
}
