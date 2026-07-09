package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestNextRandDeterministic(t *testing.T) {
	s1, r1 := nextRand(42)
	s2, r2 := nextRand(42)
	if s1 != s2 || r1 != r2 {
		t.Fatalf("same input state must give same output: (%d,%v) vs (%d,%v)", s1, r1, s2, r2)
	}
	if s1 == 42 {
		t.Fatal("state must advance")
	}
}

func TestNextRandRangeAndSpread(t *testing.T) {
	state := uint64(7)
	var lo, hi int
	for i := 0; i < 1000; i++ {
		var r float64
		state, r = nextRand(state)
		if r < 0 || r >= 1 {
			t.Fatalf("r = %v out of [0,1)", r)
		}
		if r < 0.5 {
			lo++
		} else {
			hi++
		}
	}
	if lo < 400 || hi < 400 {
		t.Fatalf("distribution too skewed: lo=%d hi=%d", lo, hi)
	}
}

func TestNextRandZeroStateWorks(t *testing.T) {
	state, r := nextRand(0)
	if state == 0 {
		t.Fatal("state must advance from 0")
	}
	if r < 0 || r >= 1 {
		t.Fatalf("r = %v out of [0,1)", r)
	}
}

func TestEventEffectsNeutralWhenEmpty(t *testing.T) {
	var s model.GameState
	b := balance.Default()
	e := eventEffects(s, b)
	if e != model.NeutralEventEffects() {
		t.Fatalf("empty Active must aggregate to neutral, got %+v", e)
	}
}

func TestEventEffectsMultiplies(t *testing.T) {
	var s model.GameState
	b := balance.Default()
	m1 := model.NeutralEventEffects()
	m1.PowerCostMult = 1.3
	m1.UserGrowthMult = 1.25
	m2 := model.NeutralEventEffects()
	m2.PowerCostMult = 0.7
	s.Events.Active = []model.ActiveModifier{
		{EventID: "a", ExpiresAt: 999, Target: -1, Effects: m1},
		{EventID: "b", ExpiresAt: 999, Target: -1, Effects: m2},
	}
	e := eventEffects(s, b)
	if diff := e.PowerCostMult - 1.3*0.7; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("PowerCostMult = %v, want %v", e.PowerCostMult, 1.3*0.7)
	}
	if e.UserGrowthMult != 1.25 {
		t.Fatalf("UserGrowthMult = %v, want 1.25", e.UserGrowthMult)
	}
	if e.TechCostMult != 1.0 {
		t.Fatalf("aggregate TechCostMult must stay neutral (branch-targeted), got %v", e.TechCostMult)
	}
}

func TestEventTechCostMultBranchTargeted(t *testing.T) {
	var s model.GameState
	m := model.NeutralEventEffects()
	m.TechCostMult = 0.5
	s.Events.Active = []model.ActiveModifier{
		{EventID: "paper", ExpiresAt: 999, Target: int(model.BranchAlgo), Effects: m},
	}
	if got := eventTechCostMult(s, model.BranchAlgo); got != 0.5 {
		t.Fatalf("targeted branch mult = %v, want 0.5", got)
	}
	if got := eventTechCostMult(s, model.BranchInfra); got != 1.0 {
		t.Fatalf("untargeted branch mult = %v, want 1.0", got)
	}
}

// eventTestState returns a state with one online model and cash, positioned
// at GameTime 0, with a fixed RNG seed for deterministic rolls.
func eventTestState() model.GameState {
	var s model.GameState
	s.Resources.Cash = 1e6
	s.Resources.RnD = 1e5
	s.Events.RandState = 12345
	s.Competitors = balance.DefaultCompetitors()
	s.Models = []model.Model{
		{Gen: 1, Segment: model.SegConsumer, Users: 10000, Price: 12, Online: true, Name: "M1",
			Quality: [model.NumQualityDims]float64{20, 15, 10, 15}},
		{Gen: 1, Segment: model.SegEnterprise, Users: 2000, Price: 180, Online: true, Name: "M2",
			Quality: [model.NumQualityDims]float64{15, 10, 20, 10}},
	}
	return s
}

