# Model Publish Flow & Gradual User Growth Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Training completes into an offline draft; the player publishes with a custom name and price; users ramp over ~8 sim-hours toward a Gen1 equilibrium of ~2–5万 instead of instantly capping near 1k.

**Architecture:** Keep sim pure. `advanceTraining` appends `Online=false` drafts. New command `PublishModel{Index,Name,Price}` flips a draft live. `advanceUsers` switches to exact exponential approach `target+(users-target)*exp(-rate*dt)` and balance raises `SegmentTargetScale` / lowers `UserGrowthRate`. TUI models page splits draft/live lists and adds a publish modal (name + price + estimate).

**Tech Stack:** Go 1.22+, Bubble Tea, Lipgloss. No new dependencies. Spec: `docs/superpowers/specs/2026-07-09-model-publish-growth-design.md`.

## Global Constraints

- Module `tokensmith`; `internal/sim` stays pure (no wall-clock/rand/IO; time only via `dt`).
- Draft identity v1: `Online==false && Users==0` (no `Status` enum).
- Name: trim, 1–24 runes, non-empty on publish; duplicate names allowed.
- `P` (shift+p) remains PrestigeReset on overview/tech — publish binds **lowercase `p`** on models page only.
- Old saves: missing `name` → `""`; already-online models stay live (show `（未命名）`).
- Spec out of scope: draft delete, rename live models, launch marketing burst, valuation rebalance.
- Prefer TDD: failing test → implement → green → commit per task.

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/model/types.go` | `Model.Name`; `PublishModel` command |
| `internal/sim/apply.go` | `applyPublishModel`; errors `ErrNotDraft`, `ErrInvalidName` |
| `internal/sim/sim.go` | Draft on train complete; exponential `advanceUsers` |
| `internal/sim/view.go` | `EstimateUserTarget`, `IsDraft`, display helpers |
| `internal/balance/balance.go` | `UserGrowthRate`, scales, `UserTargetPerAppeal` |
| `internal/tui/page_models.go` | Draft/live lists, cursor |
| `internal/tui/dialog_publish.go` | Publish modal (name + price) |
| `internal/tui/tui.go` | Wire `p` / `$` / cursor; dual-dialog routing |
| Tests beside each package | |

---

### Task 1: Model.Name + PublishModel command type

**Files:**
- Modify: `internal/model/types.go`
- Modify: `internal/model/types_test.go` (if present patterns need Name zero-value)

**Interfaces:**
- Produces: `Model.Name string`; `type PublishModel struct { ModelIndex int; Name string; Price float64 }` with `commandMarker()`

- [ ] **Step 1: Add fields**

In `Model` struct after `Online`:

```go
// Model is a trained AI model.
type Model struct {
	Gen     int
	Segment Segment
	Quality [NumQualityDims]float64
	Users   float64
	Price   float64 // per user per month; player-set
	Online  bool
	Name    string // player-chosen at publish; empty while draft
}
```

Add command next to `StartTraining` / `SetPrice`:

```go
// PublishModel names, prices, and onlines a draft model (Online==false, Users==0).
type PublishModel struct {
	ModelIndex int
	Name       string
	Price      float64
}

