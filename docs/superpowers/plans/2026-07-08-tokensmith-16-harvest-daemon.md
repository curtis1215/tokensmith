# Harvest Daemon + Offline Settlement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A background `tokensmithd` daemon continuously harvests real Claude/Codex token usage into a durable ledger; the single-process TUI consumes the ledger (online + offline unified) and settles offline progress on open, so coding done while the game is closed still fuels the company.

**Architecture:** Decoupled "daemon harvests, TUI consumes" (spec §10.2). The daemon is the only reader of the logs; it accumulates cumulative token counts + durable cursors into `ledger.json` (atomic writes, ~5s cadence, single-instance lock). The TUI reads the ledger delta each tick as token events and, on open, settles the elapsed offline window by chunk-ticking the pure sim with the offline tokens. The sim stays pure; wall-clock lives only in a TUI sidecar `meta.json` and in the daemon's `updatedAt`. When the ledger is stale/absent the TUI falls back to its built-in poller so the game still runs standalone.

**Tech Stack:** Go 1.22+, standard library only (os/signal, encoding/json). No new deps. Reuses `internal/ingest` poller and `internal/store` atomic-write pattern.

## Global Constraints

- Module `tokensmith`; Go 1.22+.
- `internal/sim` stays pure (no wall-clock/rand/I/O; dt only). Offline settlement lives in `internal/tui`, not sim.
- Non-disruptive: all existing tests across every package stay green. The TUI keeps working with no daemon and no ledger (fallback path).
- Atomic persistence: temp-file + rename (mirror `internal/store/store.go`).
- Daemon and TUI never write the same file: daemon writes `ledger.json`; TUI writes `save.json` + `meta.json`.
- Clock use is confined to `cmd/tokensmithd`, `internal/daemon` (inject a `now func() int64`), and `internal/tui` (real `time.Now`). Library packages take time as parameters so tests are deterministic.

---

## File Structure

- `internal/ledger/ledger.go` (new) — `Ledger{CumIn, CumOut int; UpdatedAt int64; Cursors []ingest.CursorState}`, atomic `Save`/`Load`, `DefaultPath`.
- `internal/ingest/cursor.go` (new) — exported `CursorState{Path string; Inode uint64; Offset int64}` + `Poller.ExportCursors()` / `Poller.ImportCursors()`.
- `internal/daemon/daemon.go` (new) — `Harvester` wrapping a poller + ledger; `New(...)` restores cursors or primes to EOF; `Step(now int64)` polls, accumulates, persists.
- `cmd/tokensmithd/main.go` (new) — single-instance lock (PID file) + harvest loop + graceful shutdown.
- `internal/daemon/lock.go` (new) — `AcquireLock(path)` / release; stale-lock detection.
- `internal/store/meta.go` (new) — `Meta{ConsumedIn, ConsumedOut int; LastRealUnix int64}`, atomic `SaveMeta`/`LoadMeta`, `DefaultMetaPath`.
- `internal/tui/settle.go` (new) — `Settle(state, cfg, elapsedSec float64, offlineIn, offlineOut int) (model.GameState, Summary)`.
- `internal/tui/tui.go` (modify) — Init offline settlement; tick consumes ledger delta with poller fallback; save persists meta.
- Tests alongside each new file.

---

## Task 1: Ledger persistence

**Files:** Create `internal/ledger/ledger.go`, `internal/ledger/ledger_test.go`.

**Interfaces:**
- `type Ledger struct { CumIn, CumOut int; UpdatedAt int64; Cursors []ingest.CursorState }`
- `func Save(path string, l Ledger) error` — atomic temp+rename.
- `func Load(path string) (Ledger, bool, error)` — ok=false when absent.
- `func DefaultPath() string` — `UserConfigDir/tokensmith/ledger.json`.

Note: this imports `internal/ingest` for `CursorState`; Task 2 creates that type. If executing strictly in order, define Task 2's `CursorState` first, or temporarily use an inline field type and swap. Recommended: do Task 2 before Task 1's `go build`.

- [ ] **Step 1: Write failing test**

```go
package ledger

import (
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "ledger.json")
	if _, ok, _ := Load(p); ok {
		t.Fatal("missing ledger should report ok=false")
	}
	in := Ledger{CumIn: 100, CumOut: 50, UpdatedAt: 1700000000}
	if err := Save(p, in); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Load(p)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.CumIn != 100 || got.CumOut != 50 || got.UpdatedAt != 1700000000 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}
```

