// Package sim is the pure, deterministic simulation core.
// No wall-clock, no I/O, and no non-deterministic randomness — all event
// randomness flows through GameState.Events.RandState (splitmix64); time
// advances only via dt. Same state + same inputs → same result.
package sim

import (
	"math"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// staffRnDPerSec is a temporary stub (aggregate researchers removed).
// Employee R&D is staffRnDPerSecFromEmployees; Task 7 rewires Tick callers.
func staffRnDPerSec(r model.Research, _ balance.Config) float64 {
	_ = r
	return 0
}

// TokenRawRnD returns the raw R&D produced by a batch of token events, before
// any soft-cap diminishing is applied. Exported so the TUI display layer can
// compute the same per-source amount it's about to book (avoids the display
// and the actual booked value drifting apart).
func TokenRawRnD(events []model.TokenEvent, b balance.Config) float64 {
	var raw float64
	for _, e := range events {
		raw += (float64(e.InputTokens)*b.TokenInputWeight + float64(e.OutputTokens)*b.TokenOutputWeight) / b.TokenDivisor
	}
	return raw
}

// totalSalaryPerSec is the aggregate staff salary per second.
// Temporary: employee path until Tick fully rewires in Task 7.
func totalSalaryPerSec(ns model.GameState, b balance.Config) float64 {
	return totalSalaryPerSecFromEmployees(ns, b)
}

// starSalaryPerSec is a neutral stub (stars removed).
func starSalaryPerSec(_ model.GameState, _ balance.Config) float64 {
	return 0
}

// Valuation is the company's estimated worth (spec §7.0).
func Valuation(ns model.GameState, b balance.Config) float64 {
	ce := campaignEffects(ns, b)
	var monthlyRev, users float64
	for _, m := range ns.Models {
		if m.Online {
			revMult := 1.0
			if int(m.Segment) >= 0 && int(m.Segment) < model.NumSegments {
				revMult = ce.RevenueMult[m.Segment]
			}
			monthlyRev += m.Users * m.Price * b.RevenueMult * revMult
			users += m.Users
		}
	}
	assets := ns.Resources.Cash
	for _, sv := range ns.Servers {
		assets += sv.Compute * b.ServerAssetValue
	}
	return (monthlyRev*b.RevenueMultiple + users*b.UserValue + assets) *
		eventEffects(ns, b).ValuationMult
}

// Tick advances the simulation by dt seconds on both the economy and industry
// clocks (online play). Pure: it does not mutate s.
func Tick(s model.GameState, dt float64, events []model.TokenEvent, b balance.Config) model.GameState {
	return tickWithClocks(s, dt, dt, events, b)
}

// OfflineTick is the narrow dual-clock entry point for offline settlement:
// economyDT drives GameTime, R&D, cash, training, frontier research, users,
// and serving; industryDT drives IndustryTime and rival league catch-up only.
func OfflineTick(s model.GameState, economyDT, industryDT float64, events []model.TokenEvent, b balance.Config) model.GameState {
	return tickWithClocks(s, economyDT, industryDT, events, b)
}

// tickWithClocks advances economy and industry on independent deltas.
func tickWithClocks(s model.GameState, economyDT, industryDT float64, events []model.TokenEvent, b balance.Config) model.GameState {
	if economyDT < 0 {
		economyDT = 0
	}
	if industryDT < 0 {
		industryDT = 0
	}
	ns := s
	ns.GameTime += economyDT
	ns.Progression.IndustryTime += industryDT
	// Events age with the economy clock (same as historical offline settle).
	ns = advanceEvents(ns, b)
	ee := eventEffects(ns, b)

	staffRnD := staffRnDPerSec(s.Research, b) * economyDT

	tokenRnD := TokenRawRnD(events, b)

	pe := PrestigeEffects(ns.Prestige.UnlockedPrestige, b)
	starRnD := starEffects(ns, b).RnDPerSec * economyDT
	ns.Resources.RnD += (staffRnD+starRnD)*pe.RnDMult + tokenRnD*b.StreakMult*pe.RnDMult
	ns.Resources.Cash -= poolRentPerSec(ns, b) * economyDT
	serverPower := 0.0
	for _, sv := range ns.Servers {
		serverPower += sv.PowerKW
	}
	ns.Resources.Cash -= serverPower * b.ElectricityPerKWSec * ee.PowerCostMult * economyDT
	ns.Resources.Cash -= (totalSalaryPerSec(ns, b) + starSalaryPerSec(ns, b)) * economyDT

	// Shared training pool (economy clock).
	trainEff := effectiveTraining(ns, b)
	frontierAlloc, modelAlloc := trainEff, trainEff
	if ns.Progression.Frontier.Active {
		pct := ns.Progression.Frontier.AllocationPct
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}
		frontierAlloc = trainEff * float64(pct) / 100
		modelAlloc = trainEff * float64(100-pct) / 100
	} else {
		frontierAlloc = 0
	}
	ns = advanceFrontierProject(ns, economyDT, frontierAlloc)
	ns = advanceTraining(ns, economyDT, modelAlloc, b)
	// Rival league / global-frontier catch-up uses the industry clock only.
	ns = advanceCompetitors(ns, industryDT, b)
	ns = advanceUsers(ns, economyDT, b)
	ns = advanceServing(ns, economyDT, b)
	val := Valuation(ns, b)
	if val > ns.PeakValuation {
		ns.PeakValuation = val
	}
	for ns.MilestonesReached < len(b.ValuationMilestones) &&
		ns.PeakValuation >= b.ValuationMilestones[ns.MilestonesReached] {
		ns.MilestonesReached++
	}
	return ns
}

