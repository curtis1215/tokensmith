# Token→R&D 手感重製 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make real Claude Code / Codex token usage the dominant, visibly-felt driver of in-game R&D — fixing the sim-time unit mismatch that let passive researcher/star income drown it out, and rebuilding the live feedback (per-source R&D amount, streak bonus) that told the player it was happening.

**Architecture:** `balance.Config` gains a `RealSecCompression`-derived scale on passive R&D rates and a per-tick `StreakMult` field (no new parameters on the pure `sim.Tick`). `ledger.Ledger` and `store.Meta` move from flat `CumIn/CumOut` ints to a `map[string]model.SourceTotals` keyed by token source, so the daemon→TUI pipeline can attribute R&D to "Claude Code" vs "Codex". The TUI computes a real-wall-clock coding streak and threads it into a per-tick copy of `Config`, then renders per-source R&D deltas and the streak badge in the status bar for the existing pulse duration (extended).

**Tech Stack:** Go 1.25, `charmbracelet/bubbletea` (TUI), stdlib `encoding/json` (ledger/meta persistence), `go test` (all verification — no new test framework).

## Global Constraints

- Module path is `tokensmith` (see `go.mod`); all imports below are `tokensmith/internal/...`.
- `internal/model` has zero internal dependencies — any new shared type goes there, never introduce a dependency edge INTO `model`.
- `internal/balance` must not import `internal/tui` (would create an import cycle back through `tui`→`balance`). Any check that needs both lives in `internal/tui`'s own test files (which already import `balance`).
- `sim.Tick`'s signature (`Tick(s model.GameState, dt float64, events []model.TokenEvent, b balance.Config) model.GameState`) must NOT change — ~70 existing calls across `internal/sim/*_test.go` depend on it and are unrelated to this feature.
- Every numeric balance constant this plan touches must be a derivable scaling of the old constant (documented divisor), never a bare rewritten magic number.
- Run `go build ./...` and `go test ./...` at the end of every task; both must be clean before moving to the next task.
- Commit messages follow this repo's existing convention: `type(scope): summary` (see recent `git log`), body optional.

---

## Task 1: Rebalance passive R&D + thread streak multiplier through `sim.Tick`

**Files:**
- Modify: `internal/balance/balance.go`
- Modify: `internal/balance/balance_test.go`
- Modify: `internal/sim/sim.go`
- Modify: `internal/sim/sim_test.go`
- Modify: `internal/sim/view_test.go`
- Modify: `internal/tui/tui.go` (only the `human()` helper)
- Modify: `internal/tui/pressure_test.go`
- Modify: `internal/tui/tui_test.go`

**Interfaces:**
- Produces: `balance.RealSecCompression` (exported `float64` const, value `14400.0`); `balance.Config.StreakMult` (new exported field, `Default()` seeds it to `1.0`); `sim.TokenRawRnD(events []model.TokenEvent, b balance.Config) float64` (renamed/exported from the former private `tokenRawRnD`, identical behavior).
- Consumes: nothing new from other tasks (this task is self-contained).

- [ ] **Step 1: Update all affected tests to the new expected numbers (red)**

Edit `internal/balance/balance_test.go` — replace the three researcher assertions and add a `StreakMult` check:

```go
func TestDefaultV0Values(t *testing.T) {
	c := Default()
	if c.ResearcherRnDPerSec[model.Tier1] != 0.005/RealSecCompression {
		t.Errorf("Tier1 R&D/s = %v, want %v", c.ResearcherRnDPerSec[model.Tier1], 0.005/RealSecCompression)
	}
	if c.ResearcherRnDPerSec[model.Tier2] != 0.015/RealSecCompression {
		t.Errorf("Tier2 R&D/s = %v, want %v", c.ResearcherRnDPerSec[model.Tier2], 0.015/RealSecCompression)
	}
	if c.ResearcherRnDPerSec[model.Tier3] != 0.04/RealSecCompression {
		t.Errorf("Tier3 R&D/s = %v, want %v", c.ResearcherRnDPerSec[model.Tier3], 0.04/RealSecCompression)
	}
	if c.TokenInputWeight != 1 || c.TokenOutputWeight != 2 || c.TokenDivisor != 10 {
		t.Errorf("token formula params wrong: %+v", c)
	}
	if c.StreakMult != 1.0 {
		t.Errorf("StreakMult default = %v, want 1.0 (neutral)", c.StreakMult)
	}
	if c.SoftCapFull != 200000 || c.SoftCapMult != 0.3 || c.SoftCapWindowSec != 86400 {
		t.Errorf("soft cap params wrong: %+v", c)
	}
```

(Leave the rest of `TestDefaultV0Values` after this point unchanged — only the shown lines change.)

In the same file, update the star assertion inside `TestDefaultStars`:

```go
	if s, ok := byID["aria-chen"]; !ok || s.Effects.QualityMult[model.DimCapability] != 1.22 || s.Effects.RnDPerSec != 300/RealSecCompression {
		t.Errorf("aria-chen wrong: %+v ok=%v", s, ok)
	}
```

Edit `internal/sim/sim_test.go` — update the three staff-R&D absolute-value tests (they scale linearly, so each new expected value is the old value divided by `balance.RealSecCompression`):

```go
func TestStaffRnDPerSec(t *testing.T) {
	b := balance.Default()
	r := model.Research{EfficiencyMult: 1.0}
	r.Researchers[model.Tier1] = 2 // 2*0.005 = 0.01 (pre-compression-fix units)
	r.Researchers[model.Tier2] = 1 // 1*0.015 = 0.015
	got := staffRnDPerSec(r, b)    // 0.025/s, scaled by RealSecCompression
	want := 0.025 / balance.RealSecCompression
	if !approx(got, want) {
		t.Fatalf("staffRnDPerSec = %v, want %v", got, want)
	}
}

func TestStaffRnDEfficiencyMult(t *testing.T) {
	b := balance.Default()
	r := model.Research{EfficiencyMult: 2.0}
	r.Researchers[model.Tier2] = 1 // 0.015 * 2.0 = 0.03, scaled by RealSecCompression
	want := 0.03 / balance.RealSecCompression
	if got := staffRnDPerSec(r, b); !approx(got, want) {
		t.Fatalf("staffRnDPerSec with mult = %v, want %v", got, want)
	}
}

func TestTickAddsStaffRnDAndAdvancesTime(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Research: model.Research{EfficiencyMult: 1.0}}
	s.Research.Researchers[model.Tier2] = 4 // 0.06/s pre-compression-fix
	ns := Tick(s, 10, nil, b)               // 0.06/s * 10s = 0.6, scaled by RealSecCompression
	want := 0.6 / balance.RealSecCompression
	if !approx(ns.Resources.RnD, want) {
		t.Fatalf("RnD = %v, want %v", ns.Resources.RnD, want)
	}
	if !approx(ns.GameTime, 10) {
		t.Fatalf("GameTime = %v, want 10", ns.GameTime)
	}
	// Tick must not mutate the input state.
	if s.Resources.RnD != 0 || s.GameTime != 0 {
		t.Fatalf("Tick mutated input: %+v", s)
	}
}
```

