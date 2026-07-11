package balance

import (
	"math"
	"testing"
)

func TestEraForGenBoundaries(t *testing.T) {
	cases := []struct {
		gen, era int
	}{
		{1, 1}, {2, 1},
		{3, 2}, {4, 2},
		{5, 3}, {6, 3}, {7, 3},
		{8, 4}, {9, 4}, {10, 4},
		{11, 5}, {12, 5}, {13, 5},
		{14, 6}, {15, 6}, {16, 6},
		{17, 7},
	}
	for _, tc := range cases {
		got, err := EraForGen(tc.gen)
		if err != nil {
			t.Fatalf("EraForGen(%d): %v", tc.gen, err)
		}
		if got != tc.era {
			t.Errorf("EraForGen(%d) = %d, want %d", tc.gen, got, tc.era)
		}
	}
	if _, err := EraForGen(0); err != ErrInvalidGenerationSpec {
		t.Fatalf("EraForGen(0) err = %v, want ErrInvalidGenerationSpec", err)
	}
	if _, err := EraForGen(-3); err != ErrInvalidGenerationSpec {
		t.Fatalf("EraForGen(-3) err = %v, want ErrInvalidGenerationSpec", err)
	}
}

func TestEraStartEndGen(t *testing.T) {
	cases := []struct {
		era, start, end int
	}{
		{1, 1, 2},
		{2, 3, 4},
		{3, 5, 7},
		{4, 8, 10},
		{5, 11, 13},
		{6, 14, 16},
		{7, 17, 19},
	}
	for _, tc := range cases {
		start, err := EraStartGen(tc.era)
		if err != nil {
			t.Fatalf("EraStartGen(%d): %v", tc.era, err)
		}
		end, err := EraEndGen(tc.era)
		if err != nil {
			t.Fatalf("EraEndGen(%d): %v", tc.era, err)
		}
		if start != tc.start || end != tc.end {
			t.Errorf("era %d: start/end = %d/%d, want %d/%d", tc.era, start, end, tc.start, tc.end)
		}
		// Round-trip: every gen in the era maps back.
		for g := start; g <= end; g++ {
			era, err := EraForGen(g)
			if err != nil || era != tc.era {
				t.Errorf("gen %d → era %d err=%v, want era %d", g, era, err, tc.era)
			}
		}
	}
	if _, err := EraStartGen(0); err != ErrInvalidGenerationSpec {
		t.Fatalf("EraStartGen(0) err = %v, want ErrInvalidGenerationSpec", err)
	}
	if _, err := EraEndGen(-1); err != ErrInvalidGenerationSpec {
		t.Fatalf("EraEndGen(-1) err = %v, want ErrInvalidGenerationSpec", err)
	}
}

func TestGenerationSpecGen1To5Compatibility(t *testing.T) {
	// Exact historical Gen1–5 training ladder (formerly Config fixed arrays).
	want := []struct {
		gen, era                     int
		trainRnD, trainWork, quality float64
		baseline                     float64
	}{
		{1, 1, 20000, 900000, 25, 0},
		{2, 1, 150000, 3600000, 45, 1000},
		{3, 2, 1000000, 14400000, 65, 2500},
		{4, 2, 6000000, 57600000, 82, 4500},
		{5, 3, 40000000, 230400000, 100, 7000},
	}
	for _, tc := range want {
		g, err := Generation(tc.gen)
		if err != nil {
			t.Fatalf("Generation(%d): %v", tc.gen, err)
		}
		if g.Gen != tc.gen || g.Era != tc.era {
			t.Errorf("gen %d: Gen/Era = %d/%d, want %d/%d", tc.gen, g.Gen, g.Era, tc.gen, tc.era)
		}
		if g.TrainRnD != tc.trainRnD {
			t.Errorf("gen %d: TrainRnD = %v, want %v", tc.gen, g.TrainRnD, tc.trainRnD)
		}
		if g.TrainWork != tc.trainWork {
			t.Errorf("gen %d: TrainWork = %v, want %v", tc.gen, g.TrainWork, tc.trainWork)
		}
		if g.QualityScale != tc.quality {
			t.Errorf("gen %d: QualityScale = %v, want %v", tc.gen, g.QualityScale, tc.quality)
		}
		if g.TimeBaselineDay != tc.baseline {
			t.Errorf("gen %d: TimeBaselineDay = %v, want %v", tc.gen, g.TimeBaselineDay, tc.baseline)
		}
	}
}

