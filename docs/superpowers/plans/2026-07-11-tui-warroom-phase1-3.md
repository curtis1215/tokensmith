# Tokensmith TUI 作戰室改造（Phase 1-3）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 Tokensmith TUI 從單色靜態畫面升級為「AI 作戰室 HUD」：完整調色盤、卡片變體、彩色進度條、等寬 grid、sparkline 趨勢、漲跌色、三級慶祝系統與決勝守成 UI。

**Architecture:** 全部改動都在表現層（`internal/tui/`）：`theme.go` 提供調色盤與卡片變體，`spark.go` 提供 ring buffer 趨勢圖，`feedback.go` 提供 tick 前後 state 比對的時刻偵測與三級慶祝（Minor=notice、Major=金色橫幅佇列、Epic=全螢幕 overlay）。sim 層唯讀，永不寫回。

**Tech Stack:** Go 1.25、charmbracelet bubbletea v1.3.10、lipgloss v1.1.0（既有依賴，禁止新增）。

**Spec:** `docs/superpowers/specs/2026-07-11-tui-warroom-gamification-design.md`

## Global Constraints

- 零新依賴：只用 go.mod 既有套件。
- 顏色一律用 hex 的 `lipgloss.Color("#RRGGBB")`；lipgloss 的 colorprofile 會在 256/16 色終端自動降級，不需要手動 fallback。
- 測試環境無 TTY，lipgloss 會剝掉所有 ANSI 色碼——**測試只斷言純文字內容與寬度，不斷言色碼**。
- 表現層原則：新程式碼只讀 `model.GameState`，永不呼叫 `sim.Apply` 以外的變異路徑；`detectMoments` 是純函式。
- 所有玩家可見文案為繁體中文。
- 動畫節奏沿用既有 250ms tick（`tickInterval`），不新增計時器。
- 寬度計算一律用 `lipgloss.Width`（CJK-aware），不用 `len()`。
- 每個 commit 訊息尾端加上：`Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`
- 測試指令：`go test ./internal/tui/`；全量迴歸 `go test ./...`。
- 既有測試若因視覺升級斷言過時（如 `▓` 字元、舊版面字串），在該任務內一併更新斷言，不可跳過。

---

### Task 1: theme.go 調色盤與卡片變體

**Files:**
- Create: `internal/tui/theme.go`
- Modify: `internal/tui/style.go`（rebind 舊樣式名到新色）
- Modify: `internal/tui/layout.go:10-13`（Card 委派給 CardIn）
- Test: `internal/tui/theme_test.go`

**Interfaces:**
- Consumes: 無（純新增）。
- Produces:
  - `type CardKind int`；常數 `CardDefault, CardAccent, CardThreat, CardGold`
  - `func CardIn(kind CardKind, width int, title, body string) string` — `width > 0` 時強制整卡總寬（含邊框）為 width
  - 調色盤變數 `colorCyan, colorPurple, colorGain, colorLoss, colorAmber, colorGold, colorDim lipgloss.Color`
  - 樣式變數 `styleCyan, stylePurple, styleGain, styleLoss, styleAmber, styleGold lipgloss.Style`
  - 既有 `Card(title, body string) string` 行為不變（= `CardIn(CardDefault, 0, ...)`）

- [ ] **Step 1: 寫失敗測試**

```go
// internal/tui/theme_test.go
package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestCardInForcesWidth(t *testing.T) {
	a := CardIn(CardDefault, 40, "短", "內容")
	b := CardIn(CardAccent, 40, "很長的標題喔喔喔", "不同長度的內容行")
	if lipgloss.Width(a) != 40 {
		t.Fatalf("CardDefault width = %d, want 40", lipgloss.Width(a))
	}
	if lipgloss.Width(b) != 40 {
		t.Fatalf("CardAccent width = %d, want 40", lipgloss.Width(b))
	}
}

func TestCardInAutoWidthWhenZero(t *testing.T) {
	got := CardIn(CardDefault, 0, "標題", "行")
	if lipgloss.Width(got) >= 40 {
		t.Fatalf("auto width should shrink to content, got %d", lipgloss.Width(got))
	}
	if !strings.Contains(got, "標題") {
		t.Fatalf("missing title: %q", got)
	}
}

func TestCardVariantsRenderTitleAndBody(t *testing.T) {
	for _, k := range []CardKind{CardDefault, CardAccent, CardThreat, CardGold} {
		got := CardIn(k, 0, "T", "B")
		if !strings.Contains(got, "T") || !strings.Contains(got, "B") {
			t.Fatalf("kind %d missing content: %q", k, got)
		}
	}
}

func TestCardBackwardCompatible(t *testing.T) {
	if Card("標題", "內容") != CardIn(CardDefault, 0, "標題", "內容") {
		t.Fatal("Card must delegate to CardIn(CardDefault, 0, ...)")
	}
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/tui/ -run 'TestCard' -v`
Expected: FAIL — `undefined: CardIn`

- [ ] **Step 3: 實作 theme.go**

```go
// internal/tui/theme.go
package tui

import "github.com/charmbracelet/lipgloss"

// HUD 調色盤 — 純 hex；lipgloss colorprofile 在低色數終端自動降級。
var (
	colorCyan   = lipgloss.Color("#00D7FF") // 主 HUD 色
	colorPurple = lipgloss.Color("#B48CFF") // 次要
	colorGain   = lipgloss.Color("#50FA7B") // 漲 / 成功
	colorLoss   = lipgloss.Color("#FF5555") // 跌 / 威脅
	colorAmber  = lipgloss.Color("#FFB86C") // 警告 / 倒數
	colorGold   = lipgloss.Color("#FFD75F") // 慶祝 / 里程碑
	colorDim    = lipgloss.Color("#6B7280") // 邊框灰
	colorInk    = lipgloss.Color("#0B1220") // 反白文字用深底色

	styleCyan   = lipgloss.NewStyle().Foreground(colorCyan)
	stylePurple = lipgloss.NewStyle().Foreground(colorPurple)
	styleGain   = lipgloss.NewStyle().Foreground(colorGain)
	styleLoss   = lipgloss.NewStyle().Foreground(colorLoss)
	styleAmber  = lipgloss.NewStyle().Foreground(colorAmber)
	styleGold   = lipgloss.NewStyle().Foreground(colorGold)
)

// CardKind 選擇作戰室卡片變體。
type CardKind int

const (
	CardDefault CardKind = iota // 灰細邊：一般資訊
	CardAccent                  // 青粗邊：作戰室重點
	CardThreat                  // 紅邊：宿敵 / 危機
	CardGold                    // 金雙線邊：慶祝
)

func cardStyle(kind CardKind) lipgloss.Style {
	base := lipgloss.NewStyle().Padding(0, 1)
	switch kind {
	case CardAccent:
		return base.Border(lipgloss.ThickBorder()).BorderForeground(colorCyan)
	case CardThreat:
		return base.Border(lipgloss.RoundedBorder()).BorderForeground(colorLoss)
	case CardGold:
		return base.Border(lipgloss.DoubleBorder()).BorderForeground(colorGold)
	default:
		return base.Border(lipgloss.RoundedBorder()).BorderForeground(colorDim)
	}
}

func cardTitleStyle(kind CardKind) lipgloss.Style {
	switch kind {
	case CardThreat:
		return lipgloss.NewStyle().Bold(true).Foreground(colorLoss)
	case CardGold:
		return lipgloss.NewStyle().Bold(true).Foreground(colorGold)
	default:
		return lipgloss.NewStyle().Bold(true).Foreground(colorCyan)
	}
}

// CardIn renders a card variant; width > 0 forces the total rendered width
// (borders included) so grid rows align flush.
func CardIn(kind CardKind, width int, title, body string) string {
	st := cardStyle(kind)
	if width > 0 {
		st = st.Width(width - 2) // Style.Width 是內容寬；左右邊框各 +1
	}
	return st.Render(cardTitleStyle(kind).Render(title) + "\n" + body)
}
```

- [ ] **Step 4: rebind style.go 舊名（保留呼叫端不動）**

把 `internal/tui/style.go` 整檔改為：

```go
package tui

import "github.com/charmbracelet/lipgloss"

// 舊樣式名 rebind 到 theme.go 的 HUD 調色盤，呼叫端逐步遷移。
var (
	styleAccent    = styleCyan
	styleWarn      = styleLoss
	styleMuted     = lipgloss.NewStyle().Faint(true)
	styleTitle     = lipgloss.NewStyle().Bold(true).Foreground(colorCyan)
	styleTabActive = lipgloss.NewStyle().Bold(true).Foreground(colorCyan).Underline(true)

	titleStyle     = styleTitle
	boxStyle       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorDim).Padding(0, 1)
	helpStyle      = styleMuted
	tabActiveStyle = styleTabActive
)
```

把 `internal/tui/layout.go` 的 `Card` 改為：

```go
// Card renders a styled box with a title and body.
func Card(title, body string) string {
	return CardIn(CardDefault, 0, title, body)
}
```

- [ ] **Step 5: 跑測試確認通過 + 全包迴歸**

Run: `go test ./internal/tui/`
Expected: PASS（無 TTY 下色碼被剝除，既有純文字斷言不受影響；若有測試斷言舊 Card 邊框行為失敗，檢查該斷言是否僅涉及文字內容並修正）

- [ ] **Step 6: Commit**

