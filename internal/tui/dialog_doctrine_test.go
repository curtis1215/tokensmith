package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
	"tokensmith/internal/store"
)

func onlineCampaignModel(t *testing.T) Model {
	t.Helper()
	m := testModel(t)
	m.page = PageOverview
	m.state.Models = []model.Model{{Online: true, Users: 1000, Price: 12, Segment: model.SegConsumer}}
	m.state.Resources.Cash = 1e6
	m.state.Resources.RnD = 1e6
	return m
}

func TestDoctrineDialogPrimaryAfterOnlineModel(t *testing.T) {
	m := onlineCampaignModel(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	got := nm.(Model)
	if got.doctrineDialog == nil {
		t.Fatal("c after first online model must open doctrine dialog")
	}
	if got.doctrineDialog.mode != doctrineChoosePrimary {
		t.Fatalf("mode=%v want primary", got.doctrineDialog.mode)
	}
	if len(got.doctrineDialog.options) != 3 {
		t.Fatalf("primary options=%v want 3 doctrines", got.doctrineDialog.options)
	}
	// Confirm first doctrine (consumer).
	nm, _ = got.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got = nm.(Model)
	if got.doctrineDialog != nil {
		t.Fatal("dialog must close on successful primary choice")
	}
	if got.state.Campaign.Doctrine != model.DoctrineConsumer {
		t.Fatalf("doctrine=%q want consumer", got.state.Campaign.Doctrine)
	}
}

func TestDoctrineDialogPerkTierShowTwoMatchingPerks(t *testing.T) {
	for _, tier := range []int{1, 2} {
		m := onlineCampaignModel(t)
		m.state.Campaign = model.CampaignState{
			Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand, PerkTierPending: tier,
		}
		d, ok := newDoctrineDialog(m, false)
		if !ok {
			t.Fatalf("tier %d: dialog should open", tier)
		}
		if d.mode != doctrineChoosePerk {
			t.Fatalf("tier %d: mode=%v want perk", tier, d.mode)
		}
		if len(d.options) != 2 {
			t.Fatalf("tier %d: options=%v want exactly 2 perks", tier, d.options)
		}
		for _, id := range d.options {
			p, ok := balance.CampaignPerkByID(m.cfg.Campaign, id)
			if !ok || p.Doctrine != model.DoctrineConsumer || p.Tier != tier {
				t.Fatalf("tier %d: bad perk option %q (%+v)", tier, id, p)
			}
		}
		out := renderDoctrineDialog(d, m)
		if !strings.Contains(out, perkLabel(d.options[0])) {
			t.Fatalf("render missing perk label:\n%s", out)
		}
	}
}

func TestDoctrineDialogSecondaryOffersTwoNonPrimary(t *testing.T) {
	m := onlineCampaignModel(t)
	m.state.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageShowdown,
	}
	d, ok := newDoctrineDialog(m, false)
	if !ok {
		t.Fatal("showdown without secondary should open dialog")
	}
	if d.mode != doctrineChooseSecondary {
		t.Fatalf("mode=%v want secondary", d.mode)
	}
	if len(d.options) != 2 {
		t.Fatalf("secondary options=%v want exactly 2", d.options)
	}
	seen := map[model.Doctrine]bool{}
	for _, id := range d.options {
		p, ok := balance.CampaignPerkByID(m.cfg.Campaign, id)
		if !ok || p.Tier != 1 {
			t.Fatalf("secondary option must be tier-1 perk, got %q ok=%v", id, ok)
		}
		if p.Doctrine == model.DoctrineConsumer {
			t.Fatalf("must not offer primary doctrine perk %q", id)
		}
		seen[p.Doctrine] = true
	}
	if len(seen) != 2 {
		t.Fatalf("want two non-primary doctrines, got %v", seen)
	}
}

func TestDoctrineDialogUppercaseCRequiresPivotConfirm(t *testing.T) {
	m := onlineCampaignModel(t)
	m.state.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand,
	}
	before := m.state.Campaign.Doctrine
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("C")})
	got := nm.(Model)
	if got.doctrineDialog == nil {
		t.Fatal("C must open pivot confirmation dialog")
	}
	if got.doctrineDialog.mode != doctrineConfirmPivot {
		t.Fatalf("mode=%v want pivot confirm", got.doctrineDialog.mode)
	}
	if got.state.Campaign.Doctrine != before {
		t.Fatal("uppercase C must never apply pivot on first key")
	}
	// Esc cancels without applying.
	nm, _ = got.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got = nm.(Model)
	if got.doctrineDialog != nil {
		t.Fatal("esc must close pivot dialog")
	}
	if got.state.Campaign.Doctrine != before || got.state.Campaign.PivotUsed {
		t.Fatalf("esc must not pivot: campaign=%+v", got.state.Campaign)
	}
}

