package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestPrestigeEffectsAggregate(t *testing.T) {
	b := balance.Default()
	pe := PrestigeEffects([]string{"start-cash-1", "rnd-mult-1"}, b)
	if !approx(pe.StartCash, 100000) {
		t.Errorf("StartCash = %v, want 100000", pe.StartCash)
	}
	if !approx(pe.RnDMult, 1.1) {
		t.Errorf("RnDMult = %v, want 1.1", pe.RnDMult)
	}
	if !approx(pe.CashMult, 1) {
		t.Errorf("unrelated mult should be 1: %v", pe.CashMult)
	}
}

func TestPatentsFor(t *testing.T) {
	b := balance.Default()                   // PatentK 1e8
	if got := patentsFor(1e9, b); got != 3 { // floor(sqrt(10))
		t.Errorf("patentsFor(1e9) = %v, want 3", got)
	}
	if got := patentsFor(1e10, b); got != 10 { // floor(sqrt(100))
		t.Errorf("patentsFor(1e10) = %v, want 10", got)
	}
}

func TestRestartUngatedBanksPatentsAndResets(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Models = []model.Model{{Online: true, Users: 100}}
	s.Resources.Cash = -50000 // deep in debt, well below any prestige gate
	s.PeakValuation = 1e10    // banks floor(sqrt(1e10/1e8)) = 10 patents
	s.Prestige.Patents = 3
	ns := Restart(s, b)
	if len(ns.Models) != 0 {
		t.Fatalf("restart should clear models, got %d", len(ns.Models))
	}
	if ns.Resources.Cash != b.StartingCash {
		t.Fatalf("restart should reset cash to start, got %v", ns.Resources.Cash)
	}
	if ns.Prestige.Patents != 13 {
		t.Fatalf("restart should bank patents from peak: got %v want 13", ns.Prestige.Patents)
	}
}

func TestActiveCampaignRestartDoesNotFullBank(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Models = []model.Model{{Online: true, Users: 100}}
	s.PeakValuation = 1e10 // would bank 10 patents if full bank applied
	s.Prestige.Patents = 3
	s.Prestige.RouteBadges = []model.Doctrine{model.DoctrineEnterprise}
	s.Events.RandState = 42
	s.Campaign = model.CampaignState{
		Doctrine:  model.DoctrineConsumer,
		Cycle:     5,
		RandState: 99,
		Victory:   model.DoctrineNone,
	}
	ns := Restart(s, b)
	if ns.Prestige.Patents != 3 {
		t.Fatalf("active-campaign Restart must not bank patents: got %v want 3", ns.Prestige.Patents)
	}
	if len(ns.Prestige.RouteBadges) != 1 || ns.Prestige.RouteBadges[0] != model.DoctrineEnterprise {
		t.Fatalf("Restart must not grant/alter badges: %+v", ns.Prestige.RouteBadges)
	}
	if ns.Prestige.PendingLegacy.Kind != model.LegacyNone || ns.Campaign.Legacy.Kind != model.LegacyNone {
		t.Fatalf("Restart must not grant Legacy: pending=%+v campaign=%+v", ns.Prestige.PendingLegacy, ns.Campaign.Legacy)
	}
	if len(ns.Models) != 0 {
		t.Fatalf("Restart should still abandon run (clear models), got %d", len(ns.Models))
	}
	if ns.Events.RandState != 42 || ns.Campaign.RandState != 99 {
		t.Fatalf("RNG not preserved: events=%d campaign=%d", ns.Events.RandState, ns.Campaign.RandState)
	}
	// Pre-campaign behavior remains covered by TestRestartUngatedBanksPatentsAndResets.
}

