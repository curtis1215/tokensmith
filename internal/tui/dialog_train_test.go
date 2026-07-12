package tui

import (
	"math"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
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

func TestTrainDialogMarginalShowsSlotSurcharge(t *testing.T) {
	m := testModel(t)
	d := newTrainDialog(m)
	// Select all four so safety is rank 2 (×1.8) and speed rank 3 (×2.5).
	for _, k := range []string{"1", "2", "3", "4"} {
		d, _, _ = d.update(key(k))
	}
	out := renderTrainDialog(d, m)
	if !strings.Contains(out, "第3件") || !strings.Contains(out, "×1.8") {
		t.Fatalf("expected 3rd-item surcharge label in:\n%s", out)
	}
	if !strings.Contains(out, "第4件") || !strings.Contains(out, "×2.5") {
		t.Fatalf("expected 4th-item surcharge label in:\n%s", out)
	}
	if !strings.Contains(out, "全滿投資") {
		t.Fatalf("missing full-pack line in:\n%s", out)
	}
	// Marginal for safety under full select = quote(all) - quote(all without safety)
	var all, noSafety [model.NumQualityDims]bool
	for i := range all {
		all[i] = true
		noSafety[i] = true
	}
	noSafety[model.DimSafety] = false
	cAll, err := sim.QuoteTrainBoostCost(m.state, 1, all, m.cfg)
	if err != nil {
		t.Fatal(err)
	}
	cNo, err := sim.QuoteTrainBoostCost(m.state, 1, noSafety, m.cfg)
	if err != nil {
		t.Fatal(err)
	}
	wantMarginal := cAll - cNo
	got := trainBoostMarginalCost(m, 1, all, model.DimSafety)
	if math.Abs(got-wantMarginal) > 1e-6 {
		t.Fatalf("marginal safety = %v, want %v", got, wantMarginal)
	}
	// Row must not show pure base (lower than slot-aware marginal for rank 2+).
	base, err := balance.TrainBoostBasePrice(1, sim.TrainBoostRefMonthly(m.state, m.cfg), model.DimSafety, m.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if wantMarginal <= base+1e-6 {
		t.Fatalf("precondition failed: slot marginal %v should exceed base %v", wantMarginal, base)
	}
	if strings.Contains(out, human(base)) && !strings.Contains(out, human(wantMarginal)) {
		// human() formatting may round; ensure marginal appears as the +price line content
		t.Logf("base=%v marginal=%v out=\n%s", base, wantMarginal, out)
	}
	if !strings.Contains(out, human(wantMarginal)) && !strings.Contains(out, "+"+human(wantMarginal)) {
		// human may format with commas; check +prefix via marginal helper display path
		if !strings.Contains(out, human(got)) {
			t.Fatalf("render should show slot-aware marginal %s:\n%s", human(got), out)
		}
	}
}

func TestTrainBoostNameZHUsesDimNotSliceOrder(t *testing.T) {
	cfg := balance.Default()
	// Reverse catalog order; names must still map by Dim.
	cfg.TrainBoosts = []balance.TrainBoost{
		cfg.TrainBoosts[3], cfg.TrainBoosts[2], cfg.TrainBoosts[1], cfg.TrainBoosts[0],
	}
	if got := trainBoostNameZH(cfg, model.DimSafety); got != "安全評測" {
		t.Fatalf("name = %q, want 安全評測", got)
	}
	if got := trainBoostNameZH(cfg, model.DimCapability); got != "優質語料" {
		t.Fatalf("name = %q, want 優質語料", got)
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
