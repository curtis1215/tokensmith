// Package sim is the pure, deterministic simulation core.
// No wall-clock, no randomness, no I/O — time advances only via dt.
package sim

import (
	"math"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// staffRnDPerSec returns R&D produced per second by the research workforce,
// before multiplying by dt.
func staffRnDPerSec(r model.Research, b balance.Config) float64 {
	var perSec float64
	for tier := model.Tier1; tier <= model.Tier3; tier++ {
		perSec += float64(r.Researchers[tier]) * b.ResearcherRnDPerSec[tier]
	}
	return perSec * r.EfficiencyMult
}

// tokenRawRnD returns the raw R&D produced by a batch of token events,
// before any soft-cap diminishing is applied.
func tokenRawRnD(events []model.TokenEvent, b balance.Config) float64 {
	var raw float64
	for _, e := range events {
		raw += (float64(e.InputTokens)*b.TokenInputWeight + float64(e.OutputTokens)*b.TokenOutputWeight) / b.TokenDivisor
	}
	return raw
}

// applySoftCap diminishes raw token R&D once cumulative window R&D passes full.
// Returns the effective R&D to grant and the updated window cumulative.
func applySoftCap(windowRnD, raw, full, mult float64) (effective, newWindow float64) {
	newWindow = windowRnD + raw
	if windowRnD >= full {
		return raw * mult, newWindow
	}
	remainingFull := full - windowRnD
	if raw <= remainingFull {
		return raw, newWindow
	}
	over := raw - remainingFull
	return remainingFull + over*mult, newWindow
}

// totalSalaryPerSec is the aggregate staff salary per second.
func totalSalaryPerSec(ns model.GameState, b balance.Config) float64 {
	var s float64
	for tier := model.Tier1; tier <= model.Tier3; tier++ {
		s += float64(ns.Research.Researchers[tier]) * b.ResearcherSalaryPerSec[tier]
	}
	s += float64(ns.Engineers) * b.EngineerSalaryPerSec
	s += float64(ns.Ops) * b.OpsSalaryPerSec
	s += float64(ns.Marketing) * b.MarketingSalaryPerSec
	return s
}

// starSalaryPerSec is the aggregate salary of all hired stars.
func starSalaryPerSec(ns model.GameState, b balance.Config) float64 {
	var s float64
	for _, st := range b.Stars {
		if isStarHired(ns, st.ID) {
			s += st.SalaryPerSec
		}
	}
	return s
}

// Valuation is the company's estimated worth (spec §7.0).
func Valuation(ns model.GameState, b balance.Config) float64 {
	var monthlyRev, users float64
	for _, m := range ns.Models {
		if m.Online {
			monthlyRev += m.Users * m.Price * b.RevenueMult
			users += m.Users
		}
	}
	assets := ns.Resources.Cash
	for _, sv := range ns.Servers {
		assets += sv.Compute * b.ServerAssetValue
	}
	return monthlyRev*b.RevenueMultiple + users*b.UserValue + assets
}

// Tick advances the simulation by dt seconds and returns the new state.
// Pure: it does not mutate s and depends only on its arguments.
func Tick(s model.GameState, dt float64, events []model.TokenEvent, b balance.Config) model.GameState {
	ns := s
	ns.GameTime += dt

	// Advance the soft-cap window; reset cumulative when the window elapses.
	ns.WindowElapsed += dt
	if ns.WindowElapsed >= b.SoftCapWindowSec {
		ns.WindowElapsed -= b.SoftCapWindowSec
		ns.WindowRnD = 0
	}

	staffRnD := staffRnDPerSec(s.Research, b) * dt

	raw := tokenRawRnD(events, b)
	tokenRnD, newWindow := applySoftCap(ns.WindowRnD, raw, b.SoftCapFull, b.SoftCapMult)
	ns.WindowRnD = newWindow

	pe := prestigeEffects(ns.Prestige.UnlockedPrestige, b)
	starRnD := starEffects(ns, b).RnDPerSec * dt
	ns.Resources.RnD += (staffRnD + tokenRnD + starRnD) * pe.RnDMult
	ns.Resources.Cash -= poolRentPerSec(ns, b) * dt
	serverPower := 0.0
	for _, sv := range ns.Servers {
		serverPower += sv.PowerKW
	}
	ns.Resources.Cash -= serverPower * b.ElectricityPerKWSec * dt
	ns.Resources.Cash -= (totalSalaryPerSec(ns, b) + starSalaryPerSec(ns, b)) * dt
	ns = advanceTraining(ns, dt, b)
	ns = advanceCompetitors(ns, dt, b)
	ns = advanceUsers(ns, dt, b)
	ns = advanceServing(ns, dt, b)
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

// infraEfficiency scales compute effectiveness with engineers.
func infraEfficiency(ns model.GameState, b balance.Config) float64 {
	return 1 + float64(ns.Engineers)*b.EngineerInfraBonus
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

// advanceTraining progresses the in-progress training job by dt and appends
// the completed model as a draft. Pure: clones Models before appending.
func advanceTraining(ns model.GameState, dt float64, b balance.Config) model.GameState {
	if !ns.HasTraining {
		return ns
	}
	ns.Training.WorkRemaining -= effectiveTraining(ns, b) * dt
	if ns.Training.WorkRemaining > 0 {
		return ns
	}
	// Completed → append draft (not online until PublishModel).
	te := techEffects(ns, b)
	se := starEffects(ns, b)
	job := ns.Training
	m := model.Model{
		Gen:     job.Gen,
		Segment: job.Segment,
		Price:   job.Price,
		Online:  false,
		Users:   0,
		Name:    "",
	}
	for d := range model.NumQualityDims {
		m.Quality[d] = job.Alloc[d] * b.GenQualityCap[job.Gen] * te.QualityMult[d] * se.QualityMult[d]
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
	pe := prestigeEffects(ns.Prestige.UnlockedPrestige, b)
	se := starEffects(ns, b)
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
		ns.Resources.Cash += m.Users * m.Price * dt / b.MonthSec * pe.CashMult * b.RevenueMult

		w := b.SegmentWeights[m.Segment]
		appeal := appealOf(m.Quality, w)
		rivalAppeal := 0.0
		for _, c := range ns.Competitors {
			rivalAppeal += appealOf(c.Quality, w)
		}
		te := techEffects(ns, b)
		refPrice := b.SegmentRefPrice[m.Segment] * te.RefPriceMult
		var demandMult float64
		if m.Price > 0 {
			demandMult = math.Pow(refPrice/m.Price, b.PriceElasticity)
		}
		share := 1.0
		if appeal+rivalAppeal > 0 {
			share = appeal / (appeal + rivalAppeal)
		}
		marketingMult := 1 + float64(ns.Marketing)*b.MarketingBonus
		target := appeal * b.SegmentTargetScale[m.Segment] * demandMult * share * marketingMult * te.UserGrowthMult * se.UserGrowthMult
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

// advanceCompetitors rubber-bands each competitor's quality toward
// Skill×max(playerFrontier, base), so rivals track the player's progress
// instead of running away on a fixed curve. Pure: clones Competitors.
//
// When the player has an online model, targets are also soft-capped at
// CompetitorMaxLead×frontier so Skill>1 rivals cannot jump a full generation
// ahead during the Gen1→Gen2 R&D farm window.
func advanceCompetitors(ns model.GameState, dt float64, b balance.Config) model.GameState {
	if len(ns.Competitors) == 0 {
		return ns
	}
	frontier := playerFrontier(ns)
	factor := b.CompetitorCatchupRate * dt
	if factor > 1 {
		factor = 1
	} else if factor < 0 {
		factor = 0
	}
	comps := append([]model.Competitor(nil), ns.Competitors...)
	for i := range comps {
		for d := range model.NumQualityDims {
			ref := frontier[d]
			if b.CompetitorBaseQuality > ref {
				ref = b.CompetitorBaseQuality
			}
			target := comps[i].Skill[d] * ref
			// Soft-cap lead only once the player has established a frontier on
			// this dim — pre-product rivals still settle near Skill×base.
			if frontier[d] > 0 && b.CompetitorMaxLead > 0 {
				leadCap := frontier[d] * b.CompetitorMaxLead
				if target > leadCap {
					target = leadCap
				}
			}
			comps[i].Quality[d] += (target - comps[i].Quality[d]) * factor
		}
	}
	ns.Competitors = comps
	return ns
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
	load := 0.0
	for _, m := range ns.Models {
		if m.Online {
			load += m.Users * b.InferenceLoadPerUser
		}
	}
	ns.Compute.InferenceLoad = load
	capacity := effectiveInference(ns, b)
	if capacity <= 0 || load <= capacity {
		return ns
	}
	opsFactor := 1.0 / (1 + float64(ns.Ops)*b.OpsChurnReduction)
	// Continuous approach of total load toward capacity.
	newLoad := capacity + (load-capacity)*math.Exp(-b.ServiceChurnRate*dt*opsFactor)
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
