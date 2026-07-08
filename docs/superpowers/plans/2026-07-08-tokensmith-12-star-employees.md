# Tokensmith 12 — 明星員工 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 加入具名明星員工：花現金簽約、每 tick 付高薪，各自提供數值加成（研發/秒、某維度品質、算力效率、用戶成長）。

**Architecture:** `model.Star`（目錄，含 `StarEffects` 數值加成）、`GameState.HiredStars []string`。`SignStar` 指令花現金簽約。`sim.starEffects()` 聚合已簽明星加成，接進 R&D 產出、訓練品質、有效算力、用戶目標；`Tick` 扣明星薪資。**非破壞性**：無明星 → 加成中性（mult 1、bonus 0）→ Plan 01–11 不變。花俏簽名絕活與對手反挖留後續 plan。相依 Plan 01–11。

**Tech Stack:** Go 1.22+、標準 `testing` + `math`。無外部依賴。

## Global Constraints

- 延續 Plan 01–11。`internal/sim` 純；不 mutate 輸入、clone slice（`HiredStars` append 前 clone）。
- `StarEffects` 中性：`QualityMult[NumQualityDims]=1`、`RnDPerSec=0`、`InfraMult=1`、`UserGrowthMult=1`。聚合：mult 相乘、bonus 相加。
- 明星加成與既有 tech/prestige/staff 倍率**相乘 / 相加疊加**（各自獨立）。
- v0 目錄（spec §17.5 數值版）：見 Task 2。

---

### Task 1: `model` 擴充——StarEffects / Star / SignStar

**Files:**
- Modify: `internal/model/types.go`
- Test: `internal/model/types_test.go`

**Interfaces:**
- Produces:
  - `model.StarEffects{ QualityMult [NumQualityDims]float64; RnDPerSec, InfraMult, UserGrowthMult float64 }` + `model.NeutralStarEffects()`
  - `model.Star{ ID, Name string; SigningCost, SalaryPerSec float64; Effects StarEffects }`
  - `GameState` 新欄位 `HiredStars []string`
  - `model.SignStar{ StarID string }`，實作 `commandMarker()`

- [ ] **Step 1: 寫失敗測試**

在 `internal/model/types_test.go` 末尾新增：
```go
func TestStarTypes(t *testing.T) {
	e := NeutralStarEffects()
	if e.QualityMult[DimCapability] != 1 || e.InfraMult != 1 || e.UserGrowthMult != 1 || e.RnDPerSec != 0 {
		t.Fatalf("neutral star effects wrong: %+v", e)
	}
	st := Star{ID: "x", Name: "X", SigningCost: 100, SalaryPerSec: 1, Effects: e}
	var s GameState
	s.HiredStars = append(s.HiredStars, st.ID)
	if len(s.HiredStars) != 1 {
		t.Fatalf("HiredStars not usable")
	}
	var c Command = SignStar{StarID: "x"}
	if _, ok := c.(SignStar); !ok {
		t.Fatalf("SignStar not a Command")
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/model/`
Expected: FAIL（`undefined: NeutralStarEffects` 等）。

- [ ] **Step 3: 寫最小實作**

在 `internal/model/types.go` 末尾新增：
```go
// StarEffects are a star employee's numeric bonuses; neutral = mults 1, bonus 0.
type StarEffects struct {
	QualityMult    [NumQualityDims]float64
	RnDPerSec      float64
	InfraMult      float64
	UserGrowthMult float64
}

// NeutralStarEffects returns effects that change nothing.
func NeutralStarEffects() StarEffects {
	e := StarEffects{RnDPerSec: 0, InfraMult: 1, UserGrowthMult: 1}
	for d := range e.QualityMult {
		e.QualityMult[d] = 1
	}
	return e
}

// Star is a named hireable employee.
type Star struct {
	ID           string
	Name         string
	SigningCost  float64
	SalaryPerSec float64
	Effects      StarEffects
}

// SignStar hires the star with the given ID.
type SignStar struct {
	StarID string
}

func (SignStar) commandMarker() {}
```

在 `GameState` 結構加入欄位（放在 `Prestige` 之後或末尾）：
```go
	HiredStars []string
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/model/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/model/types.go internal/model/types_test.go
git commit -m "feat(model): add star employee types and SignStar command"
```

---

### Task 2: `balance` — 明星名冊

**Files:**
- Modify: `internal/balance/balance.go`
- Test: `internal/balance/balance_test.go`

