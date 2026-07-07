// Package balance holds all tunable v0 numbers, copied verbatim from
// design spec §12. Keeping them in one place makes tuning easy.
package balance

import "tokensmith/internal/model"

// Config is the full set of balance knobs (plan-01 subset).
type Config struct {
	// ResearcherRnDPerSec is R&D produced per second per researcher, by tier.
	ResearcherRnDPerSec [model.NumTiers]float64

	// Token → R&D: (input*InputWeight + output*OutputWeight) / Divisor.
	TokenInputWeight  float64
	TokenOutputWeight float64
	TokenDivisor      float64

	// Daily soft cap on token-sourced R&D within a rolling window.
	SoftCapFull      float64 // R&D granted at full rate before diminishing
	SoftCapMult      float64 // multiplier applied beyond SoftCapFull
	SoftCapWindowSec float64 // window length in seconds
}

// Default returns the v0 calibration (spec §12).
func Default() Config {
	var c Config
	c.ResearcherRnDPerSec[model.Tier1] = 5
	c.ResearcherRnDPerSec[model.Tier2] = 15
	c.ResearcherRnDPerSec[model.Tier3] = 40

	c.TokenInputWeight = 1
	c.TokenOutputWeight = 2
	c.TokenDivisor = 10

	c.SoftCapFull = 200000
	c.SoftCapMult = 0.3
	c.SoftCapWindowSec = 86400
	return c
}