Still in `sim_test.go`, rename the two calls to the (about-to-be-exported) token formula:

```go
func TestTokenRawRnD(t *testing.T) {
	b := balance.Default()
	events := []model.TokenEvent{
		{InputTokens: 1000, OutputTokens: 500}, // (1000 + 2*500)/10 = 200
		{InputTokens: 0, OutputTokens: 1000},   // (0 + 2000)/10   = 200
	}
	if got := TokenRawRnD(events, b); !approx(got, 400) {
		t.Fatalf("TokenRawRnD = %v, want 400", got)
	}
}

func TestTokenRawRnDEmpty(t *testing.T) {
	if got := TokenRawRnD(nil, balance.Default()); got != 0 {
		t.Fatalf("TokenRawRnD(nil) = %v, want 0", got)
	}
}
```

Update the fast-forward-equivalence test's expected constant (it also scales by `1/RealSecCompression`):

```go
	if !approx(oneShot.Resources.RnD, 14.25/balance.RealSecCompression) { // (3*0.005 + 2*0.04)*1.5 = 0.1425/s * 100s = 14.25, scaled
		t.Fatalf("expected RnD %v, got %v", 14.25/balance.RealSecCompression, oneShot.Resources.RnD)
	}
```

Add a new test to `sim_test.go` (anywhere after `TestTickAddsTokenRnD`) proving `StreakMult` only scales the token term, not staff/star R&D:

```go
func TestTickStreakMultOnlyAffectsTokenRnD(t *testing.T) {
	b := balance.Default()
	b.StreakMult = 2.0
	staffOnly := model.GameState{Research: model.Research{EfficiencyMult: 1.0}}
	staffOnly.Research.Researchers[model.Tier2] = 4
	base := balance.Default() // StreakMult = 1.0 (neutral)
	nsStreak := Tick(staffOnly, 10, nil, b)
	nsBase := Tick(staffOnly, 10, nil, base)
	if !approx(nsStreak.Resources.RnD, nsBase.Resources.RnD) {
		t.Fatalf("StreakMult must not affect staff-only R&D: streak=%v base=%v", nsStreak.Resources.RnD, nsBase.Resources.RnD)
	}

	tokenOnly := model.GameState{}
	events := []model.TokenEvent{{OutputTokens: 1000}} // raw 200
	nsTokenStreak := Tick(tokenOnly, 1, events, b)
	nsTokenBase := Tick(tokenOnly, 1, events, base)
	if !approx(nsTokenStreak.Resources.RnD, 2*nsTokenBase.Resources.RnD) {
		t.Fatalf("StreakMult=2.0 should double token R&D: got %v, want %v", nsTokenStreak.Resources.RnD, 2*nsTokenBase.Resources.RnD)
	}
}
```

Edit `internal/sim/view_test.go`:

```go
func TestRnDRatePerSec(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Research.EfficiencyMult = 1
	s.Research.Researchers[model.Tier1] = 2 // 2 × (0.005/RealSecCompression)/s
	want := 2 * 0.005 / balance.RealSecCompression
	if RnDRatePerSec(s, b) != want {
		t.Errorf("RnDRatePerSec = %v, want %v", RnDRatePerSec(s, b), want)
	}
}
```

Edit `internal/tui/pressure_test.go` — the resource bar's per-real-second rate now correctly shows the *un-inflated* rate (the whole point of this fix: `2 researchers × (0.005/14400) × 14400 = 0.01`, no longer `144`). Replace `TestResourceBarShowsPerRealSecondRnDRate`:

```go
func TestResourceBarShowsPerRealSecondRnDRate(t *testing.T) {
	m := testModel(t) // fresh game seeds 2 T1 researchers
	bar := renderResourceBar(m)
	// 2 × (0.005/14400 game-sec) × 14400 game-sec/real-sec = 0.01/real-sec exactly —
	// small on purpose (root-cause fix: passive income no longer secretly
	// inherits the 14400x sim-time compression). human() shows sub-1 values to
	// 2dp so this doesn't misleadingly render as "+0/s".
	if !strings.Contains(bar, "+0.01/s") {
		t.Fatalf("expected the un-inflated per-real-second R&D rate:\n%s", bar)
	}
}
```

Add a new test to `internal/tui/tui_test.go` guarding the two independent derivations of the compression factor from drifting apart:

```go
func TestRealSecCompressionMatchesTickRate(t *testing.T) {
	want := tickDT * float64(time.Second) / float64(tickInterval)
	if balance.RealSecCompression != want {
		t.Fatalf("balance.RealSecCompression = %v, want %v (tui tickDT/tickInterval changed without updating balance.RealSecCompression)", balance.RealSecCompression, want)
	}
}
```

- [ ] **Step 2: Run the affected test suites and confirm they fail**

Run: `go test ./internal/balance/... ./internal/sim/... ./internal/tui/... 2>&1 | tail -60`
Expected: FAIL — `TestDefaultV0Values`, `TestDefaultStars`, `TestStaffRnDPerSec`, `TestStaffRnDEfficiencyMult`, `TestTickAddsStaffRnDAndAdvancesTime`, the fast-forward test, `TestTokenRawRnD`/`TestTokenRawRnDEmpty` (compile error: `TokenRawRnD` undefined), `TestTickStreakMultOnlyAffectsTokenRnD` (compile error: `b.StreakMult` undefined field), `TestRnDRatePerSec`, `TestResourceBarShowsPerRealSecondRnDRate`, and `TestRealSecCompressionMatchesTickRate` (compile error: `balance.RealSecCompression` undefined).

- [ ] **Step 3: Implement the balance, sim, and display changes**

Edit `internal/balance/balance.go` — add the exported compression constant right after `EntryProcessID`:

```go
// EntryProcessID is the process available from the first day (no tech unlock).
const EntryProcessID = "N7"

// RealSecCompression is how many simulated seconds the TUI advances per real
// second: tickDT(3600) × 4 ticks/sec at a 250ms tick interval
// (internal/tui/tui.go). Balance numbers meant to represent "per real second"
// production (researcher and star R&D rates) divide by this so they aren't
// silently inflated by the sim-time compression. internal/tui has a test
// (TestRealSecCompressionMatchesTickRate) asserting this constant tracks
// tui's own tickDT/tickInterval derivation.
const RealSecCompression = 14400.0
```

In `Config`, add the streak field right after `TokenDivisor`:

```go
	// Token → R&D: (input*InputWeight + output*OutputWeight) / Divisor.
	TokenInputWeight  float64
	TokenOutputWeight float64
	TokenDivisor      float64

	// StreakMult multiplies token-sourced R&D only (never staff/star R&D). Set
	// per tick by the TUI from the real-world coding-streak bonus; Default()
	// seeds it to 1.0 (neutral) so every caller that never touches it keeps
	// today's behavior unchanged.
	StreakMult float64
```