- [ ] **Step 2: Run to verify fail** — `go test ./internal/ledger` → FAIL (package/undefined).

- [ ] **Step 3: Implement** (mirror `internal/store/store.go`)

```go
// Package ledger persists the daemon's cumulative token harvest.
package ledger

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"tokensmith/internal/ingest"
)

type Ledger struct {
	CumIn     int                   `json:"cumIn"`
	CumOut    int                   `json:"cumOut"`
	UpdatedAt int64                 `json:"updatedAt"`
	Cursors   []ingest.CursorState  `json:"cursors,omitempty"`
}

func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "tokensmith", "ledger.json")
}

func Save(path string, l Ledger) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(l)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func Load(path string) (Ledger, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Ledger{}, false, nil
	}
	if err != nil {
		return Ledger{}, false, err
	}
	var l Ledger
	if err := json.Unmarshal(data, &l); err != nil {
		return Ledger{}, false, err
	}
	return l, true, nil
}
```

- [ ] **Step 4: Run to verify pass** — `go test ./internal/ledger` → PASS.

- [ ] **Step 5: Commit** — `feat(ledger): atomic cumulative-harvest ledger`.

---

## Task 2: Poller cursor export/import

**Files:** Create `internal/ingest/cursor.go`, `internal/ingest/cursor_test.go`.

**Interfaces:**
- `type CursorState struct { Path string; Inode uint64; Offset int64 }`
- `func (p *Poller) ExportCursors() []CursorState` — snapshot of the internal cursor map (stable order not required).
- `func (p *Poller) ImportCursors(cs []CursorState)` — replace the cursor map so a fresh poller resumes without re-reading.

- [ ] **Step 1: Write failing test**

```go
package ingest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCursorExportImportResumes(t *testing.T) {
	claude := t.TempDir()
	f := filepath.Join(claude, "s.jsonl")
	line := `{"type":"assistant","timestamp":"2026-07-07T10:59:19Z","message":{"id":"A","usage":{"input_tokens":100,"output_tokens":50}}}` + "\n"
	os.WriteFile(f, []byte(line), 0o644)

	p1 := NewPoller(claude, t.TempDir())
	if got := p1.Poll(); len(got) != 1 {
		t.Fatalf("p1 first poll = %d, want 1", len(got))
	}
	saved := p1.ExportCursors()

	// A fresh poller restoring the cursors must NOT re-read the existing line.
	p2 := NewPoller(claude, t.TempDir())
	p2.ImportCursors(saved)
	if got := p2.Poll(); len(got) != 0 {
		t.Fatalf("p2 after restore = %d, want 0 (resumed)", len(got))
	}
}
```

- [ ] **Step 2: Run to verify fail** — FAIL (undefined CursorState/ExportCursors).

- [ ] **Step 3: Implement** (`internal/ingest/cursor.go`) — expose the existing `cursors map[string]fileCursor`:

```go
package ingest

// CursorState is a durable, exported snapshot of one file's tail position.
type CursorState struct {
	Path   string `json:"path"`
	Inode  uint64 `json:"inode"`
	Offset int64  `json:"offset"`
}

func (p *Poller) ExportCursors() []CursorState {
	out := make([]CursorState, 0, len(p.cursors))
	for path, c := range p.cursors {
		out = append(out, CursorState{Path: path, Inode: c.inode, Offset: c.offset})
	}
	return out
}

func (p *Poller) ImportCursors(cs []CursorState) {
	for _, c := range cs {
		p.cursors[c.Path] = fileCursor{inode: c.Inode, offset: c.Offset}
	}
}
```

- [ ] **Step 4: Run to verify pass** — `go test ./internal/ingest` → PASS.

- [ ] **Step 5: Commit** — `feat(ingest): export/import poller cursors for durability`.

---

## Task 3: Daemon harvester

**Files:** Create `internal/daemon/daemon.go`, `internal/daemon/daemon_test.go`.

**Interfaces:**
- `type Harvester struct { ... }`
- `func New(claudeDir, codexDir, ledgerPath string) *Harvester` — loads ledger; restores cursors if present, else primes the poller to EOF (skip history) and starts cum at 0.
- `func (h *Harvester) Step(now int64) error` — poll, add token counts to cum, persist ledger (with fresh cursors + UpdatedAt=now).
- `func (h *Harvester) Ledger() ledger.Ledger` — current in-memory ledger (for tests).

