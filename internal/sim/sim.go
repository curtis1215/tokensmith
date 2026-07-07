// Package sim is the pure, deterministic simulation core.
// No wall-clock, no randomness, no I/O — time advances only via dt.
package sim

import (
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
	return ns
}