func TestFireChipShortageAddsModifierAndPending(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	spec, _ := balance.EventByID(b.Events, balance.EvChipShortage)
	ns := fireEvent(s, spec, b)
	if len(ns.Events.Active) != 1 || ns.Events.Active[0].Effects.BuildCostMult != balance.EvChipShortageBuildMult {
		t.Fatalf("expected BuildCostMult modifier, got %+v", ns.Events.Active)
	}
	if len(ns.Events.Pending) != 1 || ns.Events.Pending[0].EventID != balance.EvChipShortage {
		t.Fatalf("expected pending entry, got %+v", ns.Events.Pending)
	}
	if ns.Events.Pending[0].Deadline != s.GameTime+spec.DeadlineSec {
		t.Fatalf("deadline = %v, want %v", ns.Events.Pending[0].Deadline, s.GameTime+spec.DeadlineSec)
	}
	if ns.Events.FiredCount != 1 {
		t.Fatalf("FiredCount = %d, want 1", ns.Events.FiredCount)
	}
	if len(s.Events.Active) != 0 || len(s.Events.Pending) != 0 {
		t.Fatal("fireEvent must not mutate its input state")
	}
}

func TestResolveChipShortagePaidRemovesModifier(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	spec, _ := balance.EventByID(b.Events, balance.EvChipShortage)
	s = fireEvent(s, spec, b)
	cashBefore := s.Resources.Cash
	ns, err := resolveChoice(s, 0, 0, false, b)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(ns.Events.Active) != 0 {
		t.Fatalf("paid choice must remove the modifier, got %+v", ns.Events.Active)
	}
	if len(ns.Events.Pending) != 0 {
		t.Fatal("pending entry must be consumed")
	}
	if ns.Resources.Cash >= cashBefore {
		t.Fatal("choice 0 must charge cash")
	}
	if len(ns.Events.Log) != 1 || ns.Events.Log[0].Choice != 0 || ns.Events.Log[0].Auto {
		t.Fatalf("log record wrong: %+v", ns.Events.Log)
	}
}

func TestResolveDefaultKeepsModifierAndIsFree(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	spec, _ := balance.EventByID(b.Events, balance.EvChipShortage)
	s = fireEvent(s, spec, b)
	cashBefore := s.Resources.Cash
	ns, err := resolveChoice(s, 0, 1, true, b)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(ns.Events.Active) != 1 {
		t.Fatal("default choice keeps the modifier")
	}
	if ns.Resources.Cash != cashBefore {
		t.Fatal("default choice must be free")
	}
	if ns.Events.AutoCount != 1 || !ns.Events.Log[0].Auto {
		t.Fatal("auto resolve must be recorded")
	}
}

func TestResolveInsufficientCash(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	s.Resources.Cash = 1 // below every CashCostFloor
	spec, _ := balance.EventByID(b.Events, balance.EvChipShortage)
	s = fireEvent(s, spec, b)
	if _, err := resolveChoice(s, 0, 0, false, b); err != ErrInsufficientCash {
		t.Fatalf("err = %v, want ErrInsufficientCash", err)
	}
}

func TestFireRivalBreakthroughBoostsStrongestRival(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	// Strongest capability rival in DefaultCompetitors is OpenAI (10).
	before := s.Competitors[0].Quality[model.DimCapability]
	spec, _ := balance.EventByID(b.Events, balance.EvRivalBreak)
	ns := fireEvent(s, spec, b)
	after := ns.Competitors[0].Quality[model.DimCapability]
	want := before * (1 + balance.EvRivalBreakQualityPct)
	if diff := after - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("rival capability = %v, want %v", after, want)
	}
	if ns.Events.Pending[0].Target != 0 {
		t.Fatalf("Target = %d, want 0 (OpenAI)", ns.Events.Pending[0].Target)
	}
}

