package sim

import (
	"math"
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestStaffRnDPerSec(t *testing.T) {
	b := balance.Default()
	r := model.Research{EfficiencyMult: 1.0}
	r.Researchers[model.Tier1] = 2 // 2*5 = 10
	r.Researchers[model.Tier2] = 1 // 1*15 = 15
	got := staffRnDPerSec(r, b)     // 25/s
	if !approx(got, 25) {
		t.Fatalf("staffRnDPerSec = %v, want 25", got)
	}
}

func TestStaffRnDEfficiencyMult(t *testing.T) {
	b := balance.Default()
	r := model.Research{EfficiencyMult: 2.0}
	r.Researchers[model.Tier2] = 1 // 15 * 2.0 = 30
	if got := staffRnDPerSec(r, b); !approx(got, 30) {
		t.Fatalf("staffRnDPerSec with mult = %v, want 30", got)
	}
}

func TestTickAddsStaffRnDAndAdvancesTime(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Research: model.Research{EfficiencyMult: 1.0}}
	s.Research.Researchers[model.Tier2] = 4 // 60/s
	ns := Tick(s, 10, nil, b)               // 60/s * 10s = 600
	if !approx(ns.Resources.RnD, 600) {
		t.Fatalf("RnD = %v, want 600", ns.Resources.RnD)
	}
	if !approx(ns.GameTime, 10) {
		t.Fatalf("GameTime = %v, want 10", ns.GameTime)
	}
	// Tick must not mutate the input state.
	if s.Resources.RnD != 0 || s.GameTime != 0 {
		t.Fatalf("Tick mutated input: %+v", s)
	}
}

func TestTokenRawRnD(t *testing.T) {
	b := balance.Default()
	events := []model.TokenEvent{
		{InputTokens: 1000, OutputTokens: 500},  // (1000 + 2*500)/10 = 200
		{InputTokens: 0, OutputTokens: 1000},    // (0 + 2000)/10   = 200
	}
	if got := tokenRawRnD(events, b); !approx(got, 400) {
		t.Fatalf("tokenRawRnD = %v, want 400", got)
	}
}

func TestTokenRawRnDEmpty(t *testing.T) {
	if got := tokenRawRnD(nil, balance.Default()); got != 0 {
		t.Fatalf("tokenRawRnD(nil) = %v, want 0", got)
	}
}

func TestTickAddsTokenRnD(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Research: model.Research{EfficiencyMult: 1.0}}
	// no staff → only token R&D. 1000 output → (2000)/10 = 200.
	events := []model.TokenEvent{{OutputTokens: 1000}}
	ns := Tick(s, 1, events, b)
	if !approx(ns.Resources.RnD, 200) {
		t.Fatalf("RnD = %v, want 200", ns.Resources.RnD)
	}
}

func TestApplySoftCapBelowFull(t *testing.T) {
	eff, nw := applySoftCap(0, 1000, 200000, 0.3)
	if !approx(eff, 1000) || !approx(nw, 1000) {
		t.Fatalf("below full: eff=%v nw=%v, want 1000/1000", eff, nw)
	}
}

func TestApplySoftCapCrossingFull(t *testing.T) {
	// window at 199,000; raw 2,000 → 1,000 full + 1,000*0.3 = 1,300 effective
	eff, nw := applySoftCap(199000, 2000, 200000, 0.3)
	if !approx(eff, 1300) {
		t.Fatalf("crossing: eff=%v, want 1300", eff)
	}
	if !approx(nw, 201000) {
		t.Fatalf("crossing: nw=%v, want 201000", nw)
	}
}

func TestApplySoftCapAboveFull(t *testing.T) {
	// already above full → everything diminished
	eff, nw := applySoftCap(200000, 1000, 200000, 0.3)
	if !approx(eff, 300) || !approx(nw, 201000) {
		t.Fatalf("above: eff=%v nw=%v, want 300/201000", eff, nw)
	}
}

