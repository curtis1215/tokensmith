# Tokensmith Prototype — 最小可跑 TUI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 做一個單一進程、可實際啟動的 Bubble Tea TUI 原型，直接驅動既有純 sim（Plan 01–06）：資源即時推進、按鍵訓練模型 / 租算力、對手背景成長、畫面即時渲染。

**Architecture:** 不拆 daemon/IPC（單進程）。`internal/game.NewGame()` 產生初始 `GameState`（起始現金 / R&D / 研究員 / 對手 / 起始租用算力）。`internal/tui` 是 Bubble Tea `Model`：`Init` 起計時器；`Update` 於每個計時 tick 呼叫 `sim.Tick` 推進，並處理按鍵 → `sim.Apply`；`View` 用 Lipgloss 渲染儀表板。`main.go` 啟動程式。TUI/main 可用 `time`（非純 sim 層）。相依 Plan 01–06。

**Tech Stack:** Go 1.22+、Bubble Tea (`github.com/charmbracelet/bubbletea`)、Lipgloss (`github.com/charmbracelet/lipgloss`)。首次引入外部依賴。

## Global Constraints

- 延續 Plan 01–06；**但 `internal/tui`、`internal/game`、`main.go` 不受純 sim 約束**（可用 `time`、可有 I/O）。`internal/sim` 仍保持純。
- 訓練指令目前只產生消費者（Segment 0）模型（`StartTraining` 尚無 Segment 欄位——區隔選擇是後續小改）。
- prototype 速度：每個真實 tick 推進 `tickDT` 模擬秒（預設 3600，讓變化肉眼可見；可調）。

---

### Task 1: `internal/game` — 初始遊戲狀態

**Files:**
- Create: `internal/game/game.go`
- Test: `internal/game/game_test.go`

**Interfaces:**
- Consumes: `model`, `balance`（含 `balance.DefaultCompetitors`）。
- Produces: `game.NewGame() model.GameState`（起始：現金 100000、R&D 50000、2 名 T1 研究員、效率 1.0、7 家對手、租 4 訓練 GPU / 2 推理 GPU）。

- [ ] **Step 1: 寫失敗測試**

Create `internal/game/game_test.go`:
```go
package game

import (
	"testing"

	"tokensmith/internal/model"
)

func TestNewGameSeed(t *testing.T) {
	s := NewGame()
	if s.Resources.Cash <= 0 {
		t.Errorf("cash should be positive, got %v", s.Resources.Cash)
	}
	if s.Resources.RnD < 20000 {
		t.Errorf("R&D should cover a Gen1 train, got %v", s.Resources.RnD)
	}
	if len(s.Competitors) != 7 {
		t.Errorf("competitors = %d, want 7", len(s.Competitors))
	}
	if s.Research.Researchers[model.Tier1] == 0 || s.Research.EfficiencyMult == 0 {
		t.Errorf("research not seeded: %+v", s.Research)
	}
	if s.Compute.TrainingCapacity <= 0 || s.Compute.InferenceCapacity <= 0 {
		t.Errorf("compute not seeded: %+v", s.Compute)
	}
}
```

- [ ] **Step 2: 執行測試確認失敗**

Run: `go test ./internal/game/`
Expected: FAIL（`undefined: NewGame`）。

- [ ] **Step 3: 寫最小實作**

Create `internal/game/game.go`:
```go
// Package game seeds a fresh GameState for a new run.
package game

import (
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// NewGame returns the initial state for a new run.
func NewGame() model.GameState {
	var s model.GameState
	s.Resources.Cash = 100000
	s.Resources.RnD = 50000
	s.Research.EfficiencyMult = 1.0
	s.Research.Researchers[model.Tier1] = 2
	s.Competitors = balance.DefaultCompetitors()
	s.Compute.TrainingCapacity = 4
	s.Compute.InferenceCapacity = 2
	return s
}
```

