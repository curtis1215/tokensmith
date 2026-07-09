# Process-Node Compute + Early-Economy Rebalance — Design

**Date:** 2026-07-09
**Status:** Approved (brainstorming), pending spec review

## 1. Overview & Goals

Replace the flat, undifferentiated GPU rental with a **process-node** system: compute is acquired from a catalog of process nodes (N7 → N5 → N3 → N2), each with its own compute-per-chip, power, rent, and buy price. Newer nodes are unlocked with R&D via the tech tree. The same catalog serves both **renting** (opex, per-second) and **self-building** (capex, servers). Bundled with this: an early-economy rebalance so a starting model can turn a profit.

Goals:
- **Depth**: choosing a process node (cheap-old vs expensive-new) is a real decision with a dedicated interface.
- **Progression**: process access advances with the player's R&D (tech tree), same mechanism as model-generation unlocks.
- **Fix early economy (A+B)**: raise per-user revenue ~2× (A) and make entry compute cheap (B, via the N7 node) so a ~1000-user Gen1 with a lean setup is profitable.

Approved decisions (brainstorming):
1. **Unified** catalog — one process list, rent **or** buy.
2. **Tech-tree unlock** — spend R&D to unlock N5/N3/N2 (N7 free from start).
3. **Universal chip per process** — one chip type per node, freely allocated between the training and inference pools.

## 2. Data Model

Replace `Compute{TrainingCapacity, InferenceCapacity, InferenceLoad}` (single floats) with per-process rented counts:

```go
type Compute struct {
	RentedTraining  map[string]int // process ID → chips rented into training
	RentedInference map[string]int // process ID → chips rented into inference
	InferenceLoad   float64        // computed each tick
}
```

- **Effective training compute** = `Σ_p RentedTraining[p] × Process[p].Compute` + self-built training-server compute, all × engineer/tech/star multipliers (unchanged multiplier stack).
- **Effective inference compute** = analogous with `RentedInference`.
- **Rent cost/tick** = `Σ_p ( RentedInference[p]×Process[p].RentPerSec + RentedTraining[p]×Process[p].RentPerSec×TrainRentMult ) × dt`. `TrainRentMult = 1.667` preserves the old training/inference ratio (0.01/0.006).
- Map summation is order-independent → the pure sim stays deterministic.

Commands change to carry a process:
```go
type RentCompute struct { Process string; Pool ComputePool; Delta int } // replaces RentTrainingCompute / RentInferenceCompute
type BuildServer struct { Process string }                              // was ChipName
```

## 3. Process Catalog

A unified catalog replaces `Chips`:

```go
type Process struct {
	ID         string  // "N7","N5","N3","N2"
	Name       string  // display
	Compute    float64 // compute per chip (same scale as the old GPU = 1)
	PowerKW    float64 // power per chip
	RentPerSec float64 // inference rent per chip per game-second (training = ×TrainRentMult)
	BuyPrice   float64 // capex per chip (self-build)
	UnlockTech string  // "" = available from start; else tech node ID
}
```

v0 ladder (tunable; each higher node has better compute/$ and compute/watt but higher absolute cost):

| Process | Compute/chip | Power/chip | Rent/sec (inf) | Buy/chip | Unlock (R&D) |
|---|---|---|---|---|---|
| **N7** (entry) | 1 | 2.0 | 0.001 | 6,000 | — (from start) |
| **N5** | 2 | 3.0 | 0.0018 | 15,000 | 150,000 |
| **N3** | 4 | 5.0 | 0.003 | 40,000 | 1,500,000 |
| **N2** | 8 | 8.0 | 0.005 | 100,000 | 10,000,000 |

Checks: compute/$ (rent) 1000→1111→1333→1600 (improves); compute/watt 0.5→0.667→0.8→1.0 (improves). N7 inference rent = 0.001×14400 = **$14.4/real-sec** (vs the old $86.4) — this is lever **B**.

## 4. Tech-Tree Process Unlock

Add process-unlock nodes to the Infra branch (`DefaultTechNodes`), chained like the model-gen nodes:

- `process-N5` — cost 150,000, no prereq
- `process-N3` — cost 1,500,000, prereq `process-N5`
- `process-N2` — cost 10,000,000, prereq `process-N3`