func TestDoctrineDialogRejectedKeepsOpenAndShowsError(t *testing.T) {
	m := onlineCampaignModel(t)
	// Insufficient cash for pivot on expand stage — reject must keep dialog open.
	m.state.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand,
	}
	m.state.Resources.Cash = 0
	m.state.Resources.RnD = 0
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("C")})
	got := nm.(Model)
	if got.doctrineDialog == nil {
		t.Fatal("expected pivot dialog")
	}
	nm, _ = got.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got = nm.(Model)
	if got.doctrineDialog == nil {
		t.Fatal("rejected pivot must keep dialog open")
	}
	if got.campaignError == "" {
		t.Fatal("rejected command must set campaignError")
	}
	if !strings.Contains(got.campaignError, "現金") && !strings.Contains(got.campaignError, "R&D") &&
		!strings.Contains(got.campaignError, "無法") {
		t.Fatalf("unexpected error text %q", got.campaignError)
	}
	// Error visible in render.
	out := renderDoctrineDialog(*got.doctrineDialog, got)
	if !strings.Contains(out, got.campaignError) {
		t.Fatalf("dialog render must show campaignError:\n%s", out)
	}
}

func TestChooseDoctrineOverwritesLastCampaignUnix(t *testing.T) {
	m := onlineCampaignModel(t)
	const oldUnix int64 = 1_000_000
	m.lastCampaignUnix = oldUnix
	// Persist the stale arm so we can prove overwrite on disk too.
	m.saveMeta()

	before := time.Now().Unix()
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = nm.(Model)
	if m.doctrineDialog == nil || m.doctrineDialog.mode != doctrineChoosePrimary {
		t.Fatal("expected primary doctrine dialog")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if m.state.Campaign.Doctrine == model.DoctrineNone {
		t.Fatal("doctrine not applied")
	}
	if m.lastCampaignUnix == oldUnix {
		t.Fatal("lastCampaignUnix must overwrite pre-armed nonzero value")
	}
	if m.lastCampaignUnix < before {
		t.Fatalf("lastCampaignUnix=%d want >= selection time %d", m.lastCampaignUnix, before)
	}
	meta, ok, err := store.LoadMeta(m.metaPath)
	if err != nil || !ok {
		t.Fatalf("load meta: ok=%v err=%v", ok, err)
	}
	if meta.LastCampaignUnix != m.lastCampaignUnix {
		t.Fatalf("persisted LastCampaignUnix=%d want %d", meta.LastCampaignUnix, m.lastCampaignUnix)
	}
	if meta.LastCampaignUnix == oldUnix {
		t.Fatal("disk meta must not keep pre-armed lastCampaignUnix")
	}
}

func TestDoctrineDialogEscClearsCampaignError(t *testing.T) {
	m := onlineCampaignModel(t)
	d, ok := newDoctrineDialog(m, false)
	if !ok {
		t.Fatal("expected dialog")
	}
	m.doctrineDialog = &d
	m.campaignError = "現金不足"
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := nm.(Model)
	if got.doctrineDialog != nil {
		t.Fatal("esc closes dialog")
	}
	if got.campaignError != "" {
		t.Fatalf("esc must clear campaignError, got %q", got.campaignError)
	}
}

func TestCampaignErrorSurvivesTick(t *testing.T) {
	m := onlineCampaignModel(t)
	m.campaignError = "本週期已使用高層指令"
	nm, _ := m.Update(tickMsg(time.Now()))
	got := nm.(Model)
	if got.campaignError != "本週期已使用高層指令" {
		t.Fatalf("tick must not clear campaignError, got %q", got.campaignError)
	}
}

func TestCampaignKeysLockShellNavigation(t *testing.T) {
	m := onlineCampaignModel(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = nm.(Model)
	if m.doctrineDialog == nil {
		t.Fatal("precondition: dialog open")
	}
	before := m.page
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	got := nm.(Model)
	if got.page != before {
		t.Fatalf("open campaign dialog must lock tab navigation: %v → %v", before, got.page)
	}
	if got.doctrineDialog == nil {
		t.Fatal("dialog must stay open")
	}
}

func TestDialogFooterHidesShellForCampaignDialogs(t *testing.T) {
	m := onlineCampaignModel(t)
	d, _ := newDoctrineDialog(m, false)
	m.doctrineDialog = &d
	if keys := pageKeys(m); keys != "" {
		t.Fatalf("campaign dialog open: pageKeys must be empty for fixed footer, got %q", keys)
	}
}