In `Default()`, change the three researcher lines and add the streak default:

```go
	// R&D per researcher-second. Kept low so the tech tree is a real time-gate
	// (not trivially affordable) and real coding (token R&D) stays impactful.
	c.ResearcherRnDPerSec[model.Tier1] = 0.005 / RealSecCompression
	c.ResearcherRnDPerSec[model.Tier2] = 0.015 / RealSecCompression
	c.ResearcherRnDPerSec[model.Tier3] = 0.04 / RealSecCompression

	c.TokenInputWeight = 1
	c.TokenOutputWeight = 2
	c.TokenDivisor = 10
	c.StreakMult = 1.0
```

In `DefaultStars()`, change the two star `RnDPerSec` assignments:

```go
		star("aria-chen", "Dr. Aria Chen", 600000, 0.02, func(e *model.StarEffects) {
			e.QualityMult[model.DimCapability] = 1.22
			e.RnDPerSec = 300 / RealSecCompression
		}),
		star("nova", "Nova", 1000000, 0.03, func(e *model.StarEffects) {
			for d := range e.QualityMult {
				e.QualityMult[d] = 1.10
			}
			e.RnDPerSec = 400 / RealSecCompression
		}),
```

Edit `internal/sim/sim.go` — export and rename the token formula function (its doc comment and definition):

```go
// TokenRawRnD returns the raw R&D produced by a batch of token events, before
// any soft-cap diminishing is applied. Exported so the TUI display layer can
// compute the same per-source amount it's about to book (avoids the display
// and the actual booked value drifting apart).
func TokenRawRnD(events []model.TokenEvent, b balance.Config) float64 {
	var raw float64
	for _, e := range events {
		raw += (float64(e.InputTokens)*b.TokenInputWeight + float64(e.OutputTokens)*b.TokenOutputWeight) / b.TokenDivisor
	}
	return raw
}
```

In `Tick()`, update the one call site and the R&D accumulation line:

```go
	staffRnD := staffRnDPerSec(s.Research, b) * dt

	raw := TokenRawRnD(events, b)
	tokenRnD, newWindow := applySoftCap(ns.WindowRnD, raw, b.SoftCapFull, b.SoftCapMult)
	ns.WindowRnD = newWindow

	pe := prestigeEffects(ns.Prestige.UnlockedPrestige, b)
	starRnD := starEffects(ns, b).RnDPerSec * dt
	ns.Resources.RnD += (staffRnD+starRnD)*pe.RnDMult + tokenRnD*b.StreakMult*pe.RnDMult
```

Edit `internal/tui/tui.go` — give `human()` a branch for small nonzero values so they don't misleadingly round to "0":

```go
// human formats large numbers compactly (e.g. 1.84M, 340k).
func human(v float64) string {
	switch {
	case v >= 1e9:
		return fmt.Sprintf("%.2fB", v/1e9)
	case v >= 1e6:
		return fmt.Sprintf("%.2fM", v/1e6)
	case v >= 1e3:
		return fmt.Sprintf("%.0fk", v/1e3)
	case v > 0 && v < 1:
		return fmt.Sprintf("%.2f", v)
	default:
		return fmt.Sprintf("%.0f", v)
	}
}
```

- [ ] **Step 4: Run the full test suite and confirm it passes**

Run: `go build ./... && go test ./... 2>&1 | tail -60`
Expected: `ok` for every package, no failures.

- [ ] **Step 5: Commit**

```bash
git add internal/balance/balance.go internal/balance/balance_test.go internal/sim/sim.go internal/sim/sim_test.go internal/sim/view_test.go internal/tui/tui.go internal/tui/pressure_test.go internal/tui/tui_test.go
git commit -m "$(cat <<'EOF'
fix(balance): root-cause the researcher/token R&D unit mismatch

Passive researcher and star R&D rates were denominated per sim-second
but multiplied by dt every tick, silently inheriting the 14400x
sim-time compression that token-sourced R&D never got. Divide the
passive rates by that same compression factor (balance.RealSecCompression)
so token usage is the dominant R&D source again, and thread a
per-tick StreakMult (currently neutral 1.0) through Config so a later
task can apply a real-usage streak bonus without changing sim.Tick's
signature.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Per-source token ledger + daemon accumulation

**Files:**
- Modify: `internal/model/types.go`
- Modify: `internal/ledger/ledger.go`
- Modify: `internal/ledger/ledger_test.go`
- Modify: `internal/daemon/daemon.go`
- Modify: `internal/daemon/daemon_test.go`

**Interfaces:**
- Produces: `model.SourceTotals{In, Out int}`; `ledger.Ledger.Sources map[string]model.SourceTotals` (replaces `CumIn`/`CumOut`); `ledger.Ledger.TotalIn() int` / `TotalOut() int` (sum across sources).
- Consumes: nothing from Task 1 (independent).

- [ ] **Step 1: Write the new/updated tests (red)**

Edit `internal/ledger/ledger_test.go` — replace the whole file:

```go
package ledger

import (
	"os"
	"path/filepath"
	"testing"

	"tokensmith/internal/ingest"
	"tokensmith/internal/model"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "ledger.json")
	if _, ok, _ := Load(p); ok {
		t.Fatal("missing ledger should report ok=false")
	}
	in := Ledger{
		Sources: map[string]model.SourceTotals{
			"claude-code": {In: 100, Out: 50},
			"codex":       {In: 20, Out: 10},
		},
		UpdatedAt: 1700000000,
		Cursors:   []ingest.CursorState{{Path: "/x.jsonl", Inode: 7, Offset: 42}},
	}
	if err := Save(p, in); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Load(p)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.Sources["claude-code"] != (model.SourceTotals{In: 100, Out: 50}) {
		t.Fatalf("claude-code totals not round-tripped: %+v", got.Sources["claude-code"])
	}
	if got.Sources["codex"] != (model.SourceTotals{In: 20, Out: 10}) {
		t.Fatalf("codex totals not round-tripped: %+v", got.Sources["codex"])
	}
	if got.UpdatedAt != 1700000000 {
		t.Fatalf("UpdatedAt not round-tripped: %v", got.UpdatedAt)
	}
	if len(got.Cursors) != 1 || got.Cursors[0].Offset != 42 {
		t.Fatalf("cursors not round-tripped: %+v", got.Cursors)
	}
	if got.TotalIn() != 120 || got.TotalOut() != 60 {
		t.Fatalf("TotalIn/TotalOut = %d/%d, want 120/60", got.TotalIn(), got.TotalOut())
	}
}

