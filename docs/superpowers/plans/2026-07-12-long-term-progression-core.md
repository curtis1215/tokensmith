# Tokensmith Long-Term Progression Core Implementation Plan

> **For Codex:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Use `superpowers:test-driven-development` for every behavior change and `superpowers:verification-before-completion` before claiming completion.

**Goal:** Replace the Gen5 hard stop and campaign rival runaway with a paced Gen6–10 bridge, procedural Gen11+ eras, compute-sharing frontier research, bounded global-frontier rivals, versioned save migration, and integrated TUI views.

**Architecture:** Add progression value types under `internal/model`, generation/era catalogs under `internal/balance`, and pure frontier/era/rival engines under `internal/sim`. Keep wall-clock handling in `internal/tui`, but pass separate economy and industry deltas into the pure sim so offline industry progress can be capped independently. Put save-envelope detection and v0 migration in `internal/store`; TUI pages consume exported sim view models rather than duplicate formulas.

**Tech Stack:** Go 1.22+, Bubble Tea, Lip Gloss, standard `testing`, JSON persistence, existing SplitMix64 campaign RNG.

**Source design:** `docs/superpowers/specs/2026-07-12-long-term-progression-core-design.md`
**Starting commit:** `0227597` (`docs: design long-term progression core`)

---

## Working Rules

- Execute tasks in order; later tasks depend on earlier APIs.
- Each task follows red → green → focused regression → commit.
- Do not combine unrelated refactors with this feature.
- Keep `internal/sim` deterministic: no wall clock, I/O, or global RNG.
- Preserve Gen1–5 values and existing fixed tech IDs exactly.
- Run `gofmt` before each task verification.
- Commit only after the task's focused tests pass.

## Task 1: Add progression state and commands

**Files:**

- Create: `internal/model/progression.go`
- Create: `internal/model/progression_test.go`
- Modify: `internal/model/types.go`
- Modify: `internal/game/game.go`
- Modify: `internal/game/game_test.go`
- Modify: `internal/sim/prestige.go`
- Modify: `internal/sim/prestige_test.go`

**Step 1: Write failing model tests**

Test construction and JSON round-trip for:

```go
type ProgressionState struct {
    MaxUnlockedGen int
    IndustryTime   float64
    Frontier       FrontierProject
    Eras           []EraProgress
    Rivals         RivalEraState
}

type FrontierProject struct {
    Active             bool
    TargetGen          int
    RnDTotal           float64
    RnDRemaining       float64
    WorkTotal          float64
    WorkRemaining      float64
    RecommendedCompute float64
    AllocationPct      int
}

type EraProgress struct {
    Era          int
    HasPrimary   bool
    Primary      TechBranch
    UnlockedMask uint8
}

type RivalEraState struct {
    Era     int
    Leaders []string
}
```

Also assert that `StartFrontierProject`, `SetFrontierAllocation`, and `UnlockEraBreakthrough` satisfy `model.Command`.

**Step 2: Confirm red**

```bash
go test ./internal/model ./internal/game ./internal/sim -run 'TestProgression|TestNewGame|TestFreshRun' -count=1
```

Expected: compile failure because the types and commands do not exist.

**Step 3: Implement minimum state**

- Put progression types and command markers in `internal/model/progression.go`.
- Add `Progression ProgressionState` to `model.GameState` near run-scoped progress.
- Extend `model.Competitor` with:

```go
MomentumPct    [NumQualityDims]float64
MomentumCycles int
```

- Initialize `Progression.MaxUnlockedGen = 1` in `game.NewGame()` and `sim.freshRun()`.
- Ensure restart/prestige clears the rest of `ProgressionState` while preserving prestige and RNG behavior.

**Step 4: Verify green**

```bash
gofmt -w internal/model/progression.go internal/model/progression_test.go internal/model/types.go internal/game/game.go internal/game/game_test.go internal/sim/prestige.go internal/sim/prestige_test.go
go test ./internal/model ./internal/game ./internal/sim -run 'TestProgression|TestNewGame|TestFreshRun|TestRestart' -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/model/progression.go internal/model/progression_test.go internal/model/types.go internal/game/game.go internal/game/game_test.go internal/sim/prestige.go internal/sim/prestige_test.go
git commit -m "feat(model): add long-run progression state"
```

## Task 2: Introduce the generation and era catalog

**Files:**

- Create: `internal/balance/generation.go`
- Create: `internal/balance/generation_test.go`
- Modify: `internal/balance/balance.go`
- Modify: `internal/balance/balance_test.go`

**Step 1: Write failing catalog tests**

Cover:

