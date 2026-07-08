# Tokensmith 07 — 自建算力（晶片 · 服務器 · 機房） Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 加入自建算力供給側——買晶片組服務器、放進有電力/空間容量的機房；自建算力併入訓練/推理兩池的有效容量，形成「租的彈性 vs 自建的規模經濟」張力。

**Architecture:** 新增 `model.Chip`（目錄）、`model.Server`（自建單位）、`model.Datacenter`（單一機房：電力+空間容量）與 `GameState.Servers` / `Datacenter`。新增 `BuildServer` / `ExpandDatacenter` 指令。**有效容量 = 租的 + 自建**：`advanceTraining`/`advanceServing` 改用 `effectiveTraining`/`effectiveInference`（無服務器時 = 租的 → Plan 02–06 測試不變）。`Tick` 依服務器功耗扣電費。相依 Plan 01–06。

**Tech Stack:** Go 1.22+、標準 `testing` + `math`。無外部依賴。

## Global Constraints

- 延續 Plan 01–06。`internal/sim` 純；不 mutate 輸入、clone slice。
- 單一機房模型（`GameState.Datacenter` 提供 `PowerCapacity` / `SlotCapacity`）。服務器全域 `GameState.Servers`；已用電力/空間 = Σ 服務器。
- 一台服務器 = `ChipsPerServer`(=8) 顆同型晶片，佔 1 機架空間；`Compute=晶片算力×8`、`PowerKW=晶片功耗×8`；capex = `晶片價×8 + ChassisCost`。
- 有效容量：`effectiveTraining = 租訓練 + Σ(訓練型服務器 Compute)`；`effectiveInference` 同理。無服務器時等於租的。
- v0 數值（spec §12）：晶片 `H-class G3`(推理 算力2 功耗3 $8k)、`T-class G4`(訓練 算力3 功耗5 $18k)；`ChipsPerServer=8`、`ChassisCost=5000`、`ElectricityPerKWSec=0.001`（v0 placeholder）、`PowerCostPerKW=400`、`SlotCost=30000`。

---

### Task 1: `model` 擴充——Chip / Server / Datacenter / 指令

**Files:**
- Modify: `internal/model/types.go`
- Test: `internal/model/types_test.go`

**Interfaces:**
- Produces:
  - `model.ComputePool`（int）+ `PoolTraining=0, PoolInference=1`
  - `model.Chip{ Name string; Pool ComputePool; Compute, PowerKW, Price float64 }`
  - `model.Server{ Pool ComputePool; Compute, PowerKW, Slots float64 }`
  - `model.Datacenter{ PowerCapacity, SlotCapacity float64 }`
  - `GameState` 新欄位 `Servers []Server`、`Datacenter Datacenter`
  - `model.BuildServer{ ChipName string }`、`model.ExpandDatacenter{ PowerDelta, SlotDelta float64 }`，各實作 `commandMarker()`

- [ ] **Step 1: 寫失敗測試**

在 `internal/model/types_test.go` 末尾新增：
```go
func TestComputeInfraTypes(t *testing.T) {
	if PoolTraining != 0 || PoolInference != 1 {
		t.Fatalf("pool consts wrong")
	}
	ch := Chip{Name: "T", Pool: PoolTraining, Compute: 3, PowerKW: 5, Price: 18000}
	sv := Server{Pool: ch.Pool, Compute: 24, PowerKW: 40, Slots: 1}
	var s GameState
	s.Servers = append(s.Servers, sv)
	s.Datacenter = Datacenter{PowerCapacity: 800, SlotCapacity: 20}
	if len(s.Servers) != 1 || s.Datacenter.PowerCapacity != 800 {
		t.Fatalf("infra fields wrong: %+v", s)
	}
	var c1 Command = BuildServer{ChipName: "T"}
	var c2 Command = ExpandDatacenter{PowerDelta: 100, SlotDelta: 5}
	if _, ok := c1.(BuildServer); !ok {
		t.Fatalf("BuildServer not a Command")
	}
	if _, ok := c2.(ExpandDatacenter); !ok {
		t.Fatalf("ExpandDatacenter not a Command")
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/model/`
Expected: FAIL（`undefined: PoolTraining` / `Chip` / `Server` 等）。

- [ ] **Step 3: 寫最小實作**

