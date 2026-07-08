# Tokensmith 11 — Prestige Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 加入 prestige：達估值門檻後可「重開」——依峰值估值換專利（`floor(√(peak/K))`）、清空 run-state 重新開始、保留專利與永久升級；專利買永久升級節點提供跨局加成（起始資源、R&D× / 現金×）。

**Architecture:** `GameState.Prestige{ Patents; UnlockedPrestige[] }` 跨重開保留。`PrestigeReset` 指令：驗門檻 → 加專利 → `freshRun`（清 run-state、重播對手、套起始加成、保留 Prestige）。`BuyPrestigeNode` 花專利解鎖永久節點。`prestigeEffects()` 聚合永久加成，`Tick` 把 R&D×`RnDMult`、營收×`CashMult`。**非破壞性**：空 Prestige → 倍率 1、起始加成 0 → Plan 01–10 不變。相依 Plan 01–10。

**Tech Stack:** Go 1.22+、標準 `testing` + `math`。無外部依賴。

## Global Constraints

- 延續 Plan 01–10。`internal/sim` 純；不 mutate 輸入、clone slice（`UnlockedPrestige` append 前 clone）。
- 專利換算：`patents = floor(sqrt(PeakValuation / PatentK))`。
- `PrestigeEffects` 中性：`RnDMult=CashMult=1`、`StartCash=StartRnD=0`。聚合：加成相加、倍率相乘。
- `freshRun` 重播 `balance.DefaultCompetitors()`、`Research.EfficiencyMult=1`、`Cash = StartingCash + StartCash`、`RnD = StartRnD`，其餘 run-state 歸零，保留 `Prestige`。
- v0 數值：`PrestigeUnlockValuation=1e9`、`PatentK=1e8`、`StartingCash=100000`；永久節點 start-cash-1(1 專利,+$100k)、start-rnd-1(1,+50k R&D)、rnd-mult-1(2,R&D×1.1)、cash-mult-1(2,現金×1.1)。

---

### Task 1: `model` 擴充——Prestige 狀態 / 效果 / 節點 / 指令

**Files:**
- Modify: `internal/model/types.go`
- Test: `internal/model/types_test.go`

**Interfaces:**
- Produces:
  - `model.Prestige{ Patents float64; UnlockedPrestige []string }`
  - `model.PrestigeEffects{ StartCash, StartRnD, RnDMult, CashMult float64 }` + `model.NeutralPrestigeEffects()`（RnDMult=CashMult=1）
  - `model.PrestigeNode{ ID string; Cost float64; Effects PrestigeEffects }`
  - `GameState` 新欄位 `Prestige Prestige`
  - `model.PrestigeReset{}`、`model.BuyPrestigeNode{ NodeID string }`，各實作 `commandMarker()`

- [ ] **Step 1: 寫失敗測試**

在 `internal/model/types_test.go` 末尾新增：
```go
func TestPrestigeTypes(t *testing.T) {
	e := NeutralPrestigeEffects()
	if e.RnDMult != 1 || e.CashMult != 1 || e.StartCash != 0 {
		t.Fatalf("neutral prestige effects wrong: %+v", e)
	}
	var s GameState
	s.Prestige.Patents = 3
	s.Prestige.UnlockedPrestige = append(s.Prestige.UnlockedPrestige, "x")
	if s.Prestige.Patents != 3 || len(s.Prestige.UnlockedPrestige) != 1 {
		t.Fatalf("prestige state wrong: %+v", s.Prestige)
	}
	var c1 Command = PrestigeReset{}
	var c2 Command = BuyPrestigeNode{NodeID: "x"}
	if _, ok := c1.(PrestigeReset); !ok {
		t.Fatalf("PrestigeReset not a Command")
	}
	if _, ok := c2.(BuyPrestigeNode); !ok {
		t.Fatalf("BuyPrestigeNode not a Command")
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/model/`
Expected: FAIL（`undefined: NeutralPrestigeEffects` 等）。

- [ ] **Step 3: 寫最小實作**

