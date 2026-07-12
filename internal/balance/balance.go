// Package balance holds all tunable v0 numbers, copied verbatim from
// design spec §12. Keeping them in one place makes tuning easy.
package balance

import (
	"strconv"

	"tokensmith/internal/model"
)

// EntryProcessID is the process available from the first day (no tech unlock).
const EntryProcessID = "N7"

// RealSecCompression is how many simulated seconds the TUI advances per real
// second: tickDT(3600) × 4 ticks/sec at a 250ms tick interval
// (internal/tui/tui.go). Balance numbers meant to represent "per real second"
// production (researcher and star R&D rates) divide by this so they aren't
// silently inflated by the sim-time compression. internal/tui has a test
// (TestRealSecCompressionMatchesTickRate) asserting this constant tracks
// tui's own tickDT/tickInterval derivation.
const RealSecCompression = 14400.0

// GenUnlockNodeID is the tech node that unlocks training a given model
// generation (gen >= 2); gen 1 needs no unlock.
func GenUnlockNodeID(gen int) string {
	return "model-gen-" + strconv.Itoa(gen)
}

// Config is the full set of balance knobs (plan-01 subset).
type Config struct {
	// ResearcherRnDPerSec is R&D produced per second per researcher, by tier.
	ResearcherRnDPerSec [model.NumTiers]float64

	// Token → R&D: (input*InputWeight + output*OutputWeight) / Divisor.
	TokenInputWeight  float64
	TokenOutputWeight float64
	TokenDivisor      float64

	// StreakMult multiplies token-sourced R&D only (never staff/star R&D). Set
	// per tick by the TUI from the real-world coding-streak bonus; Default()
	// seeds it to 1.0 (neutral) so every caller that never touches it keeps
	// today's behavior unchanged.
	StreakMult float64

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
	// Self-build compute (plan-07; plan-13 repoints self-build onto the
	// Processes catalog below — one process-chip built per BuildServer call).
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
	EngineerInfraBonus     float64 // per engineer: compute efficiency
	OpsChurnReduction      float64 // per ops: service-churn mitigation
	MarketingBonus         float64 // per marketing: user-target boost
	CompetitorBaseQuality  float64 // quality floor rivals track before the player has a model
	CompetitorCatchupRate  float64 // per-second rubber-band rate toward Skill×frontier
	// CompetitorMaxLead caps how far a rival may target above the player's
	// frontier once the player has a model (e.g. 1.08 = +8%). Prevents
	// Skill>1 names (OpenAI 1.2×) from reading as "already Gen2" during the
	// Gen1 R&D grind. Unused while the player has no online model.
	CompetitorMaxLead float64
	TechNodes         []model.TechNode // tech-tree catalog (plan-09)
	// Valuation & milestones (plan-10).
	ValuationMilestones []float64
	RevenueMultiple     float64 // monthly revenue → valuation multiple
	UserValue           float64 // valuation per active user
	ServerAssetValue    float64 // valuation per unit of self-built compute
	// Prestige (plan-11).
	PrestigeNodes           []model.PrestigeNode
	PrestigeUnlockValuation float64
	PatentK                 float64
	// Compute-process catalog & economy scalars (plan-13).
	Processes     []Process
	TrainRentMult float64 // training rent multiplier applied over inference rent
	RevenueMult   float64 // global revenue multiplier
	// New-run baseline, shared by game.NewGame and prestige freshRun so a
	// reset reseeds the same starting researchers/R&D. Compute starts empty
	// (nil maps) — the player rents on demand, no seeded capacity.
	StartingCash          float64
	StartingRnD           float64
	StartingResearchersT1 int
	// BankruptcyDebtRatio: the run auto-restarts once cash falls below
	// -(BankruptcyDebtRatio * StartingCash).
	BankruptcyDebtRatio float64
	Stars               []model.Star // star-employee roster (plan-12)
	// Industry events (industry-events plan).
	Events           []EventSpec
	EventCheckSec    float64 // mean game-seconds between trigger rolls
	EventHitChance   float64 // probability a roll fires an event
	EventCooldownSec float64 // per-event quiet window after it resolves
	EventLogCap      int     // history entries kept in EventsState.Log
	// Strategic campaign (phase A).
	Campaign CampaignConfig
}