- [ ] **Step 1: Write failing test**

```go
package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"tokensmith/internal/ledger"
)

func writeLine(t *testing.T, f, id string) {
	line := `{"type":"assistant","timestamp":"2026-07-07T10:59:19Z","message":{"id":"` + id + `","usage":{"input_tokens":100,"output_tokens":50}}}` + "\n"
	af, err := os.OpenFile(f, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	af.WriteString(line)
	af.Close()
}

func TestHarvesterAccumulatesAndResumes(t *testing.T) {
	claude, codex := t.TempDir(), t.TempDir()
	lp := filepath.Join(t.TempDir(), "ledger.json")
	f := filepath.Join(claude, "s.jsonl")

	h := New(claude, codex, lp)
	writeLine(t, f, "A")
	if err := h.Step(1000); err != nil {
		t.Fatal(err)
	}
	if l := h.Ledger(); l.CumIn != 100 || l.CumOut != 50 {
		t.Fatalf("after 1 line: %+v, want 100/50", l)
	}
	// second Step, no new data → unchanged
	h.Step(1001)
	if l := h.Ledger(); l.CumIn != 100 {
		t.Fatalf("no-new-data Step double counted: %+v", l)
	}
	// restart from persisted ledger → must not re-read the existing line
	h2 := New(claude, codex, lp)
	writeLine(t, f, "B")
	h2.Step(2000)
	if l := h2.Ledger(); l.CumIn != 200 || l.CumOut != 100 {
		t.Fatalf("after restart+1 line: %+v, want 200/100 (resumed, not re-read)", l)
	}

	got, ok, _ := ledger.Load(lp)
	if !ok || got.UpdatedAt != 2000 {
		t.Fatalf("ledger not persisted with UpdatedAt: %+v ok=%v", got, ok)
	}
}
```

- [ ] **Step 2: Run to verify fail** — FAIL.

- [ ] **Step 3: Implement**

```go
// Package daemon harvests real token usage into the ledger.
package daemon

import (
	"tokensmith/internal/ingest"
	"tokensmith/internal/ledger"
)

type Harvester struct {
	poller     *ingest.Poller
	ledgerPath string
	cur        ledger.Ledger
}

// New loads any existing ledger and resumes from its cursors; on a fresh start
// it primes the poller to EOF so months of old logs are not counted.
func New(claudeDir, codexDir, ledgerPath string) *Harvester {
	p := ingest.NewPoller(claudeDir, codexDir)
	cur, ok, _ := ledger.Load(ledgerPath)
	if ok {
		p.ImportCursors(cur.Cursors)
	} else {
		p.Prime() // skip history on first-ever start
	}
	return &Harvester{poller: p, ledgerPath: ledgerPath, cur: cur}
}

func (h *Harvester) Step(now int64) error {
	for _, e := range h.poller.Poll() {
		h.cur.CumIn += e.InputTokens
		h.cur.CumOut += e.OutputTokens
	}
	h.cur.UpdatedAt = now
	h.cur.Cursors = h.poller.ExportCursors()
	return ledger.Save(h.ledgerPath, h.cur)
}

func (h *Harvester) Ledger() ledger.Ledger { return h.cur }
```

- [ ] **Step 4: Run to verify pass** — `go test ./internal/daemon` → PASS.

- [ ] **Step 5: Commit** — `feat(daemon): resumable token harvester over the ledger`.

---

## Task 4: Single-instance lock + `tokensmithd` binary

**Files:** Create `internal/daemon/lock.go`, `internal/daemon/lock_test.go`, `cmd/tokensmithd/main.go`.

**Interfaces:**
- `func AcquireLock(path string) (release func() error, err error)` — writes the current PID; fails if a live process already holds it; steals a stale lock (PID not running).
- `func processAlive(pid int) bool` — `os.FindProcess` + `Signal(syscall.Signal(0))`.

- [ ] **Step 1: Write failing test**

```go
package daemon

import (
	"path/filepath"
	"testing"
)

func TestLockExcludesSecondHolder(t *testing.T) {
	p := filepath.Join(t.TempDir(), "daemon.lock")
	rel, err := AcquireLock(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireLock(p); err == nil {
		t.Fatal("second AcquireLock should fail while first is held")
	}
	if err := rel(); err != nil {
		t.Fatal(err)
	}
	// after release, a new holder can acquire (stale/removed lock)
	rel2, err := AcquireLock(p)
	if err != nil {
		t.Fatalf("acquire after release failed: %v", err)
	}
	rel2()
}
```

