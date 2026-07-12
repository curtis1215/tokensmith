package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestCampaignRoadmapsDeterministicAndDistinct(t *testing.T) {
	b := balance.Default()
	a := model.GameState{Campaign: model.CampaignState{RandState: 42}}
	c := a
	a = initCampaignRoadmaps(a, model.DoctrineConsumer, b)
	c = initCampaignRoadmaps(c, model.DoctrineConsumer, b)
	if a.Campaign.Primary != c.Campaign.Primary || a.Campaign.Wildcard != c.Campaign.Wildcard {
		t.Fatalf("same seed diverged: %+v %+v", a.Campaign, c.Campaign)
	}
	if a.Campaign.Primary.Company == a.Campaign.Wildcard.Company {
		t.Fatal("rival roles must differ")
	}
}

func TestCampaignRoadmapsPrimaryMatchesDoctrine(t *testing.T) {
	b := balance.Default()
	// Seed a few times; primary must always be a Consumer primary-capable rival.
	allowed := map[string]bool{"OpenAI": true, "xAI": true, "Gemini": true}
	for seed := uint64(1); seed <= 20; seed++ {
		s := model.GameState{Campaign: model.CampaignState{RandState: seed}}
		s = initCampaignRoadmaps(s, model.DoctrineConsumer, b)
		if s.Campaign.Primary.Company == "" || s.Campaign.Wildcard.Company == "" {
			t.Fatalf("seed %d empty roadmaps: %+v", seed, s.Campaign)
		}
		if !allowed[s.Campaign.Primary.Company] {
			t.Fatalf("seed %d primary %q not in consumer primary set", seed, s.Campaign.Primary.Company)
		}
		if s.Campaign.Primary.Company == s.Campaign.Wildcard.Company {
			t.Fatalf("seed %d roles collided", seed)
		}
		if s.Campaign.Primary.ActionIndex != 0 || s.Campaign.Wildcard.ActionIndex != 0 {
			t.Fatalf("seed %d action index not zero: %+v", seed, s.Campaign)
		}
		if s.Campaign.Primary.CyclesUntilAction <= 0 || s.Campaign.Wildcard.CyclesUntilAction <= 0 {
			t.Fatalf("seed %d lead not scheduled: %+v", seed, s.Campaign)
		}
	}
}

func TestChooseDoctrineSeedsRoadmaps(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Models: []model.Model{{Online: true}}}
	s.Campaign.RandState = 7
	ns, err := Apply(s, model.ChooseDoctrine{Doctrine: model.DoctrineEnterprise}, b)
	if err != nil {
		t.Fatal(err)
	}
	if ns.Campaign.Primary.Company == "" || ns.Campaign.Wildcard.Company == "" {
		t.Fatalf("choose doctrine should seed roadmaps: %+v", ns.Campaign)
	}
	if ns.Campaign.Primary.Company == ns.Campaign.Wildcard.Company {
		t.Fatal("roles must differ")
	}
}

func TestPivotReseedsRoadmaps(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Resources: model.Resources{Cash: 100000, RnD: 50000}}
	s.Models = []model.Model{{Online: true, Users: 1000, Price: 12}}
	s.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand,
		Perks: []string{"consumer-premium"}, RandState: 99,
		Primary:  model.RivalRoadmap{Company: "OpenAI", ActionIndex: 1, CyclesUntilAction: 1},
		Wildcard: model.RivalRoadmap{Company: "DeepSeek", ActionIndex: 1, CyclesUntilAction: 1},
	}
	ns, err := Apply(s, model.PivotDoctrine{Doctrine: model.DoctrineEnterprise}, b)
	if err != nil {
		t.Fatal(err)
	}
	if ns.Campaign.Primary.Company == "" || ns.Campaign.Primary.ActionIndex != 0 {
		t.Fatalf("pivot should reseed roadmaps: %+v", ns.Campaign)
	}
}