func TestIncidentLossDeferredToResolve(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	spec, _ := balance.EventByID(b.Events, balance.EvIncident)
	ns := fireEvent(s, spec, b)
	if ns.Models[0].Users != s.Models[0].Users {
		t.Fatal("incident loss must not apply at fire time")
	}
	// Default (低調): full loss + lingering incident-chance modifier.
	ns2, err := resolveChoice(ns, 0, 1, false, b)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	wantConsumer := s.Models[0].Users * (1 - balance.EvIncidentLossPct)
	wantEnterprise := s.Models[1].Users * (1 - balance.EvIncidentEnterprisePct)
	if ns2.Models[0].Users != wantConsumer || ns2.Models[1].Users != wantEnterprise {
		t.Fatalf("users after quiet = %v/%v, want %v/%v",
			ns2.Models[0].Users, ns2.Models[1].Users, wantConsumer, wantEnterprise)
	}
	if len(ns2.Events.Active) != 1 || ns2.Events.Active[0].Effects.IncidentChanceMult != balance.EvIncidentQuietChance {
		t.Fatalf("expected lingering IncidentChanceMult, got %+v", ns2.Events.Active)
	}
	// Apology (choice 0): half loss, no lingering modifier.
	ns3, err := resolveChoice(ns, 0, 0, false, b)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	wantHalf := s.Models[0].Users * (1 - balance.EvIncidentLossPct/2)
	if ns3.Models[0].Users != wantHalf {
		t.Fatalf("users after apology = %v, want %v", ns3.Models[0].Users, wantHalf)
	}
	if len(ns3.Events.Active) != 0 {
		t.Fatal("apology must not leave a lingering modifier")
	}
}

func TestFirePaperTargetsBranchAndResolveDiscounts(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	spec, _ := balance.EventByID(b.Events, balance.EvPaper)
	ns := fireEvent(s, spec, b)
	branch := ns.Events.Pending[0].Target
	if branch < 0 || branch >= model.NumBranches {
		t.Fatalf("branch target = %d out of range", branch)
	}
	ns2, err := resolveChoice(ns, 0, 1, false, b)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	got := eventTechCostMult(ns2, model.TechBranch(branch))
	if got != balance.EvPaperAbsorbTechCost {
		t.Fatalf("absorb tech mult = %v, want %v", got, balance.EvPaperAbsorbTechCost)
	}
	ns3, err := resolveChoice(ns, 0, 0, false, b)
	if err != nil {
		t.Fatalf("resolve bet: %v", err)
	}
	if ns3.Resources.RnD >= ns.Resources.RnD {
		t.Fatal("bet choice must charge R&D")
	}
	if got := eventTechCostMult(ns3, model.TechBranch(branch)); got != balance.EvPaperBetTechCost {
		t.Fatalf("bet tech mult = %v, want %v", got, balance.EvPaperBetTechCost)
	}
}

func TestFireMarketCycleNoChoiceLogsDirectly(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	spec, _ := balance.EventByID(b.Events, balance.EvMarketCycle)
	ns := fireEvent(s, spec, b)
	if len(ns.Events.Pending) != 0 {
		t.Fatal("no-choice event must not create a pending entry")
	}
	if len(ns.Events.Active) != 1 || len(ns.Events.Log) != 1 {
		t.Fatalf("expected 1 active + 1 log, got %+v / %+v", ns.Events.Active, ns.Events.Log)
	}
	tam := ns.Events.Active[0].Effects.TAMMult
	if tam != balance.EvMarketBoomTAM && tam != balance.EvMarketBustTAM {
		t.Fatalf("TAMMult = %v, want boom or bust", tam)
	}
}

