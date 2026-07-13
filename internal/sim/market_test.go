package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestGenerateL1NeverGodOrDirector(t *testing.T) {
	b := balance.Default()
	st := uint64(1)
	for i := 0; i < 200; i++ {
		var e model.Employee
		e, st = generateEmployee(st, 1, 0, i, b)
		if e.Rank == model.RankGod || e.Rank == model.RankDirector {
			t.Fatalf("L1 rolled %v", e.Rank)
		}
	}
}

func TestGenerateDeterministic(t *testing.T) {
	b := balance.Default()
	e1, _ := generateEmployee(42, 5, 100, 0, b)
	e2, _ := generateEmployee(42, 5, 100, 0, b)
	if e1.ID != e2.ID || e1.Rank != e2.Rank || e1.Stats != e2.Stats {
		t.Fatalf("%+v vs %+v", e1, e2)
	}
}

func TestManagerSkillsNoGod(t *testing.T) {
	b := balance.Default()
	// Force many rolls at L4+; filter RankManager
	st := uint64(7)
	found := 0
	for i := 0; i < 500 && found < 20; i++ {
		var e model.Employee
		e, st = generateEmployee(st, 6, 0, i, b)
		if e.Rank != model.RankManager {
			continue
		}
		found++
		for _, id := range e.SkillIDs {
			sk, ok := balance.SkillByID(b, id)
			if !ok || sk.Tier == model.SkillTierGod {
				t.Fatalf("manager has god skill %s", id)
			}
		}
		if len(e.SkillIDs) != 1 {
			t.Fatalf("manager skills=%d", len(e.SkillIDs))
		}
	}
	if found < 5 {
		t.Fatal("not enough managers sampled")
	}
}

func TestRefreshMarketPoolSize(t *testing.T) {
	b := balance.Default()
	ns := model.GameState{Office: model.Office{Level: 1}, GameTime: 10, Market: model.TalentMarket{RandState: 9}}
	ns = RefreshMarket(ns, b)
	if len(ns.Market.Candidates) != b.MarketPoolSize {
		t.Fatal(len(ns.Market.Candidates))
	}
	if ns.Market.RerollCount != 0 || ns.Market.NextRefreshAt != 10+b.MarketRefreshSec {
		t.Fatalf("%+v", ns.Market)
	}
}

func TestDirectorSkillsNoGodAndHasDirector(t *testing.T) {
	b := balance.Default()
	st := uint64(11)
	found := 0
	for i := 0; i < 800 && found < 15; i++ {
		var e model.Employee
		e, st = generateEmployee(st, 7, 0, i, b)
		if e.Rank != model.RankDirector {
			continue
		}
		found++
		if len(e.SkillIDs) != 2 {
			t.Fatalf("director skills=%d ids=%v", len(e.SkillIDs), e.SkillIDs)
		}
		hasDir := false
		usedFamily := map[string]bool{}
		for _, id := range e.SkillIDs {
			sk, ok := balance.SkillByID(b, id)
			if !ok {
				t.Fatalf("unknown skill %s", id)
			}
			if sk.Tier == model.SkillTierGod {
				t.Fatalf("director has god skill %s", id)
			}
			if sk.Tier == model.SkillTierDirector {
				hasDir = true
			}
			if sk.Family != "" {
				if usedFamily[sk.Family] {
					t.Fatalf("duplicate family %s on %v", sk.Family, e.SkillIDs)
				}
				usedFamily[sk.Family] = true
			}
		}
		if !hasDir {
			t.Fatalf("director missing director-tier skill: %v", e.SkillIDs)
		}
	}
	if found < 3 {
		t.Fatal("not enough directors sampled")
	}
}

func TestGodSkillsConstraints(t *testing.T) {
	b := balance.Default()
	st := uint64(13)
	found := 0
	for i := 0; i < 1000 && found < 12; i++ {
		var e model.Employee
		e, st = generateEmployee(st, 8, 0, i, b)
		if e.Rank != model.RankGod {
			continue
		}
		found++
		if len(e.SkillIDs) != 3 {
			t.Fatalf("god skills=%d ids=%v", len(e.SkillIDs), e.SkillIDs)
		}
		hasGod := false
		sig := 0
		usedFamily := map[string]bool{}
		for _, id := range e.SkillIDs {
			sk, ok := balance.SkillByID(b, id)
			if !ok {
				t.Fatalf("unknown skill %s", id)
			}
			if sk.Tier == model.SkillTierGod {
				hasGod = true
			}
			if sk.Signature {
				sig++
			}
			if sk.Family != "" {
				if usedFamily[sk.Family] {
					t.Fatalf("duplicate family %s on %v", sk.Family, e.SkillIDs)
				}
				usedFamily[sk.Family] = true
			}
		}
		if !hasGod {
			t.Fatalf("god missing god-tier skill: %v", e.SkillIDs)
		}
		if sig > 1 {
			t.Fatalf("god has %d signatures: %v", sig, e.SkillIDs)
		}
	}
	if found < 2 {
		t.Fatal("not enough gods sampled")
	}
}

func TestComputeMonthlySalaryAndHireCost(t *testing.T) {
	b := balance.Default()
	var stats [model.NumRoles]int
	stats[model.RoleEngineer] = 70
	stats[model.RoleResearcher] = 40
	stats[model.RoleOps] = 35
	stats[model.RoleMarketing] = 30
	monthly := computeMonthlySalary(model.RankManager, stats, []string{"m-thrifty"}, 1, b)
	if monthly <= 0 {
		t.Fatalf("monthly=%v", monthly)
	}
	hire := computeHireCost(monthly, b)
	if hire != monthly*b.HireMonths {
		t.Fatalf("hire=%v want %v", hire, monthly*b.HireMonths)
	}
}

