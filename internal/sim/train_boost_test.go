package sim

import (
	"math"
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestBoostRefMonthlyCashIgnoresStickerPrice(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Models: []model.Model{{
			Online: true, Segment: model.SegEnterprise,
			Users: 1000, Price: 180,
			Quality: [4]float64{10, 10, 10, 10},
		}},
	}
	base := BoostRefMonthlyCash(s, b)
	s.Models[0].Price = 1
	got := BoostRefMonthlyCash(s, b)
	if base != got {
		t.Fatalf("price exploit: %v → %v", base, got)
	}
	// Must use ref price path, not 1000*1
	ref := EffectiveRefPrice(s, model.SegEnterprise, b)
	want := 1000 * ref * b.RevenueMult // no campaign/prestige
	if math.Abs(base-want) > 1e-6 {
		t.Fatalf("anchor = %v, want %v", base, want)
	}
}

func TestBoostRefMonthlyCashIncludesRevenueMult(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Models: []model.Model{{
			Online: true, Segment: model.SegConsumer,
			Users: 500, Price: 12,
		}},
	}
	a := BoostRefMonthlyCash(s, b)
	b.RevenueMult = b.RevenueMult * 2
	got := BoostRefMonthlyCash(s, b)
	if math.Abs(got-2*a) > 1e-6 {
		t.Fatalf("RevenueMult scale: %v vs 2*%v", got, a)
	}
}

func TestTrainBoostRefMonthlyUsesFloor(t *testing.T) {
	b := balance.Default()
	s := model.GameState{} // no models
	if TrainBoostRefMonthly(s, b) != b.TrainBoostFloorMonthly {
		t.Fatalf("want floor")
	}
}

func TestPredictedTrainQualityMonotonicInBoosts(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	alloc := [model.NumQualityDims]float64{0.4, 0.2, 0.2, 0.2}
	var none, one, all [model.NumQualityDims]bool
	one[model.DimSafety] = true
	for d := range all {
		all[d] = true
	}
	q0, _ := PredictedTrainQuality(s, 1, alloc, none, b)
	q1, _ := PredictedTrainQuality(s, 1, alloc, one, b)
	qA, _ := PredictedTrainQuality(s, 1, alloc, all, b)
	for _, seg := range []model.Segment{model.SegConsumer, model.SegEnterprise, model.SegDeveloper} {
		w := b.SegmentWeights[seg]
		a0, a1, aA := appealOf(q0, w), appealOf(q1, w), appealOf(qA, w)
		if a1 < a0-1e-9 || aA < a1-1e-9 {
			t.Fatalf("seg %v appeal non-monotonic: %v %v %v", seg, a0, a1, aA)
		}
	}
	if qA[model.DimSafety] < q1[model.DimSafety]-1e-9 {
		t.Fatalf("full pack reduced safety bonus")
	}
}