- [ ] **Step 2: Run to verify fail** — FAIL.

- [ ] **Step 3: Implement `lock.go`**

```go
package daemon

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// AcquireLock writes the current PID to path. It fails if a live process
// already holds the lock; a stale lock (dead PID) is stolen.
func AcquireLock(path string) (func() error, error) {
	if data, err := os.ReadFile(path); err == nil {
		if pid, perr := strconv.Atoi(strings.TrimSpace(string(data))); perr == nil && processAlive(pid) {
			return nil, errors.New("daemon: already running (pid " + strconv.Itoa(pid) + ")")
		}
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		return nil, err
	}
	return func() error { return os.Remove(path) }, nil
}

func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}
```

- [ ] **Step 4: Run to verify pass** — `go test ./internal/daemon` → PASS.

- [ ] **Step 5: Implement `cmd/tokensmithd/main.go`** (thin; no unit test — logic is in the tested packages)

```go
// Command tokensmithd is the background token-harvest daemon.
package main

import (
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"tokensmith/internal/daemon"
	"tokensmith/internal/ledger"
)

func main() {
	home, _ := os.UserHomeDir()
	claude := filepath.Join(home, ".claude", "projects")
	codex := filepath.Join(home, ".codex", "sessions")
	lp := ledger.DefaultPath()

	if err := os.MkdirAll(filepath.Dir(lp), 0o755); err != nil {
		log.Fatal(err)
	}
	release, err := daemon.AcquireLock(lp + ".lock")
	if err != nil {
		log.Fatal(err)
	}
	defer release()

	h := daemon.New(claude, codex, lp)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("tokensmithd harvesting → %s", lp)
	_ = h.Step(time.Now().Unix())
	for {
		select {
		case <-ticker.C:
			if err := h.Step(time.Now().Unix()); err != nil {
				log.Printf("step error: %v", err)
			}
		case <-stop:
			log.Print("shutting down")
			return
		}
	}
}
```

- [ ] **Step 6: Verify build** — `go build ./...` → OK. Optionally `go run ./cmd/tokensmithd &` then check `ledger.json` appears; kill it.

- [ ] **Step 7: Commit** — `feat(daemon): single-instance lock + tokensmithd binary`.

---

## Task 5: TUI sidecar meta

**Files:** Create `internal/store/meta.go`, `internal/store/meta_test.go`.

**Interfaces:**
- `type Meta struct { ConsumedIn, ConsumedOut int; LastRealUnix int64 }`
- `func SaveMeta(path string, m Meta) error`, `func LoadMeta(path string) (Meta, bool, error)`
- `func DefaultMetaPath() string` — `UserConfigDir/tokensmith/meta.json`.

- [ ] **Step 1: Write failing test** — round-trip like `store_test.go` (missing → ok=false; save/load preserves fields).

```go
func TestMetaRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "meta.json")
	if _, ok, _ := LoadMeta(p); ok {
		t.Fatal("missing meta should be ok=false")
	}
	if err := SaveMeta(p, Meta{ConsumedIn: 10, ConsumedOut: 5, LastRealUnix: 42}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadMeta(p)
	if err != nil || !ok || got.ConsumedIn != 10 || got.LastRealUnix != 42 {
		t.Fatalf("meta round-trip: %+v ok=%v err=%v", got, ok, err)
	}
}
```

- [ ] **Step 2-4:** Fail → implement (mirror `store.go` Save/Load with the `Meta` struct) → pass.

- [ ] **Step 5: Commit** — `feat(store): sidecar meta for consumed watermark + last-play time`.

---

## Task 6: Offline settlement

**Files:** Create `internal/tui/settle.go`, `internal/tui/settle_test.go`.

**Interfaces:**
- `type Summary struct { RnDGained float64; SecondsSettled float64; TrainingCompleted bool; TokensIn, TokensOut int }`
- `func Settle(s model.GameState, b balance.Config, elapsedSec float64, offIn, offOut int) (model.GameState, Summary)` — chunk `elapsedSec` into ≤3600s Ticks; distribute the offline tokens evenly across chunks (each chunk gets its share as one aggregate `TokenEvent`); clamp `elapsedSec` to `[0, 7*86400]`.

