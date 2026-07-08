# Tokensmith 06 — 推理算力池 · 服務品質 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 加入推理算力池：活躍用戶產生推理負載、消耗推理算力（可租）；推理容量不足時服務品質下降 → 額外用戶流失。把產出端（用戶）與投入端（算力）咬合。

**Architecture:** `Compute` 新增 `InferenceCapacity`（可租）與 `InferenceLoad`（每 tick 計算）。新增 `RentInferenceCompute` 指令與推理租金。`Tick` 新增 `advanceServing`：算總推理負載，容量不足（deficit）則對各模型施加服務流失。**v0 簡化**：`InferenceCapacity <= 0` 時給 grace（不施 churn，視為尚未自建/預設託管），使既有 Plan 03–05 測試不變；此簡化列為待調項。租 vs 自建的「自建」（晶片/服務器/機房）留 Plan 07。相依 Plan 01–05。

**Tech Stack:** Go 1.22+、標準 `testing` + `math`。無外部依賴。

## Global Constraints

- 延續 Plan 01–05 全部約束。`internal/sim` 純；不 mutate 輸入、clone slice。
- 服務流失：`load = Σ(online 模型 Users) · InferenceLoadPerUser`；`InferenceCapacity > 0` 且 `load > capacity` 時 `deficit = (load-capacity)/load`，每模型 `Users -= Users · ServiceChurnRate · deficit · dt`。`InferenceCapacity <= 0` 時不施 churn（v0 grace）。
- v0 數值：`InferenceRentPerGPUSec=0.006`（< 訓練 0.01）、`InferenceLoadPerUser=0.0001`（1 萬用戶 ≈ 1 GPU 負載）、`ServiceChurnRate=0.01`。

---

### Task 1: `model` 擴充——推理算力欄位 + RentInferenceCompute

**Files:**
- Modify: `internal/model/types.go`
- Test: `internal/model/types_test.go`

**Interfaces:**
- Produces:
  - `Compute` 新欄位 `InferenceCapacity float64`、`InferenceLoad float64`
  - `model.RentInferenceCompute{ Delta float64 }`，實作 `commandMarker()`

- [ ] **Step 1: 寫失敗測試**

在 `internal/model/types_test.go` 末尾新增：
```go
func TestInferenceComputeAndCommand(t *testing.T) {
	var s GameState
	s.Compute.InferenceCapacity = 4
	s.Compute.InferenceLoad = 1.5
	if s.Compute.InferenceCapacity != 4 || s.Compute.InferenceLoad != 1.5 {
		t.Fatalf("inference compute fields wrong: %+v", s.Compute)
	}
	var c Command = RentInferenceCompute{Delta: 2}
	if _, ok := c.(RentInferenceCompute); !ok {
		t.Fatalf("RentInferenceCompute not a Command")
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/model/`
Expected: FAIL（`InferenceCapacity` 欄位不存在 / `undefined: RentInferenceCompute`）。

- [ ] **Step 3: 寫最小實作**

在 `internal/model/types.go` 的 `Compute` 結構加入欄位：
```go
	InferenceCapacity float64 // rented inference GPUs
	InferenceLoad     float64 // current inference load (computed each tick)
```

在 `RentTrainingCompute` 之後新增：
```go
// RentInferenceCompute adjusts rented inference capacity by Delta.
type RentInferenceCompute struct {
	Delta float64
}

func (RentInferenceCompute) commandMarker() {}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/model/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/model/types.go internal/model/types_test.go
git commit -m "feat(model): add inference compute fields and rent command"
```

---

### Task 2: `balance` — 推理租金 / 負載 / 服務流失數值

**Files:**
- Modify: `internal/balance/balance.go`
- Test: `internal/balance/balance_test.go`

**Interfaces:**
- Produces: `Config` 新欄位 `InferenceRentPerGPUSec`, `InferenceLoadPerUser`, `ServiceChurnRate` + `Default()` 填值。

- [ ] **Step 1: 寫失敗測試**

在 `internal/balance/balance_test.go` 末尾新增：
```go
func TestDefaultInferenceValues(t *testing.T) {
	c := Default()
	if c.InferenceRentPerGPUSec != 0.006 {
		t.Errorf("InferenceRentPerGPUSec = %v, want 0.006", c.InferenceRentPerGPUSec)
	}
	if c.InferenceLoadPerUser != 0.0001 {
		t.Errorf("InferenceLoadPerUser = %v, want 0.0001", c.InferenceLoadPerUser)
	}
	if c.ServiceChurnRate != 0.01 {
		t.Errorf("ServiceChurnRate = %v, want 0.01", c.ServiceChurnRate)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/balance/`
Expected: FAIL（欄位不存在）。

- [ ] **Step 3: 寫最小實作**

