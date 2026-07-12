package balance

import (
	"math"
	"testing"

	"tokensmith/internal/model"
)

func TestOfficeTable(t *testing.T) {
	b := Default()
	if b.MaxOfficeLevel != 8 {
		t.Fatalf("max=%d", b.MaxOfficeLevel)
	}
	if OfficeSeatsAt(1, b) != 3 || OfficeSeatsAt(8, b) != 36 {
		t.Fatalf("seats L1=%d L8=%d", OfficeSeatsAt(1, b), OfficeSeatsAt(8, b))
	}
	c, ok := OfficeUpgradeCostAt(1, b)
	if !ok || c != 25000 {
		t.Fatalf("upgrade L1→2: %v %v", c, ok)
	}
	if _, ok := OfficeUpgradeCostAt(8, b); ok {
		t.Fatal("L8 should not upgrade")
	}
}

func TestRerollCostGeometric(t *testing.T) {
	b := Default()
	if RerollCost(0, b) != 5000 || RerollCost(1, b) != 10000 || RerollCost(2, b) != 20000 {
		t.Fatalf("reroll: %v %v %v", RerollCost(0, b), RerollCost(1, b), RerollCost(2, b))
	}
}

func TestRankWeightsL1NoGod(t *testing.T) {
	b := Default()
	if b.RankWeights[0][model.RankGod] != 0 || b.RankWeights[0][model.RankDirector] != 0 {
		t.Fatalf("L1 weights: %+v", b.RankWeights[0])
	}
	if b.RankWeights[7][model.RankGod] <= 0 {
		t.Fatal("L8 must allow god")
	}
}

func TestMonthlyToPerSec(t *testing.T) {
	b := Default()
	if b.SecondsPerMonth != 600 {
		t.Fatal(b.SecondsPerMonth)
	}
	got := MonthlyToPerSec(6000, b)
	if math.Abs(got-10) > 1e-9 {
		t.Fatalf("got %v want 10", got)
	}
}
