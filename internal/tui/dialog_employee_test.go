package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"tokensmith/internal/model"
)

func TestSkillEffectBlurbIncludesHooks(t *testing.T) {
	m := testModel(t)
	sk, ok := m.cfg.Skills[0], false
	for _, s := range m.cfg.Skills {
		if s.ID == "m-thrifty" {
			sk = s
			ok = true
			break
		}
	}
	if !ok {
		t.Fatal("m-thrifty missing from catalog")
	}
	blurb := skillEffectBlurb(sk)
	if !strings.Contains(blurb, "月薪") {
		t.Fatalf("blurb=%q", blurb)
	}
}

func TestEmployeeDetailDialogShowsSkillNames(t *testing.T) {
	m := testModel(t)
	m.page = PageTeam
	m.teamFocusRoster = true
	m.state.Employees = []model.Employee{{
		ID: "e1", Name: "測試官", Rank: model.RankManager,
		PrimaryRole:   model.RoleResearcher,
		Stats:         [model.NumRoles]int{70, 40, 40, 40},
		SkillIDs:      []string{"m-deep-research", "m-thrifty"},
		MonthlySalary: 15000,
	}}
	m.rosterCursor = 0
	d, ok := newEmployeeDetailDialog(m)
	if !ok {
		t.Fatal("expected dialog")
	}
	out := renderEmployeeDetailDialog(d, m)
	for _, w := range []string{"測試官", "經理", "研發", "深潛研究", "精算師", "技能", "月薪"} {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in:\n%s", w, out)
		}
	}
}

func TestEmployeeDetailDialogMarketCandidate(t *testing.T) {
	m := testModel(t)
	m.page = PageTeam
	m.teamFocusRoster = false
	m.state.Market.Candidates = []model.Employee{{
		ID: "c1", Name: "候選人A", Rank: model.RankDirector,
		PrimaryRole: model.RoleEngineer,
		Stats:       [model.NumRoles]int{50, 80, 50, 50},
		SkillIDs:    []string{"d-infra-scale", "d-lab-lead"},
		HireCost:    80000, MonthlySalary: 40000,
	}}
	m.marketCursor = 0
	d, ok := newEmployeeDetailDialog(m)
	if !ok || !d.fromMarket {
		t.Fatalf("ok=%v fromMarket=%v", ok, d.fromMarket)
	}
	out := renderEmployeeDetailDialog(d, m)
	for _, w := range []string{"候選人A", "總監", "簽約", "基建擴張", "實驗室主導"} {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in:\n%s", w, out)
		}
	}
}

func TestTeamEnterOpensEmployeeDetailAndEscCloses(t *testing.T) {
	m := testModel(t)
	m.page = PageTeam
	m.teamFocusRoster = false
	m.state.Market.Candidates = []model.Employee{{
		ID: "c1", Name: "開窗測", Rank: model.RankManager,
		SkillIDs: []string{"m-growth-hacks"}, MonthlySalary: 12000, HireCost: 20000,
		PrimaryRole: model.RoleMarketing, Stats: [model.NumRoles]int{20, 20, 20, 70},
	}}
	m.marketCursor = 0
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	g := nm.(Model)
	if g.employeeDetail == nil {
		t.Fatal("enter should open employee detail dialog")
	}
	if !strings.Contains(renderEmployeeDetailDialog(*g.employeeDetail, g), "成長黑客") {
		t.Fatalf("dialog missing skill:\n%s", renderEmployeeDetailDialog(*g.employeeDetail, g))
	}
	nm, _ = g.Update(tea.KeyMsg{Type: tea.KeyEsc})
	g = nm.(Model)
	if g.employeeDetail != nil {
		t.Fatal("esc should close dialog")
	}
}

func TestEmployeeDetailEmptySelectionNoOpen(t *testing.T) {
	m := testModel(t)
	m.page = PageTeam
	m.teamFocusRoster = true
	m.state.Employees = nil
	_, ok := newEmployeeDetailDialog(m)
	if ok {
		t.Fatal("empty roster should not open")
	}
}
