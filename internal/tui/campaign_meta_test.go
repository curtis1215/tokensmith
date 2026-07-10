package tui

import (
	"strings"
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
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

func TestRivalIntelFullVsPartialDisclosure(t *testing.T) {
	cfg := balance.Default()
	// openai-flagship: capability +15%, ref price ×1, lead 2, consumer segment.
	partial := renderRivalIntelBlock("主要宿敵", sim.RivalIntelView{
		Company: "OpenAI", ConfirmedActionID: "openai-flagship",
		RumoredActionID: "openai-platform", CyclesUntilAction: 1, IntelFull: false,
	}, cfg)
	if !strings.Contains(partial, "方向") || !strings.Contains(partial, "消費者") {
		t.Fatalf("partial intel must show direction+segment:\n%s", partial)
	}
	// Must not disclose quality %, price mult, or lead-cycle stats.
	for _, leak := range []string{"能力+", "價格×", "前置"} {
		if strings.Contains(partial, leak) {
			t.Fatalf("partial intel must not disclose %q:\n%s", leak, partial)
		}
	}

	full := renderRivalIntelBlock("主要宿敵", sim.RivalIntelView{
		Company: "OpenAI", ConfirmedActionID: "openai-flagship",
		RumoredActionID: "openai-platform", CyclesUntilAction: 1, IntelFull: true,
	}, cfg)
	for _, want := range []string{"能力+", "價格×", "前置"} {
		if !strings.Contains(full, want) {
			t.Fatalf("full intel must disclose %q:\n%s", want, full)
		}
	}
}

func TestEndlessShowsNonPrimaryRouteLines(t *testing.T) {
	m := testModel(t)
	m.state.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageWon,
		Victory: model.DoctrineConsumer, Endless: true, Cycle: 12,
	}
	card := renderCampaignStatusCard(m)
	// Non-primary doctrines get optional route lines.
	if !strings.Contains(card, "可選 "+doctrineLabel(model.DoctrineEnterprise)) {
		t.Fatalf("endless missing enterprise optional route:\n%s", card)
	}
	if !strings.Contains(card, "可選 "+doctrineLabel(model.DoctrineDeveloper)) {
		t.Fatalf("endless missing developer optional route:\n%s", card)
	}
	// Primary route must not be duplicated as a "可選" line.
	if strings.Contains(card, "可選 "+doctrineLabel(model.DoctrineConsumer)) {
		t.Fatalf("endless must not list primary as optional:\n%s", card)
	}
	// Primary still shown as 主要戰略 once.
	if !strings.Contains(card, "主要戰略") || !strings.Contains(card, doctrineLabel(model.DoctrineConsumer)) {
		t.Fatalf("primary doctrine should still appear as main route:\n%s", card)
	}
}

func TestBoardReportShowsNewestFourOnly(t *testing.T) {
	m := testModel(t)
	// Five distinct entries; oldest (first) must be dropped.
	m.state.Campaign.Reports = []model.BoardReport{{
		Cycle: 7,
		Entries: []model.CampaignReportEntry{
			{Kind: model.ReportDoctrineChosen, SubjectID: "consumer"}, // oldest — absent
			{Kind: model.ReportStageAdvanced, SubjectID: "expand"},    // kept
			{Kind: model.ReportRivalAction, SubjectID: "OpenAI", DetailID: "openai-flagship"},
			{Kind: model.ReportShowdown, SubjectID: "OpenAI"},
			{Kind: model.ReportVictory, SubjectID: "consumer"}, // newest
		},
	}}
	card := renderBoardReportCard(m)
	if strings.Contains(card, reportKindLabel(model.ReportDoctrineChosen)) {
		t.Fatalf("oldest entry must be absent:\n%s", card)
	}
	for _, want := range []string{
		reportKindLabel(model.ReportStageAdvanced),
		reportKindLabel(model.ReportRivalAction),
		reportKindLabel(model.ReportShowdown),
		reportKindLabel(model.ReportVictory),
	} {
		if !strings.Contains(card, want) {
			t.Fatalf("newest-four missing %q:\n%s", want, card)
		}
	}
}
