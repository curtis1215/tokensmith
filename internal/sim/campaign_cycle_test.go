package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestAdvanceCampaignCycleExecutesTelegraphedAction(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Competitors: balance.DefaultCompetitors()}
	s.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand,
		Primary:  model.RivalRoadmap{Company: "OpenAI", CyclesUntilAction: 1},
		Wildcard: model.RivalRoadmap{Company: "DeepSeek", CyclesUntilAction: 2},
	}
	before := s.Competitors[0].Quality[model.DimCapability]
	ns := AdvanceCampaignCycle(s, b)
	if ns.Campaign.Cycle != 1 || ns.Competitors[0].Quality[model.DimCapability] <= before {
		t.Fatalf("campaign=%+v", ns.Campaign)
	}
	if ns.Campaign.Primary.LastExecutedCycle != 1 {
		t.Fatalf("roadmap=%+v", ns.Campaign.Primary)
	}
}

func TestCampaignCycleCapsReportRing(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Campaign: model.CampaignState{Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand}}
	for i := 0; i < b.Campaign.ReportCap+5; i++ {
		s = AdvanceCampaignCycle(s, b)
	}
	if len(s.Campaign.Reports) != b.Campaign.ReportCap {
		t.Fatalf("reports=%d", len(s.Campaign.Reports))
	}
}

func TestAdvanceCampaignCycleNoDoctrineIsNoop(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Campaign: model.CampaignState{Cycle: 3}}
	ns := AdvanceCampaignCycle(s, b)
	if ns.Campaign.Cycle != 3 || len(ns.Campaign.Reports) != 0 {
		t.Fatalf("no-doctrine should noop: %+v", ns.Campaign)
	}
}

func TestCampaignCycleFinancialDistress(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Resources: model.Resources{Cash: -100},
		Campaign:  model.CampaignState{Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand},
	}
	ns := AdvanceCampaignCycle(s, b)
	if ns.Campaign.FinancialDistressCycles != 1 {
		t.Fatalf("distress=%d", ns.Campaign.FinancialDistressCycles)
	}
	if len(ns.Campaign.Reports) != 1 || len(ns.Campaign.Reports[0].Entries) == 0 {
		t.Fatalf("reports=%+v", ns.Campaign.Reports)
	}
	found := false
	for _, e := range ns.Campaign.Reports[0].Entries {
		if e.Kind == model.ReportFinancialRisk && e.Value == -100 {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing financial risk entry: %+v", ns.Campaign.Reports[0].Entries)
	}

	// Recover cash clears distress counter.
	ns.Resources.Cash = 1
	ns = AdvanceCampaignCycle(ns, b)
	if ns.Campaign.FinancialDistressCycles != 0 {
		t.Fatalf("distress should reset: %d", ns.Campaign.FinancialDistressCycles)
	}
}

func TestCampaignCompetitorsFrozenOnTickButMoveOnBoardCycle(t *testing.T) {
	b := balance.Default()
	// Pre-campaign: rubber-band still runs on Tick.
	c := model.Competitor{Name: "Rival"}
	c.Quality[model.DimCapability] = 10
	c.Skill[model.DimCapability] = 1.0
	pm := onlineModel(80, b.RefPrice)
	pre := model.GameState{Models: []model.Model{pm}, Competitors: []model.Competitor{c}}
	preTick := Tick(pre, 3600, nil, b)
	if preTick.Competitors[0].Quality[model.DimCapability] <= 10 {
		t.Fatalf("pre-campaign competitor should rubber-band, got %v", preTick.Competitors[0].Quality[model.DimCapability])
	}

	// Active campaign: Tick must not rubber-band; board cycle executes telegraphed action.
	s := model.GameState{
		Models:      []model.Model{pm},
		Competitors: balance.DefaultCompetitors(),
		Campaign: model.CampaignState{
			Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand,
			Primary:  model.RivalRoadmap{Company: "OpenAI", CyclesUntilAction: 1},
			Wildcard: model.RivalRoadmap{Company: "DeepSeek", CyclesUntilAction: 2},
		},
	}
	before := s.Competitors[0].Quality[model.DimCapability]
	afterTick := Tick(s, 3600, nil, b)
	if afterTick.Competitors[0].Quality[model.DimCapability] != before {
		t.Fatalf("active campaign Tick must freeze rivals: before=%v after=%v",
			before, afterTick.Competitors[0].Quality[model.DimCapability])
	}
	afterCycle := AdvanceCampaignCycle(s, b)
	if afterCycle.Competitors[0].Quality[model.DimCapability] <= before {
		t.Fatalf("board cycle should execute rival action: before=%v after=%v",
			before, afterCycle.Competitors[0].Quality[model.DimCapability])
	}
}
