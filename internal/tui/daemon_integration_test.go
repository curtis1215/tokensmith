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
	sp := filepath.Join(dir, "s.json")
	now := int64(1_800_000_000)
	ledger.Save(lp, ledger.Ledger{
		Sources:   map[string]model.SourceTotals{"claude-code": {In: 300000, Out: 150000}},
		UpdatedAt: now,
	}) // fresh
	// Active campaign + stale board clock: offline catch-up must advance cycles
	// even on the daemon economic-settlement path.
	staleCampaign := now - 7*24*60*60
	if err := store.Save(sp, model.GameState{
		Campaign: model.CampaignState{
			Doctrine: model.DoctrineConsumer,
			Stage:    model.CampaignStageExpand,
		},
	}); err != nil {
		t.Fatal(err)
	}
	store.SaveMeta(mp, store.Meta{
		LastRealUnix:     now - 8*3600,
		LastCampaignUnix: staleCampaign,
	})

	m := newAtPaths(sp, lp, mp).startup(now)
	if !m.daemonMode {
		t.Fatal("fresh ledger should enable daemon mode")
	}
	if m.offlineSummary == nil || m.offlineSummary.RnDGained <= 0 {
		t.Fatalf("expected offline R&D gain, got %+v", m.offlineSummary)
	}
	if m.offlineSummary.CampaignCycles != 3 {
		t.Fatalf("daemon startup should catch up capped board cycles, got CampaignCycles=%d cycle=%d last=%d",
			m.offlineSummary.CampaignCycles, m.state.Campaign.Cycle, m.lastCampaignUnix)
	}
	if m.state.Campaign.Cycle != 3 {
		t.Fatalf("campaign cycle after daemon catch-up = %d, want 3", m.state.Campaign.Cycle)
	}
	if m.lastCampaignUnix != now {
		t.Fatalf("capped catch-up should set lastCampaignUnix=now, got %d want %d", m.lastCampaignUnix, now)
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
	sp := filepath.Join(dir, "s.json")
	now := int64(1_800_000_000)
	ledger.Save(lp, ledger.Ledger{
		Sources:   map[string]model.SourceTotals{"claude-code": {In: 100}},
		UpdatedAt: now - 3600, // 1h stale
	})
	// Standalone early-return must still advance board cycles (guards the
	// historical `return m` before campaign catch-up).
	staleCampaign := now - 7*24*60*60
	priorEconomic := now - 8*3600
	if err := store.Save(sp, model.GameState{
		Campaign: model.CampaignState{
			Doctrine: model.DoctrineConsumer,
			Stage:    model.CampaignStageExpand,
		},
	}); err != nil {
		t.Fatal(err)
	}
	store.SaveMeta(mp, store.Meta{
		LastRealUnix:     priorEconomic,
		LastCampaignUnix: staleCampaign,
	})

	m := newAtPaths(sp, lp, mp).startup(now)
	if m.daemonMode {
		t.Fatal("stale ledger should fall back to standalone (poller) mode")
	}
	if m.state.Campaign.Cycle != 3 {
		t.Fatalf("standalone startup should catch up capped board cycles, cycle=%d want 3", m.state.Campaign.Cycle)
	}
	if m.lastCampaignUnix != now {
		t.Fatalf("standalone catch-up lastCampaignUnix=%d want %d", m.lastCampaignUnix, now)
	}
	if m.offlineSummary == nil || m.offlineSummary.CampaignCycles != 3 {
		t.Fatalf("standalone catch-up should surface campaign cycle count in banner: %+v", m.offlineSummary)
	}

	// C1: board-only standalone open must not burn economic offline elapsed.
	meta, ok, err := store.LoadMeta(mp)
	if err != nil || !ok {
		t.Fatalf("reload meta after standalone startup: ok=%v err=%v", ok, err)
	}
	if meta.LastRealUnix != priorEconomic {
		t.Fatalf("standalone startup burned LastRealUnix: got %d want preserved %d", meta.LastRealUnix, priorEconomic)
	}
	if meta.LastCampaignUnix != now {
		t.Fatalf("standalone startup LastCampaignUnix on disk=%d want %d", meta.LastCampaignUnix, now)
	}

	// I1: advanced campaign state must be on disk, not only in memory/meta.
	saved, sok, serr := store.Load(sp)
	if serr != nil || !sok {
		t.Fatalf("reload save after standalone catch-up: ok=%v err=%v", sok, serr)
	}
	if saved.Campaign.Cycle != 3 {
		t.Fatalf("persisted Campaign.Cycle=%d want 3", saved.Campaign.Cycle)
	}
	if len(saved.Campaign.Reports) == 0 {
		t.Fatal("persisted campaign should include board reports after catch-up cycles")
	}
}

