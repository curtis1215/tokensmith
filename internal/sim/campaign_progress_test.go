package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func campaignRouteState(d model.Doctrine, seg model.Segment) model.GameState {
	b := balance.Default()
	s := model.GameState{}
	s.Campaign = model.CampaignState{Doctrine: d, Stage: model.CampaignStageEstablish}
	s.Models = []model.Model{{Online: true, Segment: seg, Price: b.SegmentRefPrice[seg], Users: 100000, Quality: [4]float64{80, 80, 80, 80}}}
	s.Competitors = []model.Competitor{{Name: "Rival", Quality: [4]float64{1, 1, 1, 1}}}
	s.Compute.RentedInference = map[string]int{balance.EntryProcessID: 1000}
	return s
}

func TestCampaignEstablishUnlocksTierOnePerk(t *testing.T) {
	b := balance.Default()
	s := campaignRouteState(model.DoctrineConsumer, model.SegConsumer)
	ns, entries := advanceCampaignProgress(s, b)
	if ns.Campaign.Stage != model.CampaignStageExpand {
		t.Fatalf("stage=%s, want expand", ns.Campaign.Stage)
	}
	if ns.Campaign.PerkTierPending != 1 {
		t.Fatalf("PerkTierPending=%d, want 1", ns.Campaign.PerkTierPending)
	}
	if len(entries) != 1 || entries[0].Kind != model.ReportStageAdvanced || entries[0].SubjectID != string(model.CampaignStageExpand) {
		t.Fatalf("entries=%+v", entries)
	}
}

func TestCampaignExpandRequiresTierOnePerk(t *testing.T) {
	b := balance.Default()
	s := campaignRouteState(model.DoctrineConsumer, model.SegConsumer)
	s.Campaign.Stage = model.CampaignStageExpand
	// Gate is met (high share + capacity) but no perk selected yet.
	ns, entries := advanceCampaignProgress(s, b)
	if ns.Campaign.Stage != model.CampaignStageExpand {
		t.Fatalf("stage advanced without perk: %s", ns.Campaign.Stage)
	}
	if len(entries) != 0 {
		t.Fatalf("entries=%+v, want none", entries)
	}
}

func TestCampaignExpandConsumerGate(t *testing.T) {
	b := balance.Default()
	// Share too low: two equal rivals → ~33% player share with weak model.
	s := campaignRouteState(model.DoctrineConsumer, model.SegConsumer)
	s.Campaign.Stage = model.CampaignStageExpand
	s.Campaign.Perks = []string{"consumer-premium"}
	s.Models[0].Quality = [4]float64{1, 1, 1, 1}
	s.Competitors = []model.Competitor{
		{Name: "A", Quality: [4]float64{80, 80, 80, 80}},
		{Name: "B", Quality: [4]float64{80, 80, 80, 80}},
	}
	ns, _ := advanceCampaignProgress(s, b)
	if ns.Campaign.Stage != model.CampaignStageExpand {
		t.Fatalf("low share should not expand→showdown, stage=%s", ns.Campaign.Stage)
	}

	// High share + capacity OK advances.
	ok := campaignRouteState(model.DoctrineConsumer, model.SegConsumer)
	ok.Campaign.Stage = model.CampaignStageExpand
	ok.Campaign.Perks = []string{"consumer-premium"}
	ns, entries := advanceCampaignProgress(ok, b)
	if ns.Campaign.Stage != model.CampaignStageShowdown || ns.Campaign.PerkTierPending != 2 {
		t.Fatalf("campaign=%+v", ns.Campaign)
	}
	if len(entries) != 1 || entries[0].Kind != model.ReportStageAdvanced {
		t.Fatalf("entries=%+v", entries)
	}
}

