package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestCampaignStatusEstablishProgress(t *testing.T) {
	b := balance.Default()
	s := campaignRouteState(model.DoctrineConsumer, model.SegConsumer)
	st := CampaignStatus(s, b)
	if !st.Active || st.Doctrine != model.DoctrineConsumer || st.Stage != model.CampaignStageEstablish {
		t.Fatalf("status=%+v", st)
	}
	if st.Share < b.Campaign.EstablishShare {
		t.Fatalf("share=%v, want ≥ %v", st.Share, b.Campaign.EstablishShare)
	}
	if st.Progress < 1 {
		t.Fatalf("progress=%v, want 1 (gate complete)", st.Progress)
	}
	if st.Victory {
		t.Fatal("establish should not report victory")
	}
}

func TestCampaignStatusPriceOKUsesRouteModel(t *testing.T) {
	b := balance.Default()
	s := campaignRouteState(model.DoctrineEnterprise, model.SegEnterprise)
	s.Campaign.Stage = model.CampaignStageExpand
	// Unrelated consumer model is cheap; enterprise route model is at ref.
	s.Models = append(s.Models, model.Model{
		Online: true, Segment: model.SegConsumer, Price: 0.01, Users: 1, Quality: [4]float64{1, 1, 1, 1},
	})
	st := CampaignStatus(s, b)
	if !st.PriceOK {
		t.Fatalf("PriceOK should use enterprise route model at ref, status=%+v", st)
	}

	s.Models[0].Price = b.SegmentRefPrice[model.SegEnterprise] * 0.5
	st = CampaignStatus(s, b)
	if st.PriceOK {
		t.Fatalf("underpriced enterprise route model should fail PriceOK, status=%+v", st)
	}
}

func TestCampaignStatusDeveloperWinPriceAndCashflow(t *testing.T) {
	b := balance.Default()
	s := campaignRouteState(model.DoctrineDeveloper, model.SegDeveloper)
	s.Campaign.Stage = model.CampaignStageShowdown
	// developer-api raises ref (1.10); showdown requires price ≤ 90% EffectiveRefPrice.
	s.Campaign.Perks = []string{"developer-api", "developer-efficient"}
	ref := EffectiveRefPrice(s, model.SegDeveloper, b)
	s.Models[0].Price = ref // 100% effective ref
	st := CampaignStatus(s, b)
	if st.PriceOK {
		t.Fatalf("developer showdown PriceOK should fail at full ref, status=%+v", st)
	}

	s.Models[0].Price = ref * 0.85
	// High rent forces CashflowOK false (users still earn, but rent dominates).
	s.Compute.RentedInference = map[string]int{balance.EntryProcessID: 1_000_000}
	st = CampaignStatus(s, b)
	if !st.PriceOK {
		t.Fatalf("price at 85%% effective ref should be OK, status=%+v", st)
	}
	if st.CashflowOK {
		t.Fatalf("massive rent should fail CashflowOK, net=%v", NetCashPerSec(s, b))
	}
}

func TestRouteVictoryStatusDoesNotMutateDoctrine(t *testing.T) {
	b := balance.Default()
	s := campaignRouteState(model.DoctrineConsumer, model.SegConsumer)
	s.Campaign.Stage = model.CampaignStageExpand
	s.Campaign.Endless = true
	s.Campaign.Victory = model.DoctrineConsumer

	// Evaluate enterprise showdown gate on a copy.
	st := RouteVictoryStatus(s, b, model.DoctrineEnterprise)
	if st.Doctrine != model.DoctrineEnterprise || st.Stage != model.CampaignStageShowdown {
		t.Fatalf("route status=%+v", st)
	}
	if st.Victory {
		t.Fatal("RouteVictoryStatus must clear Victory on the view state")
	}
	// Original unchanged.
	if s.Campaign.Doctrine != model.DoctrineConsumer || s.Campaign.Stage != model.CampaignStageExpand {
		t.Fatalf("mutated source campaign: %+v", s.Campaign)
	}
	if !s.Campaign.Endless || s.Campaign.Victory != model.DoctrineConsumer {
		t.Fatalf("mutated source flags: %+v", s.Campaign)
	}
}

