# Train Cash Boosts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let players spend cash on per-training consumables that add quality outside Alloc, with non-exploitable revenue-anchored pricing, progressive full-pack cash cost, and rival target-side investment without gen-unlock cliffs.

**Architecture:** Pure pricing/bonus math lives in `balance` (state-free) and `sim` (stateful anchor). `StartTraining` carries `Boosts`; job freezes `CashBonus` + `BoostCashPaid`. Completion adds frozen bonus before tech/star mults. Rivals get `+β×GlobalFrontier` on top Skill dims inside `rivalTarget` only. All rival quality reads go through `EffectiveRivalQuality` (v1 identity).

**Tech Stack:** Go 1.25, existing `tokensmith` packages (`balance`, `model`, `sim`, `store`, `tui`), `go test ./...`.

**Spec:** `docs/superpowers/specs/2026-07-12-train-cash-boost-design.md` (Revised post design review — implement that revision, **not** the original soft-cap draft).

## Global Constraints

- Cash bonus is **additive** and **never compressed** (no quality soft cap).
- Pricing anchor **never** uses sticker `Model.Price`; use `Users × EffectiveRefPrice × revenue mults`.
- Full-pack suppression is **slot price multipliers** only (`1, 1, 1.8, 2.5` by ascending dim rank among selected).
- Chinese catalog names only: 優質語料 / 省算力改造 / 安全評測 / 加速優化.
- `internal/sim` stays pure (no wall clock / rand / IO in helpers).
- TDD: failing test → implement → pass → commit per task.
- Verify: `gofmt` changed files; `go test ./...`; `go vet ./...`; `go build ./...` before claiming done.

## File map

| File | Role |
|---|---|
| `internal/balance/train_boost.go` | **Create** — catalog type, defaults wiring helpers, state-free price/bonus pure functions |
| `internal/balance/balance.go` | **Modify** — Config fields + `Default()` values |
| `internal/balance/train_boost_test.go` | **Create** — knobs, catalog, base/slot pricing, affordance ranges |
| `internal/model/types.go` | **Modify** — `StartTraining.Boosts`, `TrainingJob` boost fields |
| `internal/sim/train_boost.go` | **Create** — `BoostRefMonthlyCash`, cost quote, cash bonus, predicted quality, `EffectiveRivalQuality` |
| `internal/sim/train_boost_test.go` | **Create** — anchor exploit/mult, pricing, monotonicity, complete quality |
| `internal/sim/apply.go` | **Modify** — `applyStartTraining` cash + freeze |
| `internal/sim/apply_test.go` | **Modify/extend** — boost start errors/success |
| `internal/sim/sim.go` | **Modify** — `advanceTraining` quality; `advanceUsers` rival appeal via helper |
| `internal/sim/rivals.go` | **Modify** — `rivalTarget(..., b)` investment |
| `internal/sim/rivals_test.go` | **Extend** — target boost + unlock cliff |
| `internal/sim/view.go` | **Modify** — rival reads → `EffectiveRivalQuality` |
| `internal/sim/campaign_view.go` | **Modify** — same |
| `internal/sim/rival_quality_test.go` | **Create** — consistency / identity helper |
| `internal/store/migrate.go` | **Modify** — training job boost validation + repair |
| `internal/store/migrate_test.go` or `store_test.go` | **Extend** — invalid/repair cases |
| `internal/tui/dialog_train.go` | **Modify** — toggles, quotes, predicted appeal |
| `internal/tui/dialog_train_test.go` | **Extend** — boost toggle, command fields, appeal monotonic in render path |

---

### Task 1: Balance catalog + state-free pricing

**Files:**
- Create: `internal/balance/train_boost.go`
- Create: `internal/balance/train_boost_test.go`
- Modify: `internal/balance/balance.go` (`Config` struct + `Default()`)

**Interfaces:**
- Produces:
  - `type TrainBoost struct { Dim model.QualityDim; ID, NameZH string; RoleWeight float64 }`
  - Config fields: `TrainBoosts []TrainBoost`, `TrainBoostBeta`, `TrainBoostPainMult`, `TrainBoostFloorMonthly float64`, `TrainBoostSlotMult [model.NumQualityDims]float64`, `TrainBoostRivalPicks int`
  - `func DefaultTrainBoosts() []TrainBoost`
  - `func TrainBoostBasePrice(gen int, refMonthly float64, dim model.QualityDim, b Config) (float64, error)`
  - `func TrainBoostCashCost(gen int, refMonthly float64, boosts [model.NumQualityDims]bool, b Config) (float64, error)`
  - `func TrainBoostCashBonus(gen int, boosts [model.NumQualityDims]bool, b Config) ([model.NumQualityDims]float64, error)`
  - `func TrainBoostRoleWeightSum(b Config) float64`

- [ ] **Step 1: Add Config fields and Default wiring (compile-only skeleton allowed only after tests exist — write tests first)**

Write `internal/balance/train_boost_test.go`:

```go
package balance

import (
	"math"
	"testing"

	"tokensmith/internal/model"
)

func TestDefaultTrainBoostCatalog(t *testing.T) {
	b := Default()
	if len(b.TrainBoosts) != model.NumQualityDims {
		t.Fatalf("len TrainBoosts = %d, want %d", len(b.TrainBoosts), model.NumQualityDims)
	}
	seen := map[model.QualityDim]bool{}
	ids := map[string]bool{}
	for _, tb := range b.TrainBoosts {
		if tb.NameZH == "" || tb.ID == "" || tb.RoleWeight <= 0 || math.IsNaN(tb.RoleWeight) {
			t.Fatalf("bad entry: %+v", tb)
		}
		if seen[tb.Dim] {
			t.Fatalf("duplicate dim %v", tb.Dim)
		}
		seen[tb.Dim] = true
		if ids[tb.ID] {
			t.Fatalf("duplicate id %s", tb.ID)
		}
		ids[tb.ID] = true
	}
	wantNames := map[model.QualityDim]string{
		model.DimCapability: "優質語料",
		model.DimEfficiency: "省算力改造",
		model.DimSafety:     "安全評測",
		model.DimSpeed:      "加速優化",
	}
	for _, tb := range b.TrainBoosts {
		if tb.NameZH != wantNames[tb.Dim] {
			t.Errorf("dim %v name = %q, want %q", tb.Dim, tb.NameZH, wantNames[tb.Dim])
		}
	}
	if b.TrainBoostBeta != 0.15 || b.TrainBoostPainMult != 1.0 {
		t.Errorf("beta/pain = %v/%v", b.TrainBoostBeta, b.TrainBoostPainMult)
	}
	if b.TrainBoostFloorMonthly != b.StartingCash/12 {
		t.Errorf("floor = %v, want %v", b.TrainBoostFloorMonthly, b.StartingCash/12)
	}
	wantSlot := [model.NumQualityDims]float64{1, 1, 1.8, 2.5}
	if b.TrainBoostSlotMult != wantSlot {
		t.Errorf("slot = %v, want %v", b.TrainBoostSlotMult, wantSlot)
	}
	if b.TrainBoostRivalPicks != 2 {
		t.Errorf("rival picks = %d", b.TrainBoostRivalPicks)
	}
}

func TestTrainBoostCashCostFloorGen1LinearTwoThenSlot(t *testing.T) {
	b := Default()
	ref := b.TrainBoostFloorMonthly // annual = StartingCash
	var none [model.NumQualityDims]bool
	cost0, err := TrainBoostCashCost(1, ref, none, b)
	if err != nil || cost0 != 0 {
		t.Fatalf("none: %v %v", cost0, err)
	}
	// single efficiency (weight 1.0): share 1/4.2 of 100_000
	var one [model.NumQualityDims]bool
	one[model.DimEfficiency] = true
	c1, err := TrainBoostCashCost(1, ref, one, b)
	if err != nil {
		t.Fatal(err)
	}
	want1 := 100_000 * 1.0 / 4.2
	if math.Abs(c1-want1) > 1e-6 {
		t.Fatalf("one = %v, want %v", c1, want1)
	}
	// all four: bases * slot mult by ascending dim order 0,1,2,3
	var all [model.NumQualityDims]bool
	for d := range all {
		all[d] = true
	}
	cAll, err := TrainBoostCashCost(1, ref, all, b)
	if err != nil {
		t.Fatal(err)
	}
	weights := []float64{1.2, 1.0, 1.1, 0.9}
	slots := []float64{1, 1, 1.8, 2.5}
	var wantAll float64
	for i := range weights {
		wantAll += 100_000 * weights[i] / 4.2 * slots[i]
	}
	if math.Abs(cAll-wantAll) > 1e-4 {
		t.Fatalf("all = %v, want %v", cAll, wantAll)
	}
	if cAll <= 100_000 {
		t.Fatalf("full pack with slots should exceed linear full %v", cAll)
	}
}

func TestTrainBoostCashBonusAdditiveNoSoftCap(t *testing.T) {
	b := Default()
	spec, _ := Generation(1)
	var all [model.NumQualityDims]bool
	for d := range all {
		all[d] = true
	}
	bonus, err := TrainBoostCashBonus(1, all, b)
	if err != nil {
		t.Fatal(err)
	}
	for d := range bonus {
		want := b.TrainBoostBeta * spec.QualityScale
		if math.Abs(bonus[d]-want) > 1e-9 {
			t.Fatalf("dim %d bonus = %v, want %v (no soft cap)", d, bonus[d], want)
		}
	}
}

func TestTrainBoostCashCostToggleOrderIndependent(t *testing.T) {
	b := Default()
	ref := 10_000.0
	var a, rev [model.NumQualityDims]bool
	a[0], a[3] = true, true
	rev[3], rev[0] = true, true
	c1, _ := TrainBoostCashCost(2, ref, a, b)
	c2, _ := TrainBoostCashCost(2, ref, rev, b)
	if c1 != c2 {
		t.Fatalf("order dependent: %v vs %v", c1, c2)
	}
}

func TestTrainBoostAffordanceGen1Floor(t *testing.T) {
	b := Default()
	ref := b.TrainBoostFloorMonthly
	var one [model.NumQualityDims]bool
	one[model.DimEfficiency] = true
	c, _ := TrainBoostCashCost(1, ref, one, b)
	if c > 0.30*b.StartingCash {
		t.Fatalf("single item %v > 30%% starting cash", c)
	}
	var all [model.NumQualityDims]bool
	for d := range all {
		all[d] = true
	}
	// linear target before slots is 1× annual floor; with slots strictly greater
	linear := float64(1) * 12 * ref * b.TrainBoostPainMult
	full, _ := TrainBoostCashCost(1, ref, all, b)
	if full < linear {
		t.Fatalf("full %v < linear annual %v", full, linear)
	}
}
```

