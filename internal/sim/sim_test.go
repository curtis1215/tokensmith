package sim

import (
	"math"
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestStaffRnDPerSec(t *testing.T) {
	b := balance.Default()
	r := model.Research{EfficiencyMult: 1.0}
	r.Researchers[model.Tier1] = 2 // 2*5 = 10
	r.Researchers[model.Tier2] = 1 // 1*15 = 15
	got := staffRnDPerSec(r, b)     // 25/s
	if !approx(got, 25) {
		t.Fatalf("staffRnDPerSec = %v, want 25", got)
	}
}

func TestStaffRnDEfficiencyMult(t *testing.T) {
	b := balance.Default()
	r := model.Research{EfficiencyMult: 2.0}
	r.Researchers[model.Tier2] = 1 // 15 * 2.0 = 30
	if got := staffRnDPerSec(r, b); !approx(got, 30) {
		t.Fatalf("staffRnDPerSec with mult = %v, want 30", got)
	}
}

func TestTickAddsStaffRnDAndAdvancesTime(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Research: model.Research{EfficiencyMult: 1.0}}
	s.Research.Researchers[model.Tier2] = 4 // 60/s
	ns := Tick(s, 10, nil, b)               // 60/s * 10s = 600
	if !approx(ns.Resources.RnD, 600) {
		t.Fatalf("RnD = %v, want 600", ns.Resources.RnD)
	}
	if !approx(ns.GameTime, 10) {
		t.Fatalf("GameTime = %v, want 10", ns.GameTime)
	}
	// Tick must not mutate the input state.
	if s.Resources.RnD != 0 || s.GameTime != 0 {
		t.Fatalf("Tick mutated input: %+v", s)
	}
}

func TestTokenRawRnD(t *testing.T) {
	b := balance.Default()
	events := []model.TokenEvent{
		{InputTokens: 1000, OutputTokens: 500},  // (1000 + 2*500)/10 = 200
		{InputTokens: 0, OutputTokens: 1000},    // (0 + 2000)/10   = 200
	}
	if got := tokenRawRnD(events, b); !approx(got, 400) {
		t.Fatalf("tokenRawRnD = %v, want 400", got)
	}
}

func TestTokenRawRnDEmpty(t *testing.T) {
	if got := tokenRawRnD(nil, balance.Default()); got != 0 {
		t.Fatalf("tokenRawRnD(nil) = %v, want 0", got)
	}
}

func TestTickAddsTokenRnD(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Research: model.Research{EfficiencyMult: 1.0}}
	// no staff → only token R&D. 1000 output → (2000)/10 = 200.
	events := []model.TokenEvent{{OutputTokens: 1000}}
	ns := Tick(s, 1, events, b)
	if !approx(ns.Resources.RnD, 200) {
		t.Fatalf("RnD = %v, want 200", ns.Resources.RnD)
	}
}

func TestApplySoftCapBelowFull(t *testing.T) {
	eff, nw := applySoftCap(0, 1000, 200000, 0.3)
	if !approx(eff, 1000) || !approx(nw, 1000) {
		t.Fatalf("below full: eff=%v nw=%v, want 1000/1000", eff, nw)
	}
}

func TestApplySoftCapCrossingFull(t *testing.T) {
	// window at 199,000; raw 2,000 → 1,000 full + 1,000*0.3 = 1,300 effective
	eff, nw := applySoftCap(199000, 2000, 200000, 0.3)
	if !approx(eff, 1300) {
		t.Fatalf("crossing: eff=%v, want 1300", eff)
	}
	if !approx(nw, 201000) {
		t.Fatalf("crossing: nw=%v, want 201000", nw)
	}
}

func TestApplySoftCapAboveFull(t *testing.T) {
	// already above full → everything diminished
	eff, nw := applySoftCap(200000, 1000, 200000, 0.3)
	if !approx(eff, 300) || !approx(nw, 201000) {
		t.Fatalf("above: eff=%v nw=%v, want 300/201000", eff, nw)
	}
}

func TestTickSoftCapAccumulatesWindow(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Research: model.Research{EfficiencyMult: 1.0}}
	// event that yields raw 199000 R&D: output = 199000*10/2 = 995000
	ev1 := []model.TokenEvent{{OutputTokens: 995000}}
	s = Tick(s, 1, ev1, b)
	if !approx(s.WindowRnD, 199000) {
		t.Fatalf("WindowRnD after ev1 = %v, want 199000", s.WindowRnD)
	}
	// next raw 2000 (output 10000) → 1300 effective (1000 full + 300)
	before := s.Resources.RnD
	s = Tick(s, 1, []model.TokenEvent{{OutputTokens: 10000}}, b)
	if !approx(s.Resources.RnD-before, 1300) {
		t.Fatalf("effective token R&D = %v, want 1300", s.Resources.RnD-before)
	}
}

func TestTickWindowResets(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Research: model.Research{EfficiencyMult: 1.0}, WindowRnD: 199000, WindowElapsed: 86399}
	// advancing past the 86400s window boundary resets WindowRnD to 0,
	// so the next tokens are granted at full rate again.
	before := s.Resources.RnD
	s = Tick(s, 2, []model.TokenEvent{{OutputTokens: 10000}}, b) // raw 2000
	if !approx(s.WindowRnD, 2000) {
		t.Fatalf("WindowRnD after reset = %v, want 2000", s.WindowRnD)
	}
	if !approx(s.Resources.RnD-before, 2000) {
		t.Fatalf("token R&D after reset = %v, want 2000 (full rate)", s.Resources.RnD-before)
	}
}

func TestOfflineFastForwardEquivalenceStaffOnly(t *testing.T) {
	b := balance.Default()
	base := model.GameState{Research: model.Research{EfficiencyMult: 1.5}}
	base.Research.Researchers[model.Tier1] = 3
	base.Research.Researchers[model.Tier3] = 2

	// One big tick of 100s, no token events.
	oneShot := Tick(base, 100, nil, b)

	// 100 small ticks of 1s each.
	stepwise := base
	for range 100 {
		stepwise = Tick(stepwise, 1, nil, b)
	}

	if !approx(oneShot.Resources.RnD, stepwise.Resources.RnD) {
		t.Fatalf("fast-forward mismatch: oneShot=%v stepwise=%v",
			oneShot.Resources.RnD, stepwise.Resources.RnD)
	}
	if !approx(oneShot.GameTime, stepwise.GameTime) {
		t.Fatalf("GameTime mismatch: oneShot=%v stepwise=%v",
			oneShot.GameTime, stepwise.GameTime)
	}
	if !approx(oneShot.Resources.RnD, 14250) { // (3*5 + 2*40)*1.5 = 142.5/s * 100s = 14250
		t.Fatalf("expected RnD 14250, got %v", oneShot.Resources.RnD)
	}
}