- [ ] **Step 1: Write failing test**

```go
package tui

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestSettleGrantsOfflineRnDAndAdvances(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Research.EfficiencyMult = 1
	before := s.Resources.RnD
	// 2 hours offline with a token batch
	ns, sum := Settle(s, b, 2*3600, 100000, 50000)
	if sum.RnDGained <= 0 || ns.Resources.RnD <= before {
		t.Fatalf("offline settlement granted no R&D: %+v", sum)
	}
	if ns.GameTime < 2*3600-1 {
		t.Fatalf("world did not advance: GameTime=%v", ns.GameTime)
	}
}

func TestSettleCompletesTraining(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Compute.TrainingCapacity = 1
	s.HasTraining = true
	s.Training = model.TrainingJob{Gen: 1, Alloc: [4]float64{0.4, 0.2, 0.2, 0.2}, Price: 12, WorkRemaining: 1800}
	ns, sum := Settle(s, b, 4*3600, 0, 0) // plenty of GPU-seconds
	if !sum.TrainingCompleted || ns.HasTraining {
		t.Fatalf("training should complete offline: %+v", sum)
	}
}

func TestSettleClampsElapsed(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	_, sum := Settle(s, b, -100, 0, 0)
	if sum.SecondsSettled != 0 {
		t.Fatalf("negative elapsed should clamp to 0, got %v", sum.SecondsSettled)
	}
}
```

- [ ] **Step 2: Run to verify fail** — FAIL.

- [ ] **Step 3: Implement**

```go
package tui

import (
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

type Summary struct {
	RnDGained         float64
	SecondsSettled    float64
	TrainingCompleted bool
	TokensIn, TokensOut int
}

const (
	settleChunkSec = 3600.0
	settleMaxSec   = 7 * 86400.0
)

func Settle(s model.GameState, b balance.Config, elapsedSec float64, offIn, offOut int) (model.GameState, Summary) {
	if elapsedSec < 0 {
		elapsedSec = 0
	}
	if elapsedSec > settleMaxSec {
		elapsedSec = settleMaxSec
	}
	sum := Summary{SecondsSettled: elapsedSec, TokensIn: offIn, TokensOut: offOut}
	beforeRnD := s.Resources.RnD
	wasTraining := s.HasTraining

	// number of chunks (at least 1 if there are tokens to apply)
	chunks := int(elapsedSec / settleChunkSec)
	if float64(chunks)*settleChunkSec < elapsedSec {
		chunks++
	}
	if chunks == 0 && (offIn > 0 || offOut > 0) {
		chunks = 1
	}
	remaining := elapsedSec
	for i := 0; i < chunks; i++ {
		dt := settleChunkSec
		if remaining < dt {
			dt = remaining
		}
		remaining -= dt
		// distribute tokens evenly across chunks
		var ev []model.TokenEvent
		if offIn > 0 || offOut > 0 {
			ci := offIn / chunks
			co := offOut / chunks
			if i == chunks-1 { // last chunk absorbs the remainder
				ci = offIn - ci*(chunks-1)
				co = offOut - co*(chunks-1)
			}
			ev = []model.TokenEvent{{InputTokens: ci, OutputTokens: co}}
		}
		s = sim.Tick(s, dt, ev, b)
	}
	sum.RnDGained = s.Resources.RnD - beforeRnD
	sum.TrainingCompleted = wasTraining && !s.HasTraining
	return s, sum
}
```

- [ ] **Step 4: Run to verify pass** — `go test ./internal/tui -run TestSettle` → PASS; `go test ./...` green.

- [ ] **Step 5: Commit** — `feat(tui): offline settlement via chunked ticks`.

---

## Task 7: TUI daemon integration

**Files:** Modify `internal/tui/tui.go`; Test `internal/tui/daemon_integration_test.go`.

