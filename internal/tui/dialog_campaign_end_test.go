package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"tokensmith/internal/model"
)

func TestCampaignEndDialogPAfterVictoryOffersContinueAndLegacy(t *testing.T) {
	m := onlineCampaignModel(t)
	m.state.Campaign = model.CampaignState{
		Doctrine:      model.DoctrineConsumer,
		Stage:         model.CampaignStageWon,
		Victory:       model.DoctrineConsumer,
		Secondary:     model.DoctrineDeveloper,
		SecondaryPerk: "developer-open",
	}
	m.state.UnlockedTech = []string{"algo-cap-1"}
	m.state.PeakValuation = 1e9

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	got := nm.(Model)
	if got.campaignEnd == nil {
		t.Fatal("P after victory must open campaign-end dialog")
	}
	if got.campaignEnd.mode != campaignEndVictory {
		t.Fatalf("mode=%v want victory", got.campaignEnd.mode)
	}
	// Must include only valid legacy kinds + continue.
	kinds := map[model.LegacyKind]bool{}
	for _, o := range got.campaignEnd.options {
		kinds[o.Kind] = true
		if o.Kind == model.LegacySecondary {
			if o.Doctrine != model.DoctrineDeveloper || o.PerkID != "developer-open" {
				t.Fatalf("secondary legacy payload=%+v", o)
			}
		}
		if o.Kind == model.LegacyNone {
			t.Fatal("must not offer empty legacy")
		}
	}
	if !kinds[model.LegacyIntel] {
		t.Fatal("must offer intel legacy")
	}
	if !kinds[model.LegacySecondary] {
		t.Fatal("must offer secondary legacy when secondary set")
	}
	if !kinds[model.LegacyTech] {
		t.Fatal("must offer tech legacy when unlocked tech present")
	}
	out := renderCampaignEndDialog(*got.campaignEnd, got)
	if !strings.Contains(out, "繼續") && !strings.Contains(out, "無盡") {
		t.Fatalf("victory dialog must offer continue:\n%s", out)
	}
}

func TestCampaignEndLegacyTechRequiresNestedEnter(t *testing.T) {
	m := onlineCampaignModel(t)
	m.state.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer,
		Victory:  model.DoctrineConsumer,
	}
	m.state.UnlockedTech = []string{"algo-cap-1", "biz-price-1"}
	d, ok := newCampaignEndDialog(m, campaignEndVictory)
	if !ok {
		t.Fatal("expected victory dialog")
	}
	// Find LegacyTech option.
	techIdx := -1
	for i, o := range d.options {
		if o.Kind == model.LegacyTech {
			techIdx = i
			break
		}
	}
	if techIdx < 0 {
		t.Fatal("expected LegacyTech option")
	}
	d.cursor = techIdx
	d, confirm, cancel := d.update(tea.KeyMsg{Type: tea.KeyEnter})
	if confirm || cancel || !d.choosingTech {
		t.Fatalf("first Enter on LegacyTech must enter nested tech list (confirm=%v cancel=%v choosing=%v)", confirm, cancel, d.choosingTech)
	}
	if len(d.techOptions) != 2 {
		t.Fatalf("techOptions=%v", d.techOptions)
	}
	// Esc returns to legacy choices without confirming.
	d, confirm, cancel = d.update(tea.KeyMsg{Type: tea.KeyEsc})
	if confirm || cancel || d.choosingTech {
		t.Fatalf("esc from tech list returns to legacy: confirm=%v cancel=%v choosing=%v", confirm, cancel, d.choosingTech)
	}
	// Re-enter tech and confirm second tech.
	d.cursor = techIdx
	d, _, _ = d.update(tea.KeyMsg{Type: tea.KeyEnter})
	d.techCursor = 1
	d, confirm, _ = d.update(tea.KeyMsg{Type: tea.KeyEnter})
	if !confirm {
		t.Fatal("second Enter must confirm")
	}
	cmd := d.command()
	cp, ok := cmd.(model.CampaignPrestige)
	if !ok {
		t.Fatalf("command type %T", cmd)
	}
	if cp.Legacy.Kind != model.LegacyTech || cp.Legacy.TechID != "biz-price-1" {
		t.Fatalf("legacy=%+v want tech biz-price-1", cp.Legacy)
	}
}

