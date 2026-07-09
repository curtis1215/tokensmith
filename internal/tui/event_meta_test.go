package tui

import (
	"strings"
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestEventLabelKnownAndFallback(t *testing.T) {
	if eventLabel(balance.EvChipShortage).Name == balance.EvChipShortage {
		t.Fatal("chip-shortage should have a Chinese name")
	}
	if eventLabel("mystery").Name != "mystery" {
		t.Fatal("unknown ID must fall back to the raw ID")
	}
	for _, spec := range balance.DefaultEvents() {
		meta := eventLabel(spec.ID)
		if meta.Name == spec.ID {
			t.Fatalf("%s: missing Chinese name", spec.ID)
		}
		if spec.NumChoices > 0 && (meta.Choices[0] == "" || meta.Choices[1] == "") {
			t.Fatalf("%s: choice events need both choice labels", spec.ID)
		}
	}
}

func TestEventsCardEmptyState(t *testing.T) {
	m := testModel(t)
	out := renderEventsCard(m)
	if !strings.Contains(out, "產業動態") || !strings.Contains(out, "風平浪靜") {
		t.Fatalf("empty card wrong:\n%s", out)
	}
}

func TestEventsCardShowsPendingAndLog(t *testing.T) {
	m := testModel(t)
	m.state.GameTime = 100000
	m.state.Events.Pending = []model.PendingEvent{
		{EventID: balance.EvChipShortage, Target: -1, FiredAt: 90000, Deadline: 100000 + 10*86400},
	}
	m.state.Events.Log = []model.EventRecord{
		{EventID: balance.EvMarketCycle, At: 50000, Choice: 0, Auto: false},
	}
	out := renderEventsCard(m)
	if !strings.Contains(out, eventLabel(balance.EvChipShortage).Name) {
		t.Fatal("pending event name missing")
	}
	if !strings.Contains(out, "[e]") {
		t.Fatal("pending line must point at the e key")
	}
	if !strings.Contains(out, eventLabel(balance.EvMarketCycle).Name) {
		t.Fatal("log entry name missing")
	}
}

func TestOverviewIncludesEventsCard(t *testing.T) {
	m := testModel(t)
	if !strings.Contains(renderOverview(m), "產業動態") {
		t.Fatal("overview must include the events card")
	}
}
