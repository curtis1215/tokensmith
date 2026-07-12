package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// Stars catalog removed; starEffects is a neutral stub until Task 6–7.

func TestStarEffectsNeutralStub(t *testing.T) {
	se := starEffects(model.GameState{}, balance.Default())
	if !approx(se.InfraMult, 1) || !approx(se.UserGrowthMult, 1) || !approx(se.RnDPerSec, 0) {
		t.Fatalf("neutral star effects expected: %+v", se)
	}
	for d := range model.NumQualityDims {
		if !approx(se.QualityMult[d], 1) {
			t.Fatalf("QualityMult[%d]=%v want 1", d, se.QualityMult[d])
		}
	}
}