在 `internal/model/types.go` 末尾新增：
```go
// Prestige is cross-run persistent progression.
type Prestige struct {
	Patents          float64
	UnlockedPrestige []string
}

// PrestigeEffects are permanent bonuses; neutral = mults 1, adds 0.
type PrestigeEffects struct {
	StartCash float64
	StartRnD  float64
	RnDMult   float64
	CashMult  float64
}

// NeutralPrestigeEffects returns effects that change nothing.
func NeutralPrestigeEffects() PrestigeEffects {
	return PrestigeEffects{RnDMult: 1, CashMult: 1}
}

// PrestigeNode is a permanent-upgrade-tree entry bought with patents.
type PrestigeNode struct {
	ID      string
	Cost    float64
	Effects PrestigeEffects
}

// PrestigeReset resets the run, banking patents from peak valuation.
type PrestigeReset struct{}

func (PrestigeReset) commandMarker() {}

// BuyPrestigeNode spends patents on a permanent upgrade.
type BuyPrestigeNode struct {
	NodeID string
}

func (BuyPrestigeNode) commandMarker() {}
```

在 `GameState` 結構加入欄位（放在 `MilestonesReached` 之後）：
```go
	Prestige Prestige
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/model/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/model/types.go internal/model/types_test.go
git commit -m "feat(model): add prestige state, effects, nodes, commands"
```

---

### Task 2: `balance` — prestige 數值與永久節點

**Files:**
- Modify: `internal/balance/balance.go`
- Test: `internal/balance/balance_test.go`

**Interfaces:**
- Produces:
  - `Config` 新欄位 `PrestigeNodes []model.PrestigeNode`、`PrestigeUnlockValuation, PatentK, StartingCash float64` + `Default()`
  - `balance.DefaultPrestigeNodes() []model.PrestigeNode`

- [ ] **Step 1: 寫失敗測試**

在 `internal/balance/balance_test.go` 末尾新增：
```go
func TestDefaultPrestige(t *testing.T) {
	c := Default()
	if c.PrestigeUnlockValuation != 1e9 || c.PatentK != 1e8 || c.StartingCash != 100000 {
		t.Errorf("prestige scalars wrong: %+v", c)
	}
	byID := map[string]model.PrestigeNode{}
	for _, n := range c.PrestigeNodes {
		byID[n.ID] = n
	}
	if n, ok := byID["start-cash-1"]; !ok || n.Cost != 1 || n.Effects.StartCash != 100000 {
		t.Errorf("start-cash-1 wrong: %+v ok=%v", n, ok)
	}
	if n, ok := byID["rnd-mult-1"]; !ok || n.Effects.RnDMult != 1.1 {
		t.Errorf("rnd-mult-1 wrong: %+v", n)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/balance/`
Expected: FAIL（欄位不存在）。

- [ ] **Step 3: 寫最小實作**

在 `internal/balance/balance.go` 的 `Config` 結構末尾加入欄位：
```go
	// Prestige (plan-11).
	PrestigeNodes           []model.PrestigeNode
	PrestigeUnlockValuation float64
	PatentK                 float64
	StartingCash            float64
```

在 `Default()` 的 `return c` 之前加入：
```go
	c.PrestigeUnlockValuation = 1e9
	c.PatentK = 1e8
	c.StartingCash = 100000
	c.PrestigeNodes = DefaultPrestigeNodes()
```

在檔案末尾新增：
```go
// DefaultPrestigeNodes returns the v0 permanent-upgrade catalog (spec §17.4).
func DefaultPrestigeNodes() []model.PrestigeNode {
	e := model.NeutralPrestigeEffects
	startCash := e()
	startCash.StartCash = 100000
	startRnD := e()
	startRnD.StartRnD = 50000
	rndMult := e()
	rndMult.RnDMult = 1.1
	cashMult := e()
	cashMult.CashMult = 1.1
	return []model.PrestigeNode{
		{ID: "start-cash-1", Cost: 1, Effects: startCash},
		{ID: "start-rnd-1", Cost: 1, Effects: startRnD},
		{ID: "rnd-mult-1", Cost: 2, Effects: rndMult},
		{ID: "cash-mult-1", Cost: 2, Effects: cashMult},
	}
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/balance/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/balance/balance.go internal/balance/balance_test.go
git commit -m "feat(balance): add prestige nodes and constants"
```

