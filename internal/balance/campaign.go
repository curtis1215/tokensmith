package balance

import "tokensmith/internal/model"

// DoctrinePerkSpec is a doctrine-route perk entry in the campaign catalog.
type DoctrinePerkSpec struct {
	ID       string
	Doctrine model.Doctrine
	Tier     int
	Effects  model.CampaignEffects
}

// RivalActionSpec is a scheduled rival action entry.
// FrontierProgress is the fraction of remaining distance to the rival's
// bounded global-frontier target closed on execution (per dimension).
// MomentumCycles ages MomentumPct linearly on subsequent board cycles.
type RivalActionSpec struct {
	ID               string
	Segment          model.Segment
	LeadCycles       int
	FrontierProgress [model.NumQualityDims]float64
	MomentumCycles   int
	RefPriceMult     float64
	DurationCycles   int
}

// RivalProfile maps a rival company to doctrine primary roles and action IDs.
type RivalProfile struct {
	Name       string
	PrimaryFor []model.Doctrine
	Actions    []string
}

// CampaignConfig holds Phase A strategic-campaign balance knobs.
type CampaignConfig struct {
	CycleSec              int64
	MaxCatchupCycles      int
	ReportCap             int
	PivotCashFloor        float64
	PivotRevenueMonths    float64
	PivotRnDFrac          float64
	EstablishShare        float64
	ConsumerExpandShare   float64
	EnterpriseExpandShare float64
	DeveloperExpandShare  float64
	ConsumerWinShare      float64
	EnterpriseWinShare    float64
	DeveloperWinShare     float64
	StrategyExitCycle     int
	Perks                 []DoctrinePerkSpec
	RivalActions          []RivalActionSpec
	Rivals                []RivalProfile
}

// HardBandShareCeiling is the theoretical max playerSegmentShare when the
// player defines GlobalFrontier and all N default rivals are hard-clamped to
// the 85% floor: 1/(1+N×0.85). With N=7 this is ≈0.1439. Campaign share gates
// must sit strictly below this ceiling or expand/win is unreachable under the
// hard band invariant (raw share still sums all competitors).
const HardBandFloorPct = 0.85

// HardBandPlayerShareCeiling returns 1/(1+nRivals×HardBandFloorPct).
func HardBandPlayerShareCeiling(nRivals int) float64 {
	if nRivals < 0 {
		nRivals = 0
	}
	return 1 / (1 + float64(nRivals)*HardBandFloorPct)
}

// DefaultCampaign returns the Phase A campaign catalog with share gates
// recalibrated under the hard rival band (85%–115% of GlobalFrontier).
//
// Ordering (establish < expand < win) and relative doctrine difficulty are
// preserved: consumer expand/win are strictest; enterprise win is easiest.
// Absolute levels sit under HardBandPlayerShareCeiling(7)≈0.1439 so a full
// default roster remains winnable when the player leads the frontier.
func DefaultCampaign() CampaignConfig {
	return CampaignConfig{
		CycleSec:              28800,
		MaxCatchupCycles:      3,
		ReportCap:             20,
		PivotCashFloor:        20000,
		PivotRevenueMonths:    1,
		PivotRnDFrac:          0.10,
		EstablishShare:        0.07,
		ConsumerExpandShare:   0.11,
		EnterpriseExpandShare: 0.095,
		DeveloperExpandShare:  0.095,
		ConsumerWinShare:      0.13,
		EnterpriseWinShare:    0.12,
		DeveloperWinShare:     0.13,
		StrategyExitCycle:     18,
		Perks:                 defaultCampaignPerks(),
		RivalActions:          defaultRivalActions(),
		Rivals:                defaultRivalProfiles(),
	}
}

func campaignPerk(id string, doctrine model.Doctrine, tier int, set func(e *model.CampaignEffects)) DoctrinePerkSpec {
	e := model.NeutralCampaignEffects()
	set(&e)
	return DoctrinePerkSpec{ID: id, Doctrine: doctrine, Tier: tier, Effects: e}
}

