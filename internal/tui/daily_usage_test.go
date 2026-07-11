package tui

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"tokensmith/internal/dailyusage"
	"tokensmith/internal/ingest"
	"tokensmith/internal/ledger"
	"tokensmith/internal/model"
	"tokensmith/internal/store"
)

// countingDailySink records Add calls for TUI integration tests.
type countingDailySink struct {
	mu    sync.Mutex
	saved []dailyusage.Batch
	fail  int
	err   error
}

func (s *countingDailySink) Add(b dailyusage.Batch) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	if s.fail > 0 {
		s.fail--
		return errors.New("daily temporary failure")
	}
	s.saved = append(s.saved, b)
	return nil
}

func (s *countingDailySink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.saved)
}

type stickyDailyReader struct {
	mu    sync.Mutex
	doc   dailyusage.Document
	ok    bool
	err   error
	calls int
}

func (r *stickyDailyReader) Load() (dailyusage.Document, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	return r.doc, r.ok, r.err
}

func writeClaudeAssistantLine(t *testing.T, f, id string) {
	t.Helper()
	line := `{"type":"assistant","timestamp":"2026-07-07T10:59:19Z","message":{"id":"` + id + `","usage":{"input_tokens":100,"output_tokens":50}}}` + "\n"
	af, err := os.OpenFile(f, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := af.WriteString(line); err != nil {
		t.Fatal(err)
	}
	_ = af.Close()
}

func TestStandaloneTickRecordsDailyUsageOnce(t *testing.T) {
	old := time.Local
	time.Local = time.FixedZone("UTC+8", 8*3600)
	t.Cleanup(func() { time.Local = old })

	dir := t.TempDir()
	claude, codex := t.TempDir(), t.TempDir()
	claudeFile := filepath.Join(claude, "s.jsonl")

	m := newAtPaths(filepath.Join(dir, "s.json"), filepath.Join(dir, "ledger.json"), filepath.Join(dir, "meta.json"))
	m.poller = ingest.NewPoller(claude, codex)
	m.poller.Prime()
	sink := &countingDailySink{}
	m.dailyWriter = dailyusage.NewBuffer(sink)
	// Prevent disk refresh from interfering.
	m.dailyReader = &stickyDailyReader{ok: false}

	writeClaudeAssistantLine(t, claudeFile, "A")
	at := time.Date(2026, 7, 12, 15, 0, 0, 0, time.Local)
	nm, _ := m.Update(tickMsg(at))
	got := nm.(Model)

	if sink.count() != 1 {
		t.Fatalf("standalone should write exactly one daily batch, got %d", sink.count())
	}
	batch := sink.saved[0]
	if batch.Day != "2026-07-12" {
		t.Fatalf("day=%q", batch.Day)
	}
	if batch.Sources["claude-code"] != (model.SourceTotals{In: 100, Out: 50}) {
		t.Fatalf("batch=%+v", batch.Sources)
	}
	// In-memory overlay applied immediately.
	if got.dailyDoc.Days["2026-07-12"]["claude-code"].In != 100 {
		t.Fatalf("in-memory dailyDoc=%+v", got.dailyDoc.Days)
	}
	if got.dailyDay != "2026-07-12" {
		t.Fatalf("dailyDay=%q", got.dailyDay)
	}
	// R&D still applied from the event.
	if got.state.Resources.RnD <= m.state.Resources.RnD {
		t.Fatal("standalone tick should still convert tokens to R&D")
	}
}

func TestDaemonModeTickNeverWritesDailyUsage(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, "ledger.json")
	mp := filepath.Join(dir, "meta.json")
	ledger.Save(lp, ledger.Ledger{
		Sources:   map[string]model.SourceTotals{"claude-code": {In: 1000, Out: 500}},
		UpdatedAt: 9_000_000_000,
	})
	store.SaveMeta(mp, store.Meta{LastRealUnix: 9_000_000_000})

	m := newAtPaths(filepath.Join(dir, "s.json"), lp, mp)
	m.daemonMode = true
	sink := &countingDailySink{}
	m.dailyWriter = dailyusage.NewBuffer(sink)
	m.dailyReader = &stickyDailyReader{ok: false}

	before := m.state.Resources.RnD
	nm, _ := m.Update(tickMsg(time.Unix(0, 0)))
	got := nm.(Model)
	if got.state.Resources.RnD <= before {
		t.Fatal("daemon tick should still apply ledger R&D")
	}
	if sink.count() != 0 {
		t.Fatalf("daemon-mode TUI must never write daily usage, got %d", sink.count())
	}
}