**Interfaces / behaviour:**
- `Model` gains: `ledgerPath, metaPath string`; `daemonMode bool`; `consumedIn, consumedOut int`; `offlineSummary *Summary`.
- `newAt(savePath)` also derives ledger/meta paths (test override via a helper `newAtPaths(save, ledger, meta)`), loads meta.
- On `Init` (real run): if a fresh ledger exists → `daemonMode=true`, run offline settlement from `elapsed = now - meta.LastRealUnix` and `offline = ledger.cum - consumed`, apply, set `consumed = ledger.cum`, stash `offlineSummary`; else prime the built-in poller (fallback). First-ever open (no meta) sets `consumed = ledger.cum`, no settlement.
- On `tickMsg`: if `daemonMode`, read the ledger, `delta = cum - consumed`, feed one aggregate `TokenEvent`, set `consumed = cum`; else use the built-in poller as today.
- On save (autosave + quit): also `store.SaveMeta(metaPath, Meta{consumedIn, consumedOut, time.Now().Unix()})`.
- `View`: if `offlineSummary != nil`, prepend a one-line banner (`💤 離開 %.0fh，寫了 %d tokens → +%s R&D%s`) until dismissed by any key.

Freshness helper: `func ledgerFresh(l ledger.Ledger, now int64) bool { return now-l.UpdatedAt <= 30 }`.

- [ ] **Step 1: Write failing test** (hermetic; drives Update directly, no real clock dependence by injecting via fields)

```go
package tui

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"tokensmith/internal/ledger"
	"tokensmith/internal/store"
)

func TestTickConsumesLedgerDelta(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, "ledger.json")
	mp := filepath.Join(dir, "meta.json")
	// daemon already harvested some tokens
	ledger.Save(lp, ledger.Ledger{CumIn: 1000, CumOut: 500, UpdatedAt: 9_000_000_000})
	store.SaveMeta(mp, store.Meta{ConsumedIn: 0, ConsumedOut: 0, LastRealUnix: 9_000_000_000})

	m := newAtPaths(filepath.Join(dir, "s.json"), lp, mp)
	m.daemonMode = true
	before := m.state.Resources.RnD
	nm, _ := m.Update(tickMsg{})
	got := nm.(Model)
	if got.state.Resources.RnD <= before {
		t.Fatalf("tick did not apply ledger delta as R&D")
	}
	if got.consumedIn != 1000 {
		t.Fatalf("consumed watermark not advanced: %d", got.consumedIn)
	}
}
```

(Adjust `tickMsg{}` construction to the existing type; the test asserts ledger-delta consumption, not wall-clock.)

- [ ] **Step 2: Run to verify fail** — FAIL (undefined newAtPaths/daemonMode/consumedIn).

- [ ] **Step 3: Implement** the fields, `newAtPaths`, the tick branch (daemon vs poller), settlement on Init, and meta persistence on save. Keep the existing poller path intact for `daemonMode == false`.

- [ ] **Step 4: Run to verify pass** — targeted + `go test ./...` green; `go vet ./...`; `go build ./...`.

- [ ] **Step 5: Commit** — `feat(tui): consume harvest ledger + offline settlement banner`.

---

## Self-Review

- **Spec coverage (§10.2):** daemon harvest loop=Task 3/4, durable cursors=Task 2, ledger=Task 1, single-instance lock=Task 4, sidecar meta=Task 5, unified consume + offline settlement + fallback=Task 6/7. Desktop notifications / auto-start / multi-client explicitly out of v1 (noted).
- **Type consistency:** `ledger.Ledger` fields (CumIn/CumOut/UpdatedAt/Cursors) used identically in Tasks 1/3/7; `ingest.CursorState` (Path/Inode/Offset) in Tasks 1/2/3; `store.Meta` (ConsumedIn/ConsumedOut/LastRealUnix) in Tasks 5/7; `Summary` in Tasks 6/7.
- **Non-disruptive:** every new field defaults to a neutral zero; `daemonMode=false` preserves today's built-in-poller behaviour so all existing TUI tests pass; sim untouched (settlement is TUI-side).
- **Purity:** clocks confined to `cmd/tokensmithd`, `internal/daemon` (injected `now`), and `internal/tui`. `internal/sim`, `internal/ledger`, `internal/store` take no wall-clock.
- **Placeholders:** none — every step carries real code or a concrete round-trip spec.
- **Deferred (logged):** notifications, daemon auto-start, per-tick daemon re-detection (mode fixed at Init), multi-day soft-cap window fidelity during settlement.

## Execution Handoff

Plan saved to `docs/superpowers/plans/2026-07-08-tokensmith-16-harvest-daemon.md`. Executing inline task-by-task with a test/vet/build gate and a commit per task (same rhythm as the six-page TUI).
