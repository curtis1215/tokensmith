package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestComputePageShowsPools(t *testing.T) {
	m := testModel(t)
	m.page = PageCompute
	v := renderCompute(m)
	for _, w := range []string{"訓練", "推理", "機房", "製程"} {
		if !strings.Contains(v, w) {
			t.Errorf("compute page missing %q:\n%s", w, v)
		}
	}
}

func TestComputeRentKeys(t *testing.T) {
	m := testModel(t)
	m.page = PageCompute
	before := m.state.Compute.RentedTraining[balance.EntryProcessID]
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if nm.(Model).state.Compute.RentedTraining[balance.EntryProcessID] != before+1 {
		t.Fatalf("r should add training capacity")
	}
}

func TestComputeKeysInertOffPage(t *testing.T) {
	m := testModel(t)
	m.page = PageOverview // r must NOT rent capacity off the compute page
	before := m.state.Compute.RentedTraining[balance.EntryProcessID]
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if nm.(Model).state.Compute.RentedTraining[balance.EntryProcessID] != before {
		t.Fatalf("r should be inert off the compute page")
	}
}

func TestComputePageListsProcesses(t *testing.T) {
	m := testModel(t)
	m.page = PageCompute
	v := renderCompute(m)
	for _, w := range []string{"N7", "N5", "訓練池", "推理池", "解鎖"} {
		if !strings.Contains(v, w) {
			t.Errorf("compute page missing %q:\n%s", w, v)
		}
	}
}

func TestComputeRentSelectedProcess(t *testing.T) {
	m := testModel(t)
	m.page = PageCompute
	m.procCursor = 0 // N7 (entry, unlocked)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if nm.(Model).state.Compute.RentedInference["N7"] != 1 {
		t.Fatalf("i should rent 1 N7 into inference")
	}
}

func TestComputeCannotRentLocked(t *testing.T) {
	m := testModel(t)
	m.page = PageCompute
	m.procCursor = 1 // N5 (locked at start)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if len(nm.(Model).state.Compute.RentedTraining) != 0 {
		t.Fatalf("renting a locked process should be a no-op")
	}
}

func TestComputeCursorMovesWithinCatalog(t *testing.T) {
	m := testModel(t)
	m.page = PageCompute
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m2 := nm.(Model)
	if m2.procCursor != 1 {
		t.Fatalf("down should move procCursor to 1, got %d", m2.procCursor)
	}
	// Should not overshoot past the last process.
	for i := 0; i < len(m.cfg.Processes)+5; i++ {
		nm, _ = m2.Update(tea.KeyMsg{Type: tea.KeyDown})
		m2 = nm.(Model)
	}
	if m2.procCursor != len(m.cfg.Processes)-1 {
		t.Fatalf("procCursor overshot catalog: %d", m2.procCursor)
	}
}

func TestComputeBuildSelectedIntoTraining(t *testing.T) {
	m := testModel(t)
	m.page = PageCompute
	m.procCursor = 0 // N7
	m.state.Datacenter = model.Datacenter{PowerCapacity: 100, SlotCapacity: 10}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	ns := nm.(Model).state
	if len(ns.Servers) != 1 || ns.Servers[0].Pool != model.PoolTraining {
		t.Fatalf("b should build 1 server into training pool, got %+v", ns.Servers)
	}
}

func TestComputeBuildSelectedIntoInference(t *testing.T) {
	m := testModel(t)
	m.page = PageCompute
	m.procCursor = 0 // N7
	m.state.Datacenter = model.Datacenter{PowerCapacity: 100, SlotCapacity: 10}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'B'}})
	ns := nm.(Model).state
	if len(ns.Servers) != 1 || ns.Servers[0].Pool != model.PoolInference {
		t.Fatalf("B should build 1 server into inference pool, got %+v", ns.Servers)
	}
}