**Interfaces:**
- Produces: `Config` 新欄位 `Stars []model.Star`（`Default()` 填入）+ `balance.DefaultStars() []model.Star`。

- [ ] **Step 1: 寫失敗測試**

在 `internal/balance/balance_test.go` 末尾新增：
```go
func TestDefaultStars(t *testing.T) {
	c := Default()
	if len(c.Stars) < 6 {
		t.Fatalf("stars = %d, want >= 6", len(c.Stars))
	}
	byID := map[string]model.Star{}
	for _, s := range c.Stars {
		byID[s.ID] = s
	}
	if s, ok := byID["aria-chen"]; !ok || s.Effects.QualityMult[model.DimCapability] != 1.22 || s.Effects.RnDPerSec != 300 {
		t.Errorf("aria-chen wrong: %+v ok=%v", s, ok)
	}
	if s, ok := byID["marcus-cole"]; !ok || s.Effects.UserGrowthMult != 1.30 {
		t.Errorf("marcus-cole wrong: %+v", s)
	}
	// unrelated fields neutral
	if byID["aria-chen"].Effects.InfraMult != 1 {
		t.Errorf("aria-chen InfraMult should be neutral 1")
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/balance/`
Expected: FAIL（`undefined: DefaultStars`）。

- [ ] **Step 3: 寫最小實作**

在 `internal/balance/balance.go` 的 `Config` 結構末尾加入欄位：
```go
	Stars []model.Star // star-employee roster (plan-12)
```

在 `Default()` 的 `return c` 之前加入：
```go
	c.Stars = DefaultStars()
```

在檔案末尾新增：
```go
// star builds a Star starting from neutral effects, applying set().
func star(id, name string, signing, salaryPerSec float64, set func(e *model.StarEffects)) model.Star {
	e := model.NeutralStarEffects()
	set(&e)
	return model.Star{ID: id, Name: name, SigningCost: signing, SalaryPerSec: salaryPerSec, Effects: e}
}

// DefaultStars returns the v0 star roster (spec §17.5, numeric bonuses).
func DefaultStars() []model.Star {
	return []model.Star{
		star("aria-chen", "Dr. Aria Chen", 600000, 0.02, func(e *model.StarEffects) {
			e.QualityMult[model.DimCapability] = 1.22
			e.RnDPerSec = 300
		}),
		star("nova", "Nova", 1000000, 0.03, func(e *model.StarEffects) {
			for d := range e.QualityMult {
				e.QualityMult[d] = 1.10
			}
			e.RnDPerSec = 400
		}),
		star("sofia-reyes", "Dr. Sofia Reyes", 450000, 0.018, func(e *model.StarEffects) {
			e.QualityMult[model.DimSafety] = 1.25
		}),
		star("wei-zhang", "Dr. Wei Zhang", 380000, 0.016, func(e *model.StarEffects) {
			e.QualityMult[model.DimEfficiency] = 1.25
		}),
		star("kenji-tanaka", "Kenji Tanaka", 420000, 0.017, func(e *model.StarEffects) {
			e.InfraMult = 1.12
		}),
		star("elena-volkov", "Elena Volkov", 420000, 0.017, func(e *model.StarEffects) {
			e.InfraMult = 1.10
		}),
		star("marcus-cole", "Marcus Cole", 350000, 0.015, func(e *model.StarEffects) {
			e.UserGrowthMult = 1.30
		}),
		star("james-okafor", "James Okafor", 400000, 0.017, func(e *model.StarEffects) {
			e.UserGrowthMult = 1.25
		}),
	}
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/balance/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/balance/balance.go internal/balance/balance_test.go
git commit -m "feat(balance): add star employee roster"
```

---

### Task 3: `sim` — 明星效果聚合

**Files:**
- Create: `internal/sim/star.go`
- Test: `internal/sim/star_test.go`

**Interfaces:**
- Produces:
  - `sim.isStarHired(ns model.GameState, id string) bool`
  - `sim.starEffects(ns model.GameState, b balance.Config) model.StarEffects`（聚合已簽明星；無 → 中性）

- [ ] **Step 1: 寫失敗測試**

