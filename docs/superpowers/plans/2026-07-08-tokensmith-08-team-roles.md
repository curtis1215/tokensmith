# Tokensmith 08 — 團隊：四職能聚合 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 加入四種聚合職能（研究員 / 工程師 / 營運 / 行銷）：可花現金招募 / 解雇、每 tick 付薪資，各職能提供對應加成——工程師→算力效率、營運→降服務流失、行銷→拉高用戶目標（研究員→研發人力已在 Plan 01）。

**Architecture:** `GameState` 新增 `Engineers/Ops/Marketing`（研究員沿用 `Research.Researchers[]`）。新增 `HireStaff`/`FireStaff` 指令。`Tick` 扣總薪資。工程師 → `infraEfficiency` 乘進 `effectiveTraining/Inference`；行銷 → 乘進 `advanceUsers` 目標；營運 → 除進 `advanceServing` 流失。**非破壞性**：0 人時加成 = 1、薪資 = 0 → Plan 01–07 測試不變。明星員工與科技樹留後續 plan。相依 Plan 01–07。

**Tech Stack:** Go 1.22+、標準 `testing` + `math`。無外部依賴。

## Global Constraints

- 延續 Plan 01–07。`internal/sim` 純；不 mutate 輸入、clone slice。
- 職能：`RoleResearcher=0, RoleEngineer=1, RoleOps=2, RoleMarketing=3`。研究員分 tier（沿用 `Research.Researchers[NumTiers]`）；其餘三職能為單一計數。
- 加成（0 人時中性）：`infraEfficiency = 1 + Engineers·EngineerInfraBonus`（乘進有效算力）；`marketingMult = 1 + Marketing·MarketingBonus`（乘進用戶目標）；`opsFactor = 1/(1 + Ops·OpsChurnReduction)`（乘進服務流失）。
- v0 數值：ResearcherHireCost `{0,5000,15000,40000}`、ResearcherSalaryPerSec `{0,0.001,0.002,0.005}`；Engineer/Ops/Marketing HireCost `8000/6000/6000`、SalaryPerSec `0.002/0.0015/0.0015`；`EngineerInfraBonus=0.02`、`OpsChurnReduction=0.1`、`MarketingBonus=0.03`。

---

### Task 1: `model` 擴充——職能與招募指令

**Files:**
- Modify: `internal/model/types.go`
- Test: `internal/model/types_test.go`

**Interfaces:**
- Produces:
  - `model.Role`（int）+ `RoleResearcher=0, RoleEngineer=1, RoleOps=2, RoleMarketing=3`, `NumRoles=4`
  - `GameState` 新欄位 `Engineers, Ops, Marketing int`
  - `model.HireStaff{ Role Role; Tier StaffTier; Count int }`、`model.FireStaff{ Role Role; Tier StaffTier; Count int }`，各實作 `commandMarker()`

- [ ] **Step 1: 寫失敗測試**

在 `internal/model/types_test.go` 末尾新增：
```go
func TestRolesAndStaffCommands(t *testing.T) {
	if NumRoles != 4 || RoleMarketing != 3 {
		t.Fatalf("role consts wrong")
	}
	var s GameState
	s.Engineers = 3
	s.Ops = 2
	s.Marketing = 1
	if s.Engineers != 3 || s.Ops != 2 || s.Marketing != 1 {
		t.Fatalf("staff fields wrong: %+v", s)
	}
	var c1 Command = HireStaff{Role: RoleResearcher, Tier: Tier2, Count: 3}
	var c2 Command = FireStaff{Role: RoleEngineer, Count: 1}
	if _, ok := c1.(HireStaff); !ok {
		t.Fatalf("HireStaff not a Command")
	}
	if _, ok := c2.(FireStaff); !ok {
		t.Fatalf("FireStaff not a Command")
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/model/`
Expected: FAIL（`undefined: RoleResearcher` / `HireStaff`）。

- [ ] **Step 3: 寫最小實作**

在 `internal/model/types.go` 末尾新增：
```go
// Role identifies an aggregate staff function.
type Role int

const (
	RoleResearcher Role = iota // 0
	RoleEngineer               // 1
	RoleOps                    // 2
	RoleMarketing              // 3
	NumRoles = 4
)

// HireStaff hires Count staff of Role (Tier used only for RoleResearcher).
type HireStaff struct {
	Role  Role
	Tier  StaffTier
	Count int
}

func (HireStaff) commandMarker() {}

// FireStaff removes Count staff of Role (Tier used only for RoleResearcher).
type FireStaff struct {
	Role  Role
	Tier  StaffTier
	Count int
}

func (FireStaff) commandMarker() {}
```

