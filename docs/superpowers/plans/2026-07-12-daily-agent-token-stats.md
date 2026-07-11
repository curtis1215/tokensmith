# Daily Agent Token Statistics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist and display the current local day's raw input/output token growth for Claude Code, Codex, Grok, and OpenCode, retaining seven days for debugging.

**Architecture:** Add an independent `internal/dailyusage` locked atomic JSON store and retry buffer. The daemon records exact harvested deltas; standalone TUI records direct poll events; daemon-mode TUI only reads the shared file. The overview renders the cached bucket for the host-local date.

**Tech Stack:** Go 1.25, `encoding/json`, `golang.org/x/sys/unix`, Bubble Tea, Lip Gloss, table-driven Go tests.

## Global Constraints

- Keys: `claude-code`, `codex`, `grok`, `opencode`; no model/provider/session split.
- Dates: `YYYY-MM-DD` in `time.Local`; retries keep the original observed date.
- Values: raw input/output only; no game multipliers.
- Keep seven valid date keys; no historical backfill.
- Path: `<os.UserConfigDir()>/tokensmith/daily-usage.json`; `0600` data/lock, stable lock, atomic rename.
- Statistics failure never blocks ledger, R&D, save, or gameplay.
- Do not change ledger/meta/save schemas.
- Strict RED → GREEN TDD with focused commits.

## File Map

- Create `internal/dailyusage/{types,store,lock_unix,buffer}.go` and tests.
- Modify `internal/daemon/daemon.go`, `cmd/tokensmithd/main.go`, and daemon tests.
- Modify `internal/tui/tui.go`, overview renderer, and TUI tests.
- Modify `DEPLOYMENT.md`.

---

### Task 1: Schema and pure aggregation

**Files:**
- Create: `internal/dailyusage/types.go`
- Create: `internal/dailyusage/types_test.go`
- Modify: `go.mod`, `go.sum`

**Produces:**

```go
type SourceUsage struct { In, Out int; LastUpdatedAt int64 }
type Document struct {
	SchemaVersion int
	UpdatedAt int64
	Days map[string]map[string]SourceUsage
}
type Batch struct { Day string; ObservedAt int64; Sources map[string]model.SourceTotals }
func DayKey(time.Time) string
func BatchFromEvents(time.Time, []model.TokenEvent) Batch
func Apply(*Document, Batch) bool
func DefaultPath() string
```

- [ ] **Step 1: Write failing aggregation/date/pruning tests**

```go
func TestBatchFromEventsAggregatesLocalDay(t *testing.T) {
	old := time.Local
	time.Local = time.FixedZone("UTC+8", 8*3600)
	t.Cleanup(func() { time.Local = old })
	at := time.Date(2026, 7, 12, 23, 59, 0, 0, time.Local)
	b := BatchFromEvents(at, []model.TokenEvent{
		{Source: "claude-code", InputTokens: 10, OutputTokens: 3},
		{Source: "claude-code", InputTokens: 7, OutputTokens: 2},
	})
	if b.Day != "2026-07-12" || b.Sources["claude-code"] != (model.SourceTotals{In: 17, Out: 5}) {
		t.Fatalf("batch=%+v", b)
	}
}

func TestApplyKeepsSevenNewestDays(t *testing.T) {
	var d Document
	for day := 1; day <= 8; day++ {
		Apply(&d, Batch{Day: fmt.Sprintf("2026-07-%02d", day), ObservedAt: int64(day),
			Sources: map[string]model.SourceTotals{"codex": {In: day}}})
	}
	if len(d.Days) != 7 || d.Days["2026-07-01"] != nil { t.Fatalf("days=%+v", d.Days) }
}
```