func TestCampaignExpandEnterpriseGate(t *testing.T) {
	b := balance.Default()
	s := campaignRouteState(model.DoctrineEnterprise, model.SegEnterprise)
	s.Campaign.Stage = model.CampaignStageExpand
	s.Campaign.Perks = []string{"enterprise-compliance"}
	s.Campaign.Primary.Company = "Rival"
	// Price below ref fails PriceOK.
	s.Models[0].Price = b.SegmentRefPrice[model.SegEnterprise] * 0.5
	ns, _ := advanceCampaignProgress(s, b)
	if ns.Campaign.Stage != model.CampaignStageExpand {
		t.Fatalf("underpriced enterprise should not advance, stage=%s", ns.Campaign.Stage)
	}

	s.Models[0].Price = b.SegmentRefPrice[model.SegEnterprise]
	ns, _ = advanceCampaignProgress(s, b)
	if ns.Campaign.Stage != model.CampaignStageShowdown {
		t.Fatalf("enterprise expand gate not met: stage=%s status=%+v", ns.Campaign.Stage, CampaignStatus(s, b))
	}
}

func TestCampaignExpandDeveloperGate(t *testing.T) {
	b := balance.Default()
	s := campaignRouteState(model.DoctrineDeveloper, model.SegDeveloper)
	s.Campaign.Stage = model.CampaignStageExpand
	// developer-api raises ref; expand still requires price ≤ EffectiveRefPrice.
	s.Campaign.Perks = []string{"developer-api"}
	ref := EffectiveRefPrice(s, model.SegDeveloper, b)
	// Overpriced fails developer expand PriceOK (must be ≤ ref).
	s.Models[0].Price = ref * 1.5
	ns, _ := advanceCampaignProgress(s, b)
	if ns.Campaign.Stage != model.CampaignStageExpand {
		t.Fatalf("overpriced developer should not advance, stage=%s", ns.Campaign.Stage)
	}

	s.Models[0].Price = ref
	ns, _ = advanceCampaignProgress(s, b)
	if ns.Campaign.Stage != model.CampaignStageShowdown {
		t.Fatalf("developer expand gate not met: stage=%s status=%+v", ns.Campaign.Stage, CampaignStatus(s, b))
	}
}

func TestCampaignShowdownRequiresTwoPerks(t *testing.T) {
	b := balance.Default()
	s := campaignRouteState(model.DoctrineConsumer, model.SegConsumer)
	s.Campaign.Stage = model.CampaignStageShowdown
	s.Campaign.Perks = []string{"consumer-premium"} // only one
	s.Campaign.Cycle = 5
	ns, entries := advanceCampaignProgress(s, b)
	if ns.Campaign.ShowdownStartedCycle != 0 {
		t.Fatalf("showdown started with one perk: %+v", ns.Campaign)
	}
	if len(entries) != 0 {
		t.Fatalf("entries=%+v", entries)
	}
}