- era boundaries `1→I`, `3→II`, `5→III`, `8→IV`, `11→V`, then every three generations;
- exact Gen1–5 compatibility;
- approved Gen6–10 table;
- Gen11 formulas;
- Gen11–100 positivity, finiteness, and monotonicity;
- invalid `gen < 1` returns `ErrInvalidGenerationSpec`.

**Step 2: Confirm red**

```bash
go test ./internal/balance -run 'TestGenerationSpec|TestEraForGen' -count=1
```

Expected: compile failure for missing catalog APIs.

**Step 3: Implement the catalog**

Add:

```go
type GenerationSpec struct {
    Gen, Era           int
    FrontierRnD        float64
    FrontierWork       float64
    TrainRnD           float64
    TrainWork          float64
    QualityScale       float64
    RecommendedCompute float64
    TimeBaselineDay    float64
}

func EraForGen(gen int) (int, error)
func EraStartGen(era int) (int, error)
func EraEndGen(era int) (int, error)
func Generation(gen int) (GenerationSpec, error)
```

Implementation requirements:

- Gen1–5 derive training values from current approved values before legacy arrays are removed in Task 3.
- Gen6–10 use the design table and `work = recommended × targetRealSeconds × RealSecCompression`.
- Gen11+ use the exact design formulas.
- Use checked powers plus `math.IsNaN` / `math.IsInf` before returning.

**Step 4: Verify green**

```bash
gofmt -w internal/balance/generation.go internal/balance/generation_test.go internal/balance/balance.go internal/balance/balance_test.go
go test ./internal/balance -run 'TestGenerationSpec|TestEraForGen|TestDefault' -count=1
```

**Step 5: Commit**

```bash
git add internal/balance/generation.go internal/balance/generation_test.go internal/balance/balance.go internal/balance/balance_test.go
git commit -m "feat(balance): add generation and era catalog"
```

## Task 3: Move training and legacy generation gates onto the catalog

**Files:**

- Modify: `internal/sim/tech.go`
- Modify: `internal/sim/tech_test.go`
- Modify: `internal/sim/apply.go`
- Modify: `internal/sim/apply_test.go`
- Modify: `internal/sim/sim.go`
- Modify: `internal/sim/sim_test.go`
- Modify: `internal/balance/balance.go`
- Modify: `internal/balance/balance_test.go`

**Step 1: Write failing compatibility and Gen6 tests**

Assert:

- zero-value legacy state still allows Gen1;
- contiguous legacy `model-gen-2` through `model-gen-5` nodes are reflected in `MaxUnlockedGen`;
- applying a Gen2–5 `UnlockTech` updates `Progression.MaxUnlockedGen` atomically;
- `Progression.MaxUnlockedGen = 6` allows Gen6 training;
- Gen6 training deducts `Generation(6).TrainRnD`, snapshots `TrainWork`, and completes with `QualityScale`;
- invalid generation specs return the typed generation error without mutation.

**Step 2: Confirm red**

```bash
go test ./internal/sim -run 'TestMaxUnlockedGen|TestStartTrainingGen6|TestTrainingGen6Quality|TestUnlockTechUpdatesProgression' -count=1
```

**Step 3: Implement catalog-backed training**

- Reconcile `Progression.MaxUnlockedGen` with contiguous legacy generation nodes; always return at least 1.
- Update progression when `applyUnlockTech` succeeds for `model-gen-N`.
- Resolve `balance.Generation(c.Gen)` in `applyStartTraining` and training completion.
- Remove `MaxGen`, `GenRnDCost`, `GenTrainWorkGPUSec`, and `GenQualityCap` after call sites move.

Check remaining use:

```bash
rg -n 'MaxGen|GenRnDCost|GenTrainWorkGPUSec|GenQualityCap' --glob '*.go'
```

Expected: only explicit legacy migration fixtures, or no matches.

**Step 4: Verify regressions**

```bash
gofmt -w internal/sim/tech.go internal/sim/tech_test.go internal/sim/apply.go internal/sim/apply_test.go internal/sim/sim.go internal/sim/sim_test.go internal/balance/balance.go internal/balance/balance_test.go
go test ./internal/balance ./internal/sim -run 'TestMaxUnlockedGen|TestStartTraining|TestTraining|TestTechQuality' -count=1
```

**Step 5: Commit**

```bash
git add internal/balance internal/sim
git commit -m "refactor(sim): resolve model generations from catalog"
```

## Task 4: Implement era breakthroughs and diminishing effects

**Files:**

- Create: `internal/sim/era.go`
- Create: `internal/sim/era_test.go`
- Modify: `internal/sim/tech.go`
- Modify: `internal/sim/tech_test.go`
- Modify: `internal/sim/apply.go`
- Modify: `internal/sim/apply_test.go`

**Step 1: Write failing era tests**

Cover:

- Era III base cost is `0.25 × Generation(6).FrontierRnD`;
- first branch costs `1.0×`, records `Primary`, and flips its mask bit;
- later branches cost `1.75×`;
- duplicate, invalid branch, unopened era, and insufficient R&D errors are exact and pure;
- Era IV requires Gen7 plus two Era III breakthroughs;
- aggregate effects use approved square-root formulas and are neutral with no era progress;
- algorithms/alignment affect newly trained quality only; infra/business affect runtime helpers.

**Step 2: Confirm red**

```bash
go test ./internal/sim -run 'TestEra|TestApplyUnlockEraBreakthrough' -count=1
```

**Step 3: Implement minimum era engine**

Add:

```go
func EraBreakthroughCost(s model.GameState, era int, branch model.TechBranch) (float64, error)
func EraOpen(s model.GameState, era int) bool
func EraEffects(s model.GameState) model.TechEffects
```

- Eras I–II remain fixed-tech history; generated breakthroughs begin at Era III.
- Keep `Progression.Eras` sorted.
- Use bit `1 << branch` for `UnlockedMask`.
- Aggregate era effects once into `techEffects`.
- Route `UnlockEraBreakthrough` through `sim.Apply` with typed errors.

**Step 4: Verify**

```bash
gofmt -w internal/sim/era.go internal/sim/era_test.go internal/sim/tech.go internal/sim/tech_test.go internal/sim/apply.go internal/sim/apply_test.go
go test ./internal/sim -run 'TestEra|TestApplyUnlockEraBreakthrough|TestTechEffects|TestTechQuality|TestTechInfra|TestTechGrowth' -count=1
```

**Step 5: Commit**

```bash
git add internal/sim/era.go internal/sim/era_test.go internal/sim/tech.go internal/sim/tech_test.go internal/sim/apply.go internal/sim/apply_test.go
git commit -m "feat(sim): add repeatable era breakthroughs"
```

## Task 5: Add frontier project command lifecycle

**Files:**

- Create: `internal/sim/frontier_project.go`
- Create: `internal/sim/frontier_project_test.go`
- Modify: `internal/sim/apply.go`
- Modify: `internal/sim/apply_test.go`

**Step 1: Write failing command tests**

Test:

- Gen5 starts only Gen6;
- later targets obey era gates;
- start snapshots totals, remaining values, and recommended compute;
- a second project, wrong target, or invalid generation fails without mutation;
- allocation accepts `0`, `10`, `100`; rejects `-1`, `101`; preserves progress;
- allocation without an active project returns `ErrNoFrontierProject`.

Use `AllocationPct = 100` as the default for a new project.

**Step 2: Confirm red**

```bash
go test ./internal/sim -run 'TestApplyStartFrontierProject|TestApplySetFrontierAllocation' -count=1
```

**Step 3: Implement handlers**

```go
func applyStartFrontierProject(s model.GameState, c model.StartFrontierProject, b balance.Config) (model.GameState, error)
func applySetFrontierAllocation(s model.GameState, c model.SetFrontierAllocation) (model.GameState, error)
```

Resolve and snapshot `Generation(TargetGen)` only after target and era validation.

**Step 4: Verify**

```bash
gofmt -w internal/sim/frontier_project.go internal/sim/frontier_project_test.go internal/sim/apply.go internal/sim/apply_test.go
go test ./internal/sim -run 'TestApplyStartFrontierProject|TestApplySetFrontierAllocation|TestApplyStartTraining' -count=1
```

**Step 5: Commit**

```bash
git add internal/sim/frontier_project.go internal/sim/frontier_project_test.go internal/sim/apply.go internal/sim/apply_test.go
git commit -m "feat(sim): add frontier research commands"
```

## Task 6: Share training compute and stream frontier R&D

**Files:**

- Modify: `internal/sim/frontier_project.go`
- Modify: `internal/sim/frontier_project_test.go`
- Modify: `internal/sim/sim.go`
- Modify: `internal/sim/sim_test.go`

**Step 1: Write failing Tick tests**

Cover:

- diminishing helper is linear through recommendation and square-root above it;
- 60% frontier allocation leaves 40% for model training;
- insufficient R&D advances neither frontier work nor frontier R&D;
- proportional spending preserves the remaining R&D/work ratio;
- completion sets max generation, clears the project, and never unlocks twice;
- allocation changes preserve totals;
- nested input slices remain unmodified.

**Step 2: Confirm red**

```bash
go test ./internal/sim -run 'TestDiminishedFrontierCompute|TestTickSharesTrainingCompute|TestFrontierProject' -count=1
```

**Step 3: Implement shared progression**

Add:

```go
func diminishedFrontierCompute(allocated, recommended float64) float64
func advanceFrontierProject(s model.GameState, dt, allocated float64) model.GameState
```

