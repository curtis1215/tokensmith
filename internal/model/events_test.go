package model

import "testing"

func TestNeutralEventEffectsAllOne(t *testing.T) {
	e := NeutralEventEffects()
	for _, v := range []float64{
		e.BuildCostMult, e.PowerCostMult, e.RefPriceMult, e.UserGrowthMult,
		e.TechCostMult, e.TAMMult, e.ValuationMult, e.SafetyWeightMult,
		e.IncidentChanceMult,
	} {
		if v != 1.0 {
			t.Fatalf("neutral effect = %v, want 1.0", v)
		}
	}
}

func TestResolveEventIsCommand(t *testing.T) {
	var _ Command = ResolveEvent{PendingIndex: 0, Choice: 1}
}

func TestGameStateHasEventsZeroValue(t *testing.T) {
	var s GameState
	if s.Events.RandState != 0 || len(s.Events.Pending) != 0 || len(s.Events.Active) != 0 {
		t.Fatalf("zero-value EventsState should be empty, got %+v", s.Events)
	}
}
