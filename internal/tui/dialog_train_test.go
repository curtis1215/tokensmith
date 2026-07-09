package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func key(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestDialogAdjustAndConfirm(t *testing.T) {
	m := testModel(t)
	m.state.Resources.RnD = 50000
	m.state.Compute.RentedTraining = map[string]int{"N7": 4}
	d := newTrainDialog(m)
	// move to 'safety' dim (index 2) and bump it
	d, _, _ = d.update(tea.KeyMsg{Type: tea.KeyDown})
	d, _, _ = d.update(tea.KeyMsg{Type: tea.KeyDown})
	before := d.alloc[2]
	d, _, _ = d.update(key("+"))
	if d.alloc[2] <= before {
		t.Fatalf("+ did not raise alloc: %v→%v", before, d.alloc[2])
	}
	// segment cycle
	d, _, _ = d.update(tea.KeyMsg{Type: tea.KeyTab})
	if d.segment == model.SegConsumer {
		t.Fatalf("Tab should cycle segment")
	}
	// Enter confirms
	_, confirm, _ := d.update(tea.KeyMsg{Type: tea.KeyEnter})
	if !confirm {
		t.Fatalf("Enter should confirm")
	}
	cmd := d.command(balance.Default())
	if cmd.Gen != d.gen || cmd.Segment != d.segment {
		t.Fatalf("command mismatch: %+v", cmd)
	}
}

func TestDialogAllocStaysNormalized(t *testing.T) {
	m := testModel(t)
	d := newTrainDialog(m)
	for i := 0; i < 5; i++ {
		d, _, _ = d.update(key("+"))
	}
	var sum float64
	for _, a := range d.alloc {
		sum += a
	}
	if sum < 0.999 || sum > 1.001 {
		t.Fatalf("alloc not normalized after edits: sum=%v", sum)
	}
}

func TestDialogGenClampedToUnlocked(t *testing.T) {
	m := testModel(t) // fresh game → only Gen1 unlocked
	d := newTrainDialog(m)
	if d.maxGen != 1 {
		t.Fatalf("fresh dialog maxGen = %d, want 1", d.maxGen)
	}
	d, _, _ = d.update(tea.KeyMsg{Type: tea.KeyRight}) // attempt to raise gen
	if d.gen != 1 {
		t.Fatalf("gen should clamp to maxGen 1, got %d", d.gen)
	}
}

func TestDialogConfirmStartsTraining(t *testing.T) {
	m := testModel(t)
	m.state.Resources.RnD = 50000
	m.state.Compute.RentedTraining = map[string]int{"N7": 4}
	d := newTrainDialog(m)
	m.dialog = &d
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := nm.(Model)
	if got.dialog != nil {
		t.Fatalf("dialog should close after confirm")
	}
	if !got.state.HasTraining {
		t.Fatalf("confirm should start training")
	}
}

func TestDialogEscCancels(t *testing.T) {
	m := testModel(t)
	d := newTrainDialog(m)
	m.dialog = &d
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := nm.(Model)
	if got.dialog != nil {
		t.Fatalf("Esc should close the dialog")
	}
	if got.state.HasTraining {
		t.Fatalf("Esc should not start training")
	}
}