func (PublishModel) commandMarker() {}
```

- [ ] **Step 2: Compile check**

Run: `go test ./internal/model/ -count=1`  
Expected: PASS (pure type change).

- [ ] **Step 3: Commit**

```bash
git add internal/model/types.go
git commit -m "feat(model): add Model.Name and PublishModel command"
```

---

### Task 2: Training completes as draft (not auto-online)

**Files:**
- Modify: `internal/sim/sim.go` (`advanceTraining`)
- Modify: `internal/sim/sim_test.go` (`TestTickTrainingCompletes` and related)

**Interfaces:**
- Consumes: `Model.Name`
- Produces: completed models with `Online=false`, `Users=0`, `Name=""`

- [ ] **Step 1: Update failing expectations in tests**

Change `TestTickTrainingCompletes` so the completed model is a draft:

```go
func TestTickTrainingCompletes(t *testing.T) {
	b := balance.Default()
	s := model.GameState{HasTraining: true}
	s.Compute.RentedTraining = map[string]int{"N7": 10}
	s.Training = model.TrainingJob{
		Gen:           2,
		Alloc:         [model.NumQualityDims]float64{0.4, 0.2, 0.2, 0.2},
		Price:         12,
		WorkRemaining: 7200,
	}
	ns := Tick(s, 1000, nil, b)
	if ns.HasTraining {
		t.Fatalf("training should be done")
	}
	if len(ns.Models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(ns.Models))
	}
	m := ns.Models[0]
	if m.Online || m.Users != 0 || m.Name != "" {
		t.Fatalf("completed model should be draft: %+v", m)
	}
	if m.Gen != 2 || m.Price != 12 {
		t.Fatalf("model fields wrong: %+v", m)
	}
	if !approx(m.Quality[model.DimCapability], 18) {
		t.Errorf("capability = %v, want 18", m.Quality[model.DimCapability])
	}
	if !approx(m.Quality[model.DimSafety], 9) {
		t.Errorf("safety = %v, want 9", m.Quality[model.DimSafety])
	}
	if len(s.Models) != 0 {
		t.Errorf("Tick mutated input Models")
	}
}
```

Add:

```go
func TestTickTrainingCompleteAllowsNewTraining(t *testing.T) {
	b := balance.Default()
	s := model.GameState{HasTraining: true, Resources: model.Resources{RnD: 1e9}}
	s.Compute.RentedTraining = map[string]int{"N7": 100}
	s.Training = model.TrainingJob{
		Gen: 1, Alloc: [model.NumQualityDims]float64{1, 0, 0, 0},
		Price: 12, WorkRemaining: 1,
	}
	s = Tick(s, 1, nil, b)
	if s.HasTraining || len(s.Models) != 1 || s.Models[0].Online {
		t.Fatalf("want one draft, no active job: %+v", s)
	}
	ns, err := Apply(s, model.StartTraining{
		Gen: 1, Segment: model.SegConsumer,
		Alloc: [model.NumQualityDims]float64{1, 0, 0, 0}, Price: 12,
	}, b)
	if err != nil {
		t.Fatalf("should allow new training while draft exists: %v", err)
	}
	if !ns.HasTraining {
		t.Fatal("expected new training job")
	}
}
```

- [ ] **Step 2: Run tests — expect FAIL**

Run: `go test ./internal/sim/ -run 'TestTickTrainingComplete' -count=1`  
Expected: FAIL on `Online` still true.

- [ ] **Step 3: Implement draft completion**

In `advanceTraining`, change model construction:

```go
	m := model.Model{
		Gen: job.Gen, Segment: job.Segment, Price: job.Price,
		Online: false, Users: 0, Name: "",
	}
```

Update comment: `// Completed → append draft (not online until PublishModel).`

- [ ] **Step 4: Fix any other tests that assumed auto-online**

Search and update:

```bash
rg -n "Online:\s*true|m\.Online|training should|Completes" internal/sim/ internal/tui/ --glob '*_test.go'
```

Any test that Tick-completes training and expects live users must either `PublishModel` first or assert draft. Common pattern after complete:

```go
ns, err = Apply(ns, model.PublishModel{ModelIndex: 0, Name: "t", Price: 12}, b)
```

(Publish not implemented yet — for tests that only check quality/gen, assert `!Online` instead.)

If a test needs users growing after complete, **defer fixing those to Task 3** by temporarily marking them or completing publish in Task 3. Prefer fixing `TestTickTrainingCompletes` now; leave user-growth tests until Publish exists if they break.

Run: `go test ./internal/sim/ -count=1`  
Fix remaining compile/assert failures that only need `Online: false` expectation.

- [ ] **Step 5: Commit**

```bash
git add internal/sim/sim.go internal/sim/sim_test.go
git commit -m "feat(sim): training completes as offline draft"
```

---

### Task 3: Apply PublishModel

**Files:**
- Modify: `internal/sim/apply.go`
- Modify: `internal/sim/apply_test.go`

**Interfaces:**
- Produces: `ErrNotDraft`, `ErrInvalidName`; `applyPublishModel`
- Consumes: `model.PublishModel`

- [ ] **Step 1: Write failing tests** (append `apply_test.go`)