func TestTickSoftCapAccumulatesWindow(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Research: model.Research{EfficiencyMult: 1.0}}
	// event that yields raw 199000 R&D: output = 199000*10/2 = 995000
	ev1 := []model.TokenEvent{{OutputTokens: 995000}}
	s = Tick(s, 1, ev1, b)
	if !approx(s.WindowRnD, 199000) {
		t.Fatalf("WindowRnD after ev1 = %v, want 199000", s.WindowRnD)
	}
	// next raw 2000 (output 10000) → 1300 effective (1000 full + 300)
	before := s.Resources.RnD
	s = Tick(s, 1, []model.TokenEvent{{OutputTokens: 10000}}, b)
	if !approx(s.Resources.RnD-before, 1300) {
		t.Fatalf("effective token R&D = %v, want 1300", s.Resources.RnD-before)
	}
}

func TestTickWindowResets(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Research: model.Research{EfficiencyMult: 1.0}, WindowRnD: 199000, WindowElapsed: 86399}
	// advancing past the 86400s window boundary resets WindowRnD to 0,
	// so the next tokens are granted at full rate again.
	before := s.Resources.RnD
	s = Tick(s, 2, []model.TokenEvent{{OutputTokens: 10000}}, b) // raw 2000
	if !approx(s.WindowRnD, 2000) {
		t.Fatalf("WindowRnD after reset = %v, want 2000", s.WindowRnD)
	}
	if !approx(s.Resources.RnD-before, 2000) {
		t.Fatalf("token R&D after reset = %v, want 2000 (full rate)", s.Resources.RnD-before)
	}
}

func TestOfflineFastForwardEquivalenceStaffOnly(t *testing.T) {
	b := balance.Default()
	base := model.GameState{Research: model.Research{EfficiencyMult: 1.5}}
	base.Research.Researchers[model.Tier1] = 3
	base.Research.Researchers[model.Tier3] = 2

	// One big tick of 100s, no token events.
	oneShot := Tick(base, 100, nil, b)

	// 100 small ticks of 1s each.
	stepwise := base
	for range 100 {
		stepwise = Tick(stepwise, 1, nil, b)
	}

	if !approx(oneShot.Resources.RnD, stepwise.Resources.RnD) {
		t.Fatalf("fast-forward mismatch: oneShot=%v stepwise=%v",
			oneShot.Resources.RnD, stepwise.Resources.RnD)
	}
	if !approx(oneShot.GameTime, stepwise.GameTime) {
		t.Fatalf("GameTime mismatch: oneShot=%v stepwise=%v",
			oneShot.GameTime, stepwise.GameTime)
	}
	if !approx(oneShot.Resources.RnD, 14250) { // (3*5 + 2*40)*1.5 = 142.5/s * 100s = 14250
		t.Fatalf("expected RnD 14250, got %v", oneShot.Resources.RnD)
	}
}

func TestTickTrainingProgress(t *testing.T) {
	b := balance.Default()
	s := model.GameState{HasTraining: true}
	s.Compute.TrainingCapacity = 2
	s.Training = model.TrainingJob{Gen: 1, WorkRemaining: 1800}
	ns := Tick(s, 100, nil, b) // 2 GPU * 100s = 200 work done
	if !approx(ns.Training.WorkRemaining, 1600) {
		t.Fatalf("WorkRemaining = %v, want 1600", ns.Training.WorkRemaining)
	}
	if !ns.HasTraining || len(ns.Models) != 0 {
		t.Fatalf("should still be training, no model yet")
	}
}

func TestTickTrainingCompletes(t *testing.T) {
	b := balance.Default()
	s := model.GameState{HasTraining: true}
	s.Compute.TrainingCapacity = 10
	s.Training = model.TrainingJob{
		Gen:           2, // GenQualityCap[2] = 45
		Alloc:         [model.NumQualityDims]float64{0.4, 0.2, 0.2, 0.2},
		Price:         12,
		WorkRemaining: 7200,
	}
	ns := Tick(s, 1000, nil, b) // 10*1000 = 10000 >= 7200 → completes
	if ns.HasTraining {
		t.Fatalf("training should be done")
	}
	if len(ns.Models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(ns.Models))
	}
	m := ns.Models[0]
	if !m.Online || m.Gen != 2 || m.Price != 12 {
		t.Fatalf("model fields wrong: %+v", m)
	}
	if !approx(m.Quality[model.DimCapability], 18) { // 0.4 * 45
		t.Errorf("capability = %v, want 18", m.Quality[model.DimCapability])
	}
	if !approx(m.Quality[model.DimSafety], 9) { // 0.2 * 45
		t.Errorf("safety = %v, want 9", m.Quality[model.DimSafety])
	}
	// purity: input Models slice untouched
	if len(s.Models) != 0 {
		t.Errorf("Tick mutated input Models")
	}
}

