package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestEffectiveCapacityExported(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Compute.RentedTraining = map[string]int{"N7": 4}
	s.Compute.RentedInference = map[string]int{"N7": 2}
	if EffectiveTraining(s, b) != 4 {
		t.Errorf("EffectiveTraining = %v, want 4", EffectiveTraining(s, b))
	}
	if EffectiveInference(s, b) != 2 {
		t.Errorf("EffectiveInference = %v, want 2", EffectiveInference(s, b))
	}
}

func TestRnDRatePerSec(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Research.EfficiencyMult = 1
	s.Research.Researchers[model.Tier1] = 2 // 2 × 0.005/s = 0.01/s
	if RnDRatePerSec(s, b) != 0.01 {
		t.Errorf("RnDRatePerSec = %v, want 0.01", RnDRatePerSec(s, b))
	}
}

func TestTotalUsersAndRevenue(t *testing.T) {
	s := model.GameState{Models: []model.Model{
		{Online: true, Users: 100, Price: 12},
		{Online: false, Users: 999, Price: 99}, // offline excluded
		{Online: true, Users: 50, Price: 6},
	}}
	if TotalUsers(s) != 150 {
		t.Errorf("TotalUsers = %v, want 150", TotalUsers(s))
	}
	if MonthlyRevenue(s) != 100*12+50*6 {
		t.Errorf("MonthlyRevenue = %v, want 1500", MonthlyRevenue(s))
	}
}

func TestMarketRankBeatsWeakField(t *testing.T) {
	b := balance.Default()
	strong := onlineModel(80, b.RefPrice) // high capability
	s := model.GameState{
		Models:      []model.Model{strong},
		Competitors: []model.Competitor{{Name: "weak"}},
	}
	rank, total := MarketRank(s, b, model.SegConsumer)
	if rank != 1 || total != 2 {
		t.Errorf("rank=%d total=%d, want 1/2", rank, total)
	}
}

func TestNextMilestone(t *testing.T) {
	b := balance.Default()
	s := model.GameState{PeakValuation: 5e5} // below first milestone 1e6
	target, prog, ok := NextMilestone(s, b)
	if !ok || target != 1e6 || prog != 0.5 {
		t.Errorf("got target=%v prog=%v ok=%v, want 1e6/0.5/true", target, prog, ok)
	}
}

func TestEstimateUserTargetPriceElasticity(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Models: []model.Model{{
			Online: false, Segment: model.SegConsumer,
			Quality: [model.NumQualityDims]float64{25, 0, 0, 0},
		}},
	}
	low := EstimateUserTarget(s, 0, 6, b)
	ref := EstimateUserTarget(s, 0, 12, b)
	high := EstimateUserTarget(s, 0, 24, b)
	if !(low > ref && ref > high && high > 0) {
		t.Fatalf("expected low>ref>high>0; got %v %v %v", low, ref, high)
	}
}

func TestIsDraft(t *testing.T) {
	draft := model.Model{Online: false, Users: 0}
	live := model.Model{Online: true, Users: 0}
	used := model.Model{Online: false, Users: 10}
	if !IsDraft(draft) {
		t.Error("draft should be draft")
	}
	if IsDraft(live) {
		t.Error("live should not be draft")
	}
	if IsDraft(used) {
		t.Error("used should not be draft")
	}
}