```go
func TestApplyPublishModel(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Models: []model.Model{{
			Gen: 1, Segment: model.SegConsumer, Price: 12,
			Online: false, Users: 0,
			Quality: [model.NumQualityDims]float64{25, 0, 0, 0},
		}},
	}
	ns, err := Apply(s, model.PublishModel{ModelIndex: 0, Name: "  Nova-1  ", Price: 9}, b)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if !ns.Models[0].Online || ns.Models[0].Name != "Nova-1" || ns.Models[0].Price != 9 {
		t.Fatalf("published model wrong: %+v", ns.Models[0])
	}
	// purity
	if s.Models[0].Online || s.Models[0].Name != "" {
		t.Fatal("Apply mutated input")
	}
}

func TestApplyPublishModelRejects(t *testing.T) {
	b := balance.Default()
	draft := model.Model{Online: false, Users: 0}
	live := model.Model{Online: true, Users: 0, Name: "x"}
	used := model.Model{Online: false, Users: 10} // not a draft

	s := model.GameState{Models: []model.Model{draft, live, used}}
	cases := []struct {
		cmd model.PublishModel
		err error
	}{
		{model.PublishModel{ModelIndex: -1, Name: "a", Price: 1}, ErrInvalidModelIndex},
		{model.PublishModel{ModelIndex: 99, Name: "a", Price: 1}, ErrInvalidModelIndex},
		{model.PublishModel{ModelIndex: 1, Name: "a", Price: 1}, ErrNotDraft},
		{model.PublishModel{ModelIndex: 2, Name: "a", Price: 1}, ErrNotDraft},
		{model.PublishModel{ModelIndex: 0, Name: "   ", Price: 1}, ErrInvalidName},
		{model.PublishModel{ModelIndex: 0, Name: strings.Repeat("字", 25), Price: 1}, ErrInvalidName},
		{model.PublishModel{ModelIndex: 0, Name: "ok", Price: 0}, ErrInvalidPrice},
		{model.PublishModel{ModelIndex: 0, Name: "ok", Price: -1}, ErrInvalidPrice},
	}
	for _, tc := range cases {
		if _, err := Apply(s, tc.cmd, b); !errors.Is(err, tc.err) {
			t.Errorf("cmd=%+v err=%v want %v", tc.cmd, err, tc.err)
		}
	}
}
```

Add imports: `"errors"`, `"strings"` if missing.

- [ ] **Step 2: Run — expect FAIL**

Run: `go test ./internal/sim/ -run TestApplyPublishModel -count=1`  
Expected: FAIL (unknown command or missing errors).

- [ ] **Step 3: Implement**

In `apply.go` error vars:

```go
	ErrNotDraft     = errors.New("sim: model is not a publishable draft")
	ErrInvalidName  = errors.New("sim: model name must be 1–24 characters")
```

In `Apply` switch:

```go
	case model.PublishModel:
		return applyPublishModel(s, c)
```

```go
func applyPublishModel(s model.GameState, c model.PublishModel) (model.GameState, error) {
	if c.ModelIndex < 0 || c.ModelIndex >= len(s.Models) {
		return s, ErrInvalidModelIndex
	}
	m := s.Models[c.ModelIndex]
	if m.Online || m.Users != 0 {
		return s, ErrNotDraft
	}
	name := strings.TrimSpace(c.Name)
	if name == "" || utf8.RuneCountInString(name) > 24 {
		return s, ErrInvalidName
	}
	if c.Price <= 0 {
		return s, ErrInvalidPrice
	}
	ns := s
	ns.Models = append([]model.Model(nil), s.Models...)
	ns.Models[c.ModelIndex].Name = name
	ns.Models[c.ModelIndex].Price = c.Price
	ns.Models[c.ModelIndex].Online = true
	return ns, nil
}
```

Imports: `"strings"`, `"unicode/utf8"`.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/sim/ -count=1`  
Expected: PASS. Fix any leftover auto-online assumptions by publishing in those tests.

- [ ] **Step 5: Commit**

```bash
git add internal/sim/apply.go internal/sim/apply_test.go internal/sim/sim_test.go
git commit -m "feat(sim): PublishModel names, prices, and onlines drafts"
```

---

### Task 4: Exponential user growth + EstimateUserTarget

**Files:**
- Modify: `internal/sim/sim.go` (`advanceUsers`)
- Modify: `internal/sim/view.go` (add helpers)
- Modify: `internal/sim/sim_test.go`, `internal/sim/view_test.go`

**Interfaces:**
- Produces:
  - `func IsDraft(m model.Model) bool`
  - `func EstimateUserTarget(s model.GameState, modelIndex int, price float64, b balance.Config) float64`
- Changes: `advanceUsers` uses `math.Exp`

- [ ] **Step 1: Write growth tests**

```go
func TestTickUserGrowthExponentialNotInstant(t *testing.T) {
	b := balance.Default()
	// Force known rate for the assertion regardless of balance defaults during this task.
	b.UserGrowthRate = 3.5e-5
	b.SegmentTargetScale[model.SegConsumer] = 20000
	m := onlineModel(25, b.SegmentRefPrice[model.SegConsumer]) // appeal 10 if weights 0.4
	m.Users = 0
	// No competitors → share 1 → target = 10 * 20000 = 200000
	s := model.GameState{Models: []model.Model{m}}
	ns := Tick(s, 3600, nil, b) // one sim hour
	// remaining = exp(-3.5e-5*3600)=exp(-0.126)≈0.881 → users≈0.119*target
	if ns.Models[0].Users <= 0 {
		t.Fatal("expected some users after 1h")
	}
	if ns.Models[0].Users > 0.25*200000 {
		t.Fatalf("1h users=%v; want < 25%% of target (not instant fill)", ns.Models[0].Users)
	}
}