Also cover negative-component clamping, invalid dates, positive-only timestamps, and default path.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/dailyusage -run 'TestBatch|TestApply|TestDayKey|TestDefaultPath' -count=1`

Expected: package/types missing.

- [ ] **Step 3: Implement minimal pure helpers**

Use JSON names `schemaVersion`, `updatedAt`, `days`, `in`, `out`, `lastUpdatedAt`. `DayKey` uses `at.In(time.Local).Format("2006-01-02")`. `Apply` validates ISO dates, adds `max(0, component)`, sorts ISO keys, and removes all but seven newest. Promote `golang.org/x/sys v0.44.0` to direct dependency.

- [ ] **Step 4: Verify GREEN and commit**

```bash
go test ./internal/dailyusage -count=1
git add go.mod go.sum internal/dailyusage/types.go internal/dailyusage/types_test.go
git commit -m "feat(usage): define daily token accounting schema"
```

---

### Task 2: Locked store and retry buffer

**Files:**
- Create: `internal/dailyusage/store.go`, `lock_unix.go`, `store_test.go`
- Create: `internal/dailyusage/buffer.go`, `buffer_test.go`

**Produces:**

```go
type Sink interface { Add(Batch) error }
type Reader interface { Load() (Document, bool, error) }
func New(path string) *Store
func (s *Store) Load() (Document, bool, error)
func (s *Store) Add(Batch) error
func NewBuffer(Sink) *Buffer
func (b *Buffer) Record(Batch) error
func (b *Buffer) Flush() error
func (b *Buffer) Pending() int
```

- [ ] **Step 1: Write failing store/concurrency tests**

```go
func TestStoreConcurrentAddsLoseNoUpdates(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "daily-usage.json"))
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.Add(Batch{Day: "2026-07-12", ObservedAt: 100,
				Sources: map[string]model.SourceTotals{"codex": {In: 10, Out: 2}}}); err != nil { t.Errorf("Add: %v", err) }
		}()
	}
	wg.Wait()
	doc, _, _ := s.Load()
	got := doc.Days["2026-07-12"]["codex"]
	if got.In != 200 || got.Out != 40 { t.Fatalf("lost update=%+v", got) }
}
```

Also cover missing load, `0600`, round-trip, corrupt/unsupported backup `daily-usage.json.corrupt-<unix>`, and held-lock timeout without mutation.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/dailyusage -run TestStore -count=1`

- [ ] **Step 3: Implement lock and atomic store**

Use `unix.Flock(LOCK_EX|LOCK_NB)` every 10 ms with 250 ms deadline. Under lock: load; preserve corrupt bytes by rename; `Apply`; write/Sync/close a `0600` same-dir temp; atomic rename. Parent is `0700`. `Load` never mutates corrupt data.

- [ ] **Step 4: Verify store GREEN/race**

```bash
go test ./internal/dailyusage -run TestStore -count=1
go test -race ./internal/dailyusage -run TestStoreConcurrent -count=1
```

- [ ] **Step 5: Write failing exactly-once retry test**

```go
func TestBufferRetriesOriginalBatchExactlyOnce(t *testing.T) {
	sink := &fakeSink{failures: 1}
	b := NewBuffer(sink)
	original := Batch{Day: "2026-07-12", ObservedAt: 10, Sources: map[string]model.SourceTotals{"grok": {In: 42}}}
	if err := b.Record(original); err == nil || b.Pending() != 1 { t.Fatal("want pending") }
	if err := b.Flush(); err != nil { t.Fatal(err) }
	if b.Pending() != 0 || len(sink.saved) != 1 || sink.saved[0].Day != "2026-07-12" { t.Fatal("bad retry") }
	if err := b.Flush(); err != nil || len(sink.saved) != 1 { t.Fatal("duplicate") }
}
```

- [ ] **Step 6: Verify RED, implement FIFO, verify GREEN**

`Record` appends non-empty batches then calls `Flush`. `Flush` removes the head only after success. Nil sink is a disabled compatibility no-op.

Run: `go test -race ./internal/dailyusage -count=1`

- [ ] **Step 7: Commit**

```bash
git add internal/dailyusage
git commit -m "feat(usage): persist locked daily token history"
```

---

### Task 3: Daemon records exact deltas

**Files:**
- Modify: `internal/daemon/daemon.go`, `internal/daemon/daemon_test.go`
- Modify: `cmd/tokensmithd/main.go`

**Produces:**

```go
func NewWithSourcesAndDaily(
	claudeDirs, codexDirs []string,
	snapshots []ingest.SnapshotSource,
	daily *dailyusage.Buffer,
	ledgerPath string,
) *Harvester
```

