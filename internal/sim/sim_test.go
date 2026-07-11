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
	r.Researchers[model.Tier1] = 2 // 2*0.005 = 0.01 (pre-compression-fix units)
	r.Researchers[model.Tier2] = 1 // 1*0.015 = 0.015
	got := staffRnDPerSec(r, b)    // 0.025/s, scaled by RealSecCompression
	want := 0.025 / balance.RealSecCompression
	if !approx(got, want) {
		t.Fatalf("staffRnDPerSec = %v, want %v", got, want)
	}
}

func TestStaffRnDEfficiencyMult(t *testing.T) {
	b := balance.Default()
	r := model.Research{EfficiencyMult: 2.0}
	r.Researchers[model.Tier2] = 1 // 0.015 * 2.0 = 0.03, scaled by RealSecCompression
	want := 0.03 / balance.RealSecCompression
	if got := staffRnDPerSec(r, b); !approx(got, want) {
		t.Fatalf("staffRnDPerSec with mult = %v, want %v", got, want)
	}
}

func TestTickAddsStaffRnDAndAdvancesTime(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Research: model.Research{EfficiencyMult: 1.0}}
	s.Research.Researchers[model.Tier2] = 4 // 0.06/s pre-compression-fix
	ns := Tick(s, 10, nil, b)               // 0.06/s * 10s = 0.6, scaled by RealSecCompression
	want := 0.6 / balance.RealSecCompression
	if !approx(ns.Resources.RnD, want) {
		t.Fatalf("RnD = %v, want %v", ns.Resources.RnD, want)
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
		{InputTokens: 1000, OutputTokens: 500}, // (1000 + 2*500)/10 = 200
		{InputTokens: 0, OutputTokens: 1000},   // (0 + 2000)/10   = 200
	}
	if got := TokenRawRnD(events, b); !approx(got, 400) {
		t.Fatalf("TokenRawRnD = %v, want 400", got)
	}
}