func TestTickUserGrowthEightHoursNear63Percent(t *testing.T) {
	b := balance.Default()
	b.UserGrowthRate = 3.5e-5
	b.SegmentTargetScale[model.SegConsumer] = 20000
	m := onlineModel(25, b.SegmentRefPrice[model.SegConsumer])
	s := model.GameState{Models: []model.Model{m}}
	const target = 200000.0
	for i := 0; i < 8; i++ {
		s = Tick(s, 3600, nil, b)
	}
	u := s.Models[0].Users
	if u < 0.50*target || u > 0.75*target {
		t.Fatalf("after 8h users=%v; want ~63%% of %v (50–75%%)", u, target)
	}
}
```

Note: `onlineModel` helper already sets `Online: true`. Update `TestTickUserGrowthTowardTarget` if it still expects `Users==20` after 1s with old rate — either set `b.UserGrowthRate = 0.001` locally for that legacy unit test **or** recompute expected with new formula. Prefer **local override** in old tests so they keep testing the math shape:

```go
b.UserGrowthRate = 0.001 // legacy 1s step expectation
```

For one-second Euler-equivalent: `exp(-0.001)≈0.999` → users ≈ target*0.001 still holds approximately for small rate*dt — with rate 0.001, dt=1: users = target*(1-exp(-0.001)) ≈ target*0.001. Same as old Euler. Good — old tests with rate 0.001 still pass if Default changes later; pin rate in those tests.

- [ ] **Step 2: Run — expect FAIL** (instant fill or wrong levels)

Run: `go test ./internal/sim/ -run 'TestTickUserGrowth' -count=1`

- [ ] **Step 3: Implement exponential advanceUsers**

Replace growth block in `advanceUsers`:

```go
		// Exact exponential approach so large dt (TUI/offline 1h chunks) cannot
		// jump to the target in a single step.
		decay := math.Exp(-b.UserGrowthRate * dt)
		m.Users = target + (m.Users-target)*decay
		if m.Users < 0 {
			m.Users = 0
		}
```

Ensure `"math"` imported in `sim.go`.

- [ ] **Step 4: Add view helpers**

In `view.go`:

```go
// IsDraft reports whether m is a publishable draft (v1: offline and never used).
func IsDraft(m model.Model) bool {
	return !m.Online && m.Users == 0
}