- [ ] **Step 4: 執行測試確認通過**

Run: `go test ./internal/game/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/game/game.go internal/game/game_test.go
git commit -m "feat(game): seed initial GameState for a new run"
```

---

### Task 2: `internal/tui` — Bubble Tea Model（含依賴）

**Files:**
- Modify: `go.mod`, `go.sum`
- Create: `internal/tui/tui.go`
- Test: `internal/tui/tui_test.go`

**Interfaces:**
- Consumes: `game.NewGame`, `sim.Tick`, `sim.Apply`, `model.*`, `balance.Default`；Bubble Tea / Lipgloss。
- Produces:
  - `tui.New() Model`
  - `Model` 實作 `tea.Model`（`Init() tea.Cmd`、`Update(tea.Msg) (tea.Model, tea.Cmd)`、`View() string`）
  - Update：`tickMsg` → `sim.Tick(state, tickDT, nil, cfg)` 並重排下一 tick；按鍵 `t`=訓練 Gen1 消費者模型、`r`=+1 訓練算力、`i`=+1 推理算力、`q`/`ctrl+c`=離開。

- [ ] **Step 1: 加入依賴**

Run:
```bash
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/lipgloss@latest
go mod tidy
```
Expected: `go.mod` 出現兩個 require；`go.sum` 更新。

- [ ] **Step 2: 寫失敗測試**

Create `internal/tui/tui_test.go`:
```go
package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestUpdateTickAdvancesState(t *testing.T) {
	m := New()
	before := m.state.GameTime
	nm, _ := m.Update(tickMsg(time.Unix(0, 0)))
	if nm.(Model).state.GameTime <= before {
		t.Fatalf("tick did not advance GameTime")
	}
}

func TestTrainKeyStartsTraining(t *testing.T) {
	m := New() // seeded with enough R&D + training capacity
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if !nm.(Model).state.HasTraining {
		t.Fatalf("train key did not start training")
	}
}

func TestRentKeysAddCapacity(t *testing.T) {
	m := New()
	beforeT := m.state.Compute.TrainingCapacity
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if nm.(Model).state.Compute.TrainingCapacity != beforeT+1 {
		t.Fatalf("rent-training key did not add capacity")
	}
}

func TestViewNonEmpty(t *testing.T) {
	if New().View() == "" {
		t.Fatalf("View is empty")
	}
}

func TestQuitKey(t *testing.T) {
	m := New()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatalf("quit key should return a command")
	}
}
```

- [ ] **Step 3: 執行測試確認失敗**

Run: `go test ./internal/tui/`
Expected: FAIL（`undefined: New` / `tickMsg`）。

- [ ] **Step 4: 寫最小實作**

