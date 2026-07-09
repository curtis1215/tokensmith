package sim

import (
	"math"
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

func TestSegmentShareBarsSumsToOne(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Models: []model.Model{
			{Online: true, Segment: model.SegConsumer, Quality: [model.NumQualityDims]float64{10, 0, 0, 0}, Name: "MyModel"},
		},
		Competitors: []model.Competitor{
			{Name: "Rival1", Quality: [model.NumQualityDims]float64{5, 0, 0, 0}},
			{Name: "Rival2", Quality: [model.NumQualityDims]float64{8, 0, 0, 0}},
		},
	}
	bars := SegmentShareBars(s, b, model.SegConsumer)
	if len(bars) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(bars))
	}
	sum := 0.0
	for _, bar := range bars {
		sum += bar.Share
	}
	if math.Abs(sum-1.0) > 1e-6 {
		t.Errorf("sum of shares = %v, want 1.0", sum)
	}
	// Verify sorting desc
	if bars[0].Share < bars[1].Share || bars[1].Share < bars[2].Share {
		t.Errorf("SegmentShareBars is not sorted descending: %v", bars)
	}
}

func TestServableUsers(t *testing.T) {
	b := balance.Default()
	b.InferenceLoadPerUser = 0.0001
	s := model.GameState{}
	s.Compute.RentedInference = map[string]int{"N7": 10} // 10 N7 inference
	// EffectiveInference should be 10.
	got := ServableUsers(s, b)
	want := 10.0 / 0.0001
	if got != want {
		t.Errorf("ServableUsers = %v, want %v", got, want)
	}
}

func TestThreatLevelOrdering(t *testing.T) {
	b := balance.Default()
	// player best appeal in seg = appeal of 10
	s := model.GameState{
		Models: []model.Model{
			{Online: true, Segment: model.SegConsumer, Quality: [model.NumQualityDims]float64{10, 0, 0, 0}},
		},
	}

	// ThreatLevel: 0 low, 1 mid, 2 high — rival appeal vs player's best in seg.
	// Threat: if rival > player*1.1 → high (2); >= player*0.9 → mid (1); else low (0)
	
	// rival appeal = appeal of 8 (below 0.9)
	rivalLow := model.Competitor{Name: "low", Quality: [model.NumQualityDims]float64{8, 0, 0, 0}}
	if got := ThreatLevel(s, b, model.SegConsumer, rivalLow); got != 0 {
		t.Errorf("ThreatLevel for low rival = %d, want 0", got)
	}

	// rival appeal = appeal of 9.5 (between 0.9 and 1.1)
	rivalMid := model.Competitor{Name: "mid", Quality: [model.NumQualityDims]float64{9.5, 0, 0, 0}}
	if got := ThreatLevel(s, b, model.SegConsumer, rivalMid); got != 1 {
		t.Errorf("ThreatLevel for mid rival = %d, want 1", got)
	}

	// rival appeal = appeal of 12 (above 1.1)
	rivalHigh := model.Competitor{Name: "high", Quality: [model.NumQualityDims]float64{12, 0, 0, 0}}
	if got := ThreatLevel(s, b, model.SegConsumer, rivalHigh); got != 2 {
		t.Errorf("ThreatLevel for high rival = %d, want 2", got)
	}
}
