# Process-Node Compute + Early-Economy Rebalance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace flat GPU rental with a tech-tree-gated process-node catalog (N7→N5→N3→N2), rentable or buildable and freely allocated across the training/inference pools, and rebalance early economy so a starting model turns a profit.

**Architecture:** A unified `balance.Process` catalog drives both rent (opex, per-second) and self-build (capex). `Compute` tracks rented chips per process per pool as maps; effective compute and rent are per-process sums (order-independent → sim stays pure). Newer processes unlock via Infra-branch tech nodes. Two scalars rebalance economy: `RevenueMult` (×2 per-user revenue, lever A) and the cheap N7 node (lever B).

**Tech Stack:** Go 1.22+, Bubble Tea v1.3.10, Lipgloss v1.1.0. No new deps.

## Global Constraints

- Module `tokensmith`; Go 1.22+.
- `internal/sim` stays pure: no wall-clock/rand/IO; time only via `dt`. Map summation must be the only map use in the tick path (order-independent).
- Non-disruptive where possible, but this changes the `Compute` struct shape and the rent/build commands — every affected test is updated within its task so the suite stays green per task.
- Spec: `docs/superpowers/specs/2026-07-09-process-node-compute-design.md`.
- Ledger/daemon/store/ingest packages are untouched.

---

## File Structure

- `internal/balance/balance.go` (modify) — `Process` type, `DefaultProcesses()`, `ProcessByID()`, `EntryProcessID`; `Config.Processes/TrainRentMult/RevenueMult`; process-unlock tech nodes; remove `Chip`/`Chips`/`ChipsPerServer` legacy or repoint to processes.
- `internal/model/types.go` (modify) — `Compute` per-process maps; `RentCompute` command (replaces `RentTrainingCompute`/`RentInferenceCompute`); `BuildServer{Process}` (was `ChipName`); remove `Chip`.
- `internal/sim/apply.go` (modify) — `applyRentCompute`, `applyBuildServer` (process), `ErrProcessLocked`/`ErrInvalidProcess`.
- `internal/sim/sim.go` (modify) — `effectiveTraining/Inference`, rent cost, revenue `×RevenueMult`.
- `internal/sim/compute.go` (new) — `ProcessUnlocked`, `poolRentPerSec`, map-clone helper.
- `internal/tui/page_compute.go` (modify) — process table + cursor.
- `internal/tui/tui.go` (modify) — `procCursor`, compute-page keys route to the selected process.
- Tests alongside each.

---

## Task 1: Process catalog + economy scalars (balance)

Purely additive — nothing consumes it yet, so the suite stays green.

**Files:** Modify `internal/balance/balance.go`; Test `internal/balance/balance_test.go`.

**Interfaces (Produces):**
- `const EntryProcessID = "N7"`
- `type Process struct { ID, Name string; Compute, PowerKW, RentPerSec, BuyPrice float64; UnlockTech string }`
- `func DefaultProcesses() []Process`
- `func ProcessByID(ps []Process, id string) (Process, bool)`
- `Config.Processes []Process`, `Config.TrainRentMult float64`, `Config.RevenueMult float64`
- Tech nodes `process-N5/N3/N2` in `DefaultTechNodes()`.

- [ ] **Step 1: Write failing test** (append to `balance_test.go`)