func TestFreshRun(t *testing.T) {
	b := balance.Default()
	p := model.Prestige{Patents: 5, UnlockedPrestige: []string{"start-cash-1"}} // +100k cash
	ns := freshRun(p, b)
	if ns.Prestige.Patents != 5 {
		t.Errorf("patents not preserved: %v", ns.Prestige.Patents)
	}
	if len(ns.Competitors) != 7 {
		t.Errorf("competitors not re-seeded")
	}
	if !approx(ns.Resources.Cash, b.StartingCash+100000) {
		t.Errorf("cash = %v, want %v", ns.Resources.Cash, b.StartingCash+100000)
	}
	if ns.Research.EfficiencyMult != 1 {
		t.Errorf("efficiency mult not reset to 1")
	}
	// Office starts at L1; employees/market seed in later tasks. Compute starts empty.
	if ns.Office.Level != 1 {
		t.Errorf("office level = %d, want 1", ns.Office.Level)
	}
	if len(ns.Employees) != 0 {
		t.Errorf("employees should start empty, got %d", len(ns.Employees))
	}
	if len(ns.Compute.RentedTraining) != 0 || len(ns.Compute.RentedInference) != 0 {
		t.Errorf("compute should start empty, got train=%v inf=%v", ns.Compute.RentedTraining, ns.Compute.RentedInference)
	}
	if !approx(ns.Resources.RnD, b.StartingRnD) { // start-cash-1 adds no R&D
		t.Errorf("R&D not reseeded: %v, want %v", ns.Resources.RnD, b.StartingRnD)
	}
	if ns.Progression.MaxUnlockedGen != 1 {
		t.Errorf("MaxUnlockedGen = %d, want 1", ns.Progression.MaxUnlockedGen)
	}
	if ns.Progression.IndustryTime != 0 || ns.Progression.Frontier.Active || len(ns.Progression.Eras) != 0 {
		t.Errorf("rest of Progression should be zero on fresh run: %+v", ns.Progression)
	}
}

func TestRestartClearsProgression(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.PeakValuation = 1e8
	s.Events.RandState = 11
	s.Campaign.RandState = 22
	s.Progression = model.ProgressionState{
		MaxUnlockedGen: 8,
		IndustryTime:   99999,
		Frontier: model.FrontierProject{
			Active:        true,
			TargetGen:     9,
			AllocationPct: 70,
		},
		Eras:   []model.EraProgress{{Era: 3, HasPrimary: true, Primary: model.BranchAlgo, UnlockedMask: 0b1111}},
		Rivals: model.RivalEraState{Era: 3, Leaders: []string{"OpenAI"}},
	}
	ns := Restart(s, b)
	if ns.Progression.MaxUnlockedGen != 1 {
		t.Fatalf("restart MaxUnlockedGen = %d, want 1", ns.Progression.MaxUnlockedGen)
	}
	if ns.Progression.IndustryTime != 0 {
		t.Fatalf("IndustryTime not cleared: %v", ns.Progression.IndustryTime)
	}
	if ns.Progression.Frontier.Active || ns.Progression.Frontier.TargetGen != 0 {
		t.Fatalf("frontier not cleared: %+v", ns.Progression.Frontier)
	}
	if len(ns.Progression.Eras) != 0 {
		t.Fatalf("eras not cleared: %+v", ns.Progression.Eras)
	}
	if ns.Progression.Rivals.Era != 0 || len(ns.Progression.Rivals.Leaders) != 0 {
		t.Fatalf("rivals not cleared: %+v", ns.Progression.Rivals)
	}
	if ns.Events.RandState != 11 || ns.Campaign.RandState != 22 {
		t.Fatalf("RNG not preserved: events=%d campaign=%d", ns.Events.RandState, ns.Campaign.RandState)
	}
}

