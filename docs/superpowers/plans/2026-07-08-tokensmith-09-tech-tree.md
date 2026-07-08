# Tokensmith 09 — 科技樹四分支 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 加入資料驅動的四分支科技樹：用 R&D 解鎖節點（含前置），已解鎖節點的效果聚合成倍率，乘進四維品質產出、訓練成本 / 工作量、算力效率、用戶成長、參考價。

**Architecture:** `model.TechNode`（id/分支/成本/前置/`TechEffects` 倍率）為資料，`balance.DefaultTechNodes()` 提供目錄；`GameState.UnlockedTech []string` 記已解鎖。`UnlockTech` 指令驗證前置+扣 R&D。`sim.techEffects(ns,b)` 聚合已解鎖節點的倍率（無解鎖 → 中性 1.0），乘進 sim 各處。**非破壞性**：無解鎖 → 全倍率 1 → Plan 01–08 不變。里程碑 gate 待里程碑 plan。明星員工另計。相依 Plan 01–08。

**Tech Stack:** Go 1.22+、標準 `testing` + `math`。無外部依賴。

## Global Constraints

- 延續 Plan 01–08。`internal/sim` 純；不 mutate 輸入、clone slice（`UnlockedTech` append 前 clone）。
- `TechEffects` 全為倍率，中性 = 1.0（含 `QualityMult[NumQualityDims]`）。聚合 = 對已解鎖節點逐欄相乘。
- 節點目錄用固定順序 slice（`balance.Config.TechNodes`），聚合迭代目錄檢查是否解鎖 → 確定性。
- v0 目錄（representative，取自 §17.3；里程碑 gate 暫略）：algo-cap-1 / algo-train-1 / infra-eff-1 / infra-density-1 / biz-growth-1 / biz-price-1 / align-safety-1 / align-incident-1。

---

### Task 1: `model` 擴充——TechBranch / TechEffects / TechNode / UnlockTech

**Files:**
- Modify: `internal/model/types.go`
- Test: `internal/model/types_test.go`

**Interfaces:**
- Produces:
  - `model.TechBranch`（int）+ `BranchAlgo=0, BranchInfra=1, BranchBusiness=2, BranchAlignment=3`, `NumBranches=4`
  - `model.TechEffects{ QualityMult [NumQualityDims]float64; TrainRnDMult, TrainWorkMult, InfraMult, UserGrowthMult, RefPriceMult, IncidentMult float64 }`
  - `model.NeutralTechEffects() TechEffects`（全 1.0）
  - `model.TechNode{ ID string; Branch TechBranch; Cost float64; Prereqs []string; Effects TechEffects }`
  - `GameState` 新欄位 `UnlockedTech []string`
  - `model.UnlockTech{ NodeID string }`，實作 `commandMarker()`

- [ ] **Step 1: 寫失敗測試**

在 `internal/model/types_test.go` 末尾新增：
```go
func TestTechTypes(t *testing.T) {
	if NumBranches != 4 || BranchAlignment != 3 {
		t.Fatalf("branch consts wrong")
	}
	e := NeutralTechEffects()
	if e.TrainRnDMult != 1 || e.InfraMult != 1 || e.QualityMult[DimCapability] != 1 {
		t.Fatalf("neutral effects not all 1: %+v", e)
	}
	n := TechNode{ID: "x", Branch: BranchAlgo, Cost: 100, Effects: e}
	var s GameState
	s.UnlockedTech = append(s.UnlockedTech, n.ID)
	if len(s.UnlockedTech) != 1 {
		t.Fatalf("UnlockedTech not usable")
	}
	var c Command = UnlockTech{NodeID: "x"}
	if _, ok := c.(UnlockTech); !ok {
		t.Fatalf("UnlockTech not a Command")
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/model/`
Expected: FAIL（`undefined: NumBranches` / `NeutralTechEffects` 等）。

- [ ] **Step 3: 寫最小實作**

