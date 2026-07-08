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
	// Aggregate staff (plan-08).
	ResearcherHireCost     [model.NumTiers]float64
	ResearcherSalaryPerSec [model.NumTiers]float64
	EngineerHireCost       float64
	OpsHireCost            float64
	MarketingHireCost      float64
	EngineerSalaryPerSec   float64
	OpsSalaryPerSec        float64
	MarketingSalaryPerSec  float64
	EngineerInfraBonus     float64          // per engineer: compute efficiency
	OpsChurnReduction      float64          // per ops: service-churn mitigation
	MarketingBonus         float64          // per marketing: user-target boost
	TechNodes              []model.TechNode // tech-tree catalog (plan-09)
	// Valuation & milestones (plan-10).
	ValuationMilestones []float64
	RevenueMultiple     float64 // monthly revenue → valuation multiple
	UserValue           float64 // valuation per active user
	ServerAssetValue    float64 // valuation per unit of self-built compute
	// Prestige (plan-11).
	PrestigeNodes           []model.PrestigeNode
	PrestigeUnlockValuation float64
	PatentK                 float64
	// New-run baseline, shared by game.NewGame and prestige freshRun so a
	// reset reseeds the same starting researchers/compute/R&D.
	StartingCash              float64
	StartingRnD               float64
	StartingResearchersT1     int
	StartingTrainingCapacity  float64
	StartingInferenceCapacity float64
	Stars                     []model.Star // star-employee roster (plan-12)
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
	c.ResearcherHireCost = [model.NumTiers]float64{0, 5000, 15000, 40000}
	c.ResearcherSalaryPerSec = [model.NumTiers]float64{0, 0.001, 0.002, 0.005}
	c.EngineerHireCost = 8000
	c.OpsHireCost = 6000
	c.MarketingHireCost = 6000
	c.EngineerSalaryPerSec = 0.002
	c.OpsSalaryPerSec = 0.0015
	c.MarketingSalaryPerSec = 0.0015
	c.EngineerInfraBonus = 0.02
	c.OpsChurnReduction = 0.1
	c.MarketingBonus = 0.03
	c.TechNodes = DefaultTechNodes()
	c.ValuationMilestones = []float64{1e6, 1e7, 1e8, 1e9, 1e10, 1e11, 1e12}
	c.RevenueMultiple = 120
	c.UserValue = 10
	c.ServerAssetValue = 5000
	c.PrestigeUnlockValuation = 1e9
	c.PatentK = 1e8
	c.StartingCash = 100000
	c.StartingRnD = 50000
	c.StartingResearchersT1 = 2
	c.StartingTrainingCapacity = 4
	c.StartingInferenceCapacity = 2
	c.PrestigeNodes = DefaultPrestigeNodes()
	c.Stars = DefaultStars()
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

// techNode builds a node starting from neutral effects, applying set().
func techNode(id string, br model.TechBranch, cost float64, prereqs []string, set func(e *model.TechEffects)) model.TechNode {
	e := model.NeutralTechEffects()
	set(&e)
	return model.TechNode{ID: id, Branch: br, Cost: cost, Prereqs: prereqs, Effects: e}
}

// DefaultTechNodes returns the v0 tech-tree catalog (representative; spec §17.3).
func DefaultTechNodes() []model.TechNode {
	return []model.TechNode{
		techNode("algo-cap-1", model.BranchAlgo, 15000, nil, func(e *model.TechEffects) {
			e.QualityMult[model.DimCapability] = 1.15
		}),
		techNode("algo-train-1", model.BranchAlgo, 80000, nil, func(e *model.TechEffects) {
			e.TrainRnDMult = 0.85
			e.TrainWorkMult = 0.9
		}),
		techNode("infra-eff-1", model.BranchInfra, 8000, nil, func(e *model.TechEffects) {
			e.InfraMult = 1.1
		}),
		techNode("infra-density-1", model.BranchInfra, 120000, []string{"infra-eff-1"}, func(e *model.TechEffects) {
			e.InfraMult = 1.15
		}),
		techNode("biz-growth-1", model.BranchBusiness, 6000, nil, func(e *model.TechEffects) {
			e.UserGrowthMult = 1.15
		}),
		techNode("biz-price-1", model.BranchBusiness, 15000, nil, func(e *model.TechEffects) {
			e.RefPriceMult = 1.1
		}),
		techNode("align-safety-1", model.BranchAlignment, 8000, nil, func(e *model.TechEffects) {
			e.QualityMult[model.DimSafety] = 1.15
		}),
		techNode("align-incident-1", model.BranchAlignment, 300000, []string{"align-safety-1"}, func(e *model.TechEffects) {
			e.IncidentMult = 0.5
		}),
	}
}

// DefaultPrestigeNodes returns the v0 permanent-upgrade catalog (spec §17.4).
func DefaultPrestigeNodes() []model.PrestigeNode {
	e := model.NeutralPrestigeEffects
	startCash := e()
	startCash.StartCash = 100000
	startRnD := e()
	startRnD.StartRnD = 50000
	rndMult := e()
	rndMult.RnDMult = 1.1
	cashMult := e()
	cashMult.CashMult = 1.1
	return []model.PrestigeNode{
		{ID: "start-cash-1", Cost: 1, Effects: startCash},
		{ID: "start-rnd-1", Cost: 1, Effects: startRnD},
		{ID: "rnd-mult-1", Cost: 2, Effects: rndMult},
		{ID: "cash-mult-1", Cost: 2, Effects: cashMult},
	}
}

// star builds a Star starting from neutral effects, applying set().
func star(id, name string, signing, salaryPerSec float64, set func(e *model.StarEffects)) model.Star {
	e := model.NeutralStarEffects()
	set(&e)
	return model.Star{ID: id, Name: name, SigningCost: signing, SalaryPerSec: salaryPerSec, Effects: e}
}

// DefaultStars returns the v0 star roster (spec §17.5, numeric bonuses).
func DefaultStars() []model.Star {
	return []model.Star{
		star("aria-chen", "Dr. Aria Chen", 600000, 0.02, func(e *model.StarEffects) {
			e.QualityMult[model.DimCapability] = 1.22
			e.RnDPerSec = 300
		}),
		star("nova", "Nova", 1000000, 0.03, func(e *model.StarEffects) {
			for d := range e.QualityMult {
				e.QualityMult[d] = 1.10
			}
			e.RnDPerSec = 400
		}),
		star("sofia-reyes", "Dr. Sofia Reyes", 450000, 0.018, func(e *model.StarEffects) {
			e.QualityMult[model.DimSafety] = 1.25
		}),
		star("wei-zhang", "Dr. Wei Zhang", 380000, 0.016, func(e *model.StarEffects) {
			e.QualityMult[model.DimEfficiency] = 1.25
		}),
		star("kenji-tanaka", "Kenji Tanaka", 420000, 0.017, func(e *model.StarEffects) {
			e.InfraMult = 1.12
		}),
		star("elena-volkov", "Elena Volkov", 420000, 0.017, func(e *model.StarEffects) {
			e.InfraMult = 1.10
		}),
		star("marcus-cole", "Marcus Cole", 350000, 0.015, func(e *model.StarEffects) {
			e.UserGrowthMult = 1.30
		}),
		star("james-okafor", "James Okafor", 400000, 0.017, func(e *model.StarEffects) {
			e.UserGrowthMult = 1.25
		}),
	}
}
