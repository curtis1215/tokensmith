# Tokensmith 04 — 具名對手 · 前沿老化 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 加入具名 AI 對手公司與其研發成長曲線，讓玩家模型的用戶目標乘上「競爭市佔係數」；對手品質隨時間上升 → 你的市佔下降 → 模型相對老化流失。

**Architecture:** 新增 `model.Competitor`（四維品質 + 每維每秒成長）與 `GameState.Competitors`。`Tick` 新增 `advanceCompetitors`（推進對手品質）。`advanceUsers` 的目標改乘 `競爭市佔係數 share = 你的模型吸引力 / (你的吸引力 + 對手吸引力總和)`——**無對手時 share=1，故 Plan 03 行為與測試完全不變**（非破壞性）。本 plan 仍單一用戶池；市場區隔（消費者/企業/開發者）留 Plan 05。相依 Plan 01–03。

**Tech Stack:** Go 1.22+、標準 `testing` + `math`。無外部依賴。

## Global Constraints

- 延續 Plan 01–03 全部約束。`internal/sim` 純（`math` 允許；無 wall-clock/rand/IO）。`Apply`/`Tick` 不 mutate 輸入；修改 `Models`/`Competitors` 前先 clone slice。
- 競爭市佔係數：`share = appeal / (appeal + rivalAppeal)`；`appeal+rivalAppeal == 0` 時 `share=1`（退化為無競爭）。目標 = `appeal · UserTargetPerAppeal · demandMult · share`。
- 對手吸引力用與玩家相同的 `QualityWeights`（單一聚合池；區隔權重屬 Plan 05）。
- 對手初始陣容取自 spec §17.1（7 家）；成長率為 v0（可調）。

---

### Task 1: `model` 擴充——Competitor 型別

**Files:**
- Modify: `internal/model/types.go`
- Test: `internal/model/types_test.go`

**Interfaces:**
- Consumes: `model.NumQualityDims`（Plan 02）。
- Produces:
  - `model.Competitor{ Name string; Quality [NumQualityDims]float64; GrowthPerSec [NumQualityDims]float64 }`
  - `GameState` 新欄位 `Competitors []Competitor`

- [ ] **Step 1: 寫失敗測試**

在 `internal/model/types_test.go` 末尾新增：
```go
func TestCompetitorFields(t *testing.T) {
	c := Competitor{Name: "Rival"}
	c.Quality[DimCapability] = 55
	c.GrowthPerSec[DimCapability] = 0.0001
	if c.Name != "Rival" || c.Quality[DimCapability] != 55 || c.GrowthPerSec[DimCapability] != 0.0001 {
		t.Fatalf("competitor fields wrong: %+v", c)
	}
	var s GameState
	s.Competitors = append(s.Competitors, c)
	if len(s.Competitors) != 1 {
		t.Fatalf("GameState.Competitors not usable")
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/model/`
Expected: FAIL（`undefined: Competitor`）。

- [ ] **Step 3: 寫最小實作**

在 `internal/model/types.go` 末尾新增：
```go
// Competitor is a rival AI company competing for market share.
type Competitor struct {
	Name         string
	Quality      [NumQualityDims]float64
	GrowthPerSec [NumQualityDims]float64 // per-second quality growth by dim
}
```

在 `GameState` 結構加入欄位（放在 `Models` 之後）：
```go
	Competitors []Competitor
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/model/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/model/types.go internal/model/types_test.go
git commit -m "feat(model): add Competitor type"
```

---

### Task 2: `balance` — 對手初始陣容（§17.1）

**Files:**
- Modify: `internal/balance/balance.go`
- Test: `internal/balance/balance_test.go`

**Interfaces:**
- Consumes: `model.Competitor`, `model.NumQualityDims`。
- Produces:
  - `balance.DefaultCompetitors() []model.Competitor`（7 家，四維順序 = 能力/成本/安全/速度）
  - 內部小工具 `qvec(cap, eff, safe, spd float64) [model.NumQualityDims]float64`

- [ ] **Step 1: 寫失敗測試**

在 `internal/balance/balance_test.go` 末尾新增：
```go
func TestDefaultCompetitors(t *testing.T) {
	cs := DefaultCompetitors()
	if len(cs) != 7 {
		t.Fatalf("competitors = %d, want 7", len(cs))
	}
	if cs[0].Name != "OpenAI" || cs[0].Quality[model.DimCapability] != 55 {
		t.Errorf("first competitor wrong: %+v", cs[0])
	}
	// every competitor has a name and some capability
	for _, c := range cs {
		if c.Name == "" {
			t.Errorf("competitor missing name: %+v", c)
		}
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/balance/`
Expected: FAIL（`undefined: DefaultCompetitors`）。

- [ ] **Step 3: 寫最小實作**