func TestCampaignEndContinueCommand(t *testing.T) {
	m := onlineCampaignModel(t)
	m.state.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer,
		Victory:  model.DoctrineConsumer,
	}
	d, ok := newCampaignEndDialog(m, campaignEndVictory)
	if !ok {
		t.Fatal("expected dialog")
	}
	// Continue is after all legacy options.
	d.cursor = len(d.options)
	d, confirm, _ := d.update(tea.KeyMsg{Type: tea.KeyEnter})
	if !confirm || !d.continueRun {
		t.Fatalf("continue confirm: confirm=%v continueRun=%v", confirm, d.continueRun)
	}
	if _, ok := d.command().(model.CampaignContinue); !ok {
		t.Fatalf("want CampaignContinue, got %T", d.command())
	}
}

func TestCampaignEndEOnlyAfterCycleOrDistress(t *testing.T) {
	m := onlineCampaignModel(t)
	m.state.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer,
		Cycle:    5,
	}
	// Not eligible.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("E")})
	got := nm.(Model)
	if got.campaignEnd != nil {
		t.Fatal("E must not open before cycle 18 or two distress cycles")
	}
	if got.campaignError != "第 18 週期後才能策略退出" {
		t.Fatalf("locked E error=%q", got.campaignError)
	}
	// Preserve invalid-command error across a later key that is not campaign selection.
	// Tick must also preserve it.
	errText := got.campaignError
	nm, _ = got.Update(tickMsg{})
	got = nm.(Model)
	if got.campaignError != errText {
		t.Fatalf("tick cleared campaignError: %q", got.campaignError)
	}

	// Eligible by cycle.
	m = onlineCampaignModel(t)
	m.state.Campaign = model.CampaignState{Doctrine: model.DoctrineConsumer, Cycle: 18}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("E")})
	got = nm.(Model)
	if got.campaignEnd == nil || got.campaignEnd.mode != campaignEndExit {
		t.Fatal("E at cycle 18 must open exit dialog")
	}

	// Eligible by distress.
	m = onlineCampaignModel(t)
	m.state.Campaign = model.CampaignState{Doctrine: model.DoctrineConsumer, Cycle: 3, FinancialDistressCycles: 2}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("E")})
	got = nm.(Model)
	if got.campaignEnd == nil || got.campaignEnd.mode != campaignEndExit {
		t.Fatal("E with two distress cycles must open exit dialog")
	}
}

func TestCampaignEndExitConfirmApplies(t *testing.T) {
	m := onlineCampaignModel(t)
	m.state.Campaign = model.CampaignState{Doctrine: model.DoctrineConsumer, Cycle: 20}
	m.state.PeakValuation = 1e8
	d, ok := newCampaignEndDialog(m, campaignEndExit)
	if !ok {
		t.Fatal("expected exit dialog")
	}
	m.campaignEnd = &d
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := nm.(Model)
	if got.campaignEnd != nil {
		t.Fatal("exit confirm must close dialog")
	}
	// Exit starts a fresh run (doctrine cleared).
	if got.state.Campaign.Doctrine != model.DoctrineNone {
		t.Fatalf("after exit doctrine=%q", got.state.Campaign.Doctrine)
	}
}