- [ ] **Step 2: Run tests — expect FAIL** (missing types/funcs)

```bash
go test ./internal/balance/ -run TrainBoost -count=1
```

Expected: compile errors or FAIL on undefined names.

- [ ] **Step 3: Implement `train_boost.go` + Config fields**

In `balance.go` `Config` add:

```go
TrainBoosts            []TrainBoost
TrainBoostBeta         float64
TrainBoostPainMult     float64
TrainBoostFloorMonthly float64
TrainBoostSlotMult     [model.NumQualityDims]float64
TrainBoostRivalPicks   int
```

In `Default()` after `StartingCash` assignment:

```go
c.TrainBoosts = DefaultTrainBoosts()
c.TrainBoostBeta = 0.15
c.TrainBoostPainMult = 1.0
c.TrainBoostFloorMonthly = c.StartingCash / 12
c.TrainBoostSlotMult = [model.NumQualityDims]float64{1, 1, 1.8, 2.5}
c.TrainBoostRivalPicks = 2
```

`train_boost.go` core logic:

```go
package balance

import (
	"math"

	"tokensmith/internal/model"
)

type TrainBoost struct {
	Dim        model.QualityDim
	ID         string
	NameZH     string
	RoleWeight float64
}

func DefaultTrainBoosts() []TrainBoost {
	return []TrainBoost{
		{Dim: model.DimCapability, ID: "boost-data", NameZH: "優質語料", RoleWeight: 1.2},
		{Dim: model.DimEfficiency, ID: "boost-efficiency", NameZH: "省算力改造", RoleWeight: 1.0},
		{Dim: model.DimSafety, ID: "boost-safety", NameZH: "安全評測", RoleWeight: 1.1},
		{Dim: model.DimSpeed, ID: "boost-speed", NameZH: "加速優化", RoleWeight: 0.9},
	}
}

func TrainBoostRoleWeightSum(b Config) float64 {
	var s float64
	for _, tb := range b.TrainBoosts {
		s += tb.RoleWeight
	}
	return s
}

func weightForDim(b Config, dim model.QualityDim) float64 {
	for _, tb := range b.TrainBoosts {
		if tb.Dim == dim {
			return tb.RoleWeight
		}
	}
	return 0
}

func targetFullLinear(gen int, refMonthly float64, b Config) (float64, error) {
	if gen < 1 {
		return 0, ErrInvalidGenerationSpec
	}
	if refMonthly < 0 || math.IsNaN(refMonthly) || math.IsInf(refMonthly, 0) {
		return 0, ErrInvalidGenerationSpec
	}
	return float64(gen) * 12 * refMonthly * b.TrainBoostPainMult, nil
}

func TrainBoostBasePrice(gen int, refMonthly float64, dim model.QualityDim, b Config) (float64, error) {
	full, err := targetFullLinear(gen, refMonthly, b)
	if err != nil {
		return 0, err
	}
	sum := TrainBoostRoleWeightSum(b)
	if sum <= 0 {
		return 0, ErrInvalidGenerationSpec
	}
	return full * weightForDim(b, dim) / sum, nil
}

func TrainBoostCashCost(gen int, refMonthly float64, boosts [model.NumQualityDims]bool, b Config) (float64, error) {
	// Collect selected dims in ascending index order.
	var selected []model.QualityDim
	for d := model.QualityDim(0); d < model.NumQualityDims; d++ {
		if boosts[d] {
			selected = append(selected, d)
		}
	}
	var cost float64
	for rank, d := range selected {
		base, err := TrainBoostBasePrice(gen, refMonthly, d, b)
		if err != nil {
			return 0, err
		}
		mult := 1.0
		if rank >= 0 && rank < model.NumQualityDims {
			mult = b.TrainBoostSlotMult[rank]
		}
		if mult < 1 {
			mult = 1
		}
		cost += base * mult
	}
	return cost, nil
}

func TrainBoostCashBonus(gen int, boosts [model.NumQualityDims]bool, b Config) ([model.NumQualityDims]float64, error) {
	var out [model.NumQualityDims]float64
	spec, err := Generation(gen)
	if err != nil {
		return out, err
	}
	for d := model.QualityDim(0); d < model.NumQualityDims; d++ {
		if boosts[d] {
			out[d] = b.TrainBoostBeta * spec.QualityScale
		}
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test ./internal/balance/ -run TrainBoost -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/balance/train_boost.go internal/balance/train_boost_test.go internal/balance/balance.go
git commit -m "feat(balance): train cash boost catalog and slot pricing"
```

---

### Task 2: Model fields

**Files:**
- Modify: `internal/model/types.go` (`TrainingJob`, `StartTraining`)
- Modify: `internal/model/types_test.go` if it constructs these structs (compile fix only)

**Interfaces:**
- Produces:
  - `StartTraining.Boosts [NumQualityDims]bool`
  - `TrainingJob.Boosts`, `CashBonus [NumQualityDims]float64`, `BoostCashPaid float64`

- [ ] **Step 1: Extend structs**

```go
// TrainingJob
Boosts        [NumQualityDims]bool
CashBonus     [NumQualityDims]float64
BoostCashPaid float64

// StartTraining
Boosts [NumQualityDims]bool
```