func TestGenerationSpecGen6To10Table(t *testing.T) {
	// work = recommended × targetRealSeconds × RealSecCompression
	type row struct {
		gen             int
		frontierRnD     float64
		frontierSec     float64
		trainRnD        float64
		trainSec        float64
		quality         float64
		recommended     float64
		timeBaselineDay float64
	}
	rows := []row{
		{6, 180e6, 2 * 3600, 120e6, 0.5 * 3600, 120, 500, 10000},
		{7, 500e6, 4 * 3600, 350e6, 1 * 3600, 142, 1200, 14000},
		{8, 1.5e9, 6 * 3600, 1e9, 1.5 * 3600, 166, 3000, 20000},
		{9, 4.5e9, 8 * 3600, 3e9, 2 * 3600, 192, 7500, 30000},
		{10, 13.5e9, 12 * 3600, 9e9, 3 * 3600, 220, 18000, 40000},
	}
	for _, tc := range rows {
		g, err := Generation(tc.gen)
		if err != nil {
			t.Fatalf("Generation(%d): %v", tc.gen, err)
		}
		wantFW := tc.recommended * tc.frontierSec * RealSecCompression
		wantTW := tc.recommended * tc.trainSec * RealSecCompression
		if g.FrontierRnD != tc.frontierRnD {
			t.Errorf("gen %d FrontierRnD = %v, want %v", tc.gen, g.FrontierRnD, tc.frontierRnD)
		}
		if g.FrontierWork != wantFW {
			t.Errorf("gen %d FrontierWork = %v, want %v", tc.gen, g.FrontierWork, wantFW)
		}
		if g.TrainRnD != tc.trainRnD {
			t.Errorf("gen %d TrainRnD = %v, want %v", tc.gen, g.TrainRnD, tc.trainRnD)
		}
		if g.TrainWork != wantTW {
			t.Errorf("gen %d TrainWork = %v, want %v", tc.gen, g.TrainWork, wantTW)
		}
		if g.QualityScale != tc.quality {
			t.Errorf("gen %d QualityScale = %v, want %v", tc.gen, g.QualityScale, tc.quality)
		}
		if g.RecommendedCompute != tc.recommended {
			t.Errorf("gen %d RecommendedCompute = %v, want %v", tc.gen, g.RecommendedCompute, tc.recommended)
		}
		if g.TimeBaselineDay != tc.timeBaselineDay {
			t.Errorf("gen %d TimeBaselineDay = %v, want %v", tc.gen, g.TimeBaselineDay, tc.timeBaselineDay)
		}
		era, _ := EraForGen(tc.gen)
		if g.Era != era || g.Gen != tc.gen {
			t.Errorf("gen %d identity fields Gen/Era = %d/%d, want %d/%d", tc.gen, g.Gen, g.Era, tc.gen, era)
		}
	}
}

