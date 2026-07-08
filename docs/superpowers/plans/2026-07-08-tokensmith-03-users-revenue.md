# Tokensmith 03 — 用戶 · 營收 · 訂價 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 讓已上線模型長用戶、產生訂閱營收，並加入 `SetPrice` 指令與需求價格彈性（定價越高、用戶越少）。

**Architecture:** `Tick` 新增 `advanceUsers`：對每個 online 模型，依「聚合品質吸引力 × 需求乘數（價格彈性）」算出目標用戶、指數逼近成長，並依用戶數 × 定價累積訂閱現金。新增 `SetPrice` 指令。仍純確定性。本 plan 簡化：單一用戶池、聚合品質（加權和）、無區隔 / 無對手（Plan 04）。相依 Plan 01–02。

**Tech Stack:** Go 1.22+、標準 `testing` + `math`（`math.Pow` 為確定性、允許）。無外部依賴。

## Global Constraints

- 延續 Plan 01–02 全部約束。`internal/sim` 仍不得 wall-clock / rand / 檔案·網路 I/O；**`math` 允許**（純數學、確定性）。
- `Apply` / `Tick` 不得 mutate 輸入；修改 `Models` 元素前先 clone slice。
- v0 數值（本 plan 新增，集中於 `balance`，皆可調）：`QualityWeights={0.4,0.2,0.2,0.2}`、`UserTargetPerAppeal=1000`、`UserGrowthRate=0.001`、`RefPrice=12`、`PriceElasticity=1.5`、`MonthSec=2592000`（30×86400，訂閱月費換算；v0 可調）。

---

### Task 1: `balance` 擴充——用戶 / 營收 / 訂價數值

**Files:**
- Modify: `internal/balance/balance.go`
- Test: `internal/balance/balance_test.go`

**Interfaces:**
- Consumes: `model.NumQualityDims`（Plan 02）。
- Produces: `Config` 新欄位 + `Default()` 填值：
  - `QualityWeights [model.NumQualityDims]float64`
  - `UserTargetPerAppeal float64`
  - `UserGrowthRate float64`
  - `RefPrice float64`
  - `PriceElasticity float64`
  - `MonthSec float64`

- [ ] **Step 1: 寫失敗測試**

在 `internal/balance/balance_test.go` 末尾新增：
```go
func TestDefaultUserRevenueValues(t *testing.T) {
	c := Default()
	if c.QualityWeights[model.DimCapability] != 0.4 {
		t.Errorf("QualityWeights[cap] = %v, want 0.4", c.QualityWeights[model.DimCapability])
	}
	var sum float64
	for _, w := range c.QualityWeights {
		sum += w
	}
	if sum < 0.999 || sum > 1.001 {
		t.Errorf("QualityWeights sum = %v, want 1", sum)
	}
	if c.UserTargetPerAppeal != 1000 || c.UserGrowthRate != 0.001 {
		t.Errorf("user growth params wrong: %+v", c)
	}
	if c.RefPrice != 12 || c.PriceElasticity != 1.5 {
		t.Errorf("pricing params wrong: %+v", c)
	}
	if c.MonthSec != 2592000 {
		t.Errorf("MonthSec = %v, want 2592000", c.MonthSec)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/balance/`
Expected: FAIL（欄位不存在）。

- [ ] **Step 3: 寫最小實作**

在 `internal/balance/balance.go` 的 `Config` 結構末尾加入欄位：
```go
	// User attraction & subscription revenue (plan-03).
	QualityWeights      [model.NumQualityDims]float64 // aggregate appeal weights
	UserTargetPerAppeal float64                       // target users per unit appeal
	UserGrowthRate      float64                       // per-second approach to target
	RefPrice            float64                       // reference price for elasticity
	PriceElasticity     float64                       // demand elasticity exponent
	MonthSec            float64                       // seconds per month (price is per-user-per-month)
```