// EstimateUserTarget is the equilibrium user count advanceUsers would approach
// for models[modelIndex] if it were online at the given price. Returns 0 if
// index invalid. Pure.
func EstimateUserTarget(s model.GameState, modelIndex int, price float64, b balance.Config) float64 {
	if modelIndex < 0 || modelIndex >= len(s.Models) {
		return 0
	}
	m := s.Models[modelIndex]
	if int(m.Segment) < 0 || int(m.Segment) >= model.NumSegments {
		return 0
	}
	if price <= 0 {
		return 0
	}
	w := b.SegmentWeights[m.Segment]
	appeal := appealOf(m.Quality, w)
	rivalAppeal := 0.0
	for _, c := range s.Competitors {
		rivalAppeal += appealOf(c.Quality, w)
	}
	share := 1.0
	if appeal+rivalAppeal > 0 {
		share = appeal / (appeal + rivalAppeal)
	}
	te := techEffects(s, b)
	se := starEffects(s, b)
	refPrice := b.SegmentRefPrice[m.Segment] * te.RefPriceMult
	demandMult := math.Pow(refPrice/price, b.PriceElasticity)
	marketingMult := 1 + float64(s.Marketing)*b.MarketingBonus
	return appeal * b.SegmentTargetScale[m.Segment] * demandMult * share *
		marketingMult * te.UserGrowthMult * se.UserGrowthMult
}
```

Add `view_test.go` case:

```go
func TestEstimateUserTargetPriceElasticity(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Models: []model.Model{{
			Online: false, Segment: model.SegConsumer,
			Quality: [model.NumQualityDims]float64{25, 0, 0, 0},
		}},
	}
	low := EstimateUserTarget(s, 0, 6, b)
	ref := EstimateUserTarget(s, 0, 12, b)
	high := EstimateUserTarget(s, 0, 24, b)
	if !(low > ref && ref > high && high > 0) {
		t.Fatalf("expected low>ref>high>0; got %v %v %v", low, ref, high)
	}
}
```

- [ ] **Step 5: Green + commit**

```bash
go test ./internal/sim/ -count=1
git add internal/sim/sim.go internal/sim/view.go internal/sim/sim_test.go internal/sim/view_test.go
git commit -m "feat(sim): exponential user growth and EstimateUserTarget"
```

---

### Task 5: Balance re-tune (growth rate + target scales)

**Files:**
- Modify: `internal/balance/balance.go`
- Modify: `internal/balance/balance_test.go`

**Interfaces:**
- Produces: `UserGrowthRate=3.5e-5`, `UserTargetPerAppeal=20000`, `SegmentTargetScale={20000,10000,16000}`

- [ ] **Step 1: Update balance_test expectations**

```go
	if c.UserTargetPerAppeal != 20000 || c.UserGrowthRate != 3.5e-5 {
		t.Errorf("user growth params wrong: target=%v rate=%v", c.UserTargetPerAppeal, c.UserGrowthRate)
	}
```

Keep segment mirror tests (consumer scale == UserTargetPerAppeal).

- [ ] **Step 2: Change Default()**

```go
	c.UserTargetPerAppeal = 20000
	c.UserGrowthRate = 3.5e-5
	// ...
	c.SegmentTargetScale = [model.NumSegments]float64{20000, 10000, 16000}
```

- [ ] **Step 3: Fix sim tests that hard-coded old 1000 scale / 0.001 rate**

Run: `go test ./... -count=1`  
For failures in economy/user tests, either update expected numbers or pin local `b.UserGrowthRate` / scales. `TestTickUserGrowthTowardTarget` should pin:

```go
	b.UserGrowthRate = 0.001
	b.SegmentTargetScale[model.SegConsumer] = 1000
	// or use UserTargetPerAppeal if that test still uses legacy scalar path
```

Actually advanceUsers uses `SegmentTargetScale` only — pin both rate and scale for legacy unit tests that encode exact user deltas.

- [ ] **Step 4: Commit**

```bash
go test ./... -count=1
git add internal/balance/balance.go internal/balance/balance_test.go internal/sim/
git commit -m "balance: slower user ramp and ~2–5万 Gen1 targets"
```

---

### Task 6: TUI models page — draft/live lists + cursor

**Files:**
- Modify: `internal/tui/page_models.go`
- Modify: `internal/tui/page_models_test.go`
- Modify: `internal/tui/tui.go` (add `modelCursor int` on `Model`)

**Interfaces:**
- Consumes: `sim.IsDraft`
- Produces: render with two sections; cursor index into `state.Models`

- [ ] **Step 1: Extend Model struct** in `tui.go`:

```go
	modelCursor int // selected index into state.Models on models page