Refactor training progression to accept allocated compute. Tick order:

1. accrue R&D;
2. calculate effective training compute once;
3. split by frontier allocation;
4. advance frontier with streamed R&D;
5. advance model training with the remainder.

Never auto-redirect idle or stalled shares.

**Step 4: Verify regressions**

```bash
gofmt -w internal/sim/frontier_project.go internal/sim/frontier_project_test.go internal/sim/sim.go internal/sim/sim_test.go
go test ./internal/sim -run 'TestDiminishedFrontierCompute|TestTickSharesTrainingCompute|TestFrontierProject|TestEffectiveTraining|TestTickCompletesTraining' -count=1
```

**Step 5: Commit**

```bash
git add internal/sim/frontier_project.go internal/sim/frontier_project_test.go internal/sim/sim.go internal/sim/sim_test.go
git commit -m "feat(sim): share compute with frontier research"
```

## Task 7: Add progression views and long-run calibration

**Files:**

- Create: `internal/sim/progression_view.go`
- Create: `internal/sim/progression_view_test.go`
- Create: `internal/sim/longrun_test.go`

**Step 1: Write failing tests**

Define a `ProgressionView` contract containing target gen/era, R&D/work fractions, allocation split, allocated/diminished compute, recommendation, ETA, and an unavailable reason (`no-compute`, `no-rnd`, `paused`, or empty).

Add the reference fixture: day 7,000, Gen5, sufficient R&D, no optional multipliers, recommended compute, then frontier and training sequentially. Assert Gen8–10 at day 20,000 and Gen10–13 at days 43,200–72,000.

**Step 2: Confirm red**

```bash
go test ./internal/sim -run 'TestProgressionView|TestLongRunCalibration' -count=1
```

**Step 3: Implement pure views**

```go
type ProgressionView struct { /* approved fields */ }
func FrontierProgressView(s model.GameState, b balance.Config) ProgressionView
```

Use project snapshots, not re-resolved totals, for percentages and ETA.

**Step 4: Verify**

```bash
gofmt -w internal/sim/progression_view.go internal/sim/progression_view_test.go internal/sim/longrun_test.go
go test ./internal/sim -run 'TestProgressionView|TestLongRunCalibration' -count=1
```

**Step 5: Commit**

```bash
git add internal/sim/progression_view.go internal/sim/progression_view_test.go internal/sim/longrun_test.go
git commit -m "feat(sim): expose long-run progression views"
```

## Task 8: Implement global frontier and stable model-relative views

**Files:**

- Create: `internal/sim/frontier.go`
- Create: `internal/sim/frontier_test.go`
- Modify: `internal/sim/view.go`
- Modify: `internal/sim/view_test.go`
- Modify: `internal/sim/sim.go`

**Step 1: Write failing frontier tests**

Cover best online player frontier, time interpolation, baseline scaling, per-dimension max, online industry clock advancement, stable stored model quality, equivalent-generation gap, and zero-frontier safety.

**Step 2: Confirm red**

```bash
go test ./internal/sim -run 'TestTimeFrontier|TestGlobalFrontier|TestModelFrontierView|TestTickAdvancesIndustryTime' -count=1
```

**Step 3: Implement pure frontier APIs**

```go
func PlayerFrontier(s model.GameState) [model.NumQualityDims]float64
func TimeFrontier(s model.GameState, b balance.Config) [model.NumQualityDims]float64
func GlobalFrontier(s model.GameState, b balance.Config) [model.NumQualityDims]float64
func ModelFrontierView(s model.GameState, index int, b balance.Config) ModelFrontier
```

Replace private `playerFrontier`. Online Tick adds `dt` to `Progression.IndustryTime`. v0 migration will initialize it from `GameTime`.

**Step 4: Verify**

```bash
gofmt -w internal/sim/frontier.go internal/sim/frontier_test.go internal/sim/view.go internal/sim/view_test.go internal/sim/sim.go
go test ./internal/sim -run 'TestTimeFrontier|TestGlobalFrontier|TestModelFrontierView|TestTickAdvancesIndustryTime' -count=1
```

**Step 5: Commit**

```bash
git add internal/sim/frontier.go internal/sim/frontier_test.go internal/sim/view.go internal/sim/view_test.go internal/sim/sim.go
git commit -m "feat(sim): add global frontier views"
```

## Task 9: Replace rival paths with one bounded league engine

**Files:**

- Create: `internal/sim/rivals.go`
- Create: `internal/sim/rivals_test.go`
- Modify: `internal/sim/sim.go`
- Modify: `internal/sim/sim_test.go`
- Modify: `internal/balance/balance.go`
- Modify: `internal/balance/balance_test.go`

