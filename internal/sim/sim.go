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

// Tick advances the simulation by dt seconds and returns the new state.
// Pure: it does not mutate s and depends only on its arguments.
func Tick(s model.GameState, dt float64, events []model.TokenEvent, b balance.Config) model.GameState {
	ns := s
	ns.GameTime += dt
	staffRnD := staffRnDPerSec(s.Research, b) * dt
	tokenRnD := tokenRawRnD(events, b)
	ns.Resources.RnD += staffRnD + tokenRnD
	return ns
}