在 `internal/model/types.go` 末尾新增：
```go
// TechBranch identifies a tech-tree branch.
type TechBranch int

const (
	BranchAlgo      TechBranch = iota // 0 演算法
	BranchInfra                       // 1 硬體基建
	BranchBusiness                    // 2 商業營運
	BranchAlignment                   // 3 對齊安全
	NumBranches = 4
)

// TechEffects are multiplicative modifiers; neutral value is 1.0.
type TechEffects struct {
	QualityMult    [NumQualityDims]float64 // per-dim quality output
	TrainRnDMult   float64                 // training R&D cost
	TrainWorkMult  float64                 // training work (GPU-seconds)
	InfraMult      float64                 // compute efficiency
	UserGrowthMult float64                 // user acquisition
	RefPriceMult   float64                 // reference price / willingness to pay
	IncidentMult   float64                 // anti-incident (used by later event plan)
}

// NeutralTechEffects returns effects that change nothing (all 1.0).
func NeutralTechEffects() TechEffects {
	e := TechEffects{
		TrainRnDMult: 1, TrainWorkMult: 1, InfraMult: 1,
		UserGrowthMult: 1, RefPriceMult: 1, IncidentMult: 1,
	}
	for d := range e.QualityMult {
		e.QualityMult[d] = 1
	}
	return e
}

// TechNode is a tech-tree entry unlocked with R&D.
type TechNode struct {
	ID      string
	Branch  TechBranch
	Cost    float64
	Prereqs []string
	Effects TechEffects
}

// UnlockTech unlocks the tech node with the given ID.
type UnlockTech struct {
	NodeID string
}

func (UnlockTech) commandMarker() {}
```

在 `GameState` 結構加入欄位（放在 `Prestige` 之前或末尾皆可，例如 `Marketing` 之後）：
```go
	UnlockedTech []string
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/model/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/model/types.go internal/model/types_test.go
git commit -m "feat(model): add tech tree types and UnlockTech command"
```

---

### Task 2: `balance` — 科技樹目錄

**Files:**
- Modify: `internal/balance/balance.go`
- Test: `internal/balance/balance_test.go`

**Interfaces:**
- Produces:
  - `Config` 新欄位 `TechNodes []model.TechNode`（`Default()` 填入）
  - `balance.DefaultTechNodes() []model.TechNode`

- [ ] **Step 1: 寫失敗測試**

在 `internal/balance/balance_test.go` 末尾新增：
```go
func TestDefaultTechNodes(t *testing.T) {
	c := Default()
	if len(c.TechNodes) < 8 {
		t.Fatalf("tech nodes = %d, want >= 8", len(c.TechNodes))
	}
	byID := map[string]model.TechNode{}
	for _, n := range c.TechNodes {
		byID[n.ID] = n
	}
	if n, ok := byID["algo-cap-1"]; !ok || n.Effects.QualityMult[model.DimCapability] != 1.15 {
		t.Errorf("algo-cap-1 wrong: %+v ok=%v", n, ok)
	}
	if n, ok := byID["infra-density-1"]; !ok || len(n.Prereqs) != 1 || n.Prereqs[0] != "infra-eff-1" {
		t.Errorf("infra-density-1 prereq wrong: %+v", n)
	}
	// unrelated fields stay neutral
	if byID["algo-cap-1"].Effects.InfraMult != 1 {
		t.Errorf("algo-cap-1 InfraMult should be neutral 1")
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/balance/`
Expected: FAIL（`undefined: DefaultTechNodes` / 欄位不存在）。

- [ ] **Step 3: 寫最小實作**

在 `internal/balance/balance.go` 的 `Config` 結構末尾加入欄位：
```go
	TechNodes []model.TechNode // tech-tree catalog (plan-09)
```

在 `Default()` 的 `return c` 之前加入：
```go
	c.TechNodes = DefaultTechNodes()
```

