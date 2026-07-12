package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"tokensmith/internal/model"
)

func TestWarRoomShowsCampaignCards(t *testing.T) {
	m := testModel(t)
	m.state.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand, Cycle: 4,
		Primary:  model.RivalRoadmap{Company: "OpenAI", ActionIndex: 0, CyclesUntilAction: 1},
		Wildcard: model.RivalRoadmap{Company: "DeepSeek", ActionIndex: 0, CyclesUntilAction: 2},
		Reports: []model.BoardReport{{Cycle: 4, Entries: []model.CampaignReportEntry{
			{Kind: model.ReportRivalAction, SubjectID: "OpenAI", DetailID: "openai-flagship"},
		}}},
	}
	v := renderWarRoom(m)
	for _, want := range []string{"主要戰略", "消費者霸主", "OpenAI", "下一步", "董事會報告", "產業動態"} {
		if !strings.Contains(v, want) {
			t.Fatalf("missing %q:\n%s", want, v)
		}
	}
}

func TestWarRoomPreCampaignGuidance(t *testing.T) {
	m := testModel(t)
	m.state.Campaign = model.CampaignState{}
	v := renderWarRoom(m)
	if !strings.Contains(v, "第一個模型上線後可選公司戰略") {
		t.Fatalf("pre-campaign guidance missing:\n%s", v)
	}
}

func TestWarRoomPendingEventHighlighted(t *testing.T) {
	m := pendingChipShortage(testModel(t)) // helper in dialog_event_test.go (same package)
	v := renderWarRoom(m)
	if !strings.Contains(v, "決策") {
		t.Fatalf("expected pending decision highlight:\n%s", v)
	}
}

func TestWarRoomEKeyOpensEventDialog(t *testing.T) {
	m := pendingChipShortage(testModel(t))
	m.page = PageWarRoom
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if nm.(Model).event == nil {
		t.Fatal("e on war room must open the event dialog")
	}
}

func TestWarRoomCKeyOpensDoctrineDialog(t *testing.T) {
	m := onlineCampaignModel(t)
	m.page = PageWarRoom
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if nm.(Model).doctrineDialog == nil {
		t.Fatal("c on war room with online model + no doctrine must open doctrine dialog")
	}
}

func TestWarRoomPKeyOpensVictoryDialog(t *testing.T) {
	m := testModel(t)
	m.page = PageWarRoom
	m.state.Campaign.Victory = model.DoctrineConsumer
	m.state.Campaign.Doctrine = model.DoctrineConsumer
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	if nm.(Model).campaignEnd == nil {
		t.Fatal("P on war room after victory must open campaign end dialog")
	}
}
