package balance

import "testing"

func TestDefaultEventsCatalog(t *testing.T) {
	evs := DefaultEvents()
	if len(evs) != 10 {
		t.Fatalf("catalog size = %d, want 10", len(evs))
	}
	seen := map[string]bool{}
	for _, e := range evs {
		if seen[e.ID] {
			t.Fatalf("duplicate event ID %q", e.ID)
		}
		seen[e.ID] = true
		if e.Weight <= 0 {
			t.Fatalf("%s: weight must be positive", e.ID)
		}
		if e.NumChoices > 0 {
			if e.DefaultChoice != 1 {
				t.Fatalf("%s: default choice must be the free option (1), got %d", e.ID, e.DefaultChoice)
			}
			if e.DeadlineSec <= 0 {
				t.Fatalf("%s: choice events need a deadline", e.ID)
			}
		}
	}
}

func TestEventByID(t *testing.T) {
	evs := DefaultEvents()
	if _, ok := EventByID(evs, EvChipShortage); !ok {
		t.Fatal("chip-shortage should exist")
	}
	if _, ok := EventByID(evs, "nope"); ok {
		t.Fatal("unknown ID should return ok=false")
	}
}

func TestDefaultConfigWiresEvents(t *testing.T) {
	c := Default()
	if len(c.Events) != 10 || c.EventCheckSec <= 0 || c.EventHitChance <= 0 ||
		c.EventHitChance > 1 || c.EventCooldownSec <= 0 || c.EventLogCap <= 0 {
		t.Fatalf("event knobs not wired: %+v", struct {
			N                    int
			Check, Hit, Cooldown float64
			Cap                  int
		}{len(c.Events), c.EventCheckSec, c.EventHitChance, c.EventCooldownSec, c.EventLogCap})
	}
}
