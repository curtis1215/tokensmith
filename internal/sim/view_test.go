package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestEffectiveCapacityExported(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Compute.TrainingCapacity = 4
	s.Compute.InferenceCapacity = 2
	if EffectiveTraining(s, b) != 4 {
		t.Errorf("EffectiveTraining = %v, want 4", EffectiveTraining(s, b))
	}
	if EffectiveInference(s, b) != 2 {
		t.Errorf("EffectiveInference = %v, want 2", EffectiveInference(s, b))
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