// Default returns the v0 calibration (spec §12).
func Default() Config {
	var c Config
	// R&D per researcher-second. Kept low so the tech tree is a real time-gate
	// (not trivially affordable) and real coding (token R&D) stays impactful.
	c.ResearcherRnDPerSec[model.Tier1] = 0.005 / RealSecCompression
	c.ResearcherRnDPerSec[model.Tier2] = 0.015 / RealSecCompression
	c.ResearcherRnDPerSec[model.Tier3] = 0.04 / RealSecCompression

	c.TokenInputWeight = 1
	c.TokenOutputWeight = 2
	c.TokenDivisor = 1
	c.StreakMult = 1.0

	// Per-generation train costs/work/quality live in Generation() (generation.go).
	c.TrainRentPerGPUSec = 0.01

	c.QualityWeights = [model.NumQualityDims]float64{0.4, 0.2, 0.2, 0.2}
	c.UserTargetPerAppeal = 20000
	c.UserGrowthRate = 3.5e-5
	c.RefPrice = 12
	c.PriceElasticity = 1.5
	c.MonthSec = 2592000
	c.SegmentWeights[model.SegConsumer] = qvec(0.4, 0.2, 0.2, 0.2)    // == QualityWeights
	c.SegmentWeights[model.SegEnterprise] = qvec(0.2, 0.1, 0.5, 0.2)  // values safety
	c.SegmentWeights[model.SegDeveloper] = qvec(0.15, 0.4, 0.1, 0.35) // values efficiency+speed
	c.SegmentTargetScale = [model.NumSegments]float64{20000, 10000, 16000}
	c.SegmentRefPrice = [model.NumSegments]float64{12, 180, 6}
	c.InferenceRentPerGPUSec = 0.006
	c.InferenceLoadPerUser = 0.0001
	c.ServiceChurnRate = 0.01
	// Self-build economics (plan-13 tuning): electricity per chip is kept below
	// its rent so buying capex pays back over time; chassis/slot per single-chip
	// server are lowered (old 8-chip bundling amortised them) so self-build is a
	// real alternative to renting rather than strictly dominated.
	c.ChassisCost = 1000
	c.ElectricityPerKWSec = 0.0002
	c.PowerCostPerKW = 400
	c.SlotCost = 4000
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
	c.CompetitorBaseQuality = 8
	// ~0.69% of remaining gap per real day. Old 5e-7 (~4.3%/day) let top rivals
	// climb Gen1→~Gen1-cap within ~2 weeks — faster than the player can farm
	// model-gen-2 R&D. Half-life of the gap is now ~3 months.
	c.CompetitorCatchupRate = 0.00000008
	c.CompetitorMaxLead = 1.08
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
	c.BankruptcyDebtRatio = 1.0 // game over at cash < -100000 (1× starting cash)
	c.PrestigeNodes = DefaultPrestigeNodes()
	c.Stars = DefaultStars()
	c.Processes = DefaultProcesses()
	c.TrainRentMult = 1.667
	c.RevenueMult = 2
	c.Events = DefaultEvents()
	c.EventCheckSec = 5 * 86400 // 5 game-days ≈ 30 real-sec online
	c.EventHitChance = 0.35     // → mean one event per ~85 real-sec online
	c.EventCooldownSec = 60 * 86400
	c.EventLogCap = 20
	c.Campaign = DefaultCampaign()
	return c
}

// qvec builds a per-dimension vector in dim order: capability, efficiency,
// safety, speed.
func qvec(capability, efficiency, safety, speed float64) [model.NumQualityDims]float64 {
	return [model.NumQualityDims]float64{capability, efficiency, safety, speed}
}

