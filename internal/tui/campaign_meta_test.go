package tui

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestCampaignMetaCoversCatalog(t *testing.T) {
	cfg := balance.Default()

	for _, d := range []model.Doctrine{model.DoctrineConsumer, model.DoctrineEnterprise, model.DoctrineDeveloper} {
		if doctrineLabel(d) == "" || doctrineLabel(d) == string(d) {
			t.Fatalf("doctrine %q missing Chinese label: %q", d, doctrineLabel(d))
		}
	}
	for _, st := range []model.CampaignStage{
		model.CampaignStageEstablish, model.CampaignStageExpand,
		model.CampaignStageShowdown, model.CampaignStageWon,
	} {
		if campaignStageLabel(st) == "" || campaignStageLabel(st) == string(st) {
			t.Fatalf("stage %q missing Chinese label: %q", st, campaignStageLabel(st))
		}
	}
	for _, p := range cfg.Campaign.Perks {
		if perkLabel(p.ID) == "" || perkLabel(p.ID) == p.ID {
			t.Fatalf("perk %q missing Chinese label", p.ID)
		}
	}
	for _, a := range cfg.Campaign.RivalActions {
		if rivalActionLabel(a.ID) == "" || rivalActionLabel(a.ID) == a.ID {
			t.Fatalf("rival action %q missing Chinese label", a.ID)
		}
	}
	for _, k := range []model.CampaignReportKind{
		model.ReportDoctrineChosen, model.ReportStageAdvanced, model.ReportRivalAction,
		model.ReportShowdown, model.ReportVictory, model.ReportFinancialRisk,
	} {
		if reportKindLabel(k) == "" || reportKindLabel(k) == string(k) {
			t.Fatalf("report kind %q missing Chinese label", k)
		}
	}
	for _, d := range []model.DirectiveKind{
		model.DirectiveRoutePush, model.DirectiveCounter, model.DirectiveIntel,
	} {
		if directiveLabel(d) == "" || directiveLabel(d) == string(d) {
			t.Fatalf("directive %q missing Chinese label", d)
		}
	}
}

func TestCampaignMetaUnknownFallback(t *testing.T) {
	if got := labelOrID(perkLabels, "mystery-perk"); got != "mystery-perk" {
		t.Fatalf("unknown perk: got %q", got)
	}
	if got := rivalActionLabel("mystery-action"); got != "mystery-action" {
		t.Fatalf("unknown action: got %q", got)
	}
	if got := reportKindLabel(model.CampaignReportKind("mystery-kind")); got != "mystery-kind" {
		t.Fatalf("unknown report kind: got %q", got)
	}
	if got := doctrineLabel(model.Doctrine("mystery")); got != "mystery" {
		t.Fatalf("unknown doctrine: got %q", got)
	}
	if got := campaignStageLabel(model.CampaignStage("mystery")); got != "mystery" {
		t.Fatalf("unknown stage: got %q", got)
	}
	if got := directiveLabel(model.DirectiveKind("mystery")); got != "mystery" {
		t.Fatalf("unknown directive: got %q", got)
	}
}
