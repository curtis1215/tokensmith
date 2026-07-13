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
	// Payroll shares MonthSec with subscription revenue (TUI tickDT=3600 safe).
	if b.SecondsPerMonth != b.MonthSec || b.SecondsPerMonth != 2592000 {
		t.Fatalf("SecondsPerMonth=%v MonthSec=%v", b.SecondsPerMonth, b.MonthSec)
	}
	got := MonthlyToPerSec(2592000, b)
	if math.Abs(got-1) > 1e-9 {
		t.Fatalf("got %v want 1", got)
	}
	// One TUI tick (3600 sim-sec) deducts MonthSec/3600 of a month of pay.
	frac := 3600 / b.SecondsPerMonth
	if math.Abs(frac-3600/2592000.0) > 1e-12 {
		t.Fatalf("tick fraction %v", frac)
	}
	if MonthlyToPerSec(6000, b)*3600 > 10 { // 6000*3600/2592000 ≈ 8.33, not 36000
		// guard: never again the old 600-based 6-month-per-tick burn
	}
	tickBurn := MonthlyToPerSec(6000, b) * 3600
	if tickBurn > 20 || tickBurn < 5 {
		t.Fatalf("tick burn for $6000/mo should be ~8.33, got %v", tickBurn)
	}
}

func TestRnDPerPowerIsCompressed(t *testing.T) {
	b := Default()
	want := 0.0002 / RealSecCompression
	if math.Abs(b.RnDPerPower-want) > 1e-15 {
		t.Fatalf("RnDPerPower=%v want %v", b.RnDPerPower, want)
	}
}

func TestMarketRefreshIsWallMinuteScale(t *testing.T) {
	b := Default()
	want := 600 * RealSecCompression
	if b.MarketRefreshSec != want {
		t.Fatalf("MarketRefreshSec=%v want %v", b.MarketRefreshSec, want)
	}
}
