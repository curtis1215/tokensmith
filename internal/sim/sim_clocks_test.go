package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestTickWithClocksOnlineSameDelta(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Research.EfficiencyMult = 1
	s.Progression.MaxUnlockedGen = 5
	// Engaged so EffectiveIndustryDT == economyDT (full industry when under cap).
	s.HasTraining = true
	s.Training.WorkRemaining = 1e12
	s.Competitors = []model.Competitor{{
		Name: "Rival", Skill: q(1, 1, 1, 1), Quality: q(8, 8, 8, 8),
	}}
	s.Models = []model.Model{onlineModel(20, b.RefPrice)}
	const dt = 3600.0
	a := Tick(s, dt, nil, b)
	c := tickWithClocks(s, dt, dt, nil, b)
	if !approx(a.GameTime, c.GameTime) || !approx(a.Progression.IndustryTime, c.Progression.IndustryTime) {
		t.Fatalf("Tick vs equal clocks diverged: game %v/%v industry %v/%v",
			a.GameTime, c.GameTime, a.Progression.IndustryTime, c.Progression.IndustryTime)
	}
	if !approx(a.Resources.RnD, c.Resources.RnD) {
		t.Fatalf("RnD diverged: %v vs %v", a.Resources.RnD, c.Resources.RnD)
	}
	if !approx(a.Competitors[0].Quality[model.DimCapability], c.Competitors[0].Quality[model.DimCapability]) {
		t.Fatalf("rival diverged")
	}
}

func TestTickIdleIndustryThrottle(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Progression.MaxUnlockedGen = 5
	s.Progression.IndustryTime = 0
	// Idle: no frontier, no training.
	const dt = 1000.0
	ns := Tick(s, dt, nil, b)
	want := dt * balance.IndustryIdleMult
	if !approx(ns.Progression.IndustryTime, want) {
		t.Fatalf("idle IndustryTime = %v, want %v", ns.Progression.IndustryTime, want)
	}
	if !approx(ns.GameTime, dt) {
		t.Fatalf("GameTime = %v, want %v", ns.GameTime, dt)
	}
}

func TestTickIndustryStopsAtPlayerCap(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Progression.MaxUnlockedGen = 5
	cap := IndustryTimeCapSec(s, b)
	s.Progression.IndustryTime = cap - 10
	s.HasTraining = true
	s.Training.WorkRemaining = 1e12
	ns := Tick(s, 1e6, nil, b)
	if ns.Progression.IndustryTime > cap+1e-6 {
		t.Fatalf("IndustryTime = %v > cap %v", ns.Progression.IndustryTime, cap)
	}
	if !approx(ns.Progression.IndustryTime, cap) {
		t.Fatalf("IndustryTime = %v, want cap %v", ns.Progression.IndustryTime, cap)
	}
}

func TestTickWithClocksDefensiveIndustryClamp(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Progression.MaxUnlockedGen = 5
	cap := IndustryTimeCapSec(s, b)
	ns := tickWithClocks(s, 0, cap*2, nil, b)
	if ns.Progression.IndustryTime > cap+1e-6 {
		t.Fatalf("IndustryTime = %v, want ≤ %v", ns.Progression.IndustryTime, cap)
	}
}

func TestTickWithClocksSplitsEconomyAndIndustry(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Research.EfficiencyMult = 1
	s.Progression.IndustryTime = 100
	s.Competitors = []model.Competitor{{
		Name: "Rival", Skill: q(1, 1, 1, 1), Quality: q(8, 8, 8, 8),
	}}
	s.Models = []model.Model{onlineModel(50, b.RefPrice)}
	// Economy advances 1000s of R&D/cash time; industry only 10s for rivals/clock.
	ns := tickWithClocks(s, 1000, 10, nil, b)
	if !approx(ns.GameTime, 1000) {
		t.Fatalf("GameTime = %v, want 1000", ns.GameTime)
	}
	if !approx(ns.Progression.IndustryTime, 110) {
		t.Fatalf("IndustryTime = %v, want 110", ns.Progression.IndustryTime)
	}
	// R&D uses economyDT (employee rate * 1000), not industry. Empty roster → 0.
	wantRnD := staffRnDPerSecFromEmployees(s, b) * 1000
	if !approx(ns.Resources.RnD, wantRnD) {
		t.Fatalf("RnD = %v, want %v (economy clock)", ns.Resources.RnD, wantRnD)
	}
}

func TestTickWithClocksRivalUsesIndustryDT(t *testing.T) {
	b := balance.Default()
	b.CompetitorCatchupRate = 1 // full catch-up per second of industry dt
	b.TrainBoostRivalPicks = 0  // isolate skill×frontier target (no investment boost)
	c := model.Competitor{Name: "Rival", Skill: q(1, 1, 1, 1)}
	c.Quality[model.DimCapability] = 90 // already inside band around GF=100
	pm := onlineModel(100, b.RefPrice)
	s := model.GameState{Models: []model.Model{pm}, Competitors: []model.Competitor{c}}
	s.Progression.MaxUnlockedGen = 1
	s.Progression.Rivals = model.RivalEraState{Era: 1, Leaders: []string{"Nobody"}}
	// industryDT=0: factor 0 → no approach (stays 90 after band clamp).
	ns := tickWithClocks(s, 1e6, 0, nil, b)
	if !approx(ns.Competitors[0].Quality[model.DimCapability], 90) {
		t.Fatalf("industryDT=0 should not approach: %v", ns.Competitors[0].Quality[model.DimCapability])
	}
	// industryDT=1 with rate 1 → snap to target 100.
	ns2 := tickWithClocks(s, 0, 1, nil, b)
	got2 := ns2.Competitors[0].Quality[model.DimCapability]
	if !approx(got2, 100) {
		t.Fatalf("industryDT should catch up rivals: %v, want ~100", got2)
	}
	// GameTime only moves on economyDT.
	if ns2.GameTime != 0 {
		t.Fatalf("GameTime moved on industry-only tick: %v", ns2.GameTime)
	}
	if !approx(ns2.Progression.IndustryTime, 1) {
		t.Fatalf("IndustryTime = %v, want 1", ns2.Progression.IndustryTime)
	}
}

func TestSecondsUntilNextTimeGeneration(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	// At day 0 (Gen1 baseline), next is Gen2 day 1000.
	sec := SecondsUntilNextTimeGeneration(s, b)
	want := 1000 * 86400.0
	if !approx(sec, want) {
		t.Fatalf("from day0: %v, want %v", sec, want)
	}
	// Mid Gen1–Gen2.
	s.Progression.IndustryTime = 500 * 86400
	sec = SecondsUntilNextTimeGeneration(s, b)
	if !approx(sec, 500*86400) {
		t.Fatalf("from day500: %v, want %v", sec, 500*86400)
	}
	// Exactly on Gen2 baseline → next Gen3 day 2500.
	s.Progression.IndustryTime = 1000 * 86400
	sec = SecondsUntilNextTimeGeneration(s, b)
	if !approx(sec, 1500*86400) {
		t.Fatalf("from day1000: %v, want %v", sec, 1500*86400)
	}
}