// DefaultCompetitors returns the named-competitor roster. Skill is a bounded
// specialty vector in [0.92, 1.08] (long-run progression design §9.1); initial
// Quality is near Skill×CompetitorBaseQuality so rivals start beatable.
func DefaultCompetitors() []model.Competitor {
	// Specialties preserve relative character (OpenAI cap, Anthropic safety,
	// DeepSeek efficiency, …) while staying inside the 0.92–1.08 band.
	const base = 8.0
	mk := func(name string, skill [model.NumQualityDims]float64) model.Competitor {
		var q [model.NumQualityDims]float64
		for d := range q {
			q[d] = skill[d] * base
		}
		return model.Competitor{Name: name, Quality: q, Skill: skill}
	}
	return []model.Competitor{
		mk("OpenAI", qvec(1.08, 1.00, 0.96, 1.04)),
		mk("Anthropic", qvec(1.04, 0.96, 1.08, 0.96)),
		mk("xAI", qvec(1.04, 0.96, 0.92, 1.08)),
		mk("DeepSeek", qvec(0.96, 1.08, 0.92, 1.00)),
		mk("Qwen", qvec(0.94, 1.06, 0.98, 1.00)),
		mk("Zhipu", qvec(0.92, 1.00, 0.98, 0.92)),
		mk("Gemini", qvec(1.04, 1.00, 1.04, 1.00)),
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
		// Model-generation unlocks — chained gates (no direct effect) so higher
		// generations must be earned via R&D rather than picked from the start.
		techNode(GenUnlockNodeID(2), model.BranchAlgo, 200000, nil, func(e *model.TechEffects) {}),
		techNode(GenUnlockNodeID(3), model.BranchAlgo, 1500000, []string{GenUnlockNodeID(2)}, func(e *model.TechEffects) {}),
		techNode(GenUnlockNodeID(4), model.BranchAlgo, 10000000, []string{GenUnlockNodeID(3)}, func(e *model.TechEffects) {}),
		techNode(GenUnlockNodeID(5), model.BranchAlgo, 60000000, []string{GenUnlockNodeID(4)}, func(e *model.TechEffects) {}),
		// Compute-process unlocks — chained gates (no direct effect) so smaller
		// process nodes must be earned via R&D rather than picked from the start.
		techNode("process-N5", model.BranchInfra, 150000, nil, func(e *model.TechEffects) {}),
		techNode("process-N3", model.BranchInfra, 1500000, []string{"process-N5"}, func(e *model.TechEffects) {}),
		techNode("process-N2", model.BranchInfra, 10000000, []string{"process-N3"}, func(e *model.TechEffects) {}),
	}
}

// Process is a compute node: rentable (opex) or buildable (capex).
type Process struct {
	ID         string
	Name       string
	Compute    float64 // compute per chip (old GPU scale = 1)
	PowerKW    float64
	RentPerSec float64 // inference rent per chip/sec; training = ×TrainRentMult
	BuyPrice   float64
	UnlockTech string // "" = from start
}

// DefaultProcesses returns the v0 compute-process catalog (spec §17.6).
func DefaultProcesses() []Process {
	return []Process{
		{"N7", "N7 入門", 1, 2.0, 0.001, 6000, ""},
		{"N5", "N5", 2, 3.0, 0.0018, 15000, "process-N5"},
		{"N3", "N3", 4, 5.0, 0.003, 40000, "process-N3"},
		{"N2", "N2", 8, 8.0, 0.005, 100000, "process-N2"},
	}
}

// ProcessByID looks up a process by ID within ps.
func ProcessByID(ps []Process, id string) (Process, bool) {
	for _, p := range ps {
		if p.ID == id {
			return p, true
		}
	}
	return Process{}, false
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
			e.RnDPerSec = 300 / RealSecCompression
		}),
		star("nova", "Nova", 1000000, 0.03, func(e *model.StarEffects) {
			for d := range e.QualityMult {
				e.QualityMult[d] = 1.10
			}
			e.RnDPerSec = 400 / RealSecCompression
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
