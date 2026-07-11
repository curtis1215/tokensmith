package sim

import (
	"errors"
	"reflect"
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestChooseDoctrineRequiresOnlineModel(t *testing.T) {
	_, err := Apply(model.GameState{}, model.ChooseDoctrine{Doctrine: model.DoctrineConsumer}, balance.Default())
	if !errors.Is(err, ErrCampaignNeedsModel) {
		t.Fatalf("err=%v", err)
	}
}

func TestChooseDoctrineStartsEstablishStage(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Models: []model.Model{{Online: true}}}
	s.Campaign.RandState = 1
	ns, err := Apply(s, model.ChooseDoctrine{Doctrine: model.DoctrineConsumer}, b)
	if err != nil {
		t.Fatal(err)
	}
	if ns.Campaign.Doctrine != model.DoctrineConsumer || ns.Campaign.Stage != model.CampaignStageEstablish {
		t.Fatalf("campaign=%+v", ns.Campaign)
	}
}

func TestChoosePerkValidatesTierAndDoctrine(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Campaign: model.CampaignState{Doctrine: model.DoctrineConsumer, PerkTierPending: 1}}
	ns, err := Apply(s, model.ChooseDoctrinePerk{PerkID: "consumer-premium"}, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(ns.Campaign.Perks) != 1 || ns.Campaign.PerkTierPending != 0 {
		t.Fatalf("campaign=%+v", ns.Campaign)
	}
}

func TestChooseSecondaryIncludesOneTierOnePerk(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Campaign: model.CampaignState{Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageShowdown}}
	ns, err := Apply(s, model.ChooseSecondaryDoctrine{Doctrine: model.DoctrineDeveloper, PerkID: "developer-open"}, b)
	if err != nil {
		t.Fatal(err)
	}
	if ns.Campaign.Secondary != model.DoctrineDeveloper || ns.Campaign.SecondaryPerk != "developer-open" {
		t.Fatalf("campaign=%+v", ns.Campaign)
	}
}

func TestChooseSecondaryRejectsReplacement(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Campaign: model.CampaignState{
		Doctrine:      model.DoctrineConsumer,
		Stage:         model.CampaignStageShowdown,
		Secondary:     model.DoctrineDeveloper,
		SecondaryPerk: "developer-open",
	}}

	ns, err := Apply(s, model.ChooseSecondaryDoctrine{
		Doctrine: model.DoctrineEnterprise,
		PerkID:   "enterprise-compliance",
	}, b)
	if !errors.Is(err, ErrSecondaryNotReady) {
		t.Fatalf("err=%v, want ErrSecondaryNotReady", err)
	}
	if !reflect.DeepEqual(ns, s) {
		t.Fatalf("rejected replacement mutated state:\n got=%+v\nwant=%+v", ns, s)
	}
}

func TestPivotChargesAndResetsBuild(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Resources: model.Resources{Cash: 100000, RnD: 50000}}
	s.Models = []model.Model{{Online: true, Users: 1000, Price: 12}}
	s.Campaign = model.CampaignState{Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand, Perks: []string{"consumer-premium"}}
	ns, err := Apply(s, model.PivotDoctrine{Doctrine: model.DoctrineEnterprise}, b)
	if err != nil {
		t.Fatal(err)
	}
	if ns.Campaign.Doctrine != model.DoctrineEnterprise || !ns.Campaign.PivotUsed || len(ns.Campaign.Perks) != 0 {
		t.Fatalf("campaign=%+v", ns.Campaign)
	}
	if ns.Resources.Cash != 80000 || ns.Resources.RnD != 45000 {
		t.Fatalf("resources=%+v", ns.Resources)
	}
}