func TestGenerationSpecGen11Formulas(t *testing.T) {
	g10, err := Generation(10)
	if err != nil {
		t.Fatal(err)
	}
	// gen 11 → k=1
	g11, err := Generation(11)
	if err != nil {
		t.Fatal(err)
	}
	if !approx(g11.FrontierRnD, g10.FrontierRnD*2.4) {
		t.Errorf("Gen11 FrontierRnD = %v, want %v", g11.FrontierRnD, g10.FrontierRnD*2.4)
	}
	if !approx(g11.FrontierWork, g10.FrontierWork*2.6) {
		t.Errorf("Gen11 FrontierWork = %v, want %v", g11.FrontierWork, g10.FrontierWork*2.6)
	}
	if !approx(g11.TrainRnD, g10.TrainRnD*2.2) {
		t.Errorf("Gen11 TrainRnD = %v, want %v", g11.TrainRnD, g10.TrainRnD*2.2)
	}
	if !approx(g11.TrainWork, g10.TrainWork*2.8) {
		t.Errorf("Gen11 TrainWork = %v, want %v", g11.TrainWork, g10.TrainWork*2.8)
	}
	if g11.QualityScale != 220+28 {
		t.Errorf("Gen11 QualityScale = %v, want 248", g11.QualityScale)
	}
	if !approx(g11.RecommendedCompute, 18000*1.75) {
		t.Errorf("Gen11 RecommendedCompute = %v, want %v", g11.RecommendedCompute, 18000*1.75)
	}
	// Time baseline: interval(10)=10000; interval(11)=13500; baseline=53500
	if !approx(g11.TimeBaselineDay, 53500) {
		t.Errorf("Gen11 TimeBaselineDay = %v, want 53500", g11.TimeBaselineDay)
	}
	// gen 12 → k=2
	g12, err := Generation(12)
	if err != nil {
		t.Fatal(err)
	}
	if !approx(g12.FrontierRnD, g10.FrontierRnD*math.Pow(2.4, 2)) {
		t.Errorf("Gen12 FrontierRnD = %v", g12.FrontierRnD)
	}
	if g12.QualityScale != 220+28*2 {
		t.Errorf("Gen12 QualityScale = %v, want 276", g12.QualityScale)
	}
	if !approx(g12.TimeBaselineDay, 53500+13500*1.35) {
		t.Errorf("Gen12 TimeBaselineDay = %v, want %v", g12.TimeBaselineDay, 53500+13500*1.35)
	}
}

func TestGenerationSpecGen11To100FiniteMonotonic(t *testing.T) {
	prev, err := Generation(10)
	if err != nil {
		t.Fatal(err)
	}
	for gen := 11; gen <= 100; gen++ {
		g, err := Generation(gen)
		if err != nil {
			t.Fatalf("Generation(%d): %v", gen, err)
		}
		fields := []struct {
			name string
			v, p float64
		}{
			{"FrontierRnD", g.FrontierRnD, prev.FrontierRnD},
			{"FrontierWork", g.FrontierWork, prev.FrontierWork},
			{"TrainRnD", g.TrainRnD, prev.TrainRnD},
			{"TrainWork", g.TrainWork, prev.TrainWork},
			{"QualityScale", g.QualityScale, prev.QualityScale},
			{"RecommendedCompute", g.RecommendedCompute, prev.RecommendedCompute},
			{"TimeBaselineDay", g.TimeBaselineDay, prev.TimeBaselineDay},
		}
		for _, f := range fields {
			if f.v <= 0 || math.IsNaN(f.v) || math.IsInf(f.v, 0) {
				t.Fatalf("gen %d %s not positive finite: %v", gen, f.name, f.v)
			}
			if f.v <= f.p {
				t.Fatalf("gen %d %s not monotonic: %v <= prev %v", gen, f.name, f.v, f.p)
			}
		}
		prev = g
	}
}

func TestGenerationSpecInvalidGen(t *testing.T) {
	for _, gen := range []int{0, -1, -99} {
		if _, err := Generation(gen); err != ErrInvalidGenerationSpec {
			t.Fatalf("Generation(%d) err = %v, want ErrInvalidGenerationSpec", gen, err)
		}
	}
}

func approx(a, b float64) bool {
	const eps = 1e-9
	if a == b {
		return true
	}
	d := math.Abs(a - b)
	return d <= eps*math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
}