Create `internal/tui/tui.go`:
```go
// Package tui is the single-process Bubble Tea prototype front-end.
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"tokensmith/internal/balance"
	"tokensmith/internal/game"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

// tickDT is how many simulated seconds each real tick advances.
const tickDT = 3600.0

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Model is the Bubble Tea root model.
type Model struct {
	state model.GameState
	cfg   balance.Config
}

// New returns a fresh prototype model.
func New() Model {
	return Model{state: game.NewGame(), cfg: balance.Default()}
}

func (m Model) Init() tea.Cmd { return tick() }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		m.state = sim.Tick(m.state, tickDT, nil, m.cfg)
		return m, tick()
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "t":
			cmd := model.StartTraining{
				Gen:   1,
				Alloc: [model.NumQualityDims]float64{0.4, 0.2, 0.2, 0.2},
				Price: m.cfg.SegmentRefPrice[model.SegConsumer],
			}
			if ns, err := sim.Apply(m.state, cmd, m.cfg); err == nil {
				m.state = ns
			}
		case "r":
			if ns, err := sim.Apply(m.state, model.RentTrainingCompute{Delta: 1}, m.cfg); err == nil {
				m.state = ns
			}
		case "i":
			if ns, err := sim.Apply(m.state, model.RentInferenceCompute{Delta: 1}, m.cfg); err == nil {
				m.state = ns
			}
		}
	}
	return m, nil
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	boxStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	helpStyle  = lipgloss.NewStyle().Faint(true)
)

func (m Model) View() string {
	s := m.state
	res := fmt.Sprintf("💰 $%.0f    ⚡ R&D %.0f    🖥 訓練 %.0f · 推理 %.1f/%.0f",
		s.Resources.Cash, s.Resources.RnD,
		s.Compute.TrainingCapacity, s.Compute.InferenceLoad, s.Compute.InferenceCapacity)

	var mb strings.Builder
	mb.WriteString("模型:\n")
	if s.HasTraining {
		mb.WriteString(fmt.Sprintf("  訓練中 Gen%d  剩 %.0f GPU·s\n", s.Training.Gen, s.Training.WorkRemaining))
	}
	for _, md := range s.Models {
		mb.WriteString(fmt.Sprintf("  Gen%d  用戶 %.0f  價 $%.0f  能力 %.0f\n",
			md.Gen, md.Users, md.Price, md.Quality[model.DimCapability]))
	}
	if len(s.Models) == 0 && !s.HasTraining {
		mb.WriteString("  (無 — 按 t 訓練第一個模型)\n")
	}

	var cb strings.Builder
	cb.WriteString("對手 (能力):\n")
	for _, c := range s.Competitors {
		cb.WriteString(fmt.Sprintf("  %-10s %.1f\n", c.Name, c.Quality[model.DimCapability]))
	}

	help := helpStyle.Render("[t]訓練  [r]+訓練算力  [i]+推理算力  [q]離開")
	body := lipgloss.JoinVertical(lipgloss.Left, res, "", mb.String(), cb.String(), help)
	return boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, titleStyle.Render("Tokensmith"), body))
}
```

- [ ] **Step 5: 執行測試確認通過**

Run: `go test ./internal/tui/`
Expected: PASS（5 個測試）。

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/tui/tui.go internal/tui/tui_test.go
git commit -m "feat(tui): single-process Bubble Tea prototype"
```

---

### Task 3: `main.go` + 建置驗證

**Files:**
- Create: `main.go`

**Interfaces:**
- Consumes: `tui.New`；Bubble Tea。
- Produces: 可執行入口——啟動 alt-screen Bubble Tea 程式。

- [ ] **Step 1: 寫入 main.go**

Create `main.go`:
```go
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"tokensmith/internal/tui"
)

func main() {
	p := tea.NewProgram(tui.New(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "tokensmith error:", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: 建置與整包測試**

Run:
```bash
go build -o tokensmith .
go test ./...
go vet ./...
```
Expected: 產生 `tokensmith` 執行檔；全部測試 PASS；vet 無警告。

- [ ] **Step 3: 無頭 smoke 驗證（不需互動 TTY）**

Run:
```bash
go build ./...
```
Expected: 編譯成功（TUI 的實際渲染由使用者 / 協調者在真實終端機啟動 `./tokensmith` 驗證；此步僅確認可建置）。

- [ ] **Step 4: Commit**

```bash
git add main.go
git commit -m "feat: tokensmith prototype entry point"
```

---

## 完成後狀態

`./tokensmith` 可啟動一個活的 TUI：資源隨 tick 跳動、對手能力背景上升、按 `t` 訓練模型（完成後上線長用戶產生營收）、`r`/`i` 加租算力、`q` 離開。這是把 Plan 01–06 純 sim 接上可視前端的第一個可玩原型（單進程）。

**後續**：Plan 07 自建算力、08 團隊/科技樹、09 里程碑/prestige 補完經濟；再由 Plan 10–13（store / ingest / daemon+IPC / 完整六頁 TUI）升級為 spec 的常駐 daemon + 前景 TUI 架構，並接上真實 token 採集。