---

### Task 3: `sim` — prestige 效果聚合 / 專利換算 / freshRun

**Files:**
- Create: `internal/sim/prestige.go`
- Test: `internal/sim/prestige_test.go`

**Interfaces:**
- Consumes: `GameState.Prestige`；`balance`。
- Produces:
  - `sim.isPrestigeUnlocked(s model.GameState, id string) bool`
  - `sim.prestigeEffects(unlocked []string, b balance.Config) model.PrestigeEffects`（聚合；無解鎖 → 中性）
  - `sim.patentsFor(peak float64, b balance.Config) float64`（`floor(sqrt(peak/K))`）
  - `sim.freshRun(p model.Prestige, b balance.Config) model.GameState`（清 run-state、重播對手、套起始加成、保留 Prestige）

- [ ] **Step 1: 寫失敗測試**

Create `internal/sim/prestige_test.go`:
```go
package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestPrestigeEffectsAggregate(t *testing.T) {
	b := balance.Default()
	pe := prestigeEffects([]string{"start-cash-1", "rnd-mult-1"}, b)
	if !approx(pe.StartCash, 100000) {
		t.Errorf("StartCash = %v, want 100000", pe.StartCash)
	}
	if !approx(pe.RnDMult, 1.1) {
		t.Errorf("RnDMult = %v, want 1.1", pe.RnDMult)
	}
	if !approx(pe.CashMult, 1) {
		t.Errorf("unrelated mult should be 1: %v", pe.CashMult)
	}
}

func TestPatentsFor(t *testing.T) {
	b := balance.Default() // PatentK 1e8
	if got := patentsFor(1e9, b); got != 3 { // floor(sqrt(10))
		t.Errorf("patentsFor(1e9) = %v, want 3", got)
	}
	if got := patentsFor(1e10, b); got != 10 { // floor(sqrt(100))
		t.Errorf("patentsFor(1e10) = %v, want 10", got)
	}
}

func TestFreshRun(t *testing.T) {
	b := balance.Default()
	p := model.Prestige{Patents: 5, UnlockedPrestige: []string{"start-cash-1"}} // +100k cash
	ns := freshRun(p, b)
	if ns.Prestige.Patents != 5 {
		t.Errorf("patents not preserved: %v", ns.Prestige.Patents)
	}
	if len(ns.Competitors) != 7 {
		t.Errorf("competitors not re-seeded")
	}
	if !approx(ns.Resources.Cash, b.StartingCash+100000) {
		t.Errorf("cash = %v, want %v", ns.Resources.Cash, b.StartingCash+100000)
	}
	if ns.Research.EfficiencyMult != 1 {
		t.Errorf("efficiency mult not reset to 1")
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run 'TestPrestigeEffectsAggregate|TestPatentsFor|TestFreshRun'`
Expected: FAIL（`undefined: prestigeEffects` 等）。

- [ ] **Step 3: 寫最小實作**

Create `internal/sim/prestige.go`:
```go
package sim

import (
	"math"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func isPrestigeUnlocked(s model.GameState, id string) bool {
	for _, u := range s.Prestige.UnlockedPrestige {
		if u == id {
			return true
		}
	}
	return false
}

// prestigeEffects aggregates permanent upgrades (neutral when none unlocked).
func prestigeEffects(unlocked []string, b balance.Config) model.PrestigeEffects {
	agg := model.NeutralPrestigeEffects()
	for _, node := range b.PrestigeNodes {
		if !contains(unlocked, node.ID) {
			continue
		}
		agg.StartCash += node.Effects.StartCash
		agg.StartRnD += node.Effects.StartRnD
		agg.RnDMult *= node.Effects.RnDMult
		agg.CashMult *= node.Effects.CashMult
	}
	return agg
}

func contains(ss []string, id string) bool {
	for _, s := range ss {
		if s == id {
			return true
		}
	}
	return false
}

// patentsFor is the patents earned by prestiging at a given peak valuation.
func patentsFor(peak float64, b balance.Config) float64 {
	if peak <= 0 {
		return 0
	}
	return math.Floor(math.Sqrt(peak / b.PatentK))
}

// freshRun produces a new run's starting state, preserving prestige.
func freshRun(p model.Prestige, b balance.Config) model.GameState {
	pe := prestigeEffects(p.UnlockedPrestige, b)
	var ns model.GameState
	ns.Prestige = p
	ns.Competitors = balance.DefaultCompetitors()
	ns.Research.EfficiencyMult = 1
	ns.Resources.Cash = b.StartingCash + pe.StartCash
	ns.Resources.RnD = pe.StartRnD
	return ns
}
```