在檔案末尾新增：
```go
// techNode builds a node starting from neutral effects, applying set().
func techNode(id string, br model.TechBranch, cost float64, prereqs []string, set func(e *model.TechEffects)) model.TechNode {
	e := model.NeutralTechEffects()
	set(&e)
	return model.TechNode{ID: id, Branch: br, Cost: cost, Prereqs: prereqs, Effects: e}
}

// DefaultTechNodes returns the v0 tech-tree catalog (representative; spec §17.3).
func DefaultTechNodes() []model.TechNode {
	return []model.TechNode{
		techNode("algo-cap-1", model.BranchAlgo, 15000, nil, func(e *model.TechEffects) {
			e.QualityMult[model.DimCapability] = 1.15
		}),
		techNode("algo-train-1", model.BranchAlgo, 80000, nil, func(e *model.TechEffects) {
			e.TrainRnDMult = 0.85
			e.TrainWorkMult = 0.9
		}),
		techNode("infra-eff-1", model.BranchInfra, 8000, nil, func(e *model.TechEffects) {
			e.InfraMult = 1.1
		}),
		techNode("infra-density-1", model.BranchInfra, 120000, []string{"infra-eff-1"}, func(e *model.TechEffects) {
			e.InfraMult = 1.15
		}),
		techNode("biz-growth-1", model.BranchBusiness, 6000, nil, func(e *model.TechEffects) {
			e.UserGrowthMult = 1.15
		}),
		techNode("biz-price-1", model.BranchBusiness, 15000, nil, func(e *model.TechEffects) {
			e.RefPriceMult = 1.1
		}),
		techNode("align-safety-1", model.BranchAlignment, 8000, nil, func(e *model.TechEffects) {
			e.QualityMult[model.DimSafety] = 1.15
		}),
		techNode("align-incident-1", model.BranchAlignment, 300000, []string{"align-safety-1"}, func(e *model.TechEffects) {
			e.IncidentMult = 0.5
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
git commit -m "feat(balance): add tech tree catalog"
```

---

### Task 3: `sim` — 科技效果聚合

**Files:**
- Create: `internal/sim/tech.go`
- Test: `internal/sim/tech_test.go`

**Interfaces:**
- Consumes: `GameState.UnlockedTech`；`balance.TechNodes`。
- Produces:
  - `sim.isUnlocked(ns model.GameState, id string) bool`
  - `sim.techEffects(ns model.GameState, b balance.Config) model.TechEffects`（聚合已解鎖節點的倍率；無解鎖 → 中性）

- [ ] **Step 1: 寫失敗測試**

Create `internal/sim/tech_test.go`:
```go
package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestTechEffectsAggregates(t *testing.T) {
	b := balance.Default()
	s := model.GameState{UnlockedTech: []string{"algo-cap-1", "infra-eff-1"}}
	te := techEffects(s, b)
	if !approx(te.QualityMult[model.DimCapability], 1.15) {
		t.Errorf("cap mult = %v, want 1.15", te.QualityMult[model.DimCapability])
	}
	if !approx(te.InfraMult, 1.1) {
		t.Errorf("infra mult = %v, want 1.1", te.InfraMult)
	}
	if !approx(te.TrainRnDMult, 1) {
		t.Errorf("unrelated mult should be 1, got %v", te.TrainRnDMult)
	}
}

func TestTechEffectsNeutralWhenNoneUnlocked(t *testing.T) {
	te := techEffects(model.GameState{}, balance.Default())
	if !approx(te.QualityMult[model.DimSpeed], 1) || !approx(te.UserGrowthMult, 1) {
		t.Fatalf("neutral tech effects expected: %+v", te)
	}
}

func TestIsUnlocked(t *testing.T) {
	s := model.GameState{UnlockedTech: []string{"a", "b"}}
	if !isUnlocked(s, "b") || isUnlocked(s, "c") {
		t.Fatalf("isUnlocked wrong")
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run 'TestTechEffects|TestIsUnlocked'`
Expected: FAIL（`undefined: techEffects` / `isUnlocked`）。

- [ ] **Step 3: 寫最小實作**

