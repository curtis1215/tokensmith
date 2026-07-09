package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestStarEffectsAggregate(t *testing.T) {
	b := balance.Default()
	s := model.GameState{HiredStars: []string{"aria-chen", "marcus-cole"}}
	se := starEffects(s, b)
	if !approx(se.QualityMult[model.DimCapability], 1.22) {
		t.Errorf("cap mult = %v, want 1.22", se.QualityMult[model.DimCapability])
	}
	if !approx(se.RnDPerSec, 300/balance.RealSecCompression) {
		t.Errorf("RnDPerSec = %v, want %v", se.RnDPerSec, 300/balance.RealSecCompression)
	}
	if !approx(se.UserGrowthMult, 1.30) {
		t.Errorf("UserGrowthMult = %v, want 1.30", se.UserGrowthMult)
	}
	if !approx(se.InfraMult, 1) {
		t.Errorf("InfraMult should be neutral 1, got %v", se.InfraMult)
	}
}

func TestStarEffectsNeutralWhenNoneHired(t *testing.T) {
	se := starEffects(model.GameState{}, balance.Default())
	if !approx(se.InfraMult, 1) || !approx(se.RnDPerSec, 0) {
		t.Fatalf("neutral star effects expected: %+v", se)
	}
}
