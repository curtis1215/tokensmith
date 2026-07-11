# Tokensmith Strategic Campaign Phase A Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Phase A strategic campaign loop: eight-hour board cycles, company doctrines, deterministic rival roadmaps, executive directives, route victories, campaign Prestige/Legacy rewards, offline catch-up, and a TUI CEO war room.

**Architecture:** Keep the existing high-frequency `sim.Tick` economy unchanged in purpose and add a separate pure `sim.AdvanceCampaignCycle` layer. Store campaign truth in `model.GameState`, store only wall-clock scheduling in `store.Meta`, define content in focused `balance` catalogs, and make the TUI consume `sim` view helpers rather than duplicate formulas.

**Tech Stack:** Go 1.24, Bubble Tea, Bubbles viewport, Lipgloss, JSON persistence, standard-library tests.

## Global Constraints

- Scope is **Phase A only**. Do not implement Phase B multi-stage event chains or Phase C product/commercial/infrastructure pipelines.
- One board cycle is exactly `28_800` wall-clock seconds; catch-up is capped at `3` cycles; report history is capped at `20`.
- Campaign logic in `internal/sim` must remain pure and deterministic: no wall-clock, I/O, `math/rand`, or global mutable state.
- Existing 250ms economy Tick, token ingestion, industry events, JSON saves, and six-page navigation must keep working.
- At most one executive directive may be issued per cycle; at most one campaign decision may be pending.
- Rival actions must be seeded, visible at least one cycle ahead, and unable to inspect unsubmitted TUI input.
- Old saves without campaign fields must load without resetting resources.
- Strategic TUI commands must surface typed errors; do not route them through `applyOK()`.
- No new third-party dependency.
- Use TDD and make one focused commit per task.
- Design source: `docs/superpowers/specs/2026-07-10-tokensmith-strategic-campaign-expansion-design.md` §§4–5, 8–12.

---

## File Map

| File | Responsibility |
|---|---|
| `internal/model/campaign.go` | Stable enums, saved state, reports, effects, and campaign commands |
| `internal/balance/campaign.go` | Cycle constants, doctrine perks, rival profiles, and rival action catalogs |
| `internal/sim/campaign_apply.go` | Doctrine, perk, pivot, directive, and campaign-end command validation |
| `internal/sim/campaign_effects.go` | Aggregate perk/modifier effects into economy multipliers |
| `internal/sim/campaign_progress.go` | Route gates, showdown, and victory checks |
| `internal/sim/campaign_rivals.go` | Seeded rival selection and roadmap execution |
| `internal/sim/campaign_cycle.go` | One deterministic board-cycle transaction and report ring |
| `internal/sim/campaign_view.go` | CEO-war-room projections |
| `internal/store/meta.go` | Persist `LastCampaignUnix` |
| `internal/tui/campaign_clock.go` | Calculate capped wall-clock catch-up |
| `internal/tui/campaign_meta.go` | Traditional-Chinese campaign labels |
| `internal/tui/dialog_doctrine.go` | Doctrine, perk, secondary, and pivot choices |
| `internal/tui/dialog_directive.go` | Executive directive and target choices |
| `internal/tui/dialog_campaign_end.go` | Continue, Legacy Draft, Prestige, and exit choices |
| `internal/tui/page_overview.go` | CEO war room |

---

### Task 1: Add Stable Campaign State and Balance Catalogs

**Files:**
- Create: `internal/model/campaign.go`
- Modify: `internal/model/types.go`
- Create: `internal/balance/campaign.go`
- Modify: `internal/balance/balance.go`
- Create: `internal/balance/campaign_test.go`
- Modify: `internal/store/store_test.go`

**Interfaces:**
- Produces: `model.Doctrine`, `model.CampaignStage`, `model.CampaignState`, `model.CampaignEffects`, campaign command types, `balance.CampaignConfig`, `balance.DefaultCampaign()`.
- Consumed by: every later task.

- [ ] **Step 1: Write failing catalog and JSON round-trip tests**

Create `internal/balance/campaign_test.go`:

```go
package balance

import (
	"testing"
	"tokensmith/internal/model"
)

func TestDefaultCampaignContract(t *testing.T) {
	c := DefaultCampaign()
	if c.CycleSec != 8*60*60 || c.MaxCatchupCycles != 3 || c.ReportCap != 20 {
		t.Fatalf("clock config = %+v", c)
	}
	if len(c.Perks) != 12 { t.Fatalf("perk count = %d, want 12", len(c.Perks)) }
	if len(c.Rivals) != 7 { t.Fatalf("rival count = %d, want 7", len(c.Rivals)) }
	for _, d := range []model.Doctrine{model.DoctrineConsumer, model.DoctrineEnterprise, model.DoctrineDeveloper} {
		if len(PerksFor(c, d, 1)) != 2 || len(PerksFor(c, d, 2)) != 2 {
			t.Fatalf("doctrine %q does not have two perks per tier", d)
		}
	}
}

func TestCampaignLookupsRejectUnknownIDs(t *testing.T) {
	c := DefaultCampaign()
	if _, ok := CampaignPerkByID(c, "missing"); ok { t.Fatal("unknown perk resolved") }
	if _, ok := RivalActionByID(c, "missing"); ok { t.Fatal("unknown action resolved") }
}
```

Extend `TestSaveLoadRoundTrip` in `internal/store/store_test.go`:

```go
s.Campaign = model.CampaignState{
	RandState: 7, Cycle: 4, Doctrine: model.DoctrineConsumer,
	Stage: model.CampaignStageExpand, Perks: []string{"consumer-premium"},
}
s.Prestige.RouteBadges = []model.Doctrine{model.DoctrineConsumer}
```

Assert after load:

```go
if got.Campaign.Cycle != 4 || got.Campaign.Doctrine != model.DoctrineConsumer {
	t.Fatalf("campaign not restored: %+v", got.Campaign)
}
if len(got.Prestige.RouteBadges) != 1 { t.Fatalf("badges=%+v", got.Prestige.RouteBadges) }
```

- [ ] **Step 2: Run focused tests and verify red**

Run:

```bash
go test ./internal/balance ./internal/store -run 'TestDefaultCampaignContract|TestCampaignLookupsRejectUnknownIDs|TestSaveLoadRoundTrip' -count=1
```

Expected: compile failure because campaign types and catalogs do not exist.

- [ ] **Step 3: Add exact campaign model declarations**

Create `internal/model/campaign.go`:

```go
package model

type Doctrine string

const (
	DoctrineNone Doctrine = ""
	DoctrineConsumer Doctrine = "consumer"
	DoctrineEnterprise Doctrine = "enterprise"
	DoctrineDeveloper Doctrine = "developer"
)

type CampaignStage string

const (
	CampaignStageNone CampaignStage = ""
	CampaignStageEstablish CampaignStage = "establish"
	CampaignStageExpand CampaignStage = "expand"
	CampaignStageShowdown CampaignStage = "showdown"
	CampaignStageWon CampaignStage = "won"
)

type DirectiveKind string

const (
	DirectiveRoutePush DirectiveKind = "route-push"
	DirectiveCounter DirectiveKind = "counter-rival"
	DirectiveIntel DirectiveKind = "deep-intel"
)

type LegacyKind string

const (
	LegacyNone LegacyKind = ""
	LegacySecondary LegacyKind = "secondary-doctrine"
	LegacyIntel LegacyKind = "rival-intel"
	LegacyTech LegacyKind = "starting-tech"
)

type LegacyChoice struct {
	Kind LegacyKind `json:"kind,omitempty"`
	Doctrine Doctrine `json:"doctrine,omitempty"`
	PerkID string `json:"perkId,omitempty"`
	TechID string `json:"techId,omitempty"`
}

type CampaignEffects struct {
	UserGrowthMult [NumSegments]float64
	RefPriceMult [NumSegments]float64
	RevenueMult [NumSegments]float64
	InferenceLoadMult float64
	ServiceChurnMult float64
	SafetyAppealMult float64
	RivalImpactMult float64
}

func NeutralCampaignEffects() CampaignEffects {
	e := CampaignEffects{InferenceLoadMult: 1, ServiceChurnMult: 1, SafetyAppealMult: 1, RivalImpactMult: 1}
	for i := 0; i < NumSegments; i++ {
		e.UserGrowthMult[i], e.RefPriceMult[i], e.RevenueMult[i] = 1, 1, 1
	}
	return e
}

type CampaignModifier struct { ID string; CyclesRemaining int; Effects CampaignEffects }
type RivalRoadmap struct { Company string; ActionIndex int; CyclesUntilAction int; IntelFull bool; LastExecutedCycle int }

type CampaignReportKind string
const (
	ReportDoctrineChosen CampaignReportKind = "doctrine-chosen"
	ReportStageAdvanced CampaignReportKind = "stage-advanced"
	ReportRivalAction CampaignReportKind = "rival-action"
	ReportShowdown CampaignReportKind = "showdown"
	ReportVictory CampaignReportKind = "victory"
	ReportFinancialRisk CampaignReportKind = "financial-risk"
)

type CampaignReportEntry struct { Kind CampaignReportKind; SubjectID string; DetailID string; Value float64 }
type BoardReport struct { Cycle int; Entries []CampaignReportEntry }

type CampaignState struct {
	RandState uint64
	Cycle int
	Doctrine Doctrine
	Secondary Doctrine
	SecondaryPerk string
	Stage CampaignStage
	Perks []string
	PerkTierPending int
	PivotUsed bool
	DirectiveUsed bool
	CounterTarget string
	CounterActionID string
	Primary RivalRoadmap
	Wildcard RivalRoadmap
	Active []CampaignModifier
	ShowdownStartedCycle int
	ShowdownHeld int
	ShowdownAttempts int
	Victory Doctrine
	Endless bool
	FinancialDistressCycles int
	Reports []BoardReport
	Legacy LegacyChoice
}

type ChooseDoctrine struct{ Doctrine Doctrine }
func (ChooseDoctrine) commandMarker() {}
type ChooseDoctrinePerk struct{ PerkID string }
func (ChooseDoctrinePerk) commandMarker() {}
type ChooseSecondaryDoctrine struct{ Doctrine Doctrine; PerkID string }
func (ChooseSecondaryDoctrine) commandMarker() {}
type PivotDoctrine struct{ Doctrine Doctrine }
func (PivotDoctrine) commandMarker() {}
type IssueDirective struct{ Kind DirectiveKind; Target string }
func (IssueDirective) commandMarker() {}
type CampaignPrestige struct{ Legacy LegacyChoice }
func (CampaignPrestige) commandMarker() {}
type CampaignContinue struct{}
func (CampaignContinue) commandMarker() {}
type CampaignExit struct{}
func (CampaignExit) commandMarker() {}
```

Add `Campaign CampaignState` to `GameState`. Extend `Prestige`:

```go
type Prestige struct {
	Patents float64
	UnlockedPrestige []string
	RouteBadges []Doctrine `json:"routeBadges,omitempty"`
	PendingLegacy LegacyChoice `json:"pendingLegacy,omitempty"`
}
```

- [ ] **Step 4: Add the exact Phase A balance catalog**

Create `internal/balance/campaign.go` with these structs:

```go
type DoctrinePerkSpec struct { ID string; Doctrine model.Doctrine; Tier int; Effects model.CampaignEffects }
type RivalActionSpec struct {
	ID string
	Segment model.Segment
	LeadCycles int
	QualityPct [model.NumQualityDims]float64
	RefPriceMult float64
	DurationCycles int
}
type RivalProfile struct { Name string; PrimaryFor []model.Doctrine; Actions []string }
type CampaignConfig struct {
	CycleSec int64
	MaxCatchupCycles int
	ReportCap int
	PivotCashFloor float64
	PivotRevenueMonths float64
	PivotRnDFrac float64
	EstablishShare float64
	ConsumerExpandShare float64
	EnterpriseExpandShare float64
	DeveloperExpandShare float64
	ConsumerWinShare float64
	EnterpriseWinShare float64
	DeveloperWinShare float64
	StrategyExitCycle int
	Perks []DoctrinePerkSpec
	RivalActions []RivalActionSpec
	Rivals []RivalProfile
}
```

`DefaultCampaign()` must return the following exact values:

| Field | Value |
|---|---:|
| CycleSec | 28,800 |
| MaxCatchupCycles | 3 |
| ReportCap | 20 |
| PivotCashFloor | 20,000 |
| PivotRevenueMonths | 1 |
| PivotRnDFrac | 0.10 |
| EstablishShare | 0.07 |
| Consumer/Enterprise/Developer expand share | 0.11 / 0.095 / 0.095 |
| Consumer/Enterprise/Developer win share | 0.13 / 0.12 / 0.13 |
| StrategyExitCycle | 18 |

Share gates sit under the hard rival-band player-share ceiling
`1/(1+7×0.85)≈0.1439` (player defines GlobalFrontier; all seven default rivals
hard-clamped to the 85% floor). Ordering remains establish < expand < win;
consumer expand/win stay strictest, enterprise win easiest. Absolute levels
were recalibrated when long-term progression made the floor hard — raw
economic / campaign share still sums the full roster (see progression design §9/§12).

Add these exact perk effects, starting every unspecified multiplier at `1`:

| ID | Doctrine/Tier | Effects |
|---|---|---|
| `consumer-premium` | consumer/1 | consumer ref price ×1.15; growth ×0.90 |
| `consumer-mass` | consumer/1 | consumer ref price ×0.95; growth ×1.20 |
| `consumer-resilience` | consumer/2 | rival impact ×0.75 |
| `consumer-scale` | consumer/2 | consumer growth ×1.20; inference load ×1.10 |
| `enterprise-compliance` | enterprise/1 | safety appeal ×1.15; enterprise revenue ×0.95 |
| `enterprise-premium` | enterprise/1 | enterprise ref price ×1.15; growth ×0.90 |
| `enterprise-reliability` | enterprise/2 | service churn ×0.75 |
| `enterprise-sales` | enterprise/2 | enterprise growth ×1.20; inference load ×1.10 |
| `developer-open` | developer/1 | developer ref price ×0.90; growth ×1.25 |
| `developer-api` | developer/1 | developer ref price ×1.10; growth ×0.95 |
| `developer-efficient` | developer/2 | inference load ×0.85; developer revenue ×0.95 |
| `developer-usage` | developer/2 | inference load ×1.15; developer revenue ×1.20 |

Add rival actions and profiles:

| Action ID | Segment | Lead | Quality gain | Other |
|---|---|---:|---|---|
| `openai-flagship` | consumer | 2 | capability +15% | none |
| `openai-platform` | consumer | 3 | capability +8%, speed +8% | none |
| `anthropic-trust` | enterprise | 2 | capability +8%, safety +15% | none |
| `anthropic-enterprise-suite` | enterprise | 3 | efficiency +8%, safety +10% | none |
| `xai-scale` | consumer | 2 | capability +12%, speed +15% | none |
| `xai-compute-rush` | consumer | 3 | capability +10%, speed +12% | none |
| `deepseek-price-war` | developer | 2 | efficiency +15%, speed +10% | ref price ×0.85 for 2 cycles |
| `deepseek-distill` | developer | 3 | efficiency +12% | none |
| `qwen-ecosystem` | developer | 2 | efficiency +10%, speed +10% | none |
| `qwen-release-wave` | developer | 3 | capability +5%, efficiency +8%, speed +8% | none |
| `zhipu-enterprise` | enterprise | 3 | efficiency +12%, safety +12% | none |
| `zhipu-contract` | enterprise | 3 | safety +10%, speed +6% | none |
| `gemini-balanced` | consumer | 3 | all dimensions +8% | none |
| `gemini-multimodal` | consumer | 3 | capability +10%, safety +6%, speed +8% | none |

Each profile uses its two matching actions in the table order, then loops. Primary doctrine mappings: OpenAI/xAI/Gemini consumer; Anthropic/Zhipu/Gemini enterprise; DeepSeek/Qwen developer.

Implement `CampaignPerkByID`, `PerksFor`, `RivalActionByID`, and `RivalProfileByName` as deterministic slice scans. Add `Campaign CampaignConfig` to `balance.Config` and set `c.Campaign = DefaultCampaign()` in `Default()`.

- [ ] **Step 5: Run focused and full tests**

```bash
gofmt -w internal/model/campaign.go internal/model/types.go internal/balance/campaign.go internal/balance/campaign_test.go internal/balance/balance.go internal/store/store_test.go
go test ./internal/balance ./internal/store -count=1
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/model/campaign.go internal/model/types.go internal/balance/campaign.go internal/balance/campaign_test.go internal/balance/balance.go internal/store/store_test.go
git commit -m "feat(campaign): add phase-a state and catalogs"
```

---

### Task 2: Persist and Calculate the Wall-Clock Campaign Schedule

**Files:**
- Modify: `internal/store/meta.go`
- Modify: `internal/store/meta_test.go`
- Create: `internal/tui/campaign_clock.go`
- Create: `internal/tui/campaign_clock_test.go`
- Modify: `internal/tui/tui.go`
- Modify: `internal/tui/settle_test.go`

**Interfaces:**
- Produces: `campaignCyclesDue(last, now, cycleSec int64, cap int) (due int, nextLast int64)` and `store.Meta.LastCampaignUnix`.
- Consumes: `balance.Config.Campaign` from Task 1.
- Task 8 wires the helper to the cycle engine.

- [ ] **Step 1: Write failing clock and meta tests**

Create `internal/tui/campaign_clock_test.go`:

```go
package tui

import "testing"

func TestCampaignCyclesDuePreservesCadence(t *testing.T) {
	due, next := campaignCyclesDue(100, 100+9*60*60, 8*60*60, 3)
	if due != 1 || next != 100+8*60*60 { t.Fatalf("due=%d next=%d", due, next) }
}

func TestCampaignCyclesDueCapsAndDropsOldBacklog(t *testing.T) {
	now := int64(100 + 7*24*60*60)
	due, next := campaignCyclesDue(100, now, 8*60*60, 3)
	if due != 3 || next != now { t.Fatalf("due=%d next=%d", due, next) }
}

func TestCampaignCyclesDueUninitialized(t *testing.T) {
	due, next := campaignCyclesDue(0, 500, 8*60*60, 3)
	if due != 0 || next != 500 { t.Fatalf("due=%d next=%d", due, next) }
}
```

Extend `TestMetaRoundTrip` with `LastCampaignUnix: 84` and assert it round-trips.

- [ ] **Step 2: Run tests and verify red**

```bash
go test ./internal/tui ./internal/store -run 'TestCampaignCyclesDue|TestMetaRoundTrip' -count=1
```

Expected: compile failure for the missing helper and field.

- [ ] **Step 3: Implement the pure clock helper and persisted field**

Create `internal/tui/campaign_clock.go`:

```go
package tui

func campaignCyclesDue(last, now, cycleSec int64, cap int) (due int, nextLast int64) {
	if last <= 0 || now <= last || cycleSec <= 0 || cap <= 0 { return 0, now }
	raw := int((now - last) / cycleSec)
	if raw <= 0 { return 0, last }
	if raw > cap { return cap, now }
	return raw, last + int64(raw)*cycleSec
}
```

Add to `store.Meta`:

```go
LastCampaignUnix int64 `json:"lastCampaignUnix"`
```

Add `lastCampaignUnix int64` to `tui.Model`, load it in `newAtPaths`, and save it in `saveMeta`. Seed campaign RNG beside the existing event seed:

```go
if state.Campaign.RandState == 0 {
	state.Campaign.RandState = uint64(time.Now().UnixNano()) ^ 0x9e3779b97f4a7c15
}
```

- [ ] **Step 4: Run focused and full tests**

```bash
gofmt -w internal/store/meta.go internal/store/meta_test.go internal/tui/campaign_clock.go internal/tui/campaign_clock_test.go internal/tui/tui.go internal/tui/settle_test.go
go test ./internal/tui ./internal/store -run 'TestCampaignCyclesDue|TestMetaRoundTrip|TestNewAtSeedsRandState' -count=1
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/meta.go internal/store/meta_test.go internal/tui/campaign_clock.go internal/tui/campaign_clock_test.go internal/tui/tui.go internal/tui/settle_test.go
git commit -m "feat(campaign): persist board-cycle clock"
```

---

### Task 3: Implement Doctrine, Perk, Secondary, and Pivot Commands

**Files:**
- Create: `internal/sim/campaign_apply.go`
- Create: `internal/sim/campaign_apply_test.go`
- Create: `internal/sim/campaign_effects.go`
- Create: `internal/sim/campaign_effects_test.go`
- Modify: `internal/sim/apply.go`
- Modify: `internal/sim/sim.go`
- Modify: `internal/sim/view.go`

**Interfaces:**
- Produces: `campaignEffects(state, config) model.CampaignEffects`, doctrine command handlers, and typed `ErrCampaign*` errors.
- Task 4 sets `PerkTierPending`; command tests set it directly.

- [ ] **Step 1: Write failing command tests**

Create `internal/sim/campaign_apply_test.go` with:

```go
package sim

import (
	"errors"
	"testing"
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestChooseDoctrineRequiresOnlineModel(t *testing.T) {
	_, err := Apply(model.GameState{}, model.ChooseDoctrine{Doctrine: model.DoctrineConsumer}, balance.Default())
	if !errors.Is(err, ErrCampaignNeedsModel) { t.Fatalf("err=%v", err) }
}

func TestChooseDoctrineStartsEstablishStage(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Models: []model.Model{{Online: true}}}
	s.Campaign.RandState = 1
	ns, err := Apply(s, model.ChooseDoctrine{Doctrine: model.DoctrineConsumer}, b)
	if err != nil { t.Fatal(err) }
	if ns.Campaign.Doctrine != model.DoctrineConsumer || ns.Campaign.Stage != model.CampaignStageEstablish {
		t.Fatalf("campaign=%+v", ns.Campaign)
	}
}

func TestChoosePerkValidatesTierAndDoctrine(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Campaign: model.CampaignState{Doctrine: model.DoctrineConsumer, PerkTierPending: 1}}
	ns, err := Apply(s, model.ChooseDoctrinePerk{PerkID: "consumer-premium"}, b)
	if err != nil { t.Fatal(err) }
	if len(ns.Campaign.Perks) != 1 || ns.Campaign.PerkTierPending != 0 { t.Fatalf("campaign=%+v", ns.Campaign) }
}

func TestChooseSecondaryIncludesOneTierOnePerk(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Campaign: model.CampaignState{Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageShowdown}}
	ns, err := Apply(s, model.ChooseSecondaryDoctrine{Doctrine: model.DoctrineDeveloper, PerkID: "developer-open"}, b)
	if err != nil { t.Fatal(err) }
	if ns.Campaign.Secondary != model.DoctrineDeveloper || ns.Campaign.SecondaryPerk != "developer-open" { t.Fatalf("campaign=%+v", ns.Campaign) }
}

func TestPivotChargesAndResetsBuild(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Resources: model.Resources{Cash: 100000, RnD: 50000}}
	s.Models = []model.Model{{Online: true, Users: 1000, Price: 12}}
	s.Campaign = model.CampaignState{Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand, Perks: []string{"consumer-premium"}}
	ns, err := Apply(s, model.PivotDoctrine{Doctrine: model.DoctrineEnterprise}, b)
	if err != nil { t.Fatal(err) }
	if ns.Campaign.Doctrine != model.DoctrineEnterprise || !ns.Campaign.PivotUsed || len(ns.Campaign.Perks) != 0 { t.Fatalf("campaign=%+v", ns.Campaign) }
	if ns.Resources.Cash != 80000 || ns.Resources.RnD != 45000 { t.Fatalf("resources=%+v", ns.Resources) }
}
```