// TestPivotClearsStaleCounterPin: counter on old primary → pivot → pin gone;
// Active and DirectiveUsed preserved; after cycle reset, counter on new primary works.
func TestPivotClearsStaleCounterPin(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Resources: model.Resources{Cash: 100000, RnD: 50000}}
	s.Models = []model.Model{{Online: true, Users: 1000, Price: 12}}
	s.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer,
		Stage:    model.CampaignStageExpand,
		Primary:  model.RivalRoadmap{Company: "OpenAI", ActionIndex: 0, CyclesUntilAction: 2},
		Active: []model.CampaignModifier{{
			ID: "fixture-route-push", CyclesRemaining: 1, Effects: model.NeutralCampaignEffects(),
		}},
	}
	// Issue counter against old primary → pins OpenAI/openai-flagship and spends the cycle directive.
	pinned, err := Apply(s, model.IssueDirective{Kind: model.DirectiveCounter, Target: "OpenAI"}, b)
	if err != nil {
		t.Fatal(err)
	}
	if pinned.Campaign.CounterTarget != "OpenAI" || pinned.Campaign.CounterActionID != "openai-flagship" {
		t.Fatalf("expected counter pin, got %+v", pinned.Campaign)
	}
	if !pinned.Campaign.DirectiveUsed {
		t.Fatal("counter should spend DirectiveUsed")
	}
	if len(pinned.Campaign.Active) != 1 {
		t.Fatalf("fixture Active should remain after counter: %+v", pinned.Campaign.Active)
	}

	// Pivot to enterprise re-seeds roadmaps; must drop the stale pin.
	pivoted, err := Apply(pinned, model.PivotDoctrine{Doctrine: model.DoctrineEnterprise}, b)
	if err != nil {
		t.Fatal(err)
	}
	if pivoted.Campaign.CounterTarget != "" || pivoted.Campaign.CounterActionID != "" {
		t.Fatalf("pivot must clear counter pin: target=%q action=%q",
			pivoted.Campaign.CounterTarget, pivoted.Campaign.CounterActionID)
	}
	if !pivoted.Campaign.DirectiveUsed {
		t.Fatal("pivot must not clear DirectiveUsed (one-directive-per-cycle spans pivot)")
	}
	if len(pivoted.Campaign.Active) != 1 || pivoted.Campaign.Active[0].ID != "fixture-route-push" {
		t.Fatalf("pivot must not clear Active modifiers: %+v", pivoted.Campaign.Active)
	}
	if pivoted.Campaign.Primary.Company == "" || pivoted.Campaign.Primary.Company == "OpenAI" {
		t.Fatalf("pivot should re-seed enterprise primary, got %+v", pivoted.Campaign.Primary)
	}

	// Per-cycle reset clears DirectiveUsed so a fresh counter can be issued.
	reset := AdvanceCampaignCycle(pivoted, b)
	if reset.Campaign.DirectiveUsed {
		t.Fatal("cycle should reset DirectiveUsed")
	}
	newPrimary := reset.Campaign.Primary.Company
	if newPrimary == "" {
		t.Fatal("expected primary after pivot cycle")
	}
	countered, err := Apply(reset, model.IssueDirective{Kind: model.DirectiveCounter, Target: newPrimary}, b)
	if err != nil {
		t.Fatalf("counter on new primary after pivot must succeed: %v", err)
	}
	if countered.Campaign.CounterTarget != newPrimary || countered.Campaign.CounterActionID == "" {
		t.Fatalf("new counter pin missing: %+v", countered.Campaign)
	}
}

func TestRoutePushCostsCashAndAddsModifier(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Resources: model.Resources{Cash: 50000}}
	s.Campaign = model.CampaignState{Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand}
	ns, err := Apply(s, model.IssueDirective{Kind: model.DirectiveRoutePush}, b)
	if err != nil {
		t.Fatal(err)
	}
	if ns.Resources.Cash != 45000 || len(ns.Campaign.Active) != 1 || !ns.Campaign.DirectiveUsed {
		t.Fatalf("state=%+v campaign=%+v", ns.Resources, ns.Campaign)
	}
}

func TestCounterDirectivePinsTelegraphedAction(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Campaign: model.CampaignState{Doctrine: model.DoctrineConsumer, Primary: model.RivalRoadmap{Company: "OpenAI", ActionIndex: 0}}}
	ns, err := Apply(s, model.IssueDirective{Kind: model.DirectiveCounter, Target: "OpenAI"}, b)
	if err != nil {
		t.Fatal(err)
	}
	if ns.Campaign.CounterTarget != "OpenAI" || ns.Campaign.CounterActionID != "openai-flagship" {
		t.Fatalf("campaign=%+v", ns.Campaign)
	}
}

