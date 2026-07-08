# Tokensmith 02 — 訓練管線 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Plan 01 的純 Sim 上加入訓練管線：玩家指令（租訓練算力、開始訓練）、以算力推進訓練、完成後模型上線、每 tick 扣訓練租金。

**Architecture:** 新增純函式 `sim.Apply(state, cmd, cfg) (state, error)` 處理玩家指令（驗證 + 套用），`Tick` 續管時間推進。`GameState` 擴充 `Compute`（訓練池）、`Models`、單一進行中 `Training` 工作。本 plan 的簡化：品質四維儲存但不接科技樹加成 / 世界上限係數；單一使用者池、無用戶成長 / 無營收（Plan 03）；單一進行中訓練（並行訓練屬明星絕活，後續）。相依 Plan 01。

**Tech Stack:** Go 1.22+、標準 `testing` + `errors`。無外部依賴。延續 Plan 01 的 `model` / `balance` / `sim`。

## Global Constraints

- 延續 Plan 01 全部約束：module `tokensmith`、Go 1.22+、`internal/sim` 純函式（不得 wall-clock / rand / 檔案 / 網路 I/O；`errors` 允許）、tier / gen / dim 索引資料用固定陣列不用 map、套件相依 `model ← balance ← sim`。
- `Apply` 與 `Tick` MUST 不得 mutate 輸入 `s`；含 slice（`Models`）時，修改前先 clone（`append([]T(nil), s.X...)`），避免共享底層陣列造成別名。
- 品質維度索引：`DimCapability=0, DimEfficiency=1, DimSafety=2, DimSpeed=3`，`NumQualityDims=4`。
- 世代範圍 1..`MaxGen`（=5）。gen 索引陣列大小 `MaxGen+1`，index 0 不用。
- v0 數值取自 spec §12（訓練工作量以 GPU·時 ×3600 換為 GPU·秒）；訓練租金為 v0 placeholder `0.01 $/GPU/秒`（spec 的 $500/GPU·day 因 game-day 定義未定，暫用整數值、日後調），註明於 config。

---

### Task 1: `model` 擴充——Model / 訓練 / 算力 / 指令型別

**Files:**
- Modify: `internal/model/types.go`
- Test: `internal/model/types_test.go`（新增測試）

**Interfaces:**
- Consumes: 既有 `model.GameState`, `model.Resources`（Plan 01）。
- Produces:
  - `model.QualityDim`（int）+ 常數 `DimCapability=0, DimEfficiency=1, DimSafety=2, DimSpeed=3`, `NumQualityDims=4`。
  - `model.Model{ Gen int; Quality [NumQualityDims]float64; Users, Price float64; Online bool }`
  - `model.TrainingJob{ Gen int; Alloc [NumQualityDims]float64; Price, WorkRemaining float64 }`
  - `model.Compute{ TrainingCapacity float64 }`
  - `GameState` 新欄位：`Compute Compute`、`Models []Model`、`HasTraining bool`、`Training TrainingJob`
  - `model.Command`（介面，方法 `commandMarker()`）；具體型別 `StartTraining{ Gen int; Alloc [NumQualityDims]float64; Price float64 }`、`RentTrainingCompute{ Delta float64 }`，各實作 `commandMarker()`。

- [ ] **Step 1: 寫失敗測試**

在 `internal/model/types_test.go` 末尾新增：
```go
func TestModelAndComputeFields(t *testing.T) {
	m := Model{Gen: 2, Online: true, Price: 12}
	m.Quality[DimCapability] = 40
	m.Quality[DimSpeed] = 30
	if m.Quality[DimCapability] != 40 || m.Quality[DimSpeed] != 30 {
		t.Fatalf("quality dims wrong: %+v", m.Quality)
	}
	if NumQualityDims != 4 {
		t.Fatalf("NumQualityDims = %d, want 4", NumQualityDims)
	}
	var s GameState
	s.Compute.TrainingCapacity = 4
	s.Models = append(s.Models, m)
	s.HasTraining = true
	s.Training = TrainingJob{Gen: 2, Price: 12, WorkRemaining: 7200}
	if s.Compute.TrainingCapacity != 4 || len(s.Models) != 1 || !s.HasTraining {
		t.Fatalf("gamestate extension wrong: %+v", s)
	}
}

func TestCommandsImplementInterface(t *testing.T) {
	var cmds []Command
	cmds = append(cmds, StartTraining{Gen: 1}, RentTrainingCompute{Delta: 2})
	if len(cmds) != 2 {
		t.Fatalf("commands not assignable to Command interface")
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/model/`
Expected: FAIL（`undefined: Model` / `DimCapability` / `Command` 等）。