```go
func TestDefaultProcesses(t *testing.T) {
	c := Default()
	if len(c.Processes) != 4 {
		t.Fatalf("processes = %d, want 4", len(c.Processes))
	}
	n7, ok := ProcessByID(c.Processes, EntryProcessID)
	if !ok || n7.UnlockTech != "" || n7.Compute != 1 || n7.RentPerSec != 0.001 {
		t.Errorf("N7 entry wrong: %+v ok=%v", n7, ok)
	}
	n5, _ := ProcessByID(c.Processes, "N5")
	if n5.UnlockTech != "process-N5" || n5.Compute != 2 {
		t.Errorf("N5 wrong: %+v", n5)
	}
	// higher process = better compute-per-rent and compute-per-watt
	prev := 0.0
	for _, p := range c.Processes {
		if r := p.Compute / p.RentPerSec; r < prev {
			t.Errorf("compute/rent should be non-decreasing, %s broke it", p.ID)
		} else {
			prev = r
		}
	}
	if c.RevenueMult != 2 || c.TrainRentMult < 1.6 || c.TrainRentMult > 1.7 {
		t.Errorf("economy scalars wrong: rev=%v trainmult=%v", c.RevenueMult, c.TrainRentMult)
	}
	byID := map[string]model.TechNode{}
	for _, n := range c.TechNodes {
		byID[n.ID] = n
	}
	if n, ok := byID["process-N3"]; !ok || len(n.Prereqs) != 1 || n.Prereqs[0] != "process-N5" {
		t.Errorf("process-N3 prereq wrong: %+v", n)
	}
}
```

- [ ] **Step 2: Run to verify fail** — `go test ./internal/balance -run TestDefaultProcesses` → FAIL (undefined Process/DefaultProcesses).

- [ ] **Step 3: Implement** in `balance.go`:

```go
// EntryProcessID is the process available from the first day (no tech unlock).
const EntryProcessID = "N7"

// Process is a compute node: rentable (opex) or buildable (capex).
type Process struct {
	ID         string
	Name       string
	Compute    float64 // compute per chip (old GPU scale = 1)
	PowerKW    float64
	RentPerSec float64 // inference rent per chip/sec; training = ×TrainRentMult
	BuyPrice   float64
	UnlockTech string // "" = from start
}

func DefaultProcesses() []Process {
	return []Process{
		{"N7", "N7 入門", 1, 2.0, 0.001, 6000, ""},
		{"N5", "N5", 2, 3.0, 0.0018, 15000, "process-N5"},
		{"N3", "N3", 4, 5.0, 0.003, 40000, "process-N3"},
		{"N2", "N2", 8, 8.0, 0.005, 100000, "process-N2"},
	}
}

func ProcessByID(ps []Process, id string) (Process, bool) {
	for _, p := range ps {
		if p.ID == id {
			return p, true
		}
	}
	return Process{}, false
}
```

Add to `Config` struct: `Processes []Process`, `TrainRentMult float64`, `RevenueMult float64`. In `Default()`:

```go
	c.Processes = DefaultProcesses()
	c.TrainRentMult = 1.667
	c.RevenueMult = 2
```

Append to `DefaultTechNodes()` (Infra branch, chained gates, neutral effects):

```go
		techNode("process-N5", model.BranchInfra, 150000, nil, func(e *model.TechEffects) {}),
		techNode("process-N3", model.BranchInfra, 1500000, []string{"process-N5"}, func(e *model.TechEffects) {}),
		techNode("process-N2", model.BranchInfra, 10000000, []string{"process-N3"}, func(e *model.TechEffects) {}),
```

- [ ] **Step 4: Run to verify pass** — `go test ./internal/balance` → PASS; `go test ./...` still green (additive).

- [ ] **Step 5: Commit** — `feat(balance): process catalog, RevenueMult, TrainRentMult, process-unlock nodes`.

---

## Task 2: Per-process compute data model (model + sim)

The atomic core: `Compute` becomes per-process maps; rent/build commands become process-aware; revenue gains `RevenueMult`. All affected sim/game/prestige tests are updated here so the suite ends green.

**Files:** Modify `internal/model/types.go`, `internal/sim/apply.go`, `internal/sim/sim.go`; Create `internal/sim/compute.go`; Modify affected tests in `internal/sim/*_test.go`, `internal/game/game.go`+test, `internal/model/types_test.go`.

**Interfaces:**
- `model.Compute{ RentedTraining, RentedInference map[string]int; InferenceLoad float64 }`
- `model.RentCompute{ Process string; Pool ComputePool; Delta int }` (+`commandMarker`); **remove** `RentTrainingCompute`, `RentInferenceCompute`, `Chip`.
- `model.BuildServer{ Process string }`
- `sim.ProcessUnlocked(ns model.GameState, b balance.Config, id string) bool`
- `sim.ErrProcessLocked`, `sim.ErrInvalidProcess`
- Revenue in `advanceUsers` multiplied by `b.RevenueMult`.