func TestSecondDirectiveSameCycleRejected(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Campaign: model.CampaignState{Doctrine: model.DoctrineConsumer, DirectiveUsed: true}}
	_, err := Apply(s, model.IssueDirective{Kind: model.DirectiveIntel, Target: "OpenAI"}, b)
	if !errors.Is(err, ErrDirectiveUsed) {
		t.Fatalf("err=%v", err)
	}
}

func TestCampaignCycleResetsDirective(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Campaign: model.CampaignState{
		Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand, DirectiveUsed: true,
	}}
	ns := AdvanceCampaignCycle(s, b)
	if ns.Campaign.DirectiveUsed {
		t.Fatalf("DirectiveUsed should reset after cycle: %+v", ns.Campaign)
	}
}

func TestFailedDirectivePreservesState(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Resources: model.Resources{Cash: 1000}}
	s.Campaign = model.CampaignState{Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand}
	ns, err := Apply(s, model.IssueDirective{Kind: model.DirectiveRoutePush}, b)
	if !errors.Is(err, ErrInsufficientCash) {
		t.Fatalf("err=%v", err)
	}
	if ns.Resources.Cash != 1000 || ns.Campaign.DirectiveUsed || len(ns.Campaign.Active) != 0 {
		t.Fatalf("failed directive must not mutate: cash=%v campaign=%+v", ns.Resources.Cash, ns.Campaign)
	}
}

func TestIntelDirectiveSetsIntelFull(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Campaign: model.CampaignState{
		Doctrine: model.DoctrineConsumer,
		Primary:  model.RivalRoadmap{Company: "OpenAI", ActionIndex: 0},
		Wildcard: model.RivalRoadmap{Company: "DeepSeek", ActionIndex: 0},
	}}
	ns, err := Apply(s, model.IssueDirective{Kind: model.DirectiveIntel, Target: "OpenAI"}, b)
	if err != nil {
		t.Fatal(err)
	}
	if !ns.Campaign.Primary.IntelFull || ns.Campaign.Wildcard.IntelFull || !ns.Campaign.DirectiveUsed {
		t.Fatalf("campaign=%+v", ns.Campaign)
	}
}

func TestMatchingCounterHalvesImpactAndConsumes(t *testing.T) {
	b := balance.Default()
	// Counter halves FrontierProgress impact (gap-close), then consumes the pin.
	pm := onlineModel(100, b.RefPrice)
	c := model.Competitor{Name: "OpenAI", Skill: [model.NumQualityDims]float64{1, 1, 1, 1}}
	c.Quality[model.DimCapability] = 90
	s := model.GameState{Models: []model.Model{pm}, Competitors: []model.Competitor{c}}
	s.Progression.MaxUnlockedGen = 1
	s.Progression.Rivals = model.RivalEraState{Era: 1, Leaders: []string{"Nobody"}}
	s.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand,
		Primary:         model.RivalRoadmap{Company: "OpenAI", ActionIndex: 0, CyclesUntilAction: 1},
		CounterTarget:   "OpenAI",
		CounterActionID: "openai-flagship",
	}
	// Target 100; gap 10; progress 0.15 * impact 0.5 → +0.75 → 90.75
	// ageRivalMomentum runs first but MomentumCycles is 0 so no-op.
	ns := AdvanceCampaignCycle(s, b)
	var after float64
	for _, comp := range ns.Competitors {
		if comp.Name == "OpenAI" {
			after = comp.Quality[model.DimCapability]
			break
		}
	}
	if !approx(after, 90.75) {
		t.Fatalf("quality after counter: got %v want 90.75", after)
	}
	if ns.Campaign.CounterTarget != "" || ns.Campaign.CounterActionID != "" {
		t.Fatalf("counter should be consumed: target=%q action=%q", ns.Campaign.CounterTarget, ns.Campaign.CounterActionID)
	}
}

func TestMismatchedCounterDoesNotConsume(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Competitors: balance.DefaultCompetitors()}
	s.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand,
		Primary:         model.RivalRoadmap{Company: "OpenAI", ActionIndex: 0, CyclesUntilAction: 1},
		CounterTarget:   "OpenAI",
		CounterActionID: "openai-platform", // wrong telegraphed action
	}
	ns := AdvanceCampaignCycle(s, b)
	if ns.Campaign.CounterTarget != "OpenAI" || ns.Campaign.CounterActionID != "openai-platform" {
		t.Fatalf("mismatched counter must not consume: %+v", ns.Campaign)
	}
}

