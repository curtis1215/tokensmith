# Tokensmith 05 — 市場區隔 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把單一聚合用戶池升級為三個市場區隔（消費者 / 企業 / 開發者），各有不同的品質維度權重、市場規模與參考價；模型指定主攻區隔，對手在各區隔以該區隔權重競爭。

**Architecture:** 新增 `model.Segment`（3 個）與 `Model.Segment` 欄位。`balance` 新增 per-segment 陣列 `SegmentWeights` / `SegmentTargetScale` / `SegmentRefPrice`。`advanceUsers` 依模型的 `Segment` 取用對應權重 / 規模 / 參考價與 per-segment 對手吸引力。**非破壞性**：區隔 0（消費者）的三個陣列值 = Plan 03/04 的舊 scalar（`QualityWeights`/`UserTargetPerAppeal`/`RefPrice`），且 `Model.Segment` 預設 0，故所有 Plan 03/04 測試不變。相依 Plan 01–04。

**Tech Stack:** Go 1.22+、標準 `testing` + `math`。無外部依賴。

## Global Constraints

- 延續 Plan 01–04 全部約束。`internal/sim` 純；`Apply`/`Tick` 不 mutate 輸入、clone slice。
- 區隔索引：`SegConsumer=0, SegEnterprise=1, SegDeveloper=2, NumSegments=3`。
- **保留** Plan 03 的 scalar 欄位 `QualityWeights` / `UserTargetPerAppeal` / `RefPrice`（Plan 03/04 測試仍引用；區隔 0 鏡射之），不得刪除。
- v0 區隔數值（spec §6.4.2 / §17）：消費者權重 `{0.4,0.2,0.2,0.2}`、企業 `{0.2,0.1,0.5,0.2}`、開發者 `{0.15,0.4,0.1,0.35}`（各自和為 1）；規模 `{1000,500,800}`；參考價 `{12,180,6}`。

---

### Task 1: `model` 擴充——Segment 型別與 Model.Segment 欄位

**Files:**
- Modify: `internal/model/types.go`
- Test: `internal/model/types_test.go`

**Interfaces:**
- Produces:
  - `model.Segment`（int）+ 常數 `SegConsumer=0, SegEnterprise=1, SegDeveloper=2`, `NumSegments=3`
  - `Model` 新欄位 `Segment Segment`（預設 0 = 消費者）

- [ ] **Step 1: 寫失敗測試**

在 `internal/model/types_test.go` 末尾新增：
```go
func TestSegmentConstsAndModelField(t *testing.T) {
	if NumSegments != 3 || SegDeveloper != 2 {
		t.Fatalf("segment consts wrong: NumSegments=%d SegDeveloper=%d", NumSegments, SegDeveloper)
	}
	m := Model{Segment: SegEnterprise}
	if m.Segment != SegEnterprise {
		t.Fatalf("model segment field wrong: %v", m.Segment)
	}
	var zero Model
	if zero.Segment != SegConsumer {
		t.Fatalf("default segment should be SegConsumer(0), got %v", zero.Segment)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/model/`
Expected: FAIL（`undefined: Segment` / `SegConsumer`）。

- [ ] **Step 3: 寫最小實作**

在 `internal/model/types.go` 的 `QualityDim` 常數區塊之後新增：
```go
// Segment indexes a market segment.
type Segment int

const (
	SegConsumer   Segment = iota // 0 消費者
	SegEnterprise                // 1 企業
	SegDeveloper                 // 2 開發者
	NumSegments = 3
)
```

在 `Model` 結構加入欄位（放在 `Gen` 之後）：
```go
	Segment Segment
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/model/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/model/types.go internal/model/types_test.go
git commit -m "feat(model): add market Segment and Model.Segment"
```

---

### Task 2: `balance` — per-segment 權重 / 規模 / 參考價

**Files:**
- Modify: `internal/balance/balance.go`
- Test: `internal/balance/balance_test.go`