```

- [ ] **Step 2: Rewrite `renderModels`**

```go
func renderModels(m Model) string {
	var b strings.Builder
	b.WriteString("模型\n")
	if len(m.state.Models) == 0 {
		b.WriteString("  （無 — 按 t 訓練第一個模型）\n")
		b.WriteString(helpStyle.Render("\n[t]訓練新模型 [Tab]切頁"))
		return b.String()
	}
	draftN := 0
	for _, md := range m.state.Models {
		if sim.IsDraft(md) {
			draftN++
		}
	}
	if draftN > 0 {
		b.WriteString(fmt.Sprintf("有 %d 個待發佈\n", draftN))
	}
	b.WriteString("── 待發佈 ──\n")
	anyDraft := false
	for i, md := range m.state.Models {
		if !sim.IsDraft(md) {
			continue
		}
		anyDraft = true
		cur := "  "
		if i == m.modelCursor {
			cur = "▸ "
		}
		b.WriteString(fmt.Sprintf("%s[%d] Gen%d · %s · 能力 %.0f\n",
			cur, i, md.Gen, segmentName(md.Segment), md.Quality[model.DimCapability]))
	}
	if !anyDraft {
		b.WriteString("  （無）\n")
	}
	b.WriteString("── 營運中 ──\n")
	anyLive := false
	for i, md := range m.state.Models {
		if sim.IsDraft(md) {
			continue
		}
		anyLive = true
		cur := "  "
		if i == m.modelCursor {
			cur = "▸ "
		}
		name := md.Name
		if name == "" {
			name = "（未命名）"
		}
		status := "上線"
		if !md.Online {
			status = "離線"
		}
		b.WriteString(fmt.Sprintf("%s[%d] 「%s」 Gen%d · %s · 用戶 %s · $%.0f · %s\n",
			cur, i, name, md.Gen, segmentName(md.Segment), human(md.Users), md.Price, status))
	}
	if !anyLive {
		b.WriteString("  （無）\n")
	}
	b.WriteString(helpStyle.Render("\n[↑↓]選模型 [p]發佈 [t]訓練 [$]改價 [Tab]切頁"))
	return b.String()
}
```

- [ ] **Step 3: Wire up/down on models page** in `tui.go` key handler (alongside tech/compute):

```go
		case "up":
			if m.page == PageTech && m.techCursor > 0 {
				m.techCursor--
			}
			if m.page == PageCompute && m.procCursor > 0 {
				m.procCursor--
			}
			if m.page == PageModels && m.modelCursor > 0 {
				m.modelCursor--
			}
			return m, nil
		case "down":
			if m.page == PageTech && m.techCursor < len(m.cfg.TechNodes)-1 {
				m.techCursor++
			}
			if m.page == PageCompute && m.procCursor < len(m.cfg.Processes)-1 {
				m.procCursor++
			}
			if m.page == PageModels && m.modelCursor < len(m.state.Models)-1 {
				m.modelCursor++
			}
			return m, nil
```

Clamp `modelCursor` when models shrink (e.g. start of Update):

```go
	if m.modelCursor >= len(m.state.Models) && len(m.state.Models) > 0 {
		m.modelCursor = len(m.state.Models) - 1
	}
	if len(m.state.Models) == 0 {
		m.modelCursor = 0
	}
```

- [ ] **Step 4: Tests**

```go
func TestRenderModelsShowsDraftAndLive(t *testing.T) {
	m := testModel(t)
	m.state.Models = []model.Model{
		{Gen: 1, Segment: model.SegConsumer, Online: false, Users: 0, Quality: [model.NumQualityDims]float64{25}},
		{Gen: 1, Name: "Nova", Online: true, Users: 500, Price: 12, Segment: model.SegConsumer},
	}
	v := renderModels(m)
	if !strings.Contains(v, "待發佈") || !strings.Contains(v, "營運中") {
		t.Fatalf("missing sections: %s", v)
	}
	if !strings.Contains(v, "Nova") {
		t.Fatalf("missing live name: %s", v)
	}
}
```

Run: `go test ./internal/tui/ -run TestRenderModels -count=1`

- [ ] **Step 5: Commit**

```bash
git add internal/tui/page_models.go internal/tui/page_models_test.go internal/tui/tui.go
git commit -m "feat(tui): models page lists drafts vs live with cursor"
```

---

### Task 7: Publish dialog + keys (`p`, optional `$`)

**Files:**
- Create: `internal/tui/dialog_publish.go`
- Create: `internal/tui/dialog_publish_test.go`
- Modify: `internal/tui/tui.go` (dialog routing)
- Modify: `internal/tui/page_overview.go` (optional draft hint)
- Modify: `internal/tui/settle.go` / banner if easy — training completed already set; ensure copy mentions 發佈

**Interfaces:**
- Produces: `publishDialog` with `update`, `command() model.PublishModel`, `renderPublishDialog`
- Note: existing `m.dialog` is `*trainDialog`. Add `m.publish *publishDialog`. In `Update` key path: if `m.publish != nil` handle it first; else if `m.dialog != nil` train.

- [ ] **Step 1: publishDialog type**

```go
// publishDialog is the name+price modal for onlining a draft.
type publishDialog struct {
	index    int
	name     string
	price    float64
	refPrice float64
	// gen/segment/quality snapshot for display only
	gen     int
	segment model.Segment
	quality [model.NumQualityDims]float64
}

