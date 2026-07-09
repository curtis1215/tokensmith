package tui

import (
	"path/filepath"
	"testing"
	"time"

	"tokensmith/internal/ledger"
	"tokensmith/internal/model"
	"tokensmith/internal/store"
)

func TestTickConsumesLedgerDelta(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, "ledger.json")
	mp := filepath.Join(dir, "meta.json")
	ledger.Save(lp, ledger.Ledger{
		Sources:   map[string]model.SourceTotals{"claude-code": {In: 1000, Out: 500}},
		UpdatedAt: 9_000_000_000,
	})
	store.SaveMeta(mp, store.Meta{LastRealUnix: 9_000_000_000})

	m := newAtPaths(filepath.Join(dir, "s.json"), lp, mp)
	m.daemonMode = true // exercise the tick path in isolation
	before := m.state.Resources.RnD
	nm, _ := m.Update(tickMsg(time.Unix(0, 0)))
	got := nm.(Model)
	if got.state.Resources.RnD <= before {
		t.Fatalf("tick did not apply ledger delta as R&D")
	}
	if got.consumedIn != 1000 || got.consumedOut != 500 {
		t.Fatalf("consumed watermark not advanced: %d/%d", got.consumedIn, got.consumedOut)
	}
	// a second tick with no ledger growth adds no tokens
	nm2, _ := got.Update(tickMsg(time.Unix(0, 0)))
	if nm2.(Model).lastTokens != 0 {
		t.Fatalf("second tick should see no new tokens, got %d", nm2.(Model).lastTokens)
	}
}

func TestStartupSettlesOffline(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, "ledger.json")
	mp := filepath.Join(dir, "meta.json")
	now := int64(1_800_000_000)
	ledger.Save(lp, ledger.Ledger{
		Sources:   map[string]model.SourceTotals{"claude-code": {In: 300000, Out: 150000}},
		UpdatedAt: now,
	}) // fresh
	store.SaveMeta(mp, store.Meta{LastRealUnix: now - 8*3600})

	m := newAtPaths(filepath.Join(dir, "s.json"), lp, mp).startup(now)
	if !m.daemonMode {
		t.Fatal("fresh ledger should enable daemon mode")
	}
	if m.offlineSummary == nil || m.offlineSummary.RnDGained <= 0 {
		t.Fatalf("expected offline R&D gain, got %+v", m.offlineSummary)
	}
	if m.consumedIn != 300000 || m.consumedOut != 150000 {
		t.Fatalf("consumed should adopt ledger cum after settlement: %d/%d", m.consumedIn, m.consumedOut)
	}
}

func TestStartupFirstOpenNoSettlement(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, "ledger.json")
	mp := filepath.Join(dir, "meta.json") // no meta written → first-ever open
	now := int64(1_800_000_000)
	ledger.Save(lp, ledger.Ledger{
		Sources:   map[string]model.SourceTotals{"claude-code": {In: 999999, Out: 999999}},
		UpdatedAt: now,
	})

	m := newAtPaths(filepath.Join(dir, "s.json"), lp, mp).startup(now)
	if !m.daemonMode {
		t.Fatal("fresh ledger should enable daemon mode")
	}
	if m.offlineSummary != nil {
		t.Fatal("first-ever open should not settle a huge phantom offline window")
	}
	if m.consumedIn != 999999 {
		t.Fatalf("first open should adopt cum as consumed, got %d", m.consumedIn)
	}
}

func TestStartupStandaloneWhenLedgerStale(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, "ledger.json")
	mp := filepath.Join(dir, "meta.json")
	now := int64(1_800_000_000)
	ledger.Save(lp, ledger.Ledger{
		Sources:   map[string]model.SourceTotals{"claude-code": {In: 100}},
		UpdatedAt: now - 3600,
	}) // 1h stale

	m := newAtPaths(filepath.Join(dir, "s.json"), lp, mp).startup(now)
	if m.daemonMode {
		t.Fatal("stale ledger should fall back to standalone (poller) mode")
	}
}