func TestResolveOpenSourceFollowReplacesEffects(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	spec, _ := balance.EventByID(b.Events, balance.EvOpenSourceWar)
	ns := fireEvent(s, spec, b)
	if ns.Events.Active[0].Effects.RefPriceMult != balance.EvOpenSourceRefPrice {
		t.Fatalf("fire effect = %+v", ns.Events.Active[0].Effects)
	}
	ns2, err := resolveChoice(ns, 0, 0, false, b)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	e := ns2.Events.Active[0].Effects
	if e.RefPriceMult != balance.EvOpenSourceFollowRef || e.UserGrowthMult != balance.EvOpenSourceFollowGrow {
		t.Fatalf("follow effects = %+v", e)
	}
}

func TestResolveScandalGrowthByChoice(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	spec, _ := balance.EventByID(b.Events, balance.EvRivalScandal)
	s = fireEvent(s, spec, b)
	rival := s.Events.Pending[0].Target
	if rival < 0 || rival >= len(s.Competitors) {
		t.Fatalf("scandal target = %d out of range", rival)
	}
	nsWatch, err := resolveChoice(s, 0, 1, false, b)
	if err != nil {
		t.Fatalf("resolve watch: %v", err)
	}
	if eventEffects(nsWatch, b).UserGrowthMult != balance.EvScandalWatchGrowth {
		t.Fatalf("watch growth = %v", eventEffects(nsWatch, b).UserGrowthMult)
	}
	nsPoach, err := resolveChoice(s, 0, 0, false, b)
	if err != nil {
		t.Fatalf("resolve poach: %v", err)
	}
	if eventEffects(nsPoach, b).UserGrowthMult != balance.EvScandalPoachGrowth {
		t.Fatalf("poach growth = %v", eventEffects(nsPoach, b).UserGrowthMult)
	}
}

func TestResolveRegulationComplyBoostsSafety(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	spec, _ := balance.EventByID(b.Events, balance.EvRegulation)
	s = fireEvent(s, spec, b)
	if eventEffects(s, b).SafetyWeightMult != balance.EvRegulationSafetyW {
		t.Fatal("regulation must add SafetyWeightMult at fire time")
	}
	before := s.Models[0].Quality[model.DimSafety]
	ns, err := resolveChoice(s, 0, 0, false, b)
	if err != nil {
		t.Fatalf("resolve comply: %v", err)
	}
	want := before * (1 + balance.EvRegulationComplyPct)
	if diff := ns.Models[0].Quality[model.DimSafety] - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("safety after comply = %v, want %v", ns.Models[0].Quality[model.DimSafety], want)
	}
}

func TestResolveBubbleCalmSoftensValuationHit(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	spec, _ := balance.EventByID(b.Events, balance.EvBubbleTalk)
	s = fireEvent(s, spec, b)
	if eventEffects(s, b).ValuationMult != balance.EvBubbleValuation {
		t.Fatal("bubble talk must dent valuation at fire time")
	}
	ns, err := resolveChoice(s, 0, 0, false, b)
	if err != nil {
		t.Fatalf("resolve calm: %v", err)
	}
	if eventEffects(ns, b).ValuationMult != balance.EvBubbleCalmValuation {
		t.Fatalf("calm valuation mult = %v, want %v",
			eventEffects(ns, b).ValuationMult, balance.EvBubbleCalmValuation)
	}
}

func TestResolveInvalidIndexAndChoice(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	if _, err := resolveChoice(s, 0, 1, false, b); err != ErrInvalidEventIndex {
		t.Fatalf("empty pending: err = %v, want ErrInvalidEventIndex", err)
	}
	spec, _ := balance.EventByID(b.Events, balance.EvChipShortage)
	s = fireEvent(s, spec, b)
	if _, err := resolveChoice(s, 0, 5, false, b); err != ErrInvalidEventChoice {
		t.Fatalf("bad choice: err = %v, want ErrInvalidEventChoice", err)
	}
}
