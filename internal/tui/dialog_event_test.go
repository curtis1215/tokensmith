package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// pendingChipShortage arms one pending chip-shortage event on m.
func pendingChipShortage(m Model) Model {
	spec, _ := balance.EventByID(m.cfg.Events, balance.EvChipShortage)
	m.state.Events.Pending = []model.PendingEvent{{
		EventID: spec.ID, Target: -1,
		FiredAt:  m.state.GameTime,
		Deadline: m.state.GameTime + spec.DeadlineSec,
	}}
	return m
}

func TestEventDialogNeedsPending(t *testing.T) {
	m := testModel(t)
	if _, ok := newEventDialog(m); ok {
		t.Fatal("no pending events → no dialog")
	}
	m = pendingChipShortage(m)
	d, ok := newEventDialog(m)
	if !ok {
		t.Fatal("pending event should open the dialog")
	}
	if d.cursor != 1 {
		t.Fatalf("cursor starts on the free default, got %d", d.cursor)
	}
}

func TestEKeyOpensDialogOnOverview(t *testing.T) {
	m := pendingChipShortage(testModel(t))
	m.page = PageOverview
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if nm.(Model).event == nil {
		t.Fatal("e on overview must open the event dialog")
	}
}

func TestEKeyWithoutPendingShowsNotice(t *testing.T) {
	m := testModel(t)
	m.page = PageOverview
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	got := nm.(Model)
	if got.event != nil {
		t.Fatal("no pending → no dialog")
	}
	if got.notice == "" {
		t.Fatal("expected a notice explaining there is nothing to decide")
	}
}

func TestEKeyStillExpandsDatacenterOnComputePage(t *testing.T) {
	m := pendingChipShortage(testModel(t))
	m.page = PageCompute
	m.state.Resources.Cash = 1e9
	before := m.state.Datacenter.PowerCapacity
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	got := nm.(Model)
	if got.event != nil {
		t.Fatal("e on compute page must not open the event dialog")
	}
	if got.state.Datacenter.PowerCapacity <= before {
		t.Fatal("e on compute page must still expand the datacenter")
	}
}

func TestEventDialogConfirmResolves(t *testing.T) {
	m := pendingChipShortage(testModel(t))
	m.page = PageOverview
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m = nm.(Model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // confirm default (free)
	got := nm.(Model)
	if got.event != nil {
		t.Fatal("dialog must close on confirm")
	}
	if len(got.state.Events.Pending) != 0 || len(got.state.Events.Log) != 1 {
		t.Fatalf("resolution not applied: %+v", got.state.Events)
	}
}

func TestEventDialogStalePendingShowsNotice(t *testing.T) {
	m := pendingChipShortage(testModel(t))
	m.page = PageOverview
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m = nm.(Model)
	// Simulate auto-resolve while the dialog was open.
	m.state.Events.Pending = nil
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := nm.(Model)
	if got.event != nil {
		t.Fatal("dialog must close when the pending event is gone")
	}
	if got.notice == "" {
		t.Fatal("expected notice for stale/auto-resolved event")
	}
}

func TestEventDialogEscLeavesPending(t *testing.T) {
	m := pendingChipShortage(testModel(t))
	m.page = PageOverview
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m = nm.(Model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := nm.(Model)
	if got.event != nil {
		t.Fatal("esc must close the dialog")
	}
	if len(got.state.Events.Pending) != 1 {
		t.Fatal("esc must leave the event pending")
	}
}

func TestEventDialogRenderShowsChoicesAndCost(t *testing.T) {
	m := pendingChipShortage(testModel(t))
	d, _ := newEventDialog(m)
	out := renderEventDialog(d, m)
	meta := eventLabel(balance.EvChipShortage)
	if !strings.Contains(out, meta.Name) || !strings.Contains(out, meta.Choices[0]) ||
		!strings.Contains(out, meta.Choices[1]) {
		t.Fatalf("dialog missing copy:\n%s", out)
	}
	if !strings.Contains(out, "$") {
		t.Fatal("paid option must show its cash cost")
	}
}