在 `GameState` 結構加入欄位（放在 `Research` 之後）：
```go
	Engineers int
	Ops       int
	Marketing int
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/model/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/model/types.go internal/model/types_test.go
git commit -m "feat(model): add staff roles and hire/fire commands"
```

---

### Task 2: `balance` — 招募成本 / 薪資 / 職能加成

**Files:**
- Modify: `internal/balance/balance.go`
- Test: `internal/balance/balance_test.go`

**Interfaces:**
- Produces: `Config` 新欄位 + `Default()`：
  - `ResearcherHireCost [model.NumTiers]float64`、`ResearcherSalaryPerSec [model.NumTiers]float64`
  - `EngineerHireCost, OpsHireCost, MarketingHireCost float64`
  - `EngineerSalaryPerSec, OpsSalaryPerSec, MarketingSalaryPerSec float64`
  - `EngineerInfraBonus, OpsChurnReduction, MarketingBonus float64`

- [ ] **Step 1: 寫失敗測試**

在 `internal/balance/balance_test.go` 末尾新增：
```go
func TestDefaultStaffValues(t *testing.T) {
	c := Default()
	if c.ResearcherHireCost[model.Tier2] != 15000 {
		t.Errorf("ResearcherHireCost[T2] = %v, want 15000", c.ResearcherHireCost[model.Tier2])
	}
	if c.ResearcherSalaryPerSec[model.Tier3] != 0.005 {
		t.Errorf("ResearcherSalaryPerSec[T3] = %v, want 0.005", c.ResearcherSalaryPerSec[model.Tier3])
	}
	if c.EngineerHireCost != 8000 || c.OpsHireCost != 6000 || c.MarketingHireCost != 6000 {
		t.Errorf("hire costs wrong: %+v", c)
	}
	if c.EngineerInfraBonus != 0.02 || c.OpsChurnReduction != 0.1 || c.MarketingBonus != 0.03 {
		t.Errorf("staff bonuses wrong: %+v", c)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/balance/`
Expected: FAIL（欄位不存在）。

- [ ] **Step 3: 寫最小實作**

在 `internal/balance/balance.go` 的 `Config` 結構末尾加入欄位：
```go
	// Aggregate staff (plan-08).
	ResearcherHireCost     [model.NumTiers]float64
	ResearcherSalaryPerSec [model.NumTiers]float64
	EngineerHireCost       float64
	OpsHireCost            float64
	MarketingHireCost      float64
	EngineerSalaryPerSec   float64
	OpsSalaryPerSec        float64
	MarketingSalaryPerSec  float64
	EngineerInfraBonus     float64 // per engineer: compute efficiency
	OpsChurnReduction      float64 // per ops: service-churn mitigation
	MarketingBonus         float64 // per marketing: user-target boost
```

在 `Default()` 的 `return c` 之前加入：
```go
	c.ResearcherHireCost = [model.NumTiers]float64{0, 5000, 15000, 40000}
	c.ResearcherSalaryPerSec = [model.NumTiers]float64{0, 0.001, 0.002, 0.005}
	c.EngineerHireCost = 8000
	c.OpsHireCost = 6000
	c.MarketingHireCost = 6000
	c.EngineerSalaryPerSec = 0.002
	c.OpsSalaryPerSec = 0.0015
	c.MarketingSalaryPerSec = 0.0015
	c.EngineerInfraBonus = 0.02
	c.OpsChurnReduction = 0.1
	c.MarketingBonus = 0.03
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/balance/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/balance/balance.go internal/balance/balance_test.go
git commit -m "feat(balance): add staff hire costs, salaries, bonuses"
```

---

### Task 3: `sim.Apply` — HireStaff / FireStaff

**Files:**
- Modify: `internal/sim/apply.go`
- Test: `internal/sim/apply_test.go`