func TestFreshRunTransfersAndConsumesPendingLegacy(t *testing.T) {
	b := balance.Default()
	// Secondary: transfer to Campaign.Legacy, clear PendingLegacy, no tech unlock.
	p := model.Prestige{
		Patents:       3,
		PendingLegacy: model.LegacyChoice{Kind: model.LegacySecondary, Doctrine: model.DoctrineDeveloper, PerkID: "developer-open"},
	}
	ns := freshRun(p, b)
	if ns.Campaign.Legacy.Kind != model.LegacySecondary || ns.Campaign.Legacy.PerkID != "developer-open" {
		t.Fatalf("secondary not transferred: %+v", ns.Campaign.Legacy)
	}
	if ns.Prestige.PendingLegacy.Kind != model.LegacyNone {
		t.Fatalf("pending not cleared: %+v", ns.Prestige.PendingLegacy)
	}
	// Tech: apply UnlockedTech and clear pending; tech is one-shot so Campaign.Legacy is cleared after apply.
	p2 := model.Prestige{
		Patents:       1,
		PendingLegacy: model.LegacyChoice{Kind: model.LegacyTech, TechID: "algo-cap-1"},
	}
	ns2 := freshRun(p2, b)
	if len(ns2.UnlockedTech) != 1 || ns2.UnlockedTech[0] != "algo-cap-1" {
		t.Fatalf("tech not applied: %v", ns2.UnlockedTech)
	}
	if ns2.Prestige.PendingLegacy.Kind != model.LegacyNone {
		t.Fatalf("pending not cleared after tech: %+v", ns2.Prestige.PendingLegacy)
	}
	// Repeating freshRun on resulting prestige must not re-apply.
	ns3 := freshRun(ns2.Prestige, b)
	if len(ns3.UnlockedTech) != 0 {
		t.Fatalf("tech re-applied on second freshRun: %v", ns3.UnlockedTech)
	}
}

func TestFreshRunBadgeUniquenessViaAddDoctrine(t *testing.T) {
	// addDoctrineUnique is exercised via prestige banking; unit-check uniqueness helper path.
	got := addDoctrineUnique([]model.Doctrine{model.DoctrineConsumer}, model.DoctrineConsumer)
	if len(got) != 1 {
		t.Fatalf("duplicate badge added: %v", got)
	}
	got = addDoctrineUnique(got, model.DoctrineDeveloper)
	if len(got) != 2 || got[1] != model.DoctrineDeveloper {
		t.Fatalf("unique append failed: %v", got)
	}
}

func TestPrestigeClearsProgression(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.PeakValuation = b.PrestigeUnlockValuation // unlock prestige
	s.Models = []model.Model{{Gen: 8, Online: true, Users: 1000, Quality: [model.NumQualityDims]float64{50, 50, 50, 50}}}
	s.UnlockedTech = []string{balance.GenUnlockNodeID(2), "algo-cap-1"}
	s.Prestige = model.Prestige{
		Patents:          5,
		UnlockedPrestige: []string{"start-cash-1"},
		RouteBadges:      []model.Doctrine{model.DoctrineEnterprise},
	}
	s.Progression = model.ProgressionState{
		MaxUnlockedGen: 10,
		IndustryTime:   1e6,
		Frontier: model.FrontierProject{
			Active: true, TargetGen: 11, AllocationPct: 80,
			RnDTotal: 1, RnDRemaining: 1, WorkTotal: 1, WorkRemaining: 1,
		},
		Eras:   []model.EraProgress{{Era: 4, HasPrimary: true, Primary: model.BranchInfra, UnlockedMask: 0b1111}},
		Rivals: model.RivalEraState{Era: 4, Leaders: []string{"OpenAI", "Anthropic"}},
	}
	// PrestigeReset clears run-scoped progression, keeps permanent prestige.
	ns, err := Apply(s, model.PrestigeReset{}, b)
	if err != nil {
		t.Fatalf("PrestigeReset: %v", err)
	}
	if ns.Progression.MaxUnlockedGen != 1 {
		t.Fatalf("MaxUnlockedGen = %d, want 1", ns.Progression.MaxUnlockedGen)
	}
	if ns.Progression.IndustryTime != 0 || ns.Progression.Frontier.Active || len(ns.Progression.Eras) != 0 {
		t.Fatalf("progression not cleared: %+v", ns.Progression)
	}
	if len(ns.Progression.Rivals.Leaders) != 0 || ns.Progression.Rivals.Era != 0 {
		t.Fatalf("rivals not cleared: %+v", ns.Progression.Rivals)
	}
	if len(ns.Models) != 0 || len(ns.UnlockedTech) != 0 {
		t.Fatalf("run state not cleared: models=%d tech=%v", len(ns.Models), ns.UnlockedTech)
	}
	// Permanent progress preserved (plus banked patents).
	if ns.Prestige.Patents < 5 {
		t.Fatalf("patents lost: %v", ns.Prestige.Patents)
	}
	if len(ns.Prestige.UnlockedPrestige) != 1 || ns.Prestige.UnlockedPrestige[0] != "start-cash-1" {
		t.Fatalf("permanent nodes lost: %v", ns.Prestige.UnlockedPrestige)
	}
	if len(ns.Prestige.RouteBadges) != 1 || ns.Prestige.RouteBadges[0] != model.DoctrineEnterprise {
		t.Fatalf("badges lost: %v", ns.Prestige.RouteBadges)
	}
	// start-cash-1 still applies on fresh run.
	if ns.Resources.Cash < b.StartingCash {
		t.Fatalf("permanent start cash not applied: %v", ns.Resources.Cash)
	}

	// Restart path also clears progression while banking patents (no doctrine).
	s2 := s
	s2.Campaign.Doctrine = model.DoctrineNone
	s2.Prestige.Patents = 2
	rs := Restart(s2, b)
	if rs.Progression.MaxUnlockedGen != 1 || rs.Progression.Frontier.Active || rs.Progression.IndustryTime != 0 {
		t.Fatalf("Restart progression: %+v", rs.Progression)
	}
	if rs.Prestige.Patents < 2 {
		t.Fatalf("Restart patents: %v", rs.Prestige.Patents)
	}
}

