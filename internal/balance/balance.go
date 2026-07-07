// Package balance holds all tunable v0 numbers, copied verbatim from
// design spec §12. Keeping them in one place makes tuning easy.
package balance

import "tokensmith/internal/model"

// MaxGen is the highest model generation modelled in v0.
const MaxGen = 5

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

	// Per-generation model training (index by gen 1..MaxGen; 0 unused).
	GenRnDCost         [MaxGen + 1]float64 // R&D cost to start training
	GenTrainWorkGPUSec [MaxGen + 1]float64 // training work in GPU-seconds
	GenQualityCap      [MaxGen + 1]float64 // per-dimension quality ceiling

	// TrainRentPerGPUSec is cash cost per rented training GPU per second.
	// v0 placeholder (spec §12 $500/GPU·day is game-day-ambiguous); tune later.
	TrainRentPerGPUSec float64
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

	// gen:                      1        2         3          4           5
	c.GenRnDCost = [MaxGen + 1]float64{0, 20000, 150000, 1000000, 6000000, 40000000}
	c.GenTrainWorkGPUSec = [MaxGen + 1]float64{0, 1800, 7200, 28800, 108000, 432000}
	c.GenQualityCap = [MaxGen + 1]float64{0, 25, 45, 65, 82, 100}
	c.TrainRentPerGPUSec = 0.01
	return c
}