func TestCampaignVictoryRequiresTwoHeldCycles(t *testing.T) {
	b := balance.Default()
	s := campaignRouteState(model.DoctrineConsumer, model.SegConsumer)
	s.Campaign.Stage = model.CampaignStageShowdown
	s.Campaign.Perks = []string{"consumer-premium", "consumer-resilience"}
	s.Campaign.Cycle = 10
	s.Campaign.Primary = model.RivalRoadmap{Company: "OpenAI", ActionIndex: 0, CyclesUntilAction: 2}
	// OpenAI starts just behind the player; the telegraphed flagship action
	// moves it ahead and breaks the rank-1 showdown gate.
	s.Competitors = []model.Competitor{{Name: "OpenAI", Quality: [4]float64{80, 80, 80, 80}}}

	// First complete win state starts the showdown window.
	ns, entries := advanceCampaignProgress(s, b)
	if ns.Campaign.ShowdownStartedCycle != 10 {
		t.Fatalf("ShowdownStartedCycle=%d, want 10", ns.Campaign.ShowdownStartedCycle)
	}
	if ns.Campaign.Primary.CyclesUntilAction != 1 {
		t.Fatalf("CyclesUntilAction=%d, want 1", ns.Campaign.Primary.CyclesUntilAction)
	}
	if len(entries) != 1 || entries[0].Kind != model.ReportShowdown {
		t.Fatalf("entries=%+v", entries)
	}

	// Hold counting waits until primary has executed on/after showdown start.
	ns.Campaign.Primary.LastExecutedCycle = 9 // before start
	ns, entries = advanceCampaignProgress(ns, b)
	if ns.Campaign.ShowdownHeld != 0 || len(entries) != 0 {
		t.Fatalf("held before primary action: held=%d entries=%+v", ns.Campaign.ShowdownHeld, entries)
	}

	ns.Campaign.Primary.LastExecutedCycle = 10
	ns, _ = advanceCampaignProgress(ns, b)
	if ns.Campaign.ShowdownHeld != 1 || ns.Campaign.Victory != model.DoctrineNone {
		t.Fatalf("first hold: %+v", ns.Campaign)
	}

	ns, entries = advanceCampaignProgress(ns, b)
	if ns.Campaign.Victory != model.DoctrineConsumer || ns.Campaign.Stage != model.CampaignStageWon {
		t.Fatalf("victory not awarded: %+v", ns.Campaign)
	}
	if len(entries) != 1 || entries[0].Kind != model.ReportVictory {
		t.Fatalf("entries=%+v", entries)
	}
}

func TestCampaignShowdownBrokenConditionsResetHeld(t *testing.T) {
	b := balance.Default()
	s := campaignRouteState(model.DoctrineConsumer, model.SegConsumer)
	s.Campaign.Stage = model.CampaignStageShowdown
	s.Campaign.Perks = []string{"consumer-premium", "consumer-resilience"}
	s.Campaign.Cycle = 12
	s.Campaign.ShowdownStartedCycle = 10
	s.Campaign.ShowdownHeld = 1
	s.Campaign.Primary.LastExecutedCycle = 11
	// Break win conditions: quality collapses so rank is no longer 1.
	s.Models[0].Quality = [4]float64{1, 1, 1, 1}
	s.Competitors = []model.Competitor{{Name: "Rival", Quality: [4]float64{100, 100, 100, 100}}}

	ns, entries := advanceCampaignProgress(s, b)
	if ns.Campaign.ShowdownHeld != 0 {
		t.Fatalf("held not reset: %d", ns.Campaign.ShowdownHeld)
	}
	if ns.Campaign.ShowdownAttempts != 1 {
		t.Fatalf("attempts=%d, want 1", ns.Campaign.ShowdownAttempts)
	}
	if ns.Campaign.Victory != model.DoctrineNone || ns.Campaign.Stage != model.CampaignStageShowdown {
		t.Fatalf("run should continue: %+v", ns.Campaign)
	}
	if len(entries) != 0 {
		t.Fatalf("entries=%+v", entries)
	}
}

