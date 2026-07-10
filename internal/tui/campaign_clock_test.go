package tui

import "testing"

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