```bash
git add internal/tui/theme.go internal/tui/theme_test.go internal/tui/style.go internal/tui/layout.go
git commit -m "feat(tui): add war-room HUD palette and card variants

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: 彩色進度條家族（漸層 / 負載 / 金色）

**Files:**
- Modify: `internal/tui/layout.go:78-81, 120-130`（Bar 改漸層、刪舊 progressBar）
- Modify: `internal/tui/theme.go`（加 bar 實作）
- Test: `internal/tui/theme_test.go`（追加）

**Interfaces:**
- Consumes: Task 1 的調色盤。
- Produces:
  - `func Bar(frac float64, width int) string` — 簽名不變，改為青→紫漸層 `█`，未滿部分 muted `░`
  - `func LoadBar(frac float64, width int) string` — 負載閾值變色：<0.7 青、0.7-0.9 琥珀、≥0.9 紅
  - `func GoldBar(frac float64, width int) string` — 金色漸層（里程碑 / 成就）
  - `func loadColor(frac float64) lipgloss.Color`（可測）
  - `func filledCells(frac float64, width int) int`（可測；夾住 [0,1]）

- [ ] **Step 1: 寫失敗測試**

追加到 `internal/tui/theme_test.go`：

```go
func TestBarWidthAndChars(t *testing.T) {
	for _, frac := range []float64{-0.5, 0, 0.33, 0.5, 1, 1.7} {
		got := Bar(frac, 10)
		if lipgloss.Width(got) != 10 {
			t.Fatalf("Bar(%v) width = %d, want 10", frac, lipgloss.Width(got))
		}
	}
	if !strings.Contains(Bar(1, 4), "████") {
		t.Fatalf("full bar should be solid blocks: %q", Bar(1, 4))
	}
	if !strings.Contains(Bar(0, 4), "░░░░") {
		t.Fatalf("empty bar should be all shade: %q", Bar(0, 4))
	}
}

func TestLoadColorThresholds(t *testing.T) {
	if loadColor(0.5) != colorCyan {
		t.Fatal("0.5 should be cyan")
	}
	if loadColor(0.75) != colorAmber {
		t.Fatal("0.75 should be amber")
	}
	if loadColor(0.95) != colorLoss {
		t.Fatal("0.95 should be red")
	}
}

func TestFilledCellsClamps(t *testing.T) {
	if filledCells(-1, 10) != 0 || filledCells(2, 10) != 10 {
		t.Fatal("filledCells must clamp frac to [0,1]")
	}
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/tui/ -run 'TestBar|TestLoadColor|TestFilled' -v`
Expected: FAIL — `undefined: loadColor` / `undefined: filledCells`

- [ ] **Step 3: 實作（追加到 theme.go）**

```go
// theme.go 頂部 import 改為：
import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// filledCells clamps frac to [0,1] and returns the filled cell count.
func filledCells(frac float64, width int) int {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	return int(frac * float64(width))
}

func loadColor(frac float64) lipgloss.Color {
	switch {
	case frac >= 0.9:
		return colorLoss
	case frac >= 0.7:
		return colorAmber
	default:
		return colorCyan
	}
}

// lerpHex linearly interpolates two #RRGGBB colors.
func lerpHex(a, b string, t float64) string {
	pa, _ := strconv.ParseUint(strings.TrimPrefix(a, "#"), 16, 32)
	pb, _ := strconv.ParseUint(strings.TrimPrefix(b, "#"), 16, 32)
	c := func(x, y uint64) uint64 { return uint64(float64(x) + t*(float64(y)-float64(x))) }
	return fmt.Sprintf("#%02X%02X%02X",
		c(pa>>16&0xFF, pb>>16&0xFF), c(pa>>8&0xFF, pb>>8&0xFF), c(pa&0xFF, pb&0xFF))
}

func gradientBar(frac float64, width int, from, to string) string {
	n := filledCells(frac, width)
	var sb strings.Builder
	for i := 0; i < n; i++ {
		t := 0.0
		if width > 1 {
			t = float64(i) / float64(width-1)
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(lerpHex(from, to, t))).Render("█"))
	}
	sb.WriteString(styleMuted.Render(strings.Repeat("░", width-n)))
	return sb.String()
}

// LoadBar colors by utilisation thresholds (<0.7 cyan, <0.9 amber, else red).
func LoadBar(frac float64, width int) string {
	n := filledCells(frac, width)
	return lipgloss.NewStyle().Foreground(loadColor(frac)).Render(strings.Repeat("█", n)) +
		styleMuted.Render(strings.Repeat("░", width-n))
}

// GoldBar is the milestone / achievement bar.
func GoldBar(frac float64, width int) string {
	return gradientBar(frac, width, "#FFD75F", "#FFB86C")
}
```

把 `internal/tui/layout.go` 的 `Bar` 與 `progressBar` 改為（刪除舊 `progressBar`）：

```go
// Bar renders the default cyan→purple gradient progress bar.
func Bar(frac float64, width int) string {
	return gradientBar(frac, width, "#00D7FF", "#B48CFF")
}
```

- [ ] **Step 4: 更新舊字元斷言**

Run: `grep -rn '▓' internal/tui/ --include='*_test.go'`
把所有測試裡的 `▓` 期望值換成 `█`（進度條實字元變了；`░` 不變）。

- [ ] **Step 5: 跑全包測試確認通過**

Run: `go test ./internal/tui/`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/tui/theme.go internal/tui/theme_test.go internal/tui/layout.go internal/tui/*_test.go
git commit -m "feat(tui): gradient, load and gold progress bars

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: 等寬 Grid + 總覽頁作戰室化

**Files:**
- Modify: `internal/tui/layout.go`（加 Grid）
- Modify: `internal/tui/page_overview.go`（整頁重構）
- Modify: `internal/tui/campaign_meta.go:111-155, 158-171, 219-240`（三卡加 width 參數與變體）
- Test: `internal/tui/layout_test.go`、`internal/tui/page_overview_test.go`

**Interfaces:**
- Consumes: Task 1 `CardIn`、Task 2 `Bar/LoadBar/GoldBar`。
- Produces:
  - `func Grid(cw, gap int, cells ...func(w int) string) string` — 2 欄等寬；`cw < minDashWidth` 直排（cell 拿全寬）；奇數尾 cell 拿全寬
  - `renderCampaignStatusCard(m Model, w int) string`（簽名加 width）
  - `renderRivalRoadmapCard(m Model, w int) string`
  - `renderBoardReportCard(m Model, w int) string`
  - `renderEventsCard(m Model, w int) string`
  - overview 私有 helper：`companyCard(m Model, w int) string`、`trainCard(m Model, w int) string`、`shareCard(m Model, w int) string`、`powerMilestoneCard(m Model, w int) string`

- [ ] **Step 1: 寫失敗測試**

追加到 `internal/tui/layout_test.go`：

```go
func TestGridEqualWidths(t *testing.T) {
	cell := func(label string) func(int) string {
		return func(w int) string { return CardIn(CardDefault, w, label, "x") }
	}
	got := Grid(100, 2, cell("A"), cell("B"), cell("C"))
	lines := strings.Split(got, "\n")
	// 第一行是兩張 49 寬卡 + 2 gap = 100
	if lipgloss.Width(lines[0]) != 100 {
		t.Fatalf("row width = %d, want 100", lipgloss.Width(lines[0]))
	}
	// 奇數尾 cell 拿全寬
	last := lines[len(lines)-1]
	if lipgloss.Width(last) != 100 {
		t.Fatalf("orphan cell width = %d, want 100", lipgloss.Width(last))
	}
}

func TestGridStacksWhenNarrow(t *testing.T) {
	cell := func(w int) string { return CardIn(CardDefault, w, "T", "B") }
	got := Grid(60, 2, cell, cell)
	for _, ln := range strings.Split(got, "\n") {
		if lipgloss.Width(ln) > 60 {
			t.Fatalf("narrow grid line overflows: %d > 60", lipgloss.Width(ln))
		}
	}
}
```

追加到 `internal/tui/page_overview_test.go`：

```go
func TestOverviewCardsAlignFlush(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 110, Height: 40})
	m = mm.(Model)
	out := renderOverview(m)
	// 每一行都不超過 content width，且格線行等寬（左右卡齊平）
	cw := m.contentWidth()
	for i, ln := range strings.Split(out, "\n") {
		if lipgloss.Width(ln) > cw {
			t.Fatalf("line %d overflows content width %d: %q", i, cw, ln)
		}
	}
}
```

（依該測試檔既有 import 慣例補 `filepath`、`tea`、`lipgloss`、`strings`。）

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/tui/ -run 'TestGrid|TestOverviewCardsAlign' -v`
Expected: FAIL — `undefined: Grid`

- [ ] **Step 3: 實作 Grid（追加到 layout.go）**

```go
// Grid lays cells out in two equal-width columns; below minDashWidth it
// stacks vertically with full-width cells. An odd trailing cell gets full width.
func Grid(cw, gap int, cells ...func(w int) string) string {
	if len(cells) == 0 {
		return ""
	}
	if cw < minDashWidth {
		parts := make([]string, len(cells))
		for i, c := range cells {
			parts[i] = c(cw)
		}
		return VStack(parts...)
	}
	colW := (cw - gap) / 2
	var rows []string
	for i := 0; i < len(cells); i += 2 {
		if i+1 < len(cells) {
			rows = append(rows, HRow(gap, cells[i](colW), cells[i+1](colW)))
		} else {
			rows = append(rows, cells[i](cw))
		}
	}
	return VStack(rows...)
}
```

- [ ] **Step 4: campaign_meta.go 三卡加 width 與變體**

`renderCampaignStatusCard`：簽名改 `func renderCampaignStatusCard(m Model, w int) string`；兩處 `return Card("公司戰略", ...)` 改為 `return CardIn(CardAccent, w, "公司戰略", ...)`（`campaign_meta.go:114, 154`）。

`renderRivalRoadmapCard`：簽名改 `func renderRivalRoadmapCard(m Model, w int) string`；`return Card("宿敵路線", VStack(blocks...))` 改為 `return CardIn(CardThreat, w, "宿敵路線", VStack(blocks...))`（`campaign_meta.go:170`）。

`renderBoardReportCard`：簽名改 `func renderBoardReportCard(m Model, w int) string`；三處 `Card("董事會報告", ...)` 改為 `CardIn(CardDefault, w, "董事會報告", ...)`（`campaign_meta.go:222, 227, 239`）。

- [ ] **Step 5: 重構 renderOverview**