Create `internal/sim/tech.go`:
```go
package sim

import (
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// isUnlocked reports whether a tech node ID has been unlocked.
func isUnlocked(ns model.GameState, id string) bool {
	for _, u := range ns.UnlockedTech {
		if u == id {
			return true
		}
	}
	return false
}

// techEffects aggregates the multipliers of all unlocked tech nodes.
// Iterates the catalog (deterministic order); neutral when nothing unlocked.
func techEffects(ns model.GameState, b balance.Config) model.TechEffects {
	agg := model.NeutralTechEffects()
	for _, node := range b.TechNodes {
		if !isUnlocked(ns, node.ID) {
			continue
		}
		for d := range agg.QualityMult {
			agg.QualityMult[d] *= node.Effects.QualityMult[d]
		}
		agg.TrainRnDMult *= node.Effects.TrainRnDMult
		agg.TrainWorkMult *= node.Effects.TrainWorkMult
		agg.InfraMult *= node.Effects.InfraMult
		agg.UserGrowthMult *= node.Effects.UserGrowthMult
		agg.RefPriceMult *= node.Effects.RefPriceMult
		agg.IncidentMult *= node.Effects.IncidentMult
	}
	return agg
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/ -run 'TestTechEffects|TestIsUnlocked'`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/sim/tech.go internal/sim/tech_test.go
git commit -m "feat(sim): aggregate unlocked tech effects"
```

---

### Task 4: `sim.Apply` — UnlockTech

**Files:**
- Modify: `internal/sim/apply.go`
- Test: `internal/sim/apply_test.go`

**Interfaces:**
- Consumes: `model.UnlockTech`；`balance.TechNodes`。
- Produces:
  - package vars `ErrInvalidTech`, `ErrPrereqNotMet`, `ErrAlreadyUnlocked`
  - `sim.findTechNode(nodes []model.TechNode, id string) (model.TechNode, bool)`
  - `Apply` 新增處理 `UnlockTech`：找節點；未解鎖；前置全解鎖；R&D 足夠 → 扣 R&D、append（clone）。

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/apply_test.go` 末尾新增：
```go
func TestApplyUnlockTech(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Resources.RnD = 100000
	ns, err := Apply(s, model.UnlockTech{NodeID: "algo-cap-1"}, b) // cost 15000
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !approx(ns.Resources.RnD, 85000) {
		t.Errorf("RnD = %v, want 85000", ns.Resources.RnD)
	}
	if len(ns.UnlockedTech) != 1 || ns.UnlockedTech[0] != "algo-cap-1" {
		t.Errorf("not unlocked: %+v", ns.UnlockedTech)
	}
	if len(s.UnlockedTech) != 0 {
		t.Errorf("Apply mutated input")
	}
}

func TestApplyUnlockTechErrors(t *testing.T) {
	b := balance.Default()
	rich := model.GameState{}
	rich.Resources.RnD = 1e9
	if _, err := Apply(rich, model.UnlockTech{NodeID: "nope"}, b); err != ErrInvalidTech {
		t.Errorf("invalid: err = %v, want ErrInvalidTech", err)
	}
	if _, err := Apply(rich, model.UnlockTech{NodeID: "infra-density-1"}, b); err != ErrPrereqNotMet {
		t.Errorf("prereq: err = %v, want ErrPrereqNotMet", err)
	}
	already := model.GameState{UnlockedTech: []string{"algo-cap-1"}}
	already.Resources.RnD = 1e9
	if _, err := Apply(already, model.UnlockTech{NodeID: "algo-cap-1"}, b); err != ErrAlreadyUnlocked {
		t.Errorf("already: err = %v, want ErrAlreadyUnlocked", err)
	}
	poor := model.GameState{}
	poor.Resources.RnD = 100
	if _, err := Apply(poor, model.UnlockTech{NodeID: "algo-cap-1"}, b); err != ErrInsufficientRnD {
		t.Errorf("rnd: err = %v, want ErrInsufficientRnD", err)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run TestApplyUnlockTech`
Expected: FAIL（`undefined: ErrInvalidTech` 等）。

- [ ] **Step 3: 寫最小實作**

在 `internal/sim/apply.go` 的 error var 區塊加入：
```go
	ErrInvalidTech     = errors.New("sim: unknown tech node")
	ErrPrereqNotMet    = errors.New("sim: tech prerequisites not met")
	ErrAlreadyUnlocked = errors.New("sim: tech already unlocked")
```

在 `Apply` 的 switch 內、`FireStaff` case 之後新增：
```go
	case model.UnlockTech:
		return applyUnlockTech(s, c, b)
```