func TestRoadmapNeverCompoundsBeyondFrontier(t *testing.T) {
	b := balance.Default()
	// Fixed player frontier; spam OpenAI flagship/platform for 100k cycles
	// with the full default roster so idle rivals also stay in the hard band.
	pm := onlineModel(100, b.RefPrice)
	comps := balance.DefaultCompetitors()
	if len(comps) < 7 {
		t.Fatalf("default roster too small: %d", len(comps))
	}
	s := model.GameState{
		Models:      []model.Model{pm},
		Competitors: comps,
		Campaign: model.CampaignState{
			Doctrine: model.DoctrineConsumer,
			Stage:    model.CampaignStageExpand,
			Primary:  model.RivalRoadmap{Company: "OpenAI", ActionIndex: 0, CyclesUntilAction: 1},
			Wildcard: model.RivalRoadmap{Company: "DeepSeek", ActionIndex: 0, CyclesUntilAction: 1000000}, // keep quiet
		},
	}
	const cycles = 100000
	for i := 0; i < cycles; i++ {
		// Force OpenAI action every cycle (lead already 1).
		s.Campaign.Primary.CyclesUntilAction = 1
		s = AdvanceCampaignCycle(s, b)
		// Keep player model online so global frontier stays player-dominated.
		if len(s.Models) == 0 || !s.Models[0].Online {
			t.Fatal("player model lost")
		}
	}
	if len(s.Competitors) != len(comps) {
		t.Fatalf("roster size changed: got %d want %d", len(s.Competitors), len(comps))
	}
	assertRivalsInsideBand(t, s, b, "after 100k campaign cycles")
}

func TestExecuteRivalActionClosesGapNotMultiplies(t *testing.T) {
	b := balance.Default()
	b.TrainBoostRivalPicks = 0 // isolate gap-close formula without investment boost
	pm := onlineModel(100, b.RefPrice)
	// Start inside the band [85,115] so clamp does not mask the formula.
	c := model.Competitor{Name: "OpenAI", Skill: q(1.0, 1.0, 1.0, 1.0)}
	c.Quality[model.DimCapability] = 90
	s := model.GameState{
		Models:      []model.Model{pm},
		Competitors: []model.Competitor{c},
		Campaign: model.CampaignState{
			Doctrine: model.DoctrineConsumer,
			Primary:  model.RivalRoadmap{Company: "OpenAI", ActionIndex: 0, CyclesUntilAction: 0},
		},
	}
	s.Progression.MaxUnlockedGen = 1
	s.Progression.Rivals = model.RivalEraState{Era: 1, Leaders: []string{"Nobody"}}
	// Target 100; gap-close 15% of 10 → 91.5 (multiply would be 90*1.15=103.5).
	ns, _, entry, ok := executeRivalAction(s, s.Campaign.Primary, b)
	if !ok || entry.Kind != model.ReportRivalAction {
		t.Fatalf("execute failed: ok=%v entry=%+v", ok, entry)
	}
	got := ns.Competitors[0].Quality[model.DimCapability]
	if !approx(got, 91.5) {
		t.Fatalf("quality = %v, want 91.5 (gap close); multiply would be 103.5", got)
	}
}

func TestExecuteRivalActionCounterHalvesProgress(t *testing.T) {
	b := balance.Default()
	b.TrainBoostRivalPicks = 0 // isolate counter impact without investment boost
	pm := onlineModel(100, b.RefPrice)
	c := model.Competitor{Name: "OpenAI", Skill: q(1.0, 1.0, 1.0, 1.0)}
	c.Quality[model.DimCapability] = 90
	s := model.GameState{
		Models:      []model.Model{pm},
		Competitors: []model.Competitor{c},
		Campaign: model.CampaignState{
			Doctrine:        model.DoctrineConsumer,
			Primary:         model.RivalRoadmap{Company: "OpenAI", ActionIndex: 0},
			CounterTarget:   "OpenAI",
			CounterActionID: "openai-flagship",
		},
	}
	s.Progression.MaxUnlockedGen = 1
	s.Progression.Rivals = model.RivalEraState{Era: 1, Leaders: []string{"Nobody"}}
	// impact 0.5 → close 7.5% of gap 10: 90 + 0.75 = 90.75
	ns, _, entry, ok := executeRivalAction(s, s.Campaign.Primary, b)
	if !ok || !entry.Countered {
		t.Fatalf("countered entry: ok=%v entry=%+v", ok, entry)
	}
	got := ns.Competitors[0].Quality[model.DimCapability]
	if !approx(got, 90.75) {
		t.Fatalf("countered quality = %v, want 90.75", got)
	}
	if ns.Campaign.CounterTarget != "" {
		t.Fatal("counter not consumed")
	}
}

