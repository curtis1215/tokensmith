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

	// User attraction & subscription revenue (plan-03).
	QualityWeights      [model.NumQualityDims]float64 // aggregate appeal weights
	UserTargetPerAppeal float64                       // target users per unit appeal
	UserGrowthRate      float64                       // per-second approach to target
	RefPrice            float64                       // reference price for elasticity
	PriceElasticity     float64                       // demand elasticity exponent
	MonthSec            float64                       // seconds per month (price is per-user-per-month)
	// Market segments (plan-05). Index 0 (consumer) mirrors the legacy scalars.
	SegmentWeights     [model.NumSegments][model.NumQualityDims]float64
	SegmentTargetScale [model.NumSegments]float64
	SegmentRefPrice    [model.NumSegments]float64
	// Inference serving (plan-06).
	InferenceRentPerGPUSec float64 // cash per rented inference GPU per second
	InferenceLoadPerUser   float64 // inference GPU load per active user
	ServiceChurnRate       float64 // extra churn per second at full deficit
	// Self-build compute (plan-07).
	Chips               []model.Chip
	ChipsPerServer      int
	ChassisCost         float64
	ElectricityPerKWSec float64 // cash per kW per second
	PowerCostPerKW      float64 // datacenter power-capacity expansion cost per kW
	SlotCost            float64 // datacenter rack-slot expansion cost per slot
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

	c.QualityWeights = [model.NumQualityDims]float64{0.4, 0.2, 0.2, 0.2}
	c.UserTargetPerAppeal = 1000
	c.UserGrowthRate = 0.001
	c.RefPrice = 12
	c.PriceElasticity = 1.5
	c.MonthSec = 2592000
	c.SegmentWeights[model.SegConsumer] = qvec(0.4, 0.2, 0.2, 0.2)    // == QualityWeights
	c.SegmentWeights[model.SegEnterprise] = qvec(0.2, 0.1, 0.5, 0.2)  // values safety
	c.SegmentWeights[model.SegDeveloper] = qvec(0.15, 0.4, 0.1, 0.35) // values efficiency+speed
	c.SegmentTargetScale = [model.NumSegments]float64{1000, 500, 800}
	c.SegmentRefPrice = [model.NumSegments]float64{12, 180, 6}
	c.InferenceRentPerGPUSec = 0.006
	c.InferenceLoadPerUser = 0.0001
	c.ServiceChurnRate = 0.01
	c.Chips = []model.Chip{
		{Name: "H-class G3", Pool: model.PoolInference, Compute: 2, PowerKW: 3, Price: 8000},
		{Name: "T-class G4", Pool: model.PoolTraining, Compute: 3, PowerKW: 5, Price: 18000},
	}
	c.ChipsPerServer = 8
	c.ChassisCost = 5000
	c.ElectricityPerKWSec = 0.001
	c.PowerCostPerKW = 400
	c.SlotCost = 30000
	return c
}

// qvec builds a per-dimension vector in dim order: capability, efficiency,
// safety, speed.
func qvec(capability, efficiency, safety, speed float64) [model.NumQualityDims]float64 {
	return [model.NumQualityDims]float64{capability, efficiency, safety, speed}
}

// DefaultCompetitors returns the v0 named-competitor roster (spec §17.1).
// GrowthPerSec is tunable v0; specialty dimensions grow fastest.
func DefaultCompetitors() []model.Competitor {
	return []model.Competitor{
		{Name: "OpenAI", Quality: qvec(55, 35, 35, 45), GrowthPerSec: qvec(0.0001, 0.00003, 0.00003, 0.00005)},
		{Name: "Anthropic", Quality: qvec(52, 30, 55, 40), GrowthPerSec: qvec(0.00007, 0.00003, 0.0001, 0.00004)},
		{Name: "xAI", Quality: qvec(45, 30, 20, 50), GrowthPerSec: qvec(0.0001, 0.00003, 0.00002, 0.00008)},
		{Name: "DeepSeek", Quality: qvec(42, 60, 25, 45), GrowthPerSec: qvec(0.00005, 0.0001, 0.00003, 0.00005)},
		{Name: "Qwen", Quality: qvec(40, 50, 30, 45), GrowthPerSec: qvec(0.00005, 0.00007, 0.00004, 0.00005)},
		{Name: "Zhipu", Quality: qvec(40, 45, 35, 38), GrowthPerSec: qvec(0.00004, 0.00005, 0.00004, 0.00003)},
		{Name: "Gemini", Quality: qvec(48, 40, 42, 45), GrowthPerSec: qvec(0.00006, 0.00005, 0.00006, 0.00005)},
	}
}
