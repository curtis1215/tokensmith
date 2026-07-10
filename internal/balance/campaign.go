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
type RivalActionSpec struct {
	ID             string
	Segment        model.Segment
	LeadCycles     int
	QualityPct     [model.NumQualityDims]float64
	RefPriceMult   float64
	DurationCycles int
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

// DefaultCampaign returns the exact Phase A campaign catalog.
func DefaultCampaign() CampaignConfig {
	return CampaignConfig{
		CycleSec:              28800,
		MaxCatchupCycles:      3,
		ReportCap:             20,
		PivotCashFloor:        20000,
		PivotRevenueMonths:    1,
		PivotRnDFrac:          0.10,
		EstablishShare:        0.10,
		ConsumerExpandShare:   0.25,
		EnterpriseExpandShare: 0.20,
		DeveloperExpandShare:  0.20,
		ConsumerWinShare:      0.35,
		EnterpriseWinShare:    0.30,
		DeveloperWinShare:     0.35,
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

func rivalAction(id string, seg model.Segment, lead int, quality [model.NumQualityDims]float64, refPriceMult float64, duration int) RivalActionSpec {
	return RivalActionSpec{
		ID:             id,
		Segment:        seg,
		LeadCycles:     lead,
		QualityPct:     quality,
		RefPriceMult:   refPriceMult,
		DurationCycles: duration,
	}
}

func defaultRivalActions() []RivalActionSpec {
	// QualityPct is fractional gain (e.g. 0.15 = +15%).
	return []RivalActionSpec{
		rivalAction("openai-flagship", model.SegConsumer, 2, qvec(0.15, 0, 0, 0), 1, 0),
		rivalAction("openai-platform", model.SegConsumer, 3, qvec(0.08, 0, 0, 0.08), 1, 0),
		rivalAction("anthropic-trust", model.SegEnterprise, 2, qvec(0.08, 0, 0.15, 0), 1, 0),
		rivalAction("anthropic-enterprise-suite", model.SegEnterprise, 3, qvec(0, 0.08, 0.10, 0), 1, 0),
		rivalAction("xai-scale", model.SegConsumer, 2, qvec(0.12, 0, 0, 0.15), 1, 0),
		rivalAction("xai-compute-rush", model.SegConsumer, 3, qvec(0.10, 0, 0, 0.12), 1, 0),
		rivalAction("deepseek-price-war", model.SegDeveloper, 2, qvec(0, 0.15, 0, 0.10), 0.85, 2),
		rivalAction("deepseek-distill", model.SegDeveloper, 3, qvec(0, 0.12, 0, 0), 1, 0),
		rivalAction("qwen-ecosystem", model.SegDeveloper, 2, qvec(0, 0.10, 0, 0.10), 1, 0),
		rivalAction("qwen-release-wave", model.SegDeveloper, 3, qvec(0.05, 0.08, 0, 0.08), 1, 0),
		rivalAction("zhipu-enterprise", model.SegEnterprise, 3, qvec(0, 0.12, 0.12, 0), 1, 0),
		rivalAction("zhipu-contract", model.SegEnterprise, 3, qvec(0, 0, 0.10, 0.06), 1, 0),
		rivalAction("gemini-balanced", model.SegConsumer, 3, qvec(0.08, 0.08, 0.08, 0.08), 1, 0),
		rivalAction("gemini-multimodal", model.SegConsumer, 3, qvec(0.10, 0, 0.06, 0.08), 1, 0),
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
