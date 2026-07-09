# Industry Events System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the industry-events system (design `docs/superpowers/specs/2026-07-10-industry-events-design.md`) into the pure sim: seeded deterministic RNG in `GameState`, timed `ActiveModifier` effects, pending-choice events with conservative timeout defaults, a 10-event catalog, and an overview card + choice dialog in the TUI.

**Architecture:** All randomness flows through `Events.RandState` (splitmix64) so the sim stays deterministic (same seed + same dt sequence + same commands → same result). `Tick` gains an `advanceEvents` step (expiry → auto-resolve → trigger roll → fire). Effect magnitudes live in `balance`; effect *application* is a per-`EventID` switch in `sim` (design §3.3). Player choices arrive as a new `ResolveEvent` command through `sim.Apply`. TUI reads `state.Events` directly.

**Tech Stack:** Go, Bubble Tea (existing). No new dependencies.

## Global Constraints

- Sim stays pure: no wall-clock, no I/O, no non-deterministic randomness (`internal/sim/sim.go` header comment must be updated to say exactly that).
- Timeout/auto-resolve must always pick the **free** choice — never spend player cash/R&D automatically (design §4).
- Same event never re-fires while it is in `Pending` or `Active`, plus a cooldown window after it resolves.
- **Pacing translation (deviation from design §5, intentional):** the design doc wrote cadence in sim-hours without accounting for TUI time compression (`tickDT=3600` every 250ms → 1 game-day ≈ 6 real-seconds online). This plan calibrates in game-seconds to hit the *real-time feel* the design intended: trigger roll ≈ every 30 real-sec online, decision window ≈ 2 real-min, sustained effects ≈ 3 real-min. Offline settle advances 1:1 real seconds (existing behavior), so offline-fired choice events usually stay pending until the player returns — that is desired.
- All new user-facing copy is Traditional Chinese, matching existing TUI text.
- Every commit: run `gofmt -l internal/` (must print nothing) before committing.

## File Structure

| File | Responsibility |
|---|---|
| `internal/model/events.go` (create) | `EventEffects`, `ActiveModifier`, `PendingEvent`, `EventRecord`, `EventsState`, `ResolveEvent` command |
| `internal/model/types.go` (modify) | add `Events EventsState` field to `GameState` |
| `internal/balance/events.go` (create) | event ID + magnitude constants, `EventSpec`, `DefaultEvents()`, `EventByID` |
| `internal/balance/balance.go` (modify) | `Config` gains `Events`, `EventCheckSec`, `EventHitChance`, `EventCooldownSec`, `EventLogCap`; `Default()` wires them |
| `internal/sim/events.go` (create) | splitmix64 RNG, `eventEffects` aggregation, `advanceEvents` (expiry/auto-resolve/trigger/fire), `resolveChoice`, `applyResolveEvent`, `EventChoiceCost` |
| `internal/sim/sim.go` (modify) | header comment; call `advanceEvents`; consume `PowerCostMult`, `RefPriceMult`, `UserGrowthMult`, `TAMMult`, `SafetyWeightMult`, `ValuationMult` |
| `internal/sim/apply.go` (modify) | `ResolveEvent` case + new errors; `BuildCostMult` in `applyBuildServer`; branch `TechCostMult` in `applyUnlockTech` |
| `internal/sim/prestige.go` (modify) | `Restart` / `applyPrestigeReset` carry `RandState` across runs |
| `internal/sim/view.go` (modify) | `EffectiveRefPrice` / `EstimateUserTarget` mirror the new multipliers |
| `internal/tui/event_meta.go` (create) | Chinese names/descriptions/choice labels per event ID (pattern: `tech_meta.go`) |
| `internal/tui/dialog_event.go` (create) | pending-event choice dialog (pattern: `dialog_publish.go`) |
| `internal/tui/page_overview.go` (modify) | 「產業動態」card |
| `internal/tui/tui.go` (modify) | `e` key on overview, dialog routing, fire notice, footer hints, seed `RandState` on load |
| `internal/tui/settle.go` (modify) | `Summary` gains `EventsFired` / `EventsAutoResolved` |

Choice convention used everywhere: **`Choices` index 0 = the paid/active option, index 1 = the free/passive option, `DefaultChoice = 1`** for every 2-choice event. No-choice events have `NumChoices = 0`.

---

### Task 1: Event types in `internal/model`

**Files:**
- Create: `internal/model/events.go`
- Create: `internal/model/events_test.go`
- Modify: `internal/model/types.go` (GameState — add one field)

**Interfaces:**
- Produces: `model.EventEffects` (9 float64 mult fields), `model.NeutralEventEffects()`, `model.ActiveModifier{EventID string; ExpiresAt float64; Target int; Effects EventEffects}`, `model.PendingEvent{EventID string; Target int; FiredAt, Deadline float64}`, `model.EventRecord{EventID string; At float64; Choice int; Auto bool}`, `model.EventsState{RandState uint64; NextCheckAt float64; Pending []PendingEvent; Active []ActiveModifier; Log []EventRecord; FiredCount, AutoCount int}`, `model.ResolveEvent{PendingIndex, Choice int}` (implements `Command`), `GameState.Events EventsState`.

- [ ] **Step 1: Write the failing test**

Create `internal/model/events_test.go`:

```go
package model

import "testing"

func TestNeutralEventEffectsAllOne(t *testing.T) {
	e := NeutralEventEffects()
	for _, v := range []float64{
		e.BuildCostMult, e.PowerCostMult, e.RefPriceMult, e.UserGrowthMult,
		e.TechCostMult, e.TAMMult, e.ValuationMult, e.SafetyWeightMult,
		e.IncidentChanceMult,
	} {
		if v != 1.0 {
			t.Fatalf("neutral effect = %v, want 1.0", v)
		}
	}
}

func TestResolveEventIsCommand(t *testing.T) {
	var _ Command = ResolveEvent{PendingIndex: 0, Choice: 1}
}

func TestGameStateHasEventsZeroValue(t *testing.T) {
	var s GameState
	if s.Events.RandState != 0 || len(s.Events.Pending) != 0 || len(s.Events.Active) != 0 {
		t.Fatalf("zero-value EventsState should be empty, got %+v", s.Events)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run 'TestNeutralEventEffects|TestResolveEvent|TestGameStateHasEvents' -v`
Expected: FAIL to build — `undefined: NeutralEventEffects`, `undefined: ResolveEvent`

- [ ] **Step 3: Write the implementation**

Create `internal/model/events.go`:

```go
package model

// EventEffects are an industry event's sustained multiplicative modifiers;
// neutral value is 1.0 (same convention as TechEffects / StarEffects).
// TechCostMult only applies to the tech branch in ActiveModifier.Target.
type EventEffects struct {
	BuildCostMult      float64 // self-build capex (BuildServer)
	PowerCostMult      float64 // electricity price
	RefPriceMult       float64 // willingness to pay (reference price)
	UserGrowthMult     float64 // user-target growth
	TechCostMult       float64 // tech unlock cost for the targeted branch
	TAMMult            float64 // market size (SegmentTargetScale)
	ValuationMult      float64 // valuation multiple
	SafetyWeightMult   float64 // safety-dim weight in appeal (player & rivals)
	IncidentChanceMult float64 // model-incident trigger weight
}

// NeutralEventEffects returns effects that change nothing (all 1.0).
func NeutralEventEffects() EventEffects {
	return EventEffects{
		BuildCostMult: 1, PowerCostMult: 1, RefPriceMult: 1, UserGrowthMult: 1,
		TechCostMult: 1, TAMMult: 1, ValuationMult: 1, SafetyWeightMult: 1,
		IncidentChanceMult: 1,
	}
}

// ActiveModifier is a live timed event effect. Target is the event's rolled
// target index (tech branch / competitor / direction); -1 when unused.
type ActiveModifier struct {
	EventID   string
	ExpiresAt float64 // GameTime seconds; removed once GameTime passes it
	Target    int
	Effects   EventEffects
}

// PendingEvent is a fired event awaiting the player's choice. Past Deadline
// it auto-resolves to the catalog's DefaultChoice (always the free option).
type PendingEvent struct {
	EventID  string
	Target   int
	FiredAt  float64
	Deadline float64
}

// EventRecord is one line of resolved-event history (ring, capped in balance).
type EventRecord struct {
	EventID string
	At      float64
	Choice  int  // resolved choice index; 0 for no-choice events
	Auto    bool // true = timeout / offline auto-resolve
}

// EventsState is the industry-event subsystem state carried in GameState.
// RandState is the splitmix64 state; all sim randomness flows through it.
// FiredCount / AutoCount are monotonic counters for offline summaries.
type EventsState struct {
	RandState   uint64
	NextCheckAt float64
	Pending     []PendingEvent
	Active      []ActiveModifier
	Log         []EventRecord
	FiredCount  int
	AutoCount   int
}

// ResolveEvent applies the player's choice to Pending[PendingIndex].
type ResolveEvent struct {
	PendingIndex int
	Choice       int
}

func (ResolveEvent) commandMarker() {}
```

In `internal/model/types.go`, add the field to `GameState` (after `HiredStars []string`):

```go
	HiredStars        []string
	Events            EventsState
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/model/ -v`
Expected: PASS (all, including pre-existing tests)

- [ ] **Step 5: Commit**

```bash
git add internal/model/events.go internal/model/events_test.go internal/model/types.go
git commit -m "feat(model): industry-event types and ResolveEvent command"
```

---

### Task 2: Deterministic RNG in `internal/sim`

**Files:**
- Create: `internal/sim/events.go`
- Create: `internal/sim/events_test.go`

**Interfaces:**
- Produces: `nextRand(state uint64) (uint64, float64)` — advances splitmix64 state, returns new state and a uniform float64 in [0,1). Unexported; later tasks in the same package build on it.

- [ ] **Step 1: Write the failing test**

Create `internal/sim/events_test.go`:

```go
package sim

import "testing"

func TestNextRandDeterministic(t *testing.T) {
	s1, r1 := nextRand(42)
	s2, r2 := nextRand(42)
	if s1 != s2 || r1 != r2 {
		t.Fatalf("same input state must give same output: (%d,%v) vs (%d,%v)", s1, r1, s2, r2)
	}
	if s1 == 42 {
		t.Fatal("state must advance")
	}
}

func TestNextRandRangeAndSpread(t *testing.T) {
	state := uint64(7)
	var lo, hi int
	for i := 0; i < 1000; i++ {
		var r float64
		state, r = nextRand(state)
		if r < 0 || r >= 1 {
			t.Fatalf("r = %v out of [0,1)", r)
		}
		if r < 0.5 {
			lo++
		} else {
			hi++
		}
	}
	if lo < 400 || hi < 400 {
		t.Fatalf("distribution too skewed: lo=%d hi=%d", lo, hi)
	}
}

func TestNextRandZeroStateWorks(t *testing.T) {
	state, r := nextRand(0)
	if state == 0 {
		t.Fatal("state must advance from 0")
	}
	if r < 0 || r >= 1 {
		t.Fatalf("r = %v out of [0,1)", r)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/sim/ -run TestNextRand -v`
Expected: FAIL to build — `undefined: nextRand`

- [ ] **Step 3: Write the implementation**

Create `internal/sim/events.go`:

```go
package sim

// nextRand advances a splitmix64 state and returns the new state plus a
// uniform float64 in [0,1). All event randomness flows through this so the
// sim stays deterministic: same GameState → same rolls.
func nextRand(state uint64) (uint64, float64) {
	state += 0x9E3779B97F4A7C15
	z := state
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	z ^= z >> 31
	return state, float64(z>>11) / float64(1<<53)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/sim/ -run TestNextRand -v`
