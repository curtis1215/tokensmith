# Tokensmith 01 — Foundation + Sim R&D 核心 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立 Go 專案地基與純確定性的 Sim 燃料迴圈——由研發人力（每秒）與真實 token 事件產生 R&D，含每日軟帽與離線快轉線性等價性。

**Architecture:** 三個無 I/O 的套件：`internal/model`（共用型別）、`internal/balance`（v0 數值）、`internal/sim`（純函式 `Tick`）。`Tick` 不碰時鐘、不用亂數、不做 I/O；時間只經 `dt` 參數推進，確保可完全單元測試與離線重放。對應設計 spec §3、§5、§6.2、§12。

**Tech Stack:** Go 1.22+、標準 `testing`。此 plan 無任何外部依賴。

## Global Constraints

- Module path：`tokensmith`（本地優先；日後推遠端可改為 `github.com/curtis1215/tokensmith`）。所有內部 import 以 `tokensmith/internal/...` 起頭。
- Go 版本下限：1.22。
- `internal/sim` MUST 為純函式：不得使用 wall-clock、`math/rand`、檔案 / 網路 I/O；時間僅由 `dt float64`（秒）推進（spec §3.4、§5.2）。
- 為確定性，所有依 tier 分級的資料用**固定陣列 `[4]T` 以 `StaffTier` 為索引**，不用 map（避免 map 迭代亂序）。
- 套件相依 DAG：`model`（無依賴）← `balance`（依 `model`）← `sim`（依 `model` + `balance`）。不得產生反向或循環依賴。
- v0 數值一律放 `internal/balance`，數字**逐字**取自 spec §12 v0 標定表：研究員 T1/T2/T3 = 5/15/40 R&D/秒；token→R&D = `(input + 2×output) / 10`；每日軟帽 200k 全額、超過 ×0.3；軟帽窗口 = 86400 秒（1 real 天）。

---

### Task 1: Go module + `internal/model` 核心型別

**Files:**
- Create: `go.mod`
- Create: `internal/model/types.go`
- Test: `internal/model/types_test.go`

**Interfaces:**
- Consumes: 無（起點）。
- Produces:
  - `model.StaffTier`（`int`）與常數 `TierNone=0, Tier1=1, Tier2=2, Tier3=3`，`NumTiers=4`。
  - `model.Resources{ RnD, Cash float64 }`
  - `model.Research{ Researchers [4]int; EfficiencyMult float64 }`（`Researchers` 以 `StaffTier` 為索引）
  - `model.TokenEvent{ Source string; Timestamp time.Time; InputTokens, OutputTokens int }`
  - `model.GameState{ GameTime float64; Resources Resources; Research Research; WindowRnD, WindowElapsed float64 }`

- [ ] **Step 1: 初始化 module**

Run:
```bash
cd /Users/curtis/Dev/Project_AI_Factory
go mod init tokensmith
```
Expected: 產生 `go.mod`，內容含 `module tokensmith` 與 `go 1.22`（或更高）。

- [ ] **Step 2: 寫失敗測試**

Create `internal/model/types.go`（先放空 package 讓測試可編譯前的紅燈明確）：
```go
package model
```

Create `internal/model/types_test.go`:
```go
package model

import (
	"testing"
	"time"
)

func TestResearchIndexedByTier(t *testing.T) {
	r := Research{EfficiencyMult: 1.0}
	r.Researchers[Tier1] = 2
	r.Researchers[Tier3] = 1
	if r.Researchers[Tier1] != 2 || r.Researchers[Tier3] != 1 {
		t.Fatalf("tier indexing wrong: %+v", r.Researchers)
	}
	if NumTiers != 4 {
		t.Fatalf("NumTiers = %d, want 4", NumTiers)
	}
}

func TestGameStateZeroValue(t *testing.T) {
	var s GameState
	if s.GameTime != 0 || s.Resources.RnD != 0 || s.WindowRnD != 0 {
		t.Fatalf("zero GameState not zero: %+v", s)
	}
}

func TestTokenEventFields(t *testing.T) {
	e := TokenEvent{Source: "claude-code", Timestamp: time.Unix(0, 0), InputTokens: 100, OutputTokens: 50}
	if e.Source != "claude-code" || e.InputTokens != 100 || e.OutputTokens != 50 {
		t.Fatalf("token event fields wrong: %+v", e)
	}
}
```