func TestCampaignShowdownFailureBeforeFirstHoldRequiresNewAttack(t *testing.T) {
	b := balance.Default()
	s := campaignRouteState(model.DoctrineConsumer, model.SegConsumer)
	s.Campaign.Stage = model.CampaignStageShowdown
	s.Campaign.Perks = []string{"consumer-premium", "consumer-resilience"}
	s.Campaign.Cycle = 10
	s.Campaign.Primary = model.RivalRoadmap{Company: "OpenAI", ActionIndex: 0, CyclesUntilAction: 2}
	// OpenAI starts tied on quality; skill at the ceiling makes the flagship
	// gap-close toward GF×1.15 so capability moves ahead of the rank-1 gate.
	// (Skill 0 would target the soft floor and pull quality down instead.)
	s.Competitors = []model.Competitor{{
		Name:    "OpenAI",
		Quality: [4]float64{80, 80, 80, 80},
		Skill:   [4]float64{rivalCeilPct, rivalCeilPct, rivalCeilPct, rivalCeilPct},
	}}

	started, entries := advanceCampaignProgress(s, b)
	if started.Campaign.ShowdownStartedCycle != 10 || started.Campaign.Primary.CyclesUntilAction != 1 {
		t.Fatalf("showdown did not start: campaign=%+v", started.Campaign)
	}
	if len(entries) != 1 || entries[0].Kind != model.ReportShowdown {
		t.Fatalf("start entries=%+v", entries)
	}

	// The real board cycle executes the telegraphed attack before any hold has
	// accrued. This must consume the attempt, not the challenge.
	failed := AdvanceCampaignCycle(started, b)
	if failed.Campaign.ShowdownStartedCycle != 0 || failed.Campaign.ShowdownHeld != 0 {
		t.Fatalf("failed attempt not reset: campaign=%+v competitors=%+v status=%+v",
			failed.Campaign, failed.Competitors, CampaignStatus(failed, b))
	}
	if failed.Campaign.ShowdownAttempts != 1 {
		t.Fatalf("attempts=%d, want 1", failed.Campaign.ShowdownAttempts)
	}
	if failed.Campaign.Primary.LastExecutedCycle != 11 || failed.Campaign.Primary.ActionIndex != 1 {
		t.Fatalf("primary attack did not execute/advance: roadmap=%+v", failed.Campaign.Primary)
	}

	// Restoring the gate must start and telegraph a fresh primary attack.
	failed.Models[0].Quality = [4]float64{100, 100, 100, 100}
	retried := AdvanceCampaignCycle(failed, b)
	if retried.Campaign.ShowdownStartedCycle != 12 || retried.Campaign.Primary.CyclesUntilAction != 1 {
		t.Fatalf("fresh showdown not scheduled: campaign=%+v", retried.Campaign)
	}
	if retried.Campaign.ShowdownHeld != 0 || retried.Campaign.Victory != model.DoctrineNone {
		t.Fatalf("retry bypassed fresh attack: campaign=%+v", retried.Campaign)
	}
	if len(retried.Campaign.Reports) == 0 {
		t.Fatal("retry did not emit board report")
	}
	latest := retried.Campaign.Reports[len(retried.Campaign.Reports)-1]
	if len(latest.Entries) != 1 || latest.Entries[0].Kind != model.ReportShowdown {
		t.Fatalf("retry report=%+v", latest)
	}

	// The previous attack cannot count toward the new attempt.
	waiting, entries := advanceCampaignProgress(retried, b)
	if waiting.Campaign.ShowdownHeld != 0 || len(entries) != 0 {
		t.Fatalf("old attack counted for retry: campaign=%+v entries=%+v", waiting.Campaign, entries)
	}
}

func TestCampaignProgressIgnoresWonEndlessAndNoDoctrine(t *testing.T) {
	b := balance.Default()
	base := campaignRouteState(model.DoctrineConsumer, model.SegConsumer)

	none := base
	none.Campaign.Doctrine = model.DoctrineNone
	if ns, e := advanceCampaignProgress(none, b); ns.Campaign.Stage != model.CampaignStageEstablish || len(e) != 0 {
		t.Fatalf("no doctrine: %+v entries=%+v", ns.Campaign, e)
	}

	won := base
	won.Campaign.Victory = model.DoctrineConsumer
	won.Campaign.Stage = model.CampaignStageWon
	if ns, e := advanceCampaignProgress(won, b); ns.Campaign.Stage != model.CampaignStageWon || len(e) != 0 {
		t.Fatalf("won: %+v entries=%+v", ns.Campaign, e)
	}

	endless := base
	endless.Campaign.Endless = true
	if ns, e := advanceCampaignProgress(endless, b); ns.Campaign.Stage != model.CampaignStageEstablish || len(e) != 0 {
		t.Fatalf("endless: %+v entries=%+v", ns.Campaign, e)
	}
}