func TestCampaignEndPWithoutVictoryKeepsPrestigeReset(t *testing.T) {
	// Pre-campaign (no doctrine) + unlock valuation → legacy PrestigeReset banks patents and resets.
	m := onlineCampaignModel(t)
	m.page = PageOverview
	m.state.Campaign = model.CampaignState{} // DoctrineNone
	m.state.PeakValuation = 1e9
	m.state.Resources.Cash = 5e6
	m.state.Resources.RnD = 1e6
	m.state.Engineers = 5
	m.state.Prestige.Patents = 1
	beforePatents := m.state.Prestige.Patents
	wantPatents := beforePatents + 3 // patentsFor(1e9) = 3 with default balance

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	got := nm.(Model)
	if got.campaignEnd != nil {
		t.Fatal("pre-campaign P must not open campaign-end dialog")
	}
	if got.state.Prestige.Patents != wantPatents {
		t.Fatalf("PrestigeReset patents=%v want %v (banked)", got.state.Prestige.Patents, wantPatents)
	}
	if len(got.state.Models) != 0 || got.state.Engineers != 0 || got.state.PeakValuation != 0 {
		t.Fatalf("PrestigeReset must start a fresh run: models=%d eng=%d peak=%v",
			len(got.state.Models), got.state.Engineers, got.state.PeakValuation)
	}
	if got.state.Resources.Cash != got.cfg.StartingCash {
		t.Fatalf("cash after PrestigeReset=%v want StartingCash %v", got.state.Resources.Cash, got.cfg.StartingCash)
	}
}

func TestCampaignEndPActiveCampaignNoVictoryLeavesState(t *testing.T) {
	// Active campaign without victory: no campaign-end dialog; PrestigeReset is blocked
	// (Task 7 settlement only via CampaignPrestige/Exit) so state is unchanged.
	m := onlineCampaignModel(t)
	m.page = PageOverview
	m.state.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer,
		Stage:    model.CampaignStageExpand,
		Cycle:    4,
		Perks:    []string{"consumer-premium"},
	}
	m.state.PeakValuation = m.cfg.PrestigeUnlockValuation * 2
	m.state.Resources.Cash = 123456
	m.state.Prestige.Patents = 7
	before := m.state

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	got := nm.(Model)
	if got.campaignEnd != nil {
		t.Fatal("active not-won campaign must not open campaign-end dialog")
	}
	if got.state.Campaign.Doctrine != before.Campaign.Doctrine ||
		got.state.Campaign.Stage != before.Campaign.Stage ||
		got.state.Campaign.Cycle != before.Campaign.Cycle ||
		got.state.PeakValuation != before.PeakValuation ||
		got.state.Resources.Cash != before.Resources.Cash ||
		got.state.Prestige.Patents != before.Prestige.Patents ||
		len(got.state.Models) != len(before.Models) {
		t.Fatalf("P must not bypass Task 7 settlement; state changed:\nbefore campaign=%+v patents=%v cash=%v\nafter campaign=%+v patents=%v cash=%v",
			before.Campaign, before.Prestige.Patents, before.Resources.Cash,
			got.state.Campaign, got.state.Prestige.Patents, got.state.Resources.Cash)
	}
}

func TestCampaignDialogsRouteBeforeEvent(t *testing.T) {
	m := onlineCampaignModel(t)
	m = pendingChipShortage(m)
	// Open doctrine dialog first.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = nm.(Model)
	if m.doctrineDialog == nil {
		t.Fatal("expected doctrine dialog")
	}
	// Even with pending event, campaign dialog must keep priority (Esc closes doctrine, not event).
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := nm.(Model)
	if got.doctrineDialog != nil {
		t.Fatal("esc should close doctrine")
	}
	if got.event != nil {
		t.Fatal("esc on doctrine must not open event")
	}
	// Body priority: with both set, doctrine wins.
	d, _ := newDoctrineDialog(onlineCampaignModel(t), false)
	m.doctrineDialog = &d
	ev, _ := newEventDialog(m)
	m.event = &ev
	body := m.contentBody()
	if !strings.Contains(body, "戰略") && !strings.Contains(body, "流派") && !strings.Contains(body, "消費者") {
		// Primary dialog title/content should appear, not event name alone.
		if strings.Contains(body, "晶片") && !strings.Contains(body, "選擇") {
			t.Fatalf("campaign dialog must render before event dialog:\n%s", body)
		}
	}
}
