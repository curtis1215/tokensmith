package sim

import (
	"errors"
	"reflect"
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestCampaignDeterministicAcrossSeeds(t *testing.T) {
	b := balance.Default()
	for seed := uint64(1); seed <= 100; seed++ {
		a := winningCampaignFixture(model.DoctrineConsumer, seed, b)
		c := a
		for cycle := 0; cycle < 21; cycle++ {
			// Arm directive each cycle so reset invariant is exercised.
			a.Campaign.DirectiveUsed = true
			c.Campaign.DirectiveUsed = true
			beforeAPrimary := a.Campaign.Primary
			a = AdvanceCampaignCycle(a, b)
			c = AdvanceCampaignCycle(c, b)
			if !reflect.DeepEqual(a, c) {
				t.Fatalf("seed=%d cycle=%d diverged", seed, cycle)
			}
			if len(a.Campaign.Reports) > b.Campaign.ReportCap {
				t.Fatalf("seed=%d reports=%d", seed, len(a.Campaign.Reports))
			}
			if a.Campaign.Primary.Company == a.Campaign.Wildcard.Company {
				t.Fatalf("seed=%d duplicate rivals primary=%q wildcard=%q", seed, a.Campaign.Primary.Company, a.Campaign.Wildcard.Company)
			}
			if a.Campaign.PerkTierPending < 0 || a.Campaign.PerkTierPending > 2 {
				t.Fatalf("seed=%d pending=%d", seed, a.Campaign.PerkTierPending)
			}
			if a.Campaign.DirectiveUsed {
				t.Fatalf("seed=%d cycle=%d directive did not reset", seed, a.Campaign.Cycle)
			}
			// After a rival action executes, countdown must re-arm positive
			// (telegraph at least one cycle before the next execution).
			if a.Campaign.Primary.LastExecutedCycle > beforeAPrimary.LastExecutedCycle {
				if a.Campaign.Primary.CyclesUntilAction <= 0 {
					t.Fatalf("seed=%d cycle=%d primary countdown after execution=%d", seed, a.Campaign.Cycle, a.Campaign.Primary.CyclesUntilAction)
				}
			}
			if a.Campaign.Wildcard.LastExecutedCycle > 0 && a.Campaign.Wildcard.CyclesUntilAction <= 0 {
				// Only fail when wildcard still has a company and finished an action
				// without re-arming (final action may leave countdown at next lead).
				if a.Campaign.Wildcard.Company != "" {
					// CyclesUntilAction is set from LeadCycles on re-arm; 0 only if catalog lead is 0.
					// Catalog LeadCycles are positive; treat non-positive as invariant breach.
					t.Fatalf("seed=%d cycle=%d wildcard countdown=%d after lastExec=%d", seed, a.Campaign.Cycle, a.Campaign.Wildcard.CyclesUntilAction, a.Campaign.Wildcard.LastExecutedCycle)
				}
			}
		}
	}
}

func TestCampaignInvariantsRouteBadgeUnique(t *testing.T) {
	b := balance.Default()
	s := model.GameState{PeakValuation: 1e10}
	s.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer, Victory: model.DoctrineConsumer,
		Secondary: model.DoctrineDeveloper, SecondaryPerk: "developer-open",
	}
	s.Prestige.RouteBadges = []model.Doctrine{model.DoctrineConsumer}
	ns, err := Apply(s, model.CampaignPrestige{Legacy: model.LegacyChoice{Kind: model.LegacyIntel}}, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(ns.Prestige.RouteBadges) != 1 || ns.Prestige.RouteBadges[0] != model.DoctrineConsumer {
		t.Fatalf("duplicate badge allowed: %+v", ns.Prestige.RouteBadges)
	}
}

func TestCampaignInvariantsDoubleSettlementRejected(t *testing.T) {
	b := balance.Default()
	s := model.GameState{PeakValuation: 1e10}
	s.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer, Victory: model.DoctrineConsumer,
		Secondary: model.DoctrineDeveloper, SecondaryPerk: "developer-open",
	}
	first, err := Apply(s, model.CampaignPrestige{Legacy: model.LegacyChoice{Kind: model.LegacyIntel}}, b)
	if err != nil {
		t.Fatal(err)
	}
	patents := first.Prestige.Patents
	// Same end command on the settled/fresh state: typed reject, patents unchanged.
	second, err2 := Apply(first, model.CampaignPrestige{Legacy: model.LegacyChoice{Kind: model.LegacyIntel}}, b)
	if !errors.Is(err2, ErrCampaignNotWon) {
		t.Fatalf("second prestige err=%v want ErrCampaignNotWon", err2)
	}
	if second.Prestige.Patents != patents {
		t.Fatalf("patents changed on rejected double settlement: got %v want %v", second.Prestige.Patents, patents)
	}
	// Exit twice from a lockable exit state: second must not re-pay.
	exitBase := model.GameState{PeakValuation: 1e10, Campaign: model.CampaignState{Doctrine: model.DoctrineConsumer, Cycle: 18}}
	ex1, err := Apply(exitBase, model.CampaignExit{}, b)
	if err != nil {
		t.Fatal(err)
	}
	p1 := ex1.Prestige.Patents
	ex2, err2 := Apply(ex1, model.CampaignExit{}, b)
	if !errors.Is(err2, ErrStrategyExitLocked) {
		t.Fatalf("second exit err=%v want ErrStrategyExitLocked", err2)
	}
	if ex2.Prestige.Patents != p1 {
		t.Fatalf("patents changed on second exit: got %v want %v", ex2.Prestige.Patents, p1)
	}
}

