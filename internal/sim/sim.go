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

	ns.Resources.RnD += staffRnD + tokenRnD
	ns.Resources.Cash -= ns.Compute.TrainingCapacity * b.TrainRentPerGPUSec * dt
	ns.Resources.Cash -= ns.Compute.InferenceCapacity * b.InferenceRentPerGPUSec * dt
	ns = advanceTraining(ns, dt, b)
	ns = advanceCompetitors(ns, dt)
	ns = advanceUsers(ns, dt, b)
	ns = advanceServing(ns, dt, b)
	return ns
}

// advanceTraining progresses the in-progress training job by dt and onlines
// the model on completion. Pure: clones Models before appending.
func advanceTraining(ns model.GameState, dt float64, b balance.Config) model.GameState {
	if !ns.HasTraining {
		return ns
	}
	ns.Training.WorkRemaining -= ns.Compute.TrainingCapacity * dt
	if ns.Training.WorkRemaining > 0 {
		return ns
	}
	// Completed → build the model and online it.
	job := ns.Training
	m := model.Model{Gen: job.Gen, Price: job.Price, Online: true}
	for d := range model.NumQualityDims {
		m.Quality[d] = job.Alloc[d] * b.GenQualityCap[job.Gen]
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
	models := append([]model.Model(nil), ns.Models...)
	for i := range models {
		m := &models[i]
		if !m.Online {
			continue
		}
		ns.Resources.Cash += m.Users * m.Price * dt / b.MonthSec

		w := b.SegmentWeights[m.Segment]
		appeal := appealOf(m.Quality, w)
		rivalAppeal := 0.0
		for _, c := range ns.Competitors {
			rivalAppeal += appealOf(c.Quality, w)
		}
		refPrice := b.SegmentRefPrice[m.Segment]
		var demandMult float64
		if m.Price > 0 {
			demandMult = math.Pow(refPrice/m.Price, b.PriceElasticity)
		}
		share := 1.0
		if appeal+rivalAppeal > 0 {
			share = appeal / (appeal + rivalAppeal)
		}
		target := appeal * b.SegmentTargetScale[m.Segment] * demandMult * share
		m.Users += (target - m.Users) * b.UserGrowthRate * dt
		if m.Users < 0 {
			m.Users = 0
		}
	}
	ns.Models = models
	return ns
}

// advanceCompetitors grows each competitor's quality along its scripted
// curve. Pure: clones Competitors.
func advanceCompetitors(ns model.GameState, dt float64) model.GameState {
	if len(ns.Competitors) == 0 {
		return ns
	}
	comps := append([]model.Competitor(nil), ns.Competitors...)
	for i := range comps {
		for d := range model.NumQualityDims {
			comps[i].Quality[d] += comps[i].GrowthPerSec[d] * dt
		}
	}
	ns.Competitors = comps
	return ns
}

// advanceServing computes inference load and, when provisioned inference
// capacity cannot meet it, churns users by the service deficit. Pure: clones
// Models. v0: zero capacity is graced (no churn) so pre-inference behavior is
// unchanged.
func advanceServing(ns model.GameState, dt float64, b balance.Config) model.GameState {
	load := 0.0
	for _, m := range ns.Models {
		if m.Online {
			load += m.Users * b.InferenceLoadPerUser
		}
	}
	ns.Compute.InferenceLoad = load
	if ns.Compute.InferenceCapacity <= 0 || load <= ns.Compute.InferenceCapacity {
		return ns
	}
	deficit := (load - ns.Compute.InferenceCapacity) / load
	models := append([]model.Model(nil), ns.Models...)
	for i := range models {
		m := &models[i]
		if !m.Online {
			continue
		}
		m.Users -= m.Users * b.ServiceChurnRate * deficit * dt
		if m.Users < 0 {
			m.Users = 0
		}
	}
	ns.Models = models
	return ns
}