func TestStartupDaemonPersistsSyntheticLastRealUnix(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, "ledger.json")
	mp := filepath.Join(dir, "meta.json")
	sp := filepath.Join(dir, "s.json")
	now := int64(1_800_000_000)
	ledger.Save(lp, ledger.Ledger{
		Sources:   map[string]model.SourceTotals{"claude-code": {In: 300000, Out: 150000}},
		UpdatedAt: now,
	})
	if err := store.Save(sp, model.GameState{}); err != nil {
		t.Fatal(err)
	}
	store.SaveMeta(mp, store.Meta{LastRealUnix: now - 8*3600})

	// Sleep so wall-clock now diverges from synthetic startup(now); meta must
	// still stamp the argument, not time.Now().
	time.Sleep(20 * time.Millisecond)
	_ = newAtPaths(sp, lp, mp).startup(now)

	meta, ok, err := store.LoadMeta(mp)
	if err != nil || !ok {
		t.Fatalf("reload meta after daemon startup: ok=%v err=%v", ok, err)
	}
	if meta.LastRealUnix != now {
		t.Fatalf("daemon economic startup LastRealUnix=%d want synthetic now=%d (not wall clock)", meta.LastRealUnix, now)
	}
	wall := time.Now().Unix()
	if meta.LastRealUnix == wall || meta.LastRealUnix > now {
		t.Fatalf("LastRealUnix looks like wall stamp: meta=%d wall=%d synthetic=%d", meta.LastRealUnix, wall, now)
	}
}

func TestStartupCatchUpPersistsCampaignState(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, "ledger.json")
	mp := filepath.Join(dir, "meta.json")
	sp := filepath.Join(dir, "s.json")
	now := int64(1_800_000_000)
	// Stale ledger → standalone path still persists advanced campaign state.
	ledger.Save(lp, ledger.Ledger{
		Sources:   map[string]model.SourceTotals{"claude-code": {In: 50}},
		UpdatedAt: now - 3600,
	})
	staleCampaign := now - 7*24*60*60
	if err := store.Save(sp, model.GameState{
		Campaign: model.CampaignState{
			Doctrine: model.DoctrineConsumer,
			Stage:    model.CampaignStageExpand,
			Cycle:    0,
		},
	}); err != nil {
		t.Fatal(err)
	}
	store.SaveMeta(mp, store.Meta{
		LastRealUnix:     now - 10*3600,
		LastCampaignUnix: staleCampaign,
	})

	_ = newAtPaths(sp, lp, mp).startup(now)

	saved, ok, err := store.Load(sp)
	if err != nil || !ok {
		t.Fatalf("reload save: ok=%v err=%v", ok, err)
	}
	if saved.Campaign.Cycle != 3 {
		t.Fatalf("Campaign.Cycle after reload=%d want 3", saved.Campaign.Cycle)
	}
	if len(saved.Campaign.Reports) != 3 {
		t.Fatalf("board reports after reload=%d want 3", len(saved.Campaign.Reports))
	}
	for i, r := range saved.Campaign.Reports {
		if r.Cycle != i+1 {
			t.Fatalf("report[%d].Cycle=%d want %d", i, r.Cycle, i+1)
		}
	}
	meta, mok, merr := store.LoadMeta(mp)
	if merr != nil || !mok {
		t.Fatalf("reload meta: ok=%v err=%v", mok, merr)
	}
	if meta.LastCampaignUnix != now {
		t.Fatalf("LastCampaignUnix after catch-up=%d want %d", meta.LastCampaignUnix, now)
	}
	if meta.LastRealUnix != now-10*3600 {
		t.Fatalf("catch-up must not burn LastRealUnix: got %d", meta.LastRealUnix)
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
