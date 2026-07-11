package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestRivalLeagueBandInvariant(t *testing.T) {
	b := balance.Default()
	// Player frontier high on capability; rivals start far below band floor.
	pm := onlineModel(100, b.RefPrice)
	comps := balance.DefaultCompetitors()
	for i := range comps {
		comps[i].Quality = q(1, 1, 1, 1)
	}
	s := model.GameState{Models: []model.Model{pm}, Competitors: comps}
	s.Progression.IndustryTime = 0
	ns := advanceRivalLeague(s, 1, b) // one tiny step still re-clamps
	gf := GlobalFrontier(ns, b)
	for _, c := range ns.Competitors {
		for d := range model.NumQualityDims {
			if gf[d] <= 0 {
				continue
			}
			lo, hi := gf[d]*rivalFloorPct, gf[d]*rivalCeilPct
			got := c.Quality[d]
			if got < lo-1e-6 || got > hi+1e-6 {
				t.Fatalf("%s dim %d = %v outside [%v, %v] (gf=%v)", c.Name, d, got, lo, hi, gf[d])
			}
		}
	}
}

func TestRivalLeagueCampaignNoLongerFrozen(t *testing.T) {
	b := balance.Default()
	c := model.Competitor{Name: "Rival"}
	c.Quality[model.DimCapability] = 10
	c.Skill[model.DimCapability] = 1.0
	pm := onlineModel(80, b.RefPrice)
	s := model.GameState{
		Models:      []model.Model{pm},
		Competitors: []model.Competitor{c},
		Campaign:    model.CampaignState{Doctrine: model.DoctrineConsumer},
	}
	// With band clamp, quality should leave 10 immediately toward the frontier band.
	ns := Tick(s, 3600, nil, b)
	got := ns.Competitors[0].Quality[model.DimCapability]
	if got == 10 {
		t.Fatalf("campaign Tick must advance rival league, still %v", got)
	}
	gf := GlobalFrontier(ns, b)[model.DimCapability]
	if got < gf*rivalFloorPct-1e-6 || got > gf*rivalCeilPct+1e-6 {
		t.Fatalf("campaign rival %v outside band around %v", got, gf)
	}
}

func TestRivalLeagueApproachesTarget(t *testing.T) {
	b := balance.Default()
	b.CompetitorCatchupRate = 1 // complete catch-up in 1s for the test
	c := model.Competitor{Name: "Rival"}
	c.Skill[model.DimCapability] = 1.0
	c.Quality[model.DimCapability] = 50
	pm := onlineModel(100, b.RefPrice)
	s := model.GameState{Models: []model.Model{pm}, Competitors: []model.Competitor{c}}
	// Pin era leaders so "Rival" is not auto-selected as a leader (+4%).
	s.Progression.MaxUnlockedGen = 1
	s.Progression.Rivals = model.RivalEraState{Era: 1, Leaders: []string{"Nobody"}}
	ns := advanceRivalLeague(s, 1, b)
	got := ns.Competitors[0].Quality[model.DimCapability]
	// Target = 100 * 1.0 = 100; factor=1 → snap to target then band-clamp.
	if !approx(got, 100) {
		t.Fatalf("full catch-up = %v, want ~100", got)
	}
}

func TestRivalEraLeadersDeterministicWeighted(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Competitors: balance.DefaultCompetitors()}
	s.Progression.MaxUnlockedGen = 1 // era 1 (odd) → 3 leaders
	s.Events.RandState = 42
	ns := ensureRivalEraState(s, b)
	if ns.Progression.Rivals.Era != 1 {
		t.Fatalf("era = %d, want 1", ns.Progression.Rivals.Era)
	}
	n := 2 + 1%2 // 3
	if len(ns.Progression.Rivals.Leaders) != n {
		t.Fatalf("leaders = %v, want %d", ns.Progression.Rivals.Leaders, n)
	}
	// Distinct names from roster.
	seen := map[string]bool{}
	for _, name := range ns.Progression.Rivals.Leaders {
		if seen[name] {
			t.Fatalf("duplicate leader %q", name)
		}
		seen[name] = true
		found := false
		for _, c := range s.Competitors {
			if c.Name == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("leader %q not in roster", name)
		}
	}
	// Same seed → same leaders.
	ns2 := ensureRivalEraState(s, b)
	if len(ns2.Progression.Rivals.Leaders) != len(ns.Progression.Rivals.Leaders) {
		t.Fatal("nondeterministic leader count")
	}
	for i := range ns.Progression.Rivals.Leaders {
		if ns.Progression.Rivals.Leaders[i] != ns2.Progression.Rivals.Leaders[i] {
			t.Fatalf("nondeterministic leaders: %v vs %v", ns.Progression.Rivals.Leaders, ns2.Progression.Rivals.Leaders)
		}
	}
	// Even era → 2 leaders.
	s4 := s
	s4.Progression.MaxUnlockedGen = 8 // era 4
	s4.Events.RandState = 42
	ns4 := ensureRivalEraState(s4, b)
	if ns4.Progression.Rivals.Era != 4 || len(ns4.Progression.Rivals.Leaders) != 2 {
		t.Fatalf("era4 leaders = era %d %v, want 2 leaders", ns4.Progression.Rivals.Era, ns4.Progression.Rivals.Leaders)
	}
}