func TestStandaloneDailyFailureRetriesOnNoEventTick(t *testing.T) {
	old := time.Local
	time.Local = time.FixedZone("UTC+8", 8*3600)
	t.Cleanup(func() { time.Local = old })

	dir := t.TempDir()
	claude, codex := t.TempDir(), t.TempDir()
	claudeFile := filepath.Join(claude, "s.jsonl")

	m := newAtPaths(filepath.Join(dir, "s.json"), filepath.Join(dir, "ledger.json"), filepath.Join(dir, "meta.json"))
	m.poller = ingest.NewPoller(claude, codex)
	m.poller.Prime()
	sink := &countingDailySink{fail: 1}
	m.dailyWriter = dailyusage.NewBuffer(sink)
	m.dailyReader = &stickyDailyReader{ok: false}

	writeClaudeAssistantLine(t, claudeFile, "A")
	at := time.Date(2026, 7, 12, 10, 0, 0, 0, time.Local)
	nm, _ := m.Update(tickMsg(at))
	got := nm.(Model)
	if sink.count() != 0 {
		t.Fatal("failed write must not acknowledge")
	}
	if got.dailyWriter.Pending() != 1 {
		t.Fatalf("pending=%d", got.dailyWriter.Pending())
	}
	if got.notice == "" {
		t.Fatal("want transient daily-stat warning notice")
	}
	// Overlay still applied in memory despite persistence failure.
	if got.dailyDoc.Days["2026-07-12"]["claude-code"].In != 100 {
		t.Fatalf("overlay missing: %+v", got.dailyDoc.Days)
	}

	// No-event tick retries once.
	nm2, _ := got.Update(tickMsg(at.Add(time.Second)))
	got2 := nm2.(Model)
	if sink.count() != 1 {
		t.Fatalf("retry count=%d", sink.count())
	}
	if got2.dailyWriter.Pending() != 0 {
		t.Fatalf("pending after success=%d", got2.dailyWriter.Pending())
	}
	// Empty flush must not duplicate.
	nm3, _ := got2.Update(tickMsg(at.Add(2 * time.Second)))
	if nm3.(Model).dailyWriter.Pending() != 0 || sink.count() != 1 {
		t.Fatalf("duplicate write: pending=%d count=%d", nm3.(Model).dailyWriter.Pending(), sink.count())
	}
}

func TestDailyViewRetainsCacheOnReadError(t *testing.T) {
	m := testModel(t)
	m.dailyDay = "2026-07-12"
	m.dailyDoc = dailyusage.Document{
		SchemaVersion: 1,
		Days: map[string]map[string]dailyusage.SourceUsage{
			"2026-07-12": {"claude-code": {In: 42, Out: 7}},
		},
	}
	reader := &stickyDailyReader{err: errors.New("boom")}
	m.dailyReader = reader
	m.dailyWriter = dailyusage.NewBuffer(nil)
	m.dailyRefreshTicks = dailyRefreshEveryTicks - 1

	nm, _ := m.Update(tickMsg(time.Date(2026, 7, 12, 12, 0, 0, 0, time.Local)))
	got := nm.(Model)
	if reader.calls != 1 {
		t.Fatalf("refresh calls=%d", reader.calls)
	}
	if got.dailyDoc.Days["2026-07-12"]["claude-code"].In != 42 {
		t.Fatalf("cache clobbered: %+v", got.dailyDoc.Days)
	}
}

func TestDailyViewRefreshEveryTwentyTicks(t *testing.T) {
	m := testModel(t)
	reader := &stickyDailyReader{
		ok: true,
		doc: dailyusage.Document{
			SchemaVersion: 1,
			Days: map[string]map[string]dailyusage.SourceUsage{
				"2026-07-12": {"codex": {In: 9}},
			},
		},
	}
	m.dailyReader = reader
	m.dailyWriter = dailyusage.NewBuffer(nil)
	m.daemonMode = true // skip standalone write path

	// 19 ticks: no load yet (counter starts 0, refresh at 20).
	for i := 0; i < dailyRefreshEveryTicks-1; i++ {
		nm, _ := m.Update(tickMsg(time.Unix(int64(i), 0)))
		m = nm.(Model)
	}
	if reader.calls != 0 {
		t.Fatalf("premature refresh: calls=%d", reader.calls)
	}
	nm, _ := m.Update(tickMsg(time.Unix(int64(dailyRefreshEveryTicks), 0)))
	m = nm.(Model)
	if reader.calls != 1 {
		t.Fatalf("refresh calls=%d, want 1", reader.calls)
	}
	if m.dailyDoc.Days["2026-07-12"]["codex"].In != 9 {
		t.Fatalf("doc not refreshed: %+v", m.dailyDoc.Days)
	}
}