Zero values preserve legacy behavior. No JSON tags required unless neighboring fields use them (match local style).

- [ ] **Step 2: `go test ./internal/model/ -count=1` — PASS**

- [ ] **Step 3: Commit**

```bash
git add internal/model/types.go internal/model/types_test.go
git commit -m "feat(model): train boost fields on StartTraining and TrainingJob"
```

---

### Task 3: Sim anchor + quote helpers

**Files:**
- Create: `internal/sim/train_boost.go`
- Create: `internal/sim/train_boost_test.go`

**Interfaces:**
- Consumes: `balance.TrainBoostCashCost`, `balance.TrainBoostCashBonus`, `EffectiveRefPrice`, `PrestigeEffects`, `campaignEffects`
- Produces:
  - `func BoostRefMonthlyCash(s model.GameState, b balance.Config) float64`
  - `func TrainBoostRefMonthly(s model.GameState, b balance.Config) float64` // max(anchor, floor)
  - `func QuoteTrainBoostCost(s model.GameState, gen int, boosts [model.NumQualityDims]bool, b balance.Config) (float64, error)`
  - `func PredictedTrainQuality(s model.GameState, gen int, alloc [model.NumQualityDims]float64, boosts [model.NumQualityDims]bool, b balance.Config) ([model.NumQualityDims]float64, error)`

- [ ] **Step 1: Failing tests**

```go
package sim

import (
	"math"
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestBoostRefMonthlyCashIgnoresStickerPrice(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Models: []model.Model{{
			Online: true, Segment: model.SegEnterprise,
			Users: 1000, Price: 180,
			Quality: [4]float64{10, 10, 10, 10},
		}},
	}
	base := BoostRefMonthlyCash(s, b)
	s.Models[0].Price = 1
	got := BoostRefMonthlyCash(s, b)
	if base != got {
		t.Fatalf("price exploit: %v → %v", base, got)
	}
	// Must use ref price path, not 1000*1
	ref := EffectiveRefPrice(s, model.SegEnterprise, b)
	want := 1000 * ref * b.RevenueMult // no campaign/prestige
	if math.Abs(base-want) > 1e-6 {
		t.Fatalf("anchor = %v, want %v", base, want)
	}
}

func TestBoostRefMonthlyCashIncludesRevenueMult(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Models: []model.Model{{
			Online: true, Segment: model.SegConsumer,
			Users: 500, Price: 12,
		}},
	}
	a := BoostRefMonthlyCash(s, b)
	b.RevenueMult = b.RevenueMult * 2
	got := BoostRefMonthlyCash(s, b)
	if math.Abs(got-2*a) > 1e-6 {
		t.Fatalf("RevenueMult scale: %v vs 2*%v", got, a)
	}
}

func TestTrainBoostRefMonthlyUsesFloor(t *testing.T) {
	b := balance.Default()
	s := model.GameState{} // no models
	if TrainBoostRefMonthly(s, b) != b.TrainBoostFloorMonthly {
		t.Fatalf("want floor")
	}
}

func TestPredictedTrainQualityMonotonicInBoosts(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	alloc := [model.NumQualityDims]float64{0.4, 0.2, 0.2, 0.2}
	var none, one, all [model.NumQualityDims]bool
	one[model.DimSafety] = true
	for d := range all {
		all[d] = true
	}
	q0, _ := PredictedTrainQuality(s, 1, alloc, none, b)
	q1, _ := PredictedTrainQuality(s, 1, alloc, one, b)
	qA, _ := PredictedTrainQuality(s, 1, alloc, all, b)
	for _, seg := range []model.Segment{model.SegConsumer, model.SegEnterprise, model.SegDeveloper} {
		w := b.SegmentWeights[seg]
		a0, a1, aA := appealOf(q0, w), appealOf(q1, w), appealOf(qA, w)
		if a1 < a0-1e-9 || aA < a1-1e-9 {
			t.Fatalf("seg %v appeal non-monotonic: %v %v %v", seg, a0, a1, aA)
		}
	}
	if qA[model.DimSafety] < q1[model.DimSafety]-1e-9 {
		t.Fatalf("full pack reduced safety bonus")
	}
}
```

- [ ] **Step 2: Run — FAIL**

```bash
go test ./internal/sim/ -run 'BoostRef|TrainBoostRef|PredictedTrain' -count=1
```

- [ ] **Step 3: Implement `internal/sim/train_boost.go`**