`internal/tui/page_overview.go` 的 `renderOverview` 換成：

```go
func renderOverview(m Model) string {
	cw := m.contentWidth()
	rows := []string{
		Grid(cw, 2,
			func(w int) string { return renderCampaignStatusCard(m, w) },
			func(w int) string { return renderRivalRoadmapCard(m, w) },
			func(w int) string { return companyCard(m, w) },
			func(w int) string { return trainCard(m, w) },
			func(w int) string { return shareCard(m, w) },
			func(w int) string { return powerMilestoneCard(m, w) },
		),
		renderBoardReportCard(m, cw),
		renderEventsCard(m, cw),
	}
	if warns := pressures(m); len(warns) > 0 {
		rows = append(rows, CardIn(CardThreat, cw, "注意", VStack(warns...)))
	}
	return VStack(rows...)
}
```

四張卡的內容從舊 `renderOverview` 原封搬進私有 helper（估值/用戶的 display 平滑值邏輯照舊），只把最後的 `Card(...)` 換成 `CardIn(..., w, ...)`：

```go
func companyCard(m Model, w int) string {
	s := m.state
	rank, field := sim.MarketRank(s, m.cfg, model.SegConsumer)
	val := sim.Valuation(s, m.cfg)
	totalUsers := sim.TotalUsers(s)
	if m.dispReady {
		val = m.disp.Valuation
		totalUsers = m.disp.TotalUsers
	}
	body := VStack(
		KV("估值", "$"+human(val)),
		KV("總用戶", human(totalUsers)),
		KV("排名", fmt.Sprintf("#%d / %d", rank, field)),
		KV("月營收", "$"+human(sim.MonthlyRevenue(s))),
	)
	return CardIn(CardDefault, w, "公司", body)
}

func trainCard(m Model, w int) string {
	s := m.state
	var body string
	if s.HasTraining {
		total := m.cfg.GenTrainWorkGPUSec[s.Training.Gen]
		done := 1.0
		if total > 0 {
			done = 1.0 - s.Training.WorkRemaining/total
		}
		body = fmt.Sprintf("Gen%d %s %.0f%%\n%s", s.Training.Gen, Bar(done, 12), done*100,
			KV("區隔", segmentName(s.Training.Segment)))
	} else {
		drafts := 0
		for _, md := range s.Models {
			if sim.IsDraft(md) {
				drafts++
			}
		}
		if drafts > 0 {
			body = fmt.Sprintf("無進行中訓練\n%s",
				styleWarn.Render(fmt.Sprintf("待發佈 %d 個（模型頁 p）", drafts)))
		} else {
			body = "無進行中訓練\n(到模型頁按 t 開訓)"
		}
	}
	return CardIn(CardDefault, w, "訓練 / 發佈", body)
}

func shareCard(m Model, w int) string {
	s := m.state
	var shareLines []string
	bars := sim.SegmentShareBars(s, m.cfg, model.SegConsumer)
	limit := 5
	if len(bars) < limit {
		limit = len(bars)
	}
	for i := 0; i < limit; i++ {
		bRow := bars[i]
		share := bRow.Share
		if m.dispReady && i < len(m.disp.ConsumerShares) {
			share = m.disp.ConsumerShares[i]
		}
		star := " "
		if bRow.You {
			star = "★"
		}
		name := Truncate(bRow.Name, 10)
		namePadding := strings.Repeat(" ", 10-len([]rune(name)))
		if len([]rune(name)) > 10 {
			namePadding = ""
		}
		shareLines = append(shareLines, fmt.Sprintf("%s %s%s %s %.0f%%", star, name, namePadding, Bar(share, 10), share*100))
	}
	return CardIn(CardDefault, w, "市佔 (消費者)", VStack(shareLines...))
}

func powerMilestoneCard(m Model, w int) string {
	s := m.state
	trainUtil := 0.0
	if s.HasTraining {
		trainUtil = 1.0
	}
	infUtil := 0.0
	if cap := sim.EffectiveInference(s, m.cfg); cap > 0 {
		infUtil = s.Compute.InferenceLoad / cap
	}
	if m.dispReady {
		trainUtil, infUtil = m.disp.TrainUtil, m.disp.InfUtil
	}
	infBar := fmt.Sprintf("推理 %s %.0f%%", LoadBar(infUtil, 10), infUtil*100)
	milestoneStr := ""
	if target, prog, ok := sim.NextMilestone(s, m.cfg); ok {
		milestoneStr = fmt.Sprintf("里程碑 $%s %s %.0f%%", human(target), GoldBar(prog, 10), prog*100)
	} else {
		milestoneStr = styleGold.Render("里程碑 全部達成 ✓")
	}
	body := VStack(
		fmt.Sprintf("訓練 %s %.0f%%", LoadBar(trainUtil, 10), trainUtil*100),
		infBar,
		milestoneStr,
	)
	return CardIn(CardDefault, w, "里程碑 & 算力", body)
}
```

（原 `renderOverview` 中推理 ≥0.9 的整行 `styleWarn` 高亮已由 `LoadBar` 的紅色取代，移除該分支。）

`renderEventsCard` 簽名改 `func renderEventsCard(m Model, w int) string`，最後一行改 `return CardIn(CardDefault, w, "產業動態", VStack(lines...))`。

- [ ] **Step 6: 跑測試、修呼叫端**

Run: `go test ./internal/tui/`
Expected: 編譯錯誤指出所有舊簽名呼叫端（如 `page_overview.go`、tests）——逐一補上 width 引數後 PASS。

- [ ] **Step 7: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): equal-width grid layout and war-room overview

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: 其餘頁面套用變體與 LoadBar

**Files:**
- Modify: `internal/tui/page_market.go`
- Modify: `internal/tui/page_compute.go`
- Modify: `internal/tui/page_team.go`
- Modify: `internal/tui/page_tech.go`
- Modify: `internal/tui/page_models.go`
- Test: 既有各頁測試（斷言微調）

**Interfaces:**
- Consumes: Task 1-3 全部。
- Produces: 無新介面；各頁視覺一致化。

- [ ] **Step 1: page_market.go**

`renderMarket`（`page_market.go:12-71`）：
- 三張市場卡與對手卡改用 `CardIn`，欄寬 `colW := (m.contentWidth() - 2) / 2`；市場卡 `CardIn(CardDefault, colW, ...)`，對手卡 `CardIn(CardThreat, colW, "對手檔案", ...)`。
- 窄畫面（`m.contentWidth() < minDashWidth`）時 colW 用全寬 `m.contentWidth()`。實作：

```go
	cw := m.contentWidth()
	colW := cw
	if cw >= minDashWidth {
		colW = (cw - 2) / 2
	}
```

- 對手能力條 `Bar(capFrac, 10)` 保持漸層即可，不改 LoadBar（能力不是負載）。

- [ ] **Step 2: page_compute.go**

- 池狀態卡的訓練/推理池條、機房電力/空間條改用 `LoadBar`（負載語意）。
- 卡片改 `CardIn(CardDefault, 0, ...)` 保持自動寬度——製程表格行寬不定，強制欄寬會折行（ponytail: 對齊留給表格內容不變的頁面）。

- [ ] **Step 3: page_team.go / page_tech.go / page_models.go**

- 團隊兩卡、科技四類卡、模型列表卡：`CardIn(CardDefault, m.contentWidth(), ...)` 拉滿內容寬，右緣齊平。
- 科技卡中已解鎖節點行首 `✓` 用 `styleGain` 上色；`🔒` 行整行 `styleMuted`。（page_tech.go 內格式化節點行處。）

- [ ] **Step 4: 跑全包測試、更新過時斷言**

Run: `go test ./internal/tui/`
Expected: PASS（斷言舊寬度/舊字元的測試在此任務內修正）

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): apply HUD card variants and load bars to all pages

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: 資源列膠囊化 + Tab bar 反白

**Files:**
- Modify: `internal/tui/tui.go:1037-1086`（renderResourceBar）
- Modify: `internal/tui/tui.go:1180-1190`（renderTabBar）
- Test: `internal/tui/tui_test.go`（追加）

**Interfaces:**
- Consumes: Task 1 調色盤。
- Produces: 資源列以 ` │ ` 分段；tab bar active 頁青底反白。文字內容（emoji、數字格式）不變，僅上色與分隔符。

- [ ] **Step 1: 寫失敗測試**

追加到 `internal/tui/tui_test.go`：

```go
func TestResourceBarSegments(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	bar := renderResourceBar(m)
	if !strings.Contains(bar, "│") {
		t.Fatalf("resource bar should use │ separators: %q", bar)
	}
	if !strings.Contains(bar, "💰") || !strings.Contains(bar, "📈") {
		t.Fatalf("segments missing: %q", bar)
	}
}

func TestTabBarMarksActive(t *testing.T) {
	got := renderTabBar(PageMarket)
	if !strings.Contains(got, "3 市場") {
		t.Fatalf("tab bar labels changed unexpectedly: %q", got)
	}
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/tui/ -run 'TestResourceBar|TestTabBar' -v`
Expected: FAIL —（`│` 不存在；tab 標籤格式為 `[3]市場`）

- [ ] **Step 3: 實作**

`renderResourceBar` 尾段組合改為（上半段 cash/inf/rnd 計算不動）：

```go
	valStr := stylePurple.Render(fmt.Sprintf("📈估值 $%s", human(val)))
	sep := styleMuted.Render(" │ ")
	segs := []string{
		cashStr,
		"⚡R&D " + rndSeg,
		fmt.Sprintf("🖥訓練%.0f%% %s", trainUtil*100, infStr),
		valStr,
	}
	bar := strings.Join(segs, sep)
```

（`if m.disp.PulseToken > 0 && len(m.lastTokenRnD) > 0` 的尾段照舊 append。）

`renderTabBar` 改為：