- [ ] **Step 1: Write failing tests** (`internal/sim/compute_test.go`)

```go
package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestEffectiveComputeSumsProcesses(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Compute: model.Compute{
		RentedTraining:  map[string]int{"N7": 2, "N5": 4}, // 2*1 + 4*2 = 10
		RentedInference: map[string]int{"N7": 1},          // 1*1 = 1
	}}
	if got := EffectiveTraining(s, b); got != 10 {
		t.Fatalf("EffectiveTraining = %v, want 10", got)
	}
	if got := EffectiveInference(s, b); got != 1 {
		t.Fatalf("EffectiveInference = %v, want 1", got)
	}
	// nil maps → 0, no panic
	if EffectiveTraining(model.GameState{}, b) != 0 {
		t.Fatal("nil map should give 0")
	}
}

func TestRentComputeRespectsLockAndPool(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	// N5 locked at start
	if _, err := Apply(s, model.RentCompute{Process: "N5", Pool: model.PoolTraining, Delta: 1}, b); err != ErrProcessLocked {
		t.Fatalf("locked process: err = %v, want ErrProcessLocked", err)
	}
	// N7 available
	ns, err := Apply(s, model.RentCompute{Process: "N7", Pool: model.PoolInference, Delta: 3}, b)
	if err != nil || ns.Compute.RentedInference["N7"] != 3 {
		t.Fatalf("rent N7 inf: %+v err=%v", ns.Compute.RentedInference, err)
	}
	// floors at 0, input not mutated
	ns2, _ := Apply(ns, model.RentCompute{Process: "N7", Pool: model.PoolInference, Delta: -10}, b)
	if ns2.Compute.RentedInference["N7"] != 0 {
		t.Fatalf("should floor at 0, got %v", ns2.Compute.RentedInference["N7"])
	}
	if ns.Compute.RentedInference["N7"] != 3 {
		t.Fatal("Apply mutated input map")
	}
}

func TestRevenueMultScalesRevenue(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Models: []model.Model{{Online: true, Users: 1000, Price: 12}}}
	ns := Tick(s, 1, nil, b)
	// revenue = 1000*12/MonthSec*RevenueMult(2) ; just assert it doubled vs mult=1
	b1 := b
	b1.RevenueMult = 1
	ns1 := Tick(s, 1, nil, b1)
	gain2 := ns.Resources.Cash
	gain1 := ns1.Resources.Cash
	if gain2 <= gain1 {
		t.Fatalf("RevenueMult=2 should out-earn 1: %v vs %v", gain2, gain1)
	}
}
```

- [ ] **Step 2: Run to verify fail** — `go test ./internal/sim -run 'TestEffectiveComputeSumsProcesses|TestRentComputeRespectsLockAndPool|TestRevenueMultScalesRevenue'` → FAIL (compile: Compute has no RentedTraining, etc.).

- [ ] **Step 3: Implement — model/types.go**

Replace `Compute`:
```go
type Compute struct {
	RentedTraining  map[string]int `json:"rentedTraining,omitempty"`
	RentedInference map[string]int `json:"rentedInference,omitempty"`
	InferenceLoad   float64
}
```
Replace `RentTrainingCompute`/`RentInferenceCompute` with:
```go
type RentCompute struct {
	Process string
	Pool    ComputePool
	Delta   int
}

func (RentCompute) commandMarker() {}
```
Change `BuildServer`:
```go
type BuildServer struct {
	Process string
}
```
Delete the `Chip` type (moved to `balance.Process`).

- [ ] **Step 4: Implement — sim/compute.go** (new)