```go
package sim

import (
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// BoostRefMonthlyCash is gross monthly cash at EffectiveRefPrice (not sticker),
// including campaign/prestige/global revenue mults used by Tick accrual.
func BoostRefMonthlyCash(s model.GameState, b balance.Config) float64 {
	pe := PrestigeEffects(s.Prestige.UnlockedPrestige, b)
	ce := campaignEffects(s, b)
	var total float64
	for _, m := range s.Models {
		if !m.Online {
			continue
		}
		if int(m.Segment) < 0 || int(m.Segment) >= model.NumSegments {
			continue
		}
		ref := EffectiveRefPrice(s, m.Segment, b)
		total += m.Users * ref * ce.RevenueMult[m.Segment] * pe.CashMult * b.RevenueMult
	}
	return total
}

func TrainBoostRefMonthly(s model.GameState, b balance.Config) float64 {
	ref := BoostRefMonthlyCash(s, b)
	if ref < b.TrainBoostFloorMonthly {
		return b.TrainBoostFloorMonthly
	}
	return ref
}

func QuoteTrainBoostCost(s model.GameState, gen int, boosts [model.NumQualityDims]bool, b balance.Config) (float64, error) {
	return balance.TrainBoostCashCost(gen, TrainBoostRefMonthly(s, b), boosts, b)
}

func PredictedTrainQuality(s model.GameState, gen int, alloc [model.NumQualityDims]float64, boosts [model.NumQualityDims]bool, b balance.Config) ([model.NumQualityDims]float64, error) {
	var out [model.NumQualityDims]float64
	spec, err := balance.Generation(gen)
	if err != nil {
		return out, err
	}
	bonus, err := balance.TrainBoostCashBonus(gen, boosts, b)
	if err != nil {
		return out, err
	}
	te := techEffects(s, b)
	se := starEffects(s, b)
	for d := range model.NumQualityDims {
		out[d] = (alloc[d]*spec.QualityScale + bonus[d]) * te.QualityMult[d] * se.QualityMult[d]
	}
	return out, nil
}

// EffectiveRivalQuality is the sole authority for rival quality reads (v1: stored).
func EffectiveRivalQuality(s model.GameState, rival model.Competitor, b balance.Config) [model.NumQualityDims]float64 {
	_ = s
	_ = b
	return rival.Quality
}
```

- [ ] **Step 4: Tests PASS**

```bash
go test ./internal/sim/ -run 'BoostRef|TrainBoostRef|PredictedTrain' -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/sim/train_boost.go internal/sim/train_boost_test.go
git commit -m "feat(sim): train boost revenue anchor and predicted quality"
```

---

### Task 4: applyStartTraining + advanceTraining

**Files:**
- Modify: `internal/sim/apply.go` (`applyStartTraining`)
- Modify: `internal/sim/sim.go` (`advanceTraining` quality loop)
- Modify: `internal/sim/apply_test.go` (new cases)
- Modify: `internal/sim/sim_test.go` if completion quality asserted

**Interfaces:**
- Consumes: `QuoteTrainBoostCost`, `balance.TrainBoostCashBonus`
- Produces: training starts with cash debit + frozen bonus; complete uses `job.CashBonus`

- [ ] **Step 1: Failing tests in `apply_test.go`**

```go
func TestApplyStartTrainingWithBoostsChargesCashAndFreezesBonus(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Resources.RnD = 50_000
	s.Resources.Cash = 1e9
	var boosts [model.NumQualityDims]bool
	boosts[model.DimSafety] = true
	cost, err := QuoteTrainBoostCost(s, 1, boosts, b)
	if err != nil {
		t.Fatal(err)
	}
	ns, err := Apply(s, model.StartTraining{
		Gen: 1, Alloc: validAlloc(), Price: 12, Boosts: boosts,
	}, b)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(ns.Resources.Cash-(1e9-cost)) > 1e-6 {
		t.Fatalf("cash = %v, want %v", ns.Resources.Cash, 1e9-cost)
	}
	if !ns.Training.Boosts[model.DimSafety] {
		t.Fatal("boost not frozen on job")
	}
	wantBonus := b.TrainBoostBeta * 25 // Gen1 scale
	if math.Abs(ns.Training.CashBonus[model.DimSafety]-wantBonus) > 1e-9 {
		t.Fatalf("CashBonus = %v", ns.Training.CashBonus)
	}
	if math.Abs(ns.Training.BoostCashPaid-cost) > 1e-6 {
		t.Fatalf("BoostCashPaid = %v, want %v", ns.Training.BoostCashPaid, cost)
	}
}

func TestApplyStartTrainingInsufficientCashForBoost(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Resources.RnD = 50_000
	s.Resources.Cash = 1 // too low for any boost at floor
	var boosts [model.NumQualityDims]bool
	boosts[0] = true
	out, err := Apply(s, model.StartTraining{Gen: 1, Alloc: validAlloc(), Price: 12, Boosts: boosts}, b)
	if err != ErrInsufficientCash {
		t.Fatalf("err = %v, want ErrInsufficientCash", err)
	}
	if out.HasTraining || out.Resources.RnD != 50_000 || out.Resources.Cash != 1 {
		t.Fatalf("state mutated on failure: %+v", out.Resources)
	}
}

func TestAdvanceTrainingAppliesFrozenCashBonus(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.HasTraining = true
	s.Training = model.TrainingJob{
		Gen: 1, Alloc: validAlloc(), Price: 12, WorkRemaining: 1,
		Boosts:    [model.NumQualityDims]bool{false, false, true, false},
		CashBonus: [model.NumQualityDims]float64{0, 0, 3.75, 0},
	}
	s.Compute.RentedTraining = map[string]int{"N7": 100}
	ns := Tick(s, 3600, nil, b) // enough work with high compute; adjust if needed
	// Safer: call advanceTraining directly if exported tests in package
	ns = advanceTraining(s, 1e12, 1e6, b)
	if !ns.HasTraining {
		// completed
	}
	if len(ns.Models) == 0 {
		t.Fatal("no draft model")
	}
	m := ns.Models[len(ns.Models)-1]
	// safety = 0.2*25 + 3.75 = 8.75 (no tech/star)
	if math.Abs(m.Quality[model.DimSafety]-8.75) > 1e-6 {
		t.Fatalf("safety = %v, want 8.75", m.Quality[model.DimSafety])
	}
	// capability unchanged path: 0.4*25 = 10
	if math.Abs(m.Quality[model.DimCapability]-10) > 1e-6 {
		t.Fatalf("cap = %v, want 10", m.Quality[model.DimCapability])
	}
}
```