在 `internal/model/types.go` 末尾新增：
```go
// ComputePool identifies which compute pool a chip/server feeds.
type ComputePool int

const (
	PoolTraining  ComputePool = iota // 0
	PoolInference                    // 1
)

// Chip is a catalog entry; owned compute is held as Servers.
type Chip struct {
	Name    string
	Pool    ComputePool
	Compute float64 // compute per chip
	PowerKW float64 // power draw per chip
	Price   float64 // price per chip
}

// Server is self-built compute: a bundle of chips feeding one pool.
type Server struct {
	Pool    ComputePool
	Compute float64 // total compute contributed
	PowerKW float64 // total power draw
	Slots   float64 // rack slots occupied
}

// Datacenter provides power and rack-space capacity limits (single-DC v0).
type Datacenter struct {
	PowerCapacity float64
	SlotCapacity  float64
}

// BuildServer builds one server from the named chip in the datacenter.
type BuildServer struct {
	ChipName string
}

func (BuildServer) commandMarker() {}

// ExpandDatacenter adds power / rack-space capacity for capex.
type ExpandDatacenter struct {
	PowerDelta float64
	SlotDelta  float64
}

func (ExpandDatacenter) commandMarker() {}
```

在 `GameState` 結構加入欄位（放在 `Competitors` 之後）：
```go
	Servers    []Server
	Datacenter Datacenter
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/model/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/model/types.go internal/model/types_test.go
git commit -m "feat(model): add chip/server/datacenter types and commands"
```

---

### Task 2: `balance` — 晶片目錄與自建數值

**Files:**
- Modify: `internal/balance/balance.go`
- Test: `internal/balance/balance_test.go`

**Interfaces:**
- Produces: `Config` 新欄位 + `Default()` 填值：
  - `Chips []model.Chip`
  - `ChipsPerServer int`
  - `ChassisCost, ElectricityPerKWSec, PowerCostPerKW, SlotCost float64`

- [ ] **Step 1: 寫失敗測試**

在 `internal/balance/balance_test.go` 末尾新增：
```go
func TestDefaultChipsAndInfra(t *testing.T) {
	c := Default()
	if len(c.Chips) != 2 {
		t.Fatalf("chips = %d, want 2", len(c.Chips))
	}
	if c.Chips[0].Name != "H-class G3" || c.Chips[0].Pool != model.PoolInference {
		t.Errorf("first chip wrong: %+v", c.Chips[0])
	}
	if c.Chips[1].Pool != model.PoolTraining || c.Chips[1].Price != 18000 {
		t.Errorf("second chip wrong: %+v", c.Chips[1])
	}
	if c.ChipsPerServer != 8 || c.ChassisCost != 5000 {
		t.Errorf("server params wrong: %+v", c)
	}
	if c.ElectricityPerKWSec != 0.001 || c.PowerCostPerKW != 400 || c.SlotCost != 30000 {
		t.Errorf("infra costs wrong: %+v", c)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/balance/`
Expected: FAIL（欄位不存在）。

- [ ] **Step 3: 寫最小實作**

在 `internal/balance/balance.go` 的 `Config` 結構末尾加入欄位：
```go
	// Self-build compute (plan-07).
	Chips               []model.Chip
	ChipsPerServer      int
	ChassisCost         float64
	ElectricityPerKWSec float64 // cash per kW per second
	PowerCostPerKW      float64 // datacenter power-capacity expansion cost per kW
	SlotCost            float64 // datacenter rack-slot expansion cost per slot
```

在 `Default()` 的 `return c` 之前加入：
```go
	c.Chips = []model.Chip{
		{Name: "H-class G3", Pool: model.PoolInference, Compute: 2, PowerKW: 3, Price: 8000},
		{Name: "T-class G4", Pool: model.PoolTraining, Compute: 3, PowerKW: 5, Price: 18000},
	}
	c.ChipsPerServer = 8
	c.ChassisCost = 5000
	c.ElectricityPerKWSec = 0.001
	c.PowerCostPerKW = 400
	c.SlotCost = 30000
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/balance/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/balance/balance.go internal/balance/balance_test.go
git commit -m "feat(balance): add chip catalog and self-build costs"
```

---

### Task 3: `sim.Apply` — ExpandDatacenter

**Files:**
- Modify: `internal/sim/apply.go`
- Test: `internal/sim/apply_test.go`