func TestTickDeductsTrainingRent(t *testing.T) {
	b := balance.Default() // TrainRentPerGPUSec = 0.01
	s := model.GameState{}
	s.Compute.TrainingCapacity = 4
	s.Resources.Cash = 100
	ns := Tick(s, 10, nil, b) // 4 * 0.01 * 10 = 0.4
	if !approx(ns.Resources.Cash, 99.6) {
		t.Fatalf("Cash = %v, want 99.6", ns.Resources.Cash)
	}
}

func TestTickRentZeroWhenNoCapacity(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Resources.Cash = 100
	ns := Tick(s, 10, nil, b)
	if !approx(ns.Resources.Cash, 100) {
		t.Fatalf("Cash = %v, want 100 (no capacity, no rent)", ns.Resources.Cash)
	}
}

func onlineModel(cap, price float64) model.Model {
	m := model.Model{Online: true, Price: price}
	m.Quality[model.DimCapability] = cap
	return m
}

func TestTickUserGrowthTowardTarget(t *testing.T) {
	b := balance.Default()
	// appeal = 50 * 0.4 = 20; price = ref → demandMult 1; target = 20*1000 = 20000.
	s := model.GameState{Models: []model.Model{onlineModel(50, b.RefPrice)}}
	ns := Tick(s, 1, nil, b) // Users += (20000-0)*0.001*1 = 20
	if !approx(ns.Models[0].Users, 20) {
		t.Fatalf("Users = %v, want 20", ns.Models[0].Users)
	}
	// input not mutated
	if s.Models[0].Users != 0 {
		t.Fatalf("Tick mutated input Users")
	}
}

func TestTickPriceElasticityReducesTarget(t *testing.T) {
	b := balance.Default()
	// double the reference price → demandMult = (1/2)^1.5.
	s := model.GameState{Models: []model.Model{onlineModel(50, 2*b.RefPrice)}}
	ns := Tick(s, 1, nil, b)
	wantTarget := 20.0 * b.UserTargetPerAppeal * math.Pow(0.5, b.PriceElasticity) // appeal 20
	wantUsers := wantTarget * b.UserGrowthRate * 1
	if !approx(ns.Models[0].Users, wantUsers) {
		t.Fatalf("Users = %v, want %v", ns.Models[0].Users, wantUsers)
	}
}

func TestTickHighPriceChurns(t *testing.T) {
	b := balance.Default()
	m := onlineModel(50, 2*b.RefPrice) // target well below 30000
	m.Users = 30000
	s := model.GameState{Models: []model.Model{m}}
	ns := Tick(s, 1, nil, b)
	if ns.Models[0].Users >= 30000 {
		t.Fatalf("Users = %v, want < 30000 (churn)", ns.Models[0].Users)
	}
}

func TestTickSubscriptionRevenue(t *testing.T) {
	b := balance.Default()
	m := onlineModel(50, 12)
	m.Users = 1000
	s := model.GameState{Models: []model.Model{m}}
	ns := Tick(s, 100, nil, b)
	// revenue uses pre-growth users: 1000 * 12 * 100 / MonthSec
	want := 1000.0 * 12.0 * 100.0 / b.MonthSec
	if !approx(ns.Resources.Cash, want) {
		t.Fatalf("Cash = %v, want %v", ns.Resources.Cash, want)
	}
}

func TestTickNoRevenueWhenOffline(t *testing.T) {
	b := balance.Default()
	m := model.Model{Online: false, Price: 12, Users: 1000}
	s := model.GameState{Models: []model.Model{m}}
	ns := Tick(s, 100, nil, b)
	if !approx(ns.Resources.Cash, 0) {
		t.Fatalf("Cash = %v, want 0 (offline model)", ns.Resources.Cash)
	}
}