Existing constructors remain compatible and install a disabled buffer.

- [ ] **Step 1: Write failing daemon integration tests**

Cover Claude+Codex append deltas; snapshot first-observation prime followed by exact Grok/OpenCode growth; daily sink failure still saves ledger; next no-event `Step` retries once.

```go
at := time.Date(2026, 7, 12, 23, 59, 0, 0, time.Local)
h := NewWithSourcesAndDaily([]string{claude}, []string{codex}, nil,
	dailyusage.NewBuffer(sink), ledgerPath)
writeLine(t, claudeFile, "A")
writeCodexLine(t, codexFile, 30, 15)
if err := h.Step(at.Unix()); err != nil { t.Fatal(err) }
if got := sink.saved[0].Sources["claude-code"]; got != (model.SourceTotals{In: 100, Out: 50}) {
	t.Fatalf("daily=%+v", got)
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/daemon -run TestHarvesterDaily -count=1`

- [ ] **Step 3: Expose exact positive harvest deltas**

Collect append events into `harvested []model.TokenEvent`. Change `applySnapshot` to return `(model.SourceTotals, bool)` only for known, non-decreasing, positive delta; append it using the snapshot source key. Then:

```go
batch := dailyusage.BatchFromEvents(time.Unix(now, 0), harvested)
dailyErr := h.daily.Record(batch) // empty batch also flushes pending work
```

Join `dailyErr`, snapshot errors, and `ledger.Save` via `errors.Join`. Do not alter cursor/watermark behavior.

- [ ] **Step 4: Wire production daemon**

```go
dailyStore := dailyusage.New(dailyusage.DefaultPath())
h := daemon.NewWithSourcesAndDaily(claudeRoots, codexRoots, snapshots,
	dailyusage.NewBuffer(dailyStore), lp)
```

Log both paths at startup.

- [ ] **Step 5: Verify GREEN/race and commit**

```bash
go test ./internal/daemon ./cmd/tokensmithd -count=1
go test -race ./internal/daemon -run TestHarvesterDaily -count=1
git add internal/daemon/daemon.go internal/daemon/daemon_test.go cmd/tokensmithd/main.go
git commit -m "feat(usage): record daemon token deltas by day"
```

---

### Task 4: TUI write/read/retry and midnight behavior

**Files:**
- Modify: `internal/tui/tui.go`, `internal/tui/tui_test.go`
- Modify: `internal/tui/daemon_integration_test.go`

**Model fields:**

```go
dailyDoc dailyusage.Document
dailyDay string
dailyReader dailyusage.Reader
dailyWriter *dailyusage.Buffer
dailyRefreshTicks int
```

- [ ] **Step 1: Write failing integration tests**

Prove standalone direct events write one batch, daemon-mode ledger events write zero batches, failure remains pending and a no-event tick retries it, reader error retains cache, refresh happens each 20 ticks, and midnight changes `dailyDay` to a zero new bucket without deleting yesterday.

```go
func TestDaemonModeTickNeverWritesDailyUsage(t *testing.T) {
	// Seed ledger growth, inject fake daily sink, set daemonMode=true, tick.
	// Assert R&D increases while sink Add count remains zero.
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/tui -run 'TestStandalone.*Daily|TestDaemonMode.*Daily|TestDailyView' -count=1`

- [ ] **Step 3: Initialize daily dependencies**

`newAtPaths` derives sibling `daily-usage.json`, creates one Store, loads it if valid, and initializes reader/writer/day. A daily load error must not set `startupErr` or `saveDisabled`.

- [ ] **Step 4: Record standalone events only**

Immediately after `events := m.pollTokens()`:

```go
m.dailyDay = dailyusage.DayKey(now)
if !m.daemonMode {
	batch := dailyusage.BatchFromEvents(now, events)
	if len(batch.Sources) > 0 { dailyusage.Apply(&m.dailyDoc, batch) }
	if err := m.dailyWriter.Record(batch); err != nil {
		m.setNotice("⚠ 今日 Token 統計暫存失敗，將自動重試")
	}
}
```

An empty `Record` flushes prior pending work. Never record daemon ledger deltas in TUI.

