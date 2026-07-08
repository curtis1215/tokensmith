# Tokensmith 10 — 估值 · 里程碑 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 加入公司估值計算與里程碑追蹤：每 tick 算估值、記錄峰值、依序達成估值里程碑（$1M → $1T）。

**Architecture:** `sim.Valuation(ns,b)` = 月營收×倍數 + 用戶價值 + 資產（現金 + 服務器）。`Tick` 末尾更新 `GameState.PeakValuation`（單調峰值）與 `MilestonesReached`（依峰值跨過的有序里程碑數）。**非破壞性**：只新增這兩個欄位與追蹤，不動既有數值 → Plan 01–09 測試不變。里程碑用整數計數（供 daemon/TUI 偵測新達成 + 供 prestige plan 用），不改 Tick 簽名。相依 Plan 01–09。

**Tech Stack:** Go 1.22+、標準 `testing` + `math`。無外部依賴。

## Global Constraints

- 延續 Plan 01–09。`internal/sim` 純；不 mutate 輸入。
- 估值：`Valuation = Σ(online Users·Price)·RevenueMultiple + Σ(online Users)·UserValue + Cash + Σ(Server.Compute)·ServerAssetValue`。
- 里程碑：`ValuationMilestones` 為遞增門檻；`MilestonesReached` 依 `PeakValuation` 單調前進。
- v0 數值（spec §12）：`ValuationMilestones = {1e6,1e7,1e8,1e9,1e10,1e11,1e12}`、`RevenueMultiple=120`、`UserValue=10`、`ServerAssetValue=5000`。

---

### Task 1: `model` 擴充——估值 / 里程碑欄位

**Files:**
- Modify: `internal/model/types.go`
- Test: `internal/model/types_test.go`

**Interfaces:**
- Produces: `GameState` 新欄位 `PeakValuation float64`、`MilestonesReached int`。

- [ ] **Step 1: 寫失敗測試**

在 `internal/model/types_test.go` 末尾新增：
```go
func TestValuationFields(t *testing.T) {
	var s GameState
	s.PeakValuation = 1_500_000
	s.MilestonesReached = 1
	if s.PeakValuation != 1_500_000 || s.MilestonesReached != 1 {
		t.Fatalf("valuation fields wrong: %+v", s)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/model/`
Expected: FAIL（欄位不存在）。

- [ ] **Step 3: 寫最小實作**

在 `GameState` 結構加入欄位（放在 `UnlockedTech` 之後）：
```go
	PeakValuation     float64
	MilestonesReached int
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/model/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/model/types.go internal/model/types_test.go
git commit -m "feat(model): add peak valuation and milestones-reached fields"
```

---

### Task 2: `balance` — 估值 / 里程碑數值

**Files:**
- Modify: `internal/balance/balance.go`
- Test: `internal/balance/balance_test.go`

**Interfaces:**
- Produces: `Config` 新欄位 `ValuationMilestones []float64`、`RevenueMultiple, UserValue, ServerAssetValue float64` + `Default()` 填值。

- [ ] **Step 1: 寫失敗測試**

在 `internal/balance/balance_test.go` 末尾新增：
```go
func TestDefaultValuationValues(t *testing.T) {
	c := Default()
	if len(c.ValuationMilestones) != 7 {
		t.Fatalf("milestones = %d, want 7", len(c.ValuationMilestones))
	}
	if c.ValuationMilestones[0] != 1e6 || c.ValuationMilestones[3] != 1e9 {
		t.Errorf("milestone thresholds wrong: %v", c.ValuationMilestones)
	}
	if c.RevenueMultiple != 120 || c.UserValue != 10 || c.ServerAssetValue != 5000 {
		t.Errorf("valuation params wrong: %+v", c)
	}
	// milestones strictly increasing
	for i := 1; i < len(c.ValuationMilestones); i++ {
		if c.ValuationMilestones[i] <= c.ValuationMilestones[i-1] {
			t.Errorf("milestones must be increasing at %d", i)
		}
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/balance/`
Expected: FAIL（欄位不存在）。

- [ ] **Step 3: 寫最小實作**

在 `internal/balance/balance.go` 的 `Config` 結構末尾加入欄位：
```go
	// Valuation & milestones (plan-10).
	ValuationMilestones []float64
	RevenueMultiple     float64 // monthly revenue → valuation multiple
	UserValue           float64 // valuation per active user
	ServerAssetValue    float64 // valuation per unit of self-built compute
```