func TestTickAdvancesCompetitors(t *testing.T) {
	b := balance.Default()
	c := model.Competitor{Name: "Rival"}
	c.Quality[model.DimCapability] = 10
	c.GrowthPerSec[model.DimCapability] = 0.1
	s := model.GameState{Competitors: []model.Competitor{c}}
	ns := Tick(s, 10, nil, b) // 10 + 0.1*10 = 11
	if !approx(ns.Competitors[0].Quality[model.DimCapability], 11) {
		t.Fatalf("competitor cap = %v, want 11", ns.Competitors[0].Quality[model.DimCapability])
	}
	// purity: input competitor untouched
	if s.Competitors[0].Quality[model.DimCapability] != 10 {
		t.Fatalf("Tick mutated input competitor")
	}
}

func rival(cap float64) model.Competitor {
	c := model.Competitor{Name: "Rival"}
	c.Quality[model.DimCapability] = cap
	return c
}

func TestTickCompetitorHalvesUserTarget(t *testing.T) {
	b := balance.Default()
	// your model appeal 20 (cap 50 * 0.4). equal competitor appeal 20 → share 0.5.
	s := model.GameState{
		Models:      []model.Model{onlineModel(50, b.RefPrice)},
		Competitors: []model.Competitor{rival(50)}, // GrowthPerSec 0 → stays 20
	}
	ns := Tick(s, 1, nil, b) // target = 20*1000*1*0.5 = 10000; users = 10000*0.001 = 10
	if !approx(ns.Models[0].Users, 10) {
		t.Fatalf("Users = %v, want 10 (halved by equal competitor)", ns.Models[0].Users)
	}
}

func TestTickStrongCompetitorChurnsUsers(t *testing.T) {
	b := balance.Default()
	m := onlineModel(50, b.RefPrice) // appeal 20
	m.Users = 5000
	s := model.GameState{
		Models:      []model.Model{m},
		Competitors: []model.Competitor{rival(200)}, // appeal 80 → share 0.2 → target 4000 < 5000
	}
	ns := Tick(s, 1, nil, b)
	if ns.Models[0].Users >= 5000 {
		t.Fatalf("Users = %v, want < 5000 (churn vs strong competitor)", ns.Models[0].Users)
	}
}

func TestAppealOf(t *testing.T) {
	b := balance.Default()
	q := [model.NumQualityDims]float64{50, 0, 0, 0}
	if got := appealOf(q, b.QualityWeights); !approx(got, 20) { // 50 * 0.4
		t.Fatalf("appealOf = %v, want 20", got)
	}
}

func segModel(seg model.Segment, dim model.QualityDim, q, price float64) model.Model {
	m := model.Model{Online: true, Segment: seg, Price: price}
	m.Quality[dim] = q
	return m
}

func TestSegmentWeightsChangeAppeal(t *testing.T) {
	b := balance.Default()
	// A safety-only model earns more users in Enterprise (safety-weighted)
	// than in Consumer (capability-weighted), priced at each segment's ref price.
	consumer := segModel(model.SegConsumer, model.DimSafety, 50, b.SegmentRefPrice[model.SegConsumer])
	enterprise := segModel(model.SegEnterprise, model.DimSafety, 50, b.SegmentRefPrice[model.SegEnterprise])
	nc := Tick(model.GameState{Models: []model.Model{consumer}}, 1, nil, b)
	ne := Tick(model.GameState{Models: []model.Model{enterprise}}, 1, nil, b)
	if ne.Models[0].Users <= nc.Models[0].Users {
		t.Fatalf("enterprise safety users (%v) should exceed consumer (%v)",
			ne.Models[0].Users, nc.Models[0].Users)
	}
}

func TestSegmentRefPriceNeutralAtReference(t *testing.T) {
	b := balance.Default()
	// Priced exactly at the developer ref price → demandMult 1.
	// appeal = 40 (efficiency 100 * developer weight 0.4); target = 40*800*1*1 = 32000.
	dev := segModel(model.SegDeveloper, model.DimEfficiency, 100, b.SegmentRefPrice[model.SegDeveloper])
	ns := Tick(model.GameState{Models: []model.Model{dev}}, 1, nil, b)
	want := 40.0 * b.SegmentTargetScale[model.SegDeveloper] * b.UserGrowthRate // *1 tick
	if !approx(ns.Models[0].Users, want) {
		t.Fatalf("developer users = %v, want %v", ns.Models[0].Users, want)
	}
}

