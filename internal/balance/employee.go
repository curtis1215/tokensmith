package balance

import (
	"math"

	"tokensmith/internal/model"
)

// Employee / office / talent-market knobs live on Config (see balance.go).
// Tables follow design spec §2.1–§2.4 and §4.2.

// OfficeSeatsAt returns seat capacity at the given office level (1..Max).
// Invalid levels return 0.
func OfficeSeatsAt(level int, b Config) int {
	if level < 0 || level >= len(b.OfficeSeats) {
		return 0
	}
	return b.OfficeSeats[level]
}

// OfficeUpgradeCostAt returns the cash cost to upgrade from level → level+1.
// ok is false when already at MaxOfficeLevel or level is out of range.
func OfficeUpgradeCostAt(level int, b Config) (cost float64, ok bool) {
	if level < 1 || level >= b.MaxOfficeLevel || level >= len(b.OfficeUpgradeCost) {
		return 0, false
	}
	return b.OfficeUpgradeCost[level], true
}

// RerollCost is Base × Growth^rerollCount (0-based: first paid reroll uses Base).
func RerollCost(rerollCount int, b Config) float64 {
	if rerollCount < 0 {
		rerollCount = 0
	}
	return b.MarketRerollBase * math.Pow(b.MarketRerollGrowth, float64(rerollCount))
}

// MonthlyToPerSec converts a monthly salary display amount to cash/sec tick burn.
func MonthlyToPerSec(monthly float64, b Config) float64 {
	if b.SecondsPerMonth <= 0 {
		return 0
	}
	return monthly / b.SecondsPerMonth
}

// applyEmployeeDefaults fills office, market, rank, and salary knobs on c.
// Caller must set MonthSec first: SecondsPerMonth shares the same sim-month
// unit so payroll and subscription revenue stay on one clock (TUI tickDT=3600).
func applyEmployeeDefaults(c *Config) {
	// Align monthly salary burn with MonthSec (not wall-second 600). Using 600
	// made each TUI tick deduct ~6 months of pay and bankrupted hires in ~1s.
	if c.MonthSec > 0 {
		c.SecondsPerMonth = c.MonthSec
	} else {
		c.SecondsPerMonth = 2592000
	}
	c.MaxOfficeLevel = 8

	// Index by level 1..8; index 0 unused.
	c.OfficeSeats = [9]int{0, 3, 5, 8, 12, 16, 22, 28, 36}
	// Cost to go from level i → i+1 (index = current level). L8 has no upgrade.
	c.OfficeUpgradeCost = [9]float64{
		0,
		25000,   // 1→2
		80000,   // 2→3
		200000,  // 3→4
		500000,  // 4→5
		1200000, // 5→6
		3000000, // 6→7
		8000000, // 7→8
		0,       // 8 maxed
	}
	c.OfficeNames = [9]string{
		"",
		"車庫",
		"小辦公室",
		"開放式樓層",
		"辦公樓",
		"園區",
		"摩天樓",
		"巨塔",
		"太空電梯",
	}

	c.MarketPoolSize = 5
	// Design: ~10 wall-minutes free refresh → sim seconds via RealSecCompression.
	c.MarketRefreshSec = 600 * RealSecCompression
	c.MarketRerollBase = 5000
	c.MarketRerollGrowth = 2

	// RankWeights[level-1][rank]: relative weights, normalize at roll time.
	// Spec §3.3 — L1 blocks director/god; god opens (tiny) at L3.
	c.RankWeights = [8][model.NumRanks]float64{
		// L1
		{55, 30, 12, 3, 0, 0},
		// L2
		{45, 32, 16, 5, 2, 0},
		// L3
		{35, 32, 20, 9, 3, 1},
		// L4
		{25, 30, 24, 13, 6, 2},
		// L5
		{18, 28, 26, 16, 9, 3},
		// L6
		{12, 25, 26, 18, 13, 6},
		// L7
		{8, 22, 25, 20, 16, 9},
		// L8
		{5, 18, 24, 22, 18, 13},
	}

	// Multi-spec mode base weights: single / dual / tri / quad.
	c.MultiSpecWeights = [4]float64{70, 22, 7, 1}

	// Stat bands by rank: high dims, normal dims, floor (spec §3.4).
	c.RankStatHigh = [model.NumRanks][2]int{
		{15, 30},  // grunt
		{30, 50},  // staff
		{45, 65},  // lead
		{60, 80},  // manager
		{75, 92},  // director
		{88, 100}, // god
	}
	c.RankStatNorm = [model.NumRanks][2]int{
		{8, 18},
		{15, 30},
		{25, 40},
		{35, 55},
		{45, 65},
		{55, 75},
	}
	c.RankStatFloor = [model.NumRanks]int{5, 10, 15, 20, 30, 40}

	// Monthly base pay by rank (spec §4.2).
	c.RankBaseMonth = [model.NumRanks]float64{800, 2500, 6000, 15000, 40000, 100000}

	c.SalaryStatFactor = 0.8
	c.SalarySkillFactor = 0.12
	// MultiSpecSalaryMult[highCount-1]: single, dual, tri, quad.
	c.MultiSpecSalaryMult = [4]float64{1.00, 1.08, 1.18, 1.30}

	c.HireMonths = 2
	c.SeveranceMonths = 0.5

	c.PrimaryWeight = 1.0
	c.SecondaryWeight = 0.35

	// Diminishing staff-power curve for engineer/ops/marketing mults.
	c.StaffPowerCap = 0.8
	c.StaffPowerK = 1.2
	c.StaffPowerRef = 200

	// RnDPerPower: R&D per sim-second per unit research RolePower (× EfficiencyMult).
	// 0.0002 is the pre-compression calibration; divide by RealSecCompression so
	// TUI perceived rate matches the rest of the economy (same contract as old
	// ResearcherRnDPerSec which was already stored compressed).
	c.RnDPerPower = 0.0002 / RealSecCompression

	// Flat fallback when a mid-run save has no probeable legacy headcount.
	c.RestructuringGrant = 25_000
}