func newPublishDialog(m Model, index int) (publishDialog, bool) {
	if index < 0 || index >= len(m.state.Models) {
		return publishDialog{}, false
	}
	md := m.state.Models[index]
	if !sim.IsDraft(md) {
		return publishDialog{}, false
	}
	ref := m.cfg.SegmentRefPrice[md.Segment] // tech mult applied in estimate via full state
	// Prefer job-carried price if positive, else segment ref.
	price := md.Price
	if price <= 0 {
		price = ref
	}
	name := fmt.Sprintf("Gen%d-%s", md.Gen, segmentName(md.Segment))
	return publishDialog{
		index: index, name: name, price: price, refPrice: ref,
		gen: md.Gen, segment: md.Segment, quality: md.Quality,
	}, true
}

func (d publishDialog) command() model.PublishModel {
	return model.PublishModel{ModelIndex: d.index, Name: d.name, Price: d.price}
}

func (d publishDialog) update(msg tea.KeyMsg) (next publishDialog, confirm, cancel bool) {
	switch msg.String() {
	case "esc":
		return d, false, true
	case "enter":
		return d, true, false
	case "left":
		d.price -= 1
		if d.price < 1 {
			d.price = 1
		}
	case "right":
		d.price += 1
	case "shift+left":
		d.price -= 5
		if d.price < 1 {
			d.price = 1
		}
	case "shift+right":
		d.price += 5
	case "backspace":
		if r := []rune(d.name); len(r) > 0 {
			d.name = string(r[:len(r)-1])
		}
	default:
		// Append single runes from plain typing; ignore multi-rune/control.
		if len(msg.Runes) == 1 {
			r := msg.Runes[0]
			if r >= 32 && utf8.RuneCountInString(d.name) < 24 {
				d.name += string(r)
			}
		}
	}
	return d, false, false
}

func renderPublishDialog(d publishDialog, m Model) string {
	est := sim.EstimateUserTarget(m.state, d.index, d.price, m.cfg)
	te := /* optional: show demand mult */
	demand := 1.0
	if d.price > 0 && d.refPrice > 0 {
		demand = math.Pow(d.refPrice/d.price, m.cfg.PriceElasticity)
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf(
		"發佈模型\n  Gen%d · %s · 能力 %.0f / 成本 %.0f / 安全 %.0f / 速度 %.0f\n\n",
		d.gen, segmentName(d.segment),
		d.quality[0], d.quality[1], d.quality[2], d.quality[3],
	))
	b.WriteString(fmt.Sprintf("  名稱  %s▌\n", d.name))
	b.WriteString(fmt.Sprintf("  定價  $%.0f   （推薦 $%.0f）\n\n", d.price, d.refPrice))
	b.WriteString(fmt.Sprintf("  預估  需求 ×%.2f · 封頂用戶 ~%s\n\n", demand, human(est)))
	b.WriteString(helpStyle.Render("[←→]調價 [Shift+←→]±5  輸入名稱  [Enter]發佈 [Esc]取消"))
	return boxStyle.Render(b.String())
}
```

Fix `te` comment — remove unused. Imports: `math`, `unicode/utf8`, `fmt`, `strings`, tea, model, sim.

- [ ] **Step 2: Wire tui Update**

```go
	// on Model struct:
	publish *publishDialog

// in Update, KeyMsg branch — before train dialog:
	if m.publish != nil {
		return m.updatePublishDialog(msg)
	}
	if m.dialog != nil {
		return m.updateDialog(msg)
	}
```

```go
func (m Model) updatePublishDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	d, confirm, cancel := m.publish.update(msg)
	if cancel {
		m.publish = nil
		return m, nil
	}
	if confirm {
		if ns, err := sim.Apply(m.state, d.command(), m.cfg); err == nil {
			m.state = ns
			m.notice = fmt.Sprintf("「%s」已上線", ns.Models[d.index].Name)
		}
		m.publish = nil
		return m, nil
	}
	m.publish = &d
	return m, nil
}
```

Keys:

```go
		case "p":
			if m.page == PageModels {
				if d, ok := newPublishDialog(m, m.modelCursor); ok {
					m.publish = &d
				} else {
					m.notice = "請選取待發佈草稿（先訓練模型）"
				}
			}
			return m, nil
		case "$":
			// minimal set-price for live models
			if m.page == PageModels && m.modelCursor >= 0 && m.modelCursor < len(m.state.Models) {
				md := m.state.Models[m.modelCursor]
				if md.Online {
					// cheapest path: open publish-like price-only OR nudge ± via left/right later
					// v1: cycle price using ref as base is too magic — open a tiny price dialog.
					// Reuse publishDialog fields with index and name locked:
					d := publishDialog{
						index: m.modelCursor, name: md.Name, price: md.Price,
						refPrice: m.cfg.SegmentRefPrice[md.Segment],
						gen: md.Gen, segment: md.Segment, quality: md.Quality,
						priceOnly: true,
					}
					m.publish = &d
				}
			}
			return m, nil
