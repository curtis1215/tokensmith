package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"tokensmith/internal/model"
)

func TestDirectiveDialogOffersRouteCounterIntel(t *testing.T) {
	m := onlineCampaignModel(t)
	m.state.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer,
		Stage:    model.CampaignStageExpand,
		Primary:  model.RivalRoadmap{Company: "OpenAI", ActionIndex: 0, CyclesUntilAction: 1},
		Wildcard: model.RivalRoadmap{Company: "DeepSeek", ActionIndex: 0, CyclesUntilAction: 2},
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	got := nm.(Model)
	if got.directiveDialog == nil {
		t.Fatal("d must open directive dialog when campaign active")
	}
	d := *got.directiveDialog
	if len(d.options) != 3 {
		t.Fatalf("options=%v want route/counter/intel", d.options)
	}
	want := []model.DirectiveKind{model.DirectiveRoutePush, model.DirectiveCounter, model.DirectiveIntel}
	for i, k := range want {
		if d.options[i] != k {
			t.Fatalf("options[%d]=%q want %q", i, d.options[i], k)
		}
	}
	out := renderDirectiveDialog(d, got)
	for _, label := range []string{"市場衝刺", "反制宿敵", "深度情報"} {
		if !strings.Contains(out, label) {
			t.Fatalf("render missing %q:\n%s", label, out)
		}
	}
}

func TestDirectiveDialogCounterAsksForRivalTarget(t *testing.T) {
	m := onlineCampaignModel(t)
	m.state.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer,
		Primary:  model.RivalRoadmap{Company: "OpenAI", ActionIndex: 0},
		Wildcard: model.RivalRoadmap{Company: "DeepSeek", ActionIndex: 0},
	}
	d, ok := newDirectiveDialog(m)
	if !ok {
		t.Fatal("expected directive dialog")
	}
	// Move to counter (index 1) and confirm → target phase.
	d.cursor = 1
	d, confirm, cancel := d.update(tea.KeyMsg{Type: tea.KeyEnter})
	if confirm || cancel || !d.choosingTarget {
		t.Fatalf("counter enter should enter target phase: confirm=%v cancel=%v choosing=%v", confirm, cancel, d.choosingTarget)
	}
	out := renderDirectiveDialog(d, m)
	if !strings.Contains(out, "OpenAI") || !strings.Contains(out, "DeepSeek") {
		t.Fatalf("target list missing rivals:\n%s", out)
	}
	// Confirm target.
	d, confirm, cancel = d.update(tea.KeyMsg{Type: tea.KeyEnter})
	if !confirm || cancel {
		t.Fatalf("target enter should confirm: confirm=%v cancel=%v", confirm, cancel)
	}
	cmd := d.command(m)
	if cmd.Kind != model.DirectiveCounter || cmd.Target != "OpenAI" {
		t.Fatalf("command=%+v", cmd)
	}
}

func TestDirectiveDialogIntelAsksForRivalTarget(t *testing.T) {
	m := onlineCampaignModel(t)
	m.state.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer,
		Primary:  model.RivalRoadmap{Company: "OpenAI", ActionIndex: 0},
		Wildcard: model.RivalRoadmap{Company: "DeepSeek", ActionIndex: 0},
	}
	d, ok := newDirectiveDialog(m)
	if !ok {
		t.Fatal("expected dialog")
	}
	d.cursor = 2 // intel
	d, confirm, _ := d.update(tea.KeyMsg{Type: tea.KeyEnter})
	if confirm || !d.choosingTarget {
		t.Fatal("intel must ask for target")
	}
	d.targetCursor = 1
	d, confirm, _ = d.update(tea.KeyMsg{Type: tea.KeyEnter})
	if !confirm {
		t.Fatal("expected confirm after target")
	}
	cmd := d.command(m)
	if cmd.Kind != model.DirectiveIntel || cmd.Target != "DeepSeek" {
		t.Fatalf("command=%+v", cmd)
	}
}

func TestDirectiveDialogRejectedKeepsOpen(t *testing.T) {
	m := onlineCampaignModel(t)
	m.state.Campaign = model.CampaignState{
		Doctrine:      model.DoctrineConsumer,
		DirectiveUsed: true,
		Primary:       model.RivalRoadmap{Company: "OpenAI", ActionIndex: 0},
		Wildcard:      model.RivalRoadmap{Company: "DeepSeek", ActionIndex: 0},
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	got := nm.(Model)
	if got.directiveDialog == nil {
		t.Fatal("expected dialog")
	}
	// Route push (cursor 0) — should reject as already used.
	nm, _ = got.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got = nm.(Model)
	if got.directiveDialog == nil {
		t.Fatal("rejected directive must keep dialog open")
	}
	if got.campaignError != "本週期已使用高層指令" {
		t.Fatalf("campaignError=%q", got.campaignError)
	}
	out := renderDirectiveDialog(*got.directiveDialog, got)
	if !strings.Contains(out, got.campaignError) {
		t.Fatalf("render must show error:\n%s", out)
	}
}

func TestDirectiveDialogRoutePushApplies(t *testing.T) {
	m := onlineCampaignModel(t)
	m.state.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer,
		Stage:    model.CampaignStageExpand,
	}
	m.state.Resources.Cash = 50000
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	got := nm.(Model)
	nm, _ = got.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got = nm.(Model)
	if got.directiveDialog != nil {
		t.Fatal("success must close dialog")
	}
	if !got.state.Campaign.DirectiveUsed {
		t.Fatal("directive not applied")
	}
	if got.campaignError != "" {
		t.Fatalf("success must clear campaignError, got %q", got.campaignError)
	}
}