Create `campaign_effects_test.go`:

```go
func TestCampaignEffectsMultiplySelectedPerks(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Campaign: model.CampaignState{Perks: []string{"consumer-premium", "consumer-scale"}}}
	e := campaignEffects(s, b)
	if !approx(e.RefPriceMult[model.SegConsumer], 1.15) || !approx(e.UserGrowthMult[model.SegConsumer], 1.08) {
		t.Fatalf("effects=%+v", e)
	}
	if !approx(e.InferenceLoadMult, 1.10) { t.Fatalf("effects=%+v", e) }
}
```

- [ ] **Step 2: Run tests and verify red**

```bash
go test ./internal/sim -run 'TestChooseDoctrine|TestChoosePerk|TestPivot|TestCampaignEffects' -count=1
```

Expected: compile failure for missing handlers and errors.

- [ ] **Step 3: Implement command dispatch and validation**

Add these errors to `internal/sim/apply.go`:

```go
ErrCampaignNeedsModel = errors.New("sim: campaign needs an online model")
ErrInvalidDoctrine = errors.New("sim: invalid campaign doctrine")
ErrDoctrineAlreadyChosen = errors.New("sim: doctrine already chosen")
ErrInvalidDoctrinePerk = errors.New("sim: invalid doctrine perk")
ErrPerkChoiceNotReady = errors.New("sim: doctrine perk choice not ready")
ErrSecondaryNotReady = errors.New("sim: secondary doctrine not ready")
ErrPivotAlreadyUsed = errors.New("sim: doctrine pivot already used")
ErrPivotLocked = errors.New("sim: doctrine pivot locked during showdown")
```

Add `Apply` cases for `ChooseDoctrine`, `ChooseDoctrinePerk`, `ChooseSecondaryDoctrine`, and `PivotDoctrine`, delegating to `campaign_apply.go`.

Implement these exact validations:

- primary doctrine must be consumer/enterprise/developer, require one online model, and require no existing primary;
- selection sets stage `establish` and applies a saved secondary Legacy;
- perk ID must belong to the active doctrine and current `PerkTierPending`, then clear pending;
- secondary doctrine requires stage `showdown` and cannot equal primary;
- pivot cannot target the same doctrine, run twice, or occur during showdown/won;
- pivot cash is `max(20000, MonthlyRevenue*1.0)` and R&D is 10% of current R&D;
- pivot resets secondary, stage, perks, pending perk, showdown counters, and roadmaps; it preserves cycle, reports, RNG, and active modifiers.

Use these handlers; Task 5 replaces the roadmap-zeroing lines with `initCampaignRoadmaps`:

```go
func validDoctrine(d model.Doctrine) bool {
	return d == model.DoctrineConsumer || d == model.DoctrineEnterprise || d == model.DoctrineDeveloper
}

func hasOnlineModel(s model.GameState) bool {
	for _, m := range s.Models { if m.Online { return true } }
	return false
}

func applyChooseDoctrine(s model.GameState, c model.ChooseDoctrine, b balance.Config) (model.GameState, error) {
	if !validDoctrine(c.Doctrine) { return s, ErrInvalidDoctrine }
	if s.Campaign.Doctrine != model.DoctrineNone { return s, ErrDoctrineAlreadyChosen }
	if !hasOnlineModel(s) { return s, ErrCampaignNeedsModel }
	ns := s
	ns.Campaign.Doctrine = c.Doctrine
	ns.Campaign.Stage = model.CampaignStageEstablish
	if ns.Campaign.Legacy.Kind == model.LegacySecondary {
		ns.Campaign.Secondary, ns.Campaign.SecondaryPerk = ns.Campaign.Legacy.Doctrine, ns.Campaign.Legacy.PerkID
	}
	return ns, nil
}

func applyChooseDoctrinePerk(s model.GameState, c model.ChooseDoctrinePerk, b balance.Config) (model.GameState, error) {
	p, ok := balance.CampaignPerkByID(b.Campaign, c.PerkID)
	if !ok || p.Doctrine != s.Campaign.Doctrine { return s, ErrInvalidDoctrinePerk }
	if s.Campaign.PerkTierPending == 0 || p.Tier != s.Campaign.PerkTierPending { return s, ErrPerkChoiceNotReady }
	for _, id := range s.Campaign.Perks { if id == p.ID { return s, ErrAlreadyUnlocked } }
	ns := s
	ns.Campaign.Perks = append(append([]string(nil), s.Campaign.Perks...), p.ID)
	ns.Campaign.PerkTierPending = 0
	return ns, nil
}

func applyChooseSecondaryDoctrine(s model.GameState, c model.ChooseSecondaryDoctrine, b balance.Config) (model.GameState, error) {
	if !validDoctrine(c.Doctrine) || c.Doctrine == s.Campaign.Doctrine { return s, ErrInvalidDoctrine }
	if s.Campaign.Stage != model.CampaignStageShowdown { return s, ErrSecondaryNotReady }
	p, ok := balance.CampaignPerkByID(b.Campaign, c.PerkID)
	if !ok || p.Doctrine != c.Doctrine || p.Tier != 1 { return s, ErrInvalidDoctrinePerk }
	ns := s
	ns.Campaign.Secondary, ns.Campaign.SecondaryPerk = c.Doctrine, c.PerkID
	return ns, nil
}

func applyPivotDoctrine(s model.GameState, c model.PivotDoctrine, b balance.Config) (model.GameState, error) {
	if !validDoctrine(c.Doctrine) || c.Doctrine == s.Campaign.Doctrine { return s, ErrInvalidDoctrine }
	if s.Campaign.PivotUsed { return s, ErrPivotAlreadyUsed }
	if s.Campaign.Stage == model.CampaignStageShowdown || s.Campaign.Stage == model.CampaignStageWon { return s, ErrPivotLocked }
	cashCost := math.Max(b.Campaign.PivotCashFloor, MonthlyRevenue(s)*b.Campaign.PivotRevenueMonths)
	rndCost := s.Resources.RnD * b.Campaign.PivotRnDFrac
	if s.Resources.Cash < cashCost { return s, ErrInsufficientCash }
	if s.Resources.RnD < rndCost { return s, ErrInsufficientRnD }
	ns := s
	ns.Resources.Cash -= cashCost
	ns.Resources.RnD -= rndCost
	ns.Campaign.Doctrine, ns.Campaign.Secondary, ns.Campaign.SecondaryPerk = c.Doctrine, model.DoctrineNone, ""
	ns.Campaign.Stage, ns.Campaign.Perks = model.CampaignStageEstablish, nil
	ns.Campaign.PerkTierPending, ns.Campaign.PivotUsed = 0, true
	ns.Campaign.ShowdownHeld, ns.Campaign.ShowdownStartedCycle = 0, 0
	ns.Campaign.Primary, ns.Campaign.Wildcard = model.RivalRoadmap{}, model.RivalRoadmap{}
	return ns, nil
}
```

- [ ] **Step 4: Aggregate perk and modifier effects and wire them into economy formulas**

Create `campaign_effects.go`:

```go
func multiplyCampaignEffects(dst *model.CampaignEffects, src model.CampaignEffects) {
	for i := 0; i < model.NumSegments; i++ {
		dst.UserGrowthMult[i] *= src.UserGrowthMult[i]
		dst.RefPriceMult[i] *= src.RefPriceMult[i]
		dst.RevenueMult[i] *= src.RevenueMult[i]
	}
	dst.InferenceLoadMult *= src.InferenceLoadMult
	dst.ServiceChurnMult *= src.ServiceChurnMult
	dst.SafetyAppealMult *= src.SafetyAppealMult
	dst.RivalImpactMult *= src.RivalImpactMult
}

func campaignEffects(s model.GameState, b balance.Config) model.CampaignEffects {
	out := model.NeutralCampaignEffects()
	for _, id := range s.Campaign.Perks {
		if p, ok := balance.CampaignPerkByID(b.Campaign, id); ok { multiplyCampaignEffects(&out, p.Effects) }
	}
	if p, ok := balance.CampaignPerkByID(b.Campaign, s.Campaign.SecondaryPerk); ok && p.Tier == 1 && p.Doctrine == s.Campaign.Secondary {
		multiplyCampaignEffects(&out, p.Effects)
	}
	for _, m := range s.Campaign.Active {
		if m.CyclesRemaining > 0 { multiplyCampaignEffects(&out, m.Effects) }
	}
	return out
}
```

Wire effects into both real and preview formulas:

- `advanceUsers` and `EstimateUserTarget`: segment growth, enterprise safety appeal, effective reference price, segment revenue;
- `EffectiveRefPrice`: segment reference-price multiplier;
- `advanceServing` and `ServableUsers`: inference-load multiplier;
- overload decay: service-churn multiplier;
- `Valuation`: segment revenue multiplier.

The modified expressions must be exactly:

```go
ce := campaignEffects(ns, b)
w := b.SegmentWeights[m.Segment]
if m.Segment == model.SegEnterprise { w[model.DimSafety] *= ce.SafetyAppealMult }
refPrice := EffectiveRefPrice(ns, m.Segment, b)
target *= ce.UserGrowthMult[m.Segment]
ns.Resources.Cash += m.Users * m.Price * ce.RevenueMult[m.Segment] * dt / b.MonthSec * pe.CashMult * b.RevenueMult
load += m.Users * b.InferenceLoadPerUser * ce.InferenceLoadMult
newLoad := capacity + (load-capacity)*math.Exp(-b.ServiceChurnRate*ce.ServiceChurnMult*dt*opsFactor)
```

`EstimateUserTarget` uses the same `w`, `refPrice`, and growth expressions. `ServableUsers` divides by `b.InferenceLoadPerUser*ce.InferenceLoadMult`. `Valuation` multiplies each online model's `users*price` by the matching segment revenue multiplier.

Add focused regression tests for each hook so the TUI preview and Tick cannot diverge.

- [ ] **Step 5: Run focused and full tests**

```bash
gofmt -w internal/sim/campaign_apply.go internal/sim/campaign_apply_test.go internal/sim/campaign_effects.go internal/sim/campaign_effects_test.go internal/sim/apply.go internal/sim/sim.go internal/sim/view.go
go test ./internal/sim -run 'TestChooseDoctrine|TestChoosePerk|TestPivot|TestCampaignEffects|TestTickSubscriptionRevenue|TestEffectiveRefPrice|TestServableUsers' -count=1
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/sim/campaign_apply.go internal/sim/campaign_apply_test.go internal/sim/campaign_effects.go internal/sim/campaign_effects_test.go internal/sim/apply.go internal/sim/sim.go internal/sim/view.go
git commit -m "feat(campaign): add doctrines perks and pivot"
```

---

### Task 4: Add Route Progress, Stage Gates, and Showdown Victory