func TestTokenRawRnDEmpty(t *testing.T) {
	if got := TokenRawRnD(nil, balance.Default()); got != 0 {
		t.Fatalf("TokenRawRnD(nil) = %v, want 0", got)
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

func TestTickStreakMultOnlyAffectsTokenRnD(t *testing.T) {
	b := balance.Default()
	b.StreakMult = 2.0
	staffOnly := model.GameState{Research: model.Research{EfficiencyMult: 1.0}}
	staffOnly.Research.Researchers[model.Tier2] = 4
	base := balance.Default() // StreakMult = 1.0 (neutral)
	nsStreak := Tick(staffOnly, 10, nil, b)
	nsBase := Tick(staffOnly, 10, nil, base)
	if !approx(nsStreak.Resources.RnD, nsBase.Resources.RnD) {
		t.Fatalf("StreakMult must not affect staff-only R&D: streak=%v base=%v", nsStreak.Resources.RnD, nsBase.Resources.RnD)
	}

	tokenOnly := model.GameState{}
	events := []model.TokenEvent{{OutputTokens: 1000}} // raw 200
	nsTokenStreak := Tick(tokenOnly, 1, events, b)
	nsTokenBase := Tick(tokenOnly, 1, events, base)
	if !approx(nsTokenStreak.Resources.RnD, 2*nsTokenBase.Resources.RnD) {
		t.Fatalf("StreakMult=2.0 should double token R&D: got %v, want %v", nsTokenStreak.Resources.RnD, 2*nsTokenBase.Resources.RnD)
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
	if !approx(oneShot.Resources.RnD, 14.25/balance.RealSecCompression) { // (3*0.005 + 2*0.04)*1.5 = 0.1425/s * 100s = 14.25, scaled
		t.Fatalf("expected RnD %v, got %v", 14.25/balance.RealSecCompression, oneShot.Resources.RnD)
	}
}

func TestTickTrainingProgress(t *testing.T) {
	b := balance.Default()
	s := model.GameState{HasTraining: true}
	s.Compute.RentedTraining = map[string]int{"N7": 2}
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
	s.Compute.RentedTraining = map[string]int{"N7": 10}
	s.Training = model.TrainingJob{
		Gen:           2,
		Alloc:         [model.NumQualityDims]float64{0.4, 0.2, 0.2, 0.2},
		Price:         12,
		WorkRemaining: 7200,
	}
	ns := Tick(s, 1000, nil, b)
	if ns.HasTraining {
		t.Fatalf("training should be done")
	}
	if len(ns.Models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(ns.Models))
	}
	m := ns.Models[0]
	if m.Online || m.Users != 0 || m.Name != "" {
		t.Fatalf("completed model should be draft: %+v", m)
	}
	if m.Gen != 2 || m.Price != 12 {
		t.Fatalf("model fields wrong: %+v", m)
	}
	if !approx(m.Quality[model.DimCapability], 18) {
		t.Errorf("capability = %v, want 18", m.Quality[model.DimCapability])
	}
	if !approx(m.Quality[model.DimSafety], 9) {
		t.Errorf("safety = %v, want 9", m.Quality[model.DimSafety])
	}
	if len(s.Models) != 0 {
		t.Errorf("Tick mutated input Models")
	}
}

func TestTickTrainingCompleteAllowsNewTraining(t *testing.T) {
	b := balance.Default()
	s := model.GameState{HasTraining: true, Resources: model.Resources{RnD: 1e9}}
	s.Compute.RentedTraining = map[string]int{"N7": 100}
	s.Training = model.TrainingJob{
		Gen: 1, Alloc: [model.NumQualityDims]float64{1, 0, 0, 0},
		Price: 12, WorkRemaining: 1,
	}
	s = Tick(s, 1, nil, b)
	if s.HasTraining || len(s.Models) != 1 || s.Models[0].Online {
		t.Fatalf("want one draft, no active job: %+v", s)
	}
	ns, err := Apply(s, model.StartTraining{
		Gen: 1, Segment: model.SegConsumer,
		Alloc: [model.NumQualityDims]float64{1, 0, 0, 0}, Price: 12,
	}, b)
	if err != nil {
		t.Fatalf("should allow new training while draft exists: %v", err)
	}
	if !ns.HasTraining {
		t.Fatal("expected new training job")
	}
}

func TestTickDeductsTrainingRent(t *testing.T) {
	b := balance.Default()
	n7, _ := balance.ProcessByID(b.Processes, "N7")
	s := model.GameState{}
	s.Compute.RentedTraining = map[string]int{"N7": 4}
	s.Resources.Cash = 100
	ns := Tick(s, 10, nil, b) // 4 * N7.RentPerSec * TrainRentMult * 10
	want := 100 - 4*n7.RentPerSec*b.TrainRentMult*10
	if !approx(ns.Resources.Cash, want) {
		t.Fatalf("Cash = %v, want %v", ns.Resources.Cash, want)
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

func pinLegacyBalance(b *balance.Config) {
	b.UserGrowthRate = 0.001
	b.UserTargetPerAppeal = 1000
	b.SegmentTargetScale = [model.NumSegments]float64{1000, 500, 800}
}

func onlineModel(cap, price float64) model.Model {
	m := model.Model{Online: true, Price: price}
	m.Quality[model.DimCapability] = cap
	return m
}

func TestTickUserGrowthTowardTarget(t *testing.T) {
	b := balance.Default()
	pinLegacyBalance(&b)
	// appeal = 50 * 0.4 = 20; price = ref → demandMult 1; target = 20*1000 = 20000.
	s := model.GameState{Models: []model.Model{onlineModel(50, b.RefPrice)}}
	ns := Tick(s, 1, nil, b)
	want := 20000.0 * (1.0 - math.Exp(-b.UserGrowthRate*1))
	if !approx(ns.Models[0].Users, want) {
		t.Fatalf("Users = %v, want %v", ns.Models[0].Users, want)
	}
	// input not mutated
	if s.Models[0].Users != 0 {
		t.Fatalf("Tick mutated input Users")
	}
}

func TestTickSkipsOutOfRangeSegment(t *testing.T) {
	b := balance.Default()
	// A corrupt save could carry a segment past NumSegments; the segment-indexed
	// lookups must not panic — the model is simply skipped.
	m := onlineModel(50, b.RefPrice)
	m.Segment = model.Segment(99)
	s := model.GameState{Models: []model.Model{m}}
	ns := Tick(s, 1, nil, b) // must not panic
	if ns.Models[0].Users != 0 {
		t.Fatalf("out-of-range segment model should not grow users, got %v", ns.Models[0].Users)
	}
}

func TestTickPriceElasticityReducesTarget(t *testing.T) {
	b := balance.Default()
	pinLegacyBalance(&b)
	// double the reference price → demandMult = (1/2)^1.5.
	s := model.GameState{Models: []model.Model{onlineModel(50, 2*b.RefPrice)}}
	ns := Tick(s, 1, nil, b)
	wantTarget := 20.0 * b.UserTargetPerAppeal * math.Pow(0.5, b.PriceElasticity) // appeal 20
	wantUsers := wantTarget * (1.0 - math.Exp(-b.UserGrowthRate*1))
	if !approx(ns.Models[0].Users, wantUsers) {
		t.Fatalf("Users = %v, want %v", ns.Models[0].Users, wantUsers)
	}
}

func TestTickHighPriceChurns(t *testing.T) {
	b := balance.Default()
	pinLegacyBalance(&b)
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
	// revenue uses pre-growth users: 1000 * 12 * 100 / MonthSec * RevenueMult
	want := 1000.0 * 12.0 * 100.0 / b.MonthSec * b.RevenueMult
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
	// A rival below its target rubber-bands UP toward Skill×frontier.
	c := model.Competitor{Name: "Rival"}
	c.Quality[model.DimCapability] = 10
	c.Skill[model.DimCapability] = 1.0
	pm := onlineModel(80, b.RefPrice) // player frontier cap 80
	s := model.GameState{Models: []model.Model{pm}, Competitors: []model.Competitor{c}}
	ns := Tick(s, 3600, nil, b) // target 80; moves a fraction of the gap up
	got := ns.Competitors[0].Quality[model.DimCapability]
	if got <= 10 || got > 80 {
		t.Fatalf("competitor cap = %v, want (10, 80]", got)
	}
	// purity: input competitor untouched
	if s.Competitors[0].Quality[model.DimCapability] != 10 {
		t.Fatalf("Tick mutated input competitor")
	}
}

func TestCompetitorTracksPlayerNoRunaway(t *testing.T) {
	b := balance.Default()
	// Speed up catch-up so the test can reach steady state.
	b.CompetitorCatchupRate = 0.00001
	c := model.Competitor{Name: "Rival"}
	c.Quality[model.DimCapability] = 10
	c.Skill[model.DimCapability] = 1.08 // top of specialty band
	pm := onlineModel(60, b.RefPrice)   // target 60×1.08 = 64.8; band ceil 69
	s := model.GameState{Models: []model.Model{pm}, Competitors: []model.Competitor{c}}
	for i := 0; i < 2000; i++ {
		s = Tick(s, 3600, nil, b)
	}
	got := s.Competitors[0].Quality[model.DimCapability]
	// Long horizon must not exceed the anti-runaway ceiling GF×1.15.
	if got > 60*1.15+1e-6 {
		t.Fatalf("competitor exceeded ceiling around 60: got %v", got)
	}
	if got <= 10 {
		t.Fatalf("competitor should catch up from 10, got %v", got)
	}
}

// Bounded league: with a Gen1-cap model online, rivals may not run away past GF×1.15.
func TestCompetitorCatchupRespectsGen1FarmWindow(t *testing.T) {
	b := balance.Default()
	c := model.Competitor{Name: "OpenAI"}
	c.Quality[model.DimCapability] = 10
	c.Skill[model.DimCapability] = 1.08
	pm := onlineModel(25, b.RefPrice)
	s := model.GameState{Models: []model.Model{pm}, Competitors: []model.Competitor{c}}
	const twoWeeks = 14 * 86400
	for left := float64(twoWeeks); left > 0; {
		step := 3600.0
		if step > left {
			step = left
		}
		s = Tick(s, step, nil, b)
		left -= step
	}
	got := s.Competitors[0].Quality[model.DimCapability]
	hi := 25 * rivalCeilPct
	if got > hi+1e-6 {
		t.Fatalf("after 14d Gen1-only, rival cap=%v above ceiling %v", got, hi)
	}
	if got <= 10 {
		t.Fatalf("rival should still inch up from 10, got %v", got)
	}
}

func rival(cap float64) model.Competitor {
	// All dims filled: hard band floor would otherwise lift zeroed dims and
	// inflate multi-dim appeal in share tests that only intend capability parity.
	c := model.Competitor{Name: "Rival"}
	c.Quality = q(cap, cap, cap, cap)
	c.Skill = q(1.0, 1.0, 1.0, 1.0)
	return c
}

func TestTickCompetitorHalvesUserTarget(t *testing.T) {
	b := balance.Default()
	pinLegacyBalance(&b)
	// Equal multi-dim quality so hard band floor cannot asymmetrically lift
	// zeroed rival dimensions (TimeFrontier / GF floor). skill=1; catch-up off.
	b.CompetitorCatchupRate = 0
	pm := onlineModel(50, b.RefPrice)
	pm.Quality = q(50, 50, 50, 50)
	s := model.GameState{
		Models:      []model.Model{pm},
		Competitors: []model.Competitor{rival(50)},
	}
	ns := Tick(s, 1, nil, b)
	// appeal equals rival → share 0.5; target half of monopoly.
	want := 10000.0 * (1.0 - math.Exp(-b.UserGrowthRate*1))
	// Monopoly target for full quality would be larger; pinLegacy SegmentTargetScale
	// with equal share: use the same historical want when cap-weight only was
	// considered. With full quality both sides, recompute from post-tick GF band.
	// Equal rivals → users must be half of a no-competitor control.
	solo := Tick(model.GameState{Models: []model.Model{pm}}, 1, nil, b)
	want = solo.Models[0].Users * 0.5
	if !approx(ns.Models[0].Users, want) {
		t.Fatalf("Users = %v, want %v (halved by equal competitor; solo=%v)",
			ns.Models[0].Users, want, solo.Models[0].Users)
	}
}

func TestTickStrongCompetitorChurnsUsers(t *testing.T) {
	b := balance.Default()
	pinLegacyBalance(&b)
	b.CompetitorCatchupRate = 0      // freeze league approach; ceiling still clamps
	m := onlineModel(50, b.RefPrice) // appeal 20
	// Rival quality is ceiling-clamped to GF×1.15 (~57.5 → appeal ~23, share ~0.465),
	// so equilibrium target is ~9300. Start above that so competition still churns.
	m.Users = 12000
	s := model.GameState{
		Models:      []model.Model{m},
		Competitors: []model.Competitor{rival(200)},
	}
	ns := Tick(s, 1, nil, b)
	if ns.Models[0].Users >= 12000 {
		t.Fatalf("Users = %v, want < 12000 (churn vs strong competitor)", ns.Models[0].Users)
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
	want := 40.0 * b.SegmentTargetScale[model.SegDeveloper] * (1.0 - math.Exp(-b.UserGrowthRate*1))
	if !approx(ns.Models[0].Users, want) {
		t.Fatalf("developer users = %v, want %v", ns.Models[0].Users, want)
	}
}

func TestTickDeductsInferenceRent(t *testing.T) {
	b := balance.Default()
	n7, _ := balance.ProcessByID(b.Processes, "N7")
	s := model.GameState{}
	s.Compute.RentedInference = map[string]int{"N7": 5}
	s.Resources.Cash = 100
	ns := Tick(s, 10, nil, b) // 5 * N7.RentPerSec * 10
	want := 100 - 5*n7.RentPerSec*10
	if !approx(ns.Resources.Cash, want) {
		t.Fatalf("Cash = %v, want %v", ns.Resources.Cash, want)
	}
}

func TestTickRecordsInferenceLoad(t *testing.T) {
	b := balance.Default()
	m := onlineModel(50, b.RefPrice)
	m.Users = 5000
	s := model.GameState{Models: []model.Model{m}}
	s.Compute.RentedInference = map[string]int{"N7": 1e9} // plenty → no churn
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
	low.Compute.RentedInference = map[string]int{"N7": 1} // overloaded (10 > 1)
	high := model.GameState{Models: []model.Model{m}}
	high.Compute.RentedInference = map[string]int{"N7": 1e9} // served
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
	s := model.GameState{Models: []model.Model{m}} // no rented inference
	served := model.GameState{Models: []model.Model{m}}
	served.Compute.RentedInference = map[string]int{"N7": 1e9}
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
	s := model.GameState{HasTraining: true}                             // no rented
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
	base.Compute.RentedTraining = map[string]int{"N7": 10}
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
	base.Compute.RentedInference = map[string]int{"N7": 1} // overloaded
	withOps := base
	withOps.Ops = 20
	nb := Tick(base, 1, nil, b)
	no := Tick(withOps, 1, nil, b)
	if no.Models[0].Users <= nb.Models[0].Users {
		t.Fatalf("ops should reduce churn: %v vs %v", no.Models[0].Users, nb.Models[0].Users)
	}
}

// Large TUI ticks (dt=3600) must not thrash users 0↔market-target when inference
// is overloaded. After an hour-scale tick, users should settle near the count
// capacity can serve (capacity / InferenceLoadPerUser), not collapse to 0.
func TestTickLargeDtServingSettlesAtCapacityNotZero(t *testing.T) {
	b := balance.Default()
	m := onlineModel(25, b.SegmentRefPrice[model.SegConsumer])
	m.Users = 30000 // load = 3.0 with default 0.0001/user
	s := model.GameState{Models: []model.Model{m}}
	s.Compute.RentedInference = map[string]int{"N7": 1} // capacity 1 → max ~10k users
	const hour = 3600.0
	ns := Tick(s, hour, nil, b)
	got := ns.Models[0].Users
	// Continuous approach: with rate*dt ≫ 1, load → capacity ⇒ users → 1/0.0001 = 10000.
	// Allow a band; must not be wiped near 0 or stay at the 30k start.
	if got < 5000 || got > 15000 {
		t.Fatalf("after 1h overload, users=%v; want near capacity cap ~10000 (not 0 or 30k)", got)
	}
	// A second hour should stay stable near the cap (no 0/high oscillation).
	ns2 := Tick(ns, hour, nil, b)
	if ns2.Models[0].Users < 5000 || ns2.Models[0].Users > 15000 {
		t.Fatalf("second hour users=%v; thrashing instead of settling", ns2.Models[0].Users)
	}
}

func TestTechQualityMultOnTrainedModel(t *testing.T) {
	b := balance.Default()
	s := model.GameState{HasTraining: true, UnlockedTech: []string{"algo-cap-1"}} // cap ×1.15
	s.Compute.RentedTraining = map[string]int{"N7": 1000}
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
	if !approx(ns.Training.WorkRemaining, 900000*0.9) { // 810000
		t.Errorf("WorkRemaining = %v, want 810000", ns.Training.WorkRemaining)
	}
}

func TestTechInfraSpeedsTraining(t *testing.T) {
	b := balance.Default()
	base := model.GameState{HasTraining: true}
	base.Compute.RentedTraining = map[string]int{"N7": 10}
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
	// monthlyRev 1000*12=12000, ×RevenueMult(2)=24000; *120 = 2.88M; users 1000*10=10000; cash 50000 → 2.94M
	if !approx(Valuation(s, b), 2_940_000) {
		t.Fatalf("valuation = %v, want 2940000", Valuation(s, b))
	}

	// Valuation must scale with RevenueMult: doubling it should raise valuation.
	bHigh := b
	bHigh.RevenueMult = b.RevenueMult * 2
	if !(Valuation(s, bHigh) > Valuation(s, b)) {
		t.Fatalf("valuation should increase with higher RevenueMult: base=%v high=%v", Valuation(s, b), Valuation(s, bHigh))
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

func TestTickStarSalary(t *testing.T) {
	b := balance.Default()
	s := model.GameState{HiredStars: []string{"aria-chen"}} // salary 0.02/s
	s.Resources.Cash = 100
	ns := Tick(s, 10, nil, b)
	// aria salary 0.02*10 = 0.2 (aria also adds R&D but not cash)
	if !approx(ns.Resources.Cash, 100-0.02*10) {
		t.Fatalf("Cash = %v, want %v", ns.Resources.Cash, 100-0.02*10)
	}
}

func TestTickStarRnDBonus(t *testing.T) {
	b := balance.Default()
	base := model.GameState{}
	withStar := model.GameState{HiredStars: []string{"aria-chen"}} // +300 R&D/s
	nb := Tick(base, 1, nil, b)
	nw := Tick(withStar, 1, nil, b)
	if nw.Resources.RnD <= nb.Resources.RnD {
		t.Fatalf("star should add R&D: %v vs %v", nw.Resources.RnD, nb.Resources.RnD)
	}
}

func TestStarQualityMult(t *testing.T) {
	b := balance.Default()
	s := model.GameState{HasTraining: true, HiredStars: []string{"aria-chen"}} // cap ×1.22
	s.Compute.RentedTraining = map[string]int{"N7": 1000}
	s.Training = model.TrainingJob{Gen: 2, Alloc: [model.NumQualityDims]float64{0.4, 0.2, 0.2, 0.2}, WorkRemaining: 1}
	ns := Tick(s, 1, nil, b)
	if !approx(ns.Models[0].Quality[model.DimCapability], 0.4*45*1.22) { // 21.96
		t.Fatalf("capability = %v, want %v", ns.Models[0].Quality[model.DimCapability], 0.4*45*1.22)
	}
}

func TestStarInfraSpeedsTraining(t *testing.T) {
	b := balance.Default()
	base := model.GameState{HasTraining: true}
	base.Compute.RentedTraining = map[string]int{"N7": 10}
	base.Training = model.TrainingJob{Gen: 1, WorkRemaining: 1e9}
	withStar := base
	withStar.HiredStars = []string{"kenji-tanaka"} // InfraMult 1.12
	nb := Tick(base, 1, nil, b)
	nw := Tick(withStar, 1, nil, b)
	if nw.Training.WorkRemaining >= nb.Training.WorkRemaining {
		t.Fatalf("star infra should speed training: %v vs %v", nw.Training.WorkRemaining, nb.Training.WorkRemaining)
	}
}

func TestStarGrowthBoostsUsers(t *testing.T) {
	b := balance.Default()
	base := model.GameState{Models: []model.Model{onlineModel(50, b.RefPrice)}}
	withStar := model.GameState{Models: []model.Model{onlineModel(50, b.RefPrice)}, HiredStars: []string{"marcus-cole"}} // 1.30
	nb := Tick(base, 1, nil, b)
	nw := Tick(withStar, 1, nil, b)
	if nw.Models[0].Users <= nb.Models[0].Users {
		t.Fatalf("star growth should boost users: %v vs %v", nw.Models[0].Users, nb.Models[0].Users)
	}
}

func TestUserGrowthClampedAtLargeDt(t *testing.T) {
	b := balance.Default()
	pinLegacyBalance(&b)
	// appeal 20, no rivals/tech/marketing → target = 20*1000 = 20000.
	// growthFactor = UserGrowthRate(0.001) * dt(3600) = 3.6, must clamp to 1.
	s := model.GameState{Models: []model.Model{onlineModel(50, b.RefPrice)}}
	ns := Tick(s, 3600, nil, b)
	u := ns.Models[0].Users
	if u < 0 {
		t.Fatalf("users went negative: %v", u)
	}
	if u > 20000.0001 {
		t.Fatalf("users overshot target (unstable Euler step): %v > 20000", u)
	}
}

func TestTickUserGrowthExponentialNotInstant(t *testing.T) {
	b := balance.Default()
	// Force known rate for the assertion regardless of balance defaults during this task.
	b.UserGrowthRate = 3.5e-5
	b.SegmentTargetScale[model.SegConsumer] = 20000
	m := onlineModel(25, b.SegmentRefPrice[model.SegConsumer]) // appeal 10 if weights 0.4
	m.Users = 0
	// No competitors → share 1 → target = 10 * 20000 = 200000
	s := model.GameState{Models: []model.Model{m}}
	ns := Tick(s, 3600, nil, b) // one sim hour
	// remaining = exp(-3.5e-5*3600)=exp(-0.126)≈0.881 → users≈0.119*target
	if ns.Models[0].Users <= 0 {
		t.Fatal("expected some users after 1h")
	}
	if ns.Models[0].Users > 0.25*200000 {
		t.Fatalf("1h users=%v; want < 25%% of target (not instant fill)", ns.Models[0].Users)
	}
}

func TestTickUserGrowthEightHoursNear63Percent(t *testing.T) {
	b := balance.Default()
	b.UserGrowthRate = 3.5e-5
	b.SegmentTargetScale[model.SegConsumer] = 20000
	m := onlineModel(25, b.SegmentRefPrice[model.SegConsumer])
	s := model.GameState{Models: []model.Model{m}}
	const target = 200000.0
	for i := 0; i < 8; i++ {
		s = Tick(s, 3600, nil, b)
	}
	u := s.Models[0].Users
	if u < 0.50*target || u > 0.75*target {
		t.Fatalf("after 8h users=%v; want ~63%% of %v (50–75%%)", u, target)
	}
}