- [ ] **Step 3: 執行測試確認失敗**

Run: `go test ./internal/model/`
Expected: FAIL（`undefined: Research`、`undefined: Tier1` 等）。

- [ ] **Step 4: 寫最小實作**

Replace `internal/model/types.go`:
```go
// Package model holds the shared value types used across the simulation.
// It has no dependencies and performs no I/O.
package model

import "time"

// StaffTier is a researcher skill tier. Values double as array indices.
type StaffTier int

const (
	TierNone StaffTier = iota // 0 — unused slot / no staff
	Tier1                     // 1
	Tier2                     // 2
	Tier3                     // 3
	NumTiers = 4              // size of tier-indexed arrays
)

// Resources are the fungible currencies the player accumulates.
type Resources struct {
	RnD  float64
	Cash float64
}

// Research is the R&D-generating workforce.
// Researchers is indexed by StaffTier (index 0 unused).
type Research struct {
	Researchers    [NumTiers]int
	EfficiencyMult float64 // infra bonus; 1.0 = no bonus
}

// TokenEvent is a normalized real-world AI-tool usage event.
type TokenEvent struct {
	Source       string
	Timestamp    time.Time
	InputTokens  int
	OutputTokens int
}

// GameState is the full simulation state (plan-01 subset).
// GameTime and WindowElapsed are in seconds.
type GameState struct {
	GameTime      float64
	Resources     Resources
	Research      Research
	WindowRnD     float64 // token-sourced R&D accrued in the current soft-cap window
	WindowElapsed float64 // seconds elapsed in the current soft-cap window
}
```

- [ ] **Step 5: 執行測試確認通過**

Run: `go test ./internal/model/`
Expected: PASS（3 個測試）。

- [ ] **Step 6: Commit**

```bash
git add go.mod internal/model/types.go internal/model/types_test.go
git commit -m "feat(model): add core simulation value types"
```

---

### Task 2: `internal/balance` v0 數值

**Files:**
- Create: `internal/balance/balance.go`
- Test: `internal/balance/balance_test.go`

**Interfaces:**
- Consumes: `model.StaffTier`, `model.NumTiers`（Task 1）。
- Produces:
  - `balance.Config` 結構，欄位：
    - `ResearcherRnDPerSec [model.NumTiers]float64`（以 `StaffTier` 為索引）
    - `TokenInputWeight, TokenOutputWeight, TokenDivisor float64`
    - `SoftCapFull, SoftCapMult, SoftCapWindowSec float64`
  - `balance.Default() Config`（回傳 v0 標定值）

- [ ] **Step 1: 寫失敗測試**

Create `internal/balance/balance_test.go`:
```go
package balance

import (
	"testing"

	"tokensmith/internal/model"
)

func TestDefaultV0Values(t *testing.T) {
	c := Default()
	if c.ResearcherRnDPerSec[model.Tier1] != 5 {
		t.Errorf("Tier1 R&D/s = %v, want 5", c.ResearcherRnDPerSec[model.Tier1])
	}
	if c.ResearcherRnDPerSec[model.Tier2] != 15 {
		t.Errorf("Tier2 R&D/s = %v, want 15", c.ResearcherRnDPerSec[model.Tier2])
	}
	if c.ResearcherRnDPerSec[model.Tier3] != 40 {
		t.Errorf("Tier3 R&D/s = %v, want 40", c.ResearcherRnDPerSec[model.Tier3])
	}
	if c.TokenInputWeight != 1 || c.TokenOutputWeight != 2 || c.TokenDivisor != 10 {
		t.Errorf("token formula params wrong: %+v", c)
	}
	if c.SoftCapFull != 200000 || c.SoftCapMult != 0.3 || c.SoftCapWindowSec != 86400 {
		t.Errorf("soft cap params wrong: %+v", c)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/balance/`
Expected: FAIL（`undefined: Default`）。

- [ ] **Step 3: 寫最小實作**

