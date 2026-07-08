package tui

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestSettleGrantsOfflineRnDAndAdvances(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Research.EfficiencyMult = 1
	before := s.Resources.RnD
	ns, sum := Settle(s, b, 2*3600, 100000, 50000) // 2h offline + tokens
	if sum.RnDGained <= 0 || ns.Resources.RnD <= before {
		t.Fatalf("offline settlement granted no R&D: %+v", sum)
	}
	if ns.GameTime < 2*3600-1 {
		t.Fatalf("world did not advance: GameTime=%v", ns.GameTime)
	}
	if sum.TokensIn != 100000 || sum.TokensOut != 50000 {
		t.Fatalf("summary tokens wrong: %+v", sum)
	}
}

func TestSettleCompletesTraining(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Compute.TrainingCapacity = 1
	s.HasTraining = true
	s.Training = model.TrainingJob{Gen: 1, Alloc: [4]float64{0.4, 0.2, 0.2, 0.2}, Price: 12, WorkRemaining: 1800}
	ns, sum := Settle(s, b, 4*3600, 0, 0) // 4h × 1 GPU = 14400 GPU·s ≫ 1800
	if !sum.TrainingCompleted || ns.HasTraining {
		t.Fatalf("training should complete offline: %+v", sum)
	}
}

func TestSettleClampsElapsed(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	_, sum := Settle(s, b, -100, 0, 0)
	if sum.SecondsSettled != 0 {
		t.Fatalf("negative elapsed should clamp to 0, got %v", sum.SecondsSettled)
	}
	_, sum2 := Settle(s, b, 999*86400, 0, 0)
	if sum2.SecondsSettled != settleMaxSec {
		t.Fatalf("huge elapsed should clamp to max, got %v", sum2.SecondsSettled)
	}
}