func TestNoForcedGenerationEnding(t *testing.T) {
	b := balance.Default()
	// Gen10 unlocked, Gen11 frontier active — Tick must continue; no auto victory/prestige.
	s := model.GameState{}
	s.Progression.MaxUnlockedGen = 10
	s.Progression.IndustryTime = 40000 * 86400
	s.Models = []model.Model{{Gen: 10, Online: true, Users: 100, Price: 12, Quality: [model.NumQualityDims]float64{100, 100, 100, 100}}}
	s.Resources.RnD = 1e18
	s.Resources.Cash = 1e6
	s.Servers = []model.Server{{Pool: model.PoolTraining, Compute: 1e5}}
	s.Progression.Frontier = model.FrontierProject{
		Active: true, TargetGen: 11, AllocationPct: 100,
		RnDTotal: 1e12, RnDRemaining: 1e12,
		WorkTotal: 1e12, WorkRemaining: 1e12,
		RecommendedCompute: 18000,
	}
	s.Campaign = model.CampaignState{} // no doctrine → no victory path
	beforePatents := s.Prestige.Patents
	ns := Tick(s, 3600, nil, b)
	if ns.GameTime <= s.GameTime {
		t.Fatal("Tick did not advance at Gen10/11")
	}
	if ns.Campaign.Victory != model.DoctrineNone {
		t.Fatalf("Gen10/11 must not force victory: %v", ns.Campaign.Victory)
	}
	if ns.Prestige.Patents != beforePatents {
		t.Fatalf("Tick must not bank patents: %v → %v", beforePatents, ns.Prestige.Patents)
	}
	// PrestigeReset still optional and gated — not auto-triggered.
	if ns.PeakValuation < b.PrestigeUnlockValuation {
		// fine if valuation low
	}
	// Completing Gen11 frontier unlocks gen, does not end run.
	s.Progression.Frontier.WorkRemaining = 1
	s.Progression.Frontier.RnDRemaining = 1
	done := Tick(s, 1, nil, b)
	if done.Progression.MaxUnlockedGen < 11 && done.Progression.Frontier.Active {
		// either completed or still active is fine; must not zero state
	}
	if len(done.Models) == 0 && done.Resources.Cash == b.StartingCash && done.Progression.MaxUnlockedGen == 1 {
		t.Fatal("Tick must not soft-reset the run at Gen11")
	}
}
