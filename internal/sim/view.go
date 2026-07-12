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

// RivalFrontier is a pure read-only projection of one rival versus the global
// frontier. Ranking/appeal math is unchanged; this only explains position.
type RivalFrontier struct {
	Active           bool
	Name             string
	AbsoluteQuality  [model.NumQualityDims]float64
	GlobalFrontier   [model.NumQualityDims]float64
	FrontierDeltaPct [model.NumQualityDims]float64 // quality/global - 1; 0 if global==0
	IsLeader         bool
	Specialty        model.QualityDim // strongest Skill dimension
	SpecialtyPct     float64          // Skill[Specialty]
	MomentumPct      [model.NumQualityDims]float64
	MomentumCycles   int
}

// RivalFrontierView projects competitors[index] against GlobalFrontier.
// Invalid indices return Active=false.
func RivalFrontierView(s model.GameState, index int, b balance.Config) RivalFrontier {
	if index < 0 || index >= len(s.Competitors) {
		return RivalFrontier{}
	}
	c := s.Competitors[index]
	eq := EffectiveRivalQuality(s, c, b)
	gf := GlobalFrontier(s, b)
	v := RivalFrontier{
		Active:          true,
		Name:            c.Name,
		AbsoluteQuality: eq,
		GlobalFrontier:  gf,
		MomentumPct:     c.MomentumPct,
		MomentumCycles:  c.MomentumCycles,
		IsLeader:        isRivalLeader(s, c.Name),
	}
	for d := range model.NumQualityDims {
		if gf[d] > simEpsilon {
			v.FrontierDeltaPct[d] = eq[d]/gf[d] - 1
		}
	}
	best := 0
	for d := 1; d < model.NumQualityDims; d++ {
		if c.Skill[d] > c.Skill[best] {
			best = d
		}
	}
	v.Specialty = model.QualityDim(best)
	v.SpecialtyPct = c.Skill[best]
	return v
}

// CurrentRivalEra is the rival-league era (from Progression.Rivals, else MaxUnlockedGen).
func CurrentRivalEra(s model.GameState, b balance.Config) int {
	if s.Progression.Rivals.Era > 0 {
		return s.Progression.Rivals.Era
	}
	e, err := balance.EraForGen(MaxUnlockedGen(s, b))
	if err != nil || e < 1 {
		return 1
	}
	return e
}

// EffectiveInference is the exported view of self-built + rented inference compute.
func EffectiveInference(ns model.GameState, b balance.Config) float64 {
	return effectiveInference(ns, b)
}

// RnDRatePerSec is the steady R&D generated each second by employees,
// after prestige multipliers (the bursty token inflow is surfaced separately).
func RnDRatePerSec(ns model.GameState, b balance.Config) float64 {
	pe := PrestigeEffects(ns.Prestige.UnlockedPrestige, b)
	return staffRnDPerSecFromEmployees(ns, b) * pe.RnDMult
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

// NetCashPerSec is the steady-state cash delta per game-second from subscription
// revenue minus rent, electricity, and salaries (mirrors Tick accrual, without
// mutating state). Used by developer showdown CashflowOK.
func NetCashPerSec(ns model.GameState, b balance.Config) float64 {
	ce := campaignEffects(ns, b)
	pe := PrestigeEffects(ns.Prestige.UnlockedPrestige, b)
	ee := eventEffects(ns, b)
	sk := passiveSkillEffects(ns, b)
	var rev float64
	for _, m := range ns.Models {
		if !m.Online {
			continue
		}
		if int(m.Segment) < 0 || int(m.Segment) >= model.NumSegments {
			continue
		}
		rev += m.Users * m.Price * ce.RevenueMult[m.Segment] / b.MonthSec *
			pe.CashMult * b.RevenueMult * sk.RevenueMult
	}
	serverPower := 0.0
	for _, sv := range ns.Servers {
		serverPower += sv.PowerKW
	}
	costs := poolRentPerSec(ns, b) +
		serverPower*b.ElectricityPerKWSec*ee.PowerCostMult +
		totalSalaryPerSec(ns, b)
	return rev - costs
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
		if appealOf(EffectiveRivalQuality(ns, c, b), w) > best {
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
	sk := passiveSkillEffects(s, b)
	ee := eventEffects(s, b)
	ce := campaignEffects(s, b)
	w := b.SegmentWeights[m.Segment]
	w[model.DimSafety] *= ee.SafetyWeightMult
	if m.Segment == model.SegEnterprise {
		w[model.DimSafety] *= ce.SafetyAppealMult
	}
	appeal := appealOf(m.Quality, w)
	rivalAppeal := 0.0
	for _, c := range s.Competitors {
		rivalAppeal += appealOf(EffectiveRivalQuality(s, c, b), w)
	}
	share := 1.0
	if appeal+rivalAppeal > 0 {
		share = appeal / (appeal + rivalAppeal)
	}
	refPrice := EffectiveRefPrice(s, m.Segment, b)
	demandMult := math.Pow(refPrice/price, b.PriceElasticity)
	marketingMult := employeeMarketingMult(s, b) * sk.UserGrowthMult
	target := appeal * b.SegmentTargetScale[m.Segment] * demandMult * share *
		marketingMult * te.UserGrowthMult *
		ee.UserGrowthMult * ee.TAMMult
	target *= ce.UserGrowthMult[m.Segment]
	return target
}

// EffectiveRefPrice returns the reference price of seg, incorporating tech tree
// and campaign multipliers.
func EffectiveRefPrice(s model.GameState, seg model.Segment, b balance.Config) float64 {
	if int(seg) < 0 || int(seg) >= model.NumSegments {
		return 0
	}
	te := techEffects(s, b)
	ce := campaignEffects(s, b)
	return b.SegmentRefPrice[seg] * te.RefPriceMult * eventEffects(s, b).RefPriceMult * ce.RefPriceMult[seg]
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
			appeal: appealOf(EffectiveRivalQuality(ns, c, b), w),
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
	rivalAppeal := appealOf(EffectiveRivalQuality(ns, rival, b), w)

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
	ce := campaignEffects(ns, b)
	loadPerUser := b.InferenceLoadPerUser * ce.InferenceLoadMult
	if loadPerUser <= 0 {
		return 0
	}
	cap := EffectiveInference(ns, b)
	return cap / loadPerUser
}