func TestDailyViewMidnightSelectsZeroNewDay(t *testing.T) {
	old := time.Local
	time.Local = time.FixedZone("UTC+8", 8*3600)
	t.Cleanup(func() { time.Local = old })

	m := testModel(t)
	m.dailyDay = "2026-07-11"
	m.dailyDoc = dailyusage.Document{
		SchemaVersion: 1,
		Days: map[string]map[string]dailyusage.SourceUsage{
			"2026-07-11": {
				"claude-code": {In: 100, Out: 20},
				"codex":       {In: 50},
			},
		},
	}
	m.dailyReader = &stickyDailyReader{ok: false}
	m.dailyWriter = dailyusage.NewBuffer(nil)

	nextDay := time.Date(2026, 7, 12, 0, 0, 1, 0, time.Local)
	nm, _ := m.Update(tickMsg(nextDay))
	got := nm.(Model)
	if got.dailyDay != "2026-07-12" {
		t.Fatalf("dailyDay=%q, want 2026-07-12", got.dailyDay)
	}
	// Yesterday bucket retained.
	if got.dailyDoc.Days["2026-07-11"]["claude-code"].In != 100 {
		t.Fatalf("yesterday pruned: %+v", got.dailyDoc.Days)
	}
	// Today absent → zero view.
	if got.dailyDoc.Days["2026-07-12"] != nil {
		t.Fatalf("today should be empty/zero: %+v", got.dailyDoc.Days["2026-07-12"])
	}
}

func TestNewAtPathsLoadsDailyUsageWithoutBlockingStartup(t *testing.T) {
	dir := t.TempDir()
	sp := filepath.Join(dir, "s.json")
	// Corrupt daily file must not set startupErr/saveDisabled.
	dailyPath := filepath.Join(dir, "daily-usage.json")
	if err := os.WriteFile(dailyPath, []byte("{bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := newAtPaths(sp, filepath.Join(dir, "ledger.json"), filepath.Join(dir, "meta.json"))
	if m.startupErr != nil {
		t.Fatalf("daily load must not set startupErr: %v", m.startupErr)
	}
	if m.saveDisabled {
		t.Fatal("daily load must not disable save")
	}
	if m.dailyWriter == nil || m.dailyReader == nil {
		t.Fatal("daily writer/reader must be initialized")
	}
	// Valid load path.
	store := dailyusage.New(filepath.Join(t.TempDir(), "d.json"))
	_ = store.Add(dailyusage.Batch{
		Day: "2026-07-12", ObservedAt: 1,
		Sources: map[string]model.SourceTotals{"grok": {In: 3}},
	})
	// Re-point by writing sibling file before newAtPaths.
	dir2 := t.TempDir()
	goodPath := filepath.Join(dir2, "daily-usage.json")
	good := dailyusage.New(goodPath)
	if err := good.Add(dailyusage.Batch{
		Day: "2026-07-12", ObservedAt: 2,
		Sources: map[string]model.SourceTotals{"grok": {In: 3}},
	}); err != nil {
		t.Fatal(err)
	}
	m2 := newAtPaths(filepath.Join(dir2, "s.json"), filepath.Join(dir2, "ledger.json"), filepath.Join(dir2, "meta.json"))
	if m2.dailyDoc.Days["2026-07-12"]["grok"].In != 3 {
		t.Fatalf("startup load=%+v", m2.dailyDoc.Days)
	}
}

func TestStandaloneSkipsDiskRefreshWhilePending(t *testing.T) {
	m := testModel(t)
	// Seed in-memory overlay higher than disk.
	m.dailyDoc = dailyusage.Document{
		SchemaVersion: 1,
		Days: map[string]map[string]dailyusage.SourceUsage{
			"2026-07-12": {"claude-code": {In: 999}},
		},
	}
	reader := &stickyDailyReader{
		ok: true,
		doc: dailyusage.Document{
			SchemaVersion: 1,
			Days: map[string]map[string]dailyusage.SourceUsage{
				"2026-07-12": {"claude-code": {In: 1}},
			},
		},
	}
	sink := &countingDailySink{err: errors.New("always fail")}
	m.dailyReader = reader
	m.dailyWriter = dailyusage.NewBuffer(sink)
	// Force a pending batch without going through tick events.
	_ = m.dailyWriter.Record(dailyusage.Batch{
		Day: "2026-07-12", ObservedAt: 1,
		Sources: map[string]model.SourceTotals{"claude-code": {In: 5}},
	})
	if m.dailyWriter.Pending() != 1 {
		t.Fatal("setup pending")
	}
	m.daemonMode = false
	m.dailyRefreshTicks = dailyRefreshEveryTicks - 1
	nm, _ := m.Update(tickMsg(time.Date(2026, 7, 12, 1, 0, 0, 0, time.Local)))
	got := nm.(Model)
	if reader.calls != 0 {
		t.Fatalf("must skip disk refresh while pending, calls=%d", reader.calls)
	}
	if got.dailyDoc.Days["2026-07-12"]["claude-code"].In != 999 {
		t.Fatalf("overlay clobbered: %+v", got.dailyDoc.Days)
	}
}
