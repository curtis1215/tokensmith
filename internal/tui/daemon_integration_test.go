package tui

import (
	"os"
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

// TestStartupCatchUpNoCampaignSpending asserts offline board catch-up after a
// long absence advances exactly MaxCatchupCycles (3) without synthesizing player
// commands (directives / doctrine / settlement) or spending campaign cash.
func TestStartupCatchUpNoCampaignSpending(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, "ledger.json")
	mp := filepath.Join(dir, "meta.json")
	sp := filepath.Join(dir, "s.json")
	now := int64(1_800_000_000)
	// Stale ledger → standalone: economic offline settlement skipped; only board catch-up.
	ledger.Save(lp, ledger.Ledger{
		Sources:   map[string]model.SourceTotals{"claude-code": {In: 10}},
		UpdatedAt: now - 3600,
	})
	const cash, rnd = 50_000.0, 12_000.0
	staleCampaign := now - 72*3600 // exactly 72h → 3 cycles at 8h cadence (raw=3, no backlog drop)
	if err := store.Save(sp, model.GameState{
		Resources: model.Resources{Cash: cash, RnD: rnd},
		Campaign: model.CampaignState{
			Doctrine:      model.DoctrineConsumer,
			Stage:         model.CampaignStageExpand,
			Cycle:         5,
			Perks:         []string{"consumer-premium"},
			DirectiveUsed: true, // must clear via cycle reset, not a new directive spend
			Active:        nil,  // no paid modifiers to age-in
		},
	}); err != nil {
		t.Fatal(err)
	}
	store.SaveMeta(mp, store.Meta{
		LastRealUnix:      now - 10*3600,
		LastCampaignUnix:  staleCampaign,
		LastCampaignCycle: 5, // in sync with state Cycle → legitimate wall-clock catch-up
	})

	m := newAtPaths(sp, lp, mp).startup(now)
	if m.daemonMode {
		t.Fatal("stale ledger should be standalone")
	}
	if m.offlineSummary == nil || m.offlineSummary.CampaignCycles != 3 {
		t.Fatalf("want exactly 3 catch-up cycles after 72h, got %+v cycle=%d", m.offlineSummary, m.state.Campaign.Cycle)
	}
	if m.state.Campaign.Cycle != 8 {
		t.Fatalf("cycle after catch-up=%d want 8 (started 5 + 3)", m.state.Campaign.Cycle)
	}
	// No campaign spending: cash/RnD unchanged (catch-up never Apply's IssueDirective etc.).
	if m.state.Resources.Cash != cash || m.state.Resources.RnD != rnd {
		t.Fatalf("catch-up spent resources: cash=%v rnd=%v want cash=%v rnd=%v",
			m.state.Resources.Cash, m.state.Resources.RnD, cash, rnd)
	}
	if m.state.Campaign.DirectiveUsed {
		t.Fatal("DirectiveUsed should be reset by catch-up cycles, not left spent")
	}
	if len(m.state.Campaign.Active) != 0 {
		t.Fatalf("catch-up must not synthesize directive modifiers: %+v", m.state.Campaign.Active)
	}
	// Reports only from board ticks (rival/progress/distress), not player command markers.
	for _, r := range m.state.Campaign.Reports {
		for _, e := range r.Entries {
			switch e.Kind {
			case model.ReportDoctrineChosen:
				t.Fatalf("catch-up synthesized doctrine command report: %+v", e)
			}
		}
	}

	// Daemon path after 72h: same board-cap + no cash burn from campaign commands.
	dir2 := t.TempDir()
	lp2 := filepath.Join(dir2, "ledger.json")
	mp2 := filepath.Join(dir2, "meta.json")
	sp2 := filepath.Join(dir2, "s.json")
	ledger.Save(lp2, ledger.Ledger{
		Sources:   map[string]model.SourceTotals{"claude-code": {In: 100}}, // small; no huge offline R&D required
		UpdatedAt: now,                                                     // fresh ledger → daemon
	})
	if err := store.Save(sp2, model.GameState{
		Resources: model.Resources{Cash: cash, RnD: rnd},
		Campaign: model.CampaignState{
			Doctrine: model.DoctrineEnterprise,
			Stage:    model.CampaignStageEstablish,
			Cycle:    1,
		},
	}); err != nil {
		t.Fatal(err)
	}
	store.SaveMeta(mp2, store.Meta{
		LastRealUnix:      now, // no economic offline window
		LastCampaignUnix:  now - 72*3600,
		LastCampaignCycle: 1, // in sync with state Cycle
	})
	m2 := newAtPaths(sp2, lp2, mp2).startup(now)
	if !m2.daemonMode {
		t.Fatal("fresh ledger should enable daemon mode")
	}
	if m2.offlineSummary == nil || m2.offlineSummary.CampaignCycles != 3 {
		t.Fatalf("daemon 72h catch-up want 3 cycles, got %+v", m2.offlineSummary)
	}
	if m2.state.Campaign.Cycle != 4 {
		t.Fatalf("daemon cycle=%d want 4", m2.state.Campaign.Cycle)
	}
	if m2.state.Resources.Cash != cash {
		t.Fatalf("daemon catch-up burned cash: %v want %v", m2.state.Resources.Cash, cash)
	}
}

