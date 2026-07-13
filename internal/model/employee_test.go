package model

import "testing"

func TestRankConsts(t *testing.T) {
	if NumRanks != 6 || RankGod != 5 || RankGrunt != 0 {
		t.Fatalf("ranks: grunt=%d god=%d num=%d", RankGrunt, RankGod, NumRanks)
	}
}

func TestPrimaryRoleFromStats(t *testing.T) {
	var s [NumRoles]int
	s[RoleEngineer] = 80
	s[RoleResearcher] = 40
	if PrimaryRoleFromStats(s) != RoleEngineer {
		t.Fatalf("want engineer")
	}
	// tie → lower role index wins
	s = [NumRoles]int{50, 50, 10, 10}
	if PrimaryRoleFromStats(s) != RoleResearcher {
		t.Fatalf("tie want researcher, got %v", PrimaryRoleFromStats(s))
	}
}

func TestEmployeeCommandsAreCommand(t *testing.T) {
	var cs []Command = []Command{
		UpgradeOffice{},
		HireEmployee{CandidateID: "c1"},
		FireEmployee{EmployeeID: "e1"},
		RerollMarket{},
	}
	if len(cs) != 4 {
		t.Fatal("commands")
	}
}

func TestGameStateEmployeeFields(t *testing.T) {
	var s GameState
	s.Office.Level = 1
	s.Employees = append(s.Employees, Employee{ID: "e", Rank: RankStaff, MonthlySalary: 2500})
	s.Market.Candidates = append(s.Market.Candidates, Employee{ID: "c"})
	if s.Office.Level != 1 || len(s.Employees) != 1 || len(s.Market.Candidates) != 1 {
		t.Fatalf("%+v", s)
	}
}