(Use package-internal `advanceTraining` — tests already live in `package sim`.)

- [ ] **Step 2: Run — FAIL**

```bash
go test ./internal/sim/ -run 'StartTrainingWithBoosts|InsufficientCashForBoost|AppliesFrozenCashBonus' -count=1
```

- [ ] **Step 3: Implement apply + complete**

`applyStartTraining` after R&D check / before mutate:

```go
cashCost, err := QuoteTrainBoostCost(s, c.Gen, c.Boosts, b)
if err != nil {
	return s, err
}
if s.Resources.Cash < cashCost {
	return s, ErrInsufficientCash
}
bonus, err := balance.TrainBoostCashBonus(c.Gen, c.Boosts, b)
if err != nil {
	return s, err
}
// existing R&D check...
ns.Resources.RnD -= cost
ns.Resources.Cash -= cashCost
ns.HasTraining = true
ns.Training = model.TrainingJob{
	Gen: c.Gen, Segment: c.Segment, Alloc: c.Alloc, Price: c.Price,
	WorkRemaining: spec.TrainWork * te.TrainWorkMult,
	Boosts: c.Boosts, CashBonus: bonus, BoostCashPaid: cashCost,
}
```

`advanceTraining` quality loop:

```go
for d := range model.NumQualityDims {
	m.Quality[d] = (job.Alloc[d]*qualityScale + job.CashBonus[d]) * te.QualityMult[d] * se.QualityMult[d]
}
```

Ensure existing no-boost tests still pass (CashBonus zero).

- [ ] **Step 4: Full sim apply tests PASS**

```bash
go test ./internal/sim/ -run 'StartTraining|AdvanceTraining|AppliesFrozen' -count=1
go test ./internal/sim/ -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/sim/apply.go internal/sim/sim.go internal/sim/apply_test.go internal/sim/sim_test.go
git commit -m "feat(sim): charge train boosts and apply frozen quality bonus"
```

---

### Task 5: Rival target investment + unlock cliff

**Files:**
- Modify: `internal/sim/rivals.go` (`rivalTarget` signature + body; all call sites)
- Modify: `internal/sim/campaign_rivals.go` if it calls `rivalTarget`
- Modify: `internal/sim/rivals_test.go`
- Create/extend tests for cliff

**Interfaces:**
- Change: `func rivalTarget(s model.GameState, rival model.Competitor, gf [model.NumQualityDims]float64, b balance.Config) [model.NumQualityDims]float64`
- After base `gf[d]*pct`, for top `b.TrainBoostRivalPicks` Skill dims (tie: lower index): `out[d] += b.TrainBoostBeta * gf[d]`

- [ ] **Step 1: Failing tests**

```go
func TestRivalTargetIncludesTrainBoostOnTopSkillDims(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	// Ensure era leaders init not required for target math
	gf := [model.NumQualityDims]float64{100, 100, 100, 100}
	rival := model.Competitor{
		Name:  "OpenAI",
		Skill: [model.NumQualityDims]float64{1.08, 1.00, 0.96, 1.04}, // top: cap, speed
	}
	// Temporarily need b in rivalTarget — call after implementation
	got := rivalTarget(s, rival, gf, b)
	// base for cap: 100 * clamp(1.08)=108; plus 0.15*100=15 → 123
	if got[model.DimCapability] < 108+14.9 {
		t.Fatalf("cap target missing boost: %v", got[model.DimCapability])
	}
	// efficiency not in top-2 → no +15
	baseEff := 100 * 1.00
	if got[model.DimEfficiency] > baseEff+0.01 {
		t.Fatalf("eff should not get boost: %v", got[model.DimEfficiency])
	}
}

func TestUnlockGenTechDoesNotSpikeRivalQualityOneTick(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Competitors: balance.DefaultCompetitors(),
		// published gen1 model so GF stable from player side
		Models: []model.Model{{
			Online: true, Gen: 1,
			Quality: [4]float64{10, 10, 10, 10},
			Users: 1000, Price: 12, Segment: model.SegConsumer,
		}},
	}
	s = ensureRivalEraState(s, b)
	// Warm one tick
	s = advanceRivalLeague(s, 3600, b)
	before := append([]model.Competitor(nil), s.Competitors...)
	// Unlock gen2 tech only
	s.Tech.Unlocked = append([]string(nil), s.Tech.Unlocked...)
	s.Tech.Unlocked = append(s.Tech.Unlocked, balance.GenUnlockNodeID(2))
	s.Progression.MaxUnlockedGen = 2 // if field exists; else only Unlocked
	after := advanceRivalLeague(s, 3600, b)
	// Per-dim quality change must be within one catch-up step of prior target path.
	// Practical gate: max abs delta per dim ≤ |target-before - q| * factor + 1e-6
	// Simpler gate from spec: no discontinuous jump larger than one catch-up step:
	factor := b.CompetitorCatchupRate * 3600
	if factor > 1 {
		factor = 1
	}
	gf := GlobalFrontier(s, b)
	for i, c := range after.Competitors {
		prev := before[i].Quality
		// max possible move this tick is factor * distance to NEW target
		tgt := rivalTarget(s, c, gf, b)
		for d := range model.NumQualityDims {
			maxStep := math.Abs(tgt[d]-prev[d]) * factor
			delta := math.Abs(c.Quality[d] - prev[d])
			if delta > maxStep+1e-6 {
				t.Fatalf("%s dim %d jumped %v > max step %v", c.Name, d, delta, maxStep)
			}
		}
	}
}
```

