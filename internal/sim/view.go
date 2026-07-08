package sim

import (
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// EffectiveTraining is the exported view of self-built + rented training compute.
func EffectiveTraining(ns model.GameState, b balance.Config) float64 {
	return effectiveTraining(ns, b)
}

// EffectiveInference is the exported view of self-built + rented inference compute.
func EffectiveInference(ns model.GameState, b balance.Config) float64 {
	return effectiveInference(ns, b)
}

// TotalUsers sums users across online models.
func TotalUsers(ns model.GameState) float64 {
	var u float64
	for _, m := range ns.Models {
		if m.Online {
			u += m.Users
		}
	}
	return u
}

// MonthlyRevenue is the aggregate per-month subscription revenue of online models.
func MonthlyRevenue(ns model.GameState) float64 {
	var r float64
	for _, m := range ns.Models {
		if m.Online {
			r += m.Users * m.Price
		}
	}
	return r
}

// MarketRank returns the player's 1-based rank by appeal in seg among the
// player's best online model and every competitor, plus the field size.
func MarketRank(ns model.GameState, b balance.Config, seg model.Segment) (rank, total int) {
	w := b.SegmentWeights[seg]
	best := 0.0
	for _, m := range ns.Models {
		if m.Online {
			if a := appealOf(m.Quality, w); a > best {
				best = a
			}
		}
	}
	rank = 1
	for _, c := range ns.Competitors {
		if appealOf(c.Quality, w) > best {
			rank++
		}
	}
	return rank, len(ns.Competitors) + 1
}

// NextMilestone returns the next unreached valuation milestone and progress
// toward it. ok is false when every milestone has been reached.
func NextMilestone(ns model.GameState, b balance.Config) (target, progress float64, ok bool) {
	if ns.MilestonesReached >= len(b.ValuationMilestones) {
		return 0, 0, false
	}
	target = b.ValuationMilestones[ns.MilestonesReached]
	progress = ns.PeakValuation / target
	if progress > 1 {
		progress = 1
	}
	return target, progress, true
}