**Interfaces:**
- Consumes: `model.ExpandDatacenter`；`balance.PowerCostPerKW` / `SlotCost`。
- Produces:
  - package var `ErrInsufficientCash`（`error`）
  - `Apply` 新增處理 `model.ExpandDatacenter`：負 delta 夾為 0；`cost = power·PowerCostPerKW + slots·SlotCost`；現金不足回 `ErrInsufficientCash`；否則扣現金、加容量。

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/apply_test.go` 末尾新增：
```go
func TestApplyExpandDatacenter(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Resources.Cash = 1_000_000
	ns, err := Apply(s, model.ExpandDatacenter{PowerDelta: 800, SlotDelta: 20}, b)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ns.Datacenter.PowerCapacity != 800 || ns.Datacenter.SlotCapacity != 20 {
		t.Errorf("capacity wrong: %+v", ns.Datacenter)
	}
	wantCost := 800*b.PowerCostPerKW + 20*b.SlotCost
	if !approx(ns.Resources.Cash, 1_000_000-wantCost) {
		t.Errorf("cash = %v, want %v", ns.Resources.Cash, 1_000_000-wantCost)
	}
	if s.Datacenter.PowerCapacity != 0 {
		t.Errorf("Apply mutated input")
	}
}

func TestApplyExpandDatacenterInsufficientCash(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Resources.Cash = 100
	if _, err := Apply(s, model.ExpandDatacenter{PowerDelta: 800, SlotDelta: 20}, b); err != ErrInsufficientCash {
		t.Fatalf("err = %v, want ErrInsufficientCash", err)
	}
}
```

> 註：`approx`（float 容差比較）已於 `sim_test.go` 定義，同屬 `package sim` 的 `apply_test.go` 可直接用，無需另立 helper。

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run TestApplyExpandDatacenter`
Expected: FAIL（`undefined: ErrInsufficientCash`）。

- [ ] **Step 3: 寫最小實作**

在 `internal/sim/apply.go` 的 error var 區塊加入：
```go
	ErrInsufficientCash = errors.New("sim: insufficient cash")
```

在 `Apply` 的 switch 內、`RentInferenceCompute` case 之後新增：
```go
	case model.ExpandDatacenter:
		return applyExpandDatacenter(s, c, b)
```

新增函式：
```go
func applyExpandDatacenter(s model.GameState, c model.ExpandDatacenter, b balance.Config) (model.GameState, error) {
	power := c.PowerDelta
	if power < 0 {
		power = 0
	}
	slots := c.SlotDelta
	if slots < 0 {
		slots = 0
	}
	cost := power*b.PowerCostPerKW + slots*b.SlotCost
	if s.Resources.Cash < cost {
		return s, ErrInsufficientCash
	}
	ns := s
	ns.Resources.Cash -= cost
	ns.Datacenter.PowerCapacity += power
	ns.Datacenter.SlotCapacity += slots
	return ns, nil
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/ -run TestApply`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/sim/apply.go internal/sim/apply_test.go
git commit -m "feat(sim): add ExpandDatacenter command"
```

---

### Task 4: `sim.Apply` — BuildServer

**Files:**
- Modify: `internal/sim/apply.go`
- Test: `internal/sim/apply_test.go`

**Interfaces:**
- Consumes: `model.BuildServer`；`balance.Chips` / `ChipsPerServer` / `ChassisCost`。
- Produces:
  - package vars `ErrInvalidChip`, `ErrInsufficientPower`, `ErrInsufficientSpace`
  - `sim.findChip(chips []model.Chip, name string) (model.Chip, bool)`
  - `Apply` 新增處理 `model.BuildServer`：找晶片；組服務器（8 晶片）；驗證機房剩餘電力/空間、現金；通過則扣現金、append 服務器（clone）。

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/apply_test.go` 末尾新增：
```go
func dcState(cash, power, slots float64) model.GameState {
	s := model.GameState{}
	s.Resources.Cash = cash
	s.Datacenter = model.Datacenter{PowerCapacity: power, SlotCapacity: slots}
	return s
}

func TestApplyBuildServerSuccess(t *testing.T) {
	b := balance.Default()
	s := dcState(1_000_000, 800, 20)
	ns, err := Apply(s, model.BuildServer{ChipName: "T-class G4"}, b)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(ns.Servers) != 1 {
		t.Fatalf("servers = %d, want 1", len(ns.Servers))
	}
	sv := ns.Servers[0]
	if sv.Pool != model.PoolTraining || sv.Compute != 24 || sv.PowerKW != 40 || sv.Slots != 1 {
		t.Errorf("server wrong: %+v", sv) // 3*8, 5*8
	}
	wantCapex := 18000*8 + b.ChassisCost
	if !approx(ns.Resources.Cash, 1_000_000-wantCapex) {
		t.Errorf("cash = %v, want %v", ns.Resources.Cash, 1_000_000-wantCapex)
	}
	if len(s.Servers) != 0 {
		t.Errorf("Apply mutated input Servers")
	}
}

func TestApplyBuildServerErrors(t *testing.T) {
	b := balance.Default()
	// unknown chip
	if _, err := Apply(dcState(1e9, 1e9, 1e9), model.BuildServer{ChipName: "nope"}, b); err != ErrInvalidChip {
		t.Errorf("chip: err = %v, want ErrInvalidChip", err)
	}
	// insufficient cash
	if _, err := Apply(dcState(100, 1e9, 1e9), model.BuildServer{ChipName: "T-class G4"}, b); err != ErrInsufficientCash {
		t.Errorf("cash: err = %v, want ErrInsufficientCash", err)
	}
	// insufficient power (T server draws 40kW; capacity 10)
	if _, err := Apply(dcState(1e9, 10, 1e9), model.BuildServer{ChipName: "T-class G4"}, b); err != ErrInsufficientPower {
		t.Errorf("power: err = %v, want ErrInsufficientPower", err)
	}
	// insufficient space (slots 0)
	if _, err := Apply(dcState(1e9, 1e9, 0), model.BuildServer{ChipName: "T-class G4"}, b); err != ErrInsufficientSpace {
		t.Errorf("space: err = %v, want ErrInsufficientSpace", err)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run TestApplyBuildServer`