func TestCampaignRivalIntelResolvesRoadmapActions(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Campaign.Primary = model.RivalRoadmap{
		Company: "OpenAI", ActionIndex: 0, CyclesUntilAction: 2, IntelFull: true,
	}
	view, ok := CampaignRivalIntel(s, b, true)
	if !ok {
		t.Fatal("expected ok for OpenAI primary")
	}
	if view.Company != "OpenAI" || view.ConfirmedActionID != "openai-flagship" || view.RumoredActionID != "openai-platform" {
		t.Fatalf("view=%+v", view)
	}
	if view.CyclesUntilAction != 2 || !view.IntelFull {
		t.Fatalf("view=%+v", view)
	}

	// Wildcard path.
	s.Campaign.Wildcard = model.RivalRoadmap{Company: "DeepSeek", ActionIndex: 0, CyclesUntilAction: 3}
	w, ok := CampaignRivalIntel(s, b, false)
	if !ok || w.ConfirmedActionID != "deepseek-price-war" {
		t.Fatalf("wildcard=%+v ok=%v", w, ok)
	}
}

func TestCampaignRivalIntelRejectsUnknown(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Campaign.Primary = model.RivalRoadmap{Company: "NotACompany", ActionIndex: 0}
	if _, ok := CampaignRivalIntel(s, b, true); ok {
		t.Fatal("unknown company should return ok=false")
	}
	s.Campaign.Primary = model.RivalRoadmap{Company: "OpenAI", ActionIndex: 99}
	if _, ok := CampaignRivalIntel(s, b, true); ok {
		t.Fatal("out-of-range action index should return ok=false")
	}
	s.Campaign.Primary = model.RivalRoadmap{}
	if _, ok := CampaignRivalIntel(s, b, true); ok {
		t.Fatal("empty company should return ok=false")
	}
}

// At ActionIndex=len-1, execution wraps via modulo; rumored must telegraph Actions[0].
func TestCampaignRivalIntelWrapsRumoredAtLastIndex(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	// OpenAI has two actions: flagship (0), platform (1). Last index must rumor flagship.
	s.Campaign.Primary = model.RivalRoadmap{
		Company: "OpenAI", ActionIndex: 1, CyclesUntilAction: 3, IntelFull: true,
	}
	view, ok := CampaignRivalIntel(s, b, true)
	if !ok {
		t.Fatal("expected ok for OpenAI at last action index")
	}
	if view.ConfirmedActionID != "openai-platform" {
		t.Fatalf("confirmed=%q want openai-platform", view.ConfirmedActionID)
	}
	if view.RumoredActionID != "openai-flagship" {
		t.Fatalf("rumored=%q want openai-flagship (modulo wrap)", view.RumoredActionID)
	}
}

func TestNetCashPerSecPositiveAndNegative(t *testing.T) {
	b := balance.Default()
	// Revenue-only state: 100k users * $12 / MonthSec * RevenueMult > 0.
	s := model.GameState{
		Models: []model.Model{{Online: true, Segment: model.SegConsumer, Price: 12, Users: 100000}},
	}
	if net := NetCashPerSec(s, b); net <= 0 {
		t.Fatalf("NetCashPerSec=%v, want > 0", net)
	}

	// Rent-heavy: 1e6 N7 chips with no users → negative.
	s.Models = nil
	s.Compute.RentedInference = map[string]int{balance.EntryProcessID: 1_000_000}
	if net := NetCashPerSec(s, b); net >= 0 {
		t.Fatalf("NetCashPerSec=%v, want < 0", net)
	}
}

func TestCampaignQualityRankFiltersBySegment(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Models: []model.Model{
			// Strong consumer model must not pollute developer rank.
			{Online: true, Segment: model.SegConsumer, Quality: [4]float64{100, 100, 100, 100}},
			{Online: true, Segment: model.SegDeveloper, Quality: [4]float64{10, 10, 10, 10}},
		},
		Competitors: []model.Competitor{{Name: "R", Quality: [4]float64{50, 50, 50, 50}}},
	}
	// MarketRank (wrong for campaign) would rank player #1 using the consumer model.
	mr, _ := MarketRank(s, b, model.SegDeveloper)
	cr := campaignQualityRank(s, b, model.SegDeveloper)
	if mr == cr {
		t.Fatalf("campaignQualityRank should differ from unfiltered MarketRank: both=%d", mr)
	}
	if cr != 2 {
		t.Fatalf("campaignQualityRank=%d, want 2 (developer model loses to rival)", cr)
	}
}