```go
func renderTabBar(p Page) string {
	var parts []string
	for i, name := range pageNames {
		label := fmt.Sprintf(" %d %s ", i+1, name)
		if Page(i) == p {
			parts = append(parts, lipgloss.NewStyle().Bold(true).Foreground(colorInk).Background(colorCyan).Render(label))
		} else {
			parts = append(parts, styleMuted.Render(label))
		}
	}
	return strings.Join(parts, " ")
}
```

（`tui.go` 需 import `github.com/charmbracelet/lipgloss`，檢查是否已有。）

- [ ] **Step 4: 跑測試確認通過**

Run: `go test ./internal/tui/`
Expected: PASS（footer/nav 測試若斷言 `[1]總覽` 格式，更新為 ` 1 總覽 `）

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): capsule resource bar and inverted active tab

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---
## Phase 2｜活起來（動態層）

### Task 6: spark.go ring buffer 與渲染

**Files:**
- Create: `internal/tui/spark.go`
- Test: `internal/tui/spark_test.go`

**Interfaces:**
- Consumes: 無。
- Produces:
  - `type spark struct`（私有）
  - `func newSpark(capacity int) spark`
  - `func (s *spark) push(v float64)`
  - `func (s *spark) values() []float64` — 舊→新
  - `func (s *spark) Render(width int) string` — 取最新 width 個樣本畫 `▁▂▃▄▅▆▇█`；樣本 < 2 回傳 `""`

- [ ] **Step 1: 寫失敗測試**

```go
// internal/tui/spark_test.go
package tui

import "testing"

func TestSparkPushWrapsAndOrders(t *testing.T) {
	s := newSpark(3)
	for _, v := range []float64{1, 2, 3, 4} {
		s.push(v)
	}
	got := s.values()
	want := []float64{2, 3, 4}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("values() = %v, want %v", got, want)
		}
	}
}

func TestSparkRender(t *testing.T) {
	s := newSpark(8)
	s.push(0)
	s.push(50)
	s.push(100)
	out := s.Render(8)
	r := []rune(out)
	if len(r) != 3 {
		t.Fatalf("rune len = %d, want 3 (%q)", len(r), out)
	}
	if r[0] != '▁' || r[2] != '█' {
		t.Fatalf("min/max runes wrong: %q", out)
	}
}

func TestSparkRenderFlatAndEmpty(t *testing.T) {
	s := newSpark(4)
	if s.Render(4) != "" {
		t.Fatal("empty spark should render empty string")
	}
	s.push(5)
	if s.Render(4) != "" {
		t.Fatal("single sample should render empty string")
	}
	s.push(5)
	out := []rune(s.Render(4))
	if len(out) != 2 || out[0] != out[1] {
		t.Fatalf("flat series should be uniform runes: %q", string(out))
	}
}

func TestSparkRenderTruncatesToWidth(t *testing.T) {
	s := newSpark(16)
	for i := 0; i < 16; i++ {
		s.push(float64(i))
	}
	if got := len([]rune(s.Render(6))); got != 6 {
		t.Fatalf("render width = %d, want 6", got)
	}
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/tui/ -run TestSpark -v`
Expected: FAIL — `undefined: newSpark`

- [ ] **Step 3: 實作 spark.go**

```go
// internal/tui/spark.go
package tui

var sparkRunes = []rune("▁▂▃▄▅▆▇█")

// spark is a fixed-capacity ring buffer of display-layer samples.
// TUI-memory only; never persisted.
type spark struct {
	buf  []float64
	head int // next write index
	n    int
}

func newSpark(capacity int) spark {
	return spark{buf: make([]float64, capacity)}
}

func (s *spark) push(v float64) {
	s.buf[s.head] = v
	s.head = (s.head + 1) % len(s.buf)
	if s.n < len(s.buf) {
		s.n++
	}
}

// values returns samples oldest→newest.
func (s *spark) values() []float64 {
	out := make([]float64, 0, s.n)
	start := (s.head - s.n + len(s.buf)) % len(s.buf)
	for i := 0; i < s.n; i++ {
		out = append(out, s.buf[(start+i)%len(s.buf)])
	}
	return out
}

// Render draws the newest `width` samples; "" when fewer than 2 samples.
func (s *spark) Render(width int) string {
	vals := s.values()
	if len(vals) > width {
		vals = vals[len(vals)-width:]
	}
	if len(vals) < 2 {
		return ""
	}
	lo, hi := vals[0], vals[0]
	for _, v := range vals {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	out := make([]rune, 0, len(vals))
	for _, v := range vals {
		idx := 0
		if hi > lo {
			idx = int((v - lo) / (hi - lo) * float64(len(sparkRunes)-1))
		}
		out = append(out, sparkRunes[idx])
	}
	return string(out)
}
```

- [ ] **Step 4: 跑測試確認通過**

Run: `go test ./internal/tui/ -run TestSpark -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/spark.go internal/tui/spark_test.go
git commit -m "feat(tui): sparkline ring buffer

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 7: sparkline 接線（估值 / 用戶 / R&D 速率）

**Files:**
- Modify: `internal/tui/tui.go:57-107`（Model 加欄位）、`internal/tui/tui.go:123-164`（newAtPaths 初始化）
- Modify: `internal/tui/display.go:125-141`（advanceDisplay 取樣）
- Modify: `internal/tui/page_overview.go`（companyCard 加趨勢行）
- Test: `internal/tui/display_test.go`（追加）

**Interfaces:**
- Consumes: Task 6 `spark`；Task 3 `companyCard`。
- Produces: Model 欄位 `sparkValuation, sparkUsers, sparkRnD spark`、`sparkTick int`。取樣節奏：每 4 tick（約 1 實秒）一點，容量 60。

- [ ] **Step 1: 寫失敗測試**

追加到 `internal/tui/display_test.go`：

```go
func TestAdvanceDisplaySamplesSparks(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	for i := 0; i < 9; i++ { // 9 ticks → 至少 2 個樣本（tick 4 與 8）
		m.advanceDisplay()
	}
	if m.sparkValuation.n < 2 {
		t.Fatalf("valuation spark samples = %d, want >= 2", m.sparkValuation.n)
	}
	if m.sparkUsers.n < 2 || m.sparkRnD.n < 2 {
		t.Fatal("users/rnd sparks not sampled")
	}
}

func TestCompanyCardShowsTrend(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	for i := 0; i < 9; i++ {
		m.advanceDisplay()
	}
	out := companyCard(m, 50)
	if !strings.Contains(out, "趨勢") {
		t.Fatalf("company card missing trend line: %q", out)
	}
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/tui/ -run 'TestAdvanceDisplaySamples|TestCompanyCardShowsTrend' -v`
Expected: FAIL — `m.sparkValuation undefined`

- [ ] **Step 3: 實作**

Model struct（`tui.go`，加在 `disp displayState` 附近）：

```go
	// Display-layer trend history (TUI memory only, never persisted).
	sparkValuation spark
	sparkUsers     spark
	sparkRnD       spark
	sparkTick      int
```

`newAtPaths` 的 `m := Model{...}` 之後（`m.resize` 之前）加：

```go
	m.sparkValuation = newSpark(60)
	m.sparkUsers = newSpark(60)
	m.sparkRnD = newSpark(60)
```

`advanceDisplay`（display.go）函式尾端加：

```go
	m.sparkTick++
	if m.sparkTick%4 == 0 {
		m.sparkValuation.push(m.disp.Valuation)
		m.sparkUsers.push(m.disp.TotalUsers)
		m.sparkRnD.push(sim.RnDRatePerSec(m.state, m.cfg) * gameSecPerRealSec)
	}
```

（display.go import 加 `tokensmith/internal/sim` 若未有；`gameSecPerRealSec` 已在 tui 套件內。）

`companyCard`（page_overview.go）在 `body := VStack(...)` 前加趨勢行：

```go
	lines := []string{
		KV("估值", "$"+human(val)),
		KV("總用戶", human(totalUsers)),
		KV("排名", fmt.Sprintf("#%d / %d", rank, field)),
		KV("月營收", "$"+human(sim.MonthlyRevenue(s))),
	}
	if tr := m.sparkValuation.Render(18); tr != "" {
		lines = append(lines, styleCyan.Render("趨勢 ")+stylePurple.Render(tr))
	}
	body := VStack(lines...)
```

- [ ] **Step 4: 跑測試確認通過**

Run: `go test ./internal/tui/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): wire valuation/users/rnd sparklines into overview

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 8: 資源列漲跌箭頭 + streak 常駐 + token chip

**Files:**
- Modify: `internal/tui/tui.go`（Model 加 `cashRate float64`；常數 `ticksPerRealSec`）
- Modify: `internal/tui/display.go`（advanceDisplay 算速率）
- Modify: `internal/tui/tui.go:1037-1086`（renderResourceBar）
- Test: `internal/tui/display_test.go`、`internal/tui/tui_test.go`（追加）

**Interfaces:**
- Consumes: Task 5 資源列分段結構。
- Produces: Model 欄位 `cashRate float64`（平滑後 $/實秒）；常數 `ticksPerRealSec = 4`。顯示規則：`cashRate > 0.5` → 綠 ▲、`< -0.5` → 紅 ▼、其餘不顯示。streak ≥ 2 天常駐 `🔥N天 ×M`。

- [ ] **Step 1: 寫失敗測試**

追加到 `internal/tui/display_test.go`：

```go
func TestAdvanceDisplayTracksCashRate(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	m.snapDisplay()
	m.state.Resources.Cash += 1000 // 下一 tick 現金上升
	for i := 0; i < 20; i++ {
		m.advanceDisplay()
	}
	if m.cashRate <= 0 {
		t.Fatalf("cashRate = %f, want > 0 after cash increase", m.cashRate)
	}
}
```

追加到 `internal/tui/tui_test.go`：