func TestExecuteRivalActionPriceReportPreserved(t *testing.T) {
	b := balance.Default()
	// deepseek-price-war has RefPriceMult 0.85, DurationCycles 2.
	s := model.GameState{Competitors: balance.DefaultCompetitors()}
	s.Models = []model.Model{onlineModel(50, b.RefPrice)}
	s.Campaign = model.CampaignState{
		Doctrine: model.DoctrineDeveloper,
		Primary:  model.RivalRoadmap{Company: "DeepSeek", ActionIndex: 0}, // deepseek-price-war
	}
	ns, roadmap, entry, ok := executeRivalAction(s, s.Campaign.Primary, b)
	if !ok || entry.DetailID != "deepseek-price-war" {
		t.Fatalf("ok=%v entry=%+v", ok, entry)
	}
	if len(ns.Campaign.Active) == 0 || ns.Campaign.Active[0].CyclesRemaining != 2 {
		t.Fatalf("price mod missing: %+v", ns.Campaign.Active)
	}
	if roadmap.ActionIndex != 1 || roadmap.CyclesUntilAction <= 0 {
		t.Fatalf("roadmap not advanced: %+v", roadmap)
	}
	if entry.Kind != model.ReportRivalAction || entry.SubjectID != "DeepSeek" {
		t.Fatalf("report: %+v", entry)
	}
}

func TestMomentumSetAndLinearDecay(t *testing.T) {
	b := balance.Default()
	pm := onlineModel(100, b.RefPrice)
	c := model.Competitor{Name: "OpenAI", Skill: q(1.0, 1.0, 1.0, 1.0)}
	c.Quality[model.DimCapability] = 50
	s := model.GameState{
		Models:      []model.Model{pm},
		Competitors: []model.Competitor{c},
		Campaign: model.CampaignState{
			Doctrine: model.DoctrineConsumer,
			Primary:  model.RivalRoadmap{Company: "OpenAI", ActionIndex: 0},
		},
	}
	s.Progression.MaxUnlockedGen = 1
	s.Progression.Rivals = model.RivalEraState{Era: 1, Leaders: []string{"Nobody"}}
	ns, _, _, ok := executeRivalAction(s, s.Campaign.Primary, b)
	if !ok {
		t.Fatal("execute failed")
	}
	// flagship progress 0.15 → momentum = 0.15*0.25 = 0.0375; cycles from catalog
	m := ns.Competitors[0]
	wantMom := 0.15 * 0.25
	if !approx(m.MomentumPct[model.DimCapability], wantMom) {
		t.Fatalf("momentum = %v, want %v", m.MomentumPct[model.DimCapability], wantMom)
	}
	if m.MomentumCycles <= 0 {
		t.Fatalf("MomentumCycles = %d, want > 0", m.MomentumCycles)
	}
	cycles := m.MomentumCycles
	// Age once: pct *= (cycles-1)/cycles
	aged := ageRivalMomentum(ns)
	am := aged.Competitors[0]
	if am.MomentumCycles != cycles-1 {
		t.Fatalf("cycles after age = %d, want %d", am.MomentumCycles, cycles-1)
	}
	wantAged := wantMom * float64(cycles-1) / float64(cycles)
	if cycles == 1 {
		wantAged = 0
	}
	if !approx(am.MomentumPct[model.DimCapability], wantAged) {
		t.Fatalf("aged momentum = %v, want %v", am.MomentumPct[model.DimCapability], wantAged)
	}
}

func TestMomentumClearedOnEraReset(t *testing.T) {
	// Reuse ensureRivalEraState era transition — already covered, but assert
	// roadmap momentum specifically gets zeroed when gen era advances.
	b := balance.Default()
	comps := balance.DefaultCompetitors()
	comps[0].MomentumPct[model.DimCapability] = 0.07
	comps[0].MomentumCycles = 5
	s := model.GameState{Competitors: comps}
	s.Progression.MaxUnlockedGen = 4
	s.Progression.Rivals = model.RivalEraState{Era: 2, Leaders: []string{"OpenAI", "Anthropic"}}
	s.Progression.MaxUnlockedGen = 5 // era 3
	ns := ensureRivalEraState(s, b)
	if ns.Competitors[0].MomentumCycles != 0 || ns.Competitors[0].MomentumPct[model.DimCapability] != 0 {
		t.Fatalf("era reset left momentum: %+v", ns.Competitors[0])
	}
}