- [ ] **Step 3: 寫最小實作**

在 `internal/model/types.go` 末尾新增：
```go
// QualityDim indexes Model.Quality.
type QualityDim int

const (
	DimCapability QualityDim = iota // 0 能力
	DimEfficiency                   // 1 成本效率
	DimSafety                       // 2 安全
	DimSpeed                        // 3 速度
	NumQualityDims = 4
)

// Model is a trained AI model.
type Model struct {
	Gen     int
	Quality [NumQualityDims]float64
	Users   float64
	Price   float64 // per user per month; player-set
	Online  bool
}

// TrainingJob is the single in-progress training (plan-02).
type TrainingJob struct {
	Gen           int
	Alloc         [NumQualityDims]float64 // budget fraction per dim; sums to ~1
	Price         float64
	WorkRemaining float64 // GPU-seconds of training work left
}

// Compute holds compute capacity (plan-02: training pool only).
type Compute struct {
	TrainingCapacity float64 // rented training GPUs
}

// Command is a validated player action applied via sim.Apply.
type Command interface{ commandMarker() }

// StartTraining begins training a new model of the given generation.
type StartTraining struct {
	Gen   int
	Alloc [NumQualityDims]float64
	Price float64
}

func (StartTraining) commandMarker() {}

// RentTrainingCompute adjusts rented training capacity by Delta (may be negative).
type RentTrainingCompute struct {
	Delta float64
}

func (RentTrainingCompute) commandMarker() {}
```

在 `GameState` 結構加入新欄位（放在 `WindowElapsed` 之後）：
```go
	Compute     Compute
	Models      []Model
	HasTraining bool
	Training    TrainingJob
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/model/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/model/types.go internal/model/types_test.go
git commit -m "feat(model): add Model, TrainingJob, Compute, Command types"
```

---

### Task 2: `balance` 擴充——世代 / 訓練 / 租金數值

**Files:**
- Modify: `internal/balance/balance.go`
- Test: `internal/balance/balance_test.go`

**Interfaces:**
- Consumes: `model`（Plan 01）。
- Produces: `Config` 新欄位（皆以 gen 為索引，大小 `MaxGen+1`）：
  - `MaxGen` 常數（=5，`balance` package 級）
  - `GenRnDCost [MaxGen+1]float64`
  - `GenTrainWorkGPUSec [MaxGen+1]float64`
  - `GenQualityCap [MaxGen+1]float64`
  - `TrainRentPerGPUSec float64`
  - `Default()` 填入 v0 值。

- [ ] **Step 1: 寫失敗測試**

在 `internal/balance/balance_test.go` 末尾新增：
```go
func TestDefaultGenAndTrainValues(t *testing.T) {
	c := Default()
	if MaxGen != 5 {
		t.Fatalf("MaxGen = %d, want 5", MaxGen)
	}
	if c.GenRnDCost[1] != 20000 || c.GenRnDCost[5] != 40000000 {
		t.Errorf("GenRnDCost wrong: %v", c.GenRnDCost)
	}
	if c.GenTrainWorkGPUSec[1] != 1800 { // 0.5 GPU·hr * 3600
		t.Errorf("GenTrainWorkGPUSec[1] = %v, want 1800", c.GenTrainWorkGPUSec[1])
	}
	if c.GenTrainWorkGPUSec[4] != 108000 { // 30 GPU·hr * 3600
		t.Errorf("GenTrainWorkGPUSec[4] = %v, want 108000", c.GenTrainWorkGPUSec[4])
	}
	if c.GenQualityCap[1] != 25 || c.GenQualityCap[5] != 100 {
		t.Errorf("GenQualityCap wrong: %v", c.GenQualityCap)
	}
	if c.TrainRentPerGPUSec != 0.01 {
		t.Errorf("TrainRentPerGPUSec = %v, want 0.01", c.TrainRentPerGPUSec)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/balance/`