Expected: PASS (3 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/sim/events.go internal/sim/events_test.go
git commit -m "feat(sim): splitmix64 deterministic RNG for events"
```

---

### Task 3: Event catalog in `internal/balance`

**Files:**
- Create: `internal/balance/events.go`
- Create: `internal/balance/events_test.go`
- Modify: `internal/balance/balance.go` (Config fields + `Default()` wiring)

**Interfaces:**
- Produces: event ID constants (`EvChipShortage` … `EvBubbleTalk`), magnitude constants (`EvChipShortageBuildMult` etc.), `balance.EventSpec{ID string; Weight, MinGameTime, MinValuation, DurationSec, DeadlineSec float64; NumChoices, DefaultChoice int; CashCostRevMonths, CashCostFloor, RnDCostFrac float64}`, `DefaultEvents() []EventSpec`, `EventByID(evs []EventSpec, id string) (EventSpec, bool)`; `Config.Events []EventSpec`, `Config.EventCheckSec`, `Config.EventHitChance`, `Config.EventCooldownSec float64`, `Config.EventLogCap int`.

- [ ] **Step 1: Write the failing test**

Create `internal/balance/events_test.go`:

```go
package balance

import "testing"

func TestDefaultEventsCatalog(t *testing.T) {
	evs := DefaultEvents()
	if len(evs) != 10 {
		t.Fatalf("catalog size = %d, want 10", len(evs))
	}
	seen := map[string]bool{}
	for _, e := range evs {
		if seen[e.ID] {
			t.Fatalf("duplicate event ID %q", e.ID)
		}
		seen[e.ID] = true
		if e.Weight <= 0 {
			t.Fatalf("%s: weight must be positive", e.ID)
		}
		if e.NumChoices > 0 {
			if e.DefaultChoice != 1 {
				t.Fatalf("%s: default choice must be the free option (1), got %d", e.ID, e.DefaultChoice)
			}
			if e.DeadlineSec <= 0 {
				t.Fatalf("%s: choice events need a deadline", e.ID)
			}
		}
	}
}

func TestEventByID(t *testing.T) {
	evs := DefaultEvents()
	if _, ok := EventByID(evs, EvChipShortage); !ok {
		t.Fatal("chip-shortage should exist")
	}
	if _, ok := EventByID(evs, "nope"); ok {
		t.Fatal("unknown ID should return ok=false")
	}
}

func TestDefaultConfigWiresEvents(t *testing.T) {
	c := Default()
	if len(c.Events) != 10 || c.EventCheckSec <= 0 || c.EventHitChance <= 0 ||
		c.EventHitChance > 1 || c.EventCooldownSec <= 0 || c.EventLogCap <= 0 {
		t.Fatalf("event knobs not wired: %+v", struct {
			N                    int
			Check, Hit, Cooldown float64
			Cap                  int
		}{len(c.Events), c.EventCheckSec, c.EventHitChance, c.EventCooldownSec, c.EventLogCap})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/balance/ -run 'TestDefaultEvents|TestEventByID|TestDefaultConfigWires' -v`
Expected: FAIL to build — `undefined: DefaultEvents`

- [ ] **Step 3: Write the implementation**

Create `internal/balance/events.go`:

```go
package balance

// Industry-event IDs (design §4). The TUI keys Chinese copy off these; the
// sim keys effect application off them.
const (
	EvChipShortage  = "chip-shortage"
	EvEnergySpike   = "energy-spike"
	EvRivalBreak    = "rival-breakthrough"
	EvOpenSourceWar = "open-source-war"
	EvRivalScandal  = "rival-scandal"
	EvPaper         = "breakthrough-paper"
	EvIncident      = "model-incident"
	EvRegulation    = "regulation"
	EvMarketCycle   = "market-cycle"
	EvBubbleTalk    = "bubble-talk"
)

// Per-event effect magnitudes (v0 calibration, design §4; tune in playtest).
const (
	EvChipShortageBuildMult = 1.18
	EvEnergyUpMult          = 1.3
	EvEnergyDownMult        = 0.7
	EvRivalBreakQualityPct  = 0.15 // rival capability one-shot jump
	EvRivalBreakPromoGrowth = 1.25
	EvOpenSourceRefPrice    = 0.8
	EvOpenSourceFollowRef   = 0.75
	EvOpenSourceFollowGrow  = 1.2
	EvScandalSafetyPct      = 0.20 // rival safety one-shot drop
	EvScandalPoachGrowth    = 1.3
	EvScandalWatchGrowth    = 1.1
	EvPaperBetTechCost      = 0.5
	EvPaperAbsorbTechCost   = 0.7
	EvIncidentLossPct       = 0.08 // one-shot user loss (consumer/developer)
	EvIncidentEnterprisePct = 0.15 // one-shot user loss (enterprise)
	EvIncidentQuietChance   = 1.5  // lingering IncidentChanceMult after低調
	EvRegulationSafetyW     = 1.5
	EvRegulationComplyPct   = 0.10 // one-shot player safety-quality boost
	EvMarketBoomTAM         = 1.25
	EvMarketBustTAM         = 0.8
	EvBubbleValuation       = 0.75
	EvBubbleCalmValuation   = 0.9
	// EvIncidentSafetyRef is the online-model safety quality at which the
	// incident trigger weight reaches zero (linear ramp below it).
	EvIncidentSafetyRef = 50.0
)

// EventSpec is one industry event's tuning entry. Effect application is a
// per-ID switch in internal/sim (design §3.3); this holds the numbers.
// Choice convention: index 0 = paid/active option, index 1 = free/passive
// option; DefaultChoice is always the free option so timeouts never spend.
type EventSpec struct {
	ID            string
	Weight        float64 // base trigger weight
	MinGameTime   float64 // gate: GameTime must exceed this (0 = none)
	MinValuation  float64 // gate: PeakValuation must exceed this (0 = none)
	DurationSec   float64 // sustained-modifier length (game seconds)
	DeadlineSec   float64 // decision window for choice events
	NumChoices    int     // 0 = no player choice
	DefaultChoice int
	// Choice-0 costs, charged at resolve. Cash cost scales with revenue so
	// late-game events stay meaningful: max(Floor, RevMonths×MonthlyRevenue).
	CashCostRevMonths float64
	CashCostFloor     float64
	RnDCostFrac       float64 // fraction of current R&D (breakthrough-paper)
}

// Pacing constants (game seconds). Online the TUI advances 14400 game-sec per
// real second (tickDT=3600 / 250ms), so: 5 game-days ≈ 30 real-sec (check),
// 20 game-days ≈ 2 real-min (deadline), 30 game-days ≈ 3 real-min (duration).
const (
	evDay              = 86400.0
	evDefaultDuration  = 30 * evDay
	evDefaultDeadline  = 20 * evDay
	evMarketCycleLen   = 60 * evDay
	evRegulationMinAge = 90 * evDay
)

// DefaultEvents returns the v0 industry-event catalog (design §4).
func DefaultEvents() []EventSpec {
	twoChoice := func(id string, weight, cashRevMonths, cashFloor float64) EventSpec {
		return EventSpec{
			ID: id, Weight: weight,
			DurationSec: evDefaultDuration, DeadlineSec: evDefaultDeadline,
			NumChoices: 2, DefaultChoice: 1,
			CashCostRevMonths: cashRevMonths, CashCostFloor: cashFloor,
		}
	}
	chip := twoChoice(EvChipShortage, 1.0, 0.5, 20000)
	energy := twoChoice(EvEnergySpike, 1.0, 0.4, 15000)
	rivalBreak := twoChoice(EvRivalBreak, 0.8, 0.8, 30000)
	openSource := twoChoice(EvOpenSourceWar, 0.7, 0, 0) // choice 0 is free (跟進降價)
	scandal := twoChoice(EvRivalScandal, 0.8, 0.6, 25000)
	paper := twoChoice(EvPaper, 1.0, 0, 0)
	paper.RnDCostFrac = 0.25
	incident := twoChoice(EvIncident, 1.2, 1.0, 40000)
	regulation := twoChoice(EvRegulation, 0.6, 1.0, 50000)
	regulation.MinGameTime = evRegulationMinAge
	bubble := twoChoice(EvBubbleTalk, 0.6, 0.8, 50000)
	bubble.MinValuation = 5e8
	market := EventSpec{ID: EvMarketCycle, Weight: 0.9, DurationSec: evMarketCycleLen}
	return []EventSpec{
		chip, energy, rivalBreak, openSource, scandal,
		paper, incident, regulation, market, bubble,
	}
}

// EventByID looks up an event spec by ID within evs.
func EventByID(evs []EventSpec, id string) (EventSpec, bool) {
	for _, e := range evs {
		if e.ID == id {
			return e, true
		}
	}
	return EventSpec{}, false
}
```

In `internal/balance/balance.go`, add Config fields after `Stars []model.Star` (end of the struct):

```go
	Stars               []model.Star // star-employee roster (plan-12)
	// Industry events (industry-events plan).
	Events           []EventSpec
	EventCheckSec    float64 // mean game-seconds between trigger rolls
	EventHitChance   float64 // probability a roll fires an event
	EventCooldownSec float64 // per-event quiet window after it resolves
	EventLogCap      int     // history entries kept in EventsState.Log
```

And in `Default()`, before `return c`:

```go
	c.Events = DefaultEvents()
	c.EventCheckSec = 5 * 86400  // 5 game-days ≈ 30 real-sec online
	c.EventHitChance = 0.35      // → mean one event per ~85 real-sec online
	c.EventCooldownSec = 60 * 86400
	c.EventLogCap = 20
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/balance/ -v`
Expected: PASS (all, including pre-existing tests)

- [ ] **Step 5: Commit**

```bash
git add internal/balance/events.go internal/balance/events_test.go internal/balance/balance.go
git commit -m "feat(balance): industry-event catalog and pacing knobs"
```

---

### Task 4: Effect aggregation (`eventEffects`, `eventTechCostMult`)

**Files:**
- Modify: `internal/sim/events.go`
- Modify: `internal/sim/events_test.go`

**Interfaces:**
- Consumes: `model.EventsState`, `model.ActiveModifier`, `model.NeutralEventEffects()` (Task 1).
- Produces: `eventEffects(ns model.GameState, b balance.Config) model.EventEffects` — folds all `Events.Active` entries multiplicatively, **ignoring `TechCostMult`** (kept at 1.0 because it is branch-targeted); `eventTechCostMult(ns model.GameState, branch model.TechBranch) float64` — product of `TechCostMult` over active modifiers whose `Target == int(branch)`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/sim/events_test.go`:

```go
func TestEventEffectsNeutralWhenEmpty(t *testing.T) {
	var s model.GameState
	b := balance.Default()
	e := eventEffects(s, b)
	if e != model.NeutralEventEffects() {
		t.Fatalf("empty Active must aggregate to neutral, got %+v", e)
	}
}

func TestEventEffectsMultiplies(t *testing.T) {
	var s model.GameState
	b := balance.Default()
	m1 := model.NeutralEventEffects()
	m1.PowerCostMult = 1.3
	m1.UserGrowthMult = 1.25
	m2 := model.NeutralEventEffects()
	m2.PowerCostMult = 0.7
	s.Events.Active = []model.ActiveModifier{
		{EventID: "a", ExpiresAt: 999, Target: -1, Effects: m1},
		{EventID: "b", ExpiresAt: 999, Target: -1, Effects: m2},
	}
	e := eventEffects(s, b)
	if diff := e.PowerCostMult - 1.3*0.7; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("PowerCostMult = %v, want %v", e.PowerCostMult, 1.3*0.7)
	}
	if e.UserGrowthMult != 1.25 {
		t.Fatalf("UserGrowthMult = %v, want 1.25", e.UserGrowthMult)
	}
	if e.TechCostMult != 1.0 {
		t.Fatalf("aggregate TechCostMult must stay neutral (branch-targeted), got %v", e.TechCostMult)
	}
}

func TestEventTechCostMultBranchTargeted(t *testing.T) {
	var s model.GameState
	m := model.NeutralEventEffects()
	m.TechCostMult = 0.5
	s.Events.Active = []model.ActiveModifier{
		{EventID: "paper", ExpiresAt: 999, Target: int(model.BranchAlgo), Effects: m},
	}
	if got := eventTechCostMult(s, model.BranchAlgo); got != 0.5 {
		t.Fatalf("targeted branch mult = %v, want 0.5", got)
	}
	if got := eventTechCostMult(s, model.BranchInfra); got != 1.0 {
		t.Fatalf("untargeted branch mult = %v, want 1.0", got)
	}
}
```

Add imports to the test file if missing:

```go
import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sim/ -run 'TestEventEffects|TestEventTechCost' -v`
Expected: FAIL to build — `undefined: eventEffects`

- [ ] **Step 3: Write the implementation**

Append to `internal/sim/events.go` (add imports `tokensmith/internal/balance`, `tokensmith/internal/model`):

```go
// eventEffects folds all active event modifiers into one multiplier set
// (neutral when none). TechCostMult is branch-targeted and deliberately NOT
// aggregated here — use eventTechCostMult.
func eventEffects(ns model.GameState, b balance.Config) model.EventEffects {
	agg := model.NeutralEventEffects()
	for _, m := range ns.Events.Active {
		agg.BuildCostMult *= m.Effects.BuildCostMult
		agg.PowerCostMult *= m.Effects.PowerCostMult
		agg.RefPriceMult *= m.Effects.RefPriceMult
		agg.UserGrowthMult *= m.Effects.UserGrowthMult
		agg.TAMMult *= m.Effects.TAMMult
		agg.ValuationMult *= m.Effects.ValuationMult
		agg.SafetyWeightMult *= m.Effects.SafetyWeightMult
		agg.IncidentChanceMult *= m.Effects.IncidentChanceMult
	}
	return agg
}

// eventTechCostMult is the product of active TechCostMult modifiers that
// target the given tech branch.
func eventTechCostMult(ns model.GameState, branch model.TechBranch) float64 {
	mult := 1.0
	for _, m := range ns.Events.Active {
		if m.Effects.TechCostMult != 1 && m.Target == int(branch) {
			mult *= m.Effects.TechCostMult
		}
	}
	return mult
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/sim/ -run 'TestEventEffects|TestEventTechCost' -v`
Expected: PASS (3 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/sim/events.go internal/sim/events_test.go
git commit -m "feat(sim): aggregate active event modifiers"
```

---

### Task 5: Fire & resolve per event (`fireEvent`, `resolveChoice`)

This task implements the per-ID switches. `advanceEvents` (Task 6) and `applyResolveEvent` (Task 7) call these.

**Files:**
- Modify: `internal/sim/events.go`
- Modify: `internal/sim/events_test.go`

**Interfaces:**
- Consumes: balance constants `EvChipShortage`…`EvBubbleTalk`, magnitude constants, `EventByID` (Task 3); `nextRand` (Task 2).
- Produces:
  - `fireEvent(ns model.GameState, spec balance.EventSpec, b balance.Config) model.GameState` — rolls the target (advancing `Events.RandState`), applies fire-time effects, appends `Pending` (choice events) or resolves immediately via a Log record (no-choice events), increments `FiredCount`.
  - `resolveChoice(ns model.GameState, pendingIndex, choice int, auto bool, b balance.Config) (model.GameState, error)` — charges choice-0 costs, applies choice effects, removes the pending entry, appends a `Log` record (capped at `b.EventLogCap`), increments `AutoCount` when `auto`.
  - `EventChoiceCost(ns model.GameState, spec balance.EventSpec) (cash, rnd float64)` — exported; the TUI dialog shows these. Cash = `max(spec.CashCostFloor, spec.CashCostRevMonths × MonthlyRevenue(ns))`; rnd = `spec.RnDCostFrac × ns.Resources.RnD`.
  - `appendLog(ns, rec, cap)` helper trimming the ring.
  - Fire/resolve semantics table (source of truth for the switch):

| ID | at fire | choice 0 (paid) at resolve | choice 1 (free, default) at resolve |
|---|---|---|---|
| chip-shortage | Active `BuildCostMult 1.18` | remove that Active | keep |
| energy-spike | roll dir: up → Active `PowerCostMult 1.3` + Pending(Target=0); down → Active `0.7` + Log only (no Pending) | remove that Active | keep |
| rival-breakthrough | strongest-capability rival `Quality[DimCapability] ×(1+0.15)`; Pending(Target=rival idx) | add Active `UserGrowthMult 1.25` | nothing |
| open-source-war | Active `RefPriceMult 0.8` | replace that Active's effects with `RefPriceMult 0.75, UserGrowthMult 1.2` | keep |
| rival-scandal | weighted-by-(1−safety/100) rival `Quality[DimSafety] ×(1−0.20)`; Pending(Target=rival idx) | add Active `UserGrowthMult 1.3` | add Active `UserGrowthMult 1.1` |
| breakthrough-paper | Pending(Target=random branch 0..3), no fire effect | add Active `TechCostMult 0.5` Target=branch | add Active `TechCostMult 0.7` Target=branch |
| model-incident | Pending only (loss deferred to resolve) | online models lose 4%/7.5% users (half loss) | online models lose 8%/15% users + Active `IncidentChanceMult 1.5` |
| regulation | Active `SafetyWeightMult 1.5` | one-shot player models `Quality[DimSafety] ×1.10` | nothing |
| market-cycle | roll dir: Active `TAMMult 1.25` or `0.8` + Log only | — | — |
| bubble-talk | Active `ValuationMult 0.75` | replace that Active's effects with `ValuationMult 0.9` | keep |

New Active modifiers added **at resolve** expire at `GameTime + spec.DurationSec`; those added **at fire** expire at `FiredAt + spec.DurationSec`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/sim/events_test.go`:

```go
// eventTestState returns a state with one online model and cash, positioned
// at GameTime 0, with a fixed RNG seed for deterministic rolls.
func eventTestState() model.GameState {
	var s model.GameState
	s.Resources.Cash = 1e6
	s.Resources.RnD = 1e5
	s.Events.RandState = 12345
	s.Competitors = balance.DefaultCompetitors()
	s.Models = []model.Model{
		{Gen: 1, Segment: model.SegConsumer, Users: 10000, Price: 12, Online: true, Name: "M1",
			Quality: [model.NumQualityDims]float64{20, 15, 10, 15}},
		{Gen: 1, Segment: model.SegEnterprise, Users: 2000, Price: 180, Online: true, Name: "M2",
			Quality: [model.NumQualityDims]float64{15, 10, 20, 10}},
	}
	return s
}

func TestFireChipShortageAddsModifierAndPending(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	spec, _ := balance.EventByID(b.Events, balance.EvChipShortage)
	ns := fireEvent(s, spec, b)
	if len(ns.Events.Active) != 1 || ns.Events.Active[0].Effects.BuildCostMult != balance.EvChipShortageBuildMult {
		t.Fatalf("expected BuildCostMult modifier, got %+v", ns.Events.Active)
	}
	if len(ns.Events.Pending) != 1 || ns.Events.Pending[0].EventID != balance.EvChipShortage {
		t.Fatalf("expected pending entry, got %+v", ns.Events.Pending)
	}
	if ns.Events.Pending[0].Deadline != s.GameTime+spec.DeadlineSec {
		t.Fatalf("deadline = %v, want %v", ns.Events.Pending[0].Deadline, s.GameTime+spec.DeadlineSec)
	}
	if ns.Events.FiredCount != 1 {
		t.Fatalf("FiredCount = %d, want 1", ns.Events.FiredCount)
	}
	if len(s.Events.Active) != 0 || len(s.Events.Pending) != 0 {
		t.Fatal("fireEvent must not mutate its input state")
	}
}

func TestResolveChipShortagePaidRemovesModifier(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	spec, _ := balance.EventByID(b.Events, balance.EvChipShortage)
	s = fireEvent(s, spec, b)
	cashBefore := s.Resources.Cash
	ns, err := resolveChoice(s, 0, 0, false, b)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(ns.Events.Active) != 0 {
		t.Fatalf("paid choice must remove the modifier, got %+v", ns.Events.Active)
	}
	if len(ns.Events.Pending) != 0 {
		t.Fatal("pending entry must be consumed")
	}
	if ns.Resources.Cash >= cashBefore {
		t.Fatal("choice 0 must charge cash")
	}
	if len(ns.Events.Log) != 1 || ns.Events.Log[0].Choice != 0 || ns.Events.Log[0].Auto {
		t.Fatalf("log record wrong: %+v", ns.Events.Log)
	}
}

func TestResolveDefaultKeepsModifierAndIsFree(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	spec, _ := balance.EventByID(b.Events, balance.EvChipShortage)
	s = fireEvent(s, spec, b)
	cashBefore := s.Resources.Cash
	ns, err := resolveChoice(s, 0, 1, true, b)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(ns.Events.Active) != 1 {
		t.Fatal("default choice keeps the modifier")
	}
	if ns.Resources.Cash != cashBefore {
		t.Fatal("default choice must be free")
	}
	if ns.Events.AutoCount != 1 || !ns.Events.Log[0].Auto {
		t.Fatal("auto resolve must be recorded")
	}
}

func TestResolveInsufficientCash(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	s.Resources.Cash = 1 // below every CashCostFloor
	spec, _ := balance.EventByID(b.Events, balance.EvChipShortage)
	s = fireEvent(s, spec, b)
	if _, err := resolveChoice(s, 0, 0, false, b); err != ErrInsufficientCash {
		t.Fatalf("err = %v, want ErrInsufficientCash", err)
	}
}

func TestFireRivalBreakthroughBoostsStrongestRival(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	// Strongest capability rival in DefaultCompetitors is OpenAI (10).
	before := s.Competitors[0].Quality[model.DimCapability]
	spec, _ := balance.EventByID(b.Events, balance.EvRivalBreak)
	ns := fireEvent(s, spec, b)
	after := ns.Competitors[0].Quality[model.DimCapability]
	want := before * (1 + balance.EvRivalBreakQualityPct)
	if diff := after - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("rival capability = %v, want %v", after, want)
	}
	if ns.Events.Pending[0].Target != 0 {
		t.Fatalf("Target = %d, want 0 (OpenAI)", ns.Events.Pending[0].Target)
	}
}

func TestIncidentLossDeferredToResolve(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	spec, _ := balance.EventByID(b.Events, balance.EvIncident)
	ns := fireEvent(s, spec, b)
	if ns.Models[0].Users != s.Models[0].Users {
		t.Fatal("incident loss must not apply at fire time")
	}
	// Default (低調): full loss + lingering incident-chance modifier.
	ns2, err := resolveChoice(ns, 0, 1, false, b)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	wantConsumer := s.Models[0].Users * (1 - balance.EvIncidentLossPct)
	wantEnterprise := s.Models[1].Users * (1 - balance.EvIncidentEnterprisePct)
	if ns2.Models[0].Users != wantConsumer || ns2.Models[1].Users != wantEnterprise {
		t.Fatalf("users after quiet = %v/%v, want %v/%v",
			ns2.Models[0].Users, ns2.Models[1].Users, wantConsumer, wantEnterprise)
	}
	if len(ns2.Events.Active) != 1 || ns2.Events.Active[0].Effects.IncidentChanceMult != balance.EvIncidentQuietChance {
		t.Fatalf("expected lingering IncidentChanceMult, got %+v", ns2.Events.Active)
	}
	// Apology (choice 0): half loss, no lingering modifier.
	ns3, err := resolveChoice(ns, 0, 0, false, b)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	wantHalf := s.Models[0].Users * (1 - balance.EvIncidentLossPct/2)
	if ns3.Models[0].Users != wantHalf {
		t.Fatalf("users after apology = %v, want %v", ns3.Models[0].Users, wantHalf)
	}
	if len(ns3.Events.Active) != 0 {
		t.Fatal("apology must not leave a lingering modifier")
	}
}

func TestFirePaperTargetsBranchAndResolveDiscounts(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	spec, _ := balance.EventByID(b.Events, balance.EvPaper)
	ns := fireEvent(s, spec, b)
	branch := ns.Events.Pending[0].Target
	if branch < 0 || branch >= model.NumBranches {
		t.Fatalf("branch target = %d out of range", branch)
	}
	ns2, err := resolveChoice(ns, 0, 1, false, b)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	got := eventTechCostMult(ns2, model.TechBranch(branch))
	if got != balance.EvPaperAbsorbTechCost {
		t.Fatalf("absorb tech mult = %v, want %v", got, balance.EvPaperAbsorbTechCost)
	}
	ns3, err := resolveChoice(ns, 0, 0, false, b)
	if err != nil {
		t.Fatalf("resolve bet: %v", err)
	}
	if ns3.Resources.RnD >= ns.Resources.RnD {
		t.Fatal("bet choice must charge R&D")
	}
	if got := eventTechCostMult(ns3, model.TechBranch(branch)); got != balance.EvPaperBetTechCost {
		t.Fatalf("bet tech mult = %v, want %v", got, balance.EvPaperBetTechCost)
	}
}

func TestFireMarketCycleNoChoiceLogsDirectly(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	spec, _ := balance.EventByID(b.Events, balance.EvMarketCycle)
	ns := fireEvent(s, spec, b)
	if len(ns.Events.Pending) != 0 {
		t.Fatal("no-choice event must not create a pending entry")
	}
	if len(ns.Events.Active) != 1 || len(ns.Events.Log) != 1 {
		t.Fatalf("expected 1 active + 1 log, got %+v / %+v", ns.Events.Active, ns.Events.Log)
	}
	tam := ns.Events.Active[0].Effects.TAMMult
	if tam != balance.EvMarketBoomTAM && tam != balance.EvMarketBustTAM {
		t.Fatalf("TAMMult = %v, want boom or bust", tam)
	}
}

func TestResolveOpenSourceFollowReplacesEffects(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	spec, _ := balance.EventByID(b.Events, balance.EvOpenSourceWar)
	ns := fireEvent(s, spec, b)
	if ns.Events.Active[0].Effects.RefPriceMult != balance.EvOpenSourceRefPrice {
		t.Fatalf("fire effect = %+v", ns.Events.Active[0].Effects)
	}
	ns2, err := resolveChoice(ns, 0, 0, false, b)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	e := ns2.Events.Active[0].Effects
	if e.RefPriceMult != balance.EvOpenSourceFollowRef || e.UserGrowthMult != balance.EvOpenSourceFollowGrow {
		t.Fatalf("follow effects = %+v", e)
	}
}

func TestResolveScandalGrowthByChoice(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	spec, _ := balance.EventByID(b.Events, balance.EvRivalScandal)
	s = fireEvent(s, spec, b)
	rival := s.Events.Pending[0].Target
	if rival < 0 || rival >= len(s.Competitors) {
		t.Fatalf("scandal target = %d out of range", rival)
	}
	nsWatch, err := resolveChoice(s, 0, 1, false, b)
	if err != nil {
		t.Fatalf("resolve watch: %v", err)
	}
	if eventEffects(nsWatch, b).UserGrowthMult != balance.EvScandalWatchGrowth {
		t.Fatalf("watch growth = %v", eventEffects(nsWatch, b).UserGrowthMult)
	}
	nsPoach, err := resolveChoice(s, 0, 0, false, b)
	if err != nil {
		t.Fatalf("resolve poach: %v", err)
	}
	if eventEffects(nsPoach, b).UserGrowthMult != balance.EvScandalPoachGrowth {
		t.Fatalf("poach growth = %v", eventEffects(nsPoach, b).UserGrowthMult)
	}
}

func TestResolveRegulationComplyBoostsSafety(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	spec, _ := balance.EventByID(b.Events, balance.EvRegulation)
	s = fireEvent(s, spec, b)
	if eventEffects(s, b).SafetyWeightMult != balance.EvRegulationSafetyW {
		t.Fatal("regulation must add SafetyWeightMult at fire time")
	}
	before := s.Models[0].Quality[model.DimSafety]
	ns, err := resolveChoice(s, 0, 0, false, b)
	if err != nil {
		t.Fatalf("resolve comply: %v", err)
	}
	want := before * (1 + balance.EvRegulationComplyPct)
	if diff := ns.Models[0].Quality[model.DimSafety] - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("safety after comply = %v, want %v", ns.Models[0].Quality[model.DimSafety], want)
	}
}

func TestResolveBubbleCalmSoftensValuationHit(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	spec, _ := balance.EventByID(b.Events, balance.EvBubbleTalk)
	s = fireEvent(s, spec, b)
	if eventEffects(s, b).ValuationMult != balance.EvBubbleValuation {
		t.Fatal("bubble talk must dent valuation at fire time")
	}
	ns, err := resolveChoice(s, 0, 0, false, b)
	if err != nil {
		t.Fatalf("resolve calm: %v", err)
	}
	if eventEffects(ns, b).ValuationMult != balance.EvBubbleCalmValuation {
		t.Fatalf("calm valuation mult = %v, want %v",
			eventEffects(ns, b).ValuationMult, balance.EvBubbleCalmValuation)
	}
}

func TestResolveInvalidIndexAndChoice(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	if _, err := resolveChoice(s, 0, 1, false, b); err != ErrInvalidEventIndex {
		t.Fatalf("empty pending: err = %v, want ErrInvalidEventIndex", err)
	}
	spec, _ := balance.EventByID(b.Events, balance.EvChipShortage)
	s = fireEvent(s, spec, b)
	if _, err := resolveChoice(s, 0, 5, false, b); err != ErrInvalidEventChoice {
		t.Fatalf("bad choice: err = %v, want ErrInvalidEventChoice", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sim/ -run 'TestFire|TestResolve|TestIncident' -v`
Expected: FAIL to build — `undefined: fireEvent`, `undefined: resolveChoice`, `undefined: ErrInvalidEventIndex`

- [ ] **Step 3: Write the implementation**

Append to `internal/sim/events.go`:

```go
// EventChoiceCost returns the cash and R&D cost of a spec's paid option
// (choice 0). Cash scales with revenue so late-game events stay meaningful.
func EventChoiceCost(ns model.GameState, spec balance.EventSpec) (cash, rnd float64) {
	cash = spec.CashCostRevMonths * MonthlyRevenue(ns)
	if cash < spec.CashCostFloor {
		cash = spec.CashCostFloor
	}
	rnd = spec.RnDCostFrac * ns.Resources.RnD
	return cash, rnd
}

// appendLog appends rec to a cloned log, dropping the oldest past cap.
func appendLog(log []model.EventRecord, rec model.EventRecord, cap int) []model.EventRecord {
	out := append(append([]model.EventRecord(nil), log...), rec)
	if cap > 0 && len(out) > cap {
		out = out[len(out)-cap:]
	}
	return out
}

// addModifier appends a modifier to a cloned Active slice.
func addModifier(active []model.ActiveModifier, m model.ActiveModifier) []model.ActiveModifier {
	return append(append([]model.ActiveModifier(nil), active...), m)
}

// removeModifier drops all modifiers with the given event ID (cloned).
func removeModifier(active []model.ActiveModifier, id string) []model.ActiveModifier {
	out := make([]model.ActiveModifier, 0, len(active))
	for _, m := range active {
		if m.EventID != id {
			out = append(out, m)
		}
	}
	return out
}

// replaceModifierEffects swaps the effects of the modifier with the given ID (cloned).
func replaceModifierEffects(active []model.ActiveModifier, id string, e model.EventEffects) []model.ActiveModifier {
	out := append([]model.ActiveModifier(nil), active...)
	for i := range out {
		if out[i].EventID == id {
			out[i].Effects = e
		}
	}
	return out
}

// strongestRival returns the competitor index with the highest capability.
func strongestRival(ns model.GameState) int {
	best, idx := -1.0, -1
	for i, c := range ns.Competitors {
		if c.Quality[model.DimCapability] > best {
			best, idx = c.Quality[model.DimCapability], i
		}
	}
	return idx
}

// scaleRivalDim multiplies one quality dim of one competitor (cloned).
func scaleRivalDim(ns model.GameState, idx int, dim model.QualityDim, mult float64) model.GameState {
	if idx < 0 || idx >= len(ns.Competitors) {
		return ns
	}
	comps := append([]model.Competitor(nil), ns.Competitors...)
	comps[idx].Quality[dim] *= mult
	ns.Competitors = comps
	return ns
}

// scalePlayerUsers multiplies online-model users: entMult for enterprise
// models, mult for the rest (cloned).
func scalePlayerUsers(ns model.GameState, mult, entMult float64) model.GameState {
	models := append([]model.Model(nil), ns.Models...)
	for i := range models {
		if !models[i].Online {
			continue
		}
		if models[i].Segment == model.SegEnterprise {
			models[i].Users *= entMult
		} else {
			models[i].Users *= mult
		}
	}
	ns.Models = models
	return ns
}

// fireEvent applies a triggered event's fire-time effects: rolls its target
// (advancing RandState), applies one-shots, adds sustained modifiers, and
// either queues a pending choice or logs a no-choice event. Pure.
func fireEvent(ns model.GameState, spec balance.EventSpec, b balance.Config) model.GameState {
	now := ns.GameTime
	target := -1
	mod := func(set func(e *model.EventEffects)) model.ActiveModifier {
		e := model.NeutralEventEffects()
		set(&e)
		return model.ActiveModifier{EventID: spec.ID, ExpiresAt: now + spec.DurationSec, Target: target, Effects: e}
	}
	pending := true
	switch spec.ID {
	case balance.EvChipShortage:
		ns.Events.Active = addModifier(ns.Events.Active, mod(func(e *model.EventEffects) {
			e.BuildCostMult = balance.EvChipShortageBuildMult
		}))
	case balance.EvEnergySpike:
		var r float64
		ns.Events.RandState, r = nextRand(ns.Events.RandState)
		if r < 0.5 { // price spike: player may pay to lock the old rate
			target = 0
			ns.Events.Active = addModifier(ns.Events.Active, mod(func(e *model.EventEffects) {
				e.PowerCostMult = balance.EvEnergyUpMult
			}))
		} else { // price drop: pure upside, no decision
			target = 1
			pending = false
			ns.Events.Active = addModifier(ns.Events.Active, mod(func(e *model.EventEffects) {
				e.PowerCostMult = balance.EvEnergyDownMult
			}))
		}
	case balance.EvRivalBreak:
		target = strongestRival(ns)
		ns = scaleRivalDim(ns, target, model.DimCapability, 1+balance.EvRivalBreakQualityPct)
	case balance.EvOpenSourceWar:
		ns.Events.Active = addModifier(ns.Events.Active, mod(func(e *model.EventEffects) {
			e.RefPriceMult = balance.EvOpenSourceRefPrice
		}))
	case balance.EvRivalScandal:
		// Weighted pick by (1 - safety/100): low-safety rivals are likelier.
		weights := make([]float64, len(ns.Competitors))
		total := 0.0
		for i, c := range ns.Competitors {
			w := 1 - c.Quality[model.DimSafety]/100
			if w < 0.05 {
				w = 0.05
			}
			weights[i] = w
			total += w
		}
		var r float64
		ns.Events.RandState, r = nextRand(ns.Events.RandState)
		pick := r * total
		for i, w := range weights {
			pick -= w
			if pick <= 0 {
				target = i
				break
			}
		}
		if target < 0 && len(ns.Competitors) > 0 {
			target = len(ns.Competitors) - 1
		}
		ns = scaleRivalDim(ns, target, model.DimSafety, 1-balance.EvScandalSafetyPct)
	case balance.EvPaper:
		var r float64
		ns.Events.RandState, r = nextRand(ns.Events.RandState)
		target = int(r * float64(model.NumBranches))
		if target >= model.NumBranches {
			target = model.NumBranches - 1
		}
	case balance.EvIncident:
		// User loss is deferred to resolve so the choice governs severity.
	case balance.EvRegulation:
		ns.Events.Active = addModifier(ns.Events.Active, mod(func(e *model.EventEffects) {
			e.SafetyWeightMult = balance.EvRegulationSafetyW
		}))
	case balance.EvMarketCycle:
		pending = false
		var r float64
		ns.Events.RandState, r = nextRand(ns.Events.RandState)
		tam := balance.EvMarketBoomTAM
		target = 0
		if r < 0.5 {
			tam = balance.EvMarketBustTAM
			target = 1
		}
		ns.Events.Active = addModifier(ns.Events.Active, mod(func(e *model.EventEffects) {
			e.TAMMult = tam
		}))
	case balance.EvBubbleTalk:
		ns.Events.Active = addModifier(ns.Events.Active, mod(func(e *model.EventEffects) {
			e.ValuationMult = balance.EvBubbleValuation
		}))
	default:
		return ns // unknown ID (catalog drift): skip, never panic
	}
	ns.Events.FiredCount++
	if spec.NumChoices > 0 && pending {
		ns.Events.Pending = append(append([]model.PendingEvent(nil), ns.Events.Pending...),
			model.PendingEvent{EventID: spec.ID, Target: target, FiredAt: now, Deadline: now + spec.DeadlineSec})
	} else {
		ns.Events.Log = appendLog(ns.Events.Log,
			model.EventRecord{EventID: spec.ID, At: now, Choice: 0, Auto: false}, b.EventLogCap)
	}
	return ns
}

// resolveChoice applies choice to Pending[pendingIndex]: charges the paid
// option's costs, applies choice effects per event, consumes the pending
// entry, and records history. auto marks timeout/offline resolution. Pure.
func resolveChoice(ns model.GameState, pendingIndex, choice int, auto bool, b balance.Config) (model.GameState, error) {
	if pendingIndex < 0 || pendingIndex >= len(ns.Events.Pending) {
		return ns, ErrInvalidEventIndex
	}
	p := ns.Events.Pending[pendingIndex]
	spec, ok := balance.EventByID(b.Events, p.EventID)
	if !ok {
		// Catalog drift (save from another version): drop the entry silently.
		ns.Events.Pending = removePending(ns.Events.Pending, pendingIndex)
		return ns, nil
	}
	if choice < 0 || choice >= spec.NumChoices {
		return ns, ErrInvalidEventChoice
	}
	if choice == 0 {
		cash, rnd := EventChoiceCost(ns, spec)
		if ns.Resources.Cash < cash {
			return ns, ErrInsufficientCash
		}
		if ns.Resources.RnD < rnd {
			return ns, ErrInsufficientRnD
		}
		ns.Resources.Cash -= cash
		ns.Resources.RnD -= rnd
	}
	now := ns.GameTime
	mod := func(target int, set func(e *model.EventEffects)) model.ActiveModifier {
		e := model.NeutralEventEffects()
		set(&e)
		return model.ActiveModifier{EventID: spec.ID, ExpiresAt: now + spec.DurationSec, Target: target, Effects: e}
	}
	switch spec.ID {
	case balance.EvChipShortage, balance.EvEnergySpike:
		if choice == 0 {
			ns.Events.Active = removeModifier(ns.Events.Active, spec.ID)
		}
	case balance.EvRivalBreak:
		if choice == 0 {
			ns.Events.Active = addModifier(ns.Events.Active, mod(-1, func(e *model.EventEffects) {
				e.UserGrowthMult = balance.EvRivalBreakPromoGrowth
			}))
		}
	case balance.EvOpenSourceWar:
		if choice == 0 {
			e := model.NeutralEventEffects()
			e.RefPriceMult = balance.EvOpenSourceFollowRef
			e.UserGrowthMult = balance.EvOpenSourceFollowGrow
			ns.Events.Active = replaceModifierEffects(ns.Events.Active, spec.ID, e)
		}
	case balance.EvRivalScandal:
		growth := balance.EvScandalWatchGrowth
		if choice == 0 {
			growth = balance.EvScandalPoachGrowth
		}
		g := growth
		ns.Events.Active = addModifier(ns.Events.Active, mod(-1, func(e *model.EventEffects) {
			e.UserGrowthMult = g
		}))
	case balance.EvPaper:
		cost := balance.EvPaperAbsorbTechCost
		if choice == 0 {
			cost = balance.EvPaperBetTechCost
		}
		cm := cost
		ns.Events.Active = addModifier(ns.Events.Active, mod(p.Target, func(e *model.EventEffects) {
			e.TechCostMult = cm
		}))
	case balance.EvIncident:
		loss, entLoss := balance.EvIncidentLossPct, balance.EvIncidentEnterprisePct
		if choice == 0 { // public apology halves the loss, no aftermath
			loss, entLoss = loss/2, entLoss/2
		}
		ns = scalePlayerUsers(ns, 1-loss, 1-entLoss)
		if choice == 1 { // 低調: lingering elevated incident chance
			ns.Events.Active = addModifier(ns.Events.Active, mod(-1, func(e *model.EventEffects) {
				e.IncidentChanceMult = balance.EvIncidentQuietChance
			}))
		}
	case balance.EvRegulation:
		if choice == 0 {
			models := append([]model.Model(nil), ns.Models...)
			for i := range models {
				models[i].Quality[model.DimSafety] *= 1 + balance.EvRegulationComplyPct
			}
			ns.Models = models
		}
	case balance.EvBubbleTalk:
		if choice == 0 {
			e := model.NeutralEventEffects()
			e.ValuationMult = balance.EvBubbleCalmValuation
			ns.Events.Active = replaceModifierEffects(ns.Events.Active, spec.ID, e)
		}
	}
	ns.Events.Pending = removePending(ns.Events.Pending, pendingIndex)
	ns.Events.Log = appendLog(ns.Events.Log,
		model.EventRecord{EventID: spec.ID, At: now, Choice: choice, Auto: auto}, b.EventLogCap)
	if auto {
		ns.Events.AutoCount++
	}
	return ns, nil
}

// removePending drops index i from a cloned pending slice.
func removePending(pending []model.PendingEvent, i int) []model.PendingEvent {
	out := make([]model.PendingEvent, 0, len(pending)-1)
	out = append(out, pending[:i]...)
	return append(out, pending[i+1:]...)
}
```

Add two new errors inside the existing `var (...)` block in `internal/sim/apply.go` (after `ErrInvalidName`):

```go
	ErrInvalidEventIndex  = errors.New("sim: invalid pending-event index")
	ErrInvalidEventChoice = errors.New("sim: invalid event choice")
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/sim/ -v`
Expected: PASS (all, including pre-existing tests)

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/sim internal/balance internal/model
git add internal/sim/events.go internal/sim/events_test.go internal/sim/apply.go
git commit -m "feat(sim): fire and resolve industry events per catalog ID"
```

---

### Task 6: `advanceEvents` — expiry, auto-resolve, trigger roll

**Files:**
- Modify: `internal/sim/events.go`
- Modify: `internal/sim/events_test.go`

**Interfaces:**
- Consumes: `fireEvent`, `resolveChoice`, `nextRand`, `eventEffects` (Tasks 2/4/5); `techEffects` (existing, `internal/sim/tech.go`).
- Produces: `advanceEvents(ns model.GameState, b balance.Config) model.GameState` — the per-tick event step, called by `Tick` (Task 8). Also helpers `eligibleEvents`, `eventWeight`, `avgOnlineSafety`, `hasPending`, `hasActive`, `inCooldown`.

Trigger semantics (design §3.2): when `GameTime` crosses `NextCheckAt`, roll hit/miss with `EventHitChance`; on hit, weighted-pick one eligible event. Eligible = gates pass ∧ not in `Pending` ∧ not in `Active` ∧ past `EventCooldownSec` since its last `Log` record. `NextCheckAt == 0` (fresh run / pre-events save) just schedules the first check without rolling. Weight adjustments: `model-incident` weight = `Weight × (1 − avgOnlineSafety/EvIncidentSafetyRef) × techEffects.IncidentMult × eventEffects.IncidentChanceMult` (0 when no online models or safety ≥ ref); `breakthrough-paper` weight = `Weight × min(3, 1 + 0.1×researchers)`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/sim/events_test.go`:

```go
// onlyEvent returns a Default config whose catalog contains just the named
// event, with deterministic pacing for tests.
func onlyEvent(id string) balance.Config {
	b := balance.Default()
	spec, ok := balance.EventByID(b.Events, id)
	if !ok {
		panic("unknown test event " + id)
	}
	b.Events = []balance.EventSpec{spec}
	b.EventHitChance = 1.0 // every check fires (when eligible)
	return b
}

func TestAdvanceEventsSchedulesFirstCheckWithoutFiring(t *testing.T) {
	b := onlyEvent(balance.EvChipShortage)
	s := eventTestState()
	s.GameTime = 1000
	ns := advanceEvents(s, b)
	if ns.Events.FiredCount != 0 {
		t.Fatal("first call must only schedule, not fire")
	}
	if ns.Events.NextCheckAt != 1000+b.EventCheckSec {
		t.Fatalf("NextCheckAt = %v, want %v", ns.Events.NextCheckAt, 1000+b.EventCheckSec)
	}
}

func TestAdvanceEventsFiresWhenDue(t *testing.T) {
	b := onlyEvent(balance.EvChipShortage)
	s := eventTestState()
	s = advanceEvents(s, b) // schedule
	s.GameTime = s.Events.NextCheckAt + 1
	ns := advanceEvents(s, b)
	if ns.Events.FiredCount != 1 {
		t.Fatalf("FiredCount = %d, want 1", ns.Events.FiredCount)
	}
	if ns.Events.NextCheckAt <= s.GameTime {
		t.Fatal("next check must be rescheduled into the future")
	}
}

func TestAdvanceEventsNoRefireWhilePendingOrActive(t *testing.T) {
	b := onlyEvent(balance.EvChipShortage)
	s := eventTestState()
	s = advanceEvents(s, b)
	s.GameTime = s.Events.NextCheckAt + 1
	s = advanceEvents(s, b) // fires once → pending + active
	for i := 0; i < 5; i++ {
		s.GameTime = s.Events.NextCheckAt + 1
		s = advanceEvents(s, b)
	}
	if s.Events.FiredCount != 1 {
		t.Fatalf("FiredCount = %d, want 1 (no refire while pending/active)", s.Events.FiredCount)
	}
}

func TestAdvanceEventsAutoResolvesPastDeadline(t *testing.T) {
	b := onlyEvent(balance.EvChipShortage)
	s := eventTestState()
	spec := b.Events[0]
	s = fireEvent(s, spec, b)
	s.GameTime = s.Events.Pending[0].Deadline + 1
	// Keep NextCheckAt in the future so this call only auto-resolves.
	s.Events.NextCheckAt = s.GameTime + b.EventCheckSec
	ns := advanceEvents(s, b)
	if len(ns.Events.Pending) != 0 {
		t.Fatal("overdue pending must auto-resolve")
	}
	if ns.Events.AutoCount != 1 || len(ns.Events.Log) != 1 || !ns.Events.Log[0].Auto {
		t.Fatalf("auto-resolution not recorded: %+v", ns.Events)
	}
	if len(ns.Events.Active) != 1 {
		t.Fatal("default choice (轉租度過) keeps the modifier")
	}
}

func TestAdvanceEventsExpiresModifiers(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	m := model.NeutralEventEffects()
	m.PowerCostMult = 1.3
	s.Events.Active = []model.ActiveModifier{{EventID: "x", ExpiresAt: 100, Target: -1, Effects: m}}
	s.GameTime = 101
	s.Events.NextCheckAt = 1e12 // keep the trigger roll out of this test
	ns := advanceEvents(s, b)
	if len(ns.Events.Active) != 0 {
		t.Fatal("expired modifier must be removed")
	}
}

func TestAdvanceEventsRespectsValuationGate(t *testing.T) {
	b := onlyEvent(balance.EvBubbleTalk)
	s := eventTestState()
	s.PeakValuation = 1e6 // below the 5e8 gate
	s = advanceEvents(s, b)
	for i := 0; i < 5; i++ {
		s.GameTime = s.Events.NextCheckAt + 1
		s = advanceEvents(s, b)
	}
	if s.Events.FiredCount != 0 {
		t.Fatal("bubble-talk must not fire below its valuation gate")
	}
}

func TestIncidentWeightZeroWithoutOnlineModels(t *testing.T) {
	b := balance.Default()
	var s model.GameState // no models at all
	spec, _ := balance.EventByID(b.Events, balance.EvIncident)
	if w := eventWeight(s, spec, b); w != 0 {
		t.Fatalf("incident weight = %v, want 0 with no online models", w)
	}
}

func TestAdvanceEventsDeterministic(t *testing.T) {
	b := balance.Default()
	b.EventHitChance = 1.0
	run := func() model.GameState {
		s := eventTestState()
		for i := 0; i < 200; i++ {
			s.GameTime += b.EventCheckSec / 2
			s = advanceEvents(s, b)
		}
		return s
	}
	a, c := run(), run()
	if a.Events.FiredCount != c.Events.FiredCount || a.Events.RandState != c.Events.RandState ||
		len(a.Events.Log) != len(c.Events.Log) {
		t.Fatalf("same seed must give identical event streams: %+v vs %+v", a.Events, c.Events)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sim/ -run 'TestAdvanceEvents|TestIncidentWeight' -v`
Expected: FAIL to build — `undefined: advanceEvents`, `undefined: eventWeight`

- [ ] **Step 3: Write the implementation**

Append to `internal/sim/events.go`:

```go
// advanceEvents is the per-tick event step: expire modifiers, auto-resolve
// overdue pending choices to their free default, then roll for a new trigger
// when the check timer is due. Called by Tick after GameTime advances. Pure.
func advanceEvents(ns model.GameState, b balance.Config) model.GameState {
	now := ns.GameTime
	// 1. Expire sustained modifiers.
	if len(ns.Events.Active) > 0 {
		kept := make([]model.ActiveModifier, 0, len(ns.Events.Active))
		for _, m := range ns.Events.Active {
			if m.ExpiresAt > now {
				kept = append(kept, m)
			}
		}
		ns.Events.Active = kept
	}
	// 2. Auto-resolve overdue pending events with their free default choice.
	for i := 0; i < len(ns.Events.Pending); {
		p := ns.Events.Pending[i]
		if p.Deadline > now {
			i++
			continue
		}
		spec, ok := balance.EventByID(b.Events, p.EventID)
		if !ok { // catalog drift: drop silently
			ns.Events.Pending = removePending(ns.Events.Pending, i)
			continue
		}
		var err error
		ns, err = resolveChoice(ns, i, spec.DefaultChoice, true, b)
		if err != nil { // defensive: the default choice is free and always valid
			i++
		}
	}
	// 3. Trigger roll(s) — loop covers large offline chunks crossing a check.
	if b.EventCheckSec <= 0 {
		return ns
	}
	if ns.Events.NextCheckAt == 0 {
		// Fresh run or pre-events save: schedule the first roll, no fire.
		ns.Events.NextCheckAt = now + b.EventCheckSec
		return ns
	}
	for ns.Events.NextCheckAt <= now {
		var hit, jitter float64
		ns.Events.RandState, hit = nextRand(ns.Events.RandState)
		ns.Events.RandState, jitter = nextRand(ns.Events.RandState)
		ns.Events.NextCheckAt += b.EventCheckSec * (0.75 + 0.5*jitter)
		if hit >= b.EventHitChance {
			continue
		}
		specs, weights, total := eligibleEvents(ns, b)
		if total <= 0 {
			continue
		}
		var pick float64
		ns.Events.RandState, pick = nextRand(ns.Events.RandState)
		x := pick * total
		for k, w := range weights {
			x -= w
			if x <= 0 {
				ns = fireEvent(ns, specs[k], b)
				break
			}
		}
	}
	return ns
}

// eligibleEvents returns the specs currently allowed to fire with their
// state-adjusted weights: gates passed, not pending, not active, and past
// the per-event cooldown since its last history record.
func eligibleEvents(ns model.GameState, b balance.Config) (specs []balance.EventSpec, weights []float64, total float64) {
	now := ns.GameTime
	for _, spec := range b.Events {
		if now < spec.MinGameTime || ns.PeakValuation < spec.MinValuation {
			continue
		}
		if hasPending(ns, spec.ID) || hasActive(ns, spec.ID) || inCooldown(ns, spec.ID, b.EventCooldownSec) {
			continue
		}
		w := eventWeight(ns, spec, b)
		if w <= 0 {
			continue
		}
		specs = append(specs, spec)
		weights = append(weights, w)
		total += w
	}
	return specs, weights, total
}

func hasPending(ns model.GameState, id string) bool {
	for _, p := range ns.Events.Pending {
		if p.EventID == id {
			return true
		}
	}
	return false
}

func hasActive(ns model.GameState, id string) bool {
	for _, m := range ns.Events.Active {
		if m.EventID == id {
			return true
		}
	}
	return false
}

func inCooldown(ns model.GameState, id string, cooldown float64) bool {
	for _, rec := range ns.Events.Log {
		if rec.EventID == id && ns.GameTime-rec.At < cooldown {
			return true
		}
	}
	return false
}

// avgOnlineSafety is the mean safety quality across online models (0 if none).
func avgOnlineSafety(ns model.GameState) float64 {
	var sum float64
	var n int
	for _, m := range ns.Models {
		if m.Online {
			sum += m.Quality[model.DimSafety]
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// eventWeight is the state-adjusted trigger weight of one event (design §3.2):
// incident chance scales with low model safety × alignment tech × lingering
// aftermath; paper chance scales with research headcount.
func eventWeight(ns model.GameState, spec balance.EventSpec, b balance.Config) float64 {
	switch spec.ID {
	case balance.EvIncident:
		avg := avgOnlineSafety(ns)
		if avg <= 0 {
			return 0 // nothing online → nothing to break
		}
		f := 1 - avg/balance.EvIncidentSafetyRef
		if f <= 0 {
			return 0 // safe enough: incidents effectively off
		}
		return spec.Weight * f * techEffects(ns, b).IncidentMult * eventEffects(ns, b).IncidentChanceMult
	case balance.EvPaper:
		n := 0
		for tier := model.Tier1; tier <= model.Tier3; tier++ {
			n += ns.Research.Researchers[tier]
		}
		m := 1 + float64(n)*0.1
		if m > 3 {
			m = 3
		}
		return spec.Weight * m
	default:
		return spec.Weight
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/sim/ -v`
Expected: PASS (all)

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/sim
git add internal/sim/events.go internal/sim/events_test.go
git commit -m "feat(sim): advanceEvents trigger loop with gates, weights, cooldown"
```

---

### Task 7: `ResolveEvent` through `sim.Apply`

**Files:**
- Modify: `internal/sim/apply.go`
- Modify: `internal/sim/apply_test.go`

**Interfaces:**
- Consumes: `resolveChoice` (Task 5), `model.ResolveEvent` (Task 1).
- Produces: `sim.Apply` accepts `model.ResolveEvent{PendingIndex, Choice}`; player resolutions always pass `auto=false`.

- [ ] **Step 1: Write the failing test**

Append to `internal/sim/apply_test.go` (match the file's existing import style):

```go
func TestApplyResolveEvent(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	spec, _ := balance.EventByID(b.Events, balance.EvChipShortage)
	s = fireEvent(s, spec, b)
	ns, err := Apply(s, model.ResolveEvent{PendingIndex: 0, Choice: 1}, b)
	if err != nil {
		t.Fatalf("Apply(ResolveEvent): %v", err)
	}
	if len(ns.Events.Pending) != 0 || len(ns.Events.Log) != 1 {
		t.Fatalf("resolution not applied: %+v", ns.Events)
	}
	if ns.Events.Log[0].Auto {
		t.Fatal("player resolution must not be marked auto")
	}
	if _, err := Apply(s, model.ResolveEvent{PendingIndex: 9, Choice: 1}, b); err != ErrInvalidEventIndex {
		t.Fatalf("bad index err = %v, want ErrInvalidEventIndex", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/sim/ -run TestApplyResolveEvent -v`
Expected: FAIL — `sim: unknown command` (Apply has no case yet)

- [ ] **Step 3: Write the implementation**

In `internal/sim/apply.go`, add a case to the `switch` in `Apply` (before `default`):

```go
	case model.ResolveEvent:
		return resolveChoice(s, c.PendingIndex, c.Choice, false, b)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/sim/ -run TestApplyResolveEvent -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/sim/apply.go internal/sim/apply_test.go
git commit -m "feat(sim): wire ResolveEvent command through Apply"
```

---

### Task 8: Tick integration + effect consumption points

**Files:**
- Modify: `internal/sim/sim.go` (header comment, Tick, advanceUsers, Valuation)
- Modify: `internal/sim/apply.go` (applyBuildServer, applyUnlockTech)
- Modify: `internal/sim/prestige.go` (Restart, applyPrestigeReset carry RandState)
- Modify: `internal/sim/view.go` (EffectiveRefPrice, EstimateUserTarget)
- Modify: `internal/sim/events_test.go`

**Interfaces:**
- Consumes: `advanceEvents`, `eventEffects`, `eventTechCostMult` (Tasks 4/6).
- Produces: `Tick` runs the event step and all six sustained multipliers take effect. No signature changes anywhere.

- [ ] **Step 1: Write the failing tests**

Append to `internal/sim/events_test.go`:

```go
// activeMod is a test helper installing one modifier that never expires.
func activeMod(s model.GameState, set func(e *model.EventEffects)) model.GameState {
	e := model.NeutralEventEffects()
	set(&e)
	s.Events.Active = append(s.Events.Active,
		model.ActiveModifier{EventID: "test", ExpiresAt: 1e18, Target: -1, Effects: e})
	return s
}

func TestTickRunsEventStep(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	ns := Tick(s, 3600, nil, b)
	if ns.Events.NextCheckAt == 0 {
		t.Fatal("Tick must run advanceEvents (NextCheckAt scheduled)")
	}
}

func TestPowerCostMultRaisesElectricity(t *testing.T) {
	b := balance.Default()
	base := eventTestState()
	base.Models = nil // isolate: no revenue/serving noise
	base.Servers = []model.Server{{Pool: model.PoolInference, Compute: 1, PowerKW: 10, Slots: 1}}
	plain := Tick(base, 3600, nil, b)
	spiked := Tick(activeMod(base, func(e *model.EventEffects) { e.PowerCostMult = 2.0 }), 3600, nil, b)
	extraBurn := plain.Resources.Cash - spiked.Resources.Cash
	want := 10 * b.ElectricityPerKWSec * 3600 // one extra 1× of the power bill
	if diff := extraBurn - want; diff > 1e-6 || diff < -1e-6 {
		t.Fatalf("extra burn = %v, want %v", extraBurn, want)
	}
}

func TestValuationMultApplied(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	v1 := Valuation(s, b)
	v2 := Valuation(activeMod(s, func(e *model.EventEffects) { e.ValuationMult = 0.75 }), b)
	if diff := v2 - 0.75*v1; diff > 1e-6 || diff < -1e-6 {
		t.Fatalf("valuation = %v, want %v", v2, 0.75*v1)
	}
}

func TestBuildCostMultRaisesCapex(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	s.Datacenter = model.Datacenter{PowerCapacity: 1000, SlotCapacity: 100}
	cmd := model.BuildServer{Process: "N7", Pool: model.PoolTraining}
	plain, err := Apply(s, cmd, b)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	dear, err := Apply(activeMod(s, func(e *model.EventEffects) { e.BuildCostMult = 1.18 }), cmd, b)
	if err != nil {
		t.Fatalf("build with modifier: %v", err)
	}
	plainCost := s.Resources.Cash - plain.Resources.Cash
	dearCost := s.Resources.Cash - dear.Resources.Cash
	if diff := dearCost - 1.18*plainCost; diff > 1e-6 || diff < -1e-6 {
		t.Fatalf("capex = %v, want %v", dearCost, 1.18*plainCost)
	}
}

func TestTechCostMultDiscountsTargetBranchOnly(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	s.Resources.RnD = 1e9
	e := model.NeutralEventEffects()
	e.TechCostMult = 0.5
	s.Events.Active = append(s.Events.Active, model.ActiveModifier{
		EventID: "paper", ExpiresAt: 1e18, Target: int(model.BranchBusiness), Effects: e})
	// biz-growth-1 (BranchBusiness, cost 6000) should be half price.
	ns, err := Apply(s, model.UnlockTech{NodeID: "biz-growth-1"}, b)
	if err != nil {
		t.Fatalf("unlock: %v", err)
	}
	if spent := s.Resources.RnD - ns.Resources.RnD; spent != 3000 {
		t.Fatalf("discounted cost = %v, want 3000", spent)
	}
	// infra-eff-1 (BranchInfra, cost 8000) is untargeted → full price.
	ns2, err := Apply(s, model.UnlockTech{NodeID: "infra-eff-1"}, b)
	if err != nil {
		t.Fatalf("unlock: %v", err)
	}
	if spent := s.Resources.RnD - ns2.Resources.RnD; spent != 8000 {
		t.Fatalf("untargeted cost = %v, want 8000", spent)
	}
}

func TestUserGrowthAndTAMMultRaiseTarget(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	plain := Tick(s, 3600, nil, b)
	boosted := Tick(activeMod(s, func(e *model.EventEffects) {
		e.UserGrowthMult = 1.5
		e.TAMMult = 1.25
	}), 3600, nil, b)
	if boosted.Models[0].Users <= plain.Models[0].Users {
		t.Fatalf("growth modifiers must raise users: %v vs %v",
			boosted.Models[0].Users, plain.Models[0].Users)
	}
}

func TestEffectiveRefPriceIncludesEventMult(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	p1 := EffectiveRefPrice(s, model.SegConsumer, b)
	p2 := EffectiveRefPrice(activeMod(s, func(e *model.EventEffects) { e.RefPriceMult = 0.8 }), model.SegConsumer, b)
	if diff := p2 - 0.8*p1; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("ref price = %v, want %v", p2, 0.8*p1)
	}
}

func TestTickDeterministicWithEvents(t *testing.T) {
	b := balance.Default()
	b.EventHitChance = 1.0
	run := func() model.GameState {
		s := eventTestState()
		for i := 0; i < 300; i++ {
			s = Tick(s, 3600, nil, b)
		}
		return s
	}
	a, c := run(), run()
	if a.Events.RandState != c.Events.RandState || a.Events.FiredCount != c.Events.FiredCount ||
		a.Resources.Cash != c.Resources.Cash || a.GameTime != c.GameTime {
		t.Fatal("Tick with events must be fully deterministic")
	}
}

func TestRestartPreservesRandState(t *testing.T) {
	b := balance.Default()
	s := eventTestState()
	s.Events.RandState = 999
	ns := Restart(s, b)
	if ns.Events.RandState != 999 {
		t.Fatalf("Restart RandState = %d, want 999", ns.Events.RandState)
	}
	if len(ns.Events.Pending) != 0 || len(ns.Events.Active) != 0 || len(ns.Events.Log) != 0 {
		t.Fatal("Restart must clear event history/pending/active")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sim/ -run 'TestTick|TestPowerCost|TestValuationMult|TestBuildCost|TestTechCostMultDiscount|TestUserGrowth|TestEffectiveRefPriceIncludes|TestRestartPreserves' -v`
Expected: FAIL — modifiers have no effect yet (assertions on unchanged values)

- [ ] **Step 3: Write the implementation**

**`internal/sim/sim.go`** — replace the package header comment:

```go
// Package sim is the pure, deterministic simulation core.
// No wall-clock, no I/O, and no non-deterministic randomness — all event
// randomness flows through GameState.Events.RandState (splitmix64); time
// advances only via dt. Same state + same inputs → same result.
package sim
```

In `Tick`, right after `ns.GameTime += dt`, add:

```go
	ns = advanceEvents(ns, b)
	ee := eventEffects(ns, b)
```

Change the electricity line from:

```go
	ns.Resources.Cash -= serverPower * b.ElectricityPerKWSec * dt
```

to:

```go
	ns.Resources.Cash -= serverPower * b.ElectricityPerKWSec * ee.PowerCostMult * dt
```

In `advanceUsers`, after `se := starEffects(ns, b)` add `ee := eventEffects(ns, b)`, then inside the loop:
- after `w := b.SegmentWeights[m.Segment]` add:

```go
		w[model.DimSafety] *= ee.SafetyWeightMult // arrays copy by value; safe
```

- change the refPrice line to:

```go
		refPrice := b.SegmentRefPrice[m.Segment] * te.RefPriceMult * ee.RefPriceMult
```

- change the target line to:

```go
		target := appeal * b.SegmentTargetScale[m.Segment] * demandMult * share *
			marketingMult * te.UserGrowthMult * se.UserGrowthMult *
			ee.UserGrowthMult * ee.TAMMult
```

In `Valuation`, change the return to:

```go
	return (monthlyRev*b.RevenueMultiple + users*b.UserValue + assets) *
		eventEffects(ns, b).ValuationMult
```

**`internal/sim/apply.go`** — in `applyBuildServer`, change the capex line to:

```go
	capex := (p.BuyPrice + b.ChassisCost) * eventEffects(s, b).BuildCostMult
```

In `applyUnlockTech`, change the cost check/charge to:

```go
	cost := node.Cost * eventTechCostMult(s, node.Branch)
	if s.Resources.RnD < cost {
		return s, ErrInsufficientRnD
	}
	ns := s
	ns.Resources.RnD -= cost
```

**`internal/sim/prestige.go`** — in `Restart`, change the tail to:

```go
	ns := freshRun(p, b)
	ns.Events.RandState = s.Events.RandState
	return ns
```

In `applyPrestigeReset` (`internal/sim/apply.go`), change the tail the same way:

```go
	ns := freshRun(p, b)
	ns.Events.RandState = s.Events.RandState
	return ns, nil
```

**`internal/sim/view.go`** — in `EffectiveRefPrice`, change the return to:

```go
	return b.SegmentRefPrice[seg] * te.RefPriceMult * eventEffects(s, b).RefPriceMult
```

In `EstimateUserTarget`, after `se := starEffects(s, b)` add `ee := eventEffects(s, b)`; after `w := b.SegmentWeights[m.Segment]` add `w[model.DimSafety] *= ee.SafetyWeightMult`; and change the final return to:

```go
	return appeal * b.SegmentTargetScale[m.Segment] * demandMult * share *
		marketingMult * te.UserGrowthMult * se.UserGrowthMult *
		ee.UserGrowthMult * ee.TAMMult
```

- [ ] **Step 4: Run the full sim suite**

Run: `go test ./internal/sim/ ./internal/tui/ -count=1`
Expected: PASS (TUI suite exercises Tick/Settle and must not regress)

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/sim
git add internal/sim
git commit -m "feat(sim): events step in Tick and modifier consumption points"
```

---

### Task 9: TUI event meta + 「產業動態」overview card

**Files:**
- Create: `internal/tui/event_meta.go`
- Create: `internal/tui/event_meta_test.go`
- Modify: `internal/tui/page_overview.go`
- Modify: `internal/tui/page_overview_test.go`

**Interfaces:**
- Consumes: `balance` event ID constants (Task 3); `state.Events` (Task 1); existing helpers `Card`, `VStack`, `styleWarn`, `testModel(t)` (`internal/tui/nav_test.go`).
- Produces: `eventMeta{Name, Desc string; Choices [2]string}`, `eventLabel(id string) eventMeta` (fallback: `Name = id`), `renderEventsCard(m Model) string`; the card appears on the overview page.

- [ ] **Step 1: Write the failing tests**

Create `internal/tui/event_meta_test.go`:

```go
package tui

import (
	"strings"
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestEventLabelKnownAndFallback(t *testing.T) {
	if eventLabel(balance.EvChipShortage).Name == balance.EvChipShortage {
		t.Fatal("chip-shortage should have a Chinese name")
	}
	if eventLabel("mystery").Name != "mystery" {
		t.Fatal("unknown ID must fall back to the raw ID")
	}
	for _, spec := range balance.DefaultEvents() {
		meta := eventLabel(spec.ID)
		if meta.Name == spec.ID {
			t.Fatalf("%s: missing Chinese name", spec.ID)
		}
		if spec.NumChoices > 0 && (meta.Choices[0] == "" || meta.Choices[1] == "") {
			t.Fatalf("%s: choice events need both choice labels", spec.ID)
		}
	}
}

func TestEventsCardEmptyState(t *testing.T) {
	m := testModel(t)
	out := renderEventsCard(m)
	if !strings.Contains(out, "產業動態") || !strings.Contains(out, "風平浪靜") {
		t.Fatalf("empty card wrong:\n%s", out)
	}
}

func TestEventsCardShowsPendingAndLog(t *testing.T) {
	m := testModel(t)
	m.state.GameTime = 100000
	m.state.Events.Pending = []model.PendingEvent{
		{EventID: balance.EvChipShortage, Target: -1, FiredAt: 90000, Deadline: 100000 + 10*86400},
	}
	m.state.Events.Log = []model.EventRecord{
		{EventID: balance.EvMarketCycle, At: 50000, Choice: 0, Auto: false},
	}
	out := renderEventsCard(m)
	if !strings.Contains(out, eventLabel(balance.EvChipShortage).Name) {
		t.Fatal("pending event name missing")
	}
	if !strings.Contains(out, "[e]") {
		t.Fatal("pending line must point at the e key")
	}
	if !strings.Contains(out, eventLabel(balance.EvMarketCycle).Name) {
		t.Fatal("log entry name missing")
	}
}

func TestOverviewIncludesEventsCard(t *testing.T) {
	m := testModel(t)
	if !strings.Contains(renderOverview(m), "產業動態") {
		t.Fatal("overview must include the events card")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run 'TestEventLabel|TestEventsCard|TestOverviewIncludes' -v`
Expected: FAIL to build — `undefined: eventLabel`, `undefined: renderEventsCard`

- [ ] **Step 3: Write the implementation**

Create `internal/tui/event_meta.go`:

```go
package tui

import "tokensmith/internal/balance"

// eventMeta is the Chinese copy for one industry event. Choices[0] is the
// paid/active option, Choices[1] the free/passive default — matching the
// balance.EventSpec choice convention.
type eventMeta struct {
	Name    string
	Desc    string
	Choices [2]string
}

var eventCatalog = map[string]eventMeta{
	balance.EvChipShortage: {
		Name: "晶片短缺", Desc: "供應鏈吃緊，自建服務器成本上漲",
		Choices: [2]string{"囤貨鎖價（花錢免疫漲價）", "轉租度過（承受漲價）"},
	},
	balance.EvEnergySpike: {
		Name: "能源價波動", Desc: "電價劇烈波動，影響機房電費",
		Choices: [2]string{"簽長約鎖價（花錢固定電價）", "觀望（承受波動）"},
	},
	balance.EvRivalBreak: {
		Name: "對手重大發表", Desc: "對手模型能力躍升，前沿被推高",
		Choices: [2]string{"限時促銷（花錢拉用戶成長）", "觀望"},
	},
	balance.EvOpenSourceWar: {
		Name: "開源價格戰", Desc: "開源模型衝擊付費意願",
		Choices: [2]string{"跟進降價（搶量、犧牲單價）", "守高階定位"},
	},
	balance.EvRivalScandal: {
		Name: "對手安全爭議", Desc: "低安全對手爆出爭議，用戶外流",
		Choices: [2]string{"花錢搶客（大幅拉成長）", "觀望（自然吸收）"},
	},
	balance.EvPaper: {
		Name: "突破論文", Desc: "研究突破，某科技分支解鎖成本下降",
		Choices: [2]string{"押注加碼（花 R&D 換 5 折）", "常規吸收（7 折）"},
	},
	balance.EvIncident: {
		Name: "模型安全事故", Desc: "你的模型出事，用戶信任受損",
		Choices: [2]string{"公開道歉（花錢，流失減半）", "低調處理（省錢，留後遺症）"},
	},
	balance.EvRegulation: {
		Name: "AI 監管新法", Desc: "新法上路，安全維度權重提高",
		Choices: [2]string{"投資合規（花錢，安全 +10%）", "硬扛"},
	},
	balance.EvMarketCycle: {
		Name: "市場榮枯", Desc: "宏觀週期轉向，市場規模波動",
	},
	balance.EvBubbleTalk: {
		Name: "AI 泡沫論", Desc: "市場質疑估值，估值倍數下修",
		Choices: [2]string{"釋出實績穩信心（花錢減緩）", "觀望"},
	},
}

func eventLabel(id string) eventMeta {
	if m, ok := eventCatalog[id]; ok {
		return m
	}
	return eventMeta{Name: id}
}
```

In `internal/tui/page_overview.go`, add the render function (after `renderOverview`):

```go
// renderEventsCard is the 產業動態 card: pending decisions first (highlighted
// with their remaining decision window), then recent history, max 4 lines.
func renderEventsCard(m Model) string {
	ev := m.state.Events
	var lines []string
	for _, p := range ev.Pending {
		meta := eventLabel(p.EventID)
		days := (p.Deadline - m.state.GameTime) / 86400
		if days < 0 {
			days = 0
		}
		lines = append(lines, styleWarn.Render(
			fmt.Sprintf("⏳ %s — [e]決策（剩 %.0f 天）", meta.Name, days)))
	}
	for i := len(ev.Log) - 1; i >= 0 && len(lines) < 4; i-- {
		rec := ev.Log[i]
		meta := eventLabel(rec.EventID)
		result := ""
		if meta.Choices[0] != "" && rec.Choice >= 0 && rec.Choice < 2 {
			result = " · " + meta.Choices[rec.Choice]
		}
		if rec.Auto {
			result += "（自動）"
		}
		day := int(rec.At / 86400)
		lines = append(lines, fmt.Sprintf("· D%d %s%s", day, meta.Name, result))
	}
	if len(lines) == 0 {
		lines = append(lines, styleMuted.Render("風平浪靜——尚無產業事件"))
	}
	return Card("產業動態", VStack(lines...))
}
```

In `renderOverview`, add the card as its own row after `rows = append(rows, row1, row2)`:

```go
	rows = append(rows, row1, row2)
	rows = append(rows, renderEventsCard(m))
```

(`styleMuted` already exists — `internal/tui/tui.go` uses it for the dimmed notice.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/ -run 'TestEventLabel|TestEventsCard|TestOverviewIncludes' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/tui
git add internal/tui/event_meta.go internal/tui/event_meta_test.go internal/tui/page_overview.go internal/tui/page_overview_test.go
git commit -m "feat(tui): industry-events meta copy and overview card"
```

---

### Task 10: Event choice dialog + key routing + fire notice

**Files:**
- Create: `internal/tui/dialog_event.go`
- Create: `internal/tui/dialog_event_test.go`
- Modify: `internal/tui/tui.go`

**Interfaces:**
- Consumes: `model.ResolveEvent`, `sim.Apply`, `sim.EventChoiceCost`, `balance.EventByID`, `eventLabel` (Tasks 1/5/9); existing dialog pattern (`dialog_publish.go`), `setNotice`, `Card`, `helpStyle`.
- Produces: `eventDialog{cursor int}` (always operates on `Pending[0]` — oldest first), `newEventDialog(m Model) (eventDialog, bool)`, `(d eventDialog) update(msg tea.KeyMsg) (next eventDialog, confirm, cancel bool)`, `renderEventDialog(d eventDialog, m Model) string`; `Model.event *eventDialog` field; `e` key on overview opens it; a fire notice appears the tick an event triggers.

- [ ] **Step 1: Write the failing tests**

Create `internal/tui/dialog_event_test.go`:

```go
package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// pendingChipShortage arms one pending chip-shortage event on m.
func pendingChipShortage(m Model) Model {
	spec, _ := balance.EventByID(m.cfg.Events, balance.EvChipShortage)
	m.state.Events.Pending = []model.PendingEvent{{
		EventID: spec.ID, Target: -1,
		FiredAt:  m.state.GameTime,
		Deadline: m.state.GameTime + spec.DeadlineSec,
	}}
	return m
}

func TestEventDialogNeedsPending(t *testing.T) {
	m := testModel(t)
	if _, ok := newEventDialog(m); ok {
		t.Fatal("no pending events → no dialog")
	}
	m = pendingChipShortage(m)
	d, ok := newEventDialog(m)
	if !ok {
		t.Fatal("pending event should open the dialog")
	}
	if d.cursor != 1 {
		t.Fatalf("cursor starts on the free default, got %d", d.cursor)
	}
}

func TestEKeyOpensDialogOnOverview(t *testing.T) {
	m := pendingChipShortage(testModel(t))
	m.page = PageOverview
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if nm.(Model).event == nil {
		t.Fatal("e on overview must open the event dialog")
	}
}

func TestEKeyWithoutPendingShowsNotice(t *testing.T) {
	m := testModel(t)
	m.page = PageOverview
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	got := nm.(Model)
	if got.event != nil {
		t.Fatal("no pending → no dialog")
	}
	if got.notice == "" {
		t.Fatal("expected a notice explaining there is nothing to decide")
	}
}

func TestEKeyStillExpandsDatacenterOnComputePage(t *testing.T) {
	m := pendingChipShortage(testModel(t))
	m.page = PageCompute
	m.state.Resources.Cash = 1e9
	before := m.state.Datacenter.PowerCapacity
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	got := nm.(Model)
	if got.event != nil {
		t.Fatal("e on compute page must not open the event dialog")
	}
	if got.state.Datacenter.PowerCapacity <= before {
		t.Fatal("e on compute page must still expand the datacenter")
	}
}

func TestEventDialogConfirmResolves(t *testing.T) {
	m := pendingChipShortage(testModel(t))
	m.page = PageOverview
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m = nm.(Model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // confirm default (free)
	got := nm.(Model)
	if got.event != nil {
		t.Fatal("dialog must close on confirm")
	}
	if len(got.state.Events.Pending) != 0 || len(got.state.Events.Log) != 1 {
		t.Fatalf("resolution not applied: %+v", got.state.Events)
	}
}

func TestEventDialogEscLeavesPending(t *testing.T) {
	m := pendingChipShortage(testModel(t))
	m.page = PageOverview
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m = nm.(Model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := nm.(Model)
	if got.event != nil {
		t.Fatal("esc must close the dialog")
	}
	if len(got.state.Events.Pending) != 1 {
		t.Fatal("esc must leave the event pending")
	}
}

func TestEventDialogRenderShowsChoicesAndCost(t *testing.T) {
	m := pendingChipShortage(testModel(t))
	d, _ := newEventDialog(m)
	out := renderEventDialog(d, m)
	meta := eventLabel(balance.EvChipShortage)
	if !strings.Contains(out, meta.Name) || !strings.Contains(out, meta.Choices[0]) ||
		!strings.Contains(out, meta.Choices[1]) {
		t.Fatalf("dialog missing copy:\n%s", out)
	}
	if !strings.Contains(out, "$") {
		t.Fatal("paid option must show its cash cost")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run 'TestEvent|TestEKey' -v`
Expected: FAIL to build — `undefined: newEventDialog`

- [ ] **Step 3: Write the implementation**

Create `internal/tui/dialog_event.go`:

```go
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"tokensmith/internal/balance"
	"tokensmith/internal/sim"
)

// eventDialog lets the player resolve the oldest pending industry event.
// cursor is the highlighted choice; it starts on the free default (1).
type eventDialog struct {
	cursor int
}

func newEventDialog(m Model) (eventDialog, bool) {
	if len(m.state.Events.Pending) == 0 {
		return eventDialog{}, false
	}
	return eventDialog{cursor: 1}, true
}

func (d eventDialog) update(msg tea.KeyMsg) (next eventDialog, confirm, cancel bool) {
	switch msg.String() {
	case "esc":
		return d, false, true
	case "enter":
		return d, true, false
	case "left", "up":
		d.cursor = 0
	case "right", "down":
		d.cursor = 1
	}
	return d, false, false
}

func renderEventDialog(d eventDialog, m Model) string {
	p := m.state.Events.Pending[0]
	meta := eventLabel(p.EventID)
	spec, ok := balance.EventByID(m.cfg.Events, p.EventID)

	var b strings.Builder
	b.WriteString(meta.Desc + "\n\n")
	if ok {
		days := (p.Deadline - m.state.GameTime) / 86400
		if days < 0 {
			days = 0
		}
		b.WriteString(fmt.Sprintf("決策期限：剩 %.0f 天（逾時自動選保守項）\n\n", days))
		cash, rnd := sim.EventChoiceCost(m.state, spec)
		cost := fmt.Sprintf("$%s", human(cash))
		if rnd > 0 {
			cost += fmt.Sprintf(" + %s R&D", human(rnd))
		}
		labels := [2]string{
			fmt.Sprintf("%s — 費用 %s", meta.Choices[0], cost),
			meta.Choices[1],
		}
		for i, label := range labels {
			marker := "  "
			line := fmt.Sprintf("[%d] %s", i+1, label)
			if d.cursor == i {
				marker = "▸ "
				line = styleAccent.Render(line)
			}
			b.WriteString(marker + line + "\n")
		}
	} else {
		b.WriteString("（此事件版本已不存在，確認後移除）\n")
	}
	b.WriteString("\n" + helpStyle.Render("[←→]選擇 [Enter]確認 [Esc]稍後再說"))
	return Card("📰 "+meta.Name, b.String())
}
```

In `internal/tui/tui.go`:

1. Add the field to `Model` (after `publish *publishDialog`):

```go
	event          *eventDialog   // non-nil while the event-choice modal is open
```

2. In `handleUpdate`, route dialog keys — insert **before** the `if m.publish != nil` line inside `case tea.KeyMsg:`:

```go
		if m.event != nil {
			return m.updateEventDialog(msg)
		}
```

3. Add the dialog updater (after `updatePublishDialog`):

```go
func (m Model) updateEventDialog(msg tea.KeyMsg) (Model, tea.Cmd) {
	d, confirm, cancel := m.event.update(msg)
	if cancel {
		m.event = nil
		return m, nil
	}
	if confirm {
		ns, err := sim.Apply(m.state, model.ResolveEvent{PendingIndex: 0, Choice: d.cursor}, m.cfg)
		switch {
		case err == nil:
			m.state = ns
		case errors.Is(err, sim.ErrInsufficientCash):
			m.setNotice("現金不足，付不起這個選項")
			m.event = &d
			return m, nil
		case errors.Is(err, sim.ErrInsufficientRnD):
			m.setNotice("R&D 不足，付不起這個選項")
			m.event = &d
			return m, nil
		}
		m.event = nil
		return m, nil
	}
	m.event = &d
	return m, nil
}
```

4. In the `case "e":` handler, add the overview branch first:

```go
		case "e":
			if m.page == PageOverview {
				if d, ok := newEventDialog(m); ok {
					m.event = &d
				} else {
					m.setNotice("目前沒有待決事件")
				}
			} else if m.page == PageCompute {
				m.state = applyOK(m.state, model.ExpandDatacenter{PowerDelta: 100, SlotDelta: 5}, m.cfg)
			} else if m.page == PageTeam {
				m.state = applyOK(m.state, model.HireStaff{Role: model.RoleEngineer, Count: 1}, m.cfg)
			}
			return m, nil
```

5. Fire notice — in the `case tickMsg:` handler, capture the count before `sim.Tick` and compare after:

```go
		prevFired := m.state.Events.FiredCount
		m.state = sim.Tick(m.state, tickDT, events, m.cfg)
		if m.state.Events.FiredCount > prevFired {
			m.setNotice("📰 產業事件：" + latestEventName(m.state))
		}
```

And add the helper (near `pressures`):

```go
// latestEventName names the most recently fired event for the notice line.
func latestEventName(s model.GameState) string {
	if n := len(s.Events.Pending); n > 0 {
		return eventLabel(s.Events.Pending[n-1].EventID).Name + "（總覽頁按 e 決策）"
	}
	if n := len(s.Events.Log); n > 0 {
		return eventLabel(s.Events.Log[n-1].EventID).Name
	}
	return ""
}
```

6. In `contentBody`, render the dialog first:

```go
	if m.event != nil {
		return renderEventDialog(*m.event, m)
	}
```

7. In `pageKeys`, extend the dialog guard and the overview hint:

```go
	if m.publish != nil || m.dialog != nil || m.event != nil {
		return "" // dialogs embed their own help
	}
```

and in the `default:` (overview) branch:

```go
	default: // overview
		hint := "[t]訓練 [X]重來"
		if m.state.PeakValuation >= m.cfg.PrestigeUnlockValuation {
			hint = "[t]訓練 [P]傳承重開 [X]重來"
		}
		if len(m.state.Events.Pending) > 0 {
			hint = "[e]事件決策 " + hint
		}
		return hint
```

(This keeps the original hint order and only prefixes `[e]事件決策` when a decision is waiting.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/ -count=1`
Expected: PASS (all TUI tests, including pre-existing navigation/dialog tests)

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/tui
git add internal/tui/dialog_event.go internal/tui/dialog_event_test.go internal/tui/tui.go
git commit -m "feat(tui): event choice dialog, e-key routing, fire notice"
```

---

### Task 11: Offline settle summary + RandState seeding on load

**Files:**
- Modify: `internal/tui/settle.go`
- Modify: `internal/tui/settle_test.go`
- Modify: `internal/tui/tui.go` (`newAtPaths`, `startup`, `offlineBanner`)

**Interfaces:**
- Consumes: `EventsState.FiredCount` / `AutoCount` monotonic counters (Tasks 1/5).
- Produces: `Summary.EventsFired`, `Summary.EventsAutoResolved int`; offline banner mentions events; every loaded/new state gets a nonzero `RandState`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/settle_test.go`:

```go
func TestSettleCountsEvents(t *testing.T) {
	b := balance.Default()
	b.EventHitChance = 1.0
	b.EventCheckSec = 3600 // one roll per settle chunk
	var s model.GameState
	s.Resources.Cash = 1e6
	s.Events.RandState = 42
	s.Events.NextCheckAt = 1 // pre-scheduled so rolls happen immediately
	ns, sum := Settle(s, b, 6*3600, 0, 0)
	if sum.EventsFired == 0 {
		t.Fatalf("expected events during 6h settle, got %+v", sum)
	}
	if ns.Events.FiredCount != sum.EventsFired {
		t.Fatalf("summary %d != state counter %d", sum.EventsFired, ns.Events.FiredCount)
	}
}

func TestNewAtSeedsRandState(t *testing.T) {
	m := testModel(t)
	if m.state.Events.RandState == 0 {
		t.Fatal("a fresh game must get a nonzero RNG seed")
	}
}
```

Add missing imports (`tokensmith/internal/balance`, `tokensmith/internal/model`) to the test file if absent.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run 'TestSettleCounts|TestNewAtSeeds' -v`
Expected: FAIL to build — `sum.EventsFired undefined`; then `TestNewAtSeeds` FAILs on zero seed

- [ ] **Step 3: Write the implementation**

In `internal/tui/settle.go`, extend `Summary`:

```go
type Summary struct {
	RnDGained          float64
	SecondsSettled     float64
	TrainingCompleted  bool
	TokensIn           int
	TokensOut          int
	EventsFired        int
	EventsAutoResolved int
}
```

In `Settle`, capture the counters next to `beforeRnD` and diff after the loop:

```go
	beforeFired := s.Events.FiredCount
	beforeAuto := s.Events.AutoCount
```

and after the chunk loop, next to `sum.RnDGained = ...`:

```go
	sum.EventsFired = s.Events.FiredCount - beforeFired
	sum.EventsAutoResolved = s.Events.AutoCount - beforeAuto
```

In `internal/tui/tui.go`:

1. `newAtPaths` — seed the RNG after the load/new-game block (before `meta, metaOK, _ := ...`):

```go
	if state.Events.RandState == 0 {
		// New game or pre-events save: seed once, outside the pure sim.
		state.Events.RandState = uint64(time.Now().UnixNano())
	}
```

2. `startup` — also surface the banner when events happened offline:

```go
	if sum.RnDGained > 0 || sum.TrainingCompleted || sum.EventsFired > 0 {
		m.offlineSummary = &sum
	}
```

3. `offlineBanner` — append the event count after the training line:

```go
	if s.EventsFired > 0 {
		msg += fmt.Sprintf(" · 產業事件 %d 起", s.EventsFired)
		if s.EventsAutoResolved > 0 {
			msg += fmt.Sprintf("（%d 起已自動決議）", s.EventsAutoResolved)
		}
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/ -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/tui
git add internal/tui/settle.go internal/tui/settle_test.go internal/tui/tui.go
git commit -m "feat(tui): offline event summary and RNG seeding on load"
```

---

### Task 12: Full verification & smoke run

**Files:** none created — verification only.

- [ ] **Step 1: Full test suite**

Run: `go test ./... -count=1`
Expected: PASS across `internal/model`, `internal/balance`, `internal/sim`, `internal/tui`, `internal/ingest`, `internal/ledger`, `internal/store`, `internal/daemon`, `internal/game`

- [ ] **Step 2: Vet + format check**

Run: `go vet ./... && gofmt -l internal/ cmd/ main.go`
Expected: no output (clean)

- [ ] **Step 3: Build both binaries**

Run: `go build ./... && go build -o /tmp/tokensmith-smoke .`
Expected: builds succeed

- [ ] **Step 4: Determinism spot-check under the race detector**

Run: `go test ./internal/sim/ -run 'TestTickDeterministicWithEvents|TestAdvanceEventsDeterministic' -race -count=2`
Expected: PASS

- [ ] **Step 5: Manual smoke (10 seconds)**

Run the TUI briefly with a throwaway save to confirm the overview shows the 產業動態 card and no layout breakage:

```bash
HOME=$(mktemp -d) /tmp/tokensmith-smoke
```

(quit with `q`; the throwaway HOME keeps your real save untouched)

- [ ] **Step 6: Commit any straggler formatting, then hand off**

```bash
git status --short   # expect clean
```

Implementation complete → use superpowers:finishing-a-development-branch to merge/PR.
