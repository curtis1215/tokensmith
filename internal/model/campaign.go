package model

type Doctrine string

const (
	DoctrineNone       Doctrine = ""
	DoctrineConsumer   Doctrine = "consumer"
	DoctrineEnterprise Doctrine = "enterprise"
	DoctrineDeveloper  Doctrine = "developer"
)

type CampaignStage string

const (
	CampaignStageNone      CampaignStage = ""
	CampaignStageEstablish CampaignStage = "establish"
	CampaignStageExpand    CampaignStage = "expand"
	CampaignStageShowdown  CampaignStage = "showdown"
	CampaignStageWon       CampaignStage = "won"
)

type DirectiveKind string

const (
	DirectiveRoutePush DirectiveKind = "route-push"
	DirectiveCounter   DirectiveKind = "counter-rival"
	DirectiveIntel     DirectiveKind = "deep-intel"
)

type LegacyKind string

const (
	LegacyNone      LegacyKind = ""
	LegacySecondary LegacyKind = "secondary-doctrine"
	LegacyIntel     LegacyKind = "rival-intel"
	LegacyTech      LegacyKind = "starting-tech"
)

type LegacyChoice struct {
	Kind     LegacyKind `json:"kind,omitempty"`
	Doctrine Doctrine   `json:"doctrine,omitempty"`
	PerkID   string     `json:"perkId,omitempty"`
	TechID   string     `json:"techId,omitempty"`
}

type CampaignEffects struct {
	UserGrowthMult    [NumSegments]float64
	RefPriceMult      [NumSegments]float64
	RevenueMult       [NumSegments]float64
	InferenceLoadMult float64
	ServiceChurnMult  float64
	SafetyAppealMult  float64
	RivalImpactMult   float64
}

func NeutralCampaignEffects() CampaignEffects {
	e := CampaignEffects{InferenceLoadMult: 1, ServiceChurnMult: 1, SafetyAppealMult: 1, RivalImpactMult: 1}
	for i := 0; i < NumSegments; i++ {
		e.UserGrowthMult[i], e.RefPriceMult[i], e.RevenueMult[i] = 1, 1, 1
	}
	return e
}

type CampaignModifier struct {
	ID              string
	CyclesRemaining int
	Effects         CampaignEffects
}

type RivalRoadmap struct {
	Company           string
	ActionIndex       int
	CyclesUntilAction int
	IntelFull         bool
	LastExecutedCycle int
}

type CampaignReportKind string

const (
	ReportDoctrineChosen CampaignReportKind = "doctrine-chosen"
	ReportStageAdvanced  CampaignReportKind = "stage-advanced"
	ReportRivalAction    CampaignReportKind = "rival-action"
	ReportShowdown       CampaignReportKind = "showdown"
	ReportVictory        CampaignReportKind = "victory"
	ReportFinancialRisk  CampaignReportKind = "financial-risk"
)

type CampaignReportEntry struct {
	Kind      CampaignReportKind
	SubjectID string
	DetailID  string
	Value     float64
	Countered bool // 該宿敵行動被高層指令反制（衝擊已減半）
}

type BoardReport struct {
	Cycle   int
	Entries []CampaignReportEntry
}

type CampaignState struct {
	RandState               uint64
	Cycle                   int
	Doctrine                Doctrine
	Secondary               Doctrine
	SecondaryPerk           string
	Stage                   CampaignStage
	Perks                   []string
	PerkTierPending         int
	PivotUsed               bool
	DirectiveUsed           bool
	CounterTarget           string
	CounterActionID         string
	Primary                 RivalRoadmap
	Wildcard                RivalRoadmap
	Active                  []CampaignModifier
	ShowdownStartedCycle    int
	ShowdownHeld            int
	ShowdownAttempts        int
	Victory                 Doctrine
	Endless                 bool
	FinancialDistressCycles int
	Reports                 []BoardReport
	Legacy                  LegacyChoice
}

type ChooseDoctrine struct{ Doctrine Doctrine }

func (ChooseDoctrine) commandMarker() {}

type ChooseDoctrinePerk struct{ PerkID string }

func (ChooseDoctrinePerk) commandMarker() {}

type ChooseSecondaryDoctrine struct {
	Doctrine Doctrine
	PerkID   string
}

func (ChooseSecondaryDoctrine) commandMarker() {}

type PivotDoctrine struct{ Doctrine Doctrine }

func (PivotDoctrine) commandMarker() {}

type IssueDirective struct {
	Kind   DirectiveKind
	Target string
}

func (IssueDirective) commandMarker() {}

type CampaignPrestige struct{ Legacy LegacyChoice }

func (CampaignPrestige) commandMarker() {}

type CampaignContinue struct{}

func (CampaignContinue) commandMarker() {}

type CampaignExit struct{}

func (CampaignExit) commandMarker() {}
