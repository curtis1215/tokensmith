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

func TestNewAtPreservesFailedSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "save.json")
	const garbage = "{not valid json"
	if err := os.WriteFile(path, []byte(garbage), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newAt(path)
	// Original path remains; no .corrupt rename, no silent fresh writable game.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("original save missing: %v", err)
	}
	if _, err := os.Stat(path + ".corrupt"); err == nil {
		t.Fatal("must not rename to .corrupt")
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != garbage {
		t.Fatalf("original bytes changed: %q err=%v", raw, err)
	}
	if m.startupErr == nil {
		t.Fatal("startupErr should be set")
	}
	if !m.saveDisabled {
		t.Fatal("saveDisabled should be true")
	}
	// Do not seed a playable NewGame over the failed load.
	if len(m.state.Competitors) != 0 {
		t.Fatalf("failed load must not seed NewGame competitors, got %d", len(m.state.Competitors))
	}
	// View is a blocking recovery screen with path/error (path may wrap).
	v := m.View()
	if !strings.Contains(v, "save.json") || !strings.Contains(v, "存檔載入失敗") {
		t.Fatalf("view should be blocking recovery with path: %q", v)
	}
	if !strings.Contains(v, "無法載入存檔") || !strings.Contains(v, "invalid character") {
		t.Fatalf("view should show error detail: %q", v)
	}
	// q exits without overwriting the source.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("q should quit")
	}
	raw2, _ := os.ReadFile(path)
	if string(raw2) != garbage {
		t.Fatalf("q overwrote failed save: %q", raw2)
	}
}

func TestLoadFailureDisablesAutosave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "save.json")
	const garbage = `{"schemaVersion":1,"state":`
	if err := os.WriteFile(path, []byte(garbage), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newAt(path)
	if !m.saveDisabled || m.startupErr == nil {
		t.Fatalf("want saveDisabled+startupErr, got disabled=%v err=%v", m.saveDisabled, m.startupErr)
	}
	// Tick must not write.
	m.ticksSinceSave = 40
	m2, _ := m.Update(tickMsg(time.Now()))
	mm := m2.(Model)
	raw, _ := os.ReadFile(path)
	if string(raw) != garbage {
		t.Fatalf("tick autosave rewrote failed save: %q", raw)
	}
	// Resize still allowed.
	mm2, _ := mm.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if mm2.(Model).width != 120 {
		t.Fatal("resize should still work")
	}
	// Non-quit keys do nothing harmful.
	mm3, cmd := mm2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if cmd != nil {
		t.Fatal("non-quit key should not schedule cmds in recovery mode")
	}
	_ = mm3
	raw2, _ := os.ReadFile(path)
	if string(raw2) != garbage {
		t.Fatalf("key handling rewrote save: %q", raw2)
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

func TestSaveMetaPersistsAchievements(t *testing.T) {
	dir := t.TempDir()
	m := newAt(filepath.Join(dir, "save.json"))
	m.achievements = map[string]int64{"streak-3": 42}
	m.saveMetaAt(100)
	meta, ok, _ := store.LoadMeta(filepath.Join(dir, "meta.json"))
	if !ok || meta.Achievements["streak-3"] != 42 {
		t.Fatalf("achievements not persisted: %+v", meta.Achievements)
	}
}

func TestHireShowsSuccessNotice(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	m.state.Resources.Cash = 1e9
	m.page = PageTeam
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = mm.(Model)
	if !strings.Contains(m.notice, "已雇用研究員") {
		t.Fatalf("hire should set success notice, got %q", m.notice)
	}
}

func TestTechUnlockShowsName(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	m.state.Resources.RnD = 1e12
	m.page = PageTech
	m.techCursor = 0
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(Model)
	if !strings.Contains(m.notice, "已解鎖") {
		t.Fatalf("tech unlock should set notice, got %q", m.notice)
	}
}

func TestOfflineReportCardRenders(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	m.offlineSummary = &Summary{
		SecondsSettled:    7200,
		TokensIn:          1000,
		TokensOut:         2000,
		RnDGained:         500,
		TrainingCompleted: true,
		CampaignCycles:    2,
	}
	m.offlineReports = []string{"· 宿敵行動 OpenAI · OpenAI 消費旗艦"}
	out := renderOfflineReport(m)
	for _, want := range []string{"離線戰報", "2.0h", "訓練完成", "董事會週期 2 次", "宿敵行動"} {
		if !strings.Contains(out, want) {
			t.Fatalf("offline report missing %q: %q", want, out)
		}
	}
	// View 顯示且任意鍵清除
	if v := m.View(); !strings.Contains(v, "離線戰報") {
		t.Fatal("View should embed offline report")
	}
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = mm.(Model)
	if m.offlineSummary != nil || m.offlineReports != nil {
		t.Fatal("any key should clear offline report")
	}
}