func TestCampaignPrestigeBanksBadgeAndLegacy(t *testing.T) {
	b := balance.Default()
	s := model.GameState{PeakValuation: 1e10}
	s.Campaign = model.CampaignState{Doctrine: model.DoctrineConsumer, Secondary: model.DoctrineDeveloper, SecondaryPerk: "developer-open", Stage: model.CampaignStageWon, Victory: model.DoctrineConsumer}
	ns, err := Apply(s, model.CampaignPrestige{Legacy: model.LegacyChoice{Kind: model.LegacySecondary, Doctrine: model.DoctrineDeveloper, PerkID: "developer-open"}}, b)
	if err != nil {
		t.Fatal(err)
	}
	if ns.Prestige.Patents != 10 || len(ns.Prestige.RouteBadges) != 1 {
		t.Fatalf("prestige=%+v", ns.Prestige)
	}
	if ns.Campaign.Legacy.Kind != model.LegacySecondary || ns.Prestige.PendingLegacy.Kind != model.LegacyNone {
		t.Fatalf("state=%+v", ns)
	}
}

func TestCampaignExitPaysHalfAndNoBadge(t *testing.T) {
	b := balance.Default()
	s := model.GameState{PeakValuation: 1e10, Campaign: model.CampaignState{Doctrine: model.DoctrineConsumer, Cycle: 18}}
	ns, err := Apply(s, model.CampaignExit{}, b)
	if err != nil {
		t.Fatal(err)
	}
	if ns.Prestige.Patents != 5 || len(ns.Prestige.RouteBadges) != 0 {
		t.Fatalf("prestige=%+v", ns.Prestige)
	}
}

func TestCampaignContinueKeepsRun(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Campaign: model.CampaignState{Doctrine: model.DoctrineConsumer, Victory: model.DoctrineConsumer}}
	ns, err := Apply(s, model.CampaignContinue{}, b)
	if err != nil || !ns.Campaign.Endless {
		t.Fatalf("err=%v campaign=%+v", err, ns.Campaign)
	}
}

func TestCampaignPrestigeRejectsNotWon(t *testing.T) {
	b := balance.Default()
	s := model.GameState{PeakValuation: 1e10, Campaign: model.CampaignState{Doctrine: model.DoctrineConsumer}}
	if _, err := Apply(s, model.CampaignPrestige{Legacy: model.LegacyChoice{Kind: model.LegacyIntel}}, b); !errors.Is(err, ErrCampaignNotWon) {
		t.Fatalf("err=%v want ErrCampaignNotWon", err)
	}
}

func TestCampaignPrestigeRejectsInvalidLegacy(t *testing.T) {
	b := balance.Default()
	s := model.GameState{PeakValuation: 1e10}
	s.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer, Secondary: model.DoctrineDeveloper, SecondaryPerk: "developer-open",
		Victory: model.DoctrineConsumer,
	}
	// Secondary mismatch.
	if _, err := Apply(s, model.CampaignPrestige{Legacy: model.LegacyChoice{Kind: model.LegacySecondary, Doctrine: model.DoctrineEnterprise, PerkID: "enterprise-compliance"}}, b); !errors.Is(err, ErrInvalidLegacy) {
		t.Fatalf("secondary mismatch err=%v", err)
	}
	// LegacyNone rejected.
	if _, err := Apply(s, model.CampaignPrestige{Legacy: model.LegacyChoice{}}, b); !errors.Is(err, ErrInvalidLegacy) {
		t.Fatalf("none err=%v", err)
	}
	// Tech not in UnlockedTech.
	if _, err := Apply(s, model.CampaignPrestige{Legacy: model.LegacyChoice{Kind: model.LegacyTech, TechID: "algo-cap-1"}}, b); !errors.Is(err, ErrInvalidLegacy) {
		t.Fatalf("tech err=%v", err)
	}
}