**Files:**
- Create: `internal/sim/campaign_progress.go`
- Create: `internal/sim/campaign_progress_test.go`
- Create: `internal/sim/campaign_view.go`
- Create: `internal/sim/campaign_view_test.go`
- Modify: `internal/sim/view.go`

**Interfaces:**
- Produces: `CampaignStatus(state, config) CampaignStatusView`, `RouteVictoryStatus(state, config, doctrine) CampaignStatusView`, `CampaignRivalIntel(state, config, primary) (RivalIntelView, bool)`, `advanceCampaignProgress(state, config)`.
- Task 5 calls progress after rival actions.

- [ ] **Step 1: Write failing stage and victory tests**

Use this fixture in `campaign_progress_test.go`:

```go
func campaignRouteState(d model.Doctrine, seg model.Segment) model.GameState {
	b := balance.Default()
	s := model.GameState{}
	s.Campaign = model.CampaignState{Doctrine: d, Stage: model.CampaignStageEstablish}
	s.Models = []model.Model{{Online: true, Segment: seg, Price: b.SegmentRefPrice[seg], Users: 100000, Quality: [4]float64{80, 80, 80, 80}}}
	s.Competitors = []model.Competitor{{Name: "Rival", Quality: [4]float64{1, 1, 1, 1}}}
	s.Compute.RentedInference = map[string]int{balance.EntryProcessID: 1000}
	return s
}
```

Add tests proving: establish unlocks tier-one perk; expand cannot advance before tier-one selection; each route uses its approved gate; showdown requires two selected perks; victory requires two held cycles after a primary rival action; broken conditions reset held count without ending the run.

- [ ] **Step 2: Run tests and verify red**

```bash
go test ./internal/sim -run 'TestCampaignEstablish|TestCampaignExpand|TestCampaignVictory|TestCampaignShowdown' -count=1
```

Expected: compile failure for missing helpers.

- [ ] **Step 3: Implement exact read-only status interfaces**

Define:

```go
type CampaignStatusView struct {
	Active bool
	Doctrine model.Doctrine
	Stage model.CampaignStage
	Share float64
	QualityRank int
	PriceOK bool
	CapacityOK bool
	CashflowOK bool
	Progress float64
	Victory bool
}

type RivalIntelView struct {
	Company string
	ConfirmedActionID string
	RumoredActionID string
	CyclesUntilAction int
	IntelFull bool
}
```

`CampaignRivalIntel` resolves the roadmap's current and next action IDs through `RivalProfileByName`; unknown company/action returns `ok=false` rather than inventing copy.

Do not reuse the current `MarketRank` directly because it does not filter the player's models by segment. Add these campaign-specific helpers:

```go
func bestRouteModel(s model.GameState, b balance.Config, seg model.Segment) (model.Model, float64, bool) {
	w := b.SegmentWeights[seg]
	ce := campaignEffects(s, b)
	if seg == model.SegEnterprise { w[model.DimSafety] *= ce.SafetyAppealMult }
	var best model.Model
	bestAppeal := 0.0
	found := false
	for _, m := range s.Models {
		if !m.Online || m.Segment != seg { continue }
		a := appealOf(m.Quality, w)
		if !found || a > bestAppeal { best, bestAppeal, found = m, a, true }
	}
	return best, bestAppeal, found
}

func campaignQualityRank(s model.GameState, b balance.Config, seg model.Segment) int {
	_, playerAppeal, found := bestRouteModel(s, b, seg)
	if !found { return len(s.Competitors)+1 }
	w := b.SegmentWeights[seg]
	rank := 1
	for _, c := range s.Competitors { if appealOf(c.Quality, w) > playerAppeal { rank++ } }
	return rank
}

func enterpriseSafetyOK(s model.GameState, b balance.Config) bool {
	m, _, ok := bestRouteModel(s, b, model.SegEnterprise)
	if !ok { return false }
	threshold := 15.0
	for _, c := range s.Competitors {
		if c.Name == s.Campaign.Primary.Company && c.Quality[model.DimSafety]*0.9 > threshold { threshold = c.Quality[model.DimSafety]*0.9 }
	}
	return m.Quality[model.DimSafety] >= threshold
}
```

`CampaignStatus.PriceOK` uses the price of `bestRouteModel`, never an unrelated online model. Developer and enterprise comparisons use `EffectiveRefPrice` exactly as specified.

`RouteVictoryStatus` evaluates the showdown gate for an arbitrary doctrine without mutating `Campaign.Doctrine`; the CEO war room uses it after `CampaignContinue` so endless mode can display progress toward the other two route goals. It does not award badges or additional Prestige rewards.

```go
func RouteVictoryStatus(s model.GameState, b balance.Config, d model.Doctrine) CampaignStatusView {
	viewState := s
	viewState.Campaign.Doctrine = d
	viewState.Campaign.Stage = model.CampaignStageShowdown
	viewState.Campaign.Victory = model.DoctrineNone
	viewState.Campaign.Endless = false
	return CampaignStatus(viewState, b)
}
```

Add `doctrineSegment`, `playerSegmentShare`, `campaignCapacityOK`, and exported `NetCashPerSec`. Use the exact gates from the master design:

- establish: route model online and share ≥10%;
- consumer expand: share ≥25% and inference ≤90%;
- enterprise expand: safety ≥`max(15, primary safety*0.9)`, share ≥20%, price ≥ref;
- developer expand: quality rank ≤3, share ≥20%, price ≤ref;
- consumer win: rank 1, share ≥35%, inference ≤100%;
- enterprise win: safety threshold, share ≥30%, price gate, Ops ≥1, inference ≤80%;
- developer win: rank 1, share ≥35%, price ≤90% ref, net cash/sec >0, inference ≤80%.

`Progress` is the minimum normalized completion of the current gate's required metrics, clamped to `[0,1]`.

Use one switch so view and transition logic share the same truth:

```go
func campaignGateMet(s model.GameState, b balance.Config, status CampaignStatusView) bool {
	switch s.Campaign.Stage {
	case model.CampaignStageEstablish:
		return status.Share >= b.Campaign.EstablishShare && hasOnlineModelInSegment(s, doctrineSegment(s.Campaign.Doctrine))
	case model.CampaignStageExpand:
		switch s.Campaign.Doctrine {
		case model.DoctrineConsumer:
			return status.Share >= b.Campaign.ConsumerExpandShare && campaignCapacityOK(s, b, 0.90)
		case model.DoctrineEnterprise:
			return enterpriseSafetyOK(s, b) && status.Share >= b.Campaign.EnterpriseExpandShare && status.PriceOK
		case model.DoctrineDeveloper:
			return status.QualityRank <= 3 && status.Share >= b.Campaign.DeveloperExpandShare && status.PriceOK
		}
	case model.CampaignStageShowdown:
		switch s.Campaign.Doctrine {
		case model.DoctrineConsumer:
			return status.QualityRank == 1 && status.Share >= b.Campaign.ConsumerWinShare && campaignCapacityOK(s, b, 1.0)
		case model.DoctrineEnterprise:
			return enterpriseSafetyOK(s, b) && status.Share >= b.Campaign.EnterpriseWinShare && status.PriceOK && s.Ops >= 1 && campaignCapacityOK(s, b, 0.80)
		case model.DoctrineDeveloper:
			return status.QualityRank == 1 && status.Share >= b.Campaign.DeveloperWinShare && status.PriceOK && status.CashflowOK && campaignCapacityOK(s, b, 0.80)
		}
	}
	return false
}
```

- [ ] **Step 4: Implement deterministic transitions**

`advanceCampaignProgress` must:

1. ignore no-doctrine, won, and endless states;
2. transition establish→expand and set `PerkTierPending=1`;
3. require one selected perk before expand can complete;
4. transition expand→showdown and set `PerkTierPending=2`;
5. require two selected perks before showdown starts;
6. on first complete win state, set `ShowdownStartedCycle=Cycle`, force primary countdown to `1`, and report showdown;
7. count held cycles only after `Primary.LastExecutedCycle >= ShowdownStartedCycle`;
8. reset held count and increment attempts when conditions break;
9. on two held cycles set `Victory=Doctrine`, stage `won`, and report victory.

Implement transitions with this exact state machine:

```go
func advanceCampaignProgress(s model.GameState, b balance.Config) (model.GameState, []model.CampaignReportEntry) {
	if s.Campaign.Doctrine == model.DoctrineNone || s.Campaign.Victory != model.DoctrineNone || s.Campaign.Endless { return s, nil }
	ns := s
	status := CampaignStatus(ns, b)
	if ns.Campaign.Stage == model.CampaignStageEstablish && campaignGateMet(ns, b, status) {
		ns.Campaign.Stage, ns.Campaign.PerkTierPending = model.CampaignStageExpand, 1
		return ns, []model.CampaignReportEntry{{Kind: model.ReportStageAdvanced, SubjectID: string(ns.Campaign.Stage)}}
	}
	if ns.Campaign.Stage == model.CampaignStageExpand && len(ns.Campaign.Perks) >= 1 && campaignGateMet(ns, b, status) {
		ns.Campaign.Stage, ns.Campaign.PerkTierPending = model.CampaignStageShowdown, 2
		return ns, []model.CampaignReportEntry{{Kind: model.ReportStageAdvanced, SubjectID: string(ns.Campaign.Stage)}}
	}
	if ns.Campaign.Stage != model.CampaignStageShowdown || len(ns.Campaign.Perks) < 2 { return ns, nil }
	if !campaignGateMet(ns, b, status) {
		if ns.Campaign.ShowdownHeld > 0 { ns.Campaign.ShowdownAttempts++ }
		ns.Campaign.ShowdownHeld = 0
		return ns, nil
	}
	if ns.Campaign.ShowdownStartedCycle == 0 {
		ns.Campaign.ShowdownStartedCycle = ns.Campaign.Cycle
		ns.Campaign.Primary.CyclesUntilAction = 1
		return ns, []model.CampaignReportEntry{{Kind: model.ReportShowdown, SubjectID: ns.Campaign.Primary.Company}}
	}
	if ns.Campaign.Primary.LastExecutedCycle < ns.Campaign.ShowdownStartedCycle { return ns, nil }
	ns.Campaign.ShowdownHeld++
	if ns.Campaign.ShowdownHeld < 2 { return ns, nil }
	ns.Campaign.Victory, ns.Campaign.Stage = ns.Campaign.Doctrine, model.CampaignStageWon
	return ns, []model.CampaignReportEntry{{Kind: model.ReportVictory, SubjectID: string(ns.Campaign.Doctrine)}}
}
```

- [ ] **Step 5: Run focused and full tests**

```bash
gofmt -w internal/sim/campaign_progress.go internal/sim/campaign_progress_test.go internal/sim/campaign_view.go internal/sim/campaign_view_test.go internal/sim/view.go
go test ./internal/sim -run 'TestCampaign|TestNetCashPerSec' -count=1
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/sim/campaign_progress.go internal/sim/campaign_progress_test.go internal/sim/campaign_view.go internal/sim/campaign_view_test.go internal/sim/view.go
git commit -m "feat(campaign): add route stages and victory gates"
```

---

### Task 5: Add Seeded Rival Roadmaps and the Board-Cycle Engine

**Files:**
- Create: `internal/sim/campaign_rivals.go`
- Create: `internal/sim/campaign_rivals_test.go`
- Create: `internal/sim/campaign_cycle.go`
- Create: `internal/sim/campaign_cycle_test.go`
- Modify: `internal/sim/campaign_apply.go`
- Modify: `internal/sim/sim.go`

**Interfaces:**
- Produces: `initCampaignRoadmaps`, `advanceRivalRoadmap`, exported `AdvanceCampaignCycle(state, config) model.GameState`.
- Consumes: progress logic from Task 4 and catalogs from Task 1.
- Preserves pre-campaign competitor behavior.

