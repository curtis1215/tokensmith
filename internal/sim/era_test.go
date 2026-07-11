package sim

import (
	"math"
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestEraBreakthroughCostBaseAndPrimary(t *testing.T) {
	g6, err := balance.Generation(6)
	if err != nil {
		t.Fatal(err)
	}
	wantBase := 0.25 * g6.FrontierRnD

	// Era III open: Gen4 unlocked (end of Era II).
	s := model.GameState{}
	s.Progression.MaxUnlockedGen = 4
	s.Resources.RnD = 1e18

	cost, err := EraBreakthroughCost(s, 3, model.BranchAlgo)
	if err != nil {
		t.Fatalf("first branch cost: %v", err)
	}
	if !approx(cost, wantBase) {
		t.Fatalf("first branch cost = %v, want base %v", cost, wantBase)
	}

	// After primary is set, later branches cost 1.75×.
	s.Progression.Eras = []model.EraProgress{{
		Era: 3, HasPrimary: true, Primary: model.BranchAlgo, UnlockedMask: 1 << model.BranchAlgo,
	}}
	cost2, err := EraBreakthroughCost(s, 3, model.BranchInfra)
	if err != nil {
		t.Fatalf("later branch cost: %v", err)
	}
	if !approx(cost2, wantBase*1.75) {
		t.Fatalf("later branch cost = %v, want %v", cost2, wantBase*1.75)
	}
	// Primary branch already unlocked still reports 1.75× for a different check path
	// when HasPrimary is set; cost helper does not gate ownership.
	costPrimary, err := EraBreakthroughCost(s, 3, model.BranchBusiness)
	if err != nil || !approx(costPrimary, wantBase*1.75) {
		t.Fatalf("third branch cost = %v err=%v, want %v", costPrimary, err, wantBase*1.75)
	}
}

func TestEraIVBaseCostUsesGen8(t *testing.T) {
	g8, err := balance.Generation(8)
	if err != nil {
		t.Fatal(err)
	}
	s := eraIVReadyState()
	cost, err := EraBreakthroughCost(s, 4, model.BranchAlgo)
	if err != nil {
		t.Fatal(err)
	}
	if !approx(cost, 0.25*g8.FrontierRnD) {
		t.Fatalf("Era IV base = %v, want %v", cost, 0.25*g8.FrontierRnD)
	}
}

func TestEraOpenGates(t *testing.T) {
	s := model.GameState{}
	if EraOpen(s, 1) || EraOpen(s, 2) {
		t.Fatal("Eras I–II must not expose procedural breakthroughs")
	}
	if EraOpen(s, 3) {
		t.Fatal("Era III closed at Gen1")
	}
	s.Progression.MaxUnlockedGen = 4
	if !EraOpen(s, 3) {
		t.Fatal("Era III should open once Gen4 (end of Era II) is unlocked")
	}
	if EraOpen(s, 4) {
		t.Fatal("Era IV closed without Gen7 + 2 Era III breakthroughs")
	}
	// Gen7 alone is insufficient.
	s.Progression.MaxUnlockedGen = 7
	if EraOpen(s, 4) {
		t.Fatal("Era IV needs two Era III breakthroughs")
	}
	s.Progression.Eras = []model.EraProgress{{
		Era: 3, HasPrimary: true, Primary: model.BranchAlgo,
		UnlockedMask: (1 << model.BranchAlgo) | (1 << model.BranchInfra),
	}}
	if !EraOpen(s, 4) {
		t.Fatal("Era IV should open with Gen7 + two Era III breakthroughs")
	}
	// One breakthrough still blocked.
	s.Progression.Eras[0].UnlockedMask = 1 << model.BranchAlgo
	if EraOpen(s, 4) {
		t.Fatal("Era IV closed with only one Era III breakthrough")
	}
}

func TestApplyUnlockEraBreakthroughPrimaryAndMask(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Progression.MaxUnlockedGen = 5
	s.Resources.RnD = 1e18
	before := s.Resources.RnD

	ns, err := Apply(s, model.UnlockEraBreakthrough{Era: 3, Branch: model.BranchAlgo}, b)
	if err != nil {
		t.Fatalf("unlock: %v", err)
	}
	if s.Resources.RnD != before {
		t.Fatal("Apply mutated input R&D")
	}
	if len(ns.Progression.Eras) != 1 {
		t.Fatalf("eras len = %d, want 1", len(ns.Progression.Eras))
	}
	ep := ns.Progression.Eras[0]
	if ep.Era != 3 || !ep.HasPrimary || ep.Primary != model.BranchAlgo {
		t.Fatalf("primary not recorded: %+v", ep)
	}
	if ep.UnlockedMask&(1<<model.BranchAlgo) == 0 {
		t.Fatalf("mask bit not set: %b", ep.UnlockedMask)
	}
	g6, _ := balance.Generation(6)
	wantCost := 0.25 * g6.FrontierRnD
	if !approx(ns.Resources.RnD, before-wantCost) {
		t.Fatalf("RnD = %v, want %v", ns.Resources.RnD, before-wantCost)
	}

	// Second branch: 1.75×, keeps primary, sets another bit, eras stay sorted.
	before2 := ns.Resources.RnD
	ns2, err := Apply(ns, model.UnlockEraBreakthrough{Era: 3, Branch: model.BranchInfra}, b)
	if err != nil {
		t.Fatalf("second unlock: %v", err)
	}
	ep = ns2.Progression.Eras[0]
	if ep.Primary != model.BranchAlgo || !ep.HasPrimary {
		t.Fatalf("primary must not change: %+v", ep)
	}
	if ep.UnlockedMask&((1<<model.BranchAlgo)|(1<<model.BranchInfra)) != (1<<model.BranchAlgo)|(1<<model.BranchInfra) {
		t.Fatalf("mask wrong: %b", ep.UnlockedMask)
	}
	if !approx(ns2.Resources.RnD, before2-wantCost*1.75) {
		t.Fatalf("second cost RnD = %v, want %v", ns2.Resources.RnD, before2-wantCost*1.75)
	}
}

func TestApplyUnlockEraBreakthroughErrors(t *testing.T) {
	b := balance.Default()
	base := model.GameState{}
	base.Progression.MaxUnlockedGen = 5
	base.Resources.RnD = 1e18

	// Unopened era (Era IV).
	if _, err := Apply(base, model.UnlockEraBreakthrough{Era: 4, Branch: model.BranchAlgo}, b); err != ErrEraNotOpen {
		t.Fatalf("unopened: err = %v, want ErrEraNotOpen", err)
	}
	// Invalid branch.
	if _, err := Apply(base, model.UnlockEraBreakthrough{Era: 3, Branch: model.TechBranch(9)}, b); err != ErrInvalidEraBranch {
		t.Fatalf("branch: err = %v, want ErrInvalidEraBranch", err)
	}
	if _, err := Apply(base, model.UnlockEraBreakthrough{Era: 3, Branch: -1}, b); err != ErrInvalidEraBranch {
		t.Fatalf("branch neg: err = %v, want ErrInvalidEraBranch", err)
	}
	// Insufficient R&D (pure: no mutation).
	poor := base
	poor.Resources.RnD = 1
	out, err := Apply(poor, model.UnlockEraBreakthrough{Era: 3, Branch: model.BranchAlgo}, b)
	if err != ErrInsufficientRnD {
		t.Fatalf("poor: err = %v, want ErrInsufficientRnD", err)
	}
	if out.Resources.RnD != 1 || len(out.Progression.Eras) != 0 {
		t.Fatalf("insufficient R&D mutated state: %+v", out.Progression)
	}
	// Duplicate.
	ns, err := Apply(base, model.UnlockEraBreakthrough{Era: 3, Branch: model.BranchAlgo}, b)
	if err != nil {
		t.Fatal(err)
	}
	out2, err := Apply(ns, model.UnlockEraBreakthrough{Era: 3, Branch: model.BranchAlgo}, b)
	if err != ErrEraBreakthroughOwned {
		t.Fatalf("dup: err = %v, want ErrEraBreakthroughOwned", err)
	}
	if out2.Resources.RnD != ns.Resources.RnD {
		t.Fatalf("dup mutated R&D")
	}
	// Era I/II never open.
	if _, err := Apply(base, model.UnlockEraBreakthrough{Era: 1, Branch: model.BranchAlgo}, b); err != ErrEraNotOpen {
		t.Fatalf("era1: err = %v, want ErrEraNotOpen", err)
	}
}

func TestEraEffectsNeutralAndSqrt(t *testing.T) {
	neutral := EraEffects(model.GameState{})
	wantN := model.NeutralTechEffects()
	if neutral != wantN {
		t.Fatalf("no era progress should be neutral: %+v", neutral)
	}

	// One algo + one infra + one business + one alignment across eras.
	s := model.GameState{}
	s.Progression.Eras = []model.EraProgress{
		{Era: 3, HasPrimary: true, Primary: model.BranchAlgo, UnlockedMask: (1 << model.BranchAlgo) | (1 << model.BranchInfra)},
		{Era: 4, HasPrimary: true, Primary: model.BranchBusiness, UnlockedMask: (1 << model.BranchBusiness) | (1 << model.BranchAlignment)},
	}
	e := EraEffects(s)
	if !approx(e.QualityMult[model.DimCapability], 1+0.05*math.Sqrt(1)) {
		t.Errorf("algo quality = %v", e.QualityMult[model.DimCapability])
	}
	if !approx(e.InfraMult, 1+0.08*math.Sqrt(1)) {
		t.Errorf("infra = %v", e.InfraMult)
	}
	if !approx(e.UserGrowthMult, 1+0.06*math.Sqrt(1)) {
		t.Errorf("biz growth = %v", e.UserGrowthMult)
	}
	if !approx(e.RefPriceMult, 1+0.03*math.Sqrt(1)) {
		t.Errorf("biz ref = %v", e.RefPriceMult)
	}
	if !approx(e.QualityMult[model.DimSafety], 1+0.06*math.Sqrt(1)) {
		t.Errorf("align safety = %v", e.QualityMult[model.DimSafety])
	}
	if !approx(e.IncidentMult, 1/(1+0.10*math.Sqrt(1))) {
		t.Errorf("align incident = %v", e.IncidentMult)
	}
	// Efficiency/speed quality untouched by era formulas.
	if e.QualityMult[model.DimEfficiency] != 1 || e.QualityMult[model.DimSpeed] != 1 {
		t.Errorf("eff/speed should stay 1: %+v", e.QualityMult)
	}

	// Two algo breakthroughs → sqrt(2).
	s.Progression.Eras = []model.EraProgress{
		{Era: 3, UnlockedMask: 1 << model.BranchAlgo},
		{Era: 4, UnlockedMask: 1 << model.BranchAlgo},
	}
	e2 := EraEffects(s)
	if !approx(e2.QualityMult[model.DimCapability], 1+0.05*math.Sqrt(2)) {
		t.Errorf("two algo = %v, want %v", e2.QualityMult[model.DimCapability], 1+0.05*math.Sqrt(2))
	}
}

func TestEraEffectsCombineIntoTechEffects(t *testing.T) {
	b := balance.Default()
	s := model.GameState{UnlockedTech: []string{"algo-cap-1"}} // cap ×1.15
	s.Progression.Eras = []model.EraProgress{
		{Era: 3, UnlockedMask: 1 << model.BranchAlgo}, // +0.05
	}
	te := techEffects(s, b)
	want := 1.15 * (1 + 0.05)
	if !approx(te.QualityMult[model.DimCapability], want) {
		t.Fatalf("combined cap mult = %v, want %v", te.QualityMult[model.DimCapability], want)
	}
}

func TestEraQualityAffectsNewTrainingOnly(t *testing.T) {
	b := balance.Default()
	// Existing model quality must not be rewritten by era unlocks.
	s := model.GameState{
		Models: []model.Model{{Gen: 1, Online: true, Quality: [model.NumQualityDims]float64{10, 5, 5, 5}}},
	}
	s.Progression.MaxUnlockedGen = 5
	s.Resources.RnD = 1e18
	ns, err := Apply(s, model.UnlockEraBreakthrough{Era: 3, Branch: model.BranchAlgo}, b)
	if err != nil {
		t.Fatal(err)
	}
	if ns.Models[0].Quality[model.DimCapability] != 10 {
		t.Fatalf("existing model quality mutated: %v", ns.Models[0].Quality)
	}

	// New training completion applies era quality mult.
	ns.HasTraining = true
	ns.Compute.RentedTraining = map[string]int{"N7": 10000}
	ns.Training = model.TrainingJob{Gen: 1, Alloc: validAlloc(), Price: 12, WorkRemaining: 1}
	done := Tick(ns, 1, nil, b)
	// 0.4 * 25 * (1+0.05) = 10.5
	want := 0.4 * 25 * (1 + 0.05)
	if !approx(done.Models[len(done.Models)-1].Quality[model.DimCapability], want) {
		t.Fatalf("new model cap = %v, want %v", done.Models[len(done.Models)-1].Quality[model.DimCapability], want)
	}
}

func TestEraInfraAffectsRuntimeCompute(t *testing.T) {
	b := balance.Default()
	base := model.GameState{HasTraining: true}
	base.Compute.RentedTraining = map[string]int{"N7": 10}
	base.Training = model.TrainingJob{Gen: 1, WorkRemaining: 1e9}
	withEra := base
	withEra.Progression.Eras = []model.EraProgress{
		{Era: 3, UnlockedMask: 1 << model.BranchInfra},
	}
	nb := Tick(base, 1, nil, b)
	ne := Tick(withEra, 1, nil, b)
	if ne.Training.WorkRemaining >= nb.Training.WorkRemaining {
		t.Fatalf("era infra should speed training: %v vs %v", ne.Training.WorkRemaining, nb.Training.WorkRemaining)
	}
}

func TestEraBusinessAffectsUserGrowth(t *testing.T) {
	b := balance.Default()
	// Online model with users ramping; business breakthrough raises UserGrowthMult.
	base := model.GameState{
		Models: []model.Model{{
			Gen: 1, Online: true, Users: 0, Price: b.RefPrice,
			Quality: [model.NumQualityDims]float64{25, 0, 0, 0},
		}},
	}
	withBiz := base
	withBiz.Progression.Eras = []model.EraProgress{
		{Era: 3, UnlockedMask: 1 << model.BranchBusiness},
	}
	// One hour of growth.
	nb := Tick(base, 3600, nil, b)
	ne := Tick(withBiz, 3600, nil, b)
	if ne.Models[0].Users <= nb.Models[0].Users {
		t.Fatalf("era business should raise user growth: %v vs %v", ne.Models[0].Users, nb.Models[0].Users)
	}
}

func TestEraProgressStaysSorted(t *testing.T) {
	b := balance.Default()
	s := eraIVReadyState()
	s.Resources.RnD = 1e18
	// Unlock Era IV first while Era III already has entries — insert keeps sort.
	ns, err := Apply(s, model.UnlockEraBreakthrough{Era: 4, Branch: model.BranchAlgo}, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(ns.Progression.Eras) < 2 {
		t.Fatalf("expected both eras, got %+v", ns.Progression.Eras)
	}
	for i := 1; i < len(ns.Progression.Eras); i++ {
		if ns.Progression.Eras[i].Era < ns.Progression.Eras[i-1].Era {
			t.Fatalf("eras not sorted: %+v", ns.Progression.Eras)
		}
	}
}

// eraIVReadyState has Gen7 + two Era III breakthroughs (Era IV open).
func eraIVReadyState() model.GameState {
	s := model.GameState{}
	s.Progression.MaxUnlockedGen = 7
	s.Progression.Eras = []model.EraProgress{{
		Era: 3, HasPrimary: true, Primary: model.BranchAlgo,
		UnlockedMask: (1 << model.BranchAlgo) | (1 << model.BranchInfra),
	}}
	return s
}
