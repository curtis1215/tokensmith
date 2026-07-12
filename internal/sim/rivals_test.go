package sim

import (
	"math"
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestRivalLeagueBandInvariant(t *testing.T) {
	b := balance.Default()
	// Player frontier high; rivals start mixed (below floor + near target).
	pm := onlineModel(100, b.RefPrice)
	comps := balance.DefaultCompetitors()
	for i := range comps {
		// Leave some far below the band so hard floor must snap them.
		comps[i].Quality = q(10, 10, 10, 10)
	}
	if len(comps) > 0 {
		comps[0].Quality = q(90, 90, 90, 90)
	}
	s := model.GameState{Models: []model.Model{pm}, Competitors: comps}
	s.Progression.IndustryTime = 0
	// Full catch-up then hard band clamp — every dim in [GF×0.85, GF×1.15].
	b.CompetitorCatchupRate = 1
	ns := advanceRivalLeague(s, 1, b)
	assertRivalsInsideBand(t, ns, b, "after full catch-up")
}

func TestClampRivalToBandHardFloorAndCeiling(t *testing.T) {
	// Unit: floor and ceiling are hard, not soft approach only.
	if got := clampRivalToBand(50, 100); !approx(got, 85) {
		t.Fatalf("floor clamp = %v, want 85", got)
	}
	if got := clampRivalToBand(200, 100); !approx(got, 115) {
		t.Fatalf("ceil clamp = %v, want 115", got)
	}
	if got := clampRivalToBand(100, 100); !approx(got, 100) {
		t.Fatalf("in-band = %v, want 100", got)
	}
	if got := clampRivalToBand(-3, 100); !approx(got, 85) {
		t.Fatalf("neg then floor = %v, want 85", got)
	}
	if got := clampRivalToBand(50, 0); got != 50 {
		t.Fatalf("zero frontier leaves q: got %v", got)
	}
}

// assertRivalsInsideBand fails if any rival dimension is outside GF×[0.85,1.15]
// (or negative when GF is non-positive).
func assertRivalsInsideBand(t *testing.T, s model.GameState, b balance.Config, label string) {
	t.Helper()
	gf := GlobalFrontier(s, b)
	if len(s.Competitors) == 0 {
		t.Fatalf("%s: empty rival roster", label)
	}
	for _, c := range s.Competitors {
		for d := range model.NumQualityDims {
			got := c.Quality[d]
			if gf[d] <= 0 {
				if got < 0 {
					t.Fatalf("%s: %s dim %d = %v negative (gf=%v)", label, c.Name, d, got, gf[d])
				}
				continue
			}
			lo := gf[d] * rivalFloorPct
			hi := gf[d] * rivalCeilPct
			if got < lo-1e-6 || got > hi+1e-6 {
				t.Fatalf("%s: %s dim %d = %v outside [%v, %v] (gf=%v)",
					label, c.Name, d, got, lo, hi, gf[d])
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
	// Campaign Tick runs the league (no freeze); quality should move toward target.
	ns := Tick(s, 3600, nil, b)
	got := ns.Competitors[0].Quality[model.DimCapability]
	if got <= 10 {
		t.Fatalf("campaign Tick must advance rival league, still %v", got)
	}
	gf := GlobalFrontier(ns, b)[model.DimCapability]
	if got > gf*rivalCeilPct+1e-6 {
		t.Fatalf("campaign rival %v above ceiling around %v", got, gf)
	}
}

func TestRivalLeagueApproachesTarget(t *testing.T) {
	b := balance.Default()
	b.CompetitorCatchupRate = 1 // complete catch-up in 1s for the test
	b.TrainBoostRivalPicks = 0  // isolate skill×frontier catch-up (no investment boost)
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
	gf := GlobalFrontier(s, b)
	lt := rivalTarget(s, leader, gf, b)
	ot := rivalTarget(s, other, gf, b)
	// Leader target higher by leaderBonusPct on capability (both skill 1.0).
	// Train boost also applies to top Skill dims (all equal → lowest indices);
	// leader vs other comparison is independent of that additive boost.
	if lt[model.DimCapability] <= ot[model.DimCapability] {
		t.Fatalf("leader target %v should exceed other %v", lt[model.DimCapability], ot[model.DimCapability])
	}
	// Base leader: 100*(1.0+leaderBonus); plus β*gf on top-K skill dims (cap is index 0).
	wantLead := 100*(1.0+leaderBonusPct) + b.TrainBoostBeta*100
	if !approx(lt[model.DimCapability], wantLead) {
		t.Fatalf("leader target = %v, want %v", lt[model.DimCapability], wantLead)
	}
}

func TestRivalTargetIncludesTrainBoostOnTopSkillDims(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	// Ensure era leaders init not required for target math
	gf := [model.NumQualityDims]float64{100, 100, 100, 100}
	rival := model.Competitor{
		Name:  "OpenAI",
		Skill: [model.NumQualityDims]float64{1.08, 1.00, 0.96, 1.04}, // top: cap, speed
	}
	got := rivalTarget(s, rival, gf, b)
	// base for cap: 100 * clamp(1.08)=108; plus 0.15*100=15 → 123
	if got[model.DimCapability] < 108+14.9 {
		t.Fatalf("cap target missing boost: %v", got[model.DimCapability])
	}
	// efficiency not in top-2 → no +15
	baseEff := 100 * 1.00
	if got[model.DimEfficiency] > baseEff+0.01 {
		t.Fatalf("eff should not get boost: %v", got[model.DimEfficiency])
	}
	// speed is top-2: base 104 + 15
	if got[model.DimSpeed] < 104+14.9 {
		t.Fatalf("speed target missing boost: %v", got[model.DimSpeed])
	}
}

func TestUnlockGenTechDoesNotSpikeRivalQualityOneTick(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Competitors: balance.DefaultCompetitors(),
		// published gen1 model so GF stable from player side
		Models: []model.Model{{
			Online: true, Gen: 1,
			Quality: [4]float64{10, 10, 10, 10},
			Users:   1000, Price: 12, Segment: model.SegConsumer,
		}},
	}
	s = ensureRivalEraState(s, b)
	// Warm one tick
	s = advanceRivalLeague(s, 3600, b)
	before := append([]model.Competitor(nil), s.Competitors...)
	// Unlock gen2 tech only (no Apply cash/RnD path needed for cliff gate)
	s.UnlockedTech = append(append([]string(nil), s.UnlockedTech...), balance.GenUnlockNodeID(2))
	s.Progression.MaxUnlockedGen = 2
	after := advanceRivalLeague(s, 3600, b)
	// No discontinuous jump larger than one catch-up step toward the new target.
	factor := b.CompetitorCatchupRate * 3600
	if factor > 1 {
		factor = 1
	}
	gf := GlobalFrontier(s, b)
	for i, c := range after.Competitors {
		prev := before[i].Quality
		tgt := rivalTarget(s, c, gf, b)
		for d := range model.NumQualityDims {
			maxStep := math.Abs(tgt[d]-prev[d]) * factor
			delta := math.Abs(c.Quality[d] - prev[d])
			if delta > maxStep+1e-6 {
				t.Fatalf("%s dim %d jumped %v > max step %v", c.Name, d, delta, maxStep)
			}
		}
	}
}