**Step 1: Write failing league tests**

Test default specialties in `0.92–1.08`, campaign tracking no longer frozen, per-dimension `85%–115%` invariant, deterministic 2–3 leader selection, persisted leaders surviving unrelated RNG changes, and era transition clearing momentum.

**Step 2: Confirm red**

```bash
go test ./internal/balance ./internal/sim -run 'TestDefaultCompetitorSpecialties|TestRivalLeague|TestRivalEraLeaders' -count=1
```

**Step 3: Implement bounded league**

```go
const rivalFloorPct = 0.85
const rivalCeilPct = 1.15
const leaderBonusPct = 0.04

func ensureRivalEraState(s model.GameState, b balance.Config) model.GameState
func rivalTarget(s model.GameState, rival model.Competitor, b balance.Config) [model.NumQualityDims]float64
func advanceRivalLeague(s model.GameState, dt float64, b balance.Config) model.GameState
```

Use existing SplitMix64, persist leaders/RNG, remove campaign freeze, calibrate default skills, and clamp after every approach. Select without replacement using each company's strongest specialty as its weight; choose `2 + era%2` leaders so even eras have two and odd eras have three.

**Step 4: Verify**

```bash
gofmt -w internal/sim/rivals.go internal/sim/rivals_test.go internal/sim/sim.go internal/sim/sim_test.go internal/balance/balance.go internal/balance/balance_test.go
go test ./internal/balance ./internal/sim -run 'TestDefaultCompetitor|TestRivalLeague|TestRivalEraLeaders|TestTickAdvancesCompetitors|TestMarketRank' -count=1
```

**Step 5: Commit**

```bash
git add internal/balance internal/sim/rivals.go internal/sim/rivals_test.go internal/sim/sim.go internal/sim/sim_test.go
git commit -m "feat(sim): bound rivals to global frontier"
```

## Task 10: Convert roadmap multipliers into bounded momentum

**Files:**

- Modify: `internal/balance/campaign.go`
- Modify: `internal/balance/campaign_test.go`
- Modify: `internal/model/campaign.go`
- Modify: `internal/sim/campaign_rivals.go`
- Modify: `internal/sim/campaign_rivals_test.go`
- Modify: `internal/sim/campaign_apply_test.go`
- Modify: `internal/sim/campaign_cycle.go`
- Modify: `internal/sim/campaign_cycle_test.go`
- Modify: `internal/sim/campaign_invariant_test.go`

**Step 1: Write runaway regression first**

Run repeated OpenAI flagship/platform actions for at least 100,000 cycles and assert every rival dimension remains inside `frontier × [0.85, 1.15]`. Also test counter impact, reports, price duration, linear momentum decay, and era reset.

**Step 2: Confirm red**

```bash
go test ./internal/sim -run 'TestRoadmapNeverCompoundsBeyondFrontier|TestExecuteRivalAction|TestMomentum' -count=1
```

Expected: old `quality *= 1 + pct` fails the invariant.

**Step 3: Change action contract**

Replace `QualityPct` with:

```go
FrontierProgress [model.NumQualityDims]float64
MomentumCycles   int
```

A due action closes a bounded fraction of remaining target distance, preserves counter/price/report behavior, and never directly multiplies quality. Set momentum exactly as:

```go
nextMomentum := action.FrontierProgress[d] * 0.25 * impact
competitor.MomentumPct[d] = math.Min(0.07, math.Max(competitor.MomentumPct[d], nextMomentum))
competitor.MomentumCycles = action.MomentumCycles
```

Age momentum in `AdvanceCampaignCycle` using:

```go
pct *= float64(cycles-1) / float64(cycles)
cycles--
```

**Step 4: Verify campaign regressions**

```bash
gofmt -w internal/balance/campaign.go internal/balance/campaign_test.go internal/model/campaign.go internal/sim/campaign_rivals.go internal/sim/campaign_rivals_test.go internal/sim/campaign_apply_test.go internal/sim/campaign_cycle.go internal/sim/campaign_cycle_test.go internal/sim/campaign_invariant_test.go
go test ./internal/sim ./internal/balance -run 'TestRoadmap|TestExecuteRivalAction|TestCampaignCompetitors|TestMomentum|TestCampaignInvariant' -count=1
```

**Step 5: Commit**

```bash
git add internal/model/campaign.go internal/balance/campaign.go internal/balance/campaign_test.go internal/sim/campaign_rivals.go internal/sim/campaign_rivals_test.go internal/sim/campaign_apply_test.go internal/sim/campaign_cycle.go internal/sim/campaign_cycle_test.go internal/sim/campaign_invariant_test.go
git commit -m "fix(sim): replace rival quality compounding"
```

## Task 11: Give offline industry progress its own capped clock