在 `Default()` 的 `return c` 之前加入：
```go
	c.ValuationMilestones = []float64{1e6, 1e7, 1e8, 1e9, 1e10, 1e11, 1e12}
	c.RevenueMultiple = 120
	c.UserValue = 10
	c.ServerAssetValue = 5000
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/balance/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/balance/balance.go internal/balance/balance_test.go
git commit -m "feat(balance): add valuation and milestone thresholds"
```

---

### Task 3: `sim` — 估值計算與里程碑追蹤

**Files:**
- Modify: `internal/sim/sim.go`
- Test: `internal/sim/sim_test.go`

**Interfaces:**
- Consumes: `GameState`；`balance` 估值欄位。
- Produces:
  - `sim.Valuation(ns model.GameState, b balance.Config) float64`（**匯出**，供 daemon/TUI）
  - `Tick` 末尾：算估值、更新 `PeakValuation`（取 max）、依峰值前進 `MilestonesReached`。

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/sim_test.go` 末尾新增：
```go
func TestValuation(t *testing.T) {
	b := balance.Default()
	m := onlineModel(50, 12)
	m.Users = 1000
	s := model.GameState{Models: []model.Model{m}}
	s.Resources.Cash = 50000
	// monthlyRev 1000*12=12000; *120 = 1.44M; users 1000*10=10000; cash 50000 → 1.5M
	if !approx(Valuation(s, b), 1_500_000) {
		t.Fatalf("valuation = %v, want 1500000", Valuation(s, b))
	}
}

func TestTickTracksMilestones(t *testing.T) {
	b := balance.Default()
	m := onlineModel(50, 12)
	m.Users = 1000
	s := model.GameState{Models: []model.Model{m}}
	s.Resources.Cash = 50000
	ns := Tick(s, 1, nil, b)
	if ns.PeakValuation < 1_000_000 {
		t.Errorf("peak valuation not tracked: %v", ns.PeakValuation)
	}
	if ns.MilestonesReached < 1 {
		t.Errorf("should reach $1M milestone, reached=%d peak=%v", ns.MilestonesReached, ns.PeakValuation)
	}
}

func TestPeakValuationIsMonotonic(t *testing.T) {
	b := balance.Default()
	// a model whose users will decay (price way above ref → target ~0)
	m := onlineModel(50, 100*b.RefPrice)
	m.Users = 100000
	s := model.GameState{Models: []model.Model{m}}
	s.Resources.Cash = 1e7
	ns := Tick(s, 1, nil, b)
	peak1 := ns.PeakValuation
	// drop cash to force lower valuation; peak must not decrease
	ns.Resources.Cash = 0
	ns2 := Tick(ns, 1, nil, b)
	if ns2.PeakValuation < peak1 {
		t.Fatalf("peak valuation decreased: %v -> %v", peak1, ns2.PeakValuation)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run 'TestValuation|TestTickTracksMilestones|TestPeakValuationIsMonotonic'`
Expected: FAIL（`undefined: Valuation` / 未追蹤）。

- [ ] **Step 3: 寫最小實作**

在 `internal/sim/sim.go` 新增（放在 `Tick` 之前）：
```go
// Valuation is the company's estimated worth (spec §7.0).
func Valuation(ns model.GameState, b balance.Config) float64 {
	var monthlyRev, users float64
	for _, m := range ns.Models {
		if m.Online {
			monthlyRev += m.Users * m.Price
			users += m.Users
		}
	}
	assets := ns.Resources.Cash
	for _, sv := range ns.Servers {
		assets += sv.Compute * b.ServerAssetValue
	}
	return monthlyRev*b.RevenueMultiple + users*b.UserValue + assets
}
```

在 `Tick` 內，`return ns` 之前加入里程碑追蹤：
```go
	val := Valuation(ns, b)
	if val > ns.PeakValuation {
		ns.PeakValuation = val
	}
	for ns.MilestonesReached < len(b.ValuationMilestones) &&
		ns.PeakValuation >= b.ValuationMilestones[ns.MilestonesReached] {
		ns.MilestonesReached++
	}
	return ns
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/`
Expected: PASS（全部——新增追蹤不影響既有數值斷言）。

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
git commit -m "feat(sim): compute valuation and track milestones"
```

---

## 完成後狀態

估值與里程碑成立：每 tick 算估值、記單調峰值、依序達成 $1M→$1T 里程碑（供 daemon/TUI 顯示與通知、供 prestige 換算）。`Valuation` 匯出供前端顯示。無事件通道改動 → Plan 01–09 不變。仍純確定性。

**下一步（Plan 11）**：prestige——用峰值估值換專利（`floor(√(peak/K))`）、重開 reset（清 run-state、保留專利與永久升級）、永久升級樹（patents 買永久倍率）。