func TestLoadOldSchemaIgnoresLegacyFields(t *testing.T) {
	p := filepath.Join(t.TempDir(), "ledger.json")
	// Pre-migration ledger.json shape (flat cumIn/cumOut, no "sources" key).
	legacy := `{"cumIn":100,"cumOut":50,"updatedAt":123}`
	if err := os.WriteFile(p, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Load(p)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.UpdatedAt != 123 {
		t.Fatalf("UpdatedAt should still parse: %v", got.UpdatedAt)
	}
	if got.TotalIn() != 0 || got.TotalOut() != 0 {
		t.Fatalf("legacy cumIn/cumOut must not leak into Sources: %+v", got.Sources)
	}
}
```

Edit `internal/daemon/daemon_test.go` — replace `l.CumIn`/`l.CumOut` with `l.TotalIn()`/`l.TotalOut()` throughout, and add a source-splitting test. Replace the whole file:

```go
package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func writeCodexLine(t *testing.T, f string, in, out int) {
	line := fmt.Sprintf(`{"timestamp":"2026-07-07T10:59:19Z","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":%d,"output_tokens":%d}}}}`+"\n", in, out)
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
	if l := h.Ledger(); l.TotalIn() != 100 || l.TotalOut() != 50 {
		t.Fatalf("after 1 line: %+v, want 100/50", l)
	}
	// second Step, no new data → unchanged
	h.Step(1001)
	if l := h.Ledger(); l.TotalIn() != 100 {
		t.Fatalf("no-new-data Step double counted: %+v", l)
	}
	// restart from persisted ledger → must not re-read the existing line
	h2 := New(claude, codex, lp)
	writeLine(t, f, "B")
	h2.Step(2000)
	if l := h2.Ledger(); l.TotalIn() != 200 || l.TotalOut() != 100 {
		t.Fatalf("after restart+1 line: %+v, want 200/100 (resumed, not re-read)", l)
	}

	got, ok, _ := ledger.Load(lp)
	if !ok || got.UpdatedAt != 2000 {
		t.Fatalf("ledger not persisted with UpdatedAt: %+v ok=%v", got, ok)
	}
}

func TestStepSplitsBySource(t *testing.T) {
	claude, codex := t.TempDir(), t.TempDir()
	lp := filepath.Join(t.TempDir(), "ledger.json")
	cf := filepath.Join(claude, "s.jsonl")
	xf := filepath.Join(codex, "s.jsonl")

	h := New(claude, codex, lp)
	writeLine(t, cf, "A")           // claude-code: in=100 out=50
	writeCodexLine(t, xf, 30, 15)   // codex: in=30 out=15
	if err := h.Step(1000); err != nil {
		t.Fatal(err)
	}
	l := h.Ledger()
	if l.Sources["claude-code"].In != 100 || l.Sources["claude-code"].Out != 50 {
		t.Fatalf("claude-code totals = %+v, want 100/50", l.Sources["claude-code"])
	}
	if l.Sources["codex"].In != 30 || l.Sources["codex"].Out != 15 {
		t.Fatalf("codex totals = %+v, want 30/15", l.Sources["codex"])
	}
	if l.TotalIn() != 130 || l.TotalOut() != 65 {
		t.Fatalf("combined totals = %d/%d, want 130/65", l.TotalIn(), l.TotalOut())
	}
}

func TestStepPrunesOldFileCursors(t *testing.T) {
	claude, codex := t.TempDir(), t.TempDir()
	lp := filepath.Join(t.TempDir(), "ledger.json")
	f := filepath.Join(claude, "s.jsonl")

	h := New(claude, codex, lp)
	writeLine(t, f, "A")
	now := time.Now().Unix()
	h.Step(now) // harvest A → cursor for f exists
	// age the file well past the retention window
	past := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(f, past, past); err != nil {
		t.Fatal(err)
	}
	h.Step(now)

	got, _, _ := ledger.Load(lp)
	if len(got.Cursors) != 0 {
		t.Fatalf("stale file cursor should be pruned, got %d cursors", len(got.Cursors))
	}
	if got.TotalIn() != 100 {
		t.Fatalf("pruning must not change totals: %+v", got)
	}
}

func TestPrunedFileNotRereadOnRestart(t *testing.T) {
	claude, codex := t.TempDir(), t.TempDir()
	lp := filepath.Join(t.TempDir(), "ledger.json")
	f := filepath.Join(claude, "s.jsonl")

	h := New(claude, codex, lp)
	writeLine(t, f, "A")
	now := time.Now().Unix()
	h.Step(now) // cumIn 100
	past := time.Now().Add(-30 * 24 * time.Hour)
	os.Chtimes(f, past, past)
	h.Step(now) // prune drops f's cursor from the ledger

	// Restart: f has no persisted cursor, but PrimeUnknown must prime it to EOF
	// rather than re-reading "A" (which would inflate the totals).
	h2 := New(claude, codex, lp)
	h2.Step(now)
	if l := h2.Ledger(); l.TotalIn() != 100 {
		t.Fatalf("pruned file re-read on restart (inflation): %+v", l)
	}
}

func TestHarvesterPrimesHistoryOnFirstStart(t *testing.T) {
	claude, codex := t.TempDir(), t.TempDir()
	lp := filepath.Join(t.TempDir(), "ledger.json")
	f := filepath.Join(claude, "s.jsonl")
	writeLine(t, f, "OLD") // history present before daemon ever ran

	h := New(claude, codex, lp) // fresh start → prime to EOF
	h.Step(1000)
	if l := h.Ledger(); l.TotalIn() != 0 {
		t.Fatalf("first start should skip pre-existing history, got %+v", l)
	}
	writeLine(t, f, "NEW")
	h.Step(1001)
	if l := h.Ledger(); l.TotalIn() != 100 {
		t.Fatalf("post-prime line should count, got %+v", l)
	}
}
```

- [ ] **Step 2: Run the tests and confirm they fail to compile/pass**

Run: `go test ./internal/ledger/... ./internal/daemon/... 2>&1 | tail -60`
Expected: compile failures (`Ledger.Sources`/`model.SourceTotals`/`TotalIn`/`TotalOut` undefined).

- [ ] **Step 3: Implement the model type, ledger schema, and daemon split**

Edit `internal/model/types.go` — insert right after the `TokenEvent` struct (after its closing `}`, before the `GameState` comment):

```go
// TokenEvent is a normalized real-world AI-tool usage event.
type TokenEvent struct {
	Source       string
	Timestamp    time.Time
	InputTokens  int
	OutputTokens int
	ID           string // stable dedup key (e.g. Claude message id); "" if none
}

// SourceTotals is a cumulative per-source token count, used by the ledger and
// the TUI's consumed-watermark to attribute harvested tokens to the tool that
// produced them (e.g. "claude-code" vs "codex").
type SourceTotals struct {
	In  int
	Out int
}

// GameState is the full simulation state (plan-01 subset).
```

Edit `internal/ledger/ledger.go` — replace the whole file:

```go
// Package ledger persists the daemon's cumulative token harvest so the TUI can
// consume it (online and offline) without re-reading raw logs.
package ledger

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"tokensmith/internal/ingest"
	"tokensmith/internal/model"
)

