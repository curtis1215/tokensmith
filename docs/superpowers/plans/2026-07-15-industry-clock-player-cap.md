# Industry Clock Player Cap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bind TimeFrontier and IndustryTime to MaxUnlockedGen+1, throttle online idle industry to 0.15×, align offline allowance, and soft-repair overheated saves on load.

**Architecture:** Pure helpers in `sim` own cap/residual/effective industry DT and clamp+rival reband. `Tick` uses effective DT; `tickWithClocks` defensively clamps IndustryTime. `TimeFrontier` interpolates on effective day. `tui.Settle` mins residual-to-cap. `store.Load` calls clamp soft-repair without schema bump.

**Tech Stack:** Go, existing `balance` / `sim` / `store` / `tui` packages, `go test`.

## Global Constraints

- No `schemaVersion` bump (remain 2).
- Do not rewrite player model Quality.
- `IndustryPlayerLeadGens = 1`, `IndustryIdleMult = 0.15`.
- TDD: failing test before production code for each behavior.
- Do not change share formulas, catch-up rate, or QualityScale tables.

---

### Task 1: Balance constants + industry clock helpers

**Files:**
- Modify: `internal/balance/balance.go` (add constants near `RealSecCompression`)
- Create: `internal/sim/industry_clock.go`
- Create: `internal/sim/industry_clock_test.go`

**Interfaces:**
- Produces:
  - `balance.IndustryPlayerLeadGens int = 1`
  - `balance.IndustryIdleMult float64 = 0.15`
  - `sim.IndustryTimeCapSec(s model.GameState, b balance.Config) float64`
  - `sim.IndustryTimeResidualToCap(s model.GameState, b balance.Config) float64`
  - `sim.EffectiveIndustryDT(s model.GameState, economyDT float64, b balance.Config) float64`
  - `sim.ClampIndustryToPlayerCap(s model.GameState, b balance.Config) model.GameState`

- [ ] **Step 1: Write failing tests** in `industry_clock_test.go` for residual, idle mult, engaged full, at-cap zero.

- [ ] **Step 2: Run tests — expect FAIL** (`EffectiveIndustryDT` undefined).

- [ ] **Step 3: Implement constants + helpers** (cap via `MaxUnlockedGen` + lead → `Generation.TimeBaselineDay`; idle when `!Frontier.Active && !HasTraining`; clamp uses residual + `clampAllRivalsToBand`).

- [ ] **Step 4: Tests PASS.**

- [ ] **Step 5: Commit** `feat(sim): industry time cap and effective industry DT`

---

### Task 2: TimeFrontier uses effective day

**Files:**
- Modify: `internal/sim/frontier.go` (`TimeFrontier`)
- Modify: `internal/sim/frontier_test.go`

- [ ] **Step 1: Failing test** — MaxUnlockedGen=5, IndustryTime huge → TimeFrontier equals scale at Gen6 baseline (not Gen18).

- [ ] **Step 2: Implement** — `effectiveDay = min(IndustryTime/86400, capDay)` before `interpolatedQualityScale`.

- [ ] **Step 3: Tests PASS** including existing interpolation tests (Industry below cap).

- [ ] **Step 4: Commit** `feat(sim): cap TimeFrontier to player lead generation`

---

### Task 3: Tick + defensive clamp in tickWithClocks

**Files:**
- Modify: `internal/sim/sim.go`
- Modify: `internal/sim/sim_clocks_test.go` (or industry_clock_test)

- [ ] **Step 1: Failing tests** — idle Tick industry Δ = 0.15×dt; at-cap Tick industry Δ = 0; explicit oversize industryDT clamped.

- [ ] **Step 2: `Tick` uses `EffectiveIndustryDT`; after `IndustryTime += industryDT`, clamp to cap.**

- [ ] **Step 3: Tests PASS.** Note: `TestTickWithClocksOnlineSameDelta` still compares Tick vs equal clocks only when engaged or both apply same mult — update fixture to HasTraining or Frontier so full-speed path matches, **or** change expectation that Tick uses effective DT.

- [ ] **Step 4: Commit** `feat(sim): online Tick uses throttled industry clock`

---

### Task 4: Offline settle residual-to-cap

**Files:**
- Modify: `internal/tui/settle.go`
- Modify: `internal/tui/settle_test.go`

- [ ] **Step 1: Failing test** — IndustryTime already at cap → Settle leaves IndustryTime unchanged.

- [ ] **Step 2: `offlineIndustryAllowance` min with `sim.IndustryTimeResidualToCap`.**

- [ ] **Step 3: Tests PASS.**

- [ ] **Step 4: Commit** `fix(tui): offline industry allowance respects player cap`

---

### Task 5: Store load soft-repair

**Files:**
- Modify: `internal/store/store.go`
- Modify: `internal/store/store_test.go` or `migrate_test.go`

- [ ] **Step 1: Failing test** — save with IndustryTime=40500 days, MaxUnlockedGen=5, rival Q far above post-cap GF → Load clamps IndustryTime and rival into band.

- [ ] **Step 2: Call `sim.ClampIndustryToPlayerCap` on all successful load paths (legacy migrate, schema upgrade, current).**

- [ ] **Step 3: Tests PASS; `go test ./...` green.**

- [ ] **Step 4: Commit** `fix(store): soft-repair overheated industry time on load`

---

## Spec coverage checklist

| Spec § | Task |
|--------|------|
| §3 constants | T1 |
| §4.1–4.3 helpers | T1 |
| §4.2 TimeFrontier | T2 |
| §4.4 Tick / clamp | T3 |
| §4.5 offline | T4 |
| §5 load repair | T5 |
| §8 tests | T1–T5 |