func TestTickDeductsInferenceRent(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Compute.InferenceCapacity = 5
	s.Resources.Cash = 100
	ns := Tick(s, 10, nil, b) // 5 * 0.006 * 10 = 0.3
	want := 100 - 5*b.InferenceRentPerGPUSec*10
	if !approx(ns.Resources.Cash, want) {
		t.Fatalf("Cash = %v, want %v", ns.Resources.Cash, want)
	}
}

func TestTickRecordsInferenceLoad(t *testing.T) {
	b := balance.Default()
	m := onlineModel(50, b.RefPrice)
	m.Users = 5000
	s := model.GameState{Models: []model.Model{m}}
	s.Compute.InferenceCapacity = 1e9 // plenty → no churn
	ns := Tick(s, 1, nil, b)
	want := ns.Models[0].Users * b.InferenceLoadPerUser
	if !approx(ns.Compute.InferenceLoad, want) {
		t.Fatalf("InferenceLoad = %v, want %v", ns.Compute.InferenceLoad, want)
	}
}

func TestTickInferenceOverloadChurns(t *testing.T) {
	b := balance.Default()
	m := onlineModel(50, b.RefPrice)
	m.Users = 100000 // load = 100000*0.0001 = 10
	low := model.GameState{Models: []model.Model{m}}
	low.Compute.InferenceCapacity = 1 // overloaded (10 > 1)
	high := model.GameState{Models: []model.Model{m}}
	high.Compute.InferenceCapacity = 1e9 // served
	nl := Tick(low, 1, nil, b)
	nh := Tick(high, 1, nil, b)
	if nl.Models[0].Users >= nh.Models[0].Users {
		t.Fatalf("overloaded users (%v) should be < served users (%v)",
			nl.Models[0].Users, nh.Models[0].Users)
	}
}

func TestTickZeroCapacityGrace(t *testing.T) {
	b := balance.Default()
	m := onlineModel(50, b.RefPrice)
	m.Users = 100000
	s := model.GameState{Models: []model.Model{m}} // InferenceCapacity 0
	served := model.GameState{Models: []model.Model{m}}
	served.Compute.InferenceCapacity = 1e9
	ns := Tick(s, 1, nil, b)
	nserved := Tick(served, 1, nil, b)
	// v0 grace: zero capacity behaves like fully served (no service churn)
	if !approx(ns.Models[0].Users, nserved.Models[0].Users) {
		t.Fatalf("zero-capacity grace: %v should equal served %v",
			ns.Models[0].Users, nserved.Models[0].Users)
	}
}

func TestEffectiveTrainingIncludesServers(t *testing.T) {
	b := balance.Default()
	s := model.GameState{HasTraining: true}
	s.Compute.TrainingCapacity = 0 // no rented
	s.Servers = []model.Server{{Pool: model.PoolTraining, Compute: 10}} // PowerKW 0 → no electricity
	s.Training = model.TrainingJob{Gen: 1, WorkRemaining: 100}
	ns := Tick(s, 1, nil, b) // effective training 10 → work -= 10 → 90
	if !approx(ns.Training.WorkRemaining, 90) {
		t.Fatalf("WorkRemaining = %v, want 90 (self-built training compute)", ns.Training.WorkRemaining)
	}
}

func TestSelfBuiltInferenceCapacityServes(t *testing.T) {
	b := balance.Default()
	m := onlineModel(50, b.RefPrice)
	m.Users = 100000 // load = 10
	low := model.GameState{Models: []model.Model{m}, Servers: []model.Server{{Pool: model.PoolInference, Compute: 1}}}
	high := model.GameState{Models: []model.Model{m}, Servers: []model.Server{{Pool: model.PoolInference, Compute: 1e9}}}
	nl := Tick(low, 1, nil, b)
	nh := Tick(high, 1, nil, b)
	if nl.Models[0].Users >= nh.Models[0].Users {
		t.Fatalf("overloaded self-built (%v) should be < served (%v)", nl.Models[0].Users, nh.Models[0].Users)
	}
}

func TestTickDeductsElectricity(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Servers = []model.Server{{Pool: model.PoolTraining, Compute: 24, PowerKW: 40}}
	s.Resources.Cash = 1000
	ns := Tick(s, 10, nil, b) // 40 * 0.001 * 10 = 0.4
	want := 1000 - 40*b.ElectricityPerKWSec*10
	if !approx(ns.Resources.Cash, want) {
		t.Fatalf("Cash = %v, want %v", ns.Resources.Cash, want)
	}
}