func defaultCampaignPerks() []DoctrinePerkSpec {
	return []DoctrinePerkSpec{
		campaignPerk("consumer-premium", model.DoctrineConsumer, 1, func(e *model.CampaignEffects) {
			e.RefPriceMult[model.SegConsumer] = 1.15
			e.UserGrowthMult[model.SegConsumer] = 0.90
		}),
		campaignPerk("consumer-mass", model.DoctrineConsumer, 1, func(e *model.CampaignEffects) {
			e.RefPriceMult[model.SegConsumer] = 0.95
			e.UserGrowthMult[model.SegConsumer] = 1.20
		}),
		campaignPerk("consumer-resilience", model.DoctrineConsumer, 2, func(e *model.CampaignEffects) {
			e.RivalImpactMult = 0.75
		}),
		campaignPerk("consumer-scale", model.DoctrineConsumer, 2, func(e *model.CampaignEffects) {
			e.UserGrowthMult[model.SegConsumer] = 1.20
			e.InferenceLoadMult = 1.10
		}),
		campaignPerk("enterprise-compliance", model.DoctrineEnterprise, 1, func(e *model.CampaignEffects) {
			e.SafetyAppealMult = 1.15
			e.RevenueMult[model.SegEnterprise] = 0.95
		}),
		campaignPerk("enterprise-premium", model.DoctrineEnterprise, 1, func(e *model.CampaignEffects) {
			e.RefPriceMult[model.SegEnterprise] = 1.15
			e.UserGrowthMult[model.SegEnterprise] = 0.90
		}),
		campaignPerk("enterprise-reliability", model.DoctrineEnterprise, 2, func(e *model.CampaignEffects) {
			e.ServiceChurnMult = 0.75
		}),
		campaignPerk("enterprise-sales", model.DoctrineEnterprise, 2, func(e *model.CampaignEffects) {
			e.UserGrowthMult[model.SegEnterprise] = 1.20
			e.InferenceLoadMult = 1.10
		}),
		campaignPerk("developer-open", model.DoctrineDeveloper, 1, func(e *model.CampaignEffects) {
			e.RefPriceMult[model.SegDeveloper] = 0.90
			e.UserGrowthMult[model.SegDeveloper] = 1.25
		}),
		campaignPerk("developer-api", model.DoctrineDeveloper, 1, func(e *model.CampaignEffects) {
			e.RefPriceMult[model.SegDeveloper] = 1.10
			e.UserGrowthMult[model.SegDeveloper] = 0.95
		}),
		campaignPerk("developer-efficient", model.DoctrineDeveloper, 2, func(e *model.CampaignEffects) {
			e.InferenceLoadMult = 0.85
			e.RevenueMult[model.SegDeveloper] = 0.95
		}),
		campaignPerk("developer-usage", model.DoctrineDeveloper, 2, func(e *model.CampaignEffects) {
			e.InferenceLoadMult = 1.15
			e.RevenueMult[model.SegDeveloper] = 1.20
		}),
	}
}

func rivalAction(id string, seg model.Segment, lead int, progress [model.NumQualityDims]float64, momentumCycles int, refPriceMult float64, duration int) RivalActionSpec {
	return RivalActionSpec{
		ID:               id,
		Segment:          seg,
		LeadCycles:       lead,
		FrontierProgress: progress,
		MomentumCycles:   momentumCycles,
		RefPriceMult:     refPriceMult,
		DurationCycles:   duration,
	}
}