> `contains` 若與既有同名函式衝突，改用既有的；本 plan 假設尚無 `contains`。

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/ -run 'TestPrestigeEffectsAggregate|TestPatentsFor|TestFreshRun'`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/sim/prestige.go internal/sim/prestige_test.go
git commit -m "feat(sim): prestige effects, patents, fresh-run seeding"
```

---

### Task 4: `sim.Apply` — BuyPrestigeNode

**Files:**
- Modify: `internal/sim/apply.go`
- Test: `internal/sim/apply_test.go`

**Interfaces:**
- Consumes: `model.BuyPrestigeNode`；`balance.PrestigeNodes`。
- Produces:
  - package vars `ErrInvalidPrestigeNode`, `ErrInsufficientPatents`
  - `sim.findPrestigeNode(nodes []model.PrestigeNode, id string) (model.PrestigeNode, bool)`
  - `Apply` 新增 `BuyPrestigeNode`：找節點；未解鎖；專利足夠 → 扣專利、append（clone）。

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/apply_test.go` 末尾新增：
```go
func TestApplyBuyPrestigeNode(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Prestige.Patents = 5
	ns, err := Apply(s, model.BuyPrestigeNode{NodeID: "start-cash-1"}, b) // cost 1
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ns.Prestige.Patents != 4 {
		t.Errorf("patents = %v, want 4", ns.Prestige.Patents)
	}
	if len(ns.Prestige.UnlockedPrestige) != 1 || ns.Prestige.UnlockedPrestige[0] != "start-cash-1" {
		t.Errorf("not unlocked: %+v", ns.Prestige.UnlockedPrestige)
	}
	if len(s.Prestige.UnlockedPrestige) != 0 {
		t.Errorf("Apply mutated input")
	}
}