- [ ] **Step 5: Refresh without clobbering pending overlay**

Every 20 ticks call `dailyReader.Load`. Replace `dailyDoc` only on successful present load. Retain cache on error. In standalone mode skip disk replacement while `dailyWriter.Pending() > 0`.

- [ ] **Step 6: Verify GREEN/race and commit**

```bash
go test ./internal/tui -run 'TestStandalone.*Daily|TestDaemonMode.*Daily|TestDailyView' -count=1
go test -race ./internal/tui -run Daily -count=1
git add internal/tui/tui.go internal/tui/tui_test.go internal/tui/daemon_integration_test.go
git commit -m "feat(tui): track daily usage in both harvest modes"
```

---

### Task 5: Overview card, docs, and final gate

**Files:**
- Modify: `internal/tui/page_overview.go`, `internal/tui/page_overview_test.go`
- Modify: `DEPLOYMENT.md`

- [ ] **Step 1: Write failing wide/zero/narrow tests**

```go
func TestOverviewShowsDailyUsageBySource(t *testing.T) {
	m := testModel(t)
	m.dailyDay = "2026-07-12"
	m.dailyDoc.Days = map[string]map[string]dailyusage.SourceUsage{
		"2026-07-12": {
			"claude-code": {In: 120_000, Out: 18_000},
			"codex": {In: 85_000, Out: 12_000},
			"grok": {In: 30_000},
			"opencode": {In: 42_000, Out: 9_000},
		},
	}
	out := renderOverview(m)
	for _, want := range []string{"今日 Token 收成", "Claude Code", "Codex", "Grok（估算）", "OpenCode", "316K"} {
		if !strings.Contains(out, want) { t.Fatalf("missing %q:\n%s", want, out) }
	}
}
```

Also assert missing sources display `0`, Grok has no fake output amount, narrow width keeps all sources, and each line is within `contentWidth` by `lipgloss.Width`.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/tui -run 'TestOverview.*Daily' -count=1`

- [ ] **Step 3: Implement responsive card**

Add `renderDailyUsageCard(m Model, w int) string` immediately after `renderHQ`. Wide mode shows one row/source with total and input/output. Narrow mode wraps compact segments without dropping a source. Sum only four known keys. Title: `今日 Token 收成 · MM/DD`.

- [ ] **Step 4: Update `DEPLOYMENT.md`**

Document path/owner, seven-date retention, local-midnight keys, raw `in/out/lastUpdatedAt`, no backfill, and no effect on R&D.

- [ ] **Step 5: Run full verification**

```bash
gofmt -w internal/dailyusage internal/daemon/daemon.go internal/daemon/daemon_test.go \
  cmd/tokensmithd/main.go internal/tui/tui.go internal/tui/tui_test.go \
  internal/tui/daemon_integration_test.go internal/tui/page_overview.go \
  internal/tui/page_overview_test.go
git diff --check
go test ./...
go test -race ./internal/dailyusage ./internal/daemon ./internal/tui
go vet ./...
go build ./...
GOOS=darwin GOARCH=arm64 go build ./...
GOOS=linux GOARCH=amd64 go build ./...
```

Do not reformat unrelated pre-existing `internal/tui/layout.go`.

- [ ] **Step 6: Read-only acceptance probe**

Use temporary paths/fixtures. Confirm no snapshot backfill, subsequent exact deltas under local day, raw values match fixtures, next-day render is zero without deleting prior bucket, and no production data is modified.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/page_overview.go internal/tui/page_overview_test.go DEPLOYMENT.md
git commit -m "feat(tui): show today's per-agent token harvest"
```

---

## Review Loop Acceptance

Claude must return `APPROVE` with no Critical/Major/blocking findings and verify no mode double-counting, exact positive snapshot deltas without history, no concurrent/retry lost updates, original dates across midnight, isolation from ledger/save/R&D, all four responsive source rows, and complete test/race/vet/build/cross-build success.

Any `NEEDS_CHANGES` report returns verbatim to Grok. Grok adds a failing regression test first, makes the minimal fix, runs focused/full verification, commits, and reports completion. Claude re-reviews the cumulative branch. Repeat until accepted.