func defaultRivalActions() []RivalActionSpec {
	// FrontierProgress is the fraction of remaining target distance closed
	// (e.g. 0.15 closes 15% of the gap). Momentum lasts a few board cycles.
	const mom = 3
	return []RivalActionSpec{
		rivalAction("openai-flagship", model.SegConsumer, 2, qvec(0.15, 0, 0, 0), mom, 1, 0),
		rivalAction("openai-platform", model.SegConsumer, 3, qvec(0.08, 0, 0, 0.08), mom, 1, 0),
		rivalAction("anthropic-trust", model.SegEnterprise, 2, qvec(0.08, 0, 0.15, 0), mom, 1, 0),
		rivalAction("anthropic-enterprise-suite", model.SegEnterprise, 3, qvec(0, 0.08, 0.10, 0), mom, 1, 0),
		rivalAction("xai-scale", model.SegConsumer, 2, qvec(0.12, 0, 0, 0.15), mom, 1, 0),
		rivalAction("xai-compute-rush", model.SegConsumer, 3, qvec(0.10, 0, 0, 0.12), mom, 1, 0),
		rivalAction("deepseek-price-war", model.SegDeveloper, 2, qvec(0, 0.15, 0, 0.10), mom, 0.85, 2),
		rivalAction("deepseek-distill", model.SegDeveloper, 3, qvec(0, 0.12, 0, 0), mom, 1, 0),
		rivalAction("qwen-ecosystem", model.SegDeveloper, 2, qvec(0, 0.10, 0, 0.10), mom, 1, 0),
		rivalAction("qwen-release-wave", model.SegDeveloper, 3, qvec(0.05, 0.08, 0, 0.08), mom, 1, 0),
		rivalAction("zhipu-enterprise", model.SegEnterprise, 3, qvec(0, 0.12, 0.12, 0), mom, 1, 0),
		rivalAction("zhipu-contract", model.SegEnterprise, 3, qvec(0, 0, 0.10, 0.06), mom, 1, 0),
		rivalAction("gemini-balanced", model.SegConsumer, 3, qvec(0.08, 0.08, 0.08, 0.08), mom, 1, 0),
		rivalAction("gemini-multimodal", model.SegConsumer, 3, qvec(0.10, 0, 0.06, 0.08), mom, 1, 0),
	}
}

func defaultRivalProfiles() []RivalProfile {
	return []RivalProfile{
		{Name: "OpenAI", PrimaryFor: []model.Doctrine{model.DoctrineConsumer}, Actions: []string{"openai-flagship", "openai-platform"}},
		{Name: "Anthropic", PrimaryFor: []model.Doctrine{model.DoctrineEnterprise}, Actions: []string{"anthropic-trust", "anthropic-enterprise-suite"}},
		{Name: "xAI", PrimaryFor: []model.Doctrine{model.DoctrineConsumer}, Actions: []string{"xai-scale", "xai-compute-rush"}},
		{Name: "DeepSeek", PrimaryFor: []model.Doctrine{model.DoctrineDeveloper}, Actions: []string{"deepseek-price-war", "deepseek-distill"}},
		{Name: "Qwen", PrimaryFor: []model.Doctrine{model.DoctrineDeveloper}, Actions: []string{"qwen-ecosystem", "qwen-release-wave"}},
		{Name: "Zhipu", PrimaryFor: []model.Doctrine{model.DoctrineEnterprise}, Actions: []string{"zhipu-enterprise", "zhipu-contract"}},
		{Name: "Gemini", PrimaryFor: []model.Doctrine{model.DoctrineConsumer, model.DoctrineEnterprise}, Actions: []string{"gemini-balanced", "gemini-multimodal"}},
	}
}

// CampaignPerkByID looks up a perk by ID within c.
func CampaignPerkByID(c CampaignConfig, id string) (DoctrinePerkSpec, bool) {
	for _, p := range c.Perks {
		if p.ID == id {
			return p, true
		}
	}
	return DoctrinePerkSpec{}, false
}

// PerksFor returns perks matching doctrine and tier (deterministic slice order).
func PerksFor(c CampaignConfig, doctrine model.Doctrine, tier int) []DoctrinePerkSpec {
	var out []DoctrinePerkSpec
	for _, p := range c.Perks {
		if p.Doctrine == doctrine && p.Tier == tier {
			out = append(out, p)
		}
	}
	return out
}

// RivalActionByID looks up a rival action by ID within c.
func RivalActionByID(c CampaignConfig, id string) (RivalActionSpec, bool) {
	for _, a := range c.RivalActions {
		if a.ID == id {
			return a, true
		}
	}
	return RivalActionSpec{}, false
}

// RivalProfileByName looks up a rival profile by company name within c.
func RivalProfileByName(c CampaignConfig, name string) (RivalProfile, bool) {
	for _, r := range c.Rivals {
		if r.Name == name {
			return r, true
		}
	}
	return RivalProfile{}, false
}