在 `internal/balance/balance.go` 末尾新增：
```go
// qvec builds a per-dimension vector in dim order: capability, efficiency,
// safety, speed.
func qvec(capability, efficiency, safety, speed float64) [model.NumQualityDims]float64 {
	return [model.NumQualityDims]float64{capability, efficiency, safety, speed}
}

// DefaultCompetitors returns the v0 named-competitor roster (spec §17.1).
// GrowthPerSec is tunable v0; specialty dimensions grow fastest.
func DefaultCompetitors() []model.Competitor {
	return []model.Competitor{
		{Name: "OpenAI", Quality: qvec(55, 35, 35, 45), GrowthPerSec: qvec(0.0001, 0.00003, 0.00003, 0.00005)},
		{Name: "Anthropic", Quality: qvec(52, 30, 55, 40), GrowthPerSec: qvec(0.00007, 0.00003, 0.0001, 0.00004)},
		{Name: "xAI", Quality: qvec(45, 30, 20, 50), GrowthPerSec: qvec(0.0001, 0.00003, 0.00002, 0.00008)},
		{Name: "DeepSeek", Quality: qvec(42, 60, 25, 45), GrowthPerSec: qvec(0.00005, 0.0001, 0.00003, 0.00005)},
		{Name: "Qwen", Quality: qvec(40, 50, 30, 45), GrowthPerSec: qvec(0.00005, 0.00007, 0.00004, 0.00005)},
		{Name: "Zhipu", Quality: qvec(40, 45, 35, 38), GrowthPerSec: qvec(0.00004, 0.00005, 0.00004, 0.00003)},
		{Name: "Gemini", Quality: qvec(48, 40, 42, 45), GrowthPerSec: qvec(0.00006, 0.00005, 0.00006, 0.00005)},
	}
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/balance/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/balance/balance.go internal/balance/balance_test.go
git commit -m "feat(balance): add default competitor roster (spec 17.1)"
```

---

### Task 3: `Tick` — 推進對手研發曲線

**Files:**
- Modify: `internal/sim/sim.go`
- Test: `internal/sim/sim_test.go`