```

For `priceOnly`, on confirm call `SetPrice` instead of `PublishModel`, and disable name editing in `update`. Add `priceOnly bool` to struct.

```go
	if confirm {
		if d.priceOnly {
			m.state = applyOK(m.state, model.SetPrice{ModelIndex: d.index, Price: d.price}, m.cfg)
		} else if ns, err := sim.Apply(m.state, d.command(), m.cfg); err == nil {
			m.state = ns
			m.notice = fmt.Sprintf("「%s」已上線", d.name)
		}
		m.publish = nil
		return m, nil
	}
```

In `update`, if `priceOnly`, ignore name keystrokes.

- [ ] **Step 3: View rendering**

Where train dialog is rendered:

```go
	if m.publish != nil {
		page = renderPublishDialog(*m.publish, m)
	} else if m.dialog != nil {
		page = renderTrainDialog(*m.dialog, m)
	}
```

- [ ] **Step 4: Overview hint** (optional small):

If any draft, in overview box add line `待發佈模型 N 個 — 模型頁按 p`.

- [ ] **Step 5: Tests**

```go
func TestPublishDialogCommand(t *testing.T) {
	d := publishDialog{index: 0, name: "A", price: 10}
	cmd := d.command()
	if cmd.ModelIndex != 0 || cmd.Name != "A" || cmd.Price != 10 {
		t.Fatalf("%+v", cmd)
	}
}

func TestPublishDialogRejectsNonDraft(t *testing.T) {
	m := testModel(t)
	m.state.Models = []model.Model{{Online: true, Users: 1, Name: "x"}}
	if _, ok := newPublishDialog(m, 0); ok {
		t.Fatal("should reject live model")
	}
}
```

Run: `go test ./internal/tui/ -count=1`

- [ ] **Step 6: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): publish dialog with name, price, and estimate"
```

---

### Task 8: Integration polish + full suite

**Files:** any remaining test fallout; `settle` banner copy; docs only if needed (no code in DEPLOYMENT unless version note).

- [ ] **Step 1: Full test**

```bash
go test ./... -count=1 && go vet ./... && go build ./...
```

Expected: all green.

- [ ] **Step 2: Manual smoke checklist** (agent or human)

1. Fresh save (or wipe `save.json`).  
2. Train Gen1 → wait complete → models page shows 待發佈.  
3. `p` → name + price → Enter → notice 已上線.  
4. Watch users climb across ticks (not jump to full cap in one tick).  
5. Second train while draft exists works.  
6. `$` on live model changes price; estimate/users directionally follow elasticity.

- [ ] **Step 3: Final commit if any fixups**

```bash
git add -A && git status
# if changes:
git commit -m "fix: publish-growth integration fallout"
```

- [ ] **Step 4: Release (only if user asked)**

Per `DEPLOYMENT.md`: tag patch e.g. `v0.5.2`, push, watch GoReleaser. **Do not tag unless user requests release.**

---

## Spec Coverage Checklist

| Spec requirement | Task |
|---|---|
| Train → draft Offline | Task 2 |
| PublishModel name+price | Task 3 |
| Draft allows new training | Task 2 test |
| Exponential growth | Task 4 |
| EstimateUserTarget for dialog | Task 4 |
| Balance 3.5e-5 / scales 20k | Task 5 |
| TUI draft/live + cursor | Task 6 |
| Publish dialog + p + $ | Task 7 |
| Old save name empty | Task 1 zero-value + Task 6 display |
| No auto forced modal | Task 7 (p optional) |
| Tests anchors 1h / 8h | Task 4 |
| Out of scope valuation | — not in plan |

---

## Self-Review Notes

- No TBD placeholders in steps.  
- `P` vs `p` disambiguated.  
- Legacy sim tests pin old rate/scale so Task 5 does not thrash unit math.  
- `priceOnly` reuses publish dialog to avoid a third modal type (YAGNI).  
