package tui

import (
	"os"
	"path/filepath"
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