```go
package sim

import (
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// ProcessUnlocked reports whether the player may rent/build a process: the
// entry node is always available; others require their tech node.
func ProcessUnlocked(ns model.GameState, b balance.Config, id string) bool {
	p, ok := balance.ProcessByID(b.Processes, id)
	if !ok {
		return false
	}
	return p.UnlockTech == "" || isUnlocked(ns, p.UnlockTech)
}

// poolCompute sums chip counts × per-process compute for a rented-pool map.
func poolCompute(rented map[string]int, b balance.Config) float64 {
	var c float64
	for id, n := range rented {
		if p, ok := balance.ProcessByID(b.Processes, id); ok {
			c += float64(n) * p.Compute
		}
	}
	return c
}

// poolRentPerSec is the aggregate rent per game-second across both pools
// (training pays ×TrainRentMult).
func poolRentPerSec(ns model.GameState, b balance.Config) float64 {
	var r float64
	for id, n := range ns.Compute.RentedInference {
		if p, ok := balance.ProcessByID(b.Processes, id); ok {
			r += float64(n) * p.RentPerSec
		}
	}
	for id, n := range ns.Compute.RentedTraining {
		if p, ok := balance.ProcessByID(b.Processes, id); ok {
			r += float64(n) * p.RentPerSec * b.TrainRentMult
		}
	}
	return r
}

// cloneCounts copies a rented-pool map for pure mutation.
func cloneCounts(m map[string]int) map[string]int {
	out := make(map[string]int, len(m)+1)
	for k, v := range m {
		out[k] = v
	}
	return out
}
```

- [ ] **Step 5: Implement — sim/sim.go** effective compute (replace bodies) + rent + revenue

```go
func effectiveTraining(ns model.GameState, b balance.Config) float64 {
	c := poolCompute(ns.Compute.RentedTraining, b)
	for _, sv := range ns.Servers {
		if sv.Pool == model.PoolTraining {
			c += sv.Compute
		}
	}
	return c * infraEfficiency(ns, b) * techEffects(ns, b).InfraMult * starEffects(ns, b).InfraMult
}

func effectiveInference(ns model.GameState, b balance.Config) float64 {
	c := poolCompute(ns.Compute.RentedInference, b)
	for _, sv := range ns.Servers {
		if sv.Pool == model.PoolInference {
			c += sv.Compute
		}
	}
	return c * infraEfficiency(ns, b) * techEffects(ns, b).InfraMult * starEffects(ns, b).InfraMult
}
```
In `Tick`, replace the two rent-deduction lines with:
```go
	ns.Resources.Cash -= poolRentPerSec(ns, b) * dt
```
In `advanceUsers`, multiply the revenue line by `b.RevenueMult`:
```go
		ns.Resources.Cash += m.Users * m.Price * dt / b.MonthSec * pe.CashMult * b.RevenueMult
```

- [ ] **Step 6: Implement — sim/apply.go** commands

Add errors: `ErrProcessLocked = errors.New("sim: process not unlocked")`, `ErrInvalidProcess = errors.New("sim: unknown process")`. Replace the `RentTrainingCompute`/`RentInferenceCompute` cases in `Apply`'s switch with:
```go
	case model.RentCompute:
		return applyRentCompute(s, c, b)
```
Delete `applyRentTrainingCompute`/`applyRentInferenceCompute`; add:
```go
func applyRentCompute(s model.GameState, c model.RentCompute, b balance.Config) (model.GameState, error) {
	if _, ok := balance.ProcessByID(b.Processes, c.Process); !ok {
		return s, ErrInvalidProcess
	}
	if !ProcessUnlocked(s, b, c.Process) {
		return s, ErrProcessLocked
	}
	ns := s
	if c.Pool == model.PoolTraining {
		ns.Compute.RentedTraining = cloneCounts(s.Compute.RentedTraining)
		ns.Compute.RentedTraining[c.Process] = max0(ns.Compute.RentedTraining[c.Process] + c.Delta)
	} else {
		ns.Compute.RentedInference = cloneCounts(s.Compute.RentedInference)
		ns.Compute.RentedInference[c.Process] = max0(ns.Compute.RentedInference[c.Process] + c.Delta)
	}
	return ns, nil
}
```
Change `applyBuildServer` to look up `balance.ProcessByID(b.Processes, c.Process)` instead of `findChip(b.Chips, c.ChipName)`, using `chip.Compute`/`chip.PowerKW`/`chip.Price`→`p.Compute`/`p.PowerKW`/`p.BuyPrice`, and `n := 1.0` (one chip per build, or keep `b.ChipsPerServer` if retained). Return `ErrInvalidProcess` when not found; also reject locked via `ProcessUnlocked`.