// infraEfficiency scales compute effectiveness. Temporary neutral stub;
// Task 7 wires employeeInfraMult (and skill InfraMult) into this path.
func infraEfficiency(_ model.GameState, _ balance.Config) float64 {
	return 1
}

// effectiveTraining is rented plus self-built training compute, scaled by engineer efficiency.
func effectiveTraining(ns model.GameState, b balance.Config) float64 {
	c := poolCompute(ns.Compute.RentedTraining, b)
	for _, sv := range ns.Servers {
		if sv.Pool == model.PoolTraining {
			c += sv.Compute
		}
	}
	return c * infraEfficiency(ns, b) * techEffects(ns, b).InfraMult * starEffects(ns, b).InfraMult
}

// effectiveInference is rented plus self-built inference compute, scaled by engineer efficiency.
func effectiveInference(ns model.GameState, b balance.Config) float64 {
	c := poolCompute(ns.Compute.RentedInference, b)
	for _, sv := range ns.Servers {
		if sv.Pool == model.PoolInference {
			c += sv.Compute
		}
	}
	return c * infraEfficiency(ns, b) * techEffects(ns, b).InfraMult * starEffects(ns, b).InfraMult
}

// advanceTraining progresses the in-progress training job by dt using the
// model-training share of the training pool, then appends the completed model
// as a draft. Pure: clones Models before appending.
func advanceTraining(ns model.GameState, dt, allocated float64, b balance.Config) model.GameState {
	if !ns.HasTraining {
		return ns
	}
	ns.Training.WorkRemaining -= allocated * dt
	if ns.Training.WorkRemaining > 0 {
		return ns
	}
	// Completed → append draft (not online until PublishModel).
	te := techEffects(ns, b)
	se := starEffects(ns, b)
	job := ns.Training
	spec, err := balance.Generation(job.Gen)
	qualityScale := 0.0
	if err == nil {
		qualityScale = spec.QualityScale
	}
	m := model.Model{
		Gen:     job.Gen,
		Segment: job.Segment,
		Price:   job.Price,
		Online:  false,
		Users:   0,
		Name:    "",
	}
	for d := range model.NumQualityDims {
		m.Quality[d] = (job.Alloc[d]*qualityScale + job.CashBonus[d]) * te.QualityMult[d] * se.QualityMult[d]
	}
	cloned := append([]model.Model(nil), ns.Models...)
	ns.Models = append(cloned, m)
	ns.HasTraining = false
	ns.Training = model.TrainingJob{}
	return ns
}

// appealOf is the weighted quality score of a model or competitor.
func appealOf(q, w [model.NumQualityDims]float64) float64 {
	appeal := 0.0
	for d := range model.NumQualityDims {
		appeal += q[d] * w[d]
	}
	return appeal
}