Create `internal/balance/balance.go`:
```go
// Package balance holds all tunable v0 numbers, copied verbatim from
// design spec §12. Keeping them in one place makes tuning easy.
package balance

import "tokensmith/internal/model"

// Config is the full set of balance knobs (plan-01 subset).
type Config struct {
	// ResearcherRnDPerSec is R&D produced per second per researcher, by tier.
	ResearcherRnDPerSec [model.NumTiers]float64

	// Token → R&D: (input*InputWeight + output*OutputWeight) / Divisor.
	TokenInputWeight  float64
	TokenOutputWeight float64
	TokenDivisor      float64

	// Daily soft cap on token-sourced R&D within a rolling window.
	SoftCapFull      float64 // R&D granted at full rate before diminishing
	SoftCapMult      float64 // multiplier applied beyond SoftCapFull
	SoftCapWindowSec float64 // window length in seconds
}

// Default returns the v0 calibration (spec §12).
func Default() Config {
	var c Config
	c.ResearcherRnDPerSec[model.Tier1] = 5
	c.ResearcherRnDPerSec[model.Tier2] = 15
	c.ResearcherRnDPerSec[model.Tier3] = 40

	c.TokenInputWeight = 1
	c.TokenOutputWeight = 2
	c.TokenDivisor = 10

	c.SoftCapFull = 200000
	c.SoftCapMult = 0.3
	c.SoftCapWindowSec = 86400
	return c
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/balance/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/balance/balance.go internal/balance/balance_test.go
git commit -m "feat(balance): add v0 calibration config"
```

---

### Task 3: `Tick` — 研發人力每秒產出 R&D

**Files:**
- Create: `internal/sim/sim.go`
- Test: `internal/sim/sim_test.go`

**Interfaces:**
- Consumes: `model.GameState`, `model.Research`, `model.TokenEvent`（Task 1）；`balance.Config`（Task 2）。
- Produces:
  - `sim.staffRnDPerSec(r model.Research, b balance.Config) float64`（每秒研發人力產出，未乘 dt）
  - `sim.Tick(s model.GameState, dt float64, events []model.TokenEvent, b balance.Config) model.GameState`（本 task 僅實作研發人力 + 推進 GameTime；`events` 先忽略，Task 4 接手）

- [ ] **Step 1: 寫失敗測試**

Create `internal/sim/sim_test.go`:
```go
package sim

import (
	"math"
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestStaffRnDPerSec(t *testing.T) {
	b := balance.Default()
	r := model.Research{EfficiencyMult: 1.0}
	r.Researchers[model.Tier1] = 2 // 2*5 = 10
	r.Researchers[model.Tier2] = 1 // 1*15 = 15
	got := staffRnDPerSec(r, b)     // 25/s
	if !approx(got, 25) {
		t.Fatalf("staffRnDPerSec = %v, want 25", got)
	}
}

func TestStaffRnDEfficiencyMult(t *testing.T) {
	b := balance.Default()
	r := model.Research{EfficiencyMult: 2.0}
	r.Researchers[model.Tier2] = 1 // 15 * 2.0 = 30
	if got := staffRnDPerSec(r, b); !approx(got, 30) {
		t.Fatalf("staffRnDPerSec with mult = %v, want 30", got)
	}
}

func TestTickAddsStaffRnDAndAdvancesTime(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Research: model.Research{EfficiencyMult: 1.0}}
	s.Research.Researchers[model.Tier2] = 4 // 60/s
	ns := Tick(s, 10, nil, b)               // 60/s * 10s = 600
	if !approx(ns.Resources.RnD, 600) {
		t.Fatalf("RnD = %v, want 600", ns.Resources.RnD)
	}
	if !approx(ns.GameTime, 10) {
		t.Fatalf("GameTime = %v, want 10", ns.GameTime)
	}
	// Tick must not mutate the input state.
	if s.Resources.RnD != 0 || s.GameTime != 0 {
		t.Fatalf("Tick mutated input: %+v", s)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/`
Expected: FAIL（`undefined: staffRnDPerSec`、`undefined: Tick`）。

- [ ] **Step 3: 寫最小實作**