// Ledger is the monotonically-growing per-source harvest totals plus durable
// cursors. Sources is keyed by TokenEvent.Source ("claude-code" / "codex").
// Older ledger.json files used flat cumIn/cumOut ints instead of Sources —
// those legacy fields are simply absent from a freshly-loaded Ledger (zero
// value), which the daemon treats as "start counting from here."
type Ledger struct {
	Sources   map[string]model.SourceTotals `json:"sources"`
	UpdatedAt int64                         `json:"updatedAt"`
	Cursors   []ingest.CursorState          `json:"cursors,omitempty"`
}

// TotalIn sums InputTokens across every source.
func (l Ledger) TotalIn() int {
	var n int
	for _, s := range l.Sources {
		n += s.In
	}
	return n
}

// TotalOut sums OutputTokens across every source.
func (l Ledger) TotalOut() int {
	var n int
	for _, s := range l.Sources {
		n += s.Out
	}
	return n
}

// DefaultPath is the standard ledger location.
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "tokensmith", "ledger.json")
}

// Save writes the ledger atomically (temp file + rename).
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

// Load reads the ledger. ok is false when the file does not exist yet.
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

Edit `internal/daemon/daemon.go` — split accumulation by source in `Step()`:

```go
// Package daemon harvests real Claude/Codex token usage into the ledger.
package daemon

import (
	"os"

	"tokensmith/internal/ingest"
	"tokensmith/internal/ledger"
	"tokensmith/internal/model"
)

// cursorMaxAgeSec bounds which files' cursors are persisted: a log untouched
// for this long will not receive new tokens, so its cursor is dropped to keep
// the ledger small. On restart PrimeUnknown re-primes such files to EOF.
const cursorMaxAgeSec = 7 * 86400

// Harvester tails the logs and accumulates token totals into a durable ledger.
type Harvester struct {
	poller     *ingest.Poller
	ledgerPath string
	cur        ledger.Ledger
}

// New loads any existing ledger and resumes from its persisted cursors, then
// primes every other tracked file to EOF — so months of old logs (and files
// pruned from a previous ledger) are not re-read.
func New(claudeDir, codexDir, ledgerPath string) *Harvester {
	p := ingest.NewPoller(claudeDir, codexDir)
	cur, ok, _ := ledger.Load(ledgerPath)
	if ok {
		p.ImportCursors(cur.Cursors)
	}
	p.PrimeUnknown()
	return &Harvester{poller: p, ledgerPath: ledgerPath, cur: cur}
}

// Step polls for new usage, folds it into the cumulative per-source totals,
// and persists the ledger stamped with now (unix seconds), keeping only
// recently-active cursors so the file stays small.
func (h *Harvester) Step(now int64) error {
	for _, e := range h.poller.Poll() {
		if h.cur.Sources == nil {
			h.cur.Sources = map[string]model.SourceTotals{}
		}
		st := h.cur.Sources[e.Source]
		st.In += e.InputTokens
		st.Out += e.OutputTokens
		h.cur.Sources[e.Source] = st
	}
	h.cur.UpdatedAt = now
	h.cur.Cursors = pruneCursors(h.poller.ExportCursors(), now)
	return ledger.Save(h.ledgerPath, h.cur)
}

// pruneCursors keeps only cursors for files modified within cursorMaxAgeSec.
func pruneCursors(cs []ingest.CursorState, now int64) []ingest.CursorState {
	out := make([]ingest.CursorState, 0, len(cs))
	for _, c := range cs {
		if fi, err := os.Stat(c.Path); err == nil && now-fi.ModTime().Unix() <= cursorMaxAgeSec {
			out = append(out, c)
		}
	}
	return out
}

// Ledger returns the current in-memory ledger snapshot.
func (h *Harvester) Ledger() ledger.Ledger { return h.cur }
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go build ./... && go test ./internal/ledger/... ./internal/daemon/... -v 2>&1 | tail -80`
Expected: all `PASS`, including the new `TestStepSplitsBySource` and `TestLoadOldSchemaIgnoresLegacyFields`.

- [ ] **Step 5: Commit**

```bash
git add internal/model/types.go internal/ledger/ledger.go internal/ledger/ledger_test.go internal/daemon/daemon.go internal/daemon/daemon_test.go
git commit -m "$(cat <<'EOF'
feat(ledger): track harvested tokens per source

Ledger.CumIn/CumOut collapsed which tool (Claude Code vs Codex)
produced a given token into one flat total, so the TUI could never
show the player which tool was driving their R&D. Replace it with a
Sources map keyed by TokenEvent.Source, split the daemon's
accumulation accordingly, and add TotalIn/TotalOut helpers for
callers that still want the combined figure. Old ledger.json files
(flat cumIn/cumOut) load cleanly and just start counting fresh.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Per-source consumed watermark + coding-streak fields in `store.Meta`

**Files:**
- Modify: `internal/store/meta.go`
- Modify: `internal/store/meta_test.go`

**Interfaces:**
- Consumes: `model.SourceTotals` (from Task 2).
- Produces: `store.Meta.ConsumedSources map[string]model.SourceTotals` (replaces `ConsumedIn`/`ConsumedOut`); `store.Meta.LastActiveDate string`; `store.Meta.StreakDays int`.

- [ ] **Step 1: Update the test (red)**

Edit `internal/store/meta_test.go` — replace the whole file:

```go
package store

import (
	"path/filepath"
	"testing"

	"tokensmith/internal/model"
)

func TestMetaRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "meta.json")
	if _, ok, _ := LoadMeta(p); ok {
		t.Fatal("missing meta should be ok=false")
	}
	in := Meta{
		ConsumedSources: map[string]model.SourceTotals{
			"claude-code": {In: 10, Out: 5},
		},
		LastRealUnix:   42,
		LastActiveDate: "2026-07-10",
		StreakDays:     3,
	}
	if err := SaveMeta(p, in); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadMeta(p)
	if err != nil || !ok {
		t.Fatalf("meta round-trip: ok=%v err=%v", ok, err)
	}
	if got.ConsumedSources["claude-code"] != (model.SourceTotals{In: 10, Out: 5}) {
		t.Fatalf("ConsumedSources not round-tripped: %+v", got.ConsumedSources)
	}
	if got.LastRealUnix != 42 || got.LastActiveDate != "2026-07-10" || got.StreakDays != 3 {
		t.Fatalf("meta round-trip mismatch: %+v", got)
	}
}
```

- [ ] **Step 2: Run and confirm it fails to compile**

Run: `go test ./internal/store/... 2>&1 | tail -30`
Expected: compile error (`Meta.ConsumedSources`/`LastActiveDate`/`StreakDays` undefined).

- [ ] **Step 3: Implement the Meta schema change**

Edit `internal/store/meta.go` — replace the whole file:

```go
package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"tokensmith/internal/model"
)