Create `internal/sim/star_test.go`:
```go
package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestStarEffectsAggregate(t *testing.T) {
	b := balance.Default()
	s := model.GameState{HiredStars: []string{"aria-chen", "marcus-cole"}}
	se := starEffects(s, b)
	if !approx(se.QualityMult[model.DimCapability], 1.22) {
		t.Errorf("cap mult = %v, want 1.22", se.QualityMult[model.DimCapability])
	}
	if !approx(se.RnDPerSec, 300) {
		t.Errorf("RnDPerSec = %v, want 300", se.RnDPerSec)
	}
	if !approx(se.UserGrowthMult, 1.30) {
		t.Errorf("UserGrowthMult = %v, want 1.30", se.UserGrowthMult)
	}
	if !approx(se.InfraMult, 1) {
		t.Errorf("InfraMult should be neutral 1, got %v", se.InfraMult)
	}
}

func TestStarEffectsNeutralWhenNoneHired(t *testing.T) {
	se := starEffects(model.GameState{}, balance.Default())
	if !approx(se.InfraMult, 1) || !approx(se.RnDPerSec, 0) {
		t.Fatalf("neutral star effects expected: %+v", se)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run TestStarEffects`
Expected: FAIL（`undefined: starEffects`）。

- [ ] **Step 3: 寫最小實作**

Create `internal/sim/star.go`:
```go
package sim

import (
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func isStarHired(ns model.GameState, id string) bool {
	for _, h := range ns.HiredStars {
		if h == id {
			return true
		}
	}
	return false
}

// starEffects aggregates the bonuses of all hired stars (neutral when none).
func starEffects(ns model.GameState, b balance.Config) model.StarEffects {
	agg := model.NeutralStarEffects()
	for _, st := range b.Stars {
		if !isStarHired(ns, st.ID) {
			continue
		}
		for d := range agg.QualityMult {
			agg.QualityMult[d] *= st.Effects.QualityMult[d]
		}
		agg.RnDPerSec += st.Effects.RnDPerSec
		agg.InfraMult *= st.Effects.InfraMult
		agg.UserGrowthMult *= st.Effects.UserGrowthMult
	}
	return agg
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/ -run TestStarEffects`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/sim/star.go internal/sim/star_test.go
git commit -m "feat(sim): aggregate hired star effects"
```

---

### Task 4: `sim.Apply` — SignStar

**Files:**
- Modify: `internal/sim/apply.go`
- Test: `internal/sim/apply_test.go`

**Interfaces:**
- Produces:
  - package vars `ErrInvalidStar`, `ErrAlreadyHired`
  - `sim.findStar(stars []model.Star, id string) (model.Star, bool)`
  - `Apply` 新增 `SignStar`：找明星；未簽；現金足夠付簽約金 → 扣現金、append（clone）。

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/apply_test.go` 末尾新增：
```go
func TestApplySignStar(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Resources.Cash = 1_000_000
	ns, err := Apply(s, model.SignStar{StarID: "aria-chen"}, b) // signing 600000
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !approx(ns.Resources.Cash, 400000) {
		t.Errorf("cash = %v, want 400000", ns.Resources.Cash)
	}
	if len(ns.HiredStars) != 1 || ns.HiredStars[0] != "aria-chen" {
		t.Errorf("not hired: %+v", ns.HiredStars)
	}
	if len(s.HiredStars) != 0 {
		t.Errorf("Apply mutated input")
	}
}

func TestApplySignStarErrors(t *testing.T) {
	b := balance.Default()
	rich := model.GameState{}
	rich.Resources.Cash = 1e9
	if _, err := Apply(rich, model.SignStar{StarID: "nope"}, b); err != ErrInvalidStar {
		t.Errorf("invalid: err = %v, want ErrInvalidStar", err)
	}
	already := model.GameState{HiredStars: []string{"aria-chen"}}
	already.Resources.Cash = 1e9
	if _, err := Apply(already, model.SignStar{StarID: "aria-chen"}, b); err != ErrAlreadyHired {
		t.Errorf("already: err = %v, want ErrAlreadyHired", err)
	}
	poor := model.GameState{}
	poor.Resources.Cash = 100
	if _, err := Apply(poor, model.SignStar{StarID: "aria-chen"}, b); err != ErrInsufficientCash {
		t.Errorf("cash: err = %v, want ErrInsufficientCash", err)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run TestApplySignStar`
Expected: FAIL（`undefined: ErrInvalidStar`）。

- [ ] **Step 3: 寫最小實作**

在 `internal/sim/apply.go` 的 error var 區塊加入：
```go
	ErrInvalidStar  = errors.New("sim: unknown star")
	ErrAlreadyHired = errors.New("sim: star already hired")
```

在 `Apply` 的 switch 內、`PrestigeReset` case 之後新增：
```go
	case model.SignStar:
		return applySignStar(s, c, b)
```