Create `internal/sim/sim.go`:
```go
// Package sim is the pure, deterministic simulation core.
// No wall-clock, no randomness, no I/O — time advances only via dt.
package sim

import (
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// staffRnDPerSec returns R&D produced per second by the research workforce,
// before multiplying by dt.
func staffRnDPerSec(r model.Research, b balance.Config) float64 {
	var perSec float64
	for tier := model.Tier1; tier <= model.Tier3; tier++ {
		perSec += float64(r.Researchers[tier]) * b.ResearcherRnDPerSec[tier]
	}
	return perSec * r.EfficiencyMult
}

// Tick advances the simulation by dt seconds and returns the new state.
// Pure: it does not mutate s and depends only on its arguments.
func Tick(s model.GameState, dt float64, events []model.TokenEvent, b balance.Config) model.GameState {
	ns := s
	ns.GameTime += dt
	ns.Resources.RnD += staffRnDPerSec(s.Research, b) * dt
	return ns
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/`
Expected: PASS（3 個測試）。

- [ ] **Step 5: Commit**

```bash
git add internal/sim/sim.go internal/sim/sim_test.go
git commit -m "feat(sim): add Tick with staff R&D generation"
```

---

### Task 4: `Tick` — token 燃料轉 R&D（尚未套軟帽）

**Files:**
- Modify: `internal/sim/sim.go`
- Test: `internal/sim/sim_test.go`（新增測試函式）

**Interfaces:**
- Consumes: 同 Task 3。
- Produces:
  - `sim.tokenRawRnD(events []model.TokenEvent, b balance.Config) float64`（本批事件的原始 token R&D，未套軟帽）
  - `Tick` 擴充：把 `tokenRawRnD` 的結果加入 `Resources.RnD`（此 task 直接全額加入；軟帽於 Task 5 套用）

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/sim_test.go` 末尾新增：
```go
func TestTokenRawRnD(t *testing.T) {
	b := balance.Default()
	events := []model.TokenEvent{
		{InputTokens: 1000, OutputTokens: 500},  // (1000 + 2*500)/10 = 200
		{InputTokens: 0, OutputTokens: 1000},    // (0 + 2000)/10   = 200
	}
	if got := tokenRawRnD(events, b); !approx(got, 400) {
		t.Fatalf("tokenRawRnD = %v, want 400", got)
	}
}

func TestTokenRawRnDEmpty(t *testing.T) {
	if got := tokenRawRnD(nil, balance.Default()); got != 0 {
		t.Fatalf("tokenRawRnD(nil) = %v, want 0", got)
	}
}

