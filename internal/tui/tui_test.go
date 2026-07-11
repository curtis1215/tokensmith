package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"tokensmith/internal/balance"
	"tokensmith/internal/ingest"
	"tokensmith/internal/model"
	"tokensmith/internal/store"
)

func ingestEmptyPoller(t *testing.T) *ingest.Poller {
	return ingest.NewPoller(t.TempDir(), t.TempDir())
}

func TestUpdateTickAdvancesState(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "s.json"))
	m.poller = ingestEmptyPoller(t)
	before := m.state.GameTime
	nm, _ := m.Update(tickMsg(time.Unix(0, 0)))
	if nm.(Model).state.GameTime <= before {
		t.Fatalf("tick did not advance GameTime")
	}
}

func TestTrainKeyOpensDialogThenConfirms(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "s.json")) // seeded with enough R&D + training capacity
	m.poller = ingestEmptyPoller(t)
	// t opens the training modal; Enter confirms and starts training.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if nm.(Model).dialog == nil {
		t.Fatalf("train key did not open the dialog")
	}
	nm, _ = nm.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !nm.(Model).state.HasTraining {
		t.Fatalf("confirming the dialog did not start training")
	}
}

func TestRentKeysAddCapacity(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "s.json"))
	m.poller = ingestEmptyPoller(t)
	m.page = PageCompute // rent keys live on the compute page
	beforeT := m.state.Compute.RentedTraining[balance.EntryProcessID]
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if nm.(Model).state.Compute.RentedTraining[balance.EntryProcessID] != beforeT+1 {
		t.Fatalf("rent-training key did not add capacity")
	}
}

func TestViewNonEmpty(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "s.json"))
	if m.View() == "" {
		t.Fatalf("View is empty")
	}
}

func TestQuitKey(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "s.json"))
	m.poller = ingestEmptyPoller(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatalf("quit key should return a command")
	}
}

func TestNewHasPoller(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "s.json"))
	if m.poller == nil {
		t.Fatalf("New() should create an ingest poller")
	}
}

func TestTickPollsTokens(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "s.json"))
	m.poller = ingestEmptyPoller(t) // hermetic: empty log dirs
	before := m.state.GameTime
	nm, _ := m.Update(tickMsg(time.Unix(0, 0)))
	if nm.(Model).state.GameTime <= before {
		t.Fatalf("tick did not advance after polling")
	}
}

func TestNewLoadsSaveIfPresent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "save.json")
	var s model.GameState
	s.Resources.RnD = 999999
	if err := store.Save(path, s); err != nil {
		t.Fatal(err)
	}
	m := newAt(path) // test constructor with explicit save path
	if m.state.Resources.RnD != 999999 {
		t.Fatalf("New did not load save: RnD=%v", m.state.Resources.RnD)
	}
}

func TestNewAtPreservesCorruptSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "save.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newAt(path)
	// A fresh game is started (NewGame seeds competitors)...
	if len(m.state.Competitors) == 0 {
		t.Fatalf("corrupt save should start a fresh game, got no competitors")
	}
	// ...and the corrupt save is preserved instead of clobbered.
	if _, err := os.Stat(path + ".corrupt"); err != nil {
		t.Fatalf("corrupt save not preserved: %v", err)
	}
}

func TestQuitSavesState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "save.json")
	m := newAt(path)
	m.poller = ingestEmptyPoller(t) // hermetic
	m.state.Resources.Cash = 42
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("quit should return a command")
	}
	got, ok, _ := store.Load(path)
	if !ok || got.Resources.Cash != 42 {
		t.Fatalf("quit did not save: ok=%v cash=%v", ok, got.Resources.Cash)
	}
	_ = os.Remove(path)
}

func TestViewShellHasTabsAndFooterPattern(t *testing.T) {
	m := testModel(t)
	v := m.View()
	for _, want := range []string{"Tokensmith", "總覽", "模型", "Day"} {
		if !strings.Contains(v, want) {
			t.Fatalf("missing %q in view", want)
		}
	}
}