```go
func TestResourceBarShowsCashArrowAndStreak(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	m.dispReady = true
	m.cashRate = 42
	m.streakDays = 5
	bar := renderResourceBar(m)
	if !strings.Contains(bar, "▲") {
		t.Fatalf("positive cashRate should show ▲: %q", bar)
	}
	if !strings.Contains(bar, "🔥5天") {
		t.Fatalf("streak should be persistent in bar: %q", bar)
	}
	m.cashRate = -42
	if bar = renderResourceBar(m); !strings.Contains(bar, "▼") {
		t.Fatalf("negative cashRate should show ▼: %q", bar)
	}
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/tui/ -run 'TestAdvanceDisplayTracksCashRate|TestResourceBarShowsCashArrow' -v`
Expected: FAIL — `m.cashRate undefined`

- [ ] **Step 3: 實作**

`tui.go` 常數區（`tickInterval` 旁）加：

```go
// ticksPerRealSec is the tick frequency (250ms interval).
const ticksPerRealSec = 4
```

Model struct 加欄位（sparks 旁）：

```go
	cashRate float64 // smoothed display cash delta, $/real-second
```

`advanceDisplay`（display.go）開頭記舊值、approach 後更新速率：

```go
func (m *Model) advanceDisplay() {
	prevCash := m.disp.Cash
	wasReady := m.dispReady
	truth := truthDisplay(*m)
	if !m.dispReady {
		m.disp.snap(truth)
		m.dispReady = true
	} else {
		m.disp.approach(truth, displayAlpha)
	}
	if wasReady {
		instant := (m.disp.Cash - prevCash) * ticksPerRealSec
		m.cashRate = approachScalar(m.cashRate, instant, displayAlpha, 0.001)
	}
	// ...（原 pulse 遞減與 Task 7 的 spark 取樣照舊）
}
```

`renderResourceBar` 的 cashStr 段改為：

```go
	cashStr := fmt.Sprintf("💰 $%s", human(cash))
	switch {
	case cash < 0:
		cashStr = styleLoss.Render(cashStr)
	case m.cashRate > 0.5:
		cashStr += styleGain.Render(fmt.Sprintf(" ▲$%s/s", human(m.cashRate)))
	case m.cashRate < -0.5:
		cashStr += styleLoss.Render(fmt.Sprintf(" ▼$%s/s", human(-m.cashRate)))
	}
```

streak 常駐：`segs` 組完後（`bar := strings.Join(segs, sep)` 之前）加：

```go
	if m.streakDays >= 2 {
		streak := fmt.Sprintf("🔥%d天 ×%.2f", m.streakDays, m.currentStreakMult())
		if m.disp.PulseToken > 0 {
			segs = append(segs, styleGold.Bold(true).Render(streak))
		} else {
			segs = append(segs, styleAmber.Render(streak))
		}
	}
```

並把原 pulse 尾段（`tui.go:1075-1084`）中的 streak 行刪掉（避免重複），token 來源 chip 改上色：

```go
	if m.disp.PulseToken > 0 && len(m.lastTokenRnD) > 0 {
		parts := make([]string, 0, len(m.lastTokenRnD))
		for _, src := range sourceKeysOrdered(m.lastTokenRnD) {
			chip := fmt.Sprintf(" ⚡%s +%s R&D ", sourceLabel(src), human(m.lastTokenRnD[src]))
			parts = append(parts, lipgloss.NewStyle().Bold(true).Foreground(colorInk).Background(colorCyan).Render(chip))
		}
		bar += "  " + strings.Join(parts, " ")
	}
```

- [ ] **Step 4: 跑測試確認通過**

Run: `go test ./internal/tui/`
Expected: PASS（若既有測試斷言 pulse 段舊格式 `⚡ Claude Code +`，更新為新 chip 格式 `⚡Claude Code +`）

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): cash delta arrows, persistent streak, token chips

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 9: 市佔你的列高亮 + 名次變化箭頭

**Files:**
- Modify: `internal/tui/tui.go`（Model 加名次歷史；tick 取樣）
- Modify: `internal/tui/page_market.go`（header 箭頭 + 你的列高亮）
- Modify: `internal/tui/page_overview.go`（shareCard 你的列高亮）
- Test: `internal/tui/page_market_test.go`（追加）

**Interfaces:**
- Consumes: Task 1 調色盤。
- Produces:
  - Model 欄位 `prevRank, lastRank [model.NumSegments]int`、`rankTick int`（0 = 尚無資料，rank 為 1-based）
  - `func rankArrow(prev, cur int) string` — prev==0 或相等回 `""`；上升 `↑n` 綠、下降 `↓n` 紅
  - `func youRowStyle(line string) string` — 青底反白整列

- [ ] **Step 1: 寫失敗測試**

追加到 `internal/tui/page_market_test.go`：

```go
func TestRankArrow(t *testing.T) {
	if rankArrow(0, 3) != "" {
		t.Fatal("no history → no arrow")
	}
	if got := rankArrow(5, 3); !strings.Contains(got, "↑2") {
		t.Fatalf("rank 5→3 should be ↑2: %q", got)
	}
	if got := rankArrow(3, 5); !strings.Contains(got, "↓2") {
		t.Fatalf("rank 3→5 should be ↓2: %q", got)
	}
	if rankArrow(4, 4) != "" {
		t.Fatal("same rank → no arrow")
	}
}

func TestMarketHighlightsYouRow(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 110, Height: 40})
	m = mm.(Model)
	out := renderMarket(m)
	if !strings.Contains(out, "你") {
		t.Fatalf("market should contain your row: %q", out)
	}
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/tui/ -run 'TestRankArrow|TestMarketHighlights' -v`
Expected: FAIL — `undefined: rankArrow`

- [ ] **Step 3: 實作**

Model 欄位（sparks 旁）：

```go
	prevRank [model.NumSegments]int // 上次取樣名次（0 = 無資料）
	lastRank [model.NumSegments]int
	rankTick int
```

tick 取樣（`handleUpdate` 的 `case tickMsg`，`m.advanceDisplay()` 之前）：

```go
		m.rankTick++
		if m.rankTick >= 240 { // 每 60 實秒輪替一次名次快照
			m.rankTick = 0
			for seg := 0; seg < model.NumSegments; seg++ {
				r, _ := sim.MarketRank(m.state, m.cfg, model.Segment(seg))
				m.prevRank[seg] = m.lastRank[seg]
				m.lastRank[seg] = r
			}
		}
```

helpers（page_market.go）：

```go
// rankArrow shows rank movement since the previous snapshot (1-based ranks).
func rankArrow(prev, cur int) string {
	if prev == 0 || prev == cur {
		return ""
	}
	if cur < prev {
		return styleGain.Render(fmt.Sprintf(" ↑%d", prev-cur))
	}
	return styleLoss.Render(fmt.Sprintf(" ↓%d", cur-prev))
}

// youRowStyle inverts the player's row in share leaderboards.
func youRowStyle(line string) string {
	return lipgloss.NewStyle().Bold(true).Foreground(colorInk).Background(colorCyan).Render(line)
}
```

（page_market.go import 加 `github.com/charmbracelet/lipgloss`。）

`renderMarket` header 行加箭頭：

```go
		headerInfo := fmt.Sprintf("你的用戶: %s  ·  排名: #%d / %d%s  ·  市場規模: %s",
			human(segmentUsers(s, seg)), rank, field, rankArrow(m.prevRank[seg], rank), marketSizeLabel(m.cfg, seg))
```

share 迴圈的你的列（`bRow.You`）：

```go
		line := fmt.Sprintf("%s %s%s %s %.0f%%", star, name, namePadding, Bar(share, 10), share*100)
		if bRow.You {
			line = youRowStyle(line)
		}
		shareLines = append(shareLines, line)
```

`shareCard`（page_overview.go）同樣把 `bRow.You` 的列包 `youRowStyle`。

- [ ] **Step 4: 跑測試確認通過**

Run: `go test ./internal/tui/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): leaderboard you-row highlight and rank arrows

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---
## Phase 3｜遊戲化回饋

### Task 10: feedback.go 時刻偵測器

**Files:**
- Create: `internal/tui/feedback.go`
- Modify: `internal/model/campaign.go:89-94`（CampaignReportEntry 加 `Countered bool`——spec 唯一授權的 sim 側改動，純資料欄位）
- Modify: `internal/sim/campaign_rivals.go:110-162`（反制命中時標記 entry）
- Test: `internal/tui/feedback_test.go`

**Interfaces:**
- Consumes: `model.GameState`、`balance.Config`、campaign_meta.go 的標籤函式（`campaignStageLabel`、`doctrineLabel`、`rivalActionLabel`）。
- Produces:
  - `type MomentLevel int`；常數 `LevelMinor, LevelMajor, LevelEpic`
  - `type Moment struct { Level MomentLevel; Text string }`
  - `func detectMoments(prev, next model.GameState, cfg balance.Config) []Moment` — 純函式
  - `func newReportEntries(prev, next model.GameState) []model.CampaignReportEntry`
  - `model.CampaignReportEntry.Countered bool`（JSON 零值向後相容，舊存檔安全）

覆蓋時刻（科技解鎖不在此偵測——互動解鎖在 Task 13 的按鍵處直接回饋）：

| 時刻 | 偵測 | 等級 |
|---|---|---|
| 訓練完成 | `prev.HasTraining && !next.HasTraining` 且模型數增加 | Major |
| 里程碑達成 | `MilestonesReached` 增加 | Major |
| 階段推進 | 新報告 entry Kind==ReportStageAdvanced | Major |
| 決勝開始 | Kind==ReportShowdown | Major |
| 路線勝利 | Kind==ReportVictory | Epic |
| 宿敵行動（未反制） | Kind==ReportRivalAction 且 !Countered | Major |
| 反制奏效 | Kind==ReportRivalAction 且 Countered | Major |
| 財務風險 | Kind==ReportFinancialRisk | Major |

**Step 0（sim 側資料欄位）：** `internal/model/campaign.go:89-94` 的 `CampaignReportEntry` 加欄位：

```go
type CampaignReportEntry struct {
	Kind      CampaignReportKind
	SubjectID string
	DetailID  string
	Value     float64
	Countered bool // 該宿敵行動被高層指令反制（衝擊已減半）
}
```

`internal/sim/campaign_rivals.go` 中 `matched := ns.Campaign.CounterTarget == roadmap.Company && ...`（115 行附近）的函式尾端建構 entry 處（157-161 行）改為：

```go
	entry := model.CampaignReportEntry{
		Kind:      model.ReportRivalAction,
		SubjectID: roadmap.Company,
		DetailID:  actionID,
		Countered: matched,
	}