**Files:**

- Modify: `internal/sim/sim.go`
- Modify: `internal/sim/frontier.go`
- Modify: `internal/sim/frontier_test.go`
- Modify: `internal/tui/settle.go`
- Modify: `internal/tui/settle_test.go`

**Step 1: Write failing dual-clock tests**

Cover online same-delta behavior, existing economy settlement, industry advance capped by both eight real hours and one time-baseline generation, rival approach using industry delta, and dropped backlog not replaying.

**Step 2: Confirm red**

```bash
go test ./internal/sim ./internal/tui -run 'TestTickWithClocks|TestSettleCapsIndustryFrontier|TestSettleDropsIndustryBacklog' -count=1
```

**Step 3: Implement pure dual clocks**

```go
func Tick(s model.GameState, dt float64, events []model.TokenEvent, b balance.Config) model.GameState {
    return tickWithClocks(s, dt, dt, events, b)
}

func tickWithClocks(s model.GameState, economyDT, industryDT float64, events []model.TokenEvent, b balance.Config) model.GameState
```

Expose only a narrow offline wrapper for `tui.Settle`. Add `SecondsUntilNextTimeGeneration` in `frontier.go`. Distribute allowed industry delta across existing settle chunks while economy uses existing elapsed semantics.

**Step 4: Verify**

```bash
gofmt -w internal/sim/sim.go internal/sim/frontier.go internal/sim/frontier_test.go internal/tui/settle.go internal/tui/settle_test.go
go test ./internal/sim ./internal/tui -run 'TestTickWithClocks|TestSettle|TestOffline' -count=1
```

**Step 5: Commit**

```bash
git add internal/sim/sim.go internal/sim/frontier.go internal/sim/frontier_test.go internal/tui/settle.go internal/tui/settle_test.go
git commit -m "feat(sim): cap offline industry progression"
```

## Task 12: Add a versioned save envelope

**Files:**

- Modify: `internal/store/store.go`
- Modify: `internal/store/store_test.go`

**Step 1: Write failing envelope tests**

Test top-level `schemaVersion`/`state`, new round-trip, legacy unwrapped detection, missing file, and corrupt bytes remaining untouched.

**Step 2: Confirm red**

```bash
go test ./internal/store -run 'TestSaveEnvelope|TestLoadLegacyShape|TestLoadCorruptUntouched' -count=1
```

**Step 3: Implement detection**

```go
const CurrentSchemaVersion = 1

type SaveFile struct {
    SchemaVersion int             `json:"schemaVersion"`
    State         model.GameState `json:"state"`
}
```

Probe raw JSON for `schemaVersion`; absent means legacy `GameState`. Preserve atomic temp-file rename.

**Step 4: Verify**

```bash
gofmt -w internal/store/store.go internal/store/store_test.go
go test ./internal/store -run 'TestSave|TestLoad' -count=1
```