在 `Default()` 的 `return c` 之前加入：
```go
	c.QualityWeights = [model.NumQualityDims]float64{0.4, 0.2, 0.2, 0.2}
	c.UserTargetPerAppeal = 1000
	c.UserGrowthRate = 0.001
	c.RefPrice = 12
	c.PriceElasticity = 1.5
	c.MonthSec = 2592000
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/balance/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/balance/balance.go internal/balance/balance_test.go
git commit -m "feat(balance): add user growth, pricing, revenue v0 values"
```

---

### Task 2: `model` 擴充——SetPrice 指令

**Files:**
- Modify: `internal/model/types.go`
- Test: `internal/model/types_test.go`

**Interfaces:**
- Consumes: `model.Command`（Plan 02）。
- Produces: `model.SetPrice{ ModelIndex int; Price float64 }`，實作 `commandMarker()`。

- [ ] **Step 1: 寫失敗測試**

在 `internal/model/types_test.go` 末尾新增：
```go
func TestSetPriceIsCommand(t *testing.T) {
	var c Command = SetPrice{ModelIndex: 0, Price: 15}
	sp, ok := c.(SetPrice)
	if !ok || sp.Price != 15 || sp.ModelIndex != 0 {
		t.Fatalf("SetPrice command wrong: %+v ok=%v", c, ok)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/model/`
Expected: FAIL（`undefined: SetPrice`）。

- [ ] **Step 3: 寫最小實作**

在 `internal/model/types.go` 末尾（`RentTrainingCompute` 之後）新增：
```go
// SetPrice changes the monthly price of the model at ModelIndex.
type SetPrice struct {
	ModelIndex int
	Price      float64
}

func (SetPrice) commandMarker() {}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/model/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/model/types.go internal/model/types_test.go
git commit -m "feat(model): add SetPrice command"
```

---

### Task 3: `sim.Apply` — SetPrice（驗證 + 改價）

**Files:**
- Modify: `internal/sim/apply.go`
- Test: `internal/sim/apply_test.go`

**Interfaces:**
- Consumes: `model.SetPrice`（Task 2）。
- Produces:
  - package vars `ErrInvalidModelIndex`, `ErrInvalidPrice`（`error`）
  - `Apply` 新增處理 `model.SetPrice`：驗證 index 於 `[0,len(Models))` 且 `Price > 0` → clone `Models` 後改該模型 `Price`。

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/apply_test.go` 末尾新增：
```go
func TestApplySetPriceSuccess(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Models: []model.Model{{Online: true, Price: 12}}}
	ns, err := Apply(s, model.SetPrice{ModelIndex: 0, Price: 20}, b)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ns.Models[0].Price != 20 {
		t.Errorf("price = %v, want 20", ns.Models[0].Price)
	}
	if s.Models[0].Price != 12 {
		t.Errorf("Apply mutated input Models (price = %v)", s.Models[0].Price)
	}
}