func TestTickDeductsSalary(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Research.Researchers[model.Tier2] = 3
	s.Engineers = 2
	s.Resources.Cash = 100
	ns := Tick(s, 10, nil, b)
	want := 100 - (3*b.ResearcherSalaryPerSec[model.Tier2]+2*b.EngineerSalaryPerSec)*10
	if !approx(ns.Resources.Cash, want) {
		t.Fatalf("Cash = %v, want %v", ns.Resources.Cash, want)
	}
}

func TestEngineersSpeedTraining(t *testing.T) {
	b := balance.Default()
	base := model.GameState{HasTraining: true}
	base.Compute.TrainingCapacity = 10
	base.Training = model.TrainingJob{Gen: 1, WorkRemaining: 1e9}
	withEng := base
	withEng.Engineers = 5 // infra mult 1.1
	nb := Tick(base, 1, nil, b)
	ne := Tick(withEng, 1, nil, b)
	if ne.Training.WorkRemaining >= nb.Training.WorkRemaining {
		t.Fatalf("engineers should speed training: %v vs %v", ne.Training.WorkRemaining, nb.Training.WorkRemaining)
	}
}

func TestMarketingBoostsUsers(t *testing.T) {
	b := balance.Default()
	base := model.GameState{Models: []model.Model{onlineModel(50, b.RefPrice)}}
	withMkt := model.GameState{Models: []model.Model{onlineModel(50, b.RefPrice)}, Marketing: 10}
	nb := Tick(base, 1, nil, b)
	nm := Tick(withMkt, 1, nil, b)
	if nm.Models[0].Users <= nb.Models[0].Users {
		t.Fatalf("marketing should boost users: %v vs %v", nm.Models[0].Users, nb.Models[0].Users)
	}
}

func TestOpsReducesServiceChurn(t *testing.T) {
	b := balance.Default()
	m := onlineModel(50, b.RefPrice)
	m.Users = 100000
	base := model.GameState{Models: []model.Model{m}}
	base.Compute.InferenceCapacity = 1 // overloaded
	withOps := base
	withOps.Ops = 20
	nb := Tick(base, 1, nil, b)
	no := Tick(withOps, 1, nil, b)
	if no.Models[0].Users <= nb.Models[0].Users {
		t.Fatalf("ops should reduce churn: %v vs %v", no.Models[0].Users, nb.Models[0].Users)
	}
}

func TestTechQualityMultOnTrainedModel(t *testing.T) {
	b := balance.Default()
	s := model.GameState{HasTraining: true, UnlockedTech: []string{"algo-cap-1"}} // cap ×1.15
	s.Compute.TrainingCapacity = 1000
	s.Training = model.TrainingJob{Gen: 2, Alloc: [model.NumQualityDims]float64{0.4, 0.2, 0.2, 0.2}, WorkRemaining: 1}
	ns := Tick(s, 1, nil, b)
	if !approx(ns.Models[0].Quality[model.DimCapability], 0.4*45*1.15) { // 20.7
		t.Fatalf("capability = %v, want %v", ns.Models[0].Quality[model.DimCapability], 0.4*45*1.15)
	}
}

func TestTechTrainCostAndWorkReduced(t *testing.T) {
	b := balance.Default()
	s := model.GameState{UnlockedTech: []string{"algo-train-1"}} // RnD ×0.85, work ×0.9
	s.Resources.RnD = 100000
	ns, err := Apply(s, model.StartTraining{Gen: 1, Alloc: [model.NumQualityDims]float64{0.4, 0.2, 0.2, 0.2}, Price: 12}, b)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !approx(ns.Resources.RnD, 100000-20000*0.85) { // 83000
		t.Errorf("RnD = %v, want 83000", ns.Resources.RnD)
	}
	if !approx(ns.Training.WorkRemaining, 1800*0.9) { // 1620
		t.Errorf("WorkRemaining = %v, want 1620", ns.Training.WorkRemaining)
	}
}

