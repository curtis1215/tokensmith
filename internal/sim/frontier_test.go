package sim

import (
	"math"
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestPlayerFrontierBestOnline(t *testing.T) {
	s := model.GameState{
		Models: []model.Model{
			{Online: false, Quality: q(100, 100, 100, 100)}, // draft ignored
			{Online: true, Quality: q(10, 5, 8, 3)},
			{Online: true, Quality: q(7, 20, 4, 9)},
		},
	}
	f := PlayerFrontier(s)
	if !approx(f[model.DimCapability], 10) || !approx(f[model.DimEfficiency], 20) ||
		!approx(f[model.DimSafety], 8) || !approx(f[model.DimSpeed], 9) {
		t.Fatalf("player frontier = %v", f)
	}
	// Purity: models unchanged.
	if s.Models[1].Quality[model.DimCapability] != 10 {
		t.Fatal("PlayerFrontier mutated models")
	}
}

func TestTimeFrontierInterpolationAndBaselineScale(t *testing.T) {
	b := balance.Default()
	g1, _ := balance.Generation(1)
	g2, _ := balance.Generation(2)
	// Day 0 → Gen1 scale. Unlock high enough that day 500 is under the player-lead cap.
	s := model.GameState{}
	s.Progression.MaxUnlockedGen = 5
	s.Progression.IndustryTime = 0
	tf0 := TimeFrontier(s, b)
	want0 := b.CompetitorBaseQuality * g1.QualityScale / g1.QualityScale
	for d := range model.NumQualityDims {
		if !approx(tf0[d], want0) {
			t.Fatalf("day0 dim %d = %v, want %v", d, tf0[d], want0)
		}
	}
	// Midpoint Gen1→Gen2 baselines (day 500).
	s.Progression.IndustryTime = 500 * 86400
	tf := TimeFrontier(s, b)
	midScale := g1.QualityScale + 0.5*(g2.QualityScale-g1.QualityScale) // 35
	wantMid := b.CompetitorBaseQuality * midScale / g1.QualityScale
	for d := range model.NumQualityDims {
		if !approx(tf[d], wantMid) {
			t.Fatalf("day500 dim %d = %v, want %v", d, tf[d], wantMid)
		}
	}
	// All dimensions equal (no player allocation assumed).
	if tf[0] != tf[1] || tf[1] != tf[2] || tf[2] != tf[3] {
		t.Fatalf("time frontier dims not equal: %v", tf)
	}
}

func TestTimeFrontierCappedByPlayerLead(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Progression.MaxUnlockedGen = 5
	// Raw industry far past Gen6 baseline (would be late-game without cap).
	s.Progression.IndustryTime = 40500 * 86400
	g6, err := balance.Generation(6)
	if err != nil {
		t.Fatal(err)
	}
	g1, _ := balance.Generation(1)
	want := b.CompetitorBaseQuality * g6.QualityScale / g1.QualityScale
	tf := TimeFrontier(s, b)
	for d := range model.NumQualityDims {
		if !approx(tf[d], want) {
			t.Fatalf("capped TF dim %d = %v, want Gen6-scale %v", d, tf[d], want)
		}
	}
	// Uncapped would be much higher (Gen10 baseline day 40000 is still below 40500).
	g10, _ := balance.Generation(10)
	uncappedScale := g10.QualityScale // at/after day 40000
	uncapped := b.CompetitorBaseQuality * uncappedScale / g1.QualityScale
	if tf[0] >= uncapped*0.9 {
		t.Fatalf("TF %v looks uncapped (near Gen10 %v)", tf[0], uncapped)
	}
}

func TestGlobalFrontierPerDimensionMax(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Models: []model.Model{{Online: true, Quality: q(30, 1, 1, 1)}},
	}
	// Industry at day 0 → time frontier = 8 on every dim.
	s.Progression.IndustryTime = 0
	gf := GlobalFrontier(s, b)
	// Capability: player 30 > time 8.
	if !approx(gf[model.DimCapability], 30) {
		t.Fatalf("cap global = %v, want 30", gf[model.DimCapability])
	}
	// Efficiency: time 8 > player 1.
	if !approx(gf[model.DimEfficiency], b.CompetitorBaseQuality) {
		t.Fatalf("eff global = %v, want time base %v", gf[model.DimEfficiency], b.CompetitorBaseQuality)
	}
}

