package balance

import "tokensmith/internal/model"

// SkillDef is a passive skill definition (design §5).
//
// Convention:
//   - *Mult fields: 0 = unused; when set, 1.0 is neutral, >1 buff, <1 reduction
//   - CompanyRolePower[r]: 0 = unused; 0.06 means +6% applied as (1+sum)
//   - SecondaryWeight: if >0, overrides personal secondary weight for holder
//   - ExtraSeats / MarketRarityBonus: 0 = unused
//   - Family: mutex key — skills sharing a family cannot both roll onto one employee
//   - HasPrefer + PreferRole: role-gated self effects (e.g. SelfRolePowerMult)
type SkillDef struct {
	ID         string
	NameZH     string
	Tier       model.SkillTier
	Signature  bool
	Family     string // mutex family key
	PreferRole model.Role
	HasPrefer  bool
	// Numeric hooks (v1); zero = unused
	SelfRolePowerMult float64                 // if >0 and primary matches PreferRole
	CompanyRolePower  [model.NumRoles]float64 // additive mult on company power, e.g. 0.06
	SelfSalaryMult    float64                 // e.g. 0.92 for -8%
	CompanySalaryMult float64
	HireCostMult      float64
	SeveranceMult     float64 // company-wide or self — see catalog comments
	TokenRnDMult      float64
	InfraMult         float64
	UserGrowthMult    float64
	ChurnMult         float64 // <1 reduces churn
	TrainQualityMult  float64
	RevenueMult       float64
	SecondaryWeight   float64 // if >0, overrides secondary weight for holder
	ExtraSeats        int
	MarketRarityBonus float64 // added to effective office level for weights
	RerollBaseMult    float64
	SelfStatMult      float64 // multiply all stats in power calc
	EventNegMult      float64 // <1 softens negative events
}

// SkillByID looks up a skill in the balance catalog.
func SkillByID(b Config, id string) (SkillDef, bool) {
	for _, sk := range b.Skills {
		if sk.ID == id {
			return sk, true
		}
	}
	return SkillDef{}, false
}

// SkillsByTier returns catalog entries for the given tier (includes signatures when tier is God).
func SkillsByTier(b Config, tier model.SkillTier) []SkillDef {
	out := make([]SkillDef, 0)
	for _, sk := range b.Skills {
		if sk.Tier == tier {
			out = append(out, sk)
		}
	}
	return out
}

