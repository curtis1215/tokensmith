package balance

import (
	"math"
	"testing"

	"tokensmith/internal/model"
)

func TestDefaultTrainBoostCatalog(t *testing.T) {
	b := Default()
	if len(b.TrainBoosts) != model.NumQualityDims {
		t.Fatalf("len TrainBoosts = %d, want %d", len(b.TrainBoosts), model.NumQualityDims)
	}
	seen := map[model.QualityDim]bool{}
	ids := map[string]bool{}
	for _, tb := range b.TrainBoosts {
		if tb.NameZH == "" || tb.ID == "" || tb.RoleWeight <= 0 || math.IsNaN(tb.RoleWeight) {
			t.Fatalf("bad entry: %+v", tb)
		}
		if seen[tb.Dim] {
			t.Fatalf("duplicate dim %v", tb.Dim)
		}
		seen[tb.Dim] = true
		if ids[tb.ID] {
			t.Fatalf("duplicate id %s", tb.ID)
		}
		ids[tb.ID] = true
	}
	wantNames := map[model.QualityDim]string{
		model.DimCapability: "優質語料",
		model.DimEfficiency: "省算力改造",
		model.DimSafety:     "安全評測",
		model.DimSpeed:      "加速優化",
	}
	for _, tb := range b.TrainBoosts {
		if tb.NameZH != wantNames[tb.Dim] {
			t.Errorf("dim %v name = %q, want %q", tb.Dim, tb.NameZH, wantNames[tb.Dim])
		}
	}
	if b.TrainBoostBeta != 0.15 || b.TrainBoostPainMult != 1.0 {
		t.Errorf("beta/pain = %v/%v", b.TrainBoostBeta, b.TrainBoostPainMult)
	}
	if b.TrainBoostFloorMonthly != b.StartingCash/12 {
		t.Errorf("floor = %v, want %v", b.TrainBoostFloorMonthly, b.StartingCash/12)
	}
	wantSlot := [model.NumQualityDims]float64{1, 1, 1.8, 2.5}
	if b.TrainBoostSlotMult != wantSlot {
		t.Errorf("slot = %v, want %v", b.TrainBoostSlotMult, wantSlot)
	}
	if b.TrainBoostRivalPicks != 2 {
		t.Errorf("rival picks = %d", b.TrainBoostRivalPicks)
	}
}

func TestTrainBoostCashCostFloorGen1LinearTwoThenSlot(t *testing.T) {
	b := Default()
	ref := b.TrainBoostFloorMonthly // annual = StartingCash
	var none [model.NumQualityDims]bool
	cost0, err := TrainBoostCashCost(1, ref, none, b)
	if err != nil || cost0 != 0 {
		t.Fatalf("none: %v %v", cost0, err)
	}
	// single efficiency (weight 1.0): share 1/4.2 of 100_000
	var one [model.NumQualityDims]bool
	one[model.DimEfficiency] = true
	c1, err := TrainBoostCashCost(1, ref, one, b)
	if err != nil {
		t.Fatal(err)
	}
	want1 := 100_000 * 1.0 / 4.2
	if math.Abs(c1-want1) > 1e-6 {
		t.Fatalf("one = %v, want %v", c1, want1)
	}
	// all four: bases * slot mult by ascending dim order 0,1,2,3
	var all [model.NumQualityDims]bool
	for d := range all {
		all[d] = true
	}
	cAll, err := TrainBoostCashCost(1, ref, all, b)
	if err != nil {
		t.Fatal(err)
	}
	weights := []float64{1.2, 1.0, 1.1, 0.9}
	slots := []float64{1, 1, 1.8, 2.5}
	var wantAll float64
	for i := range weights {
		wantAll += 100_000 * weights[i] / 4.2 * slots[i]
	}
	if math.Abs(cAll-wantAll) > 1e-4 {
		t.Fatalf("all = %v, want %v", cAll, wantAll)
	}
	if cAll <= 100_000 {
		t.Fatalf("full pack with slots should exceed linear full %v", cAll)
	}
}

func TestTrainBoostCashBonusAdditiveNoSoftCap(t *testing.T) {
	b := Default()
	spec, _ := Generation(1)
	var all [model.NumQualityDims]bool
	for d := range all {
		all[d] = true
	}
	bonus, err := TrainBoostCashBonus(1, all, b)
	if err != nil {
		t.Fatal(err)
	}
	for d := range bonus {
		want := b.TrainBoostBeta * spec.QualityScale
		if math.Abs(bonus[d]-want) > 1e-9 {
			t.Fatalf("dim %d bonus = %v, want %v (no soft cap)", d, bonus[d], want)
		}
	}
}

func TestTrainBoostCashCostToggleOrderIndependent(t *testing.T) {
	b := Default()
	ref := 10_000.0
	var a, rev [model.NumQualityDims]bool
	a[0], a[3] = true, true
	rev[3], rev[0] = true, true
	c1, _ := TrainBoostCashCost(2, ref, a, b)
	c2, _ := TrainBoostCashCost(2, ref, rev, b)
	if c1 != c2 {
		t.Fatalf("order dependent: %v vs %v", c1, c2)
	}
}

func TestTrainBoostAffordanceGen1Floor(t *testing.T) {
	b := Default()
	ref := b.TrainBoostFloorMonthly
	var one [model.NumQualityDims]bool
	one[model.DimEfficiency] = true
	c, _ := TrainBoostCashCost(1, ref, one, b)
	if c > 0.30*b.StartingCash {
		t.Fatalf("single item %v > 30%% starting cash", c)
	}
	var all [model.NumQualityDims]bool
	for d := range all {
		all[d] = true
	}
	// linear target before slots is 1× annual floor; with slots strictly greater
	linear := float64(1) * 12 * ref * b.TrainBoostPainMult
	full, _ := TrainBoostCashCost(1, ref, all, b)
	if full < linear {
		t.Fatalf("full %v < linear annual %v", full, linear)
	}
}
