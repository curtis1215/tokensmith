package sim

import (
	"math"
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestIndustryTimeCapSecGen5IsGen6Baseline(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Progression.MaxUnlockedGen = 5
	g6, err := balance.Generation(6)
	if err != nil {
		t.Fatal(err)
	}
	want := g6.TimeBaselineDay * 86400
	got := IndustryTimeCapSec(s, b)
	if !approx(got, want) {
		t.Fatalf("cap = %v, want %v (Gen6 baseline days=%v)", got, want, g6.TimeBaselineDay)
	}
}

func TestIndustryTimeResidualToCap(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Progression.MaxUnlockedGen = 5
	cap := IndustryTimeCapSec(s, b)
	s.Progression.IndustryTime = cap - 1000
	if !approx(IndustryTimeResidualToCap(s, b), 1000) {
		t.Fatalf("residual = %v, want 1000", IndustryTimeResidualToCap(s, b))
	}
	s.Progression.IndustryTime = cap + 5000
	if IndustryTimeResidualToCap(s, b) != 0 {
		t.Fatalf("over cap residual = %v, want 0", IndustryTimeResidualToCap(s, b))
	}
}

func TestEffectiveIndustryDTIdleMult(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Progression.MaxUnlockedGen = 5
	s.Progression.IndustryTime = 0
	// No frontier, no training → idle.
	const econ = 1000.0
	got := EffectiveIndustryDT(s, econ, b)
	want := econ * balance.IndustryIdleMult
	if !approx(got, want) {
		t.Fatalf("idle DT = %v, want %v", got, want)
	}
}

func TestEffectiveIndustryDTEngagedFull(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Progression.MaxUnlockedGen = 5
	s.Progression.IndustryTime = 0
	s.Progression.Frontier.Active = true
	const econ = 1000.0
	if !approx(EffectiveIndustryDT(s, econ, b), econ) {
		t.Fatalf("frontier engaged DT = %v, want %v", EffectiveIndustryDT(s, econ, b), econ)
	}
	s.Progression.Frontier.Active = false
	s.HasTraining = true
	if !approx(EffectiveIndustryDT(s, econ, b), econ) {
		t.Fatalf("training engaged DT = %v, want %v", EffectiveIndustryDT(s, econ, b), econ)
	}
}

func TestEffectiveIndustryDTAtCapZero(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Progression.MaxUnlockedGen = 5
	s.Progression.IndustryTime = IndustryTimeCapSec(s, b)
	s.Progression.Frontier.Active = true
	if EffectiveIndustryDT(s, 1e6, b) != 0 {
		t.Fatalf("at cap DT = %v, want 0", EffectiveIndustryDT(s, 1e6, b))
	}
}

func TestEffectiveIndustryDTNegativeEconomy(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Progression.MaxUnlockedGen = 1
	if EffectiveIndustryDT(s, -10, b) != 0 {
		t.Fatal("negative economyDT should yield 0")
	}
}

func TestEffectiveIndustryDTCappedByResidual(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Progression.MaxUnlockedGen = 5
	cap := IndustryTimeCapSec(s, b)
	s.Progression.IndustryTime = cap - 50
	s.Progression.Frontier.Active = true
	got := EffectiveIndustryDT(s, 1000, b)
	if !approx(got, 50) {
		t.Fatalf("DT = %v, want residual 50", got)
	}
}

func TestClampIndustryToPlayerCap(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Progression.MaxUnlockedGen = 5
	cap := IndustryTimeCapSec(s, b)
	s.Progression.IndustryTime = cap * 4
	// Rival quality far above post-clamp GlobalFrontier.
	s.Competitors = []model.Competitor{{
		Name: "OpenAI", Skill: q(1, 1, 1, 1), Quality: q(500, 500, 500, 500),
	}}
	s.Models = []model.Model{{Online: true, Quality: q(10, 10, 10, 10)}}

	ns := ClampIndustryToPlayerCap(s, b)
	if ns.Progression.IndustryTime > cap+1e-6 {
		t.Fatalf("IndustryTime = %v, want ≤ cap %v", ns.Progression.IndustryTime, cap)
	}
	gf := GlobalFrontier(ns, b)
	for d := range model.NumQualityDims {
		q := ns.Competitors[0].Quality[d]
		lo, hi := gf[d]*0.85, gf[d]*1.15
		if q < lo-1e-6 || q > hi+1e-6 {
			t.Fatalf("rival dim %d Q=%v outside band [%v,%v] gf=%v", d, q, lo, hi, gf[d])
		}
	}
	// Under-cap state is a no-op on IndustryTime.
	s2 := s
	s2.Progression.IndustryTime = cap / 2
	ns2 := ClampIndustryToPlayerCap(s2, b)
	if !approx(ns2.Progression.IndustryTime, cap/2) {
		t.Fatalf("under-cap IndustryTime mutated: %v", ns2.Progression.IndustryTime)
	}
}

func TestIndustryIdleMultConstant(t *testing.T) {
	if balance.IndustryPlayerLeadGens != 1 {
		t.Fatalf("LeadGens = %d, want 1", balance.IndustryPlayerLeadGens)
	}
	if math.Abs(balance.IndustryIdleMult-0.15) > 1e-12 {
		t.Fatalf("IdleMult = %v, want 0.15", balance.IndustryIdleMult)
	}
}

func TestIndustryTimeCapSecFiniteForNormalGens(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Progression.MaxUnlockedGen = 5
	cap := IndustryTimeCapSec(s, b)
	if math.IsInf(cap, 0) || math.IsNaN(cap) || cap <= 0 {
		t.Fatalf("Gen5+1 cap should be finite positive, got %v", cap)
	}
}