Expected: FAIL（`undefined: ErrInvalidChip` 等）。

- [ ] **Step 3: 寫最小實作**

在 `internal/sim/apply.go` 的 error var 區塊加入：
```go
	ErrInvalidChip       = errors.New("sim: unknown chip")
	ErrInsufficientPower = errors.New("sim: datacenter power capacity exceeded")
	ErrInsufficientSpace = errors.New("sim: datacenter rack space exceeded")
```

在 `Apply` 的 switch 內、`ExpandDatacenter` case 之後新增：
```go
	case model.BuildServer:
		return applyBuildServer(s, c, b)
```

新增函式：
```go
func findChip(chips []model.Chip, name string) (model.Chip, bool) {
	for _, ch := range chips {
		if ch.Name == name {
			return ch, true
		}
	}
	return model.Chip{}, false
}

func applyBuildServer(s model.GameState, c model.BuildServer, b balance.Config) (model.GameState, error) {
	chip, ok := findChip(b.Chips, c.ChipName)
	if !ok {
		return s, ErrInvalidChip
	}
	n := float64(b.ChipsPerServer)
	server := model.Server{
		Pool:    chip.Pool,
		Compute: chip.Compute * n,
		PowerKW: chip.PowerKW * n,
		Slots:   1,
	}
	capex := chip.Price*n + b.ChassisCost
	if s.Resources.Cash < capex {
		return s, ErrInsufficientCash
	}
	usedPower, usedSlots := 0.0, 0.0
	for _, sv := range s.Servers {
		usedPower += sv.PowerKW
		usedSlots += sv.Slots
	}
	if usedPower+server.PowerKW > s.Datacenter.PowerCapacity {
		return s, ErrInsufficientPower
	}
	if usedSlots+server.Slots > s.Datacenter.SlotCapacity {
		return s, ErrInsufficientSpace
	}
	ns := s
	ns.Resources.Cash -= capex
	ns.Servers = append(append([]model.Server(nil), s.Servers...), server)
	return ns, nil
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/ -run TestApply`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/sim/apply.go internal/sim/apply_test.go
git commit -m "feat(sim): add BuildServer command with capacity checks"
```

---

### Task 5: `Tick` — 有效容量併入兩池 + 電費

**Files:**
- Modify: `internal/sim/sim.go`
- Test: `internal/sim/sim_test.go`

**Interfaces:**
- Consumes: `GameState.Servers`；`balance.ElectricityPerKWSec`。
- Produces:
  - `sim.effectiveTraining(ns) float64`、`sim.effectiveInference(ns) float64`（租的 + 對應池服務器 Compute）
  - `advanceTraining` 改用 `effectiveTraining(ns)`；`advanceServing` 改用 `effectiveInference(ns)`（含 grace / deficit 判斷）
  - `Tick` 依 `Σ 服務器 PowerKW · ElectricityPerKWSec · dt` 扣電費（緊接推理租金之後）

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/sim_test.go` 末尾新增：
```go
func TestEffectiveTrainingIncludesServers(t *testing.T) {
	b := balance.Default()
	s := model.GameState{HasTraining: true}
	s.Compute.TrainingCapacity = 0 // no rented
	s.Servers = []model.Server{{Pool: model.PoolTraining, Compute: 10}} // PowerKW 0 → no electricity
	s.Training = model.TrainingJob{Gen: 1, WorkRemaining: 100}
	ns := Tick(s, 1, nil, b) // effective training 10 → work -= 10 → 90
	if !approx(ns.Training.WorkRemaining, 90) {
		t.Fatalf("WorkRemaining = %v, want 90 (self-built training compute)", ns.Training.WorkRemaining)
	}
}

func TestSelfBuiltInferenceCapacityServes(t *testing.T) {
	b := balance.Default()
	m := onlineModel(50, b.RefPrice)
	m.Users = 100000 // load = 10
	low := model.GameState{Models: []model.Model{m}, Servers: []model.Server{{Pool: model.PoolInference, Compute: 1}}}
	high := model.GameState{Models: []model.Model{m}, Servers: []model.Server{{Pool: model.PoolInference, Compute: 1e9}}}
	nl := Tick(low, 1, nil, b)
	nh := Tick(high, 1, nil, b)
	if nl.Models[0].Users >= nh.Models[0].Users {
		t.Fatalf("overloaded self-built (%v) should be < served (%v)", nl.Models[0].Users, nh.Models[0].Users)
	}
}

func TestTickDeductsElectricity(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Servers = []model.Server{{Pool: model.PoolTraining, Compute: 24, PowerKW: 40}}
	s.Resources.Cash = 1000
	ns := Tick(s, 10, nil, b) // 40 * 0.001 * 10 = 0.4
	want := 1000 - 40*b.ElectricityPerKWSec*10
	if !approx(ns.Resources.Cash, want) {
		t.Fatalf("Cash = %v, want %v", ns.Resources.Cash, want)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run 'TestEffectiveTraining|TestSelfBuiltInference|TestTickDeductsElectricity'`