- [ ] **Step 1: Write failing roadmap and cycle tests**

Create tests:

```go
func TestCampaignRoadmapsDeterministicAndDistinct(t *testing.T) {
	b := balance.Default()
	a := model.GameState{Campaign: model.CampaignState{RandState: 42}}
	c := a
	a = initCampaignRoadmaps(a, model.DoctrineConsumer, b)
	c = initCampaignRoadmaps(c, model.DoctrineConsumer, b)
	if a.Campaign.Primary != c.Campaign.Primary || a.Campaign.Wildcard != c.Campaign.Wildcard {
		t.Fatalf("same seed diverged: %+v %+v", a.Campaign, c.Campaign)
	}
	if a.Campaign.Primary.Company == a.Campaign.Wildcard.Company { t.Fatal("rival roles must differ") }
}

func TestAdvanceCampaignCycleExecutesTelegraphedAction(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Competitors: balance.DefaultCompetitors()}
	s.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand,
		Primary: model.RivalRoadmap{Company: "OpenAI", CyclesUntilAction: 1},
		Wildcard: model.RivalRoadmap{Company: "DeepSeek", CyclesUntilAction: 2},
	}
	before := s.Competitors[0].Quality[model.DimCapability]
	ns := AdvanceCampaignCycle(s, b)
	if ns.Campaign.Cycle != 1 || ns.Competitors[0].Quality[model.DimCapability] <= before { t.Fatalf("campaign=%+v", ns.Campaign) }
	if ns.Campaign.Primary.LastExecutedCycle != 1 { t.Fatalf("roadmap=%+v", ns.Campaign.Primary) }
}

func TestCampaignCycleCapsReportRing(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Campaign: model.CampaignState{Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand}}
	for i := 0; i < b.Campaign.ReportCap+5; i++ { s = AdvanceCampaignCycle(s, b) }
	if len(s.Campaign.Reports) != b.Campaign.ReportCap { t.Fatalf("reports=%d", len(s.Campaign.Reports)) }
}
```

- [ ] **Step 2: Run tests and verify red**

```bash
go test ./internal/sim -run 'TestCampaignRoadmaps|TestAdvanceCampaignCycle|TestCampaignCycleCaps' -count=1
```

Expected: compile failure for missing roadmap/cycle functions.

- [ ] **Step 3: Implement deterministic rival selection**

Create a local SplitMix64 helper in `campaign_rivals.go`. Selection contract:

- primary candidates are profiles whose `PrimaryFor` contains the selected doctrine;
- wildcard candidates are every other profile;
- choose both using saved `Campaign.RandState`, write advanced RNG state back;
- roadmap starts at `ActionIndex=0` and the first action's `LeadCycles`;
- primary/wildcard company names must differ;
- if a profile/action ID is missing after a catalog upgrade, leave the roadmap inert and add no report rather than panicking.

At the end of successful primary selection and pivot, call `initCampaignRoadmaps`.

Use this RNG and candidate selection:

```go
func campaignRand(state uint64) (uint64, float64) {
	state += 0x9e3779b97f4a7c15
	z := state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	z ^= z >> 31
	return state, float64(z>>11) / float64(uint64(1)<<53)
}

func pickRival(candidates []balance.RivalProfile, state uint64) (balance.RivalProfile, uint64) {
	next, roll := campaignRand(state)
	idx := int(roll * float64(len(candidates)))
	if idx >= len(candidates) { idx = len(candidates)-1 }
	return candidates[idx], next
}

func roadmapFor(p balance.RivalProfile, b balance.Config) model.RivalRoadmap {
	a, _ := balance.RivalActionByID(b.Campaign, p.Actions[0])
	return model.RivalRoadmap{Company: p.Name, ActionIndex: 0, CyclesUntilAction: a.LeadCycles}
}
```

- [ ] **Step 4: Implement action execution and reports**

For each due roadmap:

- decrement `CyclesUntilAction`; execute only when it reaches zero;
- multiply the named competitor dimension by `1 + QualityPct[d]*impact`;
- set `impact=campaignEffects(s,b).RivalImpactMult` and multiply by `0.5` for an exact stored counter match;
- append a ref-price modifier when `RefPriceMult>0`;
- set `LastExecutedCycle=Campaign.Cycle`;
- append `ReportRivalAction{SubjectID: company, DetailID: actionID}`;
- advance action index modulo profile actions and schedule the next lead time.

Keep action execution in a single function with this signature so both roadmap roles use identical rules:

```go
func executeRivalAction(s model.GameState, roadmap model.RivalRoadmap, b balance.Config) (model.GameState, model.RivalRoadmap, model.CampaignReportEntry, bool)
```

The function clones `Competitors` and `Campaign.Active` before mutation. A matching counter is cleared only after the action ID and company both match. `advanceRivalRoadmap(s, primary, b, entries)` selects `Campaign.Primary` or `Wildcard`, decrements it, calls `executeRivalAction`, stores the returned roadmap, and appends the returned entry only when `ok=true`.

Expose these internal helpers to Task 6:

```go
func roadmapActionID(r model.RivalRoadmap, b balance.Config) (string, bool) {
	p, ok := balance.RivalProfileByName(b.Campaign, r.Company)
	if !ok || len(p.Actions) == 0 { return "", false }
	return p.Actions[r.ActionIndex%len(p.Actions)], true
}

func campaignRoadmapByCompany(s model.GameState, company string) (model.RivalRoadmap, bool) {
	if s.Campaign.Primary.Company == company { return s.Campaign.Primary, true }
	if s.Campaign.Wildcard.Company == company { return s.Campaign.Wildcard, true }
	return model.RivalRoadmap{}, false
}
```

- [ ] **Step 5: Implement one board-cycle transaction**

Create `campaign_cycle.go`:

```go
func ageCampaignModifiers(in []model.CampaignModifier) []model.CampaignModifier {
	out := make([]model.CampaignModifier, 0, len(in))
	for _, m := range in {
		m.CyclesRemaining--
		if m.CyclesRemaining > 0 { out = append(out, m) }
	}
	return out
}

func appendBoardReport(in []model.BoardReport, report model.BoardReport, cap int) []model.BoardReport {
	out := append(append([]model.BoardReport(nil), in...), report)
	if cap > 0 && len(out) > cap { out = out[len(out)-cap:] }
	return out
}

func AdvanceCampaignCycle(s model.GameState, b balance.Config) model.GameState {
	if s.Campaign.Doctrine == model.DoctrineNone { return s }
	ns := s
	ns.Campaign.Cycle++
	ns.Campaign.Active = ageCampaignModifiers(ns.Campaign.Active)
	var entries []model.CampaignReportEntry
	ns, entries = advanceRivalRoadmap(ns, true, b, entries)
	ns, entries = advanceRivalRoadmap(ns, false, b, entries)
	var progress []model.CampaignReportEntry
	ns, progress = advanceCampaignProgress(ns, b)
	entries = append(entries, progress...)
	if ns.Resources.Cash < 0 {
		ns.Campaign.FinancialDistressCycles++
		entries = append(entries, model.CampaignReportEntry{Kind: model.ReportFinancialRisk, Value: ns.Resources.Cash})
	} else {
		ns.Campaign.FinancialDistressCycles = 0
	}
	ns.Campaign.DirectiveUsed = false
	ns.Campaign.Reports = appendBoardReport(ns.Campaign.Reports, model.BoardReport{Cycle: ns.Campaign.Cycle, Entries: entries}, b.Campaign.ReportCap)
	return ns
}
```

- [ ] **Step 6: Disable per-Tick rubber-band after doctrine selection**

At the start of `advanceCompetitors`:

```go
if ns.Campaign.Doctrine != model.DoctrineNone { return ns }
```

Add one regression proving pre-campaign competitors still move and an active campaign competitor moves only through board actions.

- [ ] **Step 7: Run focused and full tests**

```bash
gofmt -w internal/sim/campaign_rivals.go internal/sim/campaign_rivals_test.go internal/sim/campaign_cycle.go internal/sim/campaign_cycle_test.go internal/sim/campaign_apply.go internal/sim/sim.go
go test ./internal/sim -run 'TestCampaignRoadmaps|TestAdvanceCampaignCycle|TestCampaignCycleCaps|TestTickAdvancesCompetitors' -count=1
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/sim/campaign_rivals.go internal/sim/campaign_rivals_test.go internal/sim/campaign_cycle.go internal/sim/campaign_cycle_test.go internal/sim/campaign_apply.go internal/sim/sim.go
git commit -m "feat(campaign): add rival roadmaps and board cycles"
```

---

### Task 6: Add One-Per-Cycle Executive Directives

**Files:**
- Modify: `internal/sim/campaign_apply.go`
- Modify: `internal/sim/campaign_apply_test.go`
- Modify: `internal/sim/apply.go`
- Modify: `internal/sim/campaign_rivals.go`
- Modify: `internal/sim/campaign_cycle.go`

**Interfaces:**
- Produces: `applyIssueDirective` for route push, counter-rival, and deep-intel.
- Consumes: campaign modifiers, roadmaps, `MonthlyRevenue`, and cycle reset.

- [ ] **Step 1: Write failing directive tests**

```go
func TestRoutePushCostsCashAndAddsModifier(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Resources: model.Resources{Cash: 50000}}
	s.Campaign = model.CampaignState{Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand}
	ns, err := Apply(s, model.IssueDirective{Kind: model.DirectiveRoutePush}, b)
	if err != nil { t.Fatal(err) }
	if ns.Resources.Cash != 45000 || len(ns.Campaign.Active) != 1 || !ns.Campaign.DirectiveUsed { t.Fatalf("state=%+v campaign=%+v", ns.Resources, ns.Campaign) }
}

func TestCounterDirectivePinsTelegraphedAction(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Campaign: model.CampaignState{Doctrine: model.DoctrineConsumer, Primary: model.RivalRoadmap{Company: "OpenAI", ActionIndex: 0}}}
	ns, err := Apply(s, model.IssueDirective{Kind: model.DirectiveCounter, Target: "OpenAI"}, b)
	if err != nil { t.Fatal(err) }
	if ns.Campaign.CounterTarget != "OpenAI" || ns.Campaign.CounterActionID != "openai-flagship" { t.Fatalf("campaign=%+v", ns.Campaign) }
}

func TestSecondDirectiveSameCycleRejected(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Campaign: model.CampaignState{Doctrine: model.DoctrineConsumer, DirectiveUsed: true}}
	_, err := Apply(s, model.IssueDirective{Kind: model.DirectiveIntel, Target: "OpenAI"}, b)
	if !errors.Is(err, ErrDirectiveUsed) { t.Fatalf("err=%v", err) }
}
```

- [ ] **Step 2: Run tests and verify red**

```bash
go test ./internal/sim -run 'TestRoutePush|TestCounterDirective|TestSecondDirective' -count=1
```

Expected: compile failure for missing directive dispatch/errors.

- [ ] **Step 3: Implement exact directive behavior**

Add errors:

```go
ErrDirectiveUsed = errors.New("sim: executive directive already used this cycle")
ErrInvalidDirective = errors.New("sim: invalid executive directive")
ErrInvalidRivalTarget = errors.New("sim: invalid rival target")
ErrRivalAlreadyCountered = errors.New("sim: rival action already countered")
```

Add `IssueDirective` dispatch and these rules:

