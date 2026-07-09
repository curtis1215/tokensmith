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
	if got.consumed["claude-code"] != (model.SourceTotals{In: 1000, Out: 500}) {
		t.Fatalf("consumed watermark not advanced: %+v", got.consumed)
	}
	if !got.tokensThisTick {
		t.Fatalf("first tick should report tokensThisTick=true")
	}
	// a second tick with no ledger growth adds no tokens
	nm2, _ := got.Update(tickMsg(time.Unix(0, 0)))
	if nm2.(Model).tokensThisTick {
		t.Fatalf("second tick should see no new tokens")
	}
}

func TestPollTokensSplitsMultipleSources(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, "ledger.json")
	mp := filepath.Join(dir, "meta.json")
	ledger.Save(lp, ledger.Ledger{
		Sources: map[string]model.SourceTotals{
			"claude-code": {In: 1000, Out: 500},
			"codex":       {In: 200, Out: 100},
		},
		UpdatedAt: 9_000_000_000,
	})
	store.SaveMeta(mp, store.Meta{LastRealUnix: 9_000_000_000})

	m := newAtPaths(filepath.Join(dir, "s.json"), lp, mp)
	m.daemonMode = true
	nm, _ := m.Update(tickMsg(time.Unix(0, 0)))
	got := nm.(Model)
	if len(got.lastTokenRnD) != 2 {
		t.Fatalf("expected 2 per-source R&D entries, got %+v", got.lastTokenRnD)
	}
	if got.lastTokenRnD["claude-code"] <= 0 || got.lastTokenRnD["codex"] <= 0 {
		t.Fatalf("both sources should contribute positive R&D: %+v", got.lastTokenRnD)
	}
	if got.consumed["codex"] != (model.SourceTotals{In: 200, Out: 100}) {
		t.Fatalf("codex watermark not advanced: %+v", got.consumed["codex"])
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
	if m.consumed["claude-code"] != (model.SourceTotals{In: 300000, Out: 150000}) {
		t.Fatalf("consumed should adopt ledger cum after settlement: %+v", m.consumed)
	}
	if m.streakDays != 1 {
		t.Fatalf("first-ever settled activity should start a streak of 1, got %d", m.streakDays)
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
	if m.consumed["claude-code"] != (model.SourceTotals{In: 999999, Out: 999999}) {
		t.Fatalf("first open should adopt cum as consumed, got %+v", m.consumed)
	}
}

func TestStartupStandaloneWhenLedgerStale(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, "ledger.json")
	mp := filepath.Join(dir, "meta.json")
	now := int64(1_800_000_000)
	ledger.Save(lp, ledger.Ledger{
		Sources:   map[string]model.SourceTotals{"claude-code": {In: 100}},
		UpdatedAt: now - 3600, // 1h stale
	})

	m := newAtPaths(filepath.Join(dir, "s.json"), lp, mp).startup(now)
	if m.daemonMode {
		t.Fatal("stale ledger should fall back to standalone (poller) mode")
	}
}

func TestStreakIncrementsOnConsecutiveDays(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "s.json"))
	day1 := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 7, 9, 9, 0, 0, 0, time.UTC)
	m.updateStreak(day1)
	if m.streakDays != 1 {
		t.Fatalf("first active day: streakDays = %d, want 1", m.streakDays)
	}
	m.updateStreak(day1) // same day again: no change
	if m.streakDays != 1 {
		t.Fatalf("same-day repeat: streakDays = %d, want 1", m.streakDays)
	}
	m.updateStreak(day2) // consecutive day: increments
	if m.streakDays != 2 {
		t.Fatalf("consecutive day: streakDays = %d, want 2", m.streakDays)
	}
}

func TestStreakResetsAfterGap(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "s.json"))
	day1 := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	day3 := time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC) // skipped day 9
	m.updateStreak(day1)
	m.updateStreak(day3)
	if m.streakDays != 1 {
		t.Fatalf("a skipped day should reset the streak: streakDays = %d, want 1", m.streakDays)
	}
}

func TestStreakMultCappedAtTenDays(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "s.json"))
	m.streakDays = 50
	if got := m.currentStreakMult(); !approxEq(got, 1.6) {
		t.Fatalf("currentStreakMult() at 50 days = %v, want 1.6 (capped at 10 days)", got)
	}
	m.streakDays = 5
	if got := m.currentStreakMult(); !approxEq(got, 1.3) {
		t.Fatalf("currentStreakMult() at 5 days = %v, want 1.3", got)
	}
}

func approxEq(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-9
}