Expected: FAIL（自建算力未生效 / 電費未扣）。

- [ ] **Step 3: 寫最小實作**

在 `internal/sim/sim.go` 新增有效容量 helper（放在 `advanceTraining` 之前）：
```go
// effectiveTraining is rented plus self-built training compute.
func effectiveTraining(ns model.GameState) float64 {
	c := ns.Compute.TrainingCapacity
	for _, sv := range ns.Servers {
		if sv.Pool == model.PoolTraining {
			c += sv.Compute
		}
	}
	return c
}

// effectiveInference is rented plus self-built inference compute.
func effectiveInference(ns model.GameState) float64 {
	c := ns.Compute.InferenceCapacity
	for _, sv := range ns.Servers {
		if sv.Pool == model.PoolInference {
			c += sv.Compute
		}
	}
	return c
}
```

把 `advanceTraining` 的推進行改為用有效容量：
```go
	ns.Training.WorkRemaining -= effectiveTraining(ns) * dt
```

把 `advanceServing` 改成用有效推理容量（`capacity` 取代舊的 `ns.Compute.InferenceCapacity`）：
```go
func advanceServing(ns model.GameState, dt float64, b balance.Config) model.GameState {
	load := 0.0
	for _, m := range ns.Models {
		if m.Online {
			load += m.Users * b.InferenceLoadPerUser
		}
	}
	ns.Compute.InferenceLoad = load
	capacity := effectiveInference(ns)
	if capacity <= 0 || load <= capacity {
		return ns
	}
	deficit := (load - capacity) / load
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

在 `Tick` 內，緊接推理租金那行之後加入電費：
```go
	serverPower := 0.0
	for _, sv := range ns.Servers {
		serverPower += sv.PowerKW
	}
	ns.Resources.Cash -= serverPower * b.ElectricityPerKWSec * dt
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/`
Expected: PASS（全部——既有測試無服務器 → 有效容量 = 租的、電費 0 → 不變）。

- [ ] **Step 5: 跑整包 + vet + commit**

Run:
```bash
go test ./...
go vet ./...
```
Expected: 全部 PASS、vet 無警告。

```bash
git add internal/sim/sim.go internal/sim/sim_test.go
git commit -m "feat(sim): fold self-built compute into pools and add electricity"
```

---

## 完成後狀態

自建供給側成立：`ExpandDatacenter` 擴機房容量（電力/空間）→ `BuildServer` 買晶片組服務器（受容量約束）→ 自建算力併入訓練/推理有效容量、每 tick 扣電費。「租的彈性（無 capex、貴）vs 自建的規模經濟（capex + 電費、單位便宜）」完整張力落地。無服務器時退化為 Plan 02–06 行為（測試不變）。仍純確定性。

**下一步（Plan 08）**：團隊（四職能聚合 + 明星員工）與科技樹四分支——把研發人力來源、基建效率加成、定位加成補進來。

**v2 接點**：此晶片目錄（`balance.Chips`）就是 v2「自己設計晶片」的接點——屆時玩家設計的晶片變成屬性由你決定的新目錄條目。
