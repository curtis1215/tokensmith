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
	if math.Abs(got-10) > 1e-9 {
		t.Fatalf("got %v", got)
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