```

- [ ] **Step 1: 寫失敗測試**

```go
// internal/tui/feedback_test.go
package tui

import (
	"strings"
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestDetectTrainingCompleted(t *testing.T) {
	cfg := balance.Default()
	prev := model.GameState{HasTraining: true}
	next := model.GameState{Models: []model.Model{{Gen: 2}}}
	got := detectMoments(prev, next, cfg)
	if len(got) != 1 || got[0].Level != LevelMajor || !strings.Contains(got[0].Text, "Gen2") {
		t.Fatalf("want one Gen2 major moment, got %+v", got)
	}
}

func TestDetectMilestone(t *testing.T) {
	cfg := balance.Default()
	prev := model.GameState{MilestonesReached: 0}
	next := model.GameState{MilestonesReached: 1}
	got := detectMoments(prev, next, cfg)
	if len(got) != 1 || !strings.Contains(got[0].Text, "里程碑") {
		t.Fatalf("want milestone moment, got %+v", got)
	}
}

func TestDetectCampaignReports(t *testing.T) {
	cfg := balance.Default()
	prev := model.GameState{}
	next := model.GameState{}
	next.Campaign.Reports = []model.BoardReport{{
		Cycle: 1,
		Entries: []model.CampaignReportEntry{
			{Kind: model.ReportShowdown, SubjectID: "showdown"},
			{Kind: model.ReportVictory, SubjectID: string(model.DoctrineConsumer)},
		},
	}}
	got := detectMoments(prev, next, cfg)
	if len(got) != 2 {
		t.Fatalf("want 2 moments, got %+v", got)
	}
	if got[1].Level != LevelEpic {
		t.Fatalf("victory must be Epic, got %+v", got[1])
	}
}

func TestNewReportEntriesSameCycleGrowth(t *testing.T) {
	prev := model.GameState{}
	prev.Campaign.Reports = []model.BoardReport{{
		Cycle:   3,
		Entries: []model.CampaignReportEntry{{Kind: model.ReportRivalAction}},
	}}
	next := model.GameState{}
	next.Campaign.Reports = []model.BoardReport{{
		Cycle: 3,
		Entries: []model.CampaignReportEntry{
			{Kind: model.ReportRivalAction},
			{Kind: model.ReportFinancialRisk},
		},
	}}
	got := newReportEntries(prev, next)
	if len(got) != 1 || got[0].Kind != model.ReportFinancialRisk {
		t.Fatalf("want only the appended entry, got %+v", got)
	}
}

func TestDetectNothingOnNoChange(t *testing.T) {
	cfg := balance.Default()
	s := model.GameState{MilestonesReached: 2}
	if got := detectMoments(s, s, cfg); len(got) != 0 {
		t.Fatalf("no change should yield no moments, got %+v", got)
	}
}
```

另加反制奏效案例：

```go
func TestDetectCounteredRivalAction(t *testing.T) {
	cfg := balance.Default()
	prev := model.GameState{}
	next := model.GameState{}
	next.Campaign.Reports = []model.BoardReport{{
		Cycle: 2,
		Entries: []model.CampaignReportEntry{
			{Kind: model.ReportRivalAction, SubjectID: "OpenAI", DetailID: "openai-flagship", Countered: true},
		},
	}}
	got := detectMoments(prev, next, cfg)
	if len(got) != 1 || !strings.Contains(got[0].Text, "反制奏效") {
		t.Fatalf("countered action should celebrate the counter, got %+v", got)
	}
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/tui/ -run 'TestDetect|TestNewReportEntries' -v`
Expected: FAIL — `undefined: detectMoments`

- [ ] **Step 3: 實作 feedback.go**

```go
// internal/tui/feedback.go
package tui

import (
	"fmt"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// MomentLevel grades feedback intensity.
type MomentLevel int

const (
	LevelMinor MomentLevel = iota // setNotice
	LevelMajor                    // 金色橫幅佇列
	LevelEpic                     // 全螢幕 overlay
)

// Moment is a detected feedback-worthy game event.
type Moment struct {
	Level MomentLevel
	Text  string
}

// detectMoments compares pre/post-tick states and returns feedback moments.
// Pure and read-only; never mutates sim truth.
func detectMoments(prev, next model.GameState, cfg balance.Config) []Moment {
	var out []Moment
	if prev.HasTraining && !next.HasTraining && len(next.Models) > len(prev.Models) {
		md := next.Models[len(next.Models)-1]
		out = append(out, Moment{LevelMajor,
			fmt.Sprintf("🧪 Gen%d 訓練完成！草稿已就緒——模型頁按 p 發佈", md.Gen)})
	}
	for i := prev.MilestonesReached; i < next.MilestonesReached && i < len(cfg.ValuationMilestones); i++ {
		out = append(out, Moment{LevelMajor,
			fmt.Sprintf("🏁 里程碑達成：估值 $%s！", human(cfg.ValuationMilestones[i]))})
	}
	for _, e := range newReportEntries(prev, next) {
		if mo, ok := reportMoment(e); ok {
			out = append(out, mo)
		}
	}
	return out
}

func reportMoment(e model.CampaignReportEntry) (Moment, bool) {
	switch e.Kind {
	case model.ReportStageAdvanced:
		return Moment{LevelMajor, "📈 階段推進：" + campaignStageLabel(model.CampaignStage(e.SubjectID))}, true
	case model.ReportShowdown:
		return Moment{LevelMajor, "⚔ 決勝開始！頂住主要宿敵 2 次攻勢即可奪下路線"}, true
	case model.ReportVictory:
		return Moment{LevelEpic, "🏆 路線勝利：" + doctrineLabel(model.Doctrine(e.SubjectID)) + "！總覽頁按 P 結算"}, true
	case model.ReportRivalAction:
		if e.Countered {
			return Moment{LevelMajor, "🛡 反制奏效：" + rivalActionLabel(e.DetailID) + " 衝擊減半！"}, true
		}
		return Moment{LevelMajor, "🚨 宿敵行動：" + rivalActionLabel(e.DetailID)}, true
	case model.ReportFinancialRisk:
		return Moment{LevelMajor, "🩸 財務風險：現金為負——董事會已提高關注"}, true
	}
	return Moment{}, false
}

// newReportEntries returns board-report entries added between prev and next.
// Reports are append-only per cycle (capped at 20 total), so compare against
// the previous last cycle + entry count.
func newReportEntries(prev, next model.GameState) []model.CampaignReportEntry {
	nr := next.Campaign.Reports
	if len(nr) == 0 {
		return nil
	}
	lastCycle := -1
	prevLastLen := 0
	if pr := prev.Campaign.Reports; len(pr) > 0 {
		lastCycle = pr[len(pr)-1].Cycle
		prevLastLen = len(pr[len(pr)-1].Entries)
	}
	var out []model.CampaignReportEntry
	for _, r := range nr {
		switch {
		case r.Cycle > lastCycle:
			out = append(out, r.Entries...)
		case r.Cycle == lastCycle && len(r.Entries) > prevLastLen:
			out = append(out, r.Entries[prevLastLen:]...)
		}
	}
	return out
}
```

- [ ] **Step 4: 跑測試確認通過**

Run: `go test ./internal/tui/ -run 'TestDetect|TestNewReportEntries' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/feedback.go internal/tui/feedback_test.go internal/model/campaign.go internal/sim/campaign_rivals.go
git commit -m "feat(tui): moment detector for celebration feedback

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 11: Major 金色橫幅佇列

**Files:**
- Modify: `internal/tui/tui.go`（Model 欄位、tick 整合、View、chromeRows）
- Modify: `internal/tui/display.go`（橫幅倒數遞減）
- Test: `internal/tui/feedback_test.go`（追加）

**Interfaces:**
- Consumes: Task 10 `Moment/detectMoments`。
- Produces:
  - Model 欄位 `banners []Moment`、`bannerTicks int`、`epic *Moment`
  - `func (m *Model) pushBanner(mo Moment)`；常數 `bannerShowTicks = 12`、`maxBanners = 8`
  - View 在 notice 之後顯示 `★ <text>` 金色行；chromeRows 佇列非空時 +1
  - Epic 時刻只設 `m.epic`（渲染在 Task 12）

- [ ] **Step 1: 寫失敗測試**

追加到 `internal/tui/feedback_test.go`：

```go
func TestPushBannerCapsQueue(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	for i := 0; i < 10; i++ {
		m.pushBanner(Moment{LevelMajor, fmt.Sprintf("b%d", i)})
	}
	if len(m.banners) != maxBanners {
		t.Fatalf("queue len = %d, want %d", len(m.banners), maxBanners)
	}
	if m.banners[0].Text != "b2" {
		t.Fatalf("oldest should be dropped, head = %q", m.banners[0].Text)
	}
}

func TestBannerFadesAfterTicks(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	m.pushBanner(Moment{LevelMajor, "hello"})
	for i := 0; i < bannerShowTicks; i++ {
		m.advanceDisplay()
	}
	if len(m.banners) != 0 {
		t.Fatalf("banner should fade after %d ticks, still %d queued", bannerShowTicks, len(m.banners))
	}
}

func TestViewShowsBanner(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	m.pushBanner(Moment{LevelMajor, "🏁 里程碑達成"})
	if out := m.View(); !strings.Contains(out, "里程碑達成") {
		t.Fatalf("View should show banner: %q", out)
	}
}
```

（import 需要 `fmt`、`filepath`——依測試檔慣例。）

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/tui/ -run 'TestPushBanner|TestBannerFades|TestViewShowsBanner' -v`
Expected: FAIL — `m.pushBanner undefined`

- [ ] **Step 3: 實作**

常數與 Model 欄位（tui.go）：

```go
const (
	bannerShowTicks = 12 // 每條 Major 橫幅顯示 ~3s
	maxBanners      = 8  // 佇列上限，超出丟最舊
)
```

```go
	// Celebration feedback (TUI state, never persisted).
	banners     []Moment
	bannerTicks int
	epic        *Moment
```

```go
// pushBanner queues a Major banner, dropping the oldest beyond maxBanners.
func (m *Model) pushBanner(mo Moment) {
	if len(m.banners) >= maxBanners {
		m.banners = m.banners[1:]
	}
	m.banners = append(m.banners, mo)
	if len(m.banners) == 1 {
		m.bannerTicks = bannerShowTicks
	}
}
```

tick 整合（`case tickMsg` 內）：在 `cfgTick := m.cfg` 之前捕捉 `prevState := m.state`；在 `m, _ = m.advanceCampaignTo(now.Unix())` 之後、既有 `prevFired` 通知之後加：

```go
		for _, mo := range detectMoments(prevState, m.state, m.cfg) {
			switch mo.Level {
			case LevelMinor:
				m.setNotice(mo.Text)
			case LevelMajor:
				m.pushBanner(mo)
			case LevelEpic:
				mo := mo
				m.epic = &mo
			}
		}
```

（注意：既有 `prevFired := m.state.Events.FiredCount` 行保留原位——它必須在 `sim.Tick` 之前取值，`prevState` 同理放在 `sim.Tick` 之前。）

橫幅倒數（display.go `advanceDisplay` 尾端）：

```go
	if len(m.banners) > 0 {
		m.bannerTicks--
		if m.bannerTicks <= 0 {
			m.banners = m.banners[1:]
			if len(m.banners) > 0 {
				m.bannerTicks = bannerShowTicks
			}
		}
	}
```

View（tui.go，notice 區塊之後、campaignError 之前）：

```go
	if len(m.banners) > 0 {
		top = append(top, styleGold.Bold(true).Render("★ "+m.banners[0].Text))
	}
```

chromeRows（tui.go:887-905）對應加：

```go
	if len(m.banners) > 0 {
		n++
	}
```

- [ ] **Step 4: 跑測試確認通過**

Run: `go test ./internal/tui/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): gold major-moment banner queue

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 12: Epic 全螢幕 overlay

**Files:**
- Modify: `internal/tui/tui.go`（contentBody、KeyMsg dismiss）
- Create: `internal/tui/epic.go`
- Test: `internal/tui/feedback_test.go`（追加）

**Interfaces:**
- Consumes: Task 11 `m.epic`。
- Produces:
  - `func renderEpicOverlay(mo Moment, m Model) string` — 金框置中卡
  - contentBody 最優先分支；任意鍵先清 `m.epic` 再繼續正常流程

- [ ] **Step 1: 寫失敗測試**

追加到 `internal/tui/feedback_test.go`：

```go
func TestEpicOverlayRendersAndDismisses(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	mo := Moment{LevelEpic, "🏆 路線勝利：消費者霸主！"}
	m.epic = &mo
	if out := m.View(); !strings.Contains(out, "路線勝利") {
		t.Fatalf("epic overlay missing: %q", out)
	}
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = mm.(Model)
	if m.epic != nil {
		t.Fatal("any key should dismiss epic overlay")
	}
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/tui/ -run TestEpicOverlay -v`
Expected: FAIL — overlay 未渲染 / dismiss 未實作

- [ ] **Step 3: 實作 epic.go**

```go
// internal/tui/epic.go
package tui

import "github.com/charmbracelet/lipgloss"

// renderEpicOverlay fills the content region with a centered gold celebration.
func renderEpicOverlay(mo Moment, m Model) string {
	inner := VStack(
		"",
		styleGold.Bold(true).Render(mo.Text),
		"",
		styleMuted.Render("按任意鍵繼續"),
		"",
	)
	card := CardIn(CardGold, 0, "🏆 榮耀時刻", inner)
	h := m.vp.Height
	if h < lipgloss.Height(card) {
		h = lipgloss.Height(card)
	}
	return lipgloss.Place(m.contentWidth(), h, lipgloss.Center, lipgloss.Center, card)
}
```

contentBody（tui.go:908）最頂端加：

```go
	if m.epic != nil {
		return renderEpicOverlay(*m.epic, m)
	}
```

KeyMsg 處理（tui.go:413，campaign dialog 檢查之前）加：

```go
		if m.epic != nil {
			m.epic = nil
			return m, nil
		}
```

- [ ] **Step 4: 跑測試確認通過**

Run: `go test ./internal/tui/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): epic full-screen celebration overlay

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 13: applyNotice 統一成功回饋

**Files:**
- Modify: `internal/tui/tui.go`（新 helper + 呼叫端改造）
- Test: `internal/tui/tui_test.go`（追加）

**Interfaces:**
- Consumes: `setNotice`。
- Produces: `func (m *Model) applyNotice(cmd model.Command, okMsg string)` — 成功套用且 `okMsg != ""` 時 setNotice；失敗維持無聲 no-op。既有 `applyOK` 保留給不需回饋的呼叫端（P 鍵 PrestigeReset 分支）。

呼叫端與訊息對照（全部在 tui.go 的按鍵 switch）：

| 位置 | 指令 | okMsg |
|---|---|---|
| `case "r","R","i","I"` | RentCompute | `""`（連打調整，保持無聲） |
| `case "b","B"` | BuildServer | `🏗 伺服器建造完成` |
| `case "e"`（Compute） | ExpandDatacenter | `🏗 機房擴建完成` |
| `case "e"`（Team） | HireStaff 工程 | `🤝 已雇用工程師` |
| `case "h"` | HireStaff 研究員 | `🤝 已雇用研究員` |
| `case "o"` | HireStaff 營運 | `🤝 已雇用營運` |
| `case "k"` | HireStaff 行銷 | `🤝 已雇用行銷` |
| `case "s"` | SignStar | `🌟 已簽下明星員工` |
| `case "enter"`（Tech，成功分支） | UnlockTech | `🔬 已解鎖：` + techLabel(node.ID).Name |
| updateEventDialog 成功分支 | ResolveEvent | `✓ 事件已決議` |
| updateDialog（開訓）成功分支 | StartTraining | `🚂 訓練已啟動` |

- [ ] **Step 1: 寫失敗測試**

追加到 `internal/tui/tui_test.go`：

```go
func TestHireShowsSuccessNotice(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	m.state.Resources.Cash = 1e9
	m.page = PageTeam
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = mm.(Model)
	if !strings.Contains(m.notice, "已雇用研究員") {
		t.Fatalf("hire should set success notice, got %q", m.notice)
	}
}

func TestTechUnlockShowsName(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	m.state.Resources.RnD = 1e12
	m.page = PageTech
	m.techCursor = 0
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(Model)
	if !strings.Contains(m.notice, "已解鎖") {
		t.Fatalf("tech unlock should set notice, got %q", m.notice)
	}
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/tui/ -run 'TestHireShows|TestTechUnlockShows' -v`
Expected: FAIL — notice 為空

- [ ] **Step 3: 實作**

helper（applyOK 旁）：

```go
// applyNotice applies cmd; on success shows okMsg (empty = silent success).
// Rejected commands stay silent no-ops, same as applyOK.
func (m *Model) applyNotice(cmd model.Command, okMsg string) {
	ns, err := sim.Apply(m.state, cmd, m.cfg)
	if err != nil {
		return
	}
	m.state = ns
	if okMsg != "" {
		m.setNotice(okMsg)
	}
}
```

依對照表改造呼叫端，例如：

```go
		case "h":
			if m.page == PageTeam {
				m.applyNotice(model.HireStaff{Role: model.RoleResearcher, Tier: model.Tier1, Count: 1}, "🤝 已雇用研究員")
			}
			return m, nil
```

Tech 成功分支（`case "enter"` 的 `case err == nil:`）：

```go
				case err == nil:
					m.state = ns
					m.setNotice("🔬 已解鎖：" + techLabel(node.ID).Name)
```

（`model.TechNode` 沒有顯示名欄位；顯示名一律走 tui 側的 `techLabel(id).Name`，見 `tech_meta.go:23`。）

updateEventDialog 成功分支（`case err == nil:` 之後）加 `m.setNotice("✓ 事件已決議")`；updateDialog confirm 分支把 `m.state = applyOK(...)` 換成 `m.applyNotice(d.command(m.cfg), "🚂 訓練已啟動")`。

RentCompute 用 `m.applyNotice(..., "")`；`case "P"` 的 `applyOK(m.state, model.PrestigeReset{}, ...)` 保持不動。全部改完後若 `applyOK` 已無呼叫端，保留函式與註解（P 鍵仍用）。

- [ ] **Step 4: 跑測試確認通過**

Run: `go test ./internal/tui/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): success notices for all player actions

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 14: 決勝守成 UI + 威脅閃爍 + 財務警報

**Files:**
- Modify: `internal/tui/tui.go`（Model 加 `blink bool`；tick 翻轉）
- Modify: `internal/tui/campaign_meta.go`（狀態卡決勝行、宿敵倒數紅閃）
- Modify: `internal/tui/tui.go:1134-1178`（pressures 財務行升級）
- Test: `internal/tui/campaign_meta_test.go`（追加）

**Interfaces:**
- Consumes: `model.CampaignState.ShowdownHeld / ShowdownAttempts`（既有欄位，目前無 UI）、Task 1 變體。
- Produces: Model 欄位 `blink bool`（每 tick 翻轉，測試可直接設值）。決勝期間狀態卡轉 `CardThreat` 並顯示 `⚔ 決勝中——已頂住 X/2 次宿敵攻勢`。

- [ ] **Step 1: 寫失敗測試**

追加到 `internal/tui/campaign_meta_test.go`：

```go
func TestShowdownProgressVisible(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	m.state.Campaign.Doctrine = model.DoctrineConsumer
	m.state.Campaign.Stage = model.CampaignStageShowdown
	m.state.Campaign.ShowdownHeld = 1
	out := renderCampaignStatusCard(m, 60)
	if !strings.Contains(out, "1/2") {
		t.Fatalf("showdown held progress missing: %q", out)
	}
	if !strings.Contains(out, "決勝中") {
		t.Fatalf("showdown banner missing: %q", out)
	}
}

func TestShowdownRetryCounterVisible(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	m.state.Campaign.Doctrine = model.DoctrineConsumer
	m.state.Campaign.Stage = model.CampaignStageShowdown
	m.state.Campaign.ShowdownAttempts = 2
	out := renderCampaignStatusCard(m, 60)
	if !strings.Contains(out, "第 3 次嘗試") {
		t.Fatalf("retry counter missing: %q", out)
	}
}
```

（`sim.CampaignStatus` 需要 doctrine 已選才 Active——若測試中 status.Active 為 false 走了「未選戰略」分支，改為直接設好最小合法 Campaign state；以 `dialog_campaign_end_test.go` 既有的 campaign fixture 寫法為準。）

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/tui/ -run TestShowdown -v`
Expected: FAIL — 輸出無決勝行

- [ ] **Step 3: 實作**

Model 欄位＋tick 翻轉（`case tickMsg` 內，`m.advanceDisplay()` 旁）：

```go
	blink bool // 每 tick 翻轉；威脅行明暗交替
```

```go
		m.blink = !m.blink
```

`renderCampaignStatusCard`（campaign_meta.go）在 perk/victory 行之前加決勝區塊，並讓卡片變體隨階段切換：

```go
	kind := CardAccent
	if status.Stage == model.CampaignStageShowdown {
		kind = CardThreat
		line := fmt.Sprintf("⚔ 決勝中——已頂住 %d/2 次宿敵攻勢", camp.ShowdownHeld)
		if camp.ShowdownAttempts > 0 {
			line += fmt.Sprintf("（第 %d 次嘗試）", camp.ShowdownAttempts+1)
		}
		st := styleLoss.Bold(true)
		if m.blink {
			st = styleAmber.Bold(true)
		}
		lines = append(lines, st.Render(line))
	}
	// ...既有 victory 行...
	return CardIn(kind, w, "公司戰略", VStack(lines...))
```

宿敵倒數紅閃（`renderRivalIntelBlock`——簽名加 `blink bool`，呼叫端 `renderRivalRoadmapCard` 傳 `m.blink`）：

```go
	if intel.ConfirmedActionID != "" {
		line := fmt.Sprintf("  已確認 %s · %d 週期後",
			rivalActionLabel(intel.ConfirmedActionID), intel.CyclesUntilAction)
		if intel.CyclesUntilAction <= 1 {
			st := styleLoss.Bold(true)
			if blink {
				st = styleAmber.Bold(true)
			}
			line = st.Render(line + " ⚠")
		}
		lines = append(lines, line)
		// ...detail 行照舊...
	}
```

pressures 財務行（tui.go pressures 內找到財務困境項，若無則加）：

```go
	if c := s.Campaign.FinancialDistressCycles; c >= 1 {
		out = append(out, styleLoss.Bold(true).Render(
			fmt.Sprintf("🩸 財務危機 第 %d 週期——連續 2 週期可策略退出 [E]", c)))
	}
```

- [ ] **Step 4: 跑測試確認通過**

Run: `go test ./internal/tui/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): showdown hold UI, rival countdown flash, distress alert

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 15: 離線戰報卡

**Files:**
- Modify: `internal/tui/tui.go:181-241`（startup 捕捉離線期間新報告）
- Modify: `internal/tui/tui.go:1209-1262`（View 用新卡、刪 offlineBanner）
- Modify: `internal/tui/tui.go:887-905`（chromeRows 量測多行卡高度）
- Test: `internal/tui/tui_test.go`（追加）

**Interfaces:**
- Consumes: Task 10 `newReportEntries`、`formatReportEntry`（campaign_meta.go:242 既有）、Task 1 `CardIn`。
- Produces:
  - Model 欄位 `offlineReports []string`（最多 4 行，任意鍵與 offlineSummary 一起清除）
  - `func renderOfflineReport(m Model) string` — 取代單行 `offlineBanner`

- [ ] **Step 1: 寫失敗測試**

追加到 `internal/tui/tui_test.go`：

```go
func TestOfflineReportCardRenders(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	m.offlineSummary = &Summary{
		SecondsSettled:    7200,
		TokensIn:          1000,
		TokensOut:         2000,
		RnDGained:         500,
		TrainingCompleted: true,
		CampaignCycles:    2,
	}
	m.offlineReports = []string{"· 宿敵行動 OpenAI · OpenAI 消費旗艦"}
	out := renderOfflineReport(m)
	for _, want := range []string{"離線戰報", "2.0h", "訓練完成", "董事會週期 2 次", "宿敵行動"} {
		if !strings.Contains(out, want) {
			t.Fatalf("offline report missing %q: %q", want, out)
		}
	}
	// View 顯示且任意鍵清除
	if v := m.View(); !strings.Contains(v, "離線戰報") {
		t.Fatal("View should embed offline report")
	}
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = mm.(Model)
	if m.offlineSummary != nil || m.offlineReports != nil {
		t.Fatal("any key should clear offline report")
	}
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/tui/ -run TestOfflineReportCard -v`
Expected: FAIL — `undefined: renderOfflineReport`

- [ ] **Step 3: 實作**

Model 欄位（offlineSummary 旁）：

```go
	offlineReports []string // 離線期間新增的董事會報告行（最多 4）
```

`renderOfflineReport`（取代 tui.go:1244-1262 的 `offlineBanner`，整個函式刪除）：

```go
// renderOfflineReport summarises what happened while the game was closed.
func renderOfflineReport(m Model) string {
	s := *m.offlineSummary
	lines := []string{fmt.Sprintf("💤 離開 %.1fh · 寫了 %d tokens → +%s R&D",
		s.SecondsSettled/3600, s.TokensIn+s.TokensOut, human(s.RnDGained))}
	if s.TrainingCompleted {
		lines = append(lines, styleGain.Render("🧪 訓練完成 ✓"))
	}
	if s.EventsFired > 0 {
		ev := fmt.Sprintf("📰 產業事件 %d 起", s.EventsFired)
		if s.EventsAutoResolved > 0 {
			ev += fmt.Sprintf("（%d 起已自動決議）", s.EventsAutoResolved)
		}
		lines = append(lines, ev)
	} else if s.EventsAutoResolved > 0 {
		lines = append(lines, fmt.Sprintf("📰 %d 起待決事件已自動決議", s.EventsAutoResolved))
	}
	if s.CampaignCycles > 0 {
		lines = append(lines, fmt.Sprintf("🏛 董事會週期 %d 次", s.CampaignCycles))
	}
	lines = append(lines, m.offlineReports...)
	lines = append(lines, styleMuted.Render("按任意鍵關閉"))
	return CardIn(CardAccent, 0, "離線戰報", VStack(lines...))
}
```

startup 捕捉（tui.go:214-241 board-cycle 補算區塊）：在 `m, advanced = m.advanceCampaignTo(now)` 這行前捕捉 `preCampaign := m.state`，其後加：

```go
	if advanced > 0 {
		for _, e := range newReportEntries(preCampaign, m.state) {
			if len(m.offlineReports) >= 4 {
				break
			}
			m.offlineReports = append(m.offlineReports, formatReportEntry(e))
		}
	}
```

View（tui.go:1218-1220）：`offlineBanner(*m.offlineSummary)` 改 `renderOfflineReport(m)`。

任意鍵清除（tui.go:444）：`m.offlineSummary = nil` 改為：

```go
		m.offlineSummary = nil // any key dismisses the transient banners
		m.offlineReports = nil
```

chromeRows（tui.go:890-892）：

```go
	if m.offlineSummary != nil {
		n += lipgloss.Height(renderOfflineReport(m))
	}
```

（daemon_integration_test.go 若斷言舊單行 `💤 離開` 格式，更新為新卡片內容斷言——文案關鍵字 `離開`、`R&D` 不變。）

- [ ] **Step 4: 跑測試確認通過**

Run: `go test ./internal/tui/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): multi-line offline war report card

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 16: 全量迴歸與視覺驗收

**Files:**
- Modify: 無新增（僅修迴歸）
- Test: 全 repo

- [ ] **Step 1: 全量測試與 vet**

Run: `go test ./... && go vet ./...`
Expected: 全綠。任何失敗回到對應任務修正。

- [ ] **Step 2: 視覺 snapshot 抽查**

建立臨時檔 `internal/tui/zz_capture_test.go`（結尾刪除，不 commit）：

```go
package tui

import (
	"fmt"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestZZCapture(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 110, Height: 42})
	m = mm.(Model)
	for i := 0; i < int(numPages); i++ {
		fmt.Printf("\n===== PAGE %d =====\n%s\n", i, m.View())
		mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = mm.(Model)
	}
}
```

Run: `go test ./internal/tui -run TestZZCapture -v | head -300`
檢查：卡片右緣齊平（無鋸齒）、進度條為 `█`、tab 反白、資源列有 `│` 分段。確認後 `rm internal/tui/zz_capture_test.go`。

- [ ] **Step 3: 更新 spec 狀態**

`docs/superpowers/specs/2026-07-11-tui-warroom-gamification-design.md` 狀態行改為：
`狀態：Phase 1-3 已實作（本 plan）；Phase 4-6 待後續 plan`

- [ ] **Step 4: Commit**

```bash
git add docs/superpowers/specs/2026-07-11-tui-warroom-gamification-design.md
git commit -m "docs: mark spec phases 1-3 implemented

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```
