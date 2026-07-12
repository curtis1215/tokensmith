package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func key(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func anyBoost(b [model.NumQualityDims]bool) bool {
	for _, v := range b {
		if v {
			return true
		}
	}
	return false
}

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

func TestTrainDialogToggleBoostAndCommand(t *testing.T) {
	m := testModel(t)
	m.state.Resources.RnD = 50_000
	m.state.Resources.Cash = 1e9
	d := newTrainDialog(m)
	// focus rows 0-3 = alloc; down into first boost (focus 4)
	for i := 0; i < model.NumQualityDims; i++ {
		d, _, _ = d.update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if d.focus != model.NumQualityDims {
		t.Fatalf("focus after 4 downs = %d, want %d", d.focus, model.NumQualityDims)
	}
	d, _, _ = d.update(key(" "))
	if !d.boosts[0] {
		t.Fatal("space on first boost row should toggle boosts[0]")
	}
	// + also toggles on boost row
	d, _, _ = d.update(key("+"))
	if d.boosts[0] {
		t.Fatal("+ on boost row should toggle off")
	}
	// number keys toggle boosts directly
	d, _, _ = d.update(key("2"))
	if !d.boosts[1] {
		t.Fatal("key 2 should toggle boosts[1]")
	}
	cmd := d.command(balance.Default())
	if !cmd.Boosts[1] && !anyBoost(cmd.Boosts) {
		t.Fatalf("command should include selected boosts: %+v", cmd.Boosts)
	}
	if cmd.Boosts != d.boosts {
		t.Fatalf("command.Boosts=%v want %v", cmd.Boosts, d.boosts)
	}
}

func TestTrainDialogRenderShowsChineseBoostNames(t *testing.T) {
	m := testModel(t)
	d := newTrainDialog(m)
	out := renderTrainDialog(d, m)
	for _, name := range []string{"優質語料", "省算力改造", "安全評測", "加速優化"} {
		if !strings.Contains(out, name) {
			t.Fatalf("missing %s in %q", name, out)
		}
	}
	if !strings.Contains(out, "參考月現金") {
		t.Fatal("missing anchor label")
	}
	if !strings.Contains(out, "訓練投資") {
		t.Fatal("missing boost section title")
	}
	if !strings.Contains(out, "預測吸引力") {
		t.Fatal("missing predicted appeal label")
	}
}

func TestTrainDialogInsufficientCashKeepsOpen(t *testing.T) {
	m := testModel(t)
	m.state.Resources.RnD = 50_000
	m.state.Resources.Cash = 0
	m.state.Compute.RentedTraining = map[string]int{"N7": 4}
	d := newTrainDialog(m)
	// toggle all boosts via 1-4 so cash cost > 0
	for _, k := range []string{"1", "2", "3", "4"} {
		d, _, _ = d.update(key(k))
	}
	if !anyBoost(d.boosts) {
		t.Fatal("precondition: boosts selected")
	}
	m.dialog = &d
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := nm.(Model)
	if got.dialog == nil {
		t.Fatal("insufficient cash must keep dialog open")
	}
	if got.state.HasTraining {
		t.Fatal("must not start training on insufficient cash")
	}
	if got.dialog.errMsg == "" {
		t.Fatal("expected errMsg on dialog")
	}
	if !strings.Contains(got.dialog.errMsg, "現金") {
		t.Fatalf("errMsg=%q want 現金", got.dialog.errMsg)
	}
	out := renderTrainDialog(*got.dialog, got)
	if !strings.Contains(out, got.dialog.errMsg) {
		t.Fatalf("render must show errMsg:\n%s", out)
	}
}
