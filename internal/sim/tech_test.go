package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestTechEffectsAggregates(t *testing.T) {
	b := balance.Default()
	s := model.GameState{UnlockedTech: []string{"algo-cap-1", "infra-eff-1"}}
	te := techEffects(s, b)
	if !approx(te.QualityMult[model.DimCapability], 1.15) {
		t.Errorf("cap mult = %v, want 1.15", te.QualityMult[model.DimCapability])
	}
	if !approx(te.InfraMult, 1.1) {
		t.Errorf("infra mult = %v, want 1.1", te.InfraMult)
	}
	if !approx(te.TrainRnDMult, 1) {
		t.Errorf("unrelated mult should be 1, got %v", te.TrainRnDMult)
	}
}

func TestTechEffectsNeutralWhenNoneUnlocked(t *testing.T) {
	te := techEffects(model.GameState{}, balance.Default())
	if !approx(te.QualityMult[model.DimSpeed], 1) || !approx(te.UserGrowthMult, 1) {
		t.Fatalf("neutral tech effects expected: %+v", te)
	}
}

func TestIsUnlocked(t *testing.T) {
	s := model.GameState{UnlockedTech: []string{"a", "b"}}
	if !isUnlocked(s, "b") || isUnlocked(s, "c") {
		t.Fatalf("isUnlocked wrong")
	}
}