- [ ] **Step 7: Update affected tests + call sites**

- `internal/game/game.go`: NewGame no longer sets compute (maps start nil → 0). Remove `StartingTrainingCapacity`/`StartingInferenceCapacity` seeding; remove those balance constants (and from `freshRun`). Update `game_test.go` `TestNewGameSeed` — drop the compute assertion (compute now starts empty).
- `internal/sim/prestige.go` `freshRun`: remove the two `Compute.*Capacity` lines. `prestige_test.go` `TestFreshRun`: drop those compute assertions.
- Any sim test setting `s.Compute.TrainingCapacity = N` → `s.Compute.RentedTraining = map[string]int{"N7": N}` (N7 compute 1 keeps the effective value). Same for inference. Files: `sim_test.go` (training/serving/rent tests), `apply_test.go` (rent/build tests → `RentCompute`, `BuildServer{Process:"N7"}` or a training process).
- `internal/model/types_test.go`: any `RentTrainingCompute`/`Chip` reference → `RentCompute`/removed.
- `internal/balance/balance_test.go` `TestDefaultChipsAndInfra`: repoint to `Processes` or delete if `Chips` removed.

- [ ] **Step 8: Run to verify pass** — `go test ./... && go vet ./... && go build ./...` → all green.

- [ ] **Step 9: Commit** — `feat(sim): per-process compute model, RentCompute, RevenueMult`.

---

## Task 3: TUI compute page — process table + cursor + keys

**Files:** Modify `internal/tui/page_compute.go`, `internal/tui/tui.go`; Test `internal/tui/page_compute_test.go`.

**Interfaces:** `Model.procCursor int`; compute-page keys: `up/down` move `procCursor` (unlocked-agnostic, bounded to catalog length); `r/R/i/I` issue `RentCompute{selectedProcess, pool, ±1}`; `b` issues `BuildServer{selectedProcess}`.

- [ ] **Step 1: Write failing tests**

```go
func TestComputePageListsProcesses(t *testing.T) {
	m := testModel(t)
	m.page = PageCompute
	v := renderCompute(m)
	for _, w := range []string{"N7", "N5", "訓練池", "推理池", "解鎖"} {
		if !strings.Contains(v, w) {
			t.Errorf("compute page missing %q:\n%s", w, v)
		}
	}
}

func TestComputeRentSelectedProcess(t *testing.T) {
	m := testModel(t)
	m.page = PageCompute
	m.procCursor = 0 // N7 (entry, unlocked)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if nm.(Model).state.Compute.RentedInference["N7"] != 1 {
		t.Fatalf("i should rent 1 N7 into inference")
	}
}

func TestComputeCannotRentLocked(t *testing.T) {
	m := testModel(t)
	m.page = PageCompute
	m.procCursor = 1 // N5 (locked at start)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if len(nm.(Model).state.Compute.RentedTraining) != 0 {
		t.Fatalf("renting a locked process should be a no-op")
	}
}
```

- [ ] **Step 2: Run to verify fail** — FAIL (procCursor undefined / old page body).

- [ ] **Step 3: Implement** — add `procCursor int` to `Model`. In `Update`, on `PageCompute`: `up/down` adjust `procCursor` within `[0, len(cfg.Processes)-1]` (mirror the tech-page cursor guard). Replace the old `r/R/i/I/b` compute handlers with:
```go
		case "r", "R", "i", "I":
			if m.page == PageCompute {
				p := m.cfg.Processes[m.procCursor]
				pool := model.PoolInference
				if msg.String() == "r" || msg.String() == "R" {
					pool = model.PoolTraining
				}
				d := 1
				if msg.String() == "R" || msg.String() == "I" {
					d = -1
				}
				m.state = applyOK(m.state, model.RentCompute{Process: p.ID, Pool: pool, Delta: d}, m.cfg)
			}
			return m, nil
		case "b":
			if m.page == PageCompute {
				m.state = applyOK(m.state, model.BuildServer{Process: m.cfg.Processes[m.procCursor].ID}, m.cfg)
			}
			return m, nil
```
(`applyOK` already swallows `ErrProcessLocked` → locked rows are inert.) Guard `up/down` so `PageCompute` uses `procCursor` and `PageTech` uses `techCursor`.

