package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestComputePageShowsPools(t *testing.T) {
	m := testModel(t)
	m.page = PageCompute
	v := renderCompute(m)
	for _, w := range []string{"訓練", "推理", "機房", "晶片"} {
		if !strings.Contains(v, w) {
			t.Errorf("compute page missing %q:\n%s", w, v)
		}
	}
}

func TestComputeRentKeys(t *testing.T) {
	m := testModel(t)
	m.page = PageCompute
	before := m.state.Compute.TrainingCapacity
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if nm.(Model).state.Compute.TrainingCapacity != before+1 {
		t.Fatalf("r should add training capacity")
	}
}

func TestComputeKeysInertOffPage(t *testing.T) {
	m := testModel(t)
	m.page = PageOverview // r must NOT rent capacity off the compute page
	before := m.state.Compute.TrainingCapacity
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if nm.(Model).state.Compute.TrainingCapacity != before {
		t.Fatalf("r should be inert off the compute page")
	}
}