- route push costs `max(5000, MonthlyRevenue(s)*0.25)`, adds a one-cycle modifier with route-segment growth ×1.20, then marks used;
- counter target must equal primary/wildcard, stores the exact currently telegraphed action ID, and cannot overwrite an existing counter;
- intel target must equal primary/wildcard and sets its `IntelFull=true`;
- no failed directive spends cash or marks used;
- matching counter halves quality/ref-price impact and is consumed; mismatched action cannot consume it;
- `AdvanceCampaignCycle` resets `DirectiveUsed` after settlement.

Implement with this switch:

```go
func applyIssueDirective(s model.GameState, c model.IssueDirective, b balance.Config) (model.GameState, error) {
	if s.Campaign.Doctrine == model.DoctrineNone { return s, ErrInvalidDirective }
	if s.Campaign.DirectiveUsed { return s, ErrDirectiveUsed }
	ns := s
	switch c.Kind {
	case model.DirectiveRoutePush:
		cost := math.Max(5000, MonthlyRevenue(s)*0.25)
		if s.Resources.Cash < cost { return s, ErrInsufficientCash }
		e := model.NeutralCampaignEffects()
		e.UserGrowthMult[doctrineSegment(s.Campaign.Doctrine)] = 1.20
		ns.Resources.Cash -= cost
		ns.Campaign.Active = append(append([]model.CampaignModifier(nil), s.Campaign.Active...), model.CampaignModifier{
			ID: fmt.Sprintf("directive-route-push-%d", s.Campaign.Cycle), CyclesRemaining: 1, Effects: e,
		})
	case model.DirectiveCounter:
		r, ok := campaignRoadmapByCompany(s, c.Target)
		if !ok { return s, ErrInvalidRivalTarget }
		if s.Campaign.CounterTarget != "" { return s, ErrRivalAlreadyCountered }
		actionID, ok := roadmapActionID(r, b)
		if !ok { return s, ErrInvalidRivalTarget }
		ns.Campaign.CounterTarget, ns.Campaign.CounterActionID = c.Target, actionID
	case model.DirectiveIntel:
		if c.Target == s.Campaign.Primary.Company { ns.Campaign.Primary.IntelFull = true
		} else if c.Target == s.Campaign.Wildcard.Company { ns.Campaign.Wildcard.IntelFull = true
		} else { return s, ErrInvalidRivalTarget }
	default:
		return s, ErrInvalidDirective
	}
	ns.Campaign.DirectiveUsed = true
	return ns, nil
}
```

- [ ] **Step 4: Run focused and full tests**

```bash
gofmt -w internal/sim/campaign_apply.go internal/sim/campaign_apply_test.go internal/sim/apply.go internal/sim/campaign_rivals.go internal/sim/campaign_cycle.go
go test ./internal/sim -run 'TestRoutePush|TestCounterDirective|TestSecondDirective|TestCampaignCycleResetsDirective' -count=1
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/sim/campaign_apply.go internal/sim/campaign_apply_test.go internal/sim/apply.go internal/sim/campaign_rivals.go internal/sim/campaign_cycle.go
git commit -m "feat(campaign): add executive directives"
```

---

### Task 7: Add Campaign Settlement, Route Badges, Legacy Draft, and Safe Exit

**Files:**
- Modify: `internal/sim/campaign_apply.go`
- Modify: `internal/sim/campaign_apply_test.go`
- Modify: `internal/sim/apply.go`
- Modify: `internal/sim/prestige.go`
- Modify: `internal/sim/prestige_test.go`
- Modify: `internal/tui/tui.go`

**Interfaces:**
- Produces: campaign Prestige, continue, exit, one-run Legacy consumption, and minimal Phase A financial-distress protection.
- Preserves old valuation-gated `PrestigeReset` only for no-doctrine saves.

- [ ] **Step 1: Write failing settlement tests**

```go
func TestCampaignPrestigeBanksBadgeAndLegacy(t *testing.T) {
	b := balance.Default()
	s := model.GameState{PeakValuation: 1e10}
	s.Campaign = model.CampaignState{Doctrine: model.DoctrineConsumer, Secondary: model.DoctrineDeveloper, SecondaryPerk: "developer-open", Stage: model.CampaignStageWon, Victory: model.DoctrineConsumer}
	ns, err := Apply(s, model.CampaignPrestige{Legacy: model.LegacyChoice{Kind: model.LegacySecondary, Doctrine: model.DoctrineDeveloper, PerkID: "developer-open"}}, b)
	if err != nil { t.Fatal(err) }
	if ns.Prestige.Patents != 10 || len(ns.Prestige.RouteBadges) != 1 { t.Fatalf("prestige=%+v", ns.Prestige) }
	if ns.Campaign.Legacy.Kind != model.LegacySecondary || ns.Prestige.PendingLegacy.Kind != model.LegacyNone { t.Fatalf("state=%+v", ns) }
}

func TestCampaignExitPaysHalfAndNoBadge(t *testing.T) {
	b := balance.Default()
	s := model.GameState{PeakValuation: 1e10, Campaign: model.CampaignState{Doctrine: model.DoctrineConsumer, Cycle: 18}}
	ns, err := Apply(s, model.CampaignExit{}, b)
	if err != nil { t.Fatal(err) }
	if ns.Prestige.Patents != 5 || len(ns.Prestige.RouteBadges) != 0 { t.Fatalf("prestige=%+v", ns.Prestige) }
}

func TestCampaignContinueKeepsRun(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Campaign: model.CampaignState{Doctrine: model.DoctrineConsumer, Victory: model.DoctrineConsumer}}
	ns, err := Apply(s, model.CampaignContinue{}, b)
	if err != nil || !ns.Campaign.Endless { t.Fatalf("err=%v campaign=%+v", err, ns.Campaign) }
}
```

- [ ] **Step 2: Run tests and verify red**

```bash
go test ./internal/sim -run 'TestCampaignPrestige|TestCampaignExit|TestCampaignContinue' -count=1
```

Expected: compile failure for missing command cases.

- [ ] **Step 3: Implement settlement validation**

Add errors:

```go
ErrCampaignNotWon = errors.New("sim: campaign has not been won")
ErrInvalidLegacy = errors.New("sim: invalid legacy choice")
ErrStrategyExitLocked = errors.New("sim: strategy exit not unlocked")
```

Implement:

- continue requires victory and sets `Endless=true`;
- Prestige requires victory; secondary Legacy doctrine and tier-one perk must match saved secondary/secondary perk, intel needs no payload, tech must be in `UnlockedTech`;
- Prestige awards full `patentsFor`, adds the victory doctrine once, stores pending Legacy, then calls `freshRun`;
- exit requires cycle ≥18 or distress ≥2, awards `floor(patentsFor*0.5)`, no badge/Legacy, then calls `freshRun`;
- `freshRun` copies pending Legacy into the new run's `Campaign.Legacy`, applies a tech Legacy, then clears pending;
- doctrine selection consumes secondary/intel Legacy and clears `Campaign.Legacy`;
- active-campaign `PrestigeReset` returns `ErrCampaignNotWon`; pre-campaign saves keep the current valuation gate.

Use these core settlement functions:

```go
func addDoctrineUnique(in []model.Doctrine, d model.Doctrine) []model.Doctrine {
	for _, x := range in { if x == d { return in } }
	return append(append([]model.Doctrine(nil), in...), d)
}

func applyCampaignContinue(s model.GameState) (model.GameState, error) {
	if s.Campaign.Victory == model.DoctrineNone { return s, ErrCampaignNotWon }
	ns := s
	ns.Campaign.Endless = true
	return ns, nil
}

func applyCampaignExit(s model.GameState, b balance.Config) (model.GameState, error) {
	if s.Campaign.Cycle < b.Campaign.StrategyExitCycle && s.Campaign.FinancialDistressCycles < 2 { return s, ErrStrategyExitLocked }
	p := s.Prestige
	p.Patents += math.Floor(patentsFor(s.PeakValuation, b) * 0.5)
	p.PendingLegacy = model.LegacyChoice{}
	ns := freshRun(p, b)
	ns.Events.RandState, ns.Campaign.RandState = s.Events.RandState, s.Campaign.RandState
	return ns, nil
}

func applyCampaignPrestige(s model.GameState, c model.CampaignPrestige, b balance.Config) (model.GameState, error) {
	if s.Campaign.Victory == model.DoctrineNone { return s, ErrCampaignNotWon }
	if err := validateLegacy(s, c.Legacy); err != nil { return s, err }
	p := s.Prestige
	p.Patents += patentsFor(s.PeakValuation, b)
	p.RouteBadges = addDoctrineUnique(p.RouteBadges, s.Campaign.Victory)
	p.PendingLegacy = c.Legacy
	ns := freshRun(p, b)
	ns.Events.RandState, ns.Campaign.RandState = s.Events.RandState, s.Campaign.RandState
	return ns, nil
}
```

`validateLegacy` uses the three validation bullets above and rejects `LegacyNone`. `freshRun` must clear `PendingLegacy` after copying/applying it, so repeating a restart cannot apply the same Legacy twice.

- [ ] **Step 4: Disable automatic bankruptcy reset for active campaigns**

Change the current debt reset guard to:

```go
if m.state.Campaign.Doctrine == model.DoctrineNone &&
	m.state.Resources.Cash < -m.cfg.BankruptcyDebtRatio*m.cfg.StartingCash {
	m.state = sim.Restart(m.state, m.cfg)
	m.setNotice("💥 破產！公司已重整重來")
	m.snapDisplay()
}
```

Active campaigns use `FinancialDistressCycles`; the player can recover operationally or exit after two distressed cycles. Phase C later adds restructuring options.

- [ ] **Step 5: Run focused and full tests**

```bash
gofmt -w internal/sim/campaign_apply.go internal/sim/campaign_apply_test.go internal/sim/apply.go internal/sim/prestige.go internal/sim/prestige_test.go internal/tui/tui.go
go test ./internal/sim ./internal/tui -run 'TestCampaignPrestige|TestCampaignExit|TestCampaignContinue|TestRestart|TestFreshRun|TestBankruptcy' -count=1
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/sim/campaign_apply.go internal/sim/campaign_apply_test.go internal/sim/apply.go internal/sim/prestige.go internal/sim/prestige_test.go internal/tui/tui.go
git commit -m "feat(campaign): add victory settlement and legacy"
```

---

### Task 8: Wire Offline and Live Board-Cycle Catch-Up

**Files:**
- Modify: `internal/tui/campaign_clock.go`
- Modify: `internal/tui/campaign_clock_test.go`
- Modify: `internal/tui/tui.go`
- Modify: `internal/tui/daemon_integration_test.go`
- Modify: `internal/tui/settle.go`

**Interfaces:**
- Produces: `Model.advanceCampaignTo(now int64) (Model, int)` and campaign-cycle count in offline summary.
- Consumes: Task 2 clock helper and Task 5 `sim.AdvanceCampaignCycle`.

- [ ] **Step 1: Write failing catch-up tests**

```go
func TestAdvanceCampaignToCapsAtThreeCycles(t *testing.T) {
	m := testModel(t)
	m.state.Campaign.Doctrine = model.DoctrineConsumer
	m.state.Campaign.Stage = model.CampaignStageExpand
	m.lastCampaignUnix = 100
	nm, advanced := m.advanceCampaignTo(100 + 7*24*60*60)
	if advanced != 3 || nm.state.Campaign.Cycle != 3 || nm.lastCampaignUnix != 100+7*24*60*60 {
		t.Fatalf("advanced=%d cycle=%d last=%d", advanced, nm.state.Campaign.Cycle, nm.lastCampaignUnix)
	}
}

func TestAdvanceCampaignToDoesNothingBeforeDoctrine(t *testing.T) {
	m := testModel(t)
	m.lastCampaignUnix = 100
	nm, advanced := m.advanceCampaignTo(100 + 24*60*60)
	if advanced != 0 || nm.state.Campaign.Cycle != 0 { t.Fatalf("advanced=%d campaign=%+v", advanced, nm.state.Campaign) }
}
```

