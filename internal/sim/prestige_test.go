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
	b := balance.Default()                   // PatentK 1e8
	if got := patentsFor(1e9, b); got != 3 { // floor(sqrt(10))
		t.Errorf("patentsFor(1e9) = %v, want 3", got)
	}
	if got := patentsFor(1e10, b); got != 10 { // floor(sqrt(100))
		t.Errorf("patentsFor(1e10) = %v, want 10", got)
	}
}

func TestRestartUngatedBanksPatentsAndResets(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Models = []model.Model{{Online: true, Users: 100}}
	s.Resources.Cash = -50000 // deep in debt, well below any prestige gate
	s.PeakValuation = 1e10    // banks floor(sqrt(1e10/1e8)) = 10 patents
	s.Prestige.Patents = 3
	ns := Restart(s, b)
	if len(ns.Models) != 0 {
		t.Fatalf("restart should clear models, got %d", len(ns.Models))
	}
	if ns.Resources.Cash != b.StartingCash {
		t.Fatalf("restart should reset cash to start, got %v", ns.Resources.Cash)
	}
	if ns.Prestige.Patents != 13 {
		t.Fatalf("restart should bank patents from peak: got %v want 13", ns.Prestige.Patents)
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
	// The starting baseline (researchers, base R&D) must be reseeded so a reset
	// run is actually playable. Compute starts empty (nil maps), same as a
	// brand-new run — the player rents on demand.
	if ns.Research.Researchers[model.Tier1] != b.StartingResearchersT1 {
		t.Errorf("researchers not reseeded: %v", ns.Research.Researchers[model.Tier1])
	}
	if len(ns.Compute.RentedTraining) != 0 || len(ns.Compute.RentedInference) != 0 {
		t.Errorf("compute should start empty, got train=%v inf=%v", ns.Compute.RentedTraining, ns.Compute.RentedInference)
	}
	if !approx(ns.Resources.RnD, b.StartingRnD) { // start-cash-1 adds no R&D
		t.Errorf("R&D not reseeded: %v, want %v", ns.Resources.RnD, b.StartingRnD)
	}
}