func TestTechInfraSpeedsTraining(t *testing.T) {
	b := balance.Default()
	base := model.GameState{HasTraining: true}
	base.Compute.TrainingCapacity = 10
	base.Training = model.TrainingJob{Gen: 1, WorkRemaining: 1e9}
	withTech := base
	withTech.UnlockedTech = []string{"infra-eff-1"} // InfraMult 1.1
	nb := Tick(base, 1, nil, b)
	nt := Tick(withTech, 1, nil, b)
	if nt.Training.WorkRemaining >= nb.Training.WorkRemaining {
		t.Fatalf("infra tech should speed training: %v vs %v", nt.Training.WorkRemaining, nb.Training.WorkRemaining)
	}
}

func TestTechGrowthBoostsUsers(t *testing.T) {
	b := balance.Default()
	base := model.GameState{Models: []model.Model{onlineModel(50, b.RefPrice)}}
	withTech := model.GameState{Models: []model.Model{onlineModel(50, b.RefPrice)}, UnlockedTech: []string{"biz-growth-1"}}
	nb := Tick(base, 1, nil, b)
	nt := Tick(withTech, 1, nil, b)
	if nt.Models[0].Users <= nb.Models[0].Users {
		t.Fatalf("growth tech should boost users: %v vs %v", nt.Models[0].Users, nb.Models[0].Users)
	}
}

func TestValuation(t *testing.T) {
	b := balance.Default()
	m := onlineModel(50, 12)
	m.Users = 1000
	s := model.GameState{Models: []model.Model{m}}
	s.Resources.Cash = 50000
	// monthlyRev 1000*12=12000; *120 = 1.44M; users 1000*10=10000; cash 50000 → 1.5M
	if !approx(Valuation(s, b), 1_500_000) {
		t.Fatalf("valuation = %v, want 1500000", Valuation(s, b))
	}
}

func TestTickTracksMilestones(t *testing.T) {
	b := balance.Default()
	m := onlineModel(50, 12)
	m.Users = 1000
	s := model.GameState{Models: []model.Model{m}}
	s.Resources.Cash = 50000
	ns := Tick(s, 1, nil, b)
	if ns.PeakValuation < 1_000_000 {
		t.Errorf("peak valuation not tracked: %v", ns.PeakValuation)
	}
	if ns.MilestonesReached < 1 {
		t.Errorf("should reach $1M milestone, reached=%d peak=%v", ns.MilestonesReached, ns.PeakValuation)
	}
}

func TestPeakValuationIsMonotonic(t *testing.T) {
	b := balance.Default()
	// a model whose users will decay (price way above ref → target ~0)
	m := onlineModel(50, 100*b.RefPrice)
	m.Users = 100000
	s := model.GameState{Models: []model.Model{m}}
	s.Resources.Cash = 1e7
	ns := Tick(s, 1, nil, b)
	peak1 := ns.PeakValuation
	// drop cash to force lower valuation; peak must not decrease
	ns.Resources.Cash = 0
	ns2 := Tick(ns, 1, nil, b)
	if ns2.PeakValuation < peak1 {
		t.Fatalf("peak valuation decreased: %v -> %v", peak1, ns2.PeakValuation)
	}
}

func TestPrestigeRnDMult(t *testing.T) {
	b := balance.Default()
	base := model.GameState{Research: model.Research{EfficiencyMult: 1}}
	base.Research.Researchers[model.Tier2] = 10 // 150 R&D/s
	withP := base
	withP.Prestige.UnlockedPrestige = []string{"rnd-mult-1"} // R&D ×1.1
	nb := Tick(base, 1, nil, b)
	np := Tick(withP, 1, nil, b)
	if np.Resources.RnD <= nb.Resources.RnD {
		t.Fatalf("prestige RnD mult should boost R&D: %v vs %v", np.Resources.RnD, nb.Resources.RnD)
	}
}

func TestPrestigeCashMult(t *testing.T) {
	b := balance.Default()
	m := onlineModel(50, 12)
	m.Users = 1000
	base := model.GameState{Models: []model.Model{m}}
	withP := model.GameState{Models: []model.Model{m}}
	withP.Prestige.UnlockedPrestige = []string{"cash-mult-1"} // cash ×1.1
	nb := Tick(base, 1, nil, b)
	np := Tick(withP, 1, nil, b)
	if np.Resources.Cash <= nb.Resources.Cash {
		t.Fatalf("prestige cash mult should boost revenue: %v vs %v", np.Resources.Cash, nb.Resources.Cash)
	}
}