在 `internal/balance/balance.go` 的 `Config` 結構末尾加入欄位：
```go
	// Inference serving (plan-06).
	InferenceRentPerGPUSec float64 // cash per rented inference GPU per second
	InferenceLoadPerUser   float64 // inference GPU load per active user
	ServiceChurnRate       float64 // extra churn per second at full deficit
```

在 `Default()` 的 `return c` 之前加入：
```go
	c.InferenceRentPerGPUSec = 0.006
	c.InferenceLoadPerUser = 0.0001
	c.ServiceChurnRate = 0.01
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/balance/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/balance/balance.go internal/balance/balance_test.go
git commit -m "feat(balance): add inference rent, load, service churn values"
```

---

### Task 3: `sim.Apply` — RentInferenceCompute

**Files:**
- Modify: `internal/sim/apply.go`
- Test: `internal/sim/apply_test.go`

**Interfaces:**
- Consumes: `model.RentInferenceCompute`。
- Produces: `Apply` 新增處理 `model.RentInferenceCompute`：`InferenceCapacity += Delta`，夾 `>= 0`。

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/apply_test.go` 末尾新增：
```go
func TestApplyRentInferenceCompute(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Compute.InferenceCapacity = 2
	ns, err := Apply(s, model.RentInferenceCompute{Delta: 3}, b)
	if err != nil || ns.Compute.InferenceCapacity != 5 {
		t.Fatalf("capacity = %v err=%v, want 5", ns.Compute.InferenceCapacity, err)
	}
	ns2, _ := Apply(s, model.RentInferenceCompute{Delta: -10}, b)
	if ns2.Compute.InferenceCapacity != 0 {
		t.Fatalf("should floor at 0, got %v", ns2.Compute.InferenceCapacity)
	}
	if s.Compute.InferenceCapacity != 2 {
		t.Fatalf("Apply mutated input")
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run TestApplyRentInference`
Expected: FAIL（未處理該指令 → `ErrUnknownCommand`）。

- [ ] **Step 3: 寫最小實作**

在 `internal/sim/apply.go` 的 `Apply` switch 內、`SetPrice` case 之後新增：
```go
	case model.RentInferenceCompute:
		return applyRentInferenceCompute(s, c), nil
```

新增函式：
```go
func applyRentInferenceCompute(s model.GameState, c model.RentInferenceCompute) model.GameState {
	ns := s
	ns.Compute.InferenceCapacity += c.Delta
	if ns.Compute.InferenceCapacity < 0 {
		ns.Compute.InferenceCapacity = 0
	}
	return ns
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/ -run TestApply`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/sim/apply.go internal/sim/apply_test.go
git commit -m "feat(sim): add RentInferenceCompute command"
```

---

### Task 4: `Tick` — 扣推理租金

**Files:**
- Modify: `internal/sim/sim.go`
- Test: `internal/sim/sim_test.go`

**Interfaces:**
- Consumes: `balance.InferenceRentPerGPUSec`、`Compute.InferenceCapacity`。
- Produces: `Tick` 於訓練租金之後扣推理租金 `Cash -= InferenceCapacity · InferenceRentPerGPUSec · dt`。

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/sim_test.go` 末尾新增：
```go
func TestTickDeductsInferenceRent(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Compute.InferenceCapacity = 5
	s.Resources.Cash = 100
	ns := Tick(s, 10, nil, b) // 5 * 0.006 * 10 = 0.3
	want := 100 - 5*b.InferenceRentPerGPUSec*10
	if !approx(ns.Resources.Cash, want) {
		t.Fatalf("Cash = %v, want %v", ns.Resources.Cash, want)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run TestTickDeductsInferenceRent`
Expected: FAIL（Cash 未扣推理租金）。

- [ ] **Step 3: 寫最小實作**

在 `internal/sim/sim.go` 的 `Tick` 內，緊接訓練租金那行（`... TrainRentPerGPUSec * dt`）之後加入：
```go
	ns.Resources.Cash -= ns.Compute.InferenceCapacity * b.InferenceRentPerGPUSec * dt
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/`
Expected: PASS（全部）。

- [ ] **Step 5: Commit**

```bash
git add internal/sim/sim.go internal/sim/sim_test.go
git commit -m "feat(sim): deduct inference compute rent per tick"
```

---

### Task 5: `Tick` — 推理負載與服務品質流失

**Files:**
- Modify: `internal/sim/sim.go`
- Test: `internal/sim/sim_test.go`

**Interfaces:**
- Consumes: `balance.InferenceLoadPerUser` / `ServiceChurnRate`；`Compute.InferenceCapacity`。
- Produces: `sim.advanceServing(ns, dt, b) model.GameState`（算 `load`、寫入 `Compute.InferenceLoad`；`InferenceCapacity > 0` 且 `load > capacity` 時對各 online 模型施服務流失，clone Models）；`Tick` 於 `advanceUsers` 後呼叫。**v0 grace**：容量 <=0 不施 churn（既有測試不變）。

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/sim_test.go` 末尾新增：
```go
func TestTickRecordsInferenceLoad(t *testing.T) {
	b := balance.Default()
	m := onlineModel(50, b.RefPrice)
	m.Users = 5000
	s := model.GameState{Models: []model.Model{m}}
	s.Compute.InferenceCapacity = 1e9 // plenty → no churn
	ns := Tick(s, 1, nil, b)
	want := ns.Models[0].Users * b.InferenceLoadPerUser
	if !approx(ns.Compute.InferenceLoad, want) {
		t.Fatalf("InferenceLoad = %v, want %v", ns.Compute.InferenceLoad, want)
	}
}

func TestTickInferenceOverloadChurns(t *testing.T) {
	b := balance.Default()
	m := onlineModel(50, b.RefPrice)
	m.Users = 100000 // load = 100000*0.0001 = 10
	low := model.GameState{Models: []model.Model{m}}
	low.Compute.InferenceCapacity = 1 // overloaded (10 > 1)
	high := model.GameState{Models: []model.Model{m}}
	high.Compute.InferenceCapacity = 1e9 // served
	nl := Tick(low, 1, nil, b)
	nh := Tick(high, 1, nil, b)
	if nl.Models[0].Users >= nh.Models[0].Users {
		t.Fatalf("overloaded users (%v) should be < served users (%v)",
			nl.Models[0].Users, nh.Models[0].Users)
	}
}

func TestTickZeroCapacityGrace(t *testing.T) {
	b := balance.Default()
	m := onlineModel(50, b.RefPrice)
	m.Users = 100000
	s := model.GameState{Models: []model.Model{m}} // InferenceCapacity 0
	served := model.GameState{Models: []model.Model{m}}
	served.Compute.InferenceCapacity = 1e9
	ns := Tick(s, 1, nil, b)
	nserved := Tick(served, 1, nil, b)
	// v0 grace: zero capacity behaves like fully served (no service churn)
	if !approx(ns.Models[0].Users, nserved.Models[0].Users) {
		t.Fatalf("zero-capacity grace: %v should equal served %v",
			ns.Models[0].Users, nserved.Models[0].Users)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run 'TestTickRecordsInferenceLoad|TestTickInferenceOverloadChurns'`
Expected: FAIL（負載未記錄 / 過載未流失）。

- [ ] **Step 3: 寫最小實作**

在 `internal/sim/sim.go` 的 `Tick` 內，把 `ns = advanceUsers(ns, dt, b)` 之後改為：
```go
	ns = advanceUsers(ns, dt, b)
	ns = advanceServing(ns, dt, b)
	return ns
```

新增函式：
```go
// advanceServing computes inference load and, when provisioned inference
// capacity cannot meet it, churns users by the service deficit. Pure: clones
// Models. v0: zero capacity is graced (no churn) so pre-inference behavior is
// unchanged.
func advanceServing(ns model.GameState, dt float64, b balance.Config) model.GameState {
	load := 0.0
	for _, m := range ns.Models {
		if m.Online {
			load += m.Users * b.InferenceLoadPerUser
		}
	}
	ns.Compute.InferenceLoad = load
	if ns.Compute.InferenceCapacity <= 0 || load <= ns.Compute.InferenceCapacity {
		return ns
	}
	deficit := (load - ns.Compute.InferenceCapacity) / load
	models := append([]model.Model(nil), ns.Models...)
	for i := range models {
		m := &models[i]
		if !m.Online {
			continue
		}
		m.Users -= m.Users * b.ServiceChurnRate * deficit * dt
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
Expected: PASS（全部——既有測試因 `InferenceCapacity` 預設 0 走 grace，用戶不變）。

- [ ] **Step 5: 跑整包 + vet + commit**

Run:
```bash
go test ./...
go vet ./...
```
Expected: 全部 PASS、vet 無警告。

```bash
git add internal/sim/sim.go internal/sim/sim_test.go
git commit -m "feat(sim): inference load and service-quality churn"
```

---

## 完成後狀態

推理服務側成立：用戶產生推理負載、消耗推理算力（租）；容量不足 → 服務品質下降 → 用戶流失。「擴用戶必須同步擴推理算力」的張力落地。v0 以 grace 處理零容量（列為待調：未來讓未自建/未租的用戶也有基本服務成本或流失）。仍純確定性。

**待調項（開放）**：`InferenceCapacity <= 0` 的 grace 造成「0 容量優於極小容量」的非單調；未來由「初始/onboarding 給起始推理容量」或「基本服務成本」修正。

**下一步（Plan 07）**：自建算力——晶片 → 服務器 → 機房（電力/空間容量），為訓練/推理兩池提供更便宜但需 capex 的容量，形成「租的彈性 vs 自建的規模經濟」完整張力。