Rewrite `renderCompute` to show the two pool bars (using `sim.EffectiveTraining/Inference`), then a table over `m.cfg.Processes` with a `▸` cursor, compute/rent columns, per-process rented counts (`m.state.Compute.RentedTraining[p.ID]`/inference), and a lock marker (`🔒需 …`) when `!sim.ProcessUnlocked(m.state, m.cfg, p.ID)`, then the datacenter line and the help line from §5 of the spec.

- [ ] **Step 4: Run to verify pass** — targeted + `go test ./...` green.

- [ ] **Step 5: Commit** — `feat(tui): process-node compute page with per-process rent/build`.

---

## Task 4: Break-even integration test

Locks in the spec's economy target so future tuning can't silently regress it.

**Files:** Test `internal/sim/economy_test.go` (new).

- [ ] **Step 1: Write the test**

```go
package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestEarlyGameBreakEven(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Research.EfficiencyMult = 1
	s.Research.Researchers[model.Tier1] = 2
	s.Marketing = 1
	s.Compute.RentedInference = map[string]int{"N7": 1}
	s.Models = []model.Model{{Online: true, Segment: model.SegConsumer, Price: 12, Users: 1000,
		Quality: [model.NumQualityDims]float64{25, 0, 0, 0}}}
	before := s.Resources.Cash
	s = Tick(s, 3600, nil, b) // one hour
	if s.Resources.Cash <= before {
		t.Fatalf("Gen1 ~1000 users + 1 N7 + 3 staff should be cash-positive, delta=%v", s.Resources.Cash-before)
	}
}
```

- [ ] **Step 2: Run to verify pass** — should PASS given the calibrated numbers (revenue ~$133/s > costs ~$64/s). If it fails, the plan's numbers need re-tuning before proceeding — do NOT weaken the assertion.

- [ ] **Step 3: Commit** — `test(sim): lock in early-game break-even target`.

---

## Self-Review

- **Spec coverage:** §2 model=Task 2; §3 catalog=Task 1; §4 tech unlock=Task 1 (nodes)+Task 2 (`ProcessUnlocked`, gating); §5 UI=Task 3; §6 A(`RevenueMult`)=Task 1/2, B(N7 cheap)=Task 1; §7 back-compat: nil maps → 0 (Task 2 handles nil); §8 testing across all tasks (Task 4 break-even). Migration of *old numeric* saves into N7 is best-effort — noted in spec; since the `Compute` JSON keys changed, old saves deserialize to nil maps (zero compute) and the player re-rents, which is acceptable and simpler than a bespoke migration.
- **Type consistency:** `RentCompute{Process,Pool,Delta}` identical in Tasks 2/3; `BuildServer{Process}` in Tasks 2/3; `balance.Process`/`ProcessByID`/`EntryProcessID` in Tasks 1/2/3; `EffectiveTraining/Inference` exported names reused; `RevenueMult`/`TrainRentMult` in Tasks 1/2.
- **Placeholders:** none — real code for the model, sim, apply, and UI key routing; test bodies provided.
- **Non-obvious gotchas called out:** purity via `cloneCounts` before map mutation; `up/down` must branch by page (compute vs tech cursor); `applyOK` making locked rows inert; removing `StartingTrainingCapacity`/`Chips` and updating their tests.

## Execution Handoff

Plan saved to `docs/superpowers/plans/2026-07-09-process-node-compute.md`. Two execution options:

1. **Subagent-Driven (recommended)** — fresh subagent per task + review between tasks.
2. **Inline Execution** — execute here with a test/vet/build gate and commit per task (the rhythm used for the earlier plans this session).