Extend daemon and standalone startup tests with an active campaign and stale `LastCampaignUnix`; both paths must advance campaign cycles. This specifically guards the current standalone early return.

- [ ] **Step 2: Run tests and verify red**

```bash
go test ./internal/tui -run 'TestAdvanceCampaignTo|TestStartup' -count=1
```

Expected: compile failure for missing method.

- [ ] **Step 3: Implement catch-up outside the sim**

Add:

```go
func (m Model) advanceCampaignTo(now int64) (Model, int) {
	if m.state.Campaign.Doctrine == model.DoctrineNone {
		if m.lastCampaignUnix == 0 { m.lastCampaignUnix = now }
		return m, 0
	}
	due, next := campaignCyclesDue(m.lastCampaignUnix, now, m.cfg.Campaign.CycleSec, m.cfg.Campaign.MaxCatchupCycles)
	for i := 0; i < due; i++ { m.state = sim.AdvanceCampaignCycle(m.state, m.cfg) }
	m.lastCampaignUnix = next
	return m, due
}
```

Refactor `startup(now)` so daemon economic settlement remains conditional but `advanceCampaignTo(now)` always executes before return. On live `tickMsg`, call it after `sim.Tick`. When primary doctrine selection succeeds, initialize `lastCampaignUnix` to `time.Now().Unix()` and save meta.

- [ ] **Step 4: Extend offline summary**

Add `CampaignCycles int` to `tui.Summary`. Append:

```go
if s.CampaignCycles > 0 {
	msg += fmt.Sprintf(" · 董事會週期 %d 次", s.CampaignCycles)
}
```

The latest Board Report remains the detailed source; the banner only gives the count.

- [ ] **Step 5: Run focused and full tests**

```bash
gofmt -w internal/tui/campaign_clock.go internal/tui/campaign_clock_test.go internal/tui/tui.go internal/tui/daemon_integration_test.go internal/tui/settle.go
go test ./internal/tui -run 'TestAdvanceCampaignTo|TestStartup|TestOfflineBanner' -count=1
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/campaign_clock.go internal/tui/campaign_clock_test.go internal/tui/tui.go internal/tui/daemon_integration_test.go internal/tui/settle.go
git commit -m "feat(campaign): settle board cycles online and offline"
```

---

### Task 9: Render the CEO War Room and Rival Intelligence

**Files:**
- Create: `internal/tui/campaign_meta.go`
- Create: `internal/tui/campaign_meta_test.go`
- Modify: `internal/tui/page_overview.go`
- Modify: `internal/tui/page_overview_test.go`
- Modify: `internal/tui/tui.go`
- Modify: `internal/tui/pressure_test.go`

**Interfaces:**
- Produces: read-only campaign, roadmap, board-report, and distress cards.
- Consumes: `sim.CampaignStatus`, roadmap/report IDs, and current layout helpers.

- [ ] **Step 1: Write failing overview tests**

```go
func TestOverviewShowsCampaignWarRoom(t *testing.T) {
	m := testModel(t)
	m.state.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand, Cycle: 4,
		Primary: model.RivalRoadmap{Company: "OpenAI", ActionIndex: 0, CyclesUntilAction: 1},
		Wildcard: model.RivalRoadmap{Company: "DeepSeek", ActionIndex: 0, CyclesUntilAction: 2},
		Reports: []model.BoardReport{{Cycle: 4, Entries: []model.CampaignReportEntry{{Kind: model.ReportRivalAction, SubjectID: "OpenAI", DetailID: "openai-flagship"}}}},
	}
	v := renderOverview(m)
	for _, want := range []string{"主要戰略", "消費者霸主", "OpenAI", "下一步", "董事會報告"} {
		if !strings.Contains(v, want) { t.Fatalf("missing %q:\n%s", want, v) }
	}
}
```

Also assert a pre-campaign overview displays `第一個模型上線後可選公司戰略`.

- [ ] **Step 2: Run tests and verify red**

```bash
go test ./internal/tui -run 'TestOverviewShowsCampaign|TestCampaignMeta' -count=1
```

Expected: FAIL for missing labels/cards.

- [ ] **Step 3: Add total ID-to-copy mappings**

Create `campaign_meta.go` with mappings for:

- doctrines: 消費者霸主／企業信任／開發者生態;
- stages: 立足／擴張／決勝／已勝利;
- all twelve perk IDs from Task 1;
- all fourteen rival action IDs;
- all six report kinds;
- directives: 市場衝刺／反制宿敵／深度情報.

Unknown IDs return the original ID. Tests must enumerate every catalog ID and reject empty copy.

Use explicit maps:

```go
var doctrineLabels = map[model.Doctrine]string{
	model.DoctrineConsumer: "消費者霸主",
	model.DoctrineEnterprise: "企業信任",
	model.DoctrineDeveloper: "開發者生態",
}

var campaignStageLabels = map[model.CampaignStage]string{
	model.CampaignStageEstablish: "立足",
	model.CampaignStageExpand: "擴張",
	model.CampaignStageShowdown: "決勝",
	model.CampaignStageWon: "已勝利",
}

var rivalActionLabels = map[string]string{
	"openai-flagship": "OpenAI 消費旗艦",
	"openai-platform": "OpenAI 平台攻勢",
	"anthropic-trust": "Anthropic 企業信任攻勢",
	"anthropic-enterprise-suite": "Anthropic 企業套件",
	"xai-scale": "xAI 暴力擴張",
	"xai-compute-rush": "xAI 算力突進",
	"deepseek-price-war": "DeepSeek 開源價格戰",
	"deepseek-distill": "DeepSeek 蒸餾突破",
	"qwen-ecosystem": "Qwen 開發者生態攻勢",
	"qwen-release-wave": "Qwen 模型機海",
	"zhipu-enterprise": "Zhipu 企業市場攻勢",
	"zhipu-contract": "Zhipu 大單攻勢",
	"gemini-balanced": "Gemini 全面發表",
	"gemini-multimodal": "Gemini 多模態發表",
}
```

Define the twelve perk labels from their catalog IDs, and implement one `labelOrID(map[string]string,id string) string` helper used by action/perk/report copy.

- [ ] **Step 4: Add campaign cards before existing KPI rows**

Implement:

```go
func renderCampaignStatusCard(m Model) string
func renderRivalRoadmapCard(m Model) string
func renderBoardReportCard(m Model) string
```

Use `sim.CampaignStatus` and `sim.CampaignRivalIntel` only. The status card body is:

```go
status := sim.CampaignStatus(m.state, m.cfg)
if !status.Active {
	return Card("公司戰略", "第一個模型上線後可選公司戰略")
}
body := VStack(
	KV("主要戰略", doctrineLabel(status.Doctrine)),
	KV("階段", campaignStageLabel(status.Stage)),
	KV("董事會週期", fmt.Sprintf("%d", m.state.Campaign.Cycle)),
	fmt.Sprintf("下一目標 %s %.0f%%", Bar(status.Progress, 12), status.Progress*100),
)
return Card("公司戰略", body)
```

The roadmap card always prints the rumored next action label for both roles. `IntelFull` additionally prints its exact quality percentages, price modifier, and lead cycles; without full intel it prints only direction and target segment. The report card renders the latest report's newest four entries by report kind and IDs.

Required content:

- doctrine, stage, cycle, next-gate progress bar, perks, pending perk;
- confirmed primary action/countdown and rumored next action; wildcard below;
- newest four report entries;
- financial-distress warning;
- first-model/no-doctrine call to action.

When `Campaign.Endless` is true, append two compact lines from `sim.RouteVictoryStatus` for the non-primary doctrines so “continue” has visible optional goals while Roadmaps keep advancing.

Update `pressures(m)` without removing existing warnings.

- [ ] **Step 5: Update overview help without adding a seventh page**

Append `[c]公司策略 [d]高層指令`; show `[P]勝利結算` only after victory and `[E]策略退出` only at cycle 18 or two distress cycles. Retain existing `t`, `X`, and event `e` actions.

- [ ] **Step 6: Run focused and full tests**

```bash
gofmt -w internal/tui/campaign_meta.go internal/tui/campaign_meta_test.go internal/tui/page_overview.go internal/tui/page_overview_test.go internal/tui/tui.go internal/tui/pressure_test.go
go test ./internal/tui -run 'TestOverview|TestCampaignMeta|TestPressure|TestModelResponsiveLayout' -count=1
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/campaign_meta.go internal/tui/campaign_meta_test.go internal/tui/page_overview.go internal/tui/page_overview_test.go internal/tui/tui.go internal/tui/pressure_test.go
git commit -m "feat(tui): add campaign ceo war room"
```

---

### Task 10: Add Doctrine, Directive, and Campaign-End Dialogs

**Files:**
- Create: `internal/tui/dialog_doctrine.go`
- Create: `internal/tui/dialog_doctrine_test.go`
- Create: `internal/tui/dialog_directive.go`
- Create: `internal/tui/dialog_directive_test.go`
- Create: `internal/tui/dialog_campaign_end.go`
- Create: `internal/tui/dialog_campaign_end_test.go`
- Modify: `internal/tui/tui.go`

**Interfaces:**
- Produces: TUI access to every Phase A command.
- Consumes: typed sim errors and campaign metadata.
- Preserves current dialog priority and fixed-footer behavior.

- [ ] **Step 1: Write failing dialog state-machine tests**

Cover:

1. `c` after first online model chooses primary doctrine.
2. `c` with `PerkTierPending=1/2` shows exactly the two matching perks.
3. `c` in showdown with no secondary offers exactly two non-primary doctrines.
4. uppercase `C` requires pivot confirmation and never applies on first key.
5. `d` offers route push/counter/intel; counter/intel ask for rival target.
6. a rejected command keeps dialog open and renders the error.
7. `P` after victory offers continue and only valid Legacy choices.
8. `E` opens only after cycle 18 or two distress cycles.

Use Bubble Tea key messages and rendering assertions like existing event/publish dialog tests.

- [ ] **Step 2: Run tests and verify red**

```bash
go test ./internal/tui -run 'TestDoctrineDialog|TestDirectiveDialog|TestCampaignEndDialog|TestCampaignKeys' -count=1
```

Expected: compile failure for missing dialog types/fields.

- [ ] **Step 3: Implement three focused dialog models**

Each file owns a value type with `cursor`, `update`, `render`, and `command`. Add to `tui.Model`:

```go
doctrineDialog *doctrineDialog
directiveDialog *directiveDialog
campaignEnd *campaignEndDialog
campaignError string
```

Use these exact dialog interfaces:

```go
type doctrineDialogMode int
const (
	doctrineChoosePrimary doctrineDialogMode = iota
	doctrineChoosePerk
	doctrineChooseSecondary
	doctrineConfirmPivot
)
type doctrineDialog struct { mode doctrineDialogMode; options []string; cursor int }
func newDoctrineDialog(m Model, pivot bool) (doctrineDialog, bool)
func (d doctrineDialog) update(msg tea.KeyMsg) (doctrineDialog, bool, bool)
func (d doctrineDialog) command(m Model) model.Command
func renderDoctrineDialog(d doctrineDialog, m Model) string

type directiveDialog struct { options []model.DirectiveKind; cursor int; choosingTarget bool; targetCursor int }
func newDirectiveDialog(m Model) (directiveDialog, bool)
func (d directiveDialog) update(msg tea.KeyMsg) (directiveDialog, bool, bool)
func (d directiveDialog) command(m Model) model.IssueDirective
func renderDirectiveDialog(d directiveDialog, m Model) string

type campaignEndMode int
const (campaignEndVictory campaignEndMode = iota; campaignEndExit)
type campaignEndDialog struct {
	mode campaignEndMode
	options []model.LegacyChoice
	cursor int
	continueRun bool
	choosingTech bool
	techOptions []string
	techCursor int
}
func newCampaignEndDialog(m Model, mode campaignEndMode) (campaignEndDialog, bool)
func (d campaignEndDialog) update(msg tea.KeyMsg) (campaignEndDialog, bool, bool)
func (d campaignEndDialog) command() model.Command
func renderCampaignEndDialog(d campaignEndDialog, m Model) string
```

Arrow keys move within bounds, Enter confirms, Esc cancels. `command` returns only commands defined in Task 1; it never mutates `Model` directly.

Command mapping is exact:

```go
func (d doctrineDialog) command(m Model) model.Command {
	choice := d.options[d.cursor]
	switch d.mode {
	case doctrineChoosePrimary: return model.ChooseDoctrine{Doctrine: model.Doctrine(choice)}
	case doctrineChoosePerk: return model.ChooseDoctrinePerk{PerkID: choice}
	case doctrineChooseSecondary:
		p, _ := balance.CampaignPerkByID(m.cfg.Campaign, choice)
		return model.ChooseSecondaryDoctrine{Doctrine: p.Doctrine, PerkID: p.ID}
	default: return model.PivotDoctrine{Doctrine: model.Doctrine(choice)}
	}
}

func (d campaignEndDialog) command() model.Command {
	if d.mode == campaignEndExit { return model.CampaignExit{} }
	if d.continueRun { return model.CampaignContinue{} }
	choice := d.options[d.cursor]
	if choice.Kind == model.LegacyTech { choice.TechID = d.techOptions[d.techCursor] }
	return model.CampaignPrestige{Legacy: choice}
}
```

Selecting `LegacyTech` enters the nested `techOptions` list and requires a second Enter; Esc returns to Legacy choices. Do not silently choose the first or last technology.

Route these before existing event/publish/train dialogs. Treat any open campaign dialog as a shell-navigation lock.

- [ ] **Step 4: Map typed errors and keep them visible**

```go
func campaignErrorText(err error) string {
	switch {
	case errors.Is(err, sim.ErrInsufficientCash): return "現金不足"
	case errors.Is(err, sim.ErrInsufficientRnD): return "R&D 不足"
	case errors.Is(err, sim.ErrDirectiveUsed): return "本週期已使用高層指令"
	case errors.Is(err, sim.ErrPerkChoiceNotReady): return "目前沒有可選的流派能力"
	case errors.Is(err, sim.ErrPivotLocked): return "決勝階段不可轉型"
	case errors.Is(err, sim.ErrStrategyExitLocked): return "第 18 週期後才能策略退出"
	default: return "此策略目前無法執行"
	}
}
```

Clear `campaignError` only on success, Esc, or a new campaign selection. Tick must not clear it.

- [ ] **Step 5: Initialize campaign clock after doctrine selection**

After successful `ChooseDoctrine`, set `lastCampaignUnix=time.Now().Unix()` and save meta. Do not backdate from save creation.

- [ ] **Step 6: Run focused and full tests**

```bash
gofmt -w internal/tui/dialog_doctrine.go internal/tui/dialog_doctrine_test.go internal/tui/dialog_directive.go internal/tui/dialog_directive_test.go internal/tui/dialog_campaign_end.go internal/tui/dialog_campaign_end_test.go internal/tui/tui.go
go test ./internal/tui -run 'TestDoctrineDialog|TestDirectiveDialog|TestCampaignEndDialog|TestCampaignKeys|TestDialogFooter' -count=1
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/dialog_doctrine.go internal/tui/dialog_doctrine_test.go internal/tui/dialog_directive.go internal/tui/dialog_directive_test.go internal/tui/dialog_campaign_end.go internal/tui/dialog_campaign_end_test.go internal/tui/tui.go
git commit -m "feat(tui): add strategic campaign dialogs"
```

---

### Task 11: Lock In Campaign Invariants and Balance Scenarios

**Files:**
- Create: `internal/sim/campaign_scenario_test.go`
- Create: `internal/sim/campaign_invariant_test.go`
- Modify: `internal/tui/daemon_integration_test.go`
- Modify: `internal/store/store_test.go`

**Interfaces:**
- Consumes all Phase A interfaces.
- Produces regression coverage for determinism, reachability, save compatibility, and catch-up.

- [ ] **Step 1: Add three route scenario fixtures**

Create fixed-seed tests:

```go
func TestConsumerCampaignWinsWithinTargetCycles(t *testing.T)
func TestEnterpriseCampaignWinsWithinTargetCycles(t *testing.T)
func TestDeveloperCampaignWinsWithinTargetCycles(t *testing.T)
```

Use seeds `101`, `202`, `303`. Each fixture selects doctrine/perks/secondary through `Apply`, advances route stages, sets route-specific model/price/capacity conditions, advances board cycles, and asserts victory occurs between cycles 9 and 21 inclusive. Log `CampaignStatus` each failed cycle.

Use this concrete scenario helper:

```go
func winningCampaignFixture(d model.Doctrine, seed uint64, b balance.Config) model.GameState {
	seg := doctrineSegment(d)
	price := b.SegmentRefPrice[seg]
	if d == model.DoctrineDeveloper { price *= 0.85 }
	s := model.GameState{
		Resources: model.Resources{Cash: 1e7, RnD: 1e7},
		Models: []model.Model{{Online: true, Segment: seg, Price: price, Users: 100000, Quality: [4]float64{100, 100, 100, 100}}},
		Competitors: balance.DefaultCompetitors(),
		Ops: 2,
	}
	s.Compute.RentedInference = map[string]int{balance.EntryProcessID: 1000}
	s.Campaign.RandState = seed
	s, err := Apply(s, model.ChooseDoctrine{Doctrine: d}, b)
	if err != nil { panic(err) }
	s.Campaign.Stage = model.CampaignStageShowdown
	s.Campaign.Cycle = 8
	perks1 := balance.PerksFor(b.Campaign, d, 1)
	perks2 := balance.PerksFor(b.Campaign, d, 2)
	s.Campaign.Perks = []string{perks1[0].ID, perks2[0].ID}
	s.Campaign.ShowdownStartedCycle = 8
	s.Campaign.Primary.CyclesUntilAction = 1
	return s
}

func assertWinsWithinTarget(t *testing.T, d model.Doctrine, seed uint64) {
	t.Helper()
	b := balance.Default()
	s := winningCampaignFixture(d, seed, b)
	for s.Campaign.Cycle < 21 && s.Campaign.Victory == model.DoctrineNone { s = AdvanceCampaignCycle(s, b) }
	if s.Campaign.Victory != d || s.Campaign.Cycle < 9 || s.Campaign.Cycle > 21 {
		t.Fatalf("doctrine=%s cycle=%d victory=%s status=%+v", d, s.Campaign.Cycle, s.Campaign.Victory, CampaignStatus(s, b))
	}
}

func TestConsumerCampaignWinsWithinTargetCycles(t *testing.T) { assertWinsWithinTarget(t, model.DoctrineConsumer, 101) }
func TestEnterpriseCampaignWinsWithinTargetCycles(t *testing.T) { assertWinsWithinTarget(t, model.DoctrineEnterprise, 202) }
func TestDeveloperCampaignWinsWithinTargetCycles(t *testing.T) { assertWinsWithinTarget(t, model.DoctrineDeveloper, 303) }
```

- [ ] **Step 2: Add deterministic and invariant loops**

For seeds `1..100`, advance two identical states for 21 cycles and assert `reflect.DeepEqual` after every cycle. Assert:

- reports ≤20;
- primary and wildcard differ;
- pending perk tier is 0, 1, or 2;
- directive resets each cycle;
- roadmap countdown is positive after execution;
- route badges remain unique;
- exit and Prestige cannot both reward one run.

The deterministic loop is:

```go
func TestCampaignDeterministicAcrossSeeds(t *testing.T) {
	b := balance.Default()
	for seed := uint64(1); seed <= 100; seed++ {
		a := winningCampaignFixture(model.DoctrineConsumer, seed, b)
		c := a
		for cycle := 0; cycle < 21; cycle++ {
			a = AdvanceCampaignCycle(a, b)
			c = AdvanceCampaignCycle(c, b)
			if !reflect.DeepEqual(a, c) { t.Fatalf("seed=%d cycle=%d diverged", seed, cycle) }
			if len(a.Campaign.Reports) > b.Campaign.ReportCap { t.Fatalf("seed=%d reports=%d", seed, len(a.Campaign.Reports)) }
			if a.Campaign.Primary.Company == a.Campaign.Wildcard.Company { t.Fatalf("seed=%d duplicate rivals", seed) }
			if a.Campaign.PerkTierPending < 0 || a.Campaign.PerkTierPending > 2 { t.Fatalf("seed=%d pending=%d", seed, a.Campaign.PerkTierPending) }
		}
	}
}
```

Add separate focused tests for route-badge uniqueness and double settlement: call the same end command twice and assert the second returns a typed error without changing patents.

- [ ] **Step 3: Add old-save and offline regressions**

- Load literal pre-campaign JSON containing only resources; assert preserved resources and zero campaign.
- Round-trip active roadmaps, perks, reports, badge, and Legacy.
- Start fresh-ledger and standalone TUI after 72 hours; assert exactly three cycles.
- Assert campaign catch-up synthesizes no command and performs no campaign spending.

- [ ] **Step 4: Run the full verification matrix**

```bash
gofmt -w internal/sim/campaign_scenario_test.go internal/sim/campaign_invariant_test.go internal/tui/daemon_integration_test.go internal/store/store_test.go
go test ./internal/sim -run 'TestConsumerCampaign|TestEnterpriseCampaign|TestDeveloperCampaign|TestCampaignDeterministic|TestCampaignInvariants' -count=1
go test ./internal/tui ./internal/store -run 'TestStartup|TestSaveLoad|TestOldSave|TestCampaign' -count=1
go test ./... -count=1
go vet ./...
go build ./...
git diff --check
```

Expected: all commands exit `0`; diff check prints nothing.

- [ ] **Step 5: Smoke-test with an isolated config directory**

```bash
tmp=$(mktemp -d)
XDG_CONFIG_HOME="$tmp" go run .
```

Verify: pre-model overview works; publish enables doctrine selection; selection shows two roadmaps; directive enforces one-per-cycle; strategic errors persist; save/relaunch preserves campaign.

- [ ] **Step 6: Commit**

```bash
git add internal/sim/campaign_scenario_test.go internal/sim/campaign_invariant_test.go internal/tui/daemon_integration_test.go internal/store/store_test.go
git commit -m "test(campaign): cover phase-a scenarios and invariants"
```

---

## Final Acceptance Gate

- `go test ./... -count=1` passes.
- `go vet ./...` passes.
- `go build ./...` passes.
- `git diff --check` is empty.
- Old saves load without resource reset.
- Long absence advances at most three campaign cycles.
- Every rival action is visible at least one cycle before execution.
- All three victories are reachable; one run cannot pay rewards twice.
- Every Phase A command is reachable from TUI and rejected commands explain why.
- No Phase B event chain or Phase C pipeline behavior is present.