新增函式：
```go
func findStar(stars []model.Star, id string) (model.Star, bool) {
	for _, st := range stars {
		if st.ID == id {
			return st, true
		}
	}
	return model.Star{}, false
}

func applySignStar(s model.GameState, c model.SignStar, b balance.Config) (model.GameState, error) {
	st, ok := findStar(b.Stars, c.StarID)
	if !ok {
		return s, ErrInvalidStar
	}
	if isStarHired(s, st.ID) {
		return s, ErrAlreadyHired
	}
	if s.Resources.Cash < st.SigningCost {
		return s, ErrInsufficientCash
	}
	ns := s
	ns.Resources.Cash -= st.SigningCost
	ns.HiredStars = append(append([]string(nil), s.HiredStars...), st.ID)
	return ns, nil
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/ -run TestApply`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/sim/apply.go internal/sim/apply_test.go
git commit -m "feat(sim): add SignStar command"
```

---

### Task 5: `Tick` — 明星薪資 + 研發/秒

**Files:**
- Modify: `internal/sim/sim.go`
- Test: `internal/sim/sim_test.go`

**Interfaces:**
- Produces:
  - `sim.starSalaryPerSec(ns model.GameState, b balance.Config) float64`
  - `Tick`：薪資扣除加上明星薪資；R&D 產出加上 `starEffects().RnDPerSec * dt`（併入既有 `* pe.RnDMult`）。

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/sim_test.go` 末尾新增：
```go
func TestTickStarSalary(t *testing.T) {
	b := balance.Default()
	s := model.GameState{HiredStars: []string{"aria-chen"}} // salary 0.02/s
	s.Resources.Cash = 100
	ns := Tick(s, 10, nil, b)
	// aria salary 0.02*10 = 0.2 (aria also adds R&D but not cash)
	if !approx(ns.Resources.Cash, 100-0.02*10) {
		t.Fatalf("Cash = %v, want %v", ns.Resources.Cash, 100-0.02*10)
	}
}

func TestTickStarRnDBonus(t *testing.T) {
	b := balance.Default()
	base := model.GameState{}
	withStar := model.GameState{HiredStars: []string{"aria-chen"}} // +300 R&D/s
	nb := Tick(base, 1, nil, b)
	nw := Tick(withStar, 1, nil, b)
	if nw.Resources.RnD <= nb.Resources.RnD {
		t.Fatalf("star should add R&D: %v vs %v", nw.Resources.RnD, nb.Resources.RnD)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run 'TestTickStarSalary|TestTickStarRnDBonus'`
Expected: FAIL（薪資 / R&D 未加）。

- [ ] **Step 3: 寫最小實作**

在 `internal/sim/sim.go` 新增 helper（放在 `totalSalaryPerSec` 附近）：
```go
// starSalaryPerSec is the aggregate salary of all hired stars.
func starSalaryPerSec(ns model.GameState, b balance.Config) float64 {
	var s float64
	for _, st := range b.Stars {
		if isStarHired(ns, st.ID) {
			s += st.SalaryPerSec
		}
	}
	return s
}
```

在 `Tick` 內，把 R&D 產出行改為併入明星 R&D/秒：
```go
	pe := prestigeEffects(ns.Prestige.UnlockedPrestige, b)
	starRnD := starEffects(ns, b).RnDPerSec * dt
	ns.Resources.RnD += (staffRnD + tokenRnD + starRnD) * pe.RnDMult
```

在 `Tick` 內，把聚合薪資那行加上明星薪資：
```go
	ns.Resources.Cash -= (totalSalaryPerSec(ns, b) + starSalaryPerSec(ns, b)) * dt
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/`
Expected: PASS（全部）。

- [ ] **Step 5: Commit**

```bash
git add internal/sim/sim.go internal/sim/sim_test.go
git commit -m "feat(sim): star salary and R&D bonus in Tick"
```

---

### Task 6: 明星加成——品質 / 算力 / 用戶

**Files:**
- Modify: `internal/sim/sim.go`
- Test: `internal/sim/sim_test.go`