**Interfaces:**
- Consumes: `model.HireStaff` / `FireStaff`；`balance` 招募成本。
- Produces:
  - package vars `ErrInvalidCount`, `ErrInvalidTier`, `ErrInvalidRole`
  - `Apply` 新增處理 `HireStaff`（驗證 count>0、研究員 tier、現金足夠 → 扣現金、加人）與 `FireStaff`（減人、夾 0）。

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/apply_test.go` 末尾新增：
```go
func TestApplyHireStaff(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Resources.Cash = 100000
	// hire 2 T2 researchers
	ns, err := Apply(s, model.HireStaff{Role: model.RoleResearcher, Tier: model.Tier2, Count: 2}, b)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ns.Research.Researchers[model.Tier2] != 2 {
		t.Errorf("researchers = %d, want 2", ns.Research.Researchers[model.Tier2])
	}
	if !approx(ns.Resources.Cash, 100000-2*b.ResearcherHireCost[model.Tier2]) {
		t.Errorf("cash wrong: %v", ns.Resources.Cash)
	}
	// hire 3 engineers
	ns2, _ := Apply(ns, model.HireStaff{Role: model.RoleEngineer, Count: 3}, b)
	if ns2.Engineers != 3 {
		t.Errorf("engineers = %d, want 3", ns2.Engineers)
	}
	// purity
	if s.Research.Researchers[model.Tier2] != 0 {
		t.Errorf("Apply mutated input")
	}
}

func TestApplyHireStaffErrors(t *testing.T) {
	b := balance.Default()
	rich := model.GameState{}
	rich.Resources.Cash = 1e9
	if _, err := Apply(rich, model.HireStaff{Role: model.RoleResearcher, Tier: model.Tier2, Count: 0}, b); err != ErrInvalidCount {
		t.Errorf("count: err = %v, want ErrInvalidCount", err)
	}
	if _, err := Apply(rich, model.HireStaff{Role: model.RoleResearcher, Tier: model.TierNone, Count: 1}, b); err != ErrInvalidTier {
		t.Errorf("tier: err = %v, want ErrInvalidTier", err)
	}
	poor := model.GameState{}
	poor.Resources.Cash = 10
	if _, err := Apply(poor, model.HireStaff{Role: model.RoleEngineer, Count: 1}, b); err != ErrInsufficientCash {
		t.Errorf("cash: err = %v, want ErrInsufficientCash", err)
	}
}