// Meta is TUI-side runtime persistence kept beside the save: how much of the
// harvest ledger has been consumed (per source), the coding-streak counter,
// and the wall-clock time of the last play (used to settle offline
// progress). The sim itself never sees wall-clock.
type Meta struct {
	ConsumedSources map[string]model.SourceTotals `json:"consumedSources"`
	LastRealUnix    int64                          `json:"lastRealUnix"`
	// LastActiveDate is "YYYY-MM-DD" in local time — the last calendar day any
	// tokens were harvested. StreakDays counts consecutive such days; a
	// skipped day resets it to 1 on the next active day.
	LastActiveDate string `json:"lastActiveDate"`
	StreakDays     int    `json:"streakDays"`
}

// DefaultMetaPath is the standard meta-file location.
func DefaultMetaPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "tokensmith", "meta.json")
}

// SaveMeta writes the meta atomically (temp file + rename).
func SaveMeta(path string, m Meta) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadMeta reads the meta. ok is false when the file does not exist yet.
func LoadMeta(path string) (Meta, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Meta{}, false, nil
	}
	if err != nil {
		return Meta{}, false, err
	}
	var m Meta
	if err := json.Unmarshal(data, &m); err != nil {
		return Meta{}, false, err
	}
	return m, true, nil
}
```

- [ ] **Step 4: Run and confirm it passes**

Run: `go build ./... && go test ./internal/store/... -v 2>&1 | tail -30`
Expected: `PASS`.

- [ ] **Step 5: Commit**

```bash
git add internal/store/meta.go internal/store/meta_test.go
git commit -m "$(cat <<'EOF'
feat(store): persist per-source watermark and coding streak