func TestEnsureMarketRefreshesWhenEmptyOrExpired(t *testing.T) {
	b := balance.Default()
	ns := model.GameState{
		Office:   model.Office{Level: 1},
		GameTime: 100,
		Market:   model.TalentMarket{RandState: 3, NextRefreshAt: 50},
	}
	ns = ensureMarket(ns, b)
	if len(ns.Market.Candidates) != b.MarketPoolSize {
		t.Fatalf("expired: candidates=%d", len(ns.Market.Candidates))
	}
	// Not expired, keep pool.
	keep := ns.Market.Candidates[0].ID
	ns.GameTime = ns.Market.NextRefreshAt - 1
	ns = ensureMarket(ns, b)
	if ns.Market.Candidates[0].ID != keep {
		t.Fatal("should not refresh before expiry")
	}
}

// TestEmployeeIDsUniqueAcrossRerollSameGameTime guards hire+reroll ID collisions
// (same GameTime reuses seq 0..n-1; IDs must still differ via RandState bits).
func TestEmployeeIDsUniqueAcrossRerollSameGameTime(t *testing.T) {
	b := balance.Default()
	ns := model.GameState{
		Office:    model.Office{Level: 1},
		GameTime:  42,
		Resources: model.Resources{Cash: 1e12},
		Market:    model.TalentMarket{RandState: 99},
	}
	ns = RefreshMarket(ns, b)
	if len(ns.Market.Candidates) == 0 {
		t.Fatal("empty market")
	}
	// Hire first candidate so their ID stays on the roster after reroll.
	hireID := ns.Market.Candidates[0].ID
	var err error
	ns, err = Apply(ns, model.HireEmployee{CandidateID: hireID}, b)
	if err != nil {
		t.Fatalf("hire: %v", err)
	}
	// Same GameTime paid reroll (regenerateCandidatesOnly).
	ns.Market.NextRefreshAt = 9999
	ns, err = Apply(ns, model.RerollMarket{}, b)
	if err != nil {
		t.Fatalf("reroll: %v", err)
	}
	seen := map[string]bool{hireID: true}
	for _, c := range ns.Market.Candidates {
		if seen[c.ID] {
			t.Fatalf("duplicate ID %q after hire+reroll at same GameTime", c.ID)
		}
		seen[c.ID] = true
	}
	// Sequential generateEmployee with same gameTime, different seq still unique.
	st := uint64(12345)
	e1, st := generateEmployee(st, 1, 100, 0, b)
	e2, _ := generateEmployee(st, 1, 100, 1, b)
	if e1.ID == e2.ID {
		t.Fatalf("sequential generate same gameTime collided: %q", e1.ID)
	}
	// Same inputs still deterministic.
	a, _ := generateEmployee(77, 3, 50, 2, b)
	bb, _ := generateEmployee(77, 3, 50, 2, b)
	if a.ID != bb.ID {
		t.Fatalf("determinism broken: %q vs %q", a.ID, bb.ID)
	}
}

func TestMarketRarityBonusRaisesOfficeLevel(t *testing.T) {
	b := balance.Default()
	base := model.GameState{Office: model.Office{Level: 1}}
	if marketOfficeLevel(base, b) != 1 {
		t.Fatalf("base level=%d", marketOfficeLevel(base, b))
	}
	// d-talent-magnet = +0.75 → rounds to +1 effective market level.
	with := model.GameState{
		Office: model.Office{Level: 1},
		Employees: []model.Employee{{
			SkillIDs: []string{"d-talent-magnet"},
		}},
	}
	if marketOfficeLevel(with, b) != 2 {
		t.Fatalf("with magnet level=%d want 2", marketOfficeLevel(with, b))
	}
	// Soft-cap +2 even if stacked beyond.
	stacked := model.GameState{
		Office: model.Office{Level: 3},
		Employees: []model.Employee{
			{SkillIDs: []string{"d-talent-magnet"}},
			{SkillIDs: []string{"g-talent-blackhole"}},
			{SkillIDs: []string{"d-talent-magnet"}}, // extra stack
		},
	}
	if marketOfficeLevel(stacked, b) != 5 { // 3 + min(0.75+1+0.75, 2)=3+2
		t.Fatalf("soft-cap level=%d want 5", marketOfficeLevel(stacked, b))
	}
}

func TestHireAndRerollCostQuotes(t *testing.T) {
	b := balance.Default()
	cand := model.Employee{HireCost: 1000}
	plain := model.GameState{}
	if HireCostQuote(plain, cand, b) != 1000 {
		t.Fatalf("plain hire quote=%v", HireCostQuote(plain, cand, b))
	}
	// d-hiring-blitz HireCostMult 0.90
	disc := model.GameState{
		Employees: []model.Employee{{SkillIDs: []string{"d-hiring-blitz"}}},
	}
	if !approx(HireCostQuote(disc, cand, b), 900) {
		t.Fatalf("discounted hire=%v want 900", HireCostQuote(disc, cand, b))
	}
	s := model.GameState{Market: model.TalentMarket{RerollCount: 1}}
	want := balance.RerollCost(1, b)
	if RerollCostQuote(s, b) != want {
		t.Fatalf("reroll quote=%v want %v", RerollCostQuote(s, b), want)
	}
}