func TestApplyFireStaffFloorsAtZero(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Engineers: 2}
	ns, err := Apply(s, model.FireStaff{Role: model.RoleEngineer, Count: 5}, b)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ns.Engineers != 0 {
		t.Fatalf("engineers = %d, want 0", ns.Engineers)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run 'TestApplyHireStaff|TestApplyFireStaff'`
Expected: FAIL（`undefined: ErrInvalidCount` 等）。

- [ ] **Step 3: 寫最小實作**

在 `internal/sim/apply.go` 的 error var 區塊加入：
```go
	ErrInvalidCount = errors.New("sim: count must be positive")
	ErrInvalidTier  = errors.New("sim: invalid researcher tier")
	ErrInvalidRole  = errors.New("sim: invalid role")
```

在 `Apply` 的 switch 內、`BuildServer` case 之後新增：
```go
	case model.HireStaff:
		return applyHireStaff(s, c, b)
	case model.FireStaff:
		return applyFireStaff(s, c)
```

新增函式：
```go
func applyHireStaff(s model.GameState, c model.HireStaff, b balance.Config) (model.GameState, error) {
	if c.Count <= 0 {
		return s, ErrInvalidCount
	}
	n := float64(c.Count)
	ns := s
	switch c.Role {
	case model.RoleResearcher:
		if c.Tier < model.Tier1 || c.Tier > model.Tier3 {
			return s, ErrInvalidTier
		}
		cost := n * b.ResearcherHireCost[c.Tier]
		if s.Resources.Cash < cost {
			return s, ErrInsufficientCash
		}
		ns.Resources.Cash -= cost
		ns.Research.Researchers[c.Tier] += c.Count
	case model.RoleEngineer:
		if s.Resources.Cash < n*b.EngineerHireCost {
			return s, ErrInsufficientCash
		}
		ns.Resources.Cash -= n * b.EngineerHireCost
		ns.Engineers += c.Count
	case model.RoleOps:
		if s.Resources.Cash < n*b.OpsHireCost {
			return s, ErrInsufficientCash
		}
		ns.Resources.Cash -= n * b.OpsHireCost
		ns.Ops += c.Count
	case model.RoleMarketing:
		if s.Resources.Cash < n*b.MarketingHireCost {
			return s, ErrInsufficientCash
		}
		ns.Resources.Cash -= n * b.MarketingHireCost
		ns.Marketing += c.Count
	default:
		return s, ErrInvalidRole
	}
	return ns, nil
}

func applyFireStaff(s model.GameState, c model.FireStaff) (model.GameState, error) {
	if c.Count <= 0 {
		return s, ErrInvalidCount
	}
	ns := s
	switch c.Role {
	case model.RoleResearcher:
		if c.Tier < model.Tier1 || c.Tier > model.Tier3 {
			return s, ErrInvalidTier
		}
		ns.Research.Researchers[c.Tier] = max0(ns.Research.Researchers[c.Tier] - c.Count)
	case model.RoleEngineer:
		ns.Engineers = max0(ns.Engineers - c.Count)
	case model.RoleOps:
		ns.Ops = max0(ns.Ops - c.Count)
	case model.RoleMarketing:
		ns.Marketing = max0(ns.Marketing - c.Count)
	default:
		return s, ErrInvalidRole
	}
	return ns, nil
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/ -run TestApply`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/sim/apply.go internal/sim/apply_test.go
git commit -m "feat(sim): add HireStaff and FireStaff commands"
```

---

### Task 4: `Tick` — 扣總薪資

**Files:**
- Modify: `internal/sim/sim.go`
- Test: `internal/sim/sim_test.go`

**Interfaces:**
- Consumes: `balance` 薪資欄位。
- Produces: `sim.totalSalaryPerSec(ns, b) float64`；`Tick` 於電費之後扣 `totalSalaryPerSec · dt`。

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/sim_test.go` 末尾新增：
```go
func TestTickDeductsSalary(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Research.Researchers[model.Tier2] = 3
	s.Engineers = 2
	s.Resources.Cash = 100
	ns := Tick(s, 10, nil, b)
	want := 100 - (3*b.ResearcherSalaryPerSec[model.Tier2]+2*b.EngineerSalaryPerSec)*10
	if !approx(ns.Resources.Cash, want) {
		t.Fatalf("Cash = %v, want %v", ns.Resources.Cash, want)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run TestTickDeductsSalary`
Expected: FAIL（薪資未扣）。

- [ ] **Step 3: 寫最小實作**

在 `internal/sim/sim.go` 新增 helper（放在 `Tick` 之前）：
```go
// totalSalaryPerSec is the aggregate staff salary per second.
func totalSalaryPerSec(ns model.GameState, b balance.Config) float64 {
	var s float64
	for tier := model.Tier1; tier <= model.Tier3; tier++ {
		s += float64(ns.Research.Researchers[tier]) * b.ResearcherSalaryPerSec[tier]
	}
	s += float64(ns.Engineers) * b.EngineerSalaryPerSec
	s += float64(ns.Ops) * b.OpsSalaryPerSec
	s += float64(ns.Marketing) * b.MarketingSalaryPerSec
	return s
}
```

在 `Tick` 內，緊接電費那行（`... ElectricityPerKWSec * dt`）之後加入：
```go
	ns.Resources.Cash -= totalSalaryPerSec(ns, b) * dt
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/`
Expected: PASS（全部）。

- [ ] **Step 5: Commit**

```bash
git add internal/sim/sim.go internal/sim/sim_test.go
git commit -m "feat(sim): deduct aggregate staff salary per tick"
```

---

### Task 5: 職能加成——工程師 / 行銷 / 營運

**Files:**
- Modify: `internal/sim/sim.go`
- Test: `internal/sim/sim_test.go`

**Interfaces:**
- Consumes: `GameState.Engineers/Marketing/Ops`；`balance.EngineerInfraBonus/MarketingBonus/OpsChurnReduction`。
- Produces:
  - `sim.infraEfficiency(ns, b) float64`（`1 + Engineers·EngineerInfraBonus`）
  - `effectiveTraining`/`effectiveInference` 改簽名為 `(ns, b)` 並乘 `infraEfficiency`
  - `advanceUsers` 目標乘 `marketingMult = 1 + Marketing·MarketingBonus`
  - `advanceServing` 流失乘 `opsFactor = 1/(1 + Ops·OpsChurnReduction)`

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/sim_test.go` 末尾新增：
```go
func TestEngineersSpeedTraining(t *testing.T) {
	b := balance.Default()
	base := model.GameState{HasTraining: true}
	base.Compute.TrainingCapacity = 10
	base.Training = model.TrainingJob{Gen: 1, WorkRemaining: 1e9}
	withEng := base
	withEng.Engineers = 5 // infra mult 1.1
	nb := Tick(base, 1, nil, b)
	ne := Tick(withEng, 1, nil, b)
	if ne.Training.WorkRemaining >= nb.Training.WorkRemaining {
		t.Fatalf("engineers should speed training: %v vs %v", ne.Training.WorkRemaining, nb.Training.WorkRemaining)
	}
}

func TestMarketingBoostsUsers(t *testing.T) {
	b := balance.Default()
	base := model.GameState{Models: []model.Model{onlineModel(50, b.RefPrice)}}
	withMkt := model.GameState{Models: []model.Model{onlineModel(50, b.RefPrice)}, Marketing: 10}
	nb := Tick(base, 1, nil, b)
	nm := Tick(withMkt, 1, nil, b)
	if nm.Models[0].Users <= nb.Models[0].Users {
		t.Fatalf("marketing should boost users: %v vs %v", nm.Models[0].Users, nb.Models[0].Users)
	}
}

func TestOpsReducesServiceChurn(t *testing.T) {
	b := balance.Default()
	m := onlineModel(50, b.RefPrice)
	m.Users = 100000
	base := model.GameState{Models: []model.Model{m}}
	base.Compute.InferenceCapacity = 1 // overloaded
	withOps := base
	withOps.Ops = 20
	nb := Tick(base, 1, nil, b)
	no := Tick(withOps, 1, nil, b)
	if no.Models[0].Users <= nb.Models[0].Users {
		t.Fatalf("ops should reduce churn: %v vs %v", no.Models[0].Users, nb.Models[0].Users)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run 'TestEngineersSpeedTraining|TestMarketingBoostsUsers|TestOpsReducesServiceChurn'`
Expected: FAIL（加成未生效）。

- [ ] **Step 3: 寫最小實作**

在 `internal/sim/sim.go` 新增 `infraEfficiency`，並把 `effectiveTraining`/`effectiveInference` 改為 `(ns, b)` 且乘上它：
```go
// infraEfficiency scales compute effectiveness with engineers.
func infraEfficiency(ns model.GameState, b balance.Config) float64 {
	return 1 + float64(ns.Engineers)*b.EngineerInfraBonus
}

func effectiveTraining(ns model.GameState, b balance.Config) float64 {
	c := ns.Compute.TrainingCapacity
	for _, sv := range ns.Servers {
		if sv.Pool == model.PoolTraining {
			c += sv.Compute
		}
	}
	return c * infraEfficiency(ns, b)
}

func effectiveInference(ns model.GameState, b balance.Config) float64 {
	c := ns.Compute.InferenceCapacity
	for _, sv := range ns.Servers {
		if sv.Pool == model.PoolInference {
			c += sv.Compute
		}
	}
	return c * infraEfficiency(ns, b)
}
```

把 `advanceTraining` 的推進行改為傳 `b`：
```go
	ns.Training.WorkRemaining -= effectiveTraining(ns, b) * dt
```

把 `advanceServing` 的容量行改為傳 `b`，並讓流失乘 `opsFactor`：
```go
	capacity := effectiveInference(ns, b)
	if capacity <= 0 || load <= capacity {
		return ns
	}
	deficit := (load - capacity) / load
	opsFactor := 1.0 / (1 + float64(ns.Ops)*b.OpsChurnReduction)
	models := append([]model.Model(nil), ns.Models...)
	for i := range models {
		m := &models[i]
		if !m.Online {
			continue
		}
		m.Users -= m.Users * b.ServiceChurnRate * deficit * dt * opsFactor
		if m.Users < 0 {
			m.Users = 0
		}
	}
	ns.Models = models
	return ns
```

在 `advanceUsers` 迴圈內，把 `target := ...` 那行改為乘上行銷加成：
```go
		marketingMult := 1 + float64(ns.Marketing)*b.MarketingBonus
		target := appeal * b.SegmentTargetScale[m.Segment] * demandMult * share * marketingMult
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/`
Expected: PASS（全部——既有測試 0 工程師/行銷/營運 → 加成 1、流失不變）。

- [ ] **Step 5: 跑整包 + vet + commit**

Run:
```bash
go test ./...
go vet ./...
go build ./...
```
Expected: 全部 PASS、vet 無警告、可建置。

```bash
git add internal/sim/sim.go internal/sim/sim_test.go
git commit -m "feat(sim): wire engineer/marketing/ops staff bonuses"
```

---

## 完成後狀態

四職能聚合團隊成立：花現金招募研究員 / 工程師 / 營運 / 行銷、每 tick 付薪資（過度擴編壓垮現金流）；工程師拉高算力效率、行銷拉高用戶目標、營運降低服務流失、研究員產研發人力。0 人時全部中性 → Plan 01–07 不變。仍純確定性。

**下一步（Plan 09）**：科技樹四分支（演算法 / 硬體基建 / 商業營運 / 對齊安全）——用 R&D 解鎖永久加成，接進四維品質產出、算力效率、用戶 / 定價、安全與抗事故。之後再做明星員工、里程碑 / prestige，然後 store / ingest / daemon / 完整 TUI。