Expected: FAIL（`undefined: MaxGen` / 欄位不存在）。

- [ ] **Step 3: 寫最小實作**

在 `internal/balance/balance.go`：新增常數與欄位。於檔案頂部 import 之後加入：
```go
// MaxGen is the highest model generation modelled in v0.
const MaxGen = 5
```

在 `Config` 結構末尾加入欄位：
```go
	// Per-generation model training (index by gen 1..MaxGen; 0 unused).
	GenRnDCost         [MaxGen + 1]float64 // R&D cost to start training
	GenTrainWorkGPUSec [MaxGen + 1]float64 // training work in GPU-seconds
	GenQualityCap      [MaxGen + 1]float64 // per-dimension quality ceiling

	// TrainRentPerGPUSec is cash cost per rented training GPU per second.
	// v0 placeholder (spec §12 $500/GPU·day is game-day-ambiguous); tune later.
	TrainRentPerGPUSec float64
```

在 `Default()` 的 `return c` 之前加入：
```go
	// gen:                      1        2         3          4           5
	c.GenRnDCost = [MaxGen + 1]float64{0, 20000, 150000, 1000000, 6000000, 40000000}
	c.GenTrainWorkGPUSec = [MaxGen + 1]float64{0, 1800, 7200, 28800, 108000, 432000}
	c.GenQualityCap = [MaxGen + 1]float64{0, 25, 45, 65, 82, 100}
	c.TrainRentPerGPUSec = 0.01
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/balance/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/balance/balance.go internal/balance/balance_test.go
git commit -m "feat(balance): add gen training and rent v0 values"
```

---

### Task 3: `sim.Apply` — RentTrainingCompute

**Files:**
- Create: `internal/sim/apply.go`
- Test: `internal/sim/apply_test.go`

**Interfaces:**
- Consumes: `model.GameState`, `model.Command`, `model.RentTrainingCompute`（Task 1）；`balance.Config`（Task 2）。
- Produces:
  - `sim.ErrUnknownCommand`（`error`，package var）
  - `sim.Apply(s model.GameState, cmd model.Command, b balance.Config) (model.GameState, error)`（本 task 只處理 `RentTrainingCompute`；其他型別回 `ErrUnknownCommand`，後續 task 補）
  - 行為：`TrainingCapacity += Delta`，並夾在 `>= 0`（負到 0）。

- [ ] **Step 1: 寫失敗測試**

Create `internal/sim/apply_test.go`:
```go
package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestApplyRentTrainingComputeAdds(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Compute.TrainingCapacity = 2
	ns, err := Apply(s, model.RentTrainingCompute{Delta: 3}, b)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ns.Compute.TrainingCapacity != 5 {
		t.Fatalf("capacity = %v, want 5", ns.Compute.TrainingCapacity)
	}
	// input not mutated
	if s.Compute.TrainingCapacity != 2 {
		t.Fatalf("Apply mutated input: %v", s.Compute.TrainingCapacity)
	}
}

func TestApplyRentTrainingComputeFloorsAtZero(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Compute.TrainingCapacity = 2
	ns, _ := Apply(s, model.RentTrainingCompute{Delta: -5}, b)
	if ns.Compute.TrainingCapacity != 0 {
		t.Fatalf("capacity = %v, want 0", ns.Compute.TrainingCapacity)
	}
}
```

> 註：`model.Command` 用未匯出 marker 方法（密封介面），外部套件無法自建 command 型別，故不從 `sim` 測試「未知指令」；`ErrUnknownCommand` 仍保留為 `Apply` 的防禦性 default。

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run TestApply`
Expected: FAIL（`undefined: Apply`）。

- [ ] **Step 3: 寫最小實作**

Create `internal/sim/apply.go`:
```go
package sim