// DefaultSkills returns the full passive catalog: Manager 18 + Director 18 +
// God 12 + Signature 9 = 57 (design §5.1–§5.4).
func DefaultSkills() []SkillDef {
	crp := func(r model.Role, v float64) [model.NumRoles]float64 {
		var a [model.NumRoles]float64
		a[r] = v
		return a
	}
	crpAll := func(v float64) [model.NumRoles]float64 {
		var a [model.NumRoles]float64
		for i := range a {
			a[i] = v
		}
		return a
	}
	crp2 := func(r1 model.Role, v1 float64, r2 model.Role, v2 float64) [model.NumRoles]float64 {
		var a [model.NumRoles]float64
		a[r1] = v1
		a[r2] = v2
		return a
	}

	// --- Manager (18) — personal / small company hooks ---
	mgr := []SkillDef{
		{
			ID: "m-deep-research", NameZH: "深潛研究", Tier: model.SkillTierManager,
			Family: "self_power_res", PreferRole: model.RoleResearcher, HasPrefer: true,
			SelfRolePowerMult: 1.12,
		},
		{
			ID: "m-sre-craft", NameZH: "穩健工程", Tier: model.SkillTierManager,
			Family: "self_power_eng", PreferRole: model.RoleEngineer, HasPrefer: true,
			SelfRolePowerMult: 1.12,
		},
		{
			ID: "m-ops-playbook", NameZH: "營運手冊", Tier: model.SkillTierManager,
			Family: "self_power_ops", PreferRole: model.RoleOps, HasPrefer: true,
			SelfRolePowerMult: 1.12,
		},
		{
			ID: "m-growth-hacks", NameZH: "成長黑客", Tier: model.SkillTierManager,
			Family: "self_power_mkt", PreferRole: model.RoleMarketing, HasPrefer: true,
			SelfRolePowerMult: 1.12,
		},
		{
			// Self monthly salary -8%.
			ID: "m-thrifty", NameZH: "精算師", Tier: model.SkillTierManager,
			Family: "salary_self", SelfSalaryMult: 0.92,
		},
		{
			// Same-primary lower-rank power +3% (sim gates by rank; here company role hook).
			ID: "m-mentor", NameZH: "帶人", Tier: model.SkillTierManager,
			Family: "mentor", PreferRole: model.RoleResearcher, HasPrefer: true,
			CompanyRolePower: crp(model.RoleResearcher, 0.03),
		},
		{
			ID: "m-night-owl", NameZH: "夜貓子", Tier: model.SkillTierManager,
			Family: "self_stat_night", SelfStatMult: 1.05,
		},
		{
			// Prefer research: small R&D power bump (flat approximated as mult).
			ID: "m-doc-driven", NameZH: "文件狂", Tier: model.SkillTierManager,
			Family: "doc_rnd", PreferRole: model.RoleResearcher, HasPrefer: true,
			SelfRolePowerMult: 1.05,
		},
		{
			ID: "m-perf-budget", NameZH: "效能預算", Tier: model.SkillTierManager,
			Family: "infra_mgr", PreferRole: model.RoleEngineer, HasPrefer: true,
			InfraMult: 1.03,
		},
		{
			ID: "m-oncall", NameZH: "值班魂", Tier: model.SkillTierManager,
			Family: "churn_mgr", PreferRole: model.RoleOps, HasPrefer: true,
			ChurnMult: 0.97,
		},
		{
			ID: "m-copy-chief", NameZH: "文案手", Tier: model.SkillTierManager,
			Family: "growth_mgr", PreferRole: model.RoleMarketing, HasPrefer: true,
			UserGrowthMult: 1.02,
		},
		{
			// Secondary weight 0.35 → 0.42 for holder.
			ID: "m-cross-train", NameZH: "跨訓", Tier: model.SkillTierManager,
			Family: "secondary_weight", SecondaryWeight: 0.42,
		},
		{
			// Self severance -25%.
			ID: "m-loyal", NameZH: "死忠", Tier: model.SkillTierManager,
			Family: "severance_self", SeveranceMult: 0.75,
		},
		{
			// Self power +10% while training (sim may gate; base mult here).
			ID: "m-sprinter", NameZH: "衝刺型", Tier: model.SkillTierManager,
			Family: "self_stat_sprint", SelfStatMult: 1.10,
		},
		{
			ID: "m-frugal-stack", NameZH: "省雲費", Tier: model.SkillTierManager,
			Family: "infra_frugal", PreferRole: model.RoleEngineer, HasPrefer: true,
			InfraMult: 1.03,
		},
		{
			ID: "m-pipeline", NameZH: "資料管線", Tier: model.SkillTierManager,
			Family: "token_rnd", TokenRnDMult: 1.02,
		},
		{
			ID: "m-community", NameZH: "社群耳目", Tier: model.SkillTierManager,
			Family: "event_mgr", PreferRole: model.RoleMarketing, HasPrefer: true,
			EventNegMult: 0.95,
		},
		{
			ID: "m-process-nerd", NameZH: "流程控", Tier: model.SkillTierManager,
			Family: "company_ops_small", CompanyRolePower: crp(model.RoleOps, 0.02),
		},
	}

	// --- Director (18) — department / company ---
	dir := []SkillDef{
		{
			ID: "d-lab-lead", NameZH: "實驗室主導", Tier: model.SkillTierDirector,
			Family: "company_rnd", CompanyRolePower: crp(model.RoleResearcher, 0.06),
		},
		{
			ID: "d-infra-scale", NameZH: "基建擴張", Tier: model.SkillTierDirector,
			Family: "infra_dir", InfraMult: 1.05,
		},
		{
			ID: "d-sla-guard", NameZH: "SLA 守護", Tier: model.SkillTierDirector,
			Family: "churn_dir", ChurnMult: 0.94,
		},
		{
			ID: "d-brand", NameZH: "品牌操盤", Tier: model.SkillTierDirector,
			Family: "growth_dir", UserGrowthMult: 1.06,
		},
		{
			// Effective office level +0.75 for market rarity weights (soft-cap with talent blackhole).
			ID: "d-talent-magnet", NameZH: "伯樂", Tier: model.SkillTierDirector,
			Family: "market_rarity", MarketRarityBonus: 0.75,
		},
		{
			ID: "d-comp-opt", NameZH: "薪酬優化", Tier: model.SkillTierDirector,
			Family: "company_salary", CompanySalaryMult: 0.96,
		},
		{
			ID: "d-hiring-blitz", NameZH: "招聘衝刺", Tier: model.SkillTierDirector,
			Family: "hire_cost", HireCostMult: 0.90,
		},
		{
			// Lead-and-below power +5% approximated as company-wide role power.
			ID: "d-bench-strength", NameZH: "板凳深度", Tier: model.SkillTierDirector,
			Family: "bench", CompanyRolePower: crpAll(0.05),
		},
		{
			ID: "d-qa-gate", NameZH: "品質閘", Tier: model.SkillTierDirector,
			Family: "train_quality", TrainQualityMult: 1.04,
		},
		{
			// Cash-pressure event damage -15%.
			ID: "d-cost-ctrl", NameZH: "成本中心", Tier: model.SkillTierDirector,
			Family: "event_cost", EventNegMult: 0.85,
		},
		{
			ID: "d-partner", NameZH: "生態合作", Tier: model.SkillTierDirector,
			Family: "partner", UserGrowthMult: 1.03, RevenueMult: 1.02,
		},
		{
			// Safety / incident resistance.
			ID: "d-security", NameZH: "安全長視角", Tier: model.SkillTierDirector,
			Family: "event_security", EventNegMult: 0.90,
		},
		{
			ID: "d-platform", NameZH: "平台化", Tier: model.SkillTierDirector,
			Family: "company_eng", CompanyRolePower: crp(model.RoleEngineer, 0.04),
		},
		{
			ID: "d-revops", NameZH: "RevOps", Tier: model.SkillTierDirector,
			Family:           "revops",
			CompanyRolePower: crp2(model.RoleMarketing, 0.03, model.RoleOps, 0.03),
		},
		{
			ID: "d-research-ops", NameZH: "ResearchOps", Tier: model.SkillTierDirector,
			Family:           "research_ops",
			CompanyRolePower: crp2(model.RoleResearcher, 0.03, model.RoleOps, 0.03),
		},
		{
			// Market sense: mild growth + softer event outcomes.
			ID: "d-market-sense", NameZH: "市場嗅覺", Tier: model.SkillTierDirector,
			Family: "market_sense", UserGrowthMult: 1.03, EventNegMult: 0.95,
		},
		{
			// Effective seats +1 (company stacks cap +2 in sim).
			ID: "d-desk-layout", NameZH: "工位配置", Tier: model.SkillTierDirector,
			Family: "desk", ExtraSeats: 1,
		},
		{
			// Company-wide severance -20%.
			ID: "d-retention", NameZH: "留才", Tier: model.SkillTierDirector,
			Family: "severance_company", SeveranceMult: 0.80,
		},
	}

	// --- God permanent (12) ---
	god := []SkillDef{
		{
			ID: "g-polymath", NameZH: "通才光環", Tier: model.SkillTierGod,
			Family: "secondary_weight", SecondaryWeight: 0.55,
		},
		{
			ID: "g-frontier", NameZH: "前沿直覺", Tier: model.SkillTierGod,
			Family: "train_quality", TrainQualityMult: 1.08,
		},
		{
			ID: "g-rainmaker", NameZH: "印鈔機", Tier: model.SkillTierGod,
			Family: "revenue", RevenueMult: 1.05,
		},
		{
			ID: "g-crisis", NameZH: "危機大腦", Tier: model.SkillTierGod,
			Family: "event_crisis", EventNegMult: 0.80,
		},
		{
			ID: "g-architect", NameZH: "系統架構師", Tier: model.SkillTierGod,
			Family: "infra_god", InfraMult: 1.08,
		},
		{
			ID: "g-scientist", NameZH: "首席科學家", Tier: model.SkillTierGod,
			Family: "company_rnd", CompanyRolePower: crp(model.RoleResearcher, 0.10),
		},
		{
			ID: "g-operator", NameZH: "營運之神", Tier: model.SkillTierGod,
			Family: "churn_god", ChurnMult: 0.88,
		},
		{
			ID: "g-evangelist", NameZH: "傳道者", Tier: model.SkillTierGod,
			Family: "growth_god", UserGrowthMult: 1.10,
		},
		{
			// Soft-cap with 伯樂 (shared market_rarity family).
			ID: "g-talent-blackhole", NameZH: "人才黑洞", Tier: model.SkillTierGod,
			Family: "market_rarity", MarketRarityBonus: 1.0,
		},
		{
			// Self salary -15%; company research +3%.
			ID: "g-equity-mind", NameZH: "股權思維", Tier: model.SkillTierGod,
			Family: "salary_self", SelfSalaryMult: 0.85,
			CompanyRolePower: crp(model.RoleResearcher, 0.03),
		},
		{
			ID: "g-compounder", NameZH: "複利腦", Tier: model.SkillTierGod,
			Family: "token_rnd", TokenRnDMult: 1.05,
		},
		{
			ID: "g-full-stack-exec", NameZH: "全能高管", Tier: model.SkillTierGod,
			Family: "full_stack", CompanyRolePower: crpAll(0.03),
		},
	}

	// --- God Signature (9) — Signature=true; max 1 per god employee ---
	sig := []SkillDef{
		{
			ID: "gs-token-oracle", NameZH: "Token 神諭", Tier: model.SkillTierGod, Signature: true,
			Family: "token_rnd", TokenRnDMult: 1.15,
		},
		{
			// Self severance -50% (colleague -25% is sim-side).
			ID: "gs-poach-shield", NameZH: "挖角結界", Tier: model.SkillTierGod, Signature: true,
			Family: "severance_self", SeveranceMult: 0.50,
		},
		{
			ID: "gs-moonshot", NameZH: "登月提案", Tier: model.SkillTierGod, Signature: true,
			Family: "train_quality", TrainQualityMult: 1.12,
		},
		{
			ID: "gs-open-source-halo", NameZH: "開源光環", Tier: model.SkillTierGod, Signature: true,
			Family: "growth_oss", UserGrowthMult: 1.08,
		},
		{
			ID: "gs-chip-whisperer", NameZH: "晶片低語", Tier: model.SkillTierGod, Signature: true,
			Family: "infra_sig", InfraMult: 1.12,
		},
		{
			// Safety / compliance shock -40%.
			ID: "gs-regulatory-sage", NameZH: "監管智者", Tier: model.SkillTierGod, Signature: true,
			Family: "event_reg", EventNegMult: 0.60,
		},
		{
			ID: "gs-viral-loop", NameZH: "病毒迴路", Tier: model.SkillTierGod, Signature: true,
			Family: "growth_viral", UserGrowthMult: 1.12,
		},
		{
			// Revenue +8%; paid reroll base -30%.
			ID: "gs-war-chest", NameZH: "戰爭金庫", Tier: model.SkillTierGod, Signature: true,
			Family: "war_chest", RevenueMult: 1.08, RerollBaseMult: 0.70,
		},
		{
			ID: "gs-one-person-army", NameZH: "一人成軍", Tier: model.SkillTierGod, Signature: true,
			Family: "self_stat_army", SelfStatMult: 1.25,
		},
	}

	out := make([]SkillDef, 0, len(mgr)+len(dir)+len(god)+len(sig))
	out = append(out, mgr...)
	out = append(out, dir...)
	out = append(out, god...)
	out = append(out, sig...)
	return out
}