**Interfaces:**
- Consumes: `model.NumSegments`, `model.NumQualityDims`, `model.Seg*`, `model.Dim*`；既有 `qvec`（Plan 04）。
- Produces: `Config` 新欄位 + `Default()` 填值：
  - `SegmentWeights [model.NumSegments][model.NumQualityDims]float64`
  - `SegmentTargetScale [model.NumSegments]float64`
  - `SegmentRefPrice [model.NumSegments]float64`
  - 區隔 0（消費者）鏡射舊 scalar：`SegmentWeights[0]==QualityWeights`、`SegmentTargetScale[0]==UserTargetPerAppeal`、`SegmentRefPrice[0]==RefPrice`。

- [ ] **Step 1: 寫失敗測試**

在 `internal/balance/balance_test.go` 末尾新增：
```go
func TestDefaultSegments(t *testing.T) {
	c := Default()
	// consumer(0) mirrors legacy scalars
	if c.SegmentWeights[model.SegConsumer] != c.QualityWeights {
		t.Errorf("consumer weights should mirror QualityWeights")
	}
	if c.SegmentTargetScale[model.SegConsumer] != c.UserTargetPerAppeal {
		t.Errorf("consumer scale should mirror UserTargetPerAppeal")
	}
	if c.SegmentRefPrice[model.SegConsumer] != c.RefPrice {
		t.Errorf("consumer ref price should mirror RefPrice")
	}
	// enterprise weights safety over capability
	ew := c.SegmentWeights[model.SegEnterprise]
	if ew[model.DimSafety] <= ew[model.DimCapability] {
		t.Errorf("enterprise should weight safety over capability: %+v", ew)
	}
	if c.SegmentRefPrice[model.SegEnterprise] != 180 || c.SegmentRefPrice[model.SegDeveloper] != 6 {
		t.Errorf("segment ref prices wrong: %+v", c.SegmentRefPrice)
	}
	// every segment's weights sum to 1
	for s, sw := range c.SegmentWeights {
		var sum float64
		for _, w := range sw {
			sum += w
		}
		if sum < 0.999 || sum > 1.001 {
			t.Errorf("segment %d weights sum = %v, want 1", s, sum)
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
	// Market segments (plan-05). Index 0 (consumer) mirrors the legacy scalars.
	SegmentWeights     [model.NumSegments][model.NumQualityDims]float64
	SegmentTargetScale [model.NumSegments]float64
	SegmentRefPrice    [model.NumSegments]float64
```

在 `Default()` 的 `return c` 之前加入：
```go
	c.SegmentWeights[model.SegConsumer] = qvec(0.4, 0.2, 0.2, 0.2)   // == QualityWeights
	c.SegmentWeights[model.SegEnterprise] = qvec(0.2, 0.1, 0.5, 0.2) // values safety
	c.SegmentWeights[model.SegDeveloper] = qvec(0.15, 0.4, 0.1, 0.35) // values efficiency+speed
	c.SegmentTargetScale = [model.NumSegments]float64{1000, 500, 800}
	c.SegmentRefPrice = [model.NumSegments]float64{12, 180, 6}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/balance/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/balance/balance.go internal/balance/balance_test.go
git commit -m "feat(balance): add per-segment weights, scale, ref price"
```

---

### Task 3: `advanceUsers` — 依區隔取權重 / 規模 / 參考價與對手競爭

**Files:**
- Modify: `internal/sim/sim.go`
- Test: `internal/sim/sim_test.go`

**Interfaces:**
- Consumes: `Model.Segment`；`balance.SegmentWeights` / `SegmentTargetScale` / `SegmentRefPrice`。
- Produces: `advanceUsers` 改為依 `m.Segment` 取 `w=SegmentWeights[seg]`、`scale=SegmentTargetScale[seg]`、`refPrice=SegmentRefPrice[seg]`；`appeal` 與 `rivalAppeal` 皆以該區隔權重 `w` 計算（對手在該區隔競爭）；`demandMult` 用該區隔參考價。區隔 0 + 無對手時等同 Plan 03/04。

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/sim_test.go` 末尾新增：
```go
func segModel(seg model.Segment, dim model.QualityDim, q, price float64) model.Model {
	m := model.Model{Online: true, Segment: seg, Price: price}
	m.Quality[dim] = q
	return m
}

