package sim

import (
	"math"
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestCampaignEffectsMultiplySelectedPerks(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Campaign: model.CampaignState{Perks: []string{"consumer-premium", "consumer-scale"}}}
	e := campaignEffects(s, b)
	if !approx(e.RefPriceMult[model.SegConsumer], 1.15) || !approx(e.UserGrowthMult[model.SegConsumer], 1.08) {
		t.Fatalf("effects=%+v", e)
	}
	if !approx(e.InferenceLoadMult, 1.10) {
		t.Fatalf("effects=%+v", e)
	}
}

// TestCampaignEffectsIncludeSecondaryPerk ensures secondary tier-1 perks stack.
func TestCampaignEffectsIncludeSecondaryPerk(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Campaign: model.CampaignState{
		Secondary:     model.DoctrineDeveloper,
		SecondaryPerk: "developer-open",
	}}
	e := campaignEffects(s, b)
	if !approx(e.RefPriceMult[model.SegDeveloper], 0.90) || !approx(e.UserGrowthMult[model.SegDeveloper], 1.25) {
		t.Fatalf("secondary effects=%+v", e)
	}
}

// TestEffectiveRefPriceIncludesCampaignMult keeps TUI preview aligned with Tick.
func TestEffectiveRefPriceIncludesCampaignMult(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Campaign: model.CampaignState{Perks: []string{"consumer-premium"}}}
	base := EffectiveRefPrice(model.GameState{}, model.SegConsumer, b)
	got := EffectiveRefPrice(s, model.SegConsumer, b)
	want := base * 1.15
	if !approx(got, want) {
		t.Fatalf("EffectiveRefPrice = %v, want %v", got, want)
	}
}

// TestServableUsersIncludesCampaignLoadMult keeps capacity preview aligned with advanceServing load.
func TestServableUsersIncludesCampaignLoadMult(t *testing.T) {
	b := balance.Default()
	b.InferenceLoadPerUser = 0.0001
	s := model.GameState{Campaign: model.CampaignState{Perks: []string{"consumer-scale"}}}
	s.Compute.RentedInference = map[string]int{"N7": 10}
	got := ServableUsers(s, b)
	want := 10.0 / (0.0001 * 1.10)
	if !approx(got, want) {
		t.Fatalf("ServableUsers = %v, want %v", got, want)
	}
}

// TestEstimateUserTargetIncludesCampaignGrowth keeps publish preview aligned with advanceUsers.
func TestEstimateUserTargetIncludesCampaignGrowth(t *testing.T) {
	b := balance.Default()
	m := model.Model{
		Online:  true,
		Segment: model.SegConsumer,
		Price:   b.SegmentRefPrice[model.SegConsumer],
		Quality: [model.NumQualityDims]float64{50, 50, 50, 50},
	}
	plain := model.GameState{Models: []model.Model{m}}
	// Isolate growth via Active modifier (catalog perks often co-modify ref price).
	growth := model.NeutralCampaignEffects()
	growth.UserGrowthMult[model.SegConsumer] = 1.20
	boosted := plain
	boosted.Campaign = model.CampaignState{Active: []model.CampaignModifier{{CyclesRemaining: 1, Effects: growth}}}
	p := EstimateUserTarget(plain, 0, m.Price, b)
	g := EstimateUserTarget(boosted, 0, m.Price, b)
	if !approx(g, p*1.20) {
		t.Fatalf("EstimateUserTarget boosted=%v plain=%v ratio=%v", g, p, g/p)
	}
}

// TestTickSubscriptionRevenueIncludesCampaignRevenueMult keeps cash accrual aligned with Valuation.
func TestTickSubscriptionRevenueIncludesCampaignRevenueMult(t *testing.T) {
	b := balance.Default()
	m := model.Model{Online: true, Segment: model.SegDeveloper, Price: 12, Users: 1000, Quality: [model.NumQualityDims]float64{1, 1, 1, 1}}
	s := model.GameState{
		Models:   []model.Model{m},
		Campaign: model.CampaignState{Perks: []string{"developer-usage"}}, // RevenueMult developer 1.20
	}
	// Prevent growth from changing users during the revenue sample.
	b.UserGrowthRate = 0
	ns := Tick(s, 100, nil, b)
	want := 1000.0 * 12.0 * 1.20 * 100.0 / b.MonthSec * b.RevenueMult
	if !approx(ns.Resources.Cash, want) {
		t.Fatalf("Cash = %v, want %v", ns.Resources.Cash, want)
	}
}

// TestValuationIncludesCampaignRevenueMult keeps valuation aligned with cash revenue.
func TestValuationIncludesCampaignRevenueMult(t *testing.T) {
	b := balance.Default()
	m := model.Model{Online: true, Segment: model.SegDeveloper, Price: 12, Users: 1000}
	s := model.GameState{
		Models:   []model.Model{m},
		Campaign: model.CampaignState{Perks: []string{"developer-usage"}},
	}
	plain := Valuation(model.GameState{Models: []model.Model{m}}, b)
	got := Valuation(s, b)
	// Only monthlyRev term changes: multiply by 1.20.
	// Valuation = (monthlyRev*RevenueMultiple + users*UserValue + assets) * event mult
	// With cash=0 and no servers: plain monthly = 1000*12*RevenueMult; boosted = *1.20
	wantMonthly := 1000.0 * 12.0 * b.RevenueMult * 1.20
	want := wantMonthly*b.RevenueMultiple + 1000*b.UserValue
	if !approx(got, want) {
		t.Fatalf("Valuation = %v, want %v (plain=%v)", got, want, plain)
	}
	if got <= plain {
		t.Fatalf("campaign revenue mult must raise valuation: %v vs %v", got, plain)
	}
}

// TestAdvanceServingAppliesCampaignLoadAndChurn ensures overload uses load+churn mults.
func TestAdvanceServingAppliesCampaignLoadAndChurn(t *testing.T) {
	b := balance.Default()
	b.InferenceLoadPerUser = 1
	b.ServiceChurnRate = 1
	// Capacity 50, load without mult = 100 → overload.
	s := model.GameState{
		Models: []model.Model{{Online: true, Users: 100, Segment: model.SegEnterprise}},
		Campaign: model.CampaignState{
			Perks: []string{"enterprise-reliability", "enterprise-sales"},
			// reliability: ServiceChurnMult 0.75; sales: InferenceLoadMult 1.10
		},
	}
	s.Compute.RentedInference = map[string]int{} // capacity 0 without servers/rental
	// Give capacity of 50 via rented inference: Process compute values vary; set load path directly.
	// Use effectiveInference path: rent enough N7 that capacity is known.
	// Simpler: set Servers with known Compute.
	s.Servers = []model.Server{{Pool: model.PoolInference, Compute: 50}}
	// Disable engineer/tech scaling: infraEfficiency with 0 engineers = 1.
	// load = 100 * 1 * 1.10 = 110; capacity ≈ 50 * 1 * 1 * 1 = 50
	ns := advanceServing(s, 1, b)
	// newLoad = 50 + (110-50)*exp(-1*0.75*1*1) = 50 + 60*exp(-0.75)
	wantLoad := 50 + 60*math.Exp(-0.75)
	if !approx(ns.Compute.InferenceLoad, wantLoad) {
		t.Fatalf("InferenceLoad = %v, want %v", ns.Compute.InferenceLoad, wantLoad)
	}
}