func TestCampaignInvariantsExitAndPrestigeCannotBothReward(t *testing.T) {
	b := balance.Default()
	// Prestige first (won run) then exit on the resulting fresh run: exit locked / no double pay.
	won := model.GameState{PeakValuation: 1e10}
	won.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer, Victory: model.DoctrineConsumer,
		Secondary: model.DoctrineDeveloper, SecondaryPerk: "developer-open",
	}
	afterPrestige, err := Apply(won, model.CampaignPrestige{Legacy: model.LegacyChoice{Kind: model.LegacyIntel}}, b)
	if err != nil {
		t.Fatal(err)
	}
	patents := afterPrestige.Prestige.Patents
	afterExit, err2 := Apply(afterPrestige, model.CampaignExit{}, b)
	if err2 == nil {
		t.Fatal("exit after prestige must not succeed on a fresh pre-campaign run without cycle/distress gate")
	}
	if afterExit.Prestige.Patents != patents {
		t.Fatalf("exit after prestige paid patents: got %v want %v", afterExit.Prestige.Patents, patents)
	}

	// Exit first then prestige: no victory, prestige rejected, patents stay exit-only.
	exitBase := model.GameState{PeakValuation: 1e10, Campaign: model.CampaignState{Doctrine: model.DoctrineConsumer, Cycle: 18}}
	afterExitOnly, err := Apply(exitBase, model.CampaignExit{}, b)
	if err != nil {
		t.Fatal(err)
	}
	exitPatents := afterExitOnly.Prestige.Patents
	afterPrestige2, err2 := Apply(afterExitOnly, model.CampaignPrestige{Legacy: model.LegacyChoice{Kind: model.LegacyIntel}}, b)
	if !errors.Is(err2, ErrCampaignNotWon) {
		t.Fatalf("prestige after exit err=%v want ErrCampaignNotWon", err2)
	}
	if afterPrestige2.Prestige.Patents != exitPatents {
		t.Fatalf("prestige after exit paid: got %v want %v", afterPrestige2.Prestige.Patents, exitPatents)
	}
	// One run never banks both full prestige patents and half exit patents.
	full := patentsFor(1e10, b)
	half := full * 0.5
	if afterPrestige.Prestige.Patents == full+half || afterExitOnly.Prestige.Patents == full+half {
		t.Fatalf("combined rewards observed: prestige=%v exit=%v full=%v half=%v", afterPrestige.Prestige.Patents, afterExitOnly.Prestige.Patents, full, half)
	}
}
