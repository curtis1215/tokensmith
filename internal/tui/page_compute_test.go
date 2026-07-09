package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"tokensmith/internal/balance"
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