func TestApplySetPriceErrors(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Models: []model.Model{{Online: true, Price: 12}}}
	if _, err := Apply(s, model.SetPrice{ModelIndex: 5, Price: 20}, b); err != ErrInvalidModelIndex {
		t.Errorf("index: err = %v, want ErrInvalidModelIndex", err)
	}
	if _, err := Apply(s, model.SetPrice{ModelIndex: 0, Price: 0}, b); err != ErrInvalidPrice {
		t.Errorf("price: err = %v, want ErrInvalidPrice", err)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run TestApplySetPrice`
Expected: FAIL（`undefined: ErrInvalidModelIndex`）。

- [ ] **Step 3: 寫最小實作**

在 `internal/sim/apply.go` 的 error var 區塊加入兩個：
```go
	ErrInvalidModelIndex = errors.New("sim: invalid model index")
	ErrInvalidPrice      = errors.New("sim: price must be positive")
```

在 `Apply` 的 switch 內、`StartTraining` case 之後新增：
```go
	case model.SetPrice:
		return applySetPrice(s, c)
```

新增函式：
```go
func applySetPrice(s model.GameState, c model.SetPrice) (model.GameState, error) {
	if c.ModelIndex < 0 || c.ModelIndex >= len(s.Models) {
		return s, ErrInvalidModelIndex
	}
	if c.Price <= 0 {
		return s, ErrInvalidPrice
	}
	ns := s
	ns.Models = append([]model.Model(nil), s.Models...)
	ns.Models[c.ModelIndex].Price = c.Price
	return ns, nil
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/ -run TestApply`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/sim/apply.go internal/sim/apply_test.go
git commit -m "feat(sim): add SetPrice command with validation"
```

---

### Task 4: `Tick` — 用戶成長（吸引力 × 需求彈性）

**Files:**
- Modify: `internal/sim/sim.go`
- Test: `internal/sim/sim_test.go`

**Interfaces:**
- Consumes: `balance` 的 `QualityWeights` / `UserTargetPerAppeal` / `UserGrowthRate` / `RefPrice` / `PriceElasticity`；`model.Model`。
- Produces: `sim.advanceUsers(ns, dt, b) model.GameState`（clone `Models`，對每個 online 模型：`appeal=Σ Quality[d]·Weight[d]`；`demandMult=(RefPrice/Price)^Elasticity`（`Price<=0` 時為 0）；`target=appeal·UserTargetPerAppeal·demandMult`；`Users += (target-Users)·UserGrowthRate·dt`，夾 `>=0`）；`Tick` 於 `advanceTraining` 後呼叫。

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/sim_test.go` 頂部 import 區塊確認含 `"math"`（若無則加入）。在檔案末尾新增：
```go
func onlineModel(cap, price float64) model.Model {
	m := model.Model{Online: true, Price: price}
	m.Quality[model.DimCapability] = cap
	return m
}

func TestTickUserGrowthTowardTarget(t *testing.T) {
	b := balance.Default()
	// appeal = 50 * 0.4 = 20; price = ref → demandMult 1; target = 20*1000 = 20000.
	s := model.GameState{Models: []model.Model{onlineModel(50, b.RefPrice)}}
	ns := Tick(s, 1, nil, b) // Users += (20000-0)*0.001*1 = 20
	if !approx(ns.Models[0].Users, 20) {
		t.Fatalf("Users = %v, want 20", ns.Models[0].Users)
	}
	// input not mutated
	if s.Models[0].Users != 0 {
		t.Fatalf("Tick mutated input Users")
	}
}

func TestTickPriceElasticityReducesTarget(t *testing.T) {
	b := balance.Default()
	// double the reference price → demandMult = (1/2)^1.5.
	s := model.GameState{Models: []model.Model{onlineModel(50, 2*b.RefPrice)}}
	ns := Tick(s, 1, nil, b)
	wantTarget := 20.0 * b.UserTargetPerAppeal * math.Pow(0.5, b.PriceElasticity) // appeal 20
	wantUsers := wantTarget * b.UserGrowthRate * 1
	if !approx(ns.Models[0].Users, wantUsers) {
		t.Fatalf("Users = %v, want %v", ns.Models[0].Users, wantUsers)
	}
}

func TestTickHighPriceChurns(t *testing.T) {
	b := balance.Default()
	m := onlineModel(50, 2*b.RefPrice) // target well below 30000
	m.Users = 30000
	s := model.GameState{Models: []model.Model{m}}
	ns := Tick(s, 1, nil, b)
	if ns.Models[0].Users >= 30000 {
		t.Fatalf("Users = %v, want < 30000 (churn)", ns.Models[0].Users)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run TestTickUser`
Expected: FAIL（用戶未成長）。

- [ ] **Step 3: 寫最小實作**

在 `internal/sim/sim.go` 的 import 區塊加入 `"math"`：
```go
import (
	"math"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)
```

在 `Tick` 內，把 `ns = advanceTraining(ns, dt, b)` 之後、`return ns` 之前改為：
```go
	ns = advanceTraining(ns, dt, b)
	ns = advanceUsers(ns, dt, b)
	return ns
```

新增函式：
```go
// advanceUsers grows each online model's user base toward a demand target and
// (in a later task) accrues subscription revenue. Pure: clones Models.
func advanceUsers(ns model.GameState, dt float64, b balance.Config) model.GameState {
	if len(ns.Models) == 0 {
		return ns
	}
	models := append([]model.Model(nil), ns.Models...)
	for i := range models {
		m := &models[i]
		if !m.Online {
			continue
		}
		appeal := 0.0
		for d := range model.NumQualityDims {
			appeal += m.Quality[d] * b.QualityWeights[d]
		}
		var demandMult float64
		if m.Price > 0 {
			demandMult = math.Pow(b.RefPrice/m.Price, b.PriceElasticity)
		}
		target := appeal * b.UserTargetPerAppeal * demandMult
		m.Users += (target - m.Users) * b.UserGrowthRate * dt
		if m.Users < 0 {
			m.Users = 0
		}
	}
	ns.Models = models
	return ns
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/`
Expected: PASS（全部）。

- [ ] **Step 5: Commit**

```bash
git add internal/sim/sim.go internal/sim/sim_test.go
git commit -m "feat(sim): grow model users with price elasticity"
```

---

### Task 5: `Tick` — 訂閱營收

**Files:**
- Modify: `internal/sim/sim.go`
- Test: `internal/sim/sim_test.go`

**Interfaces:**
- Consumes: `balance.MonthSec`。
- Produces: `advanceUsers` 對每個 online 模型於成長前依現有用戶累積訂閱現金：`Cash += Users · Price · dt / MonthSec`。

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/sim_test.go` 末尾新增：
```go
func TestTickSubscriptionRevenue(t *testing.T) {
	b := balance.Default()
	m := onlineModel(50, 12)
	m.Users = 1000
	s := model.GameState{Models: []model.Model{m}}
	ns := Tick(s, 100, nil, b)
	// revenue uses pre-growth users: 1000 * 12 * 100 / MonthSec
	want := 1000.0 * 12.0 * 100.0 / b.MonthSec
	if !approx(ns.Resources.Cash, want) {
		t.Fatalf("Cash = %v, want %v", ns.Resources.Cash, want)
	}
}

func TestTickNoRevenueWhenOffline(t *testing.T) {
	b := balance.Default()
	m := model.Model{Online: false, Price: 12, Users: 1000}
	s := model.GameState{Models: []model.Model{m}}
	ns := Tick(s, 100, nil, b)
	if !approx(ns.Resources.Cash, 0) {
		t.Fatalf("Cash = %v, want 0 (offline model)", ns.Resources.Cash)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run TestTickSubscriptionRevenue`
Expected: FAIL（Cash 為 0）。

- [ ] **Step 3: 寫最小實作**

在 `advanceUsers` 的 for 迴圈內、`if !m.Online { continue }` 之後、`appeal := 0.0` 之前加入一行：
```go
		ns.Resources.Cash += m.Users * m.Price * dt / b.MonthSec
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/`
Expected: PASS。

- [ ] **Step 5: 跑整包 + vet + commit**

Run:
```bash
go test ./...
go vet ./...
```
Expected: 全部 PASS、vet 無警告。

```bash
git add internal/sim/sim.go internal/sim/sim_test.go
git commit -m "feat(sim): accrue subscription revenue per tick"
```

---

## 完成後狀態

完整最小經濟閉環：訓練上線模型 → 依品質吸引力與定價彈性長用戶 → 用戶產生訂閱現金 → 現金可再投資（未來 plan 的算力 / 團隊）。定價是玩家槓桿（高價高毛利低成長、低價衝量）。仍純確定性、不 mutate 輸入。

**下一步（Plan 04）**：四維品質分區隔（消費者 / 企業 / 開發者）、具名對手與市佔競爭、相對前沿老化——把單一用戶池升級為競爭市場。