func TestTickAddsTokenRnD(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Research: model.Research{EfficiencyMult: 1.0}}
	// no staff → only token R&D. 1000 output → (2000)/10 = 200.
	events := []model.TokenEvent{{OutputTokens: 1000}}
	ns := Tick(s, 1, events, b)
	if !approx(ns.Resources.RnD, 200) {
		t.Fatalf("RnD = %v, want 200", ns.Resources.RnD)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/`
Expected: FAIL（`undefined: tokenRawRnD`；`TestTickAddsTokenRnD` 得 0）。

- [ ] **Step 3: 寫最小實作**

在 `internal/sim/sim.go` 新增 `tokenRawRnD`，並修改 `Tick`：
```go
// tokenRawRnD returns the raw R&D produced by a batch of token events,
// before any soft-cap diminishing is applied.
func tokenRawRnD(events []model.TokenEvent, b balance.Config) float64 {
	var raw float64
	for _, e := range events {
		raw += (float64(e.InputTokens)*b.TokenInputWeight + float64(e.OutputTokens)*b.TokenOutputWeight) / b.TokenDivisor
	}
	return raw
}
```

把 `Tick` 改成：
```go
func Tick(s model.GameState, dt float64, events []model.TokenEvent, b balance.Config) model.GameState {
	ns := s
	ns.GameTime += dt
	staffRnD := staffRnDPerSec(s.Research, b) * dt
	tokenRnD := tokenRawRnD(events, b)
	ns.Resources.RnD += staffRnD + tokenRnD
	return ns
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/`
Expected: PASS（全部，含 Task 3 既有測試）。

- [ ] **Step 5: Commit**

```bash
git add internal/sim/sim.go internal/sim/sim_test.go
git commit -m "feat(sim): add token-fuel R&D to Tick"
```

---

### Task 5: `Tick` — 每日軟帽 / 遞減

**Files:**
- Modify: `internal/sim/sim.go`
- Test: `internal/sim/sim_test.go`（新增測試函式）

**Interfaces:**
- Consumes: 同上。
- Produces:
  - `sim.applySoftCap(windowRnD, raw, full, mult float64) (effective, newWindow float64)`（純函式：在 `windowRnD` 基礎上對 `raw` 套軟帽，回傳有效 R&D 與更新後的窗口累計）
  - `Tick` 擴充：token R&D 經 `applySoftCap` 後才加入；並維護 `WindowRnD` / `WindowElapsed`，跨窗口（`WindowElapsed >= SoftCapWindowSec`）時重置窗口累計。

- [ ] **Step 1: 寫失敗測試**

在 `internal/sim/sim_test.go` 末尾新增：
```go
func TestApplySoftCapBelowFull(t *testing.T) {
	eff, nw := applySoftCap(0, 1000, 200000, 0.3)
	if !approx(eff, 1000) || !approx(nw, 1000) {
		t.Fatalf("below full: eff=%v nw=%v, want 1000/1000", eff, nw)
	}
}

func TestApplySoftCapCrossingFull(t *testing.T) {
	// window at 199,000; raw 2,000 → 1,000 full + 1,000*0.3 = 1,300 effective
	eff, nw := applySoftCap(199000, 2000, 200000, 0.3)
	if !approx(eff, 1300) {
		t.Fatalf("crossing: eff=%v, want 1300", eff)
	}
	if !approx(nw, 201000) {
		t.Fatalf("crossing: nw=%v, want 201000", nw)
	}
}

func TestApplySoftCapAboveFull(t *testing.T) {
	// already above full → everything diminished
	eff, nw := applySoftCap(200000, 1000, 200000, 0.3)
	if !approx(eff, 300) || !approx(nw, 201000) {
		t.Fatalf("above: eff=%v nw=%v, want 300/201000", eff, nw)
	}
}

func TestTickSoftCapAccumulatesWindow(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Research: model.Research{EfficiencyMult: 1.0}}
	// event that yields raw 199000 R&D: output = 199000*10/2 = 995000
	ev1 := []model.TokenEvent{{OutputTokens: 995000}}
	s = Tick(s, 1, ev1, b)
	if !approx(s.WindowRnD, 199000) {
		t.Fatalf("WindowRnD after ev1 = %v, want 199000", s.WindowRnD)
	}
	// next raw 2000 (output 10000) → 1300 effective (1000 full + 300)
	before := s.Resources.RnD
	s = Tick(s, 1, []model.TokenEvent{{OutputTokens: 10000}}, b)
	if !approx(s.Resources.RnD-before, 1300) {
		t.Fatalf("effective token R&D = %v, want 1300", s.Resources.RnD-before)
	}
}

func TestTickWindowResets(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Research: model.Research{EfficiencyMult: 1.0}, WindowRnD: 199000, WindowElapsed: 86399}
	// advancing past the 86400s window boundary resets WindowRnD to 0,
	// so the next tokens are granted at full rate again.
	before := s.Resources.RnD
	s = Tick(s, 2, []model.TokenEvent{{OutputTokens: 10000}}, b) // raw 2000
	if !approx(s.WindowRnD, 2000) {
		t.Fatalf("WindowRnD after reset = %v, want 2000", s.WindowRnD)
	}
	if !approx(s.Resources.RnD-before, 2000) {
		t.Fatalf("token R&D after reset = %v, want 2000 (full rate)", s.Resources.RnD-before)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/sim/`
Expected: FAIL（`undefined: applySoftCap`；window 相關測試失敗）。

- [ ] **Step 3: 寫最小實作**

在 `internal/sim/sim.go` 新增 `applySoftCap`（放在 `tokenRawRnD` 之後）：
```go
// applySoftCap diminishes raw token R&D once cumulative window R&D passes full.
// Returns the effective R&D to grant and the updated window cumulative.
func applySoftCap(windowRnD, raw, full, mult float64) (effective, newWindow float64) {
	newWindow = windowRnD + raw
	if windowRnD >= full {
		return raw * mult, newWindow
	}
	remainingFull := full - windowRnD
	if raw <= remainingFull {
		return raw, newWindow
	}
	over := raw - remainingFull
	return remainingFull + over*mult, newWindow
}
```

把 `Tick` 改成（先處理窗口重置，再套軟帽）：
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
	return ns
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/sim/`
Expected: PASS（全部）。

- [ ] **Step 5: Commit**

```bash
git add internal/sim/sim.go internal/sim/sim_test.go
git commit -m "feat(sim): add daily soft cap to token R&D"
```

---

### Task 6: 研發人力 R&D 的離線快轉線性等價性（property test）

**Files:**
- Test: `internal/sim/sim_test.go`（新增測試函式）

**Interfaces:**
- Consumes: `sim.Tick`（Task 3–5）。
- Produces: 無新增產出型別；僅新增回歸保證——在**無 token 事件**下，研發人力 R&D 對 dt 為線性，故一次 `Tick(dt)` 等於 N 次 `Tick(dt/N)`。這是離線結算「快轉 == 逐步」的基礎（spec §5.3）；含 token/軟帽的完整子步結算屬 Plan 05。

- [ ] **Step 1: 寫失敗測試（先以錯誤期望值確認測試有效）**

在 `internal/sim/sim_test.go` 末尾新增：
```go
func TestOfflineFastForwardEquivalenceStaffOnly(t *testing.T) {
	b := balance.Default()
	base := model.GameState{Research: model.Research{EfficiencyMult: 1.5}}
	base.Research.Researchers[model.Tier1] = 3
	base.Research.Researchers[model.Tier3] = 2

	// One big tick of 100s, no token events.
	oneShot := Tick(base, 100, nil, b)

	// 100 small ticks of 1s each.
	stepwise := base
	for i := 0; i < 100; i++ {
		stepwise = Tick(stepwise, 1, nil, b)
	}

	if !approx(oneShot.Resources.RnD, stepwise.Resources.RnD) {
		t.Fatalf("fast-forward mismatch: oneShot=%v stepwise=%v",
			oneShot.Resources.RnD, stepwise.Resources.RnD)
	}
	if !approx(oneShot.GameTime, stepwise.GameTime) {
		t.Fatalf("GameTime mismatch: oneShot=%v stepwise=%v",
			oneShot.GameTime, stepwise.GameTime)
	}
	if !approx(oneShot.Resources.RnD, 14250) { // (3*5 + 2*40)*1.5 = 142.5/s * 100s = 14250
		t.Fatalf("expected RnD 14250, got %v", oneShot.Resources.RnD)
	}
}
```

- [ ] **Step 2: 執行測試確認通過**

Run: `go test ./internal/sim/ -run TestOfflineFastForwardEquivalenceStaffOnly -v`
Expected: PASS（研發人力對 dt 線性，兩路徑相等且等於 14250）。

> 註：此 task 是純測試（無實作變更），驗證既有 `Tick` 已滿足線性等價性。若失敗，代表 Task 3 的 `Tick` 引入了非線性（不該發生）——回頭檢查 `staffRnDPerSec * dt`。

- [ ] **Step 3: 跑整包測試 + vet**

Run:
```bash
go test ./...
go vet ./...
```
Expected: 全部 PASS、vet 無警告。

- [ ] **Step 4: Commit**

```bash
git add internal/sim/sim_test.go
git commit -m "test(sim): add offline fast-forward equivalence property"
```

---

## 完成後狀態

此 plan 完成後，`tokensmith` 有一個可完全單元測試的純確定性 R&D 燃料迴圈：研發人力每秒產出 + token 事件灌注 + 每日軟帽 + 離線快轉線性等價。無 daemon / TUI / store / ingest——那些在 Plan 02–06 疊加。

**下一步（Plan 02）**：在此 `GameState` / `Tick` 上擴充模型訓練、算力、用戶 / 市場 / 對手、收入與里程碑。