func TestApplyBuyPrestigeErrors(t *testing.T) {
	b := balance.Default()
	rich := model.GameState{}
	rich.Prestige.Patents = 100
	if _, err := Apply(rich, model.BuyPrestigeNode{NodeID: "nope"}, b); err != ErrInvalidPrestigeNode {
		t.Errorf("invalid: err = %v, want ErrInvalidPrestigeNode", err)
	}
	if _, err := Apply(rich, model.BuyPrestigeNode{NodeID: "start-cash-1"}, b); err != nil {
		t.Errorf("rich buy should succeed: %v", err)
	}
	already := model.GameState{}
	already.Prestige.Patents = 100
	already.Prestige.UnlockedPrestige = []string{"start-cash-1"}
	if _, err := Apply(already, model.BuyPrestigeNode{NodeID: "start-cash-1"}, b); err != ErrAlreadyUnlocked {
		t.Errorf("already: err = %v, want ErrAlreadyUnlocked", err)
	}
	poor := model.GameState{}
	poor.Prestige.Patents = 0
	if _, err := Apply(poor, model.BuyPrestigeNode{NodeID: "start-cash-1"}, b); err != ErrInsufficientPatents {
		t.Errorf("patents: err = %v, want ErrInsufficientPatents", err)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run TestApplyBuyPrestige`
Expected: FAIL（`undefined: ErrInvalidPrestigeNode` 等）。

- [ ] **Step 3: 寫最小實作**

在 `internal/sim/apply.go` 的 error var 區塊加入：
```go
	ErrInvalidPrestigeNode = errors.New("sim: unknown prestige node")
	ErrInsufficientPatents = errors.New("sim: insufficient patents")
```

在 `Apply` 的 switch 內、`UnlockTech` case 之後新增：
```go
	case model.BuyPrestigeNode:
		return applyBuyPrestigeNode(s, c, b)
```

新增函式：
```go
func findPrestigeNode(nodes []model.PrestigeNode, id string) (model.PrestigeNode, bool) {
	for _, n := range nodes {
		if n.ID == id {
			return n, true
		}
	}
	return model.PrestigeNode{}, false
}

func applyBuyPrestigeNode(s model.GameState, c model.BuyPrestigeNode, b balance.Config) (model.GameState, error) {
	node, ok := findPrestigeNode(b.PrestigeNodes, c.NodeID)
	if !ok {
		return s, ErrInvalidPrestigeNode
	}
	if isPrestigeUnlocked(s, node.ID) {
		return s, ErrAlreadyUnlocked
	}
	if s.Prestige.Patents < node.Cost {
		return s, ErrInsufficientPatents
	}
	ns := s
	ns.Prestige.Patents -= node.Cost
	ns.Prestige.UnlockedPrestige = append(append([]string(nil), s.Prestige.UnlockedPrestige...), node.ID)
	return ns, nil
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/ -run TestApply`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/sim/apply.go internal/sim/apply_test.go
git commit -m "feat(sim): add BuyPrestigeNode command"
```

---

### Task 5: `sim.Apply` — PrestigeReset

**Files:**
- Modify: `internal/sim/apply.go`
- Test: `internal/sim/apply_test.go`

**Interfaces:**
- Consumes: `model.PrestigeReset`；`balance.PrestigeUnlockValuation`；`patentsFor` / `freshRun`（Task 3）。
- Produces:
  - package var `ErrPrestigeLocked`
  - `Apply` 新增 `PrestigeReset`：驗 `PeakValuation >= PrestigeUnlockValuation`；`Patents += patentsFor(PeakValuation)`；回 `freshRun`（保留更新後 Prestige）。

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/apply_test.go` 末尾新增：
```go
func TestApplyPrestigeReset(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.PeakValuation = 1e9 // patents = floor(sqrt(1e9/1e8)) = 3
	s.Resources.Cash = 5e6
	s.Resources.RnD = 1e6
	s.Models = []model.Model{{Online: true}}
	s.Engineers = 5
	s.Prestige.Patents = 1
	ns, err := Apply(s, model.PrestigeReset{}, b)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ns.Prestige.Patents != 4 { // 1 existing + 3 gained
		t.Errorf("patents = %v, want 4", ns.Prestige.Patents)
	}
	if len(ns.Models) != 0 || ns.Engineers != 0 || ns.PeakValuation != 0 {
		t.Errorf("run state not reset: %+v", ns)
	}
	if !approx(ns.Resources.Cash, b.StartingCash) {
		t.Errorf("cash not reset to starting: %v", ns.Resources.Cash)
	}
	if len(ns.Competitors) != 7 {
		t.Errorf("competitors not re-seeded")
	}
}

func TestApplyPrestigeLocked(t *testing.T) {
	b := balance.Default()
	s := model.GameState{} // peak 0 < 1e9
	if _, err := Apply(s, model.PrestigeReset{}, b); err != ErrPrestigeLocked {
		t.Fatalf("err = %v, want ErrPrestigeLocked", err)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run TestApplyPrestige`
Expected: FAIL（`undefined: ErrPrestigeLocked`）。

- [ ] **Step 3: 寫最小實作**

在 `internal/sim/apply.go` 的 error var 區塊加入：
```go
	ErrPrestigeLocked = errors.New("sim: prestige not unlocked")
```

在 `Apply` 的 switch 內、`BuyPrestigeNode` case 之後新增：
```go
	case model.PrestigeReset:
		return applyPrestigeReset(s, b)
```

新增函式：
```go
func applyPrestigeReset(s model.GameState, b balance.Config) (model.GameState, error) {
	if s.PeakValuation < b.PrestigeUnlockValuation {
		return s, ErrPrestigeLocked
	}
	p := s.Prestige
	p.Patents += patentsFor(s.PeakValuation, b)
	return freshRun(p, b), nil
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/ -run TestApply`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/sim/apply.go internal/sim/apply_test.go
git commit -m "feat(sim): add PrestigeReset command"
```

---

### Task 6: `Tick` — prestige 永久加成（R&D× / 現金×）

**Files:**
- Modify: `internal/sim/sim.go`
- Test: `internal/sim/sim_test.go`

**Interfaces:**
- Consumes: `prestigeEffects`。
- Produces: `Tick` 把新增 R&D 乘 `RnDMult`；`advanceUsers` 訂閱營收乘 `CashMult`。

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/sim_test.go` 末尾新增：
```go
func TestPrestigeRnDMult(t *testing.T) {
	b := balance.Default()
	base := model.GameState{Research: model.Research{EfficiencyMult: 1}}
	base.Research.Researchers[model.Tier2] = 10 // 150 R&D/s
	withP := base
	withP.Prestige.UnlockedPrestige = []string{"rnd-mult-1"} // R&D ×1.1
	nb := Tick(base, 1, nil, b)
	np := Tick(withP, 1, nil, b)
	if np.Resources.RnD <= nb.Resources.RnD {
		t.Fatalf("prestige RnD mult should boost R&D: %v vs %v", np.Resources.RnD, nb.Resources.RnD)
	}
}

func TestPrestigeCashMult(t *testing.T) {
	b := balance.Default()
	m := onlineModel(50, 12)
	m.Users = 1000
	base := model.GameState{Models: []model.Model{m}}
	withP := model.GameState{Models: []model.Model{m}}
	withP.Prestige.UnlockedPrestige = []string{"cash-mult-1"} // cash ×1.1
	nb := Tick(base, 1, nil, b)
	np := Tick(withP, 1, nil, b)
	if np.Resources.Cash <= nb.Resources.Cash {
		t.Fatalf("prestige cash mult should boost revenue: %v vs %v", np.Resources.Cash, nb.Resources.Cash)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run TestPrestige`
Expected: FAIL（加成未生效）。

- [ ] **Step 3: 寫最小實作**

**(a)** `internal/sim/sim.go` 的 `Tick` 內，把 `ns.Resources.RnD += staffRnD + tokenRnD` 改為：
```go
	pe := prestigeEffects(ns.Prestige.UnlockedPrestige, b)
	ns.Resources.RnD += (staffRnD + tokenRnD) * pe.RnDMult
```

**(b)** `internal/sim/sim.go` 的 `advanceUsers` 內，在 `models := append(...)` 之前取 `pe`，並把訂閱營收行乘 `pe.CashMult`：
```go
	pe := prestigeEffects(ns.Prestige.UnlockedPrestige, b)
	models := append([]model.Model(nil), ns.Models...)
	for i := range models {
		m := &models[i]
		if !m.Online {
			continue
		}
		ns.Resources.Cash += m.Users * m.Price * dt / b.MonthSec * pe.CashMult
```
（其餘迴圈內容不變。）

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/`
Expected: PASS（全部——空 Prestige → 倍率 1 → Plan 01–10 不變）。

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
git commit -m "feat(sim): apply prestige R&D and cash multipliers"
```

---

## 完成後狀態

Prestige 成立：估值破 $1B 後 `PrestigeReset` 換專利（`floor(√(peak/K))`）並重開（清 run-state、重播對手、保留專利與永久升級）；`BuyPrestigeNode` 花專利買永久加成（起始資源、R&D× / 現金×），下局更快。無 prestige → 全中性 → Plan 01–10 不變。仍純確定性。無盡重玩循環閉環。

**下一步**：明星員工（挖角/留任）、真實 token 採集（ingest），然後 store / daemon+IPC / 完整 TUI。