func TestModelResponsiveLayout(t *testing.T) {
	// 1. Narrow width (< 80) — resize so contentWidth/vp track terminal size
	mNarrow := testModel(t)
	mNarrow.resize(50, 40)
	viewNarrow := mNarrow.View()

	// 2. Wide width (>= 80)
	mWide := testModel(t)
	mWide.resize(120, 40)
	viewWide := mWide.View()

	// They should both contain essential elements
	if !strings.Contains(viewNarrow, "公司") || !strings.Contains(viewWide, "公司") {
		t.Fatalf("missing company card")
	}

	// Verify that viewNarrow is different from viewWide
	if viewNarrow == viewWide {
		t.Fatalf("narrow and wide views should not be identical")
	}
}

func TestChromeRowsPositiveAndContentHeight(t *testing.T) {
	m := testModel(t)
	ch := m.chromeRows()
	if ch < 5 {
		t.Fatalf("chromeRows=%d, want >= 5", ch)
	}
	m.resize(100, 40)
	if m.vp.Height < 3 {
		t.Fatalf("vp.Height=%d, want >= 3", m.vp.Height)
	}
	if m.height != 40 || m.width != 100 {
		t.Fatalf("size=%dx%d", m.width, m.height)
	}
}

func TestViewportHoldsTallContent(t *testing.T) {
	m := testModel(t)
	m.resize(80, 24)
	// Tall synthetic body via SetContent
	var lines []string
	for i := 0; i < 60; i++ {
		lines = append(lines, fmt.Sprintf("line-%02d", i))
	}
	m.vp.SetContent(strings.Join(lines, "\n"))
	if m.vp.TotalLineCount() < 60 {
		t.Fatalf("TotalLineCount=%d", m.vp.TotalLineCount())
	}
	m.vp.PageDown()
	if m.vp.YOffset <= 0 {
		t.Fatalf("after PageDown YOffset=%d, want > 0", m.vp.YOffset)
	}
	v := m.View()
	if !strings.Contains(v, "Tokensmith") {
		t.Fatalf("shell chrome missing after scroll:\n%s", v)
	}
	if !strings.Contains(v, "[Tab]切頁") {
		t.Fatalf("shell footer missing:\n%s", v)
	}
}

func TestRealSecCompressionMatchesTickRate(t *testing.T) {
	want := tickDT * float64(time.Second) / float64(tickInterval)
	if balance.RealSecCompression != want {
		t.Fatalf("balance.RealSecCompression = %v, want %v (tui tickDT/tickInterval changed without updating balance.RealSecCompression)", balance.RealSecCompression, want)
	}
}

func TestResourceBarSegments(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	bar := renderResourceBar(m)
	if !strings.Contains(bar, "│") {
		t.Fatalf("resource bar should use │ separators: %q", bar)
	}
	if !strings.Contains(bar, "💰") || !strings.Contains(bar, "📈") {
		t.Fatalf("segments missing: %q", bar)
	}
}

func TestTabBarMarksActive(t *testing.T) {
	got := renderTabBar(PageMarket)
	if !strings.Contains(got, "3 市場") {
		t.Fatalf("tab bar labels changed unexpectedly: %q", got)
	}
}

func TestResourceBarShowsCashArrowAndStreak(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	m.dispReady = true
	m.cashRate = 42
	m.streakDays = 5
	bar := renderResourceBar(m)
	if !strings.Contains(bar, "▲") {
		t.Fatalf("positive cashRate should show ▲: %q", bar)
	}
	if !strings.Contains(bar, "🔥5天") {
		t.Fatalf("streak should be persistent in bar: %q", bar)
	}
	m.cashRate = -42
	if bar = renderResourceBar(m); !strings.Contains(bar, "▼") {
		t.Fatalf("negative cashRate should show ▼: %q", bar)
	}
}