新增函式：
```go
func findTechNode(nodes []model.TechNode, id string) (model.TechNode, bool) {
	for _, n := range nodes {
		if n.ID == id {
			return n, true
		}
	}
	return model.TechNode{}, false
}

func applyUnlockTech(s model.GameState, c model.UnlockTech, b balance.Config) (model.GameState, error) {
	node, ok := findTechNode(b.TechNodes, c.NodeID)
	if !ok {
		return s, ErrInvalidTech
	}
	if isUnlocked(s, node.ID) {
		return s, ErrAlreadyUnlocked
	}
	for _, p := range node.Prereqs {
		if !isUnlocked(s, p) {
			return s, ErrPrereqNotMet
		}
	}
	if s.Resources.RnD < node.Cost {
		return s, ErrInsufficientRnD
	}
	ns := s
	ns.Resources.RnD -= node.Cost
	ns.UnlockedTech = append(append([]string(nil), s.UnlockedTech...), node.ID)
	return ns, nil
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/ -run TestApply`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/sim/apply.go internal/sim/apply_test.go
git commit -m "feat(sim): add UnlockTech command"
```

---

### Task 5: 科技效果接進 sim（品質 / 訓練 / 算力 / 用戶 / 定價）

**Files:**
- Modify: `internal/sim/sim.go`, `internal/sim/apply.go`
- Test: `internal/sim/sim_test.go`

**Interfaces:**
- Consumes: `techEffects`（Task 3）。
- Produces：
  - `advanceTraining` 完成時品質乘 `QualityMult[d]`
  - `applyStartTraining` 成本乘 `TrainRnDMult`、工作量乘 `TrainWorkMult`
  - `effectiveTraining`/`effectiveInference` 乘 `InfraMult`
  - `advanceUsers` 目標乘 `UserGrowthMult`、參考價乘 `RefPriceMult`

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/sim_test.go` 末尾新增：
```go
func TestTechQualityMultOnTrainedModel(t *testing.T) {
	b := balance.Default()
	s := model.GameState{HasTraining: true, UnlockedTech: []string{"algo-cap-1"}} // cap ×1.15
	s.Compute.TrainingCapacity = 1000
	s.Training = model.TrainingJob{Gen: 2, Alloc: [model.NumQualityDims]float64{0.4, 0.2, 0.2, 0.2}, WorkRemaining: 1}
	ns := Tick(s, 1, nil, b)
	if !approx(ns.Models[0].Quality[model.DimCapability], 0.4*45*1.15) { // 20.7
		t.Fatalf("capability = %v, want %v", ns.Models[0].Quality[model.DimCapability], 0.4*45*1.15)
	}
}

func TestTechTrainCostAndWorkReduced(t *testing.T) {
	b := balance.Default()
	s := model.GameState{UnlockedTech: []string{"algo-train-1"}} // RnD ×0.85, work ×0.9
	s.Resources.RnD = 100000
	ns, err := Apply(s, model.StartTraining{Gen: 1, Alloc: [model.NumQualityDims]float64{0.4, 0.2, 0.2, 0.2}, Price: 12}, b)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !approx(ns.Resources.RnD, 100000-20000*0.85) { // 83000
		t.Errorf("RnD = %v, want 83000", ns.Resources.RnD)
	}
	if !approx(ns.Training.WorkRemaining, 1800*0.9) { // 1620
		t.Errorf("WorkRemaining = %v, want 1620", ns.Training.WorkRemaining)
	}
}

func TestTechInfraSpeedsTraining(t *testing.T) {
	b := balance.Default()
	base := model.GameState{HasTraining: true}
	base.Compute.TrainingCapacity = 10
	base.Training = model.TrainingJob{Gen: 1, WorkRemaining: 1e9}
	withTech := base
	withTech.UnlockedTech = []string{"infra-eff-1"} // InfraMult 1.1
	nb := Tick(base, 1, nil, b)
	nt := Tick(withTech, 1, nil, b)
	if nt.Training.WorkRemaining >= nb.Training.WorkRemaining {
		t.Fatalf("infra tech should speed training: %v vs %v", nt.Training.WorkRemaining, nb.Training.WorkRemaining)
	}
}

func TestTechGrowthBoostsUsers(t *testing.T) {
	b := balance.Default()
	base := model.GameState{Models: []model.Model{onlineModel(50, b.RefPrice)}}
	withTech := model.GameState{Models: []model.Model{onlineModel(50, b.RefPrice)}, UnlockedTech: []string{"biz-growth-1"}}
	nb := Tick(base, 1, nil, b)
	nt := Tick(withTech, 1, nil, b)
	if nt.Models[0].Users <= nb.Models[0].Users {
		t.Fatalf("growth tech should boost users: %v vs %v", nt.Models[0].Users, nb.Models[0].Users)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run 'TestTech'`
