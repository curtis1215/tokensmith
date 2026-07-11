package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestProgressionViewInactiveEmpty(t *testing.T) {
	b := balance.Default()
	v := FrontierProgressView(model.GameState{}, b)
	if v.Active || v.UnavailableReason != "" || v.TargetGen != 0 {
		t.Fatalf("inactive view wrong: %+v", v)
	}
}

func TestProgressionViewUsesSnapshotsForFractionsAndETA(t *testing.T) {
	b := balance.Default()
	// Totals snapshotted; remaining half. Live catalog must not rewrite fractions.
	s := model.GameState{}
	s.Servers = []model.Server{{Pool: model.PoolTraining, Compute: 200}}
	s.Resources.RnD = 1e12
	s.Progression.Frontier = model.FrontierProject{
		Active:             true,
		TargetGen:          6,
		RnDTotal:           1000, // not necessarily Generation(6).FrontierRnD
		RnDRemaining:       250,
		WorkTotal:          400,
		WorkRemaining:      100,
		RecommendedCompute: 100,
		AllocationPct:      50,
	}
	v := FrontierProgressView(s, b)
	if !v.Active || v.TargetGen != 6 {
		t.Fatalf("active/target: %+v", v)
	}
	era, _ := balance.EraForGen(6)
	if v.TargetEra != era {
		t.Fatalf("era = %d, want %d", v.TargetEra, era)
	}
	// Completed fractions from snapshots: R&D 75%, work 75%.
	if !approx(v.RnDFraction, 0.75) || !approx(v.WorkFraction, 0.75) {
		t.Fatalf("fractions rnd/work = %v/%v, want 0.75/0.75", v.RnDFraction, v.WorkFraction)
	}
	if v.AllocationPct != 50 || v.ModelAllocationPct != 50 {
		t.Fatalf("alloc split = %d/%d", v.AllocationPct, v.ModelAllocationPct)
	}
	// 50% of 200 compute = 100 allocated; at recommendation → diminished 100.
	if !approx(v.AllocatedCompute, 100) || !approx(v.DiminishedCompute, 100) {
		t.Fatalf("compute alloc/dim = %v/%v, want 100/100", v.AllocatedCompute, v.DiminishedCompute)
	}
	if !approx(v.RecommendedCompute, 100) {
		t.Fatalf("recommended = %v (snapshot), want 100", v.RecommendedCompute)
	}
	// ETA from snapshot remaining work / diminished.
	if !approx(v.ETASec, 1) { // 100/100
		t.Fatalf("ETA = %v, want 1", v.ETASec)
	}
	if v.UnavailableReason != "" {
		t.Fatalf("reason should be empty when progressing, got %q", v.UnavailableReason)
	}
}

func TestProgressionViewUnavailableReasons(t *testing.T) {
	b := balance.Default()
	base := model.GameState{}
	base.Progression.Frontier = model.FrontierProject{
		Active:             true,
		TargetGen:          6,
		RnDTotal:           100,
		RnDRemaining:       100,
		WorkTotal:          100,
		WorkRemaining:      100,
		RecommendedCompute: 50,
		AllocationPct:      100,
	}

	// Paused: allocation 0 (takes priority when compute exists).
	paused := base
	paused.Servers = []model.Server{{Pool: model.PoolTraining, Compute: 50}}
	paused.Resources.RnD = 1e9
	paused.Progression.Frontier.AllocationPct = 0
	if got := FrontierProgressView(paused, b).UnavailableReason; got != "paused" {
		t.Fatalf("paused reason = %q", got)
	}

	// No compute.
	noComp := base
	noComp.Resources.RnD = 1e9
	if got := FrontierProgressView(noComp, b).UnavailableReason; got != "no-compute" {
		t.Fatalf("no-compute reason = %q", got)
	}

	// No R&D (compute present, alloc > 0).
	noRnD := base
	noRnD.Servers = []model.Server{{Pool: model.PoolTraining, Compute: 50}}
	noRnD.Resources.RnD = 0
	v := FrontierProgressView(noRnD, b)
	if v.UnavailableReason != "no-rnd" {
		t.Fatalf("no-rnd reason = %q", v.UnavailableReason)
	}
	if v.ETASec != 0 {
		t.Fatalf("stalled ETA should be 0, got %v", v.ETASec)
	}
}

func TestProgressionViewDiminishedAboveRecommendation(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Servers = []model.Server{{Pool: model.PoolTraining, Compute: 400}}
	s.Resources.RnD = 1e9
	s.Progression.Frontier = model.FrontierProject{
		Active: true, TargetGen: 6,
		RnDTotal: 1, RnDRemaining: 1,
		WorkTotal: 1, WorkRemaining: 1,
		RecommendedCompute: 100, AllocationPct: 100,
	}
	v := FrontierProgressView(s, b)
	want := diminishedFrontierCompute(400, 100)
	if !approx(v.DiminishedCompute, want) {
		t.Fatalf("diminished = %v, want %v", v.DiminishedCompute, want)
	}
	if !approx(v.ETASec, 1/want) {
		t.Fatalf("ETA = %v, want %v", v.ETASec, 1/want)
	}
}