import (
	"errors"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// ErrUnknownCommand is returned by Apply for an unrecognized command type.
var ErrUnknownCommand = errors.New("sim: unknown command")

// Apply validates and applies a single player command, returning the new
// state. Pure: it does not mutate s.
func Apply(s model.GameState, cmd model.Command, b balance.Config) (model.GameState, error) {
	switch c := cmd.(type) {
	case model.RentTrainingCompute:
		return applyRentTrainingCompute(s, c), nil
	default:
		return s, ErrUnknownCommand
	}
}

func applyRentTrainingCompute(s model.GameState, c model.RentTrainingCompute) model.GameState {
	ns := s
	ns.Compute.TrainingCapacity += c.Delta
	if ns.Compute.TrainingCapacity < 0 {
		ns.Compute.TrainingCapacity = 0
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
git commit -m "feat(sim): add Apply with RentTrainingCompute command"
```

---

### Task 4: `sim.Apply` — StartTraining（驗證 + 扣 R&D + 建工作）

**Files:**
- Modify: `internal/sim/apply.go`
- Test: `internal/sim/apply_test.go`

**Interfaces:**
- Consumes: 同 Task 3 + `model.StartTraining`、`balance` 的 `GenRnDCost` / `GenTrainWorkGPUSec` / `MaxGen`。
- Produces:
  - package vars：`ErrTrainingInProgress`, `ErrInsufficientRnD`, `ErrInvalidGen`, `ErrInvalidAlloc`（皆 `error`）
  - `Apply` 新增處理 `model.StartTraining`：驗證（無進行中訓練、gen 於 1..MaxGen、alloc 各 >=0 且總和 ≈ 1.0±0.001、R&D 足夠 `GenRnDCost[gen]`）→ 扣 R&D、設 `HasTraining=true` 與 `Training{Gen, Alloc, Price, WorkRemaining=GenTrainWorkGPUSec[gen]}`。

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/apply_test.go` 末尾新增：
```go
func validAlloc() [model.NumQualityDims]float64 {
	return [model.NumQualityDims]float64{0.4, 0.2, 0.2, 0.2}
}

func TestApplyStartTrainingSuccess(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Resources.RnD = 50000 // > Gen1 cost 20000
	cmd := model.StartTraining{Gen: 1, Alloc: validAlloc(), Price: 12}
	ns, err := Apply(s, cmd, b)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ns.Resources.RnD != 30000 { // 50000 - 20000
		t.Errorf("RnD = %v, want 30000", ns.Resources.RnD)
	}
	if !ns.HasTraining || ns.Training.Gen != 1 || ns.Training.Price != 12 {
		t.Errorf("training not set: %+v", ns.Training)
	}
	if ns.Training.WorkRemaining != 1800 {
		t.Errorf("WorkRemaining = %v, want 1800", ns.Training.WorkRemaining)
	}
	if s.HasTraining {
		t.Errorf("Apply mutated input")
	}
}

func TestApplyStartTrainingErrors(t *testing.T) {
	b := balance.Default()
	base := model.GameState{}
	base.Resources.RnD = 50000

	// already training
	busy := base
	busy.HasTraining = true
	if _, err := Apply(busy, model.StartTraining{Gen: 1, Alloc: validAlloc()}, b); err != ErrTrainingInProgress {
		t.Errorf("busy: err = %v, want ErrTrainingInProgress", err)
	}
	// invalid gen
	if _, err := Apply(base, model.StartTraining{Gen: 9, Alloc: validAlloc()}, b); err != ErrInvalidGen {
		t.Errorf("gen: err = %v, want ErrInvalidGen", err)
	}
	// bad alloc (sums to 0.8)
	bad := [model.NumQualityDims]float64{0.4, 0.2, 0.1, 0.1}
	if _, err := Apply(base, model.StartTraining{Gen: 1, Alloc: bad}, b); err != ErrInvalidAlloc {
		t.Errorf("alloc: err = %v, want ErrInvalidAlloc", err)
	}
	// insufficient R&D
	poor := model.GameState{}
	poor.Resources.RnD = 100
	if _, err := Apply(poor, model.StartTraining{Gen: 1, Alloc: validAlloc()}, b); err != ErrInsufficientRnD {
		t.Errorf("poor: err = %v, want ErrInsufficientRnD", err)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run TestApplyStartTraining`
Expected: FAIL（`undefined: ErrTrainingInProgress` 等）。

- [ ] **Step 3: 寫最小實作**

在 `internal/sim/apply.go` 新增 error vars（`ErrUnknownCommand` 那行下方）：
```go
var (
	ErrTrainingInProgress = errors.New("sim: training already in progress")
	ErrInsufficientRnD    = errors.New("sim: insufficient R&D")
	ErrInvalidGen         = errors.New("sim: invalid generation")
	ErrInvalidAlloc       = errors.New("sim: allocation must sum to 1")
)
```

在 `Apply` 的 switch 內、`RentTrainingCompute` case 之後新增：
```go
	case model.StartTraining:
		return applyStartTraining(s, c, b)
```

新增函式：
```go
func applyStartTraining(s model.GameState, c model.StartTraining, b balance.Config) (model.GameState, error) {
	if s.HasTraining {
		return s, ErrTrainingInProgress
	}
	if c.Gen < 1 || c.Gen > balance.MaxGen {
		return s, ErrInvalidGen
	}
	var sum float64
	for _, a := range c.Alloc {
		if a < 0 {
			return s, ErrInvalidAlloc
		}
		sum += a
	}
	if sum < 0.999 || sum > 1.001 {
		return s, ErrInvalidAlloc
	}
	cost := b.GenRnDCost[c.Gen]
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
		WorkRemaining: b.GenTrainWorkGPUSec[c.Gen],
	}
	return ns, nil
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/ -run TestApply`
Expected: PASS（含 Task 3 既有）。

- [ ] **Step 5: Commit**

```bash
git add internal/sim/apply.go internal/sim/apply_test.go
git commit -m "feat(sim): add StartTraining command with validation"
```

---

### Task 5: `Tick` — 以算力推進訓練，完成則模型上線

**Files:**
- Modify: `internal/sim/sim.go`
- Test: `internal/sim/sim_test.go`

**Interfaces:**
- Consumes: `GameState.Compute` / `Training` / `HasTraining` / `Models`（Task 1）；`balance.GenQualityCap`（Task 2）。
- Produces: `Tick` 於既有 R&D 邏輯後新增訓練推進——若 `HasTraining`：`WorkRemaining -= TrainingCapacity * dt`；`<= 0` 則完成，建 `Model{Gen, Quality[d]=Alloc[d]*GenQualityCap[gen], Price, Online=true}` append 進 `Models`（append 前 clone slice），並清 `HasTraining`。

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/sim_test.go` 末尾新增：
```go
func TestTickTrainingProgress(t *testing.T) {
	b := balance.Default()
	s := model.GameState{HasTraining: true}
	s.Compute.TrainingCapacity = 2
	s.Training = model.TrainingJob{Gen: 1, WorkRemaining: 1800}
	ns := Tick(s, 100, nil, b) // 2 GPU * 100s = 200 work done
	if !approx(ns.Training.WorkRemaining, 1600) {
		t.Fatalf("WorkRemaining = %v, want 1600", ns.Training.WorkRemaining)
	}
	if !ns.HasTraining || len(ns.Models) != 0 {
		t.Fatalf("should still be training, no model yet")
	}
}

func TestTickTrainingCompletes(t *testing.T) {
	b := balance.Default()
	s := model.GameState{HasTraining: true}
	s.Compute.TrainingCapacity = 10
	s.Training = model.TrainingJob{
		Gen:           2, // GenQualityCap[2] = 45
		Alloc:         [model.NumQualityDims]float64{0.4, 0.2, 0.2, 0.2},
		Price:         12,
		WorkRemaining: 7200,
	}
	ns := Tick(s, 1000, nil, b) // 10*1000 = 10000 >= 7200 → completes
	if ns.HasTraining {
		t.Fatalf("training should be done")
	}
	if len(ns.Models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(ns.Models))
	}
	m := ns.Models[0]
	if !m.Online || m.Gen != 2 || m.Price != 12 {
		t.Fatalf("model fields wrong: %+v", m)
	}
	if !approx(m.Quality[model.DimCapability], 18) { // 0.4 * 45
		t.Errorf("capability = %v, want 18", m.Quality[model.DimCapability])
	}
	if !approx(m.Quality[model.DimSafety], 9) { // 0.2 * 45
		t.Errorf("safety = %v, want 9", m.Quality[model.DimSafety])
	}
	// purity: input Models slice untouched
	if len(s.Models) != 0 {
		t.Errorf("Tick mutated input Models")
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run TestTickTraining`
Expected: FAIL（訓練未推進 / 無模型）。

- [ ] **Step 3: 寫最小實作**

把 `internal/sim/sim.go` 的 `Tick` 改成（保留既有 R&D 邏輯，於 return 前加訓練推進）：
```go
func Tick(s model.GameState, dt float64, events []model.TokenEvent, b balance.Config) model.GameState {
	ns := s
	ns.GameTime += dt

	// Advance the soft-cap window; reset cumulative when the window elapses.
	ns.WindowElapsed += dt
	if ns.WindowElapsed >= b.SoftCapWindowSec {
		ns.WindowElapsed -= b.SoftCapWindowSec
		ns.WindowRnD = 0
	}

	staffRnD := staffRnDPerSec(s.Research, b) * dt

	raw := tokenRawRnD(events, b)
	tokenRnD, newWindow := applySoftCap(ns.WindowRnD, raw, b.SoftCapFull, b.SoftCapMult)
	ns.WindowRnD = newWindow

	ns.Resources.RnD += staffRnD + tokenRnD

	ns = advanceTraining(ns, dt, b)
	return ns
}

// advanceTraining progresses the in-progress training job by dt and onlines
// the model on completion. Pure: clones Models before appending.
func advanceTraining(ns model.GameState, dt float64, b balance.Config) model.GameState {
	if !ns.HasTraining {
		return ns
	}
	ns.Training.WorkRemaining -= ns.Compute.TrainingCapacity * dt
	if ns.Training.WorkRemaining > 0 {
		return ns
	}
	// Completed → build the model and online it.
	job := ns.Training
	m := model.Model{Gen: job.Gen, Price: job.Price, Online: true}
	for d := 0; d < model.NumQualityDims; d++ {
		m.Quality[d] = job.Alloc[d] * b.GenQualityCap[job.Gen]
	}
	cloned := append([]model.Model(nil), ns.Models...)
	ns.Models = append(cloned, m)
	ns.HasTraining = false
	ns.Training = model.TrainingJob{}
	return ns
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/`
Expected: PASS（全部，含 Plan 01 既有）。

- [ ] **Step 5: Commit**

```bash
git add internal/sim/sim.go internal/sim/sim_test.go
git commit -m "feat(sim): advance training and online model in Tick"
```

---

### Task 6: `Tick` — 扣訓練租金

**Files:**
- Modify: `internal/sim/sim.go`
- Test: `internal/sim/sim_test.go`

**Interfaces:**
- Consumes: `balance.TrainRentPerGPUSec`、`GameState.Compute.TrainingCapacity`。
- Produces: `Tick` 每 tick 扣訓練租金 `Cash -= TrainingCapacity * TrainRentPerGPUSec * dt`（允許 Cash 為負；破產處理屬後續 plan）。

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/sim_test.go` 末尾新增：
```go
func TestTickDeductsTrainingRent(t *testing.T) {
	b := balance.Default() // TrainRentPerGPUSec = 0.01
	s := model.GameState{}
	s.Compute.TrainingCapacity = 4
	s.Resources.Cash = 100
	ns := Tick(s, 10, nil, b) // 4 * 0.01 * 10 = 0.4
	if !approx(ns.Resources.Cash, 99.6) {
		t.Fatalf("Cash = %v, want 99.6", ns.Resources.Cash)
	}
}

func TestTickRentZeroWhenNoCapacity(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Resources.Cash = 100
	ns := Tick(s, 10, nil, b)
	if !approx(ns.Resources.Cash, 100) {
		t.Fatalf("Cash = %v, want 100 (no capacity, no rent)", ns.Resources.Cash)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run TestTickDeductsTrainingRent`
Expected: FAIL（Cash 未被扣）。

- [ ] **Step 3: 寫最小實作**

在 `Tick` 內、`ns = advanceTraining(ns, dt, b)` 之前加入一行：
```go
	ns.Resources.Cash -= ns.Compute.TrainingCapacity * b.TrainRentPerGPUSec * dt
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
git commit -m "feat(sim): deduct training compute rent per tick"
```

---

## 完成後狀態

訓練管線可測試閉環：`Apply(RentTrainingCompute)` 租算力 → `Apply(StartTraining)` 扣 R&D 建工作 → 多次 `Tick` 以算力推進 → 完成上線成 `Model`，過程扣訓練租金。仍純確定性、不 mutate 輸入。

**下一步（Plan 03）**：online 模型長用戶（品質 → 目標用戶、指數逼近）、產生營收（用戶 × 玩家定價）、`SetPrice` 指令與需求彈性。