func TestRivalEraLeadersPersistAcrossUnrelatedRNG(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Competitors: balance.DefaultCompetitors()}
	s.Progression.MaxUnlockedGen = 5 // era 3
	s.Events.RandState = 7
	ns := ensureRivalEraState(s, b)
	leaders := append([]string(nil), ns.Progression.Rivals.Leaders...)
	// Unrelated RNG draws must not reshuffle leaders for the same era.
	for i := 0; i < 20; i++ {
		ns.Events.RandState, _ = nextRand(ns.Events.RandState)
	}
	ns2 := ensureRivalEraState(ns, b)
	if len(ns2.Progression.Rivals.Leaders) != len(leaders) {
		t.Fatalf("leader count changed after RNG noise")
	}
	for i := range leaders {
		if ns2.Progression.Rivals.Leaders[i] != leaders[i] {
			t.Fatalf("leaders reshuffled: %v → %v", leaders, ns2.Progression.Rivals.Leaders)
		}
	}
	// RandState may have advanced only during initial selection, not on re-ensure.
	if ns2.Progression.Rivals.Era != 3 {
		t.Fatalf("era = %d", ns2.Progression.Rivals.Era)
	}
}

func TestRivalEraTransitionClearsMomentum(t *testing.T) {
	b := balance.Default()
	comps := balance.DefaultCompetitors()
	for i := range comps {
		comps[i].MomentumPct[model.DimCapability] = 0.05
		comps[i].MomentumCycles = 3
	}
	s := model.GameState{Competitors: comps}
	s.Progression.MaxUnlockedGen = 4 // era 2
	s.Progression.Rivals = model.RivalEraState{Era: 2, Leaders: []string{"OpenAI", "Anthropic"}}
	s.Events.RandState = 1
	// Jump to era 3 (Gen5).
	s.Progression.MaxUnlockedGen = 5
	ns := ensureRivalEraState(s, b)
	if ns.Progression.Rivals.Era != 3 {
		t.Fatalf("era = %d, want 3", ns.Progression.Rivals.Era)
	}
	for _, c := range ns.Competitors {
		if c.MomentumCycles != 0 {
			t.Fatalf("%s MomentumCycles not cleared: %d", c.Name, c.MomentumCycles)
		}
		for d := range model.NumQualityDims {
			if c.MomentumPct[d] != 0 {
				t.Fatalf("%s MomentumPct[%d]=%v not cleared", c.Name, d, c.MomentumPct[d])
			}
		}
	}
	// New leaders selected for the new era (may or may not overlap).
	if len(ns.Progression.Rivals.Leaders) != 2+3%2 {
		t.Fatalf("era3 leader count = %d", len(ns.Progression.Rivals.Leaders))
	}
}

func TestRivalTargetLeaderBonus(t *testing.T) {
	b := balance.Default()
	pm := onlineModel(100, b.RefPrice)
	leader := model.Competitor{Name: "Lead", Skill: q(1.0, 1.0, 1.0, 1.0)}
	other := model.Competitor{Name: "Other", Skill: q(1.0, 1.0, 1.0, 1.0)}
	s := model.GameState{
		Models:      []model.Model{pm},
		Competitors: []model.Competitor{leader, other},
		Progression: model.ProgressionState{
			MaxUnlockedGen: 1,
			Rivals:         model.RivalEraState{Era: 1, Leaders: []string{"Lead"}},
		},
	}
	lt := rivalTarget(s, leader, b)
	ot := rivalTarget(s, other, b)
	// Leader target higher by leaderBonusPct on capability (both skill 1.0).
	if lt[model.DimCapability] <= ot[model.DimCapability] {
		t.Fatalf("leader target %v should exceed other %v", lt[model.DimCapability], ot[model.DimCapability])
	}
	wantLead := 100 * (1.0 + leaderBonusPct)
	if !approx(lt[model.DimCapability], wantLead) {
		t.Fatalf("leader target = %v, want %v", lt[model.DimCapability], wantLead)
	}
}