**Step 5: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go
git commit -m "feat(store): version save files"
```

## Task 13: Implement safe v0 migration

**Files:**

- Create: `internal/store/migrate.go`
- Create: `internal/store/migrate_test.go`
- Modify: `internal/store/store.go`
- Modify: `internal/store/store_test.go`

**Step 1: Write failing migration tests**

Fixtures cover inferred max generation, active training, `IndustryTime = GameTime`, per-dimension rival rank mapping to `88/92/96/100/104/109/114%`, skill clamp with strongest dimension preserved, zero momentum, initialized leaders, one-time backup, finite/non-negative validation, idempotence, and failure preserving original bytes.

**Step 2: Confirm red**

```bash
go test ./internal/store -run 'TestMigrateV0|TestMigrationBackup|TestMigrationIdempotent|TestMigrationFailurePreservesOriginal' -count=1
```

**Step 3: Implement migration with explicit config**

Expose an explicit-config entry point for migration tests while retaining the current caller API:

```go
func Load(path string) (model.GameState, bool, error) {
    return LoadWithConfig(path, balance.Default())
}
func LoadWithConfig(path string, b balance.Config) (model.GameState, bool, error)
func migrateV0(s model.GameState, b balance.Config) (model.GameState, error)
func validateState(s model.GameState, b balance.Config) error
```

Use sim frontier helpers; this import is acyclic. Back up before writing, and never overwrite an existing v0 backup.

**Step 4: Verify**

```bash
gofmt -w internal/store/migrate.go internal/store/migrate_test.go internal/store/store.go internal/store/store_test.go
go test ./internal/store -run 'TestMigrate|TestMigration|TestSave|TestLoad' -count=1
```

**Step 5: Commit**

```bash
git add internal/store
git commit -m "feat(store): migrate legacy progression saves"
```

## Task 14: Block writes after load or migration failure

**Files:**

- Modify: `internal/tui/tui.go`
- Modify: `internal/tui/tui_test.go`
- Modify: `internal/tui/daemon_integration_test.go`

**Step 1: Replace old corrupt-save test**

Assert original path remains, no `.corrupt` rename occurs, Model records startup error and disables autosave, View shows a blocking recovery message with path/error, and `q` exits without overwriting the source.

**Step 2: Confirm red**

```bash
go test ./internal/tui -run 'TestNewAtPreservesFailedSave|TestLoadFailureDisablesAutosave' -count=1
```

**Step 3: Implement failure state**

Add:

```go
startupErr   error
saveDisabled bool
```

When load fails, do not seed RNG, settle offline, write meta, or write state. Permit only quit and resize.

**Step 4: Verify**

```bash
gofmt -w internal/tui/tui.go internal/tui/tui_test.go internal/tui/daemon_integration_test.go
go test ./internal/tui -run 'TestNew|TestLoad|TestQuit|TestDaemon' -count=1
```

**Step 5: Commit**

```bash
git add internal/tui/tui.go internal/tui/tui_test.go internal/tui/daemon_integration_test.go
git commit -m "fix(tui): block writes after save load failure"
```

## Task 15: Rebuild technology page as an era ladder

**Files:**

- Modify: `internal/tui/tui.go`
- Modify: `internal/tui/page_tech.go`
- Modify: `internal/tui/page_tech_test.go`
- Modify: `internal/tui/tech_meta.go`
- Modify: `internal/tui/tech_meta_test.go`

**Step 1: Write failing render/interaction tests**

Cover fixed Eras I–II, expanded current era, collapsed past, one-level next preview, generated labels, `[`/`]` era navigation, visible-entry cursor, `Enter` dispatch by entry kind, `+`/`-` allocation in 10-point steps, help text, and narrow width.

**Step 2: Confirm red**

```bash
go test ./internal/tui -run 'TestTechEra|TestTechCursor|TestTechStartsFrontier|TestTechAdjustsAllocation|TestTechPageFits' -count=1
```

**Step 3: Implement TUI-only entries**

Use a discriminated adapter, not ID parsing:

```go
type techEntryKind int
const (
    techEntryFixed techEntryKind = iota
    techEntryGeneration
    techEntryBreakthrough
)

