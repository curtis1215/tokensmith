package tui

import (
	"strings"
	"testing"

	"tokensmith/internal/model"
)

func TestCampaignCyclesDuePreservesCadence(t *testing.T) {
	due, next := campaignCyclesDue(100, 100+9*60*60, 8*60*60, 3)
	if due != 1 || next != 100+8*60*60 {
		t.Fatalf("due=%d next=%d", due, next)
	}
}

func TestCampaignCyclesDueCapsAndDropsOldBacklog(t *testing.T) {
	now := int64(100 + 7*24*60*60)
	due, next := campaignCyclesDue(100, now, 8*60*60, 3)
	if due != 3 || next != now {
		t.Fatalf("due=%d next=%d", due, next)
	}
}

func TestCampaignCyclesDueUninitialized(t *testing.T) {
	due, next := campaignCyclesDue(0, 500, 8*60*60, 3)
	if due != 0 || next != 500 {
		t.Fatalf("due=%d next=%d", due, next)
	}
}

func TestAdvanceCampaignToCapsAtThreeCycles(t *testing.T) {
	m := testModel(t)
	m.state.Campaign.Doctrine = model.DoctrineConsumer
	m.state.Campaign.Stage = model.CampaignStageExpand
	m.lastCampaignUnix = 100
	nm, advanced := m.advanceCampaignTo(100 + 7*24*60*60)
	if advanced != 3 || nm.state.Campaign.Cycle != 3 || nm.lastCampaignUnix != 100+7*24*60*60 {
		t.Fatalf("advanced=%d cycle=%d last=%d", advanced, nm.state.Campaign.Cycle, nm.lastCampaignUnix)
	}
}

func TestAdvanceCampaignToDoesNothingBeforeDoctrine(t *testing.T) {
	m := testModel(t)
	m.lastCampaignUnix = 100
	nm, advanced := m.advanceCampaignTo(100 + 24*60*60)
	if advanced != 0 || nm.state.Campaign.Cycle != 0 {
		t.Fatalf("advanced=%d campaign=%+v", advanced, nm.state.Campaign)
	}
}

func TestOfflineBannerCampaignCycles(t *testing.T) {
	out := offlineBanner(Summary{SecondsSettled: 3600, CampaignCycles: 2})
	if !strings.Contains(out, "董事會週期 2 次") {
		t.Fatalf("expected campaign cycle count in banner, got %q", out)
	}
}
