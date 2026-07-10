package sim

import (
	"errors"
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
	// OpenAI quality baseline from DefaultCompetitors; action openai-flagship is +15% capability.
	s := model.GameState{Competitors: balance.DefaultCompetitors()}
	s.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand,
		Primary:         model.RivalRoadmap{Company: "OpenAI", ActionIndex: 0, CyclesUntilAction: 1},
		CounterTarget:   "OpenAI",
		CounterActionID: "openai-flagship",
	}
	// Find OpenAI competitor quality before.
	var before float64
	for _, c := range s.Competitors {
		if c.Name == "OpenAI" {
			before = c.Quality[model.DimCapability]
			break
		}
	}
	// Uncountered impact would be before * (1 + 0.15); matched is before * (1 + 0.15*0.5).
	want := before * (1 + 0.15*0.5)
	ns := AdvanceCampaignCycle(s, b)
	var after float64
	for _, c := range ns.Competitors {
		if c.Name == "OpenAI" {
			after = c.Quality[model.DimCapability]
			break
		}
	}
	if after != want {
		t.Fatalf("quality after counter: got %v want %v (before=%v)", after, want, before)
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
