package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestNextRandDeterministic(t *testing.T) {
	s1, r1 := nextRand(42)
	s2, r2 := nextRand(42)
	if s1 != s2 || r1 != r2 {
		t.Fatalf("same input state must give same output: (%d,%v) vs (%d,%v)", s1, r1, s2, r2)
	}
	if s1 == 42 {
		t.Fatal("state must advance")
	}
}

func TestNextRandRangeAndSpread(t *testing.T) {
	state := uint64(7)
	var lo, hi int
	for i := 0; i < 1000; i++ {
		var r float64
		state, r = nextRand(state)
		if r < 0 || r >= 1 {
			t.Fatalf("r = %v out of [0,1)", r)
		}
		if r < 0.5 {
			lo++
		} else {
			hi++
		}
	}
	if lo < 400 || hi < 400 {
		t.Fatalf("distribution too skewed: lo=%d hi=%d", lo, hi)
	}
}

func TestNextRandZeroStateWorks(t *testing.T) {
	state, r := nextRand(0)
	if state == 0 {
		t.Fatal("state must advance from 0")
	}
	if r < 0 || r >= 1 {
		t.Fatalf("r = %v out of [0,1)", r)
	}
}

func TestEventEffectsNeutralWhenEmpty(t *testing.T) {
	var s model.GameState
	b := balance.Default()
	e := eventEffects(s, b)
	if e != model.NeutralEventEffects() {
		t.Fatalf("empty Active must aggregate to neutral, got %+v", e)
	}
}

func TestEventEffectsMultiplies(t *testing.T) {
	var s model.GameState
	b := balance.Default()
	m1 := model.NeutralEventEffects()
	m1.PowerCostMult = 1.3
	m1.UserGrowthMult = 1.25
	m2 := model.NeutralEventEffects()
	m2.PowerCostMult = 0.7
	s.Events.Active = []model.ActiveModifier{
		{EventID: "a", ExpiresAt: 999, Target: -1, Effects: m1},
		{EventID: "b", ExpiresAt: 999, Target: -1, Effects: m2},
	}
	e := eventEffects(s, b)
	if diff := e.PowerCostMult - 1.3*0.7; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("PowerCostMult = %v, want %v", e.PowerCostMult, 1.3*0.7)
	}
	if e.UserGrowthMult != 1.25 {
		t.Fatalf("UserGrowthMult = %v, want 1.25", e.UserGrowthMult)
	}
	if e.TechCostMult != 1.0 {
		t.Fatalf("aggregate TechCostMult must stay neutral (branch-targeted), got %v", e.TechCostMult)
	}
}

func TestEventTechCostMultBranchTargeted(t *testing.T) {
	var s model.GameState
	m := model.NeutralEventEffects()
	m.TechCostMult = 0.5
	s.Events.Active = []model.ActiveModifier{
		{EventID: "paper", ExpiresAt: 999, Target: int(model.BranchAlgo), Effects: m},
	}
	if got := eventTechCostMult(s, model.BranchAlgo); got != 0.5 {
		t.Fatalf("targeted branch mult = %v, want 0.5", got)
	}
	if got := eventTechCostMult(s, model.BranchInfra); got != 1.0 {
		t.Fatalf("untargeted branch mult = %v, want 1.0", got)
	}
}
