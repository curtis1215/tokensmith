package sim

import (
	"errors"
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestApplyUpgradeOffice(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Office: model.Office{Level: 1}, Resources: model.Resources{Cash: 100_000}}
	ns, err := Apply(s, model.UpgradeOffice{}, b)
	if err != nil || ns.Office.Level != 2 {
		t.Fatalf("err=%v level=%d", err, ns.Office.Level)
	}
	if ns.Resources.Cash != 100_000-25000 {
		t.Fatal(ns.Resources.Cash)
	}
	// input not mutated
	if s.Office.Level != 1 || s.Resources.Cash != 100_000 {
		t.Fatal("Apply mutated input")
	}
}

func TestApplyUpgradeOfficeMaxed(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Office:    model.Office{Level: b.MaxOfficeLevel},
		Resources: model.Resources{Cash: 1e12},
	}
	if _, err := Apply(s, model.UpgradeOffice{}, b); !errors.Is(err, ErrOfficeMaxed) {
		t.Fatalf("got %v", err)
	}
}

func TestApplyUpgradeOfficeInsufficientCash(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Office:    model.Office{Level: 1},
		Resources: model.Resources{Cash: 100},
	}
	if _, err := Apply(s, model.UpgradeOffice{}, b); !errors.Is(err, ErrInsufficientCash) {
		t.Fatalf("got %v", err)
	}
}

func TestApplyHireAndFire(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Office:    model.Office{Level: 1},
		Resources: model.Resources{Cash: 1e9},
		Market: model.TalentMarket{
			Candidates: []model.Employee{{
				ID: "c1", HireCost: 1000, MonthlySalary: 2000, Rank: model.RankStaff,
				Stats: [model.NumRoles]int{30, 10, 10, 10}, PrimaryRole: model.RoleResearcher,
			}},
			RandState: 1,
		},
	}
	ns, err := Apply(s, model.HireEmployee{CandidateID: "c1"}, b)
	if err != nil || len(ns.Employees) != 1 || len(ns.Market.Candidates) != 0 {
		t.Fatalf("hire err=%v emp=%d cand=%d", err, len(ns.Employees), len(ns.Market.Candidates))
	}
	if ns.Resources.Cash != 1e9-1000 {
		t.Fatalf("hire cash=%v want %v", ns.Resources.Cash, 1e9-1000)
	}
	// severance 0.5 * 2000 = 1000
	cashAfterHire := ns.Resources.Cash
	ns2, err := Apply(ns, model.FireEmployee{EmployeeID: "c1"}, b)
	if err != nil || len(ns2.Employees) != 0 {
		t.Fatalf("fire err=%v emp=%d", err, len(ns2.Employees))
	}
	if ns2.Resources.Cash != cashAfterHire-1000 {
		t.Fatalf("fire cash=%v want %v", ns2.Resources.Cash, cashAfterHire-1000)
	}
}

func TestApplyHireNoSeats(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Office:    model.Office{Level: 1},
		Resources: model.Resources{Cash: 1e9},
		Employees: []model.Employee{{ID: "a"}, {ID: "b"}, {ID: "c"}},
		Market:    model.TalentMarket{Candidates: []model.Employee{{ID: "c1", HireCost: 1}}},
	}
	if _, err := Apply(s, model.HireEmployee{CandidateID: "c1"}, b); !errors.Is(err, ErrNoSeats) {
		t.Fatalf("got %v", err)
	}
}

func TestApplyHireUnknownCandidate(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Office:    model.Office{Level: 1},
		Resources: model.Resources{Cash: 1e9},
		Market:    model.TalentMarket{Candidates: []model.Employee{{ID: "c1", HireCost: 1}}},
	}
	if _, err := Apply(s, model.HireEmployee{CandidateID: "missing"}, b); !errors.Is(err, ErrUnknownCandidate) {
		t.Fatalf("got %v", err)
	}
}

func TestApplyFireUnknownEmployee(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Office:    model.Office{Level: 1},
		Resources: model.Resources{Cash: 1e9},
		Employees: []model.Employee{{ID: "a", MonthlySalary: 100}},
	}
	if _, err := Apply(s, model.FireEmployee{EmployeeID: "missing"}, b); !errors.Is(err, ErrUnknownEmployee) {
		t.Fatalf("got %v", err)
	}
}

func TestApplyRerollEscalates(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Office: model.Office{Level: 1}, Resources: model.Resources{Cash: 1e9},
		Market: model.TalentMarket{RandState: 3, NextRefreshAt: 999},
	}
	s = RefreshMarket(s, b)
	// free timer is independent of paid reroll; pin and assert preservation
	s.Market.NextRefreshAt = 999
	beforeCash := s.Resources.Cash
	ns, err := Apply(s, model.RerollMarket{}, b)
	if err != nil || ns.Market.RerollCount != 1 {
		t.Fatalf("err=%v n=%d", err, ns.Market.RerollCount)
	}
	if ns.Market.NextRefreshAt != 999 {
		t.Fatal("reroll must not reset free timer")
	}
	wantCost := balance.RerollCost(0, b)
	if ns.Resources.Cash != beforeCash-wantCost {
		t.Fatalf("cash=%v want %v (cost %v)", ns.Resources.Cash, beforeCash-wantCost, wantCost)
	}
	// second paid reroll escalates
	ns2, err := Apply(ns, model.RerollMarket{}, b)
	if err != nil || ns2.Market.RerollCount != 2 {
		t.Fatalf("second err=%v n=%d", err, ns2.Market.RerollCount)
	}
	if ns2.Market.NextRefreshAt != 999 {
		t.Fatal("second reroll must not reset free timer")
	}
	wantCost2 := balance.RerollCost(1, b)
	if ns2.Resources.Cash != ns.Resources.Cash-wantCost2 {
		t.Fatalf("second cash delta: got %v want %v", beforeCash-ns2.Resources.Cash, wantCost+wantCost2)
	}
}
