package sim

import (
	"math"
	"sort"

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

// RnDRatePerSec is the steady R&D generated each second by staff and stars,
// after prestige multipliers (the bursty token inflow is surfaced separately).
func RnDRatePerSec(ns model.GameState, b balance.Config) float64 {
	pe := PrestigeEffects(ns.Prestige.UnlockedPrestige, b)
	return (staffRnDPerSec(ns.Research, b) + starEffects(ns, b).RnDPerSec) * pe.RnDMult
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

// IsDraft reports whether m is a publishable draft (v1: offline and never used).
func IsDraft(m model.Model) bool {
	return !m.Online && m.Users == 0
}

// EstimateUserTarget is the equilibrium user count advanceUsers would approach
// for models[modelIndex] if it were online at the given price. Returns 0 if
// index invalid. Pure.
func EstimateUserTarget(s model.GameState, modelIndex int, price float64, b balance.Config) float64 {
	if modelIndex < 0 || modelIndex >= len(s.Models) {
		return 0
	}
	m := s.Models[modelIndex]
	if int(m.Segment) < 0 || int(m.Segment) >= model.NumSegments {
		return 0
	}
	if price <= 0 {
		return 0
	}
	te := techEffects(s, b)
	se := starEffects(s, b)
	ee := eventEffects(s, b)
	w := b.SegmentWeights[m.Segment]
	w[model.DimSafety] *= ee.SafetyWeightMult
	appeal := appealOf(m.Quality, w)
	rivalAppeal := 0.0
	for _, c := range s.Competitors {
		rivalAppeal += appealOf(c.Quality, w)
	}
	share := 1.0
	if appeal+rivalAppeal > 0 {
		share = appeal / (appeal + rivalAppeal)
	}
	refPrice := EffectiveRefPrice(s, m.Segment, b)
	demandMult := math.Pow(refPrice/price, b.PriceElasticity)
	marketingMult := 1 + float64(s.Marketing)*b.MarketingBonus
	return appeal * b.SegmentTargetScale[m.Segment] * demandMult * share *
		marketingMult * te.UserGrowthMult * se.UserGrowthMult *
		ee.UserGrowthMult * ee.TAMMult
}

// EffectiveRefPrice returns the reference price of seg, incorporating tech tree multipliers.
func EffectiveRefPrice(s model.GameState, seg model.Segment, b balance.Config) float64 {
	if int(seg) < 0 || int(seg) >= model.NumSegments {
		return 0
	}
	te := techEffects(s, b)
	return b.SegmentRefPrice[seg] * te.RefPriceMult * eventEffects(s, b).RefPriceMult
}

// ShareRow is one entry for market/overview bars.
type ShareRow struct {
	Name  string
	Share float64 // 0..1 of (player best + all competitors) appeal in segment
	You   bool
}

// SegmentShareBars returns player + competitors sorted by share desc.
func SegmentShareBars(ns model.GameState, b balance.Config, seg model.Segment) []ShareRow {
	w := b.SegmentWeights[seg]
	playerBestAppeal := 0.0
	var bestModel *model.Model
	for i := range ns.Models {
		m := &ns.Models[i]
		if m.Online && m.Segment == seg {
			a := appealOf(m.Quality, w)
			if a > playerBestAppeal {
				playerBestAppeal = a
				bestModel = m
			}
		}
	}

	playerName := "你"
	if bestModel != nil {
		playerName = bestModel.Name
	}

	type rivalRow struct {
		name   string
		appeal float64
	}
	var rivals []rivalRow
	for _, c := range ns.Competitors {
		rivals = append(rivals, rivalRow{
			name:   c.Name,
			appeal: appealOf(c.Quality, w),
		})
	}

	totalAppeal := playerBestAppeal
	for _, r := range rivals {
		totalAppeal += r.appeal
	}

	var rows []ShareRow
	if totalAppeal > 0 {
		rows = append(rows, ShareRow{
			Name:  playerName,
			Share: playerBestAppeal / totalAppeal,
			You:   true,
		})
		for _, r := range rivals {
			rows = append(rows, ShareRow{
				Name:  r.name,
				Share: r.appeal / totalAppeal,
				You:   false,
			})
		}
	} else {
		rows = append(rows, ShareRow{
			Name:  playerName,
			Share: 0.0,
			You:   true,
		})
		for _, r := range rivals {
			rows = append(rows, ShareRow{
				Name:  r.name,
				Share: 0.0,
				You:   false,
			})
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Share == rows[j].Share {
			return rows[i].You
		}
		return rows[i].Share > rows[j].Share
	})

	return rows
}

// ThreatLevel: 0 low, 1 mid, 2 high — rival appeal vs player's best in seg.
func ThreatLevel(ns model.GameState, b balance.Config, seg model.Segment, rival model.Competitor) int {
	w := b.SegmentWeights[seg]
	rivalAppeal := appealOf(rival.Quality, w)

	playerBestAppeal := 0.0
	for _, m := range ns.Models {
		if m.Online && m.Segment == seg {
			a := appealOf(m.Quality, w)
			if a > playerBestAppeal {
				playerBestAppeal = a
			}
		}
	}

	if playerBestAppeal == 0 {
		if rivalAppeal > 0 {
			return 2
		}
		return 0
	}

	if rivalAppeal > playerBestAppeal*1.1 {
		return 2
	}
	if rivalAppeal >= playerBestAppeal*0.9 {
		return 1
	}
	return 0
}

// ServableUsers is max users inference capacity can support; 0 capacity → 0
// (caller displays grace copy when capacity==0).
func ServableUsers(ns model.GameState, b balance.Config) float64 {
	if b.InferenceLoadPerUser <= 0 {
		return 0
	}
	cap := EffectiveInference(ns, b)
	return cap / b.InferenceLoadPerUser
}