Meta.ConsumedIn/ConsumedOut become ConsumedSources (mirrors the
ledger's per-source shape from the previous commit) so the TUI can
diff each tool's harvest independently. Add LastActiveDate/StreakDays
so a later task can compute a real-usage coding-streak bonus that
survives restarts.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Wire per-source token events and the coding streak into the TUI tick loop

**Files:**
- Modify: `internal/tui/tui.go`
- Modify: `internal/tui/daemon_integration_test.go`

**Interfaces:**
- Consumes: `ledger.Ledger.Sources`/`TotalIn`/`TotalOut` (Task 2); `store.Meta.ConsumedSources`/`LastActiveDate`/`StreakDays` (Task 3); `balance.Config.StreakMult` (Task 1); `sim.TokenRawRnD` (Task 1).
- Produces: `Model.consumed map[string]model.SourceTotals`; `Model.tokensThisTick bool`; `Model.lastTokenRnD map[string]float64` (sticky across the pulse-decay window — consumed by Task 5's display code); `Model.streakDays int`; `Model.lastActiveDate string`; `(m Model) currentStreakMult() float64`; `(m *Model) updateStreak(now time.Time)`.

- [ ] **Step 1: Rewrite the daemon-integration tests for the new shapes (red)**

Edit `internal/tui/daemon_integration_test.go` — replace the whole file:

```go
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
```

- [ ] **Step 2: Run and confirm it fails to compile**

Run: `go test ./internal/tui/... 2>&1 | tail -60`
Expected: compile errors (`m.consumed`, `m.tokensThisTick`, `m.lastTokenRnD`, `m.streakDays`, `m.updateStreak`, `m.currentStreakMult` all undefined).

- [ ] **Step 3: Implement the Model fields and tick-loop wiring**

Edit `internal/tui/tui.go` — replace the harvest-daemon-integration field block inside the `Model` struct:

```go
	// Harvest-daemon integration (§10.2).
	ledgerPath     string
	metaPath       string
	daemonMode     bool // consume the daemon ledger instead of the built-in poller
	consumed       map[string]model.SourceTotals // per-source ledger tokens already applied
	lastRealUnix   int64
	metaMissing    bool
	offlineSummary *Summary // shown as a banner until dismissed by any key
	// tokensThisTick is true only on the tick that just harvested new tokens
	// (drives the pulse restart). lastTokenRnD is the per-source R&D delta from
	// that tick, kept (not cleared) across the pulse-decay window so the status
	// bar can keep showing it while it fades — see internal/tui/display.go.
	tokensThisTick bool
	lastTokenRnD   map[string]float64
	// streakDays/lastActiveDate mirror store.Meta; kept in memory between saves
	// so every tick can read the current streak multiplier cheaply.
	streakDays     int
	lastActiveDate string
```

Replace `newAtPaths()`'s meta-derived field assignments:

```go
	meta, metaOK, _ := store.LoadMeta(metaPath)
	m := Model{
		state:          state,
		cfg:            balance.Default(),
		poller:         ingest.NewDefaultPoller(),
		savePath:       savePath,
		ledgerPath:     ledgerPath,
		metaPath:       metaPath,
		consumed:       meta.ConsumedSources,
		lastRealUnix:   meta.LastRealUnix,
		metaMissing:    !metaOK,
		streakDays:     meta.StreakDays,
		lastActiveDate: meta.LastActiveDate,
		width:          100,
		height:         40,
		vp:             viewport.New(80, 20),
	}
```

Replace `startup()`:

```go
// startup detects daemon mode and settles offline progress. Called by New()
// only, so unit-test constructors stay hermetic.
func (m Model) startup(now int64) Model {
	l, ok, _ := ledger.Load(m.ledgerPath)
	if !ok || !ledgerFresh(l, now) {
		return m // standalone: Init primes the built-in poller
	}
	m.daemonMode = true
	if m.metaMissing {
		// First-ever open: adopt the current total so we don't settle a phantom
		// window of everything harvested before the player ever played.
		m.consumed = copySourceTotals(l.Sources)
		return m
	}
	prevIn, prevOut := sumSourceTotals(m.consumed)
	offIn := l.TotalIn() - prevIn
	offOut := l.TotalOut() - prevOut
	elapsed := float64(now - m.lastRealUnix)
	if offIn > 0 || offOut > 0 {
		m.updateStreak(time.Unix(now, 0))
	}
	cfg := m.cfg
	cfg.StreakMult = m.currentStreakMult()
	ns, sum := Settle(m.state, cfg, elapsed, offIn, offOut)
	m.state = ns
	m.consumed = copySourceTotals(l.Sources)
	if sum.RnDGained > 0 || sum.TrainingCompleted || sum.EventsFired > 0 || sum.EventsAutoResolved > 0 {
		m.offlineSummary = &sum
	}
	return m
}

// copySourceTotals deep-copies a per-source totals map so callers don't alias
// a ledger snapshot that gets discarded.
func copySourceTotals(src map[string]model.SourceTotals) map[string]model.SourceTotals {
	out := make(map[string]model.SourceTotals, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// sumSourceTotals adds up every source's In/Out.
func sumSourceTotals(m map[string]model.SourceTotals) (in, out int) {
	for _, t := range m {
		in += t.In
		out += t.Out
	}
	return
}

// currentStreakMult returns the token-R&D multiplier for m.streakDays, capped
// at streakCapDays consecutive days.
func (m Model) currentStreakMult() float64 {
	days := m.streakDays
	if days > streakCapDays {
		days = streakCapDays
	}
	return 1 + streakBonusPerDay*float64(days)
}

// updateStreak advances the coding-streak counter from the real calendar
// date. Call only when this tick actually harvested tokens (an idle tick
// must not break or extend the streak).
func (m *Model) updateStreak(now time.Time) {
	today := now.Format("2006-01-02")
	if today == m.lastActiveDate {
		return
	}
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	if m.lastActiveDate == yesterday {
		m.streakDays++
	} else {
		m.streakDays = 1
	}
	m.lastActiveDate = today
}

const (
	streakCapDays     = 10   // multiplier stops growing past this many consecutive days
	streakBonusPerDay = 0.06 // +6%/day, so day 5 = ×1.3 and the day-10 cap = ×1.6
)
```

Replace `saveMeta()`:

```go
// saveMeta persists the consumed watermark, streak state, and the current
// wall-clock time.
func (m Model) saveMeta() {
	_ = store.SaveMeta(m.metaPath, store.Meta{
		ConsumedSources: m.consumed,
		LastRealUnix:    time.Now().Unix(),
		LastActiveDate:  m.lastActiveDate,
		StreakDays:      m.streakDays,
	})
}
```

Replace `pollTokens()`:

```go
// pollTokens returns the token events for this tick, either from the daemon
// ledger (advancing the consumed watermark) or the built-in poller.
func (m *Model) pollTokens() []model.TokenEvent {
	if !m.daemonMode {
		return m.poller.Poll()
	}
	l, ok, _ := ledger.Load(m.ledgerPath)
	if !ok {
		return nil
	}
	var events []model.TokenEvent
	for src, tot := range l.Sources {
		prev := m.consumed[src]
		di := tot.In - prev.In
		do := tot.Out - prev.Out
		if di <= 0 && do <= 0 {
			continue
		}
		events = append(events, model.TokenEvent{Source: src, InputTokens: di, OutputTokens: do})
	}
	if len(events) == 0 {
		return nil
	}
	m.consumed = copySourceTotals(l.Sources)
	return events
}
```

Replace the `case tickMsg:` branch inside `handleUpdate()`:

```go
	case tickMsg:
		now := time.Time(msg)
		events := m.pollTokens()
		m.tokensThisTick = len(events) > 0
		if m.tokensThisTick {
			m.updateStreak(now)
		}
		cfgTick := m.cfg
		cfgTick.StreakMult = m.currentStreakMult()
		if m.tokensThisTick {
			rnd := make(map[string]float64, len(events))
			for _, e := range events {
				rnd[e.Source] += sim.TokenRawRnD([]model.TokenEvent{e}, cfgTick) * cfgTick.StreakMult
			}
			m.lastTokenRnD = rnd
		}
		prevFired := m.state.Events.FiredCount
		m.state = sim.Tick(m.state, tickDT, events, cfgTick)
		if m.state.Events.FiredCount > prevFired {
			m.setNotice("📰 產業事件：" + latestEventName(m.state))
		}
		// Mechanism B: auto game-over + restart once debt passes the threshold.
		if m.state.Resources.Cash < -m.cfg.BankruptcyDebtRatio*m.cfg.StartingCash {
			m.state = sim.Restart(m.state, m.cfg)
			m.setNotice("💥 破產！公司已重整重來")
			m.snapDisplay()
		}
		m.advanceDisplay()
		m.ticksSinceSave++
		if m.ticksSinceSave >= 40 {
			m.ticksSinceSave = 0
			_ = store.Save(m.savePath, m.state)
			m.saveMeta()
		}
		return m, tick()
```

- [ ] **Step 4: Run and confirm it passes**

Run: `go build ./... && go test ./internal/tui/... -v 2>&1 | tail -100`
Expected: all `PASS`. (`internal/tui/display_test.go`'s `TestPulseTokenOnTokens` will still fail here — it references the now-removed `m.lastTokens` field — Task 5 fixes it. Confirm every OTHER test in the package passes and that this is the only remaining failure before proceeding.)

- [ ] **Step 5: Commit**

```bash
git add internal/tui/tui.go internal/tui/daemon_integration_test.go
git commit -m "$(cat <<'EOF'
feat(tui): wire per-source token events and streak bonus into sim

pollTokens() now diffs the ledger per source and emits one TokenEvent
per active tool instead of one flattened event, so the display layer
(next commit) can attribute R&D to Claude Code vs Codex. Add a
real-wall-clock coding-streak counter (updateStreak/currentStreakMult)
that feeds balance.Config.StreakMult per tick — capped at +60% for a
10-day streak, reset by any fully-skipped calendar day.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Live per-source R&D + streak badge in the status bar

**Files:**
- Modify: `internal/tui/display.go`
- Modify: `internal/tui/display_test.go`
- Modify: `internal/tui/tui.go` (`renderResourceBar`, `pressures` doc comment)

**Interfaces:**
- Consumes: `Model.tokensThisTick`, `Model.lastTokenRnD`, `Model.streakDays`, `(m Model) currentStreakMult()` (Task 4).
- Produces: nothing further downstream — this is the leaf/presentation task.

- [ ] **Step 1: Update display tests and add new rendering tests (red)**

Edit `internal/tui/display_test.go` — replace `TestPulseTokenOnTokens`:

```go
func TestPulseTokenOnTokens(t *testing.T) {
	m := testModel(t)
	m.dispReady = true
	m.disp.snap(truthDisplay(m))
	m.tokensThisTick = true
	m.advanceDisplay()
	if m.disp.PulseToken != tokenPulseTicks {
		t.Fatalf("PulseToken=%d want %d", m.disp.PulseToken, tokenPulseTicks)
	}
	m.tokensThisTick = false
	m.advanceDisplay()
	if m.disp.PulseToken != tokenPulseTicks-1 {
		t.Fatalf("PulseToken should decay to %d, got %d", tokenPulseTicks-1, m.disp.PulseToken)
	}
}
```

In the same file, add two new tests (anywhere after `TestPulseTokenOnTokens`):

```go
func TestRenderResourceBarShowsPerSourceRnD(t *testing.T) {
	m := testModel(t)
	m.lastTokenRnD = map[string]float64{"claude-code": 842, "codex": 15}
	m.disp.PulseToken = 5
	bar := renderResourceBar(m)
	if !strings.Contains(bar, "Claude Code +842 R&D") {
		t.Fatalf("expected Claude Code R&D segment, got:\n%s", bar)
	}
	if !strings.Contains(bar, "Codex +15 R&D") {
		t.Fatalf("expected Codex R&D segment, got:\n%s", bar)
	}
}

func TestRenderResourceBarShowsStreakBadge(t *testing.T) {
	m := testModel(t)
	m.lastTokenRnD = map[string]float64{"claude-code": 100}
	m.disp.PulseToken = 5
	m.streakDays = 3
	bar := renderResourceBar(m)
	if !strings.Contains(bar, "連續3天") || !strings.Contains(bar, "×1.18") {
		t.Fatalf("expected streak badge, got:\n%s", bar)
	}
}

func TestRenderResourceBarHidesTokensAfterPulseEnds(t *testing.T) {
	m := testModel(t)
	m.lastTokenRnD = map[string]float64{"claude-code": 100}
	m.disp.PulseToken = 0 // pulse has fully decayed
	bar := renderResourceBar(m)
	if strings.Contains(bar, "Claude Code") {
		t.Fatalf("token segment should be hidden once the pulse ends:\n%s", bar)
	}
}
```

`display_test.go` already imports `"strings"`? Confirm the import block includes it; if not, add it:

```go
import (
	"math"
	"strings"
	"testing"
)
```

- [ ] **Step 2: Run and confirm failures**

Run: `go test ./internal/tui/... -run 'TestPulseTokenOnTokens|TestRenderResourceBar' -v 2>&1 | tail -60`
Expected: `TestPulseTokenOnTokens` fails (wrong PulseToken values, and `tokenPulseTicks` undefined → compile error), the three new render tests fail (compile error: `m.lastTokenRnD` type mismatch is fine since Task 4 defined it, but the rendered bar won't yet contain "R&D"/streak text).

- [ ] **Step 3: Implement the pulse-duration and status-bar changes**

Edit `internal/tui/display.go` — add the duration constant near `displayAlpha` and use it in `advanceDisplay()`:

```go
// displayAlpha is the exponential approach factor per tick (α ≈ 0.3).
const displayAlpha = 0.3

// tokenPulseTicks is how many ticks the token flash stays lit (~3s at the
// 250ms tick interval) before fading — long enough to actually notice,
// unlike the old 4-tick (~1s) blink-and-miss window.
const tokenPulseTicks = 12
```

```go
// advanceDisplay updates displayState after a sim tick.
func (m *Model) advanceDisplay() {
	truth := truthDisplay(*m)
	if !m.dispReady {
		m.disp.snap(truth)
		m.dispReady = true
	} else {
		m.disp.approach(truth, displayAlpha)
	}
	if m.tokensThisTick {
		m.disp.PulseToken = tokenPulseTicks
	} else if m.disp.PulseToken > 0 {
		m.disp.PulseToken--
	}
	if m.disp.PulseNotice > 0 {
		m.disp.PulseNotice--
	}
}
```

Edit `internal/tui/tui.go` — replace the token-flash block in `renderResourceBar()`:

```go
	bar := fmt.Sprintf("%s   ⚡R&D %s   🖥訓練%.0f%% %s   📈估值 $%s",
		cashStr, rndSeg,
		trainUtil*100, infStr, human(val))

	if m.disp.PulseToken > 0 && len(m.lastTokenRnD) > 0 {
		parts := make([]string, 0, len(m.lastTokenRnD)+1)
		for _, src := range sourceKeysOrdered(m.lastTokenRnD) {
			parts = append(parts, fmt.Sprintf("⚡ %s +%s R&D", sourceLabel(src), human(m.lastTokenRnD[src])))
		}
		if m.streakDays > 0 {
			parts = append(parts, fmt.Sprintf("🔥連續%d天 ×%.2f", m.streakDays, m.currentStreakMult()))
		}
		bar += "   " + strings.Join(parts, "   ")
	}
	return bar
}

// knownSourceOrder fixes the display order of the two known token sources;
// any future/unknown source is appended after them in map-iteration order.
var knownSourceOrder = []string{"claude-code", "codex"}

// sourceKeysOrdered returns m's keys in a stable, deterministic order so the
// status bar doesn't reorder itself between renders.
func sourceKeysOrdered(m map[string]float64) []string {
	var out []string
	seen := make(map[string]bool, len(m))
	for _, k := range knownSourceOrder {
		if _, ok := m[k]; ok {
			out = append(out, k)
			seen[k] = true
		}
	}
	for k := range m {
		if !seen[k] {
			out = append(out, k)
		}
	}
	return out
}

// sourceLabel maps a TokenEvent.Source to its display name.
func sourceLabel(src string) string {
	switch src {
	case "claude-code":
		return "Claude Code"
	case "codex":
		return "Codex"
	default:
		return src
	}
}
```

Remove the old block that used to sit right after the `rndSeg` styling (the `if m.lastTokens > 0 { bar += fmt.Sprintf("   ⚡token +%d", m.lastTokens) }` lines) — it's fully replaced by the block above, do not leave both.

Still in `tui.go`, clean up the stale comment left over from before this feature existed:

```go
// pressures returns ⚠ attention items surfaced on the overview page.
func pressures(m Model) []string {
```

(was: `// pressures returns ⚠ attention items surfaced on the overview page. (A real coding-streak counter is deferred to a later plan.)` — the streak counter now exists, so the parenthetical is stale.)

- [ ] **Step 4: Run the full test suite and confirm everything passes**

Run: `go build ./... && go test ./... 2>&1 | tail -80`
Expected: `ok` for every package, zero failures.

- [ ] **Step 5: Manually verify in the running TUI**

This is an interactive smoke check, not something to automate — the TUI is a full-screen `bubbletea` program that needs a real terminal, and this game reads real Claude Code / Codex usage logs, so it can't be meaningfully faked in a headless run. Hand off to the user (or run it yourself in a foreground terminal, not backgrounded) with: "This task's automated tests are green. Please run `go run .` from the repo root in your own terminal (quit any already-running `tokensmith` process first, and make sure the `tokensmithd` background daemon is running so daemon mode kicks in) and confirm: (1) the status bar's `+X/s` R&D-rate segment no longer misleadingly shows `+0/s` on a fresh game — it should show a small decimal like `+0.01/s`; (2) using Claude Code or Codex in another terminal makes a `⚡ Claude Code +N R&D` (or `Codex`) segment appear in the status bar and linger for a few seconds before fading, instead of a single-frame blink; (3) after the same source contributes on two consecutive real calendar days, a `🔥連續2天 ×1.12` badge appears alongside it. Press `q` to quit when done." Only proceed to Step 6 once the user confirms.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/display.go internal/tui/display_test.go internal/tui/tui.go
git commit -m "$(cat <<'EOF'
feat(tui): show live per-source R&D and streak badge in status bar

Replace the old single-frame "⚡token +N" (raw token count, ~1s) with
a ~3s fade showing the actual R&D each source (Claude Code / Codex)
just added, plus a 🔥 streak badge when a coding streak is active —
closing the loop the rebalance opened: the player can now see the
real-usage R&D that's supposed to dominate.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```