These are pure gates (neutral `TechEffects`). Helper `sim.ProcessUnlocked(ns, b, procID) bool` (N7 always true; others require their node). Rent/Build reject a locked process with `ErrProcessLocked`.

## 5. Rental / Build UI (算力 page)

The compute page becomes process-aware. Top: per-pool effective capacity + utilisation. Middle: a table of process nodes (unlocked ones actionable, locked ones greyed with the R&D requirement). A cursor selects a row; the existing r/R/i/I keys act on the selected process.

```
訓練池  ▓▓▓▓▓▓▓░░ 72%   有效算力 10
推理池  ▓▓▓▓▓▓▓▓░ 88% ⚠  有效算力 5 · 負載 4.4

製程算力                                可用 R&D 1.2M
              算力  租金/秒*  訓練張  推理張
▸ N7 入門       1    $0.0010    2       1
  N5 ✓          2    $0.0018    4       2
  N3 🔒需1.5M    4    $0.0030    —       —
  N2 🔒需10M     8    $0.0050    —       —
* 推理租金/張/秒；訓練池 ×1.667。表格數字為示意。

機房  電力 71% · 空間 63%
[↑↓]選製程 [r/R]±訓練 [i/I]±推理 [b]自建 [e]擴機房 [Tab]切頁
```
(有效算力已含工程師/科技倍率，故大於「張數×算力」的裸值。)

Interaction on `PageCompute`:
- `↑↓` — move the process cursor (`Model.procCursor`), unlocked rows only actionable.
- `r`/`R` — `RentCompute{Process:sel, Pool:Training, Delta:±1}`
- `i`/`I` — `RentCompute{Process:sel, Pool:Inference, Delta:±1}`
- `b` — `BuildServer{Process:sel}` (capex, self-build)
- `e` — `ExpandDatacenter` (unchanged)
- Acting on a locked process is a no-op (the row shows its unlock cost).

## 6. Economy Rebalance (A + B)

**A — per-user revenue ×2.** Add `RevenueMult float64` (default 2). Revenue becomes `Users × Price / MonthSec × RevenueMult × CashMult`. (Chosen over lowering MonthSec, which would muddy the "month" semantics.)

**B — cheap entry compute.** Handled by the N7 node (§3): inference rent $14.4/real-sec vs the old $86.4, so a small early user base is affordable.

Target break-even (Gen1 ~1000 users, 1 N7 inference chip, 3 staff):
```
營收 (1000×$12×2)     +$133/s
N7 推理租金 (1 張)     -$14/s
薪水 (3 員工)         -$50/s
────────────────────
淨                   +$69/s  → profitable
```
Growth (more users / higher process) stays profitable; a bloated setup (many idle chips / over-hiring) can still run at a loss — that's intended tension.

## 7. Backward Compatibility

The `Compute` struct changes shape, so old saves' `TrainingCapacity`/`InferenceCapacity` no longer bind. On load, old numeric capacity (if present) is migrated into `N7` rentals (`RentedTraining["N7"] = round(oldTrainingCapacity)`, same for inference); absent → nil maps → zero compute. Since balance changes already prompt a fresh run, this is best-effort. Nil maps are initialised lazily in the sim (treat nil as empty).

## 8. Testing Strategy

Pure-sim (`internal/sim`, `internal/balance`):
- Effective training/inference compute = Σ per-process (with mults); nil maps → 0.
- Rent cost sums per process with the training multiplier.
- `RentCompute` adds/removes per pool, floors at 0, rejects locked process (`ErrProcessLocked`).
- `ProcessUnlocked` gating; `BuildServer` uses the process catalog.
- `RevenueMult` scales revenue; break-even scenario is cash-positive.
- Determinism: identical result regardless of map order.

TUI (`internal/tui`):
- Compute page renders the process table, cursor moves, locked rows greyed.
- r/R/i/I act on the selected process and the correct pool; b builds the selected process.

## 9. Out of Scope (v2+)

- Player-designed chips / foundry line (spec §17.5 v2 preview) — process advancement here is R&D-gated access to an external catalog, not designing your own.
- Auto-advancing industry (nodes appearing over time) — access is tech-tree only.
- Per-pool-optimised chip variants (training vs inference products) — one universal chip per process.