// advanceUsers grows each online model's user base toward a segment-specific
// demand target and accrues subscription revenue, scaled by competitive market
// share. Pure: clones Models.
func advanceUsers(ns model.GameState, dt float64, b balance.Config) model.GameState {
	if len(ns.Models) == 0 {
		return ns
	}
	pe := PrestigeEffects(ns.Prestige.UnlockedPrestige, b)
	se := starEffects(ns, b)
	ee := eventEffects(ns, b)
	ce := campaignEffects(ns, b)
	models := append([]model.Model(nil), ns.Models...)
	for i := range models {
		m := &models[i]
		if !m.Online {
			continue
		}
		// Guard against a corrupt save carrying an out-of-range segment, which
		// would panic the segment-indexed lookups below.
		if int(m.Segment) < 0 || int(m.Segment) >= model.NumSegments {
			continue
		}
		ns.Resources.Cash += m.Users * m.Price * ce.RevenueMult[m.Segment] * dt / b.MonthSec * pe.CashMult * b.RevenueMult

		w := b.SegmentWeights[m.Segment]
		w[model.DimSafety] *= ee.SafetyWeightMult // arrays copy by value; safe
		if m.Segment == model.SegEnterprise {
			w[model.DimSafety] *= ce.SafetyAppealMult
		}
		appeal := appealOf(m.Quality, w)
		rivalAppeal := 0.0
		for _, c := range ns.Competitors {
			rivalAppeal += appealOf(EffectiveRivalQuality(ns, c, b), w)
		}
		te := techEffects(ns, b)
		refPrice := EffectiveRefPrice(ns, m.Segment, b)
		var demandMult float64
		if m.Price > 0 {
			demandMult = math.Pow(refPrice/m.Price, b.PriceElasticity)
		}
		share := 1.0
		if appeal+rivalAppeal > 0 {
			share = appeal / (appeal + rivalAppeal)
		}
		// Marketing headcount removed; employee mult lands in Task 7.
		marketingMult := 1.0
		target := appeal * b.SegmentTargetScale[m.Segment] * demandMult * share *
			marketingMult * te.UserGrowthMult * se.UserGrowthMult *
			ee.UserGrowthMult * ee.TAMMult
		target *= ce.UserGrowthMult[m.Segment]
		decay := math.Exp(-b.UserGrowthRate * dt)
		m.Users = target + (m.Users-target)*decay
		if m.Users < 0 {
			m.Users = 0
		}
	}
	ns.Models = models
	return ns
}

// playerFrontier is the player's best online-model quality per dimension.
func playerFrontier(ns model.GameState) [model.NumQualityDims]float64 {
	var f [model.NumQualityDims]float64
	for _, m := range ns.Models {
		if !m.Online {
			continue
		}
		for d := range model.NumQualityDims {
			if m.Quality[d] > f[d] {
				f[d] = m.Quality[d]
			}
		}
	}
	return f
}

// advanceCompetitors runs the bounded rival league (global-frontier band).
// Campaign and non-campaign play share the same engine — Tick no longer freezes
// rivals during an active campaign (roadmap actions are additive in Task 10).
func advanceCompetitors(ns model.GameState, dt float64, b balance.Config) model.GameState {
	return advanceRivalLeague(ns, dt, b)
}

// advanceServing computes inference load and, when provisioned inference
// capacity cannot meet it, churns users by the service deficit. Pure: clones
// Models. v0: zero capacity is graced (no churn) so pre-inference behavior is
// unchanged.
//
// Large-dt stability: the old Euler form
//
//	users -= users * rate * deficit * dt
//
// with TUI tickDT=3600 and rate=0.01 wipes users to 0 on any meaningful
// overload (rate*dt=36). Next tick they regrow toward market target, then wipe
// again → visible 5k/9k/0k thrash after the Gen1 scale-up. Instead we
// exponentially approach the load that capacity can support:
//
//	newLoad = capacity + (load-capacity)*exp(-rate*dt*ops)
//
// then scale every online model's users by newLoad/load. For small dt this
// matches the linear Euler to first order; for hour ticks it settles at the
// servable user count instead of oscillating through zero.
func advanceServing(ns model.GameState, dt float64, b balance.Config) model.GameState {
	ce := campaignEffects(ns, b)
	load := 0.0
	for _, m := range ns.Models {
		if m.Online {
			load += m.Users * b.InferenceLoadPerUser * ce.InferenceLoadMult
		}
	}
	ns.Compute.InferenceLoad = load
	capacity := effectiveInference(ns, b)
	if capacity <= 0 || load <= capacity {
		return ns
	}
	// Ops headcount removed; employeeOpsChurnFactor lands in Task 7.
	opsFactor := 1.0
	// Continuous approach of total load toward capacity.
	newLoad := capacity + (load-capacity)*math.Exp(-b.ServiceChurnRate*ce.ServiceChurnMult*dt*opsFactor)
	if newLoad < 0 {
		newLoad = 0
	}
	scale := newLoad / load
	models := append([]model.Model(nil), ns.Models...)
	for i := range models {
		m := &models[i]
		if !m.Online {
			continue
		}
		m.Users *= scale
		if m.Users < 0 {
			m.Users = 0
		}
	}
	ns.Models = models
	// Keep the displayed load consistent with post-churn users.
	ns.Compute.InferenceLoad = newLoad
	return ns
}