**Interfaces:**
- Consumes: `GameState.Competitors`。
- Produces: `sim.advanceCompetitors(ns, dt) model.GameState`（clone `Competitors`，每個對手每維 `Quality[d] += GrowthPerSec[d]·dt`）；`Tick` 於 `advanceTraining` 後、`advanceUsers` 前呼叫。

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/sim_test.go` 末尾新增：
```go
func TestTickAdvancesCompetitors(t *testing.T) {
	b := balance.Default()
	c := model.Competitor{Name: "Rival"}
	c.Quality[model.DimCapability] = 10
	c.GrowthPerSec[model.DimCapability] = 0.1
	s := model.GameState{Competitors: []model.Competitor{c}}
	ns := Tick(s, 10, nil, b) // 10 + 0.1*10 = 11
	if !approx(ns.Competitors[0].Quality[model.DimCapability], 11) {
		t.Fatalf("competitor cap = %v, want 11", ns.Competitors[0].Quality[model.DimCapability])
	}
	// purity: input competitor untouched
	if s.Competitors[0].Quality[model.DimCapability] != 10 {
		t.Fatalf("Tick mutated input competitor")
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run TestTickAdvancesCompetitors`
Expected: FAIL（對手品質未成長）。

- [ ] **Step 3: 寫最小實作**

在 `internal/sim/sim.go` 的 `Tick` 內，把 `ns = advanceTraining(ns, dt, b)` 之後改為：
```go
	ns = advanceTraining(ns, dt, b)
	ns = advanceCompetitors(ns, dt)
	ns = advanceUsers(ns, dt, b)
	return ns
```

新增函式：
```go
// advanceCompetitors grows each competitor's quality along its scripted
// curve. Pure: clones Competitors.
func advanceCompetitors(ns model.GameState, dt float64) model.GameState {
	if len(ns.Competitors) == 0 {
		return ns
	}
	comps := append([]model.Competitor(nil), ns.Competitors...)
	for i := range comps {
		for d := range model.NumQualityDims {
			comps[i].Quality[d] += comps[i].GrowthPerSec[d] * dt
		}
	}
	ns.Competitors = comps
	return ns
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/`
Expected: PASS（全部，含 Plan 01–03）。

- [ ] **Step 5: Commit**

```bash
git add internal/sim/sim.go internal/sim/sim_test.go
git commit -m "feat(sim): advance competitor quality curves in Tick"
```

---

### Task 4: `advanceUsers` — 競爭市佔係數（前沿老化）

**Files:**
- Modify: `internal/sim/sim.go`
- Test: `internal/sim/sim_test.go`

**Interfaces:**
- Consumes: `GameState.Competitors`, `balance.QualityWeights`。
- Produces:
  - `sim.appealOf(q, w [model.NumQualityDims]float64) float64`（`Σ q[d]·w[d]`）
  - `advanceUsers` 改用 `appealOf`，計算 `rivalAppeal = Σ 對手 appealOf`，每模型目標乘 `share = appeal/(appeal+rivalAppeal)`（分母 0 時 share=1）。無對手時行為與 Plan 03 完全相同。

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/sim_test.go` 末尾新增：
```go
func rival(cap float64) model.Competitor {
	c := model.Competitor{Name: "Rival"}
	c.Quality[model.DimCapability] = cap
	return c
}

func TestTickCompetitorHalvesUserTarget(t *testing.T) {
	b := balance.Default()
	// your model appeal 20 (cap 50 * 0.4). equal competitor appeal 20 → share 0.5.
	s := model.GameState{
		Models:      []model.Model{onlineModel(50, b.RefPrice)},
		Competitors: []model.Competitor{rival(50)}, // GrowthPerSec 0 → stays 20
	}
	ns := Tick(s, 1, nil, b) // target = 20*1000*1*0.5 = 10000; users = 10000*0.001 = 10
	if !approx(ns.Models[0].Users, 10) {
		t.Fatalf("Users = %v, want 10 (halved by equal competitor)", ns.Models[0].Users)
	}
}

func TestTickStrongCompetitorChurnsUsers(t *testing.T) {
	b := balance.Default()
	m := onlineModel(50, b.RefPrice) // appeal 20
	m.Users = 5000
	s := model.GameState{
		Models:      []model.Model{m},
		Competitors: []model.Competitor{rival(200)}, // appeal 80 → share 0.2 → target 4000 < 5000
	}
	ns := Tick(s, 1, nil, b)
	if ns.Models[0].Users >= 5000 {
		t.Fatalf("Users = %v, want < 5000 (churn vs strong competitor)", ns.Models[0].Users)
	}
}

func TestAppealOf(t *testing.T) {
	b := balance.Default()
	q := [model.NumQualityDims]float64{50, 0, 0, 0}
	if got := appealOf(q, b.QualityWeights); !approx(got, 20) { // 50 * 0.4
		t.Fatalf("appealOf = %v, want 20", got)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run 'TestTickCompetitor|TestTickStrongCompetitor|TestAppealOf'`
Expected: FAIL（`undefined: appealOf`；競爭未影響用戶）。

- [ ] **Step 3: 寫最小實作**

在 `internal/sim/sim.go` 新增 `appealOf`（放在 `advanceUsers` 之前）：
```go
// appealOf is the weighted quality score of a model or competitor.
func appealOf(q, w [model.NumQualityDims]float64) float64 {
	appeal := 0.0
	for d := range model.NumQualityDims {
		appeal += q[d] * w[d]
	}
	return appeal
}
```

把 `advanceUsers` 改成（新增 rivalAppeal + share；用 appealOf 取代內嵌迴圈）：
```go
func advanceUsers(ns model.GameState, dt float64, b balance.Config) model.GameState {
	if len(ns.Models) == 0 {
		return ns
	}
	rivalAppeal := 0.0
	for _, c := range ns.Competitors {
		rivalAppeal += appealOf(c.Quality, b.QualityWeights)
	}
	models := append([]model.Model(nil), ns.Models...)
	for i := range models {
		m := &models[i]
		if !m.Online {
			continue
		}
		ns.Resources.Cash += m.Users * m.Price * dt / b.MonthSec
		appeal := appealOf(m.Quality, b.QualityWeights)
		var demandMult float64
		if m.Price > 0 {
			demandMult = math.Pow(b.RefPrice/m.Price, b.PriceElasticity)
		}
		share := 1.0
		if appeal+rivalAppeal > 0 {
			share = appeal / (appeal + rivalAppeal)
		}
		target := appeal * b.UserTargetPerAppeal * demandMult * share
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
Expected: PASS（全部，含 Plan 03 用戶測試——無對手 → share=1 → 不受影響）。

- [ ] **Step 5: 跑整包 + vet + commit**

Run:
```bash
go test ./...
go vet ./...
```
Expected: 全部 PASS、vet 無警告。

```bash
git add internal/sim/sim.go internal/sim/sim_test.go
git commit -m "feat(sim): add competitive market-share and frontier obsolescence"
```

---

## 完成後狀態

競爭市場成立：7 家具名對手依研發曲線推升品質 → 你的模型市佔（吸引力占比）下降 → 用戶流失。玩家須持續訓練更強模型維持市佔（token 燃料的長期意義）。無對手時退化為 Plan 03 行為（測試不變）。仍純確定性、不 mutate 輸入。

**下一步（Plan 05）**：市場區隔（消費者 / 企業 / 開發者），各區隔不同維度權重與 TAM，模型指定主攻區隔，對手在各區隔分別競爭——把單一聚合池升級為三區隔市場。
