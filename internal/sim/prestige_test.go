package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestPrestigeEffectsAggregate(t *testing.T) {
	b := balance.Default()
	pe := prestigeEffects([]string{"start-cash-1", "rnd-mult-1"}, b)
	if !approx(pe.StartCash, 100000) {
		t.Errorf("StartCash = %v, want 100000", pe.StartCash)
	}
	if !approx(pe.RnDMult, 1.1) {
		t.Errorf("RnDMult = %v, want 1.1", pe.RnDMult)
	}
	if !approx(pe.CashMult, 1) {
		t.Errorf("unrelated mult should be 1: %v", pe.CashMult)
	}
}

func TestPatentsFor(t *testing.T) {
	b := balance.Default() // PatentK 1e8
	if got := patentsFor(1e9, b); got != 3 { // floor(sqrt(10))
		t.Errorf("patentsFor(1e9) = %v, want 3", got)
	}
	if got := patentsFor(1e10, b); got != 10 { // floor(sqrt(100))
		t.Errorf("patentsFor(1e10) = %v, want 10", got)
	}
}

func TestFreshRun(t *testing.T) {
	b := balance.Default()
	p := model.Prestige{Patents: 5, UnlockedPrestige: []string{"start-cash-1"}} // +100k cash
	ns := freshRun(p, b)
	if ns.Prestige.Patents != 5 {
		t.Errorf("patents not preserved: %v", ns.Prestige.Patents)
	}
	if len(ns.Competitors) != 7 {
		t.Errorf("competitors not re-seeded")
	}
	if !approx(ns.Resources.Cash, b.StartingCash+100000) {
		t.Errorf("cash = %v, want %v", ns.Resources.Cash, b.StartingCash+100000)
	}
	if ns.Research.EfficiencyMult != 1 {
		t.Errorf("efficiency mult not reset to 1")
	}
}