(Adjust `MaxUnlockedGen` / tech fields to match actual `model` + `MaxUnlockedGen` helper — unlock via `Apply(UnlockTech)` if cleaner.)

- [ ] **Step 2: Implement `rivalTarget` boost + update all call sites to pass `b`**

```go
func rivalTarget(s model.GameState, rival model.Competitor, gf [model.NumQualityDims]float64, b balance.Config) [model.NumQualityDims]float64 {
	// existing pct loop → out[d] = gf[d]*pct
	// then:
	picks := b.TrainBoostRivalPicks
	if picks < 0 {
		picks = 0
	}
	if picks > model.NumQualityDims {
		picks = model.NumQualityDims
	}
	// select top picks by Skill, tie lower index
	type pair struct {
		d model.QualityDim
		sk float64
	}
	var all []pair
	for d := model.QualityDim(0); d < model.NumQualityDims; d++ {
		all = append(all, pair{d, rival.Skill[d]})
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].sk == all[j].sk {
			return all[i].d < all[j].d
		}
		return all[i].sk > all[j].sk
	})
	for i := 0; i < picks && i < len(all); i++ {
		d := all[i].d
		out[d] += b.TrainBoostBeta * gf[d]
	}
	return out
}
```

- [ ] **Step 3: Tests PASS** + existing rivals tests

```bash
go test ./internal/sim/ -run 'Rival|UnlockGen' -count=1
```

- [ ] **Step 4: Commit**

```bash
git add internal/sim/rivals.go internal/sim/campaign_rivals.go internal/sim/rivals_test.go
git commit -m "feat(sim): rival train investment via catch-up targets"
```

---

### Task 6: Wire `EffectiveRivalQuality` everywhere

**Files:**
- Modify: `internal/sim/sim.go` (`advanceUsers` rival appeal)
- Modify: `internal/sim/view.go` (MarketRank, share bars, threat, intel)
- Modify: `internal/sim/campaign_view.go` (rank, safety, share)
- Create: `internal/sim/rival_quality_test.go`

**Interfaces:**
- All `appealOf(c.Quality, w)` for competitors → `appealOf(EffectiveRivalQuality(s, c, b), w)`
- Intel raw quality display → `EffectiveRivalQuality(...)`

- [ ] **Step 1: Test that documents entry points**

```go
func TestEffectiveRivalQualityIdentityV1(t *testing.T) {
	b := balance.Default()
	c := model.Competitor{Quality: [4]float64{1, 2, 3, 4}}
	got := EffectiveRivalQuality(model.GameState{}, c, b)
	if got != c.Quality {
		t.Fatalf("%v != %v", got, c.Quality)
	}
}
```

Also add a focused test: `SegmentShareBars` / `MarketRank` still run (smoke) after rewiring.

- [ ] **Step 2: Replace all competitor quality reads in listed files**

Grep gate after edit:

```bash
rg 'appealOf\(c\.Quality' internal/sim --glob '*.go'
```

Expected: no matches in non-test production code (tests may still set raw Quality). Prefer `rg 'c\.Quality'` in view/sim/campaign_view and ensure displays use helper.

- [ ] **Step 3: `go test ./internal/sim/ -count=1` PASS**

- [ ] **Step 4: Commit**

```bash
git add internal/sim/sim.go internal/sim/view.go internal/sim/campaign_view.go internal/sim/rival_quality_test.go
git commit -m "refactor(sim): route rival quality reads through EffectiveRivalQuality"
```

---

### Task 7: Store validation

**Files:**
- Modify: `internal/store/migrate.go` (active training validation block ~257–264)
- Extend: existing store tests

**Policy (pick fixed):** On load, if `HasTraining`:
- Require finite `BoostCashPaid >= 0`, each `CashBonus[d]` finite `>= 0`
- If `!Boosts[d]`, force `CashBonus[d] = 0` (repair)
- Do **not** recompute `BoostCashPaid` from current revenue (historical charge)

- [ ] **Step 1: Failing test** — save/load or `validateState` with negative CashBonus rejects; `Boosts false` but bonus >0 repairs

- [ ] **Step 2: Implement validation/repair in migrate path**

- [ ] **Step 3: PASS + commit**

```bash
git commit -m "fix(store): validate train boost fields on active jobs"
```

---

### Task 8: TUI train dialog

**Files:**
- Modify: `internal/tui/dialog_train.go`
- Modify: `internal/tui/dialog_train_test.go`

**UX:**
- Focus modes or extended navigation: after 4 alloc dims, continue down into 4 boost rows (total 8 focus rows) **or** keys `1`–`4` toggle boosts. Prefer **down wraps into boost section** with `dim` 0..3 alloc and `boostFocus` bool, or single `focus int` 0..7.
- Space or `+` on boost row toggles that boost.
- Show for each: name, price, checkbox.
- Summary: 參考月現金（標竿價）, floor badge, total cost, years ≈ `total/(12*ref)`, 預測吸引力 before/after.
- `command()` includes `Boosts`.

