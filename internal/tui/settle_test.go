package tui

import (
	"strings"
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
	s.Compute.RentedTraining = map[string]int{"N7": 1}
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

func TestSettleCountsEvents(t *testing.T) {
	b := balance.Default()
	b.EventHitChance = 1.0
	b.EventCheckSec = 3600 // one roll per settle chunk
	var s model.GameState
	s.Resources.Cash = 1e6
	s.Events.RandState = 42
	s.Events.NextCheckAt = 1 // pre-scheduled so rolls happen immediately
	ns, sum := Settle(s, b, 6*3600, 0, 0)
	if sum.EventsFired == 0 {
		t.Fatalf("expected events during 6h settle, got %+v", sum)
	}
	if ns.Events.FiredCount != sum.EventsFired {
		t.Fatalf("summary %d != state counter %d", sum.EventsFired, ns.Events.FiredCount)
	}
}

func TestNewAtSeedsRandState(t *testing.T) {
	m := testModel(t)
	if m.state.Events.RandState == 0 {
		t.Fatal("a fresh game must get a nonzero RNG seed")
	}
}

func TestOfflineBannerAutoResolvedOnly(t *testing.T) {
	out := offlineBanner(Summary{EventsAutoResolved: 2, SecondsSettled: 3600})
	if !strings.Contains(out, "自動決議") {
		t.Fatalf("expected auto-resolve line when EventsFired==0, got %q", out)
	}
}
