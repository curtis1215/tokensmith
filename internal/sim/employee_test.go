package sim

import (
	"math"
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestEmployeeRolePowerPrimarySecondary(t *testing.T) {
	b := balance.Default()
	e := model.Employee{
		PrimaryRole: model.RoleResearcher,
		Stats:       [model.NumRoles]int{80, 40, 40, 40},
	}
	p := employeeRolePower(e, b)
	if p[model.RoleResearcher] <= p[model.RoleEngineer] {
		t.Fatalf("primary should dominate: %+v", p)
	}
}

func TestSeatCapAndFull(t *testing.T) {
	b := balance.Default()
	ns := model.GameState{Office: model.Office{Level: 1}}
	if seatCap(ns, b) != 3 {
		t.Fatal(seatCap(ns, b))
	}
	ns.Employees = make([]model.Employee, 3)
	if !rosterFull(ns, b) {
		t.Fatal("full")
	}
}

func TestSalaryPerSec(t *testing.T) {
	b := balance.Default()
	ns := model.GameState{Employees: []model.Employee{{MonthlySalary: 6000}}}
	got := totalSalaryPerSecFromEmployees(ns, b)
	want := balance.MonthlyToPerSec(6000, b)
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("got %v want %v", got, want)
	}
	// TUI tickDT=3600 must not deduct multi-month pay for a modest salary.
	tickBurn := got * 3600
	if tickBurn > 20 {
		t.Fatalf("tick burn too high: %v (SecondsPerMonth=%v)", tickBurn, b.SecondsPerMonth)
	}
}

func TestEffectiveMonthlySalaryQuotesSkillMults(t *testing.T) {
	b := balance.Default()
	// m-thrifty SelfSalaryMult 0.92; d-comp-opt CompanySalaryMult 0.96
	ns := model.GameState{
		Employees: []model.Employee{
			{ID: "a", MonthlySalary: 10000, SkillIDs: []string{"m-thrifty"}},
			{ID: "b", MonthlySalary: 10000, SkillIDs: []string{"d-comp-opt"}},
		},
	}
	// Company mult applies to both: 0.96
	// a: 10000 * 0.92 * 0.96 = 8832
	// b: 10000 * 1 * 0.96 = 9600
	if !approx(EffectiveMonthlySalary(ns.Employees[0], ns, b), 8832) {
		t.Fatalf("a pay=%v", EffectiveMonthlySalary(ns.Employees[0], ns, b))
	}
	if !approx(EffectiveMonthlySalary(ns.Employees[1], ns, b), 9600) {
		t.Fatalf("b pay=%v", EffectiveMonthlySalary(ns.Employees[1], ns, b))
	}
	if !approx(TotalMonthlyPayroll(ns, b), 8832+9600) {
		t.Fatalf("payroll=%v", TotalMonthlyPayroll(ns, b))
	}
	// Severance uses SeveranceMult families only — thrifty is salary, not severance.
	if !approx(SeveranceQuote(ns.Employees[0], ns, b), 10000*0.5) {
		t.Fatalf("sev a=%v want 5000", SeveranceQuote(ns.Employees[0], ns, b))
	}
}

func TestEffectiveMonthlySalaryForHireIncludesCandidateCompanyMult(t *testing.T) {
	b := balance.Default()
	ns := model.GameState{Employees: nil}
	cand := model.Employee{
		ID: "c1", MonthlySalary: 10000, SkillIDs: []string{"d-comp-opt"}, // CompanySalaryMult 0.96
	}
	// Pre-hire quote must match post-hire payroll (candidate brings company mult).
	pre := EffectiveMonthlySalaryForHire(cand, ns, b)
	if !approx(pre, 9600) {
		t.Fatalf("pre-hire quote=%v want 9600", pre)
	}
	hired := ns
	hired.Employees = []model.Employee{cand}
	post := TotalMonthlyPayroll(hired, b)
	if !approx(pre, post) {
		t.Fatalf("pre-hire %v != post-hire %v", pre, post)
	}
}

func TestRoleBonusZeroNeutral(t *testing.T) {
	b := balance.Default()
	if roleBonus(0, b) != 0 {
		t.Fatal(roleBonus(0, b))
	}
}

func TestPassiveSkillEffectsNeutral(t *testing.T) {
	p := passiveSkillEffects(model.GameState{}, balance.Default())
	if p.TokenRnDMult != 1 || p.InfraMult != 1 || p.UserGrowthMult != 1 ||
		p.ChurnMult != 1 || p.TrainQualityMult != 1 || p.RevenueMult != 1 {
		t.Fatalf("neutral passives expected: %+v", p)
	}
}

func TestPassiveSkillEffectsProduct(t *testing.T) {
	b := balance.Default()
	ns := model.GameState{
		Employees: []model.Employee{{
			SkillIDs: []string{"m-pipeline", "d-qa-gate"}, // TokenRnD 1.02, TrainQ 1.04
		}},
	}
	p := passiveSkillEffects(ns, b)
	if !approx(p.TokenRnDMult, 1.02) {
		t.Fatalf("TokenRnDMult = %v, want 1.02", p.TokenRnDMult)
	}
	if !approx(p.TrainQualityMult, 1.04) {
		t.Fatalf("TrainQualityMult = %v, want 1.04", p.TrainQualityMult)
	}
}

func TestCompanyRolePowerScalesTotal(t *testing.T) {
	b := balance.Default()
	base := model.GameState{
		Employees: []model.Employee{{
			PrimaryRole: model.RoleResearcher,
			Stats:       [model.NumRoles]int{100, 0, 0, 0},
		}},
	}
	with := base
	with.Employees = []model.Employee{{
		PrimaryRole: model.RoleResearcher,
		Stats:       [model.NumRoles]int{100, 0, 0, 0},
		SkillIDs:    []string{"d-lab-lead"}, // CompanyRolePower research +0.06
	}}
	pb := totalRolePower(base, b)[model.RoleResearcher]
	pw := totalRolePower(with, b)[model.RoleResearcher]
	if !approx(pw, pb*1.06) {
		t.Fatalf("company role power: base=%v with=%v want %v", pb, pw, pb*1.06)
	}
}