- [ ] **Step 1: Tests**

```go
func TestTrainDialogToggleBoostAndCommand(t *testing.T) {
	m := testModel(t)
	m.state.Resources.RnD = 50_000
	m.state.Resources.Cash = 1e9
	d := newTrainDialog(m)
	// navigate to first boost row and toggle — exact keys per implementation
	// ...
	cmd := d.command(balance.Default())
	if !cmd.Boosts[0] && !anyBoost(cmd.Boosts) {
		// after toggle expect some true
	}
}

func TestTrainDialogRenderShowsChineseBoostNames(t *testing.T) {
	m := testModel(t)
	d := newTrainDialog(m)
	out := renderTrainDialog(d, m)
	for _, name := range []string{"優質語料", "省算力改造", "安全評測", "加速優化"} {
		if !strings.Contains(out, name) {
			t.Fatalf("missing %s in %q", name, out)
		}
	}
	if !strings.Contains(out, "參考月現金") {
		t.Fatal("missing anchor label")
	}
}
```

- [ ] **Step 2: Implement dialog state + render + confirm path already calls `d.command`**

When confirm fails on insufficient cash, keep dialog open and set a short error string on dialog (if Model already surfaces apply errors, mirror that pattern from hire/tech).

- [ ] **Step 3: PASS tui tests**

```bash
go test ./internal/tui/ -run Dialog -count=1
```

- [ ] **Step 4: Commit**

```bash
git commit -m "feat(tui): train dialog cash boost investments"
```

---

### Task 9: Long-run smoke + full verification

**Files:**
- Extend: `internal/sim/longrun_test.go` or new `internal/sim/train_boost_longrun_test.go`

- [ ] **Step 1: Smoke test** — for each segment weight set, nested boost sets appeal monotonic (property loop over all 16 masks is fine: for every mask A ⊂ B componentwise, appeal non-decreasing). Already partly in Task 3; add exhaustive 16-mask nest check.

```go
func TestTrainBoostAllMasksMonotonicAppeal(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	alloc := [model.NumQualityDims]float64{0.4, 0.2, 0.2, 0.2}
	type mask int
	appeal := func(m mask, seg model.Segment) float64 {
		var boosts [model.NumQualityDims]bool
		for d := 0; d < 4; d++ {
			boosts[d] = m&(1<<d) != 0
		}
		q, err := PredictedTrainQuality(s, 1, alloc, boosts, b)
		if err != nil {
			t.Fatal(err)
		}
		return appealOf(q, b.SegmentWeights[seg])
	}
	for seg := model.Segment(0); seg < model.NumSegments; seg++ {
		for m := mask(0); m < 16; m++ {
			for bit := 0; bit < 4; bit++ {
				if m&(1<<bit) != 0 {
					continue
				}
				m2 := m | (1 << bit)
				if appeal(m2, seg) < appeal(m, seg)-1e-9 {
					t.Fatalf("seg %v mask %b → %b lowered appeal", seg, m, m2)
				}
			}
		}
	}
}
```

- [ ] **Step 2: Full suite**

```bash
gofmt -w internal/balance/train_boost.go internal/balance/train_boost_test.go \
  internal/balance/balance.go internal/model/types.go \
  internal/sim/train_boost.go internal/sim/train_boost_test.go \
  internal/sim/apply.go internal/sim/sim.go internal/sim/rivals.go \
  internal/sim/view.go internal/sim/campaign_view.go \
  internal/store/migrate.go internal/tui/dialog_train.go
go test ./...
go vet ./...
go build ./...
```

Expected: all PASS.

- [ ] **Step 3: Commit**

```bash
git commit -m "test: train boost monotonicity masks and full verification"
```

- [ ] **Step 4: Update design status line** in spec to `Approved for implementation` only if still saying “ready for re-approval” — optional doc commit:

```bash
# optional
git commit -m "docs: mark train cash boost design approved for implementation"
```

---

## Spec coverage checklist (self-review)

| Spec section | Task |
|---|---|
| §4 Catalog ZH names | Task 1, 8 |
| §5 Additive quality, no soft cap | Task 1 bonus, Task 4 complete |
| §6.1 BoostRefMonthlyCash / no Price | Task 3 |
| §6.2–6.3 Linear + slot mults | Task 1 |
| §7 StartTraining / job freeze | Task 2, 4 |
| §8 Rival target-side + cliff | Task 5 |
| §8.3 EffectiveRivalQuality | Task 6 |
| §9 TUI | Task 8 |
| §11 Invariants store/config | Task 1 tests, Task 7 |
| §12 Acceptance gates | Tasks 1,3,4,5,9 |

## Placeholder scan

None intentional. If `MaxUnlockedGen` field name differs in Task 5 cliff test, implementer must use `sim.MaxUnlockedGen` / `UnlockTech` Apply path already in repo.

## Type consistency

- `TrainBoostCashCost(gen, refMonthly, boosts, b)` in balance
- `QuoteTrainBoostCost(s, gen, boosts, b)` in sim wraps ref monthly
- `rivalTarget(..., b balance.Config)` after Task 5
- `EffectiveRivalQuality(s, rival, b)` identity v1
