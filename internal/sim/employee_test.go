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
