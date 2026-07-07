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
