package tui

import (
	"math"
	"testing"

	"tokensmith/internal/model"
)

func TestLerpApproaches(t *testing.T) {
	x := 0.0
	for i := 0; i < 30; i++ {
		x = lerp(x, 100, 0.3)
	}
	if x < 99 {
		t.Fatalf("x=%v want ~100", x)
	}
}

func TestDisplaySnap(t *testing.T) {
	var d displayState
	d.snap(displayState{Cash: 50})
	if d.Cash != 50 {
		t.Fatal(d.Cash)
	}
}

func TestDisplayApproachTowardTruth(t *testing.T) {
	var d displayState
	truth := displayState{Cash: 1000, RnD: 500, Valuation: 2e6, TotalUsers: 100, TrainUtil: 1, InfUtil: 0.5}
	for i := 0; i < 40; i++ {
		d.approach(truth, 0.3)
	}
	if math.Abs(d.Cash-truth.Cash) > 0.02 {
		t.Fatalf("Cash=%v want %v", d.Cash, truth.Cash)
	}
	if math.Abs(d.InfUtil-truth.InfUtil) > 1e-3 {
		t.Fatalf("InfUtil=%v want %v", d.InfUtil, truth.InfUtil)
	}
}

func TestDisplaySnapAfterRestartReady(t *testing.T) {
	m := testModel(t)
	m.disp.Cash = 99999
	m.dispReady = true
	m.snapDisplay()
	if m.disp.Cash != m.state.Resources.Cash {
		t.Fatalf("snap Cash=%v want %v", m.disp.Cash, m.state.Resources.Cash)
	}
	if !m.dispReady {
		t.Fatal("dispReady should be true after snap")
	}
}

func TestAdvanceDisplayFirstTickSnaps(t *testing.T) {
	m := testModel(t)
	m.state.Resources.Cash = 1234
	m.dispReady = false
	m.advanceDisplay()
	if m.disp.Cash != 1234 {
		t.Fatalf("first advance should snap: got %v", m.disp.Cash)
	}
	if !m.dispReady {
		t.Fatal("dispReady should become true")
	}
}

func TestPulseTokenOnTokens(t *testing.T) {
	m := testModel(t)
	m.dispReady = true
	m.disp.snap(truthDisplay(m))
	m.lastTokens = 10
	m.advanceDisplay()
	if m.disp.PulseToken != 4 {
		t.Fatalf("PulseToken=%d want 4", m.disp.PulseToken)
	}
	m.lastTokens = 0
	m.advanceDisplay()
	if m.disp.PulseToken != 3 {
		t.Fatalf("PulseToken should decay to 3, got %d", m.disp.PulseToken)
	}
}

func TestDisplayApproachesUsersAndUtils(t *testing.T) {
	m := testModel(t)
	m.state.Models = []model.Model{{Online: true, Users: 1000, Price: 12, Segment: 0}}
	m.state.HasTraining = true
	m.state.Compute.InferenceLoad = 50
	m.state.Compute.RentedInference = map[string]int{"N7": 10}
	truth := truthDisplay(m)
	if truth.TotalUsers < 1000 {
		t.Fatalf("truth TotalUsers=%v", truth.TotalUsers)
	}
	var d displayState
	for i := 0; i < 40; i++ {
		d.approach(truth, 0.3)
	}
	if d.TotalUsers < 990 {
		t.Fatalf("TotalUsers approached=%v want ~%v", d.TotalUsers, truth.TotalUsers)
	}
	if d.TrainUtil < 0.99 {
		t.Fatalf("TrainUtil=%v want ~1", d.TrainUtil)
	}
}