func TestSegmentWeightsChangeAppeal(t *testing.T) {
	b := balance.Default()
	// A safety-only model earns more users in Enterprise (safety-weighted)
	// than in Consumer (capability-weighted), priced at each segment's ref price.
	consumer := segModel(model.SegConsumer, model.DimSafety, 50, b.SegmentRefPrice[model.SegConsumer])
	enterprise := segModel(model.SegEnterprise, model.DimSafety, 50, b.SegmentRefPrice[model.SegEnterprise])
	nc := Tick(model.GameState{Models: []model.Model{consumer}}, 1, nil, b)
	ne := Tick(model.GameState{Models: []model.Model{enterprise}}, 1, nil, b)
	if ne.Models[0].Users <= nc.Models[0].Users {
		t.Fatalf("enterprise safety users (%v) should exceed consumer (%v)",
			ne.Models[0].Users, nc.Models[0].Users)
	}
}

func TestSegmentRefPriceNeutralAtReference(t *testing.T) {
	b := balance.Default()
	// Priced exactly at the developer ref price → demandMult 1.
	// appeal = 40 (efficiency 100 * developer weight 0.4); target = 40*800*1*1 = 32000.
	dev := segModel(model.SegDeveloper, model.DimEfficiency, 100, b.SegmentRefPrice[model.SegDeveloper])
	ns := Tick(model.GameState{Models: []model.Model{dev}}, 1, nil, b)
	want := 40.0 * b.SegmentTargetScale[model.SegDeveloper] * b.UserGrowthRate // *1 tick
	if !approx(ns.Models[0].Users, want) {
		t.Fatalf("developer users = %v, want %v", ns.Models[0].Users, want)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/ -run TestSegment`
Expected: FAIL（區隔權重未生效——用戶數不隨區隔改變）。

- [ ] **Step 3: 寫最小實作**

把 `internal/sim/sim.go` 的 `advanceUsers` 改成（依 `m.Segment` 取 per-segment 參數；`rivalAppeal` 移入迴圈以該區隔權重計算）：
```go
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
		ns.Resources.Cash += m.Users * m.Price * dt / b.MonthSec

		w := b.SegmentWeights[m.Segment]
		appeal := appealOf(m.Quality, w)
		rivalAppeal := 0.0
		for _, c := range ns.Competitors {
			rivalAppeal += appealOf(c.Quality, w)
		}
		refPrice := b.SegmentRefPrice[m.Segment]
		var demandMult float64
		if m.Price > 0 {
			demandMult = math.Pow(refPrice/m.Price, b.PriceElasticity)
		}
		share := 1.0
		if appeal+rivalAppeal > 0 {
			share = appeal / (appeal + rivalAppeal)
		}
		target := appeal * b.SegmentTargetScale[m.Segment] * demandMult * share
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
Expected: PASS（全部——含 Plan 03/04：模型預設區隔 0 + 區隔 0 鏡射舊 scalar → 行為不變）。

- [ ] **Step 5: 跑整包 + vet + commit**

Run:
```bash
go test ./...
go vet ./...
```
Expected: 全部 PASS、vet 無警告。

```bash
git add internal/sim/sim.go internal/sim/sim_test.go
git commit -m "feat(sim): segment-specific appeal, scale, and pricing"
```

---

## 完成後狀態

三區隔市場成立：模型指定主攻區隔，各區隔以不同維度權重評價（消費者重能力/速度、企業重安全、開發者重成本/速度）、不同市場規模與參考價；對手在各區隔以該區隔權重競爭。定位策略（訓什麼維度、主打哪區隔、對抗誰）完整成形。區隔 0 + 無對手退化為既有行為（測試不變）。仍純確定性。

**下一步（Plan 06）**：推理算力池 + 服務器 / 機房 / 晶片自建——把「用戶產生推理負載、吃推理算力、算力不足則流失」與「租 vs 自建」的投入端加進來。