func TestTickAdvancesIndustryTime(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Progression.MaxUnlockedGen = 5
	s.Progression.IndustryTime = 100
	// Engaged path: full industry DT (under player-lead cap).
	s.HasTraining = true
	s.Training.WorkRemaining = 1e12
	ns := Tick(s, 50, nil, b)
	if !approx(ns.Progression.IndustryTime, 150) {
		t.Fatalf("IndustryTime = %v, want 150", ns.Progression.IndustryTime)
	}
	if s.Progression.IndustryTime != 100 {
		t.Fatal("Tick mutated input IndustryTime")
	}
	// GameTime still advances independently.
	if !approx(ns.GameTime, 50) {
		t.Fatalf("GameTime = %v, want 50", ns.GameTime)
	}
}

func TestModelFrontierViewStableQualityAndGap(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Models: []model.Model{{
			Gen: 1, Online: true,
			Quality: q(10, 5, 5, 5),
		}},
	}
	s.Progression.MaxUnlockedGen = 5
	s.Progression.IndustryTime = 0
	// Engaged so industry advances at full rate under the player-lead cap.
	s.HasTraining = true
	s.Training.WorkRemaining = 1e18
	before := s.Models[0].Quality
	v := ModelFrontierView(s, 0, b)
	if v.AbsoluteQuality != before {
		t.Fatalf("absolute quality wrong: %v", v.AbsoluteQuality)
	}
	// Advancing industry clock must not rewrite stored quality.
	s2 := Tick(s, 86400*2000, nil, b) // +2000 game days on industry clock
	if s2.Models[0].Quality != before {
		t.Fatalf("stored quality mutated by Tick: %v → %v", before, s2.Models[0].Quality)
	}
	v2 := ModelFrontierView(s2, 0, b)
	// Relative view should move as time frontier rises, while absolute stays.
	if v2.AbsoluteQuality != before {
		t.Fatalf("view absolute changed: %v", v2.AbsoluteQuality)
	}
	if v2.GlobalFrontier[model.DimCapability] <= v.GlobalFrontier[model.DimCapability] {
		t.Fatalf("global frontier should rise with industry time: %v → %v",
			v.GlobalFrontier[model.DimCapability], v2.GlobalFrontier[model.DimCapability])
	}
	// Delta pct = abs/global - 1
	g := v.GlobalFrontier[model.DimCapability]
	wantDelta := before[model.DimCapability]/g - 1
	if !approx(v.FrontierDeltaPct[model.DimCapability], wantDelta) {
		t.Fatalf("delta = %v, want %v", v.FrontierDeltaPct[model.DimCapability], wantDelta)
	}
	// Generation gap is explanatory (equiv frontier gen - model gen).
	if v.ModelGen != 1 {
		t.Fatalf("ModelGen = %d", v.ModelGen)
	}
	if math.IsNaN(v.GenerationGap) || math.IsInf(v.GenerationGap, 0) {
		t.Fatalf("GenerationGap not finite: %v", v.GenerationGap)
	}
}

func TestModelFrontierViewZeroFrontierSafety(t *testing.T) {
	b := balance.Default()
	// Force zero global frontier: no online models, and zero base quality.
	b.CompetitorBaseQuality = 0
	s := model.GameState{
		Models: []model.Model{{Gen: 2, Online: true, Quality: q(10, 10, 10, 10)}},
	}
	// Wait - player frontier would still be 10. Use offline-only models.
	s.Models[0].Online = false
	v := ModelFrontierView(s, 0, b)
	for d := range model.NumQualityDims {
		if v.GlobalFrontier[d] != 0 {
			t.Fatalf("expected zero global frontier, got %v", v.GlobalFrontier)
		}
		if v.FrontierDeltaPct[d] != 0 {
			t.Fatalf("zero frontier delta must be 0, got %v", v.FrontierDeltaPct[d])
		}
	}
	if math.IsNaN(v.GenerationGap) || math.IsInf(v.GenerationGap, 0) {
		t.Fatalf("gap unsafe: %v", v.GenerationGap)
	}
}

func TestModelFrontierViewInvalidIndex(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Models: []model.Model{{Gen: 1, Online: true}}}
	v := ModelFrontierView(s, -1, b)
	if v.Active {
		t.Fatal("invalid index should not be active")
	}
	v2 := ModelFrontierView(s, 3, b)
	if v2.Active {
		t.Fatal("oob index should not be active")
	}
}

func q(a, b, c, d float64) [model.NumQualityDims]float64 {
	return [model.NumQualityDims]float64{a, b, c, d}
}