Expected: FAIL（科技加成未接進 sim）。

- [ ] **Step 3: 寫最小實作**

**(a)** `internal/sim/apply.go` 的 `applyStartTraining` 內，把 `cost := b.GenRnDCost[c.Gen]` 與後面設定 `WorkRemaining` 改為套科技倍率：
```go
	te := techEffects(s, b)
	cost := b.GenRnDCost[c.Gen] * te.TrainRnDMult
	if s.Resources.RnD < cost {
		return s, ErrInsufficientRnD
	}
	ns := s
	ns.Resources.RnD -= cost
	ns.HasTraining = true
	ns.Training = model.TrainingJob{
		Gen:           c.Gen,
		Alloc:         c.Alloc,
		Price:         c.Price,
		WorkRemaining: b.GenTrainWorkGPUSec[c.Gen] * te.TrainWorkMult,
	}
	return ns, nil
```

**(b)** `internal/sim/sim.go` 的 `advanceTraining` 完成時品質乘 `QualityMult`：
```go
	te := techEffects(ns, b)
	job := ns.Training
	m := model.Model{Gen: job.Gen, Price: job.Price, Online: true}
	for d := range model.NumQualityDims {
		m.Quality[d] = job.Alloc[d] * b.GenQualityCap[job.Gen] * te.QualityMult[d]
	}
```

**(c)** `internal/sim/sim.go` 的 `effectiveTraining`/`effectiveInference` 末尾乘 `InfraMult`：
```go
	return c * infraEfficiency(ns, b) * techEffects(ns, b).InfraMult
```
（兩個函式都改。）

**(d)** `internal/sim/sim.go` 的 `advanceUsers` 內，取科技倍率並套到參考價與目標：
```go
		te := techEffects(ns, b)
		refPrice := b.SegmentRefPrice[m.Segment] * te.RefPriceMult
		var demandMult float64
		if m.Price > 0 {
			demandMult = math.Pow(refPrice/m.Price, b.PriceElasticity)
		}
		share := 1.0
		if appeal+rivalAppeal > 0 {
			share = appeal / (appeal + rivalAppeal)
		}
		marketingMult := 1 + float64(ns.Marketing)*b.MarketingBonus
		target := appeal * b.SegmentTargetScale[m.Segment] * demandMult * share * marketingMult * te.UserGrowthMult
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/`
Expected: PASS（全部——無解鎖 → 倍率 1 → Plan 01–08 不變）。

- [ ] **Step 5: 跑整包 + vet + build + commit**

Run:
```bash
go test ./...
go vet ./...
go build ./...
```
Expected: 全部 PASS、vet 無警告、可建置。

```bash
git add internal/sim/sim.go internal/sim/apply.go internal/sim/sim_test.go
git commit -m "feat(sim): wire tech effects into training, compute, users"
```

---

## 完成後狀態

科技樹四分支成立：用 R&D 沿分支（含前置）解鎖節點，效果聚合成倍率乘進四維品質產出（§6.4.3 疊乘完整）、訓練成本 / 工作量、算力效率、用戶成長、參考價。偏科投資能在某定位取得優勢。無解鎖 → 全中性 → Plan 01–08 不變。仍純確定性。`IncidentMult` 已備好給未來事件 plan。

**待補**：里程碑 gate（需估值，待里程碑 plan）、更多節點（目錄可擴充）。

**下一步**：明星員工（挖角/留任）、里程碑 + prestige（含估值），然後 store / ingest（招牌 token 採集）/ daemon+IPC / 完整六頁 TUI。