// TestStartupStateAheadMetaStaleRecovery: GameState saved at Cycle=3 with reports,
// but meta still has LastCampaignCycle=0 and an old LastCampaignUnix. A long gap
// must NOT re-apply catch-up cycles (would go to 6); instead arm the clock at now
// and persist LastCampaignCycle=3.
func TestStartupStateAheadMetaStaleRecovery(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, "ledger.json")
	mp := filepath.Join(dir, "meta.json")
	sp := filepath.Join(dir, "s.json")
	now := int64(1_800_000_000)
	// Stale ledger → standalone (board-only open; preserve economic watermark).
	ledger.Save(lp, ledger.Ledger{
		Sources:   map[string]model.SourceTotals{"claude-code": {In: 10}},
		UpdatedAt: now - 3600,
	})
	priorEconomic := now - 20*3600
	staleCampaign := now - 7*24*60*60
	if err := store.Save(sp, model.GameState{
		Campaign: model.CampaignState{
			Doctrine: model.DoctrineConsumer,
			Stage:    model.CampaignStageExpand,
			Cycle:    3,
			Reports: []model.BoardReport{
				{Cycle: 1}, {Cycle: 2}, {Cycle: 3},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	// Meta high-water lagging behind saved state (Save succeeded, SaveMeta failed).
	store.SaveMeta(mp, store.Meta{
		LastRealUnix:      priorEconomic,
		LastCampaignUnix:  staleCampaign,
		LastCampaignCycle: 0,
	})

	m := newAtPaths(sp, lp, mp).startup(now)
	if m.state.Campaign.Cycle != 3 {
		t.Fatalf("recovery must keep Cycle=3, got %d (replay would yield 6)", m.state.Campaign.Cycle)
	}
	if m.lastCampaignUnix != now {
		t.Fatalf("recovery lastCampaignUnix=%d want now=%d", m.lastCampaignUnix, now)
	}
	if m.offlineSummary != nil && m.offlineSummary.CampaignCycles != 0 {
		t.Fatalf("recovery must not report catch-up cycles: %+v", m.offlineSummary)
	}
	// Disk state unchanged (no replay).
	saved, ok, err := store.Load(sp)
	if err != nil || !ok {
		t.Fatalf("reload save: ok=%v err=%v", ok, err)
	}
	if saved.Campaign.Cycle != 3 || len(saved.Campaign.Reports) != 3 {
		t.Fatalf("disk state mutated: cycle=%d reports=%d", saved.Campaign.Cycle, len(saved.Campaign.Reports))
	}
	meta, mok, merr := store.LoadMeta(mp)
	if merr != nil || !mok {
		t.Fatalf("reload meta: ok=%v err=%v", mok, merr)
	}
	if meta.LastCampaignUnix != now {
		t.Fatalf("reconciled LastCampaignUnix=%d want %d", meta.LastCampaignUnix, now)
	}
	if meta.LastCampaignCycle != 3 {
		t.Fatalf("reconciled LastCampaignCycle=%d want 3", meta.LastCampaignCycle)
	}
	if meta.LastRealUnix != priorEconomic {
		t.Fatalf("recovery must not burn LastRealUnix: got %d want %d", meta.LastRealUnix, priorEconomic)
	}
}

// TestStartupCatchUpSkipsMetaWhenStateSaveFails: if store.Save fails after
// catch-up, meta watermarks must remain unchanged so a later launch can retry.
func TestStartupCatchUpSkipsMetaWhenStateSaveFails(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, "ledger.json")
	mp := filepath.Join(dir, "meta.json")
	// Put save path inside a file-as-directory so MkdirAll/WriteFile fails.
	blocked := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocked, []byte("not-a-dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	sp := filepath.Join(blocked, "s.json") // parent is a file → Save fails
	now := int64(1_800_000_000)
	ledger.Save(lp, ledger.Ledger{
		Sources:   map[string]model.SourceTotals{"claude-code": {In: 10}},
		UpdatedAt: now - 3600,
	})
	staleCampaign := now - 7*24*60*60
	priorEconomic := now - 10*3600
	// Seed meta as if a prior successful session armed the clocks; state lives only
	// in memory (save path cannot load) so use newAtPaths after writing meta, then
	// force campaign fields. Instead: write state elsewhere and point savePath to
	// a failing path after construction — exercise startup via a thin helper path.
	// Simpler approach: create a readable save, then swap savePath to unwritable.
	goodSP := filepath.Join(dir, "good.json")
	if err := store.Save(goodSP, model.GameState{
		Campaign: model.CampaignState{
			Doctrine: model.DoctrineConsumer,
			Stage:    model.CampaignStageExpand,
			Cycle:    0,
		},
	}); err != nil {
		t.Fatal(err)
	}
	store.SaveMeta(mp, store.Meta{
		LastRealUnix:      priorEconomic,
		LastCampaignUnix:  staleCampaign,
		LastCampaignCycle: 0,
	})
	m := newAtPaths(goodSP, lp, mp)
	// Redirect writes to the failing path so catch-up Save fails.
	m.savePath = sp
	m = m.startup(now)
	if m.state.Campaign.Cycle != 3 {
		// In-memory still advanced; disk/meta must not claim the watermark.
		t.Fatalf("in-memory cycle after catch-up=%d want 3", m.state.Campaign.Cycle)
	}
	meta, ok, err := store.LoadMeta(mp)
	if err != nil || !ok {
		t.Fatalf("reload meta: ok=%v err=%v", ok, err)
	}
	if meta.LastCampaignUnix != staleCampaign {
		t.Fatalf("failed Save must leave LastCampaignUnix: got %d want %d", meta.LastCampaignUnix, staleCampaign)
	}
	if meta.LastCampaignCycle != 0 {
		t.Fatalf("failed Save must leave LastCampaignCycle: got %d want 0", meta.LastCampaignCycle)
	}
	if meta.LastRealUnix != priorEconomic {
		t.Fatalf("failed Save must leave LastRealUnix: got %d want %d", meta.LastRealUnix, priorEconomic)
	}
	// Ensure the failing path never produced a save file.
	if _, err := os.Stat(sp); err == nil {
		t.Fatal("expected save path to remain unwritable / missing")
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