type techEntry struct { /* kind plus fixed index/target gen/era/branch */ }
```

Store selected era and visible-entry cursor in `tui.Model`. Keep existing Chinese fixed-node labels; generate late-game labels from catalog data.

**Step 4: Verify**

```bash
gofmt -w internal/tui/tui.go internal/tui/page_tech.go internal/tui/page_tech_test.go internal/tui/tech_meta.go internal/tui/tech_meta_test.go
go test ./internal/tui -run 'TestTech' -count=1
```

**Step 5: Commit**

```bash
git add internal/tui/tui.go internal/tui/page_tech.go internal/tui/page_tech_test.go internal/tui/tech_meta.go internal/tui/tech_meta_test.go
git commit -m "feat(tui): render technology eras"
```

## Task 16: Integrate progression into overview and compute pages

**Files:**

- Modify: `internal/tui/page_overview.go`
- Modify: `internal/tui/page_overview_test.go`
- Modify: `internal/tui/page_compute.go`
- Modify: `internal/tui/page_compute_test.go`
- Modify: `internal/tui/pressure_test.go`
- Modify: `internal/tui/tui.go`

**Step 1: Write failing render tests**

Assert concurrent frontier/training progress, split percentages, effective and recommended compute, diminished result, ETA, persistent R&D stall copy, no per-Tick toast spam, and content-width safety.

**Step 2: Confirm red**

```bash
go test ./internal/tui -run 'TestOverviewShowsFrontier|TestComputeShowsAllocation|TestFrontierStallPressure|Test.*Fits' -count=1
```

**Step 3: Render exported sim views only**

Call `sim.FrontierProgressView`; do not duplicate balance math in TUI. Preserve process rental/build keys and existing cards.

**Step 4: Verify**

```bash
gofmt -w internal/tui/page_overview.go internal/tui/page_overview_test.go internal/tui/page_compute.go internal/tui/page_compute_test.go internal/tui/pressure_test.go internal/tui/tui.go
go test ./internal/tui -run 'TestOverview|TestCompute|TestFrontierStallPressure|TestPageBodyFitsViewportWidth' -count=1
```

**Step 5: Commit**

```bash
git add internal/tui/page_overview.go internal/tui/page_overview_test.go internal/tui/page_compute.go internal/tui/page_compute_test.go internal/tui/pressure_test.go internal/tui/tui.go
git commit -m "feat(tui): show frontier research allocation"
```

## Task 17: Add model obsolescence and rival market views

**Files:**

- Modify: `internal/sim/view.go`
- Modify: `internal/sim/view_test.go`
- Modify: `internal/tui/page_models.go`
- Modify: `internal/tui/page_models_test.go`
- Modify: `internal/tui/page_market.go`
- Modify: `internal/tui/page_market_test.go`
- Modify: `internal/tui/campaign_meta.go`
- Modify: `internal/tui/campaign_meta_test.go`

**Step 1: Write failing tests**

Cover stable model absolute quality, changing relative delta, equivalent-generation safety, rival leader/delta/specialty/momentum views, global frontier/era text, band explanation, and market-effect duration.

**Step 2: Confirm red**

```bash
go test ./internal/sim ./internal/tui -run 'TestModelFrontierView|TestRivalFrontierView|TestModelShowsObsolescence|TestMarketShowsFrontier' -count=1
```

**Step 3: Implement view structs and pages**

Add one exported rival view helper. Keep ranking/appeal calculations unchanged. Resolve model bar scale with `balance.Generation`; never normalize stored model values.

**Step 4: Verify**

```bash
gofmt -w internal/sim/view.go internal/sim/view_test.go internal/tui/page_models.go internal/tui/page_models_test.go internal/tui/page_market.go internal/tui/page_market_test.go internal/tui/campaign_meta.go internal/tui/campaign_meta_test.go
go test ./internal/sim ./internal/tui -run 'TestModelFrontierView|TestRivalFrontierView|TestModel|TestMarket|TestCampaignMeta' -count=1
```

**Step 5: Commit**

```bash
git add internal/sim/view.go internal/sim/view_test.go internal/tui/page_models.go internal/tui/page_models_test.go internal/tui/page_market.go internal/tui/page_market_test.go internal/tui/campaign_meta.go internal/tui/campaign_meta_test.go
git commit -m "feat(tui): show model and rival frontier position"
```

## Task 18: Verify prestige resets and full-system invariants

**Files:**

- Modify: `internal/sim/prestige_test.go`
- Modify: `internal/sim/campaign_scenario_test.go`
- Modify: `internal/sim/campaign_invariant_test.go`
- Modify: `internal/tui/restart_test.go`
- Modify: `internal/store/store_test.go`
- Modify: `internal/store/migrate_test.go`

**Step 1: Add cross-feature tests**

Assert:

- restart/prestige clears progression, era choices, active frontier work, and industry time;
- patents, permanent nodes, badges, and legacy behavior remain;
- Gen10/Gen11 never force campaign victory, prestige, or Tick stop;
- migrated exploded save can Tick, run campaign cycles, save/reload, and remain bounded;
- model quality survives save/load and later frontier movement unchanged.

**Step 2: Run new tests**

```bash
go test ./internal/sim ./internal/store ./internal/tui -run 'TestPrestigeClearsProgression|TestNoForcedGenerationEnding|TestMigratedExplodedSavePlayable|TestModelQualityStableAcrossReload' -count=1
```

Expected before final fixes: any missed reset, clone, migration, or order assertion fails.

**Step 3: Make only minimum integration fixes**

Do not add features. Fix only state reset, cloning, validation, or call ordering revealed by tests.

**Step 4: Full verification**

```bash
gofmt -w $(git diff --name-only --diff-filter=ACM 0227597 -- '*.go')
git diff --check
go test ./...
go vet ./...
go build ./...
go test ./internal/sim -run 'TestLongRunCalibration|TestRoadmapNeverCompoundsBeyondFrontier' -count=1 -v
```

Expected: every command exits 0; calibration stays in approved bands; 100,000-cycle rivals remain bounded.

**Step 5: Commit final coverage**

```bash
git add internal/sim internal/store internal/tui
git commit -m "test: cover long-run progression invariants"
```

## Completion Checklist

- [ ] Gen1–5 compatibility tests pass.
- [ ] Gen6–10 match approved costs, durations, scales, and recommended compute.
- [ ] Gen11–100 catalog values are finite and monotonic.
- [ ] Frontier/model compute shares sum to 100%; R&D stalls are explicit.
- [ ] Day 20,000 and day 43,200–72,000 calibration bands pass.
- [ ] Campaign and non-campaign rivals remain within `85%–115%`.
- [ ] Repeated OpenAI roadmap actions cannot compound quality.
- [ ] Offline industry advancement obeys both caps.
- [ ] Existing saves are backed up, migrated once, and never silently overwritten on failure.
- [ ] TUI era ladder, allocation, obsolescence, and market views fit supported widths.
- [ ] Prestige remains optional and clears run-scoped progression only.
- [ ] `git diff --check`, `go test ./...`, `go vet ./...`, and `go build ./...` all pass.