**Interfaces:**
- Produces:
  - `advanceTraining` 完成品質乘 `starEffects().QualityMult[d]`
  - `effectiveTraining`/`effectiveInference` 乘 `starEffects().InfraMult`
  - `advanceUsers` 目標乘 `starEffects().UserGrowthMult`

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/sim_test.go` 末尾新增：
```go
func TestStarQualityMult(t *testing.T) {
	b := balance.Default()
	s := model.GameState{HasTraining: true, HiredStars: []string{"aria-chen"}} // cap ×1.22
	s.Compute.TrainingCapacity = 1000
	s.Training = model.TrainingJob{Gen: 2, Alloc: [model.NumQualityDims]float64{0.4, 0.2, 0.2, 0.2}, WorkRemaining: 1}
	ns := Tick(s, 1, nil, b)
	if !approx(ns.Models[0].Quality[model.DimCapability], 0.4*45*1.22) { // 21.96
		t.Fatalf("capability = %v, want %v", ns.Models[0].Quality[model.DimCapability], 0.4*45*1.22)
	}
}

func TestStarInfraSpeedsTraining(t *testing.T) {
	b := balance.Default()
	base := model.GameState{HasTraining: true}
	base.Compute.TrainingCapacity = 10
	base.Training = model.TrainingJob{Gen: 1, WorkRemaining: 1e9}
	withStar := base
	withStar.HiredStars = []string{"kenji-tanaka"} // InfraMult 1.12
	nb := Tick(base, 1, nil, b)
	nw := Tick(withStar, 1, nil, b)
	if nw.Training.WorkRemaining >= nb.Training.WorkRemaining {
		t.Fatalf("star infra should speed training: %v vs %v", nw.Training.WorkRemaining, nb.Training.WorkRemaining)
	}
}

func TestStarGrowthBoostsUsers(t *testing.T) {
	b := balance.Default()
	base := model.GameState{Models: []model.Model{onlineModel(50, b.RefPrice)}}
	withStar := model.GameState{Models: []model.Model{onlineModel(50, b.RefPrice)}, HiredStars: []string{"marcus-cole"}} // 1.30
	nb := Tick(base, 1, nil, b)
	nw := Tick(withStar, 1, nil, b)
	if nw.Models[0].Users <= nb.Models[0].Users {
		t.Fatalf("star growth should boost users: %v vs %v", nw.Models[0].Users, nb.Models[0].Users)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run TestStar`
Expected: FAIL（明星加成未接進）。

- [ ] **Step 3: 寫最小實作**

**(a)** `advanceTraining` 完成時品質再乘明星：既有行為 `m.Quality[d] = job.Alloc[d] * b.GenQualityCap[job.Gen] * te.QualityMult[d]`，在其後乘上 `se := starEffects(ns, b)` 的 `QualityMult[d]`：
```go
	te := techEffects(ns, b)
	se := starEffects(ns, b)
	job := ns.Training
	m := model.Model{Gen: job.Gen, Price: job.Price, Online: true}
	for d := range model.NumQualityDims {
		m.Quality[d] = job.Alloc[d] * b.GenQualityCap[job.Gen] * te.QualityMult[d] * se.QualityMult[d]
	}
```

**(b)** `effectiveTraining`/`effectiveInference` 末尾再乘 `starEffects(ns, b).InfraMult`：
```go
	return c * infraEfficiency(ns, b) * techEffects(ns, b).InfraMult * starEffects(ns, b).InfraMult
```
（兩個函式都改。）

**(c)** `advanceUsers` 目標乘明星成長：既有 `target := appeal * b.SegmentTargetScale[m.Segment] * demandMult * share * marketingMult * te.UserGrowthMult`，末尾再乘 `starEffects(ns, b).UserGrowthMult`（可在迴圈前取 `se := starEffects(ns, b)`）：
```go
		target := appeal * b.SegmentTargetScale[m.Segment] * demandMult * share * marketingMult * te.UserGrowthMult * se.UserGrowthMult
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/`
Expected: PASS（全部——無明星 → 加成 1 → Plan 01–11 不變）。

- [ ] **Step 5: 跑整包 + vet + build + commit**

Run:
```bash
go test ./...
go vet ./...
go build ./...
```
Expected: 全部 PASS、vet 無警告、可建置。

```bash
git add internal/sim/sim.go internal/sim/sim_test.go
git commit -m "feat(sim): wire star quality, infra, growth bonuses"
```

---

## 完成後狀態

明星員工成立：花現金簽約 8 位具名明星、付高薪、各自提供數值加成（研發/秒、某維度品質、算力效率、用戶成長）。無明星 → 全中性 → Plan 01–11 不變。仍純確定性。

**待補**：花俏簽名絕活（並行研發、免疫事故、兩池共享…）、對手反挖與留任度（需事件 / rng）。

**下一步**：接真實 token 採集（ingest 招牌機制），然後 store / daemon+IPC / 完整 TUI。