func TestCampaignExitLockedUntilCycleOrDistress(t *testing.T) {
	b := balance.Default()
	s := model.GameState{PeakValuation: 1e10, Campaign: model.CampaignState{Doctrine: model.DoctrineConsumer, Cycle: 10}}
	if _, err := Apply(s, model.CampaignExit{}, b); !errors.Is(err, ErrStrategyExitLocked) {
		t.Fatalf("err=%v want ErrStrategyExitLocked", err)
	}
	s.Campaign.FinancialDistressCycles = 2
	ns, err := Apply(s, model.CampaignExit{}, b)
	if err != nil {
		t.Fatal(err)
	}
	if ns.Prestige.Patents != 5 {
		t.Fatalf("distress exit patents=%v", ns.Prestige.Patents)
	}
}

func TestActiveCampaignBlocksPrestigeReset(t *testing.T) {
	b := balance.Default()
	s := model.GameState{PeakValuation: 1e10, Campaign: model.CampaignState{Doctrine: model.DoctrineConsumer}}
	if _, err := Apply(s, model.PrestigeReset{}, b); !errors.Is(err, ErrCampaignNotWon) {
		t.Fatalf("err=%v want ErrCampaignNotWon", err)
	}
}

func TestCampaignSettlementPreservesRNG(t *testing.T) {
	b := balance.Default()
	s := model.GameState{PeakValuation: 1e10}
	s.Events.RandState = 42
	s.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer, Secondary: model.DoctrineDeveloper, SecondaryPerk: "developer-open",
		Victory: model.DoctrineConsumer, RandState: 99,
	}
	ns, err := Apply(s, model.CampaignPrestige{Legacy: model.LegacyChoice{Kind: model.LegacyIntel}}, b)
	if err != nil {
		t.Fatal(err)
	}
	if ns.Events.RandState != 42 || ns.Campaign.RandState != 99 {
		t.Fatalf("rng not preserved: events=%d campaign=%d", ns.Events.RandState, ns.Campaign.RandState)
	}
}

func TestDoctrineSelectionConsumesSecondaryLegacy(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Models: []model.Model{{Online: true}}}
	s.Campaign.Legacy = model.LegacyChoice{Kind: model.LegacySecondary, Doctrine: model.DoctrineDeveloper, PerkID: "developer-open"}
	ns, err := Apply(s, model.ChooseDoctrine{Doctrine: model.DoctrineConsumer}, b)
	if err != nil {
		t.Fatal(err)
	}
	if ns.Campaign.Secondary != model.DoctrineDeveloper || ns.Campaign.SecondaryPerk != "developer-open" {
		t.Fatalf("secondary not applied: %+v", ns.Campaign)
	}
	if ns.Campaign.Legacy.Kind != model.LegacyNone {
		t.Fatalf("legacy not consumed: %+v", ns.Campaign.Legacy)
	}
}

func TestDoctrineSelectionConsumesIntelLegacy(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Models: []model.Model{{Online: true}}}
	s.Campaign.Legacy = model.LegacyChoice{Kind: model.LegacyIntel}
	ns, err := Apply(s, model.ChooseDoctrine{Doctrine: model.DoctrineConsumer}, b)
	if err != nil {
		t.Fatal(err)
	}
	if !ns.Campaign.Primary.IntelFull || !ns.Campaign.Wildcard.IntelFull {
		t.Fatalf("intel not applied: primary=%+v wildcard=%+v", ns.Campaign.Primary, ns.Campaign.Wildcard)
	}
	if ns.Campaign.Legacy.Kind != model.LegacyNone {
		t.Fatalf("legacy not consumed: %+v", ns.Campaign.Legacy)
	}
}

func TestFailedSettlementPreservesState(t *testing.T) {
	b := balance.Default()
	s := model.GameState{PeakValuation: 1e10, Resources: model.Resources{Cash: 12345}}
	s.Campaign = model.CampaignState{Doctrine: model.DoctrineConsumer, Cycle: 3}
	s.Prestige.Patents = 7
	ns, err := Apply(s, model.CampaignExit{}, b)
	if !errors.Is(err, ErrStrategyExitLocked) {
		t.Fatalf("err=%v", err)
	}
	if ns.Prestige.Patents != 7 || ns.Resources.Cash != 12345 || len(ns.Prestige.RouteBadges) != 0 {
		t.Fatalf("failure mutated state: %+v", ns)
	}
}
