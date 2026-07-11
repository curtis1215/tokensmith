# Tokensmith TUI 作戰室改造（Phase 4-6）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Phase 1-3 的 HUD 與慶祝系統之上補完遊戲化機制：跨局成就系統（第 7 頁徽章牆）、ASCII 總部成長視覺、Prestige 儀式化結算與傳承開局。

**Architecture:** 成就存 `meta.json`（跨 prestige 保留，玩家層），目錄與檢查函式在 tui 側靜態表；解鎖走 Phase 3 的 Major 金色橫幅。ASCII 總部是純渲染（`MilestonesReached` 選階段、`m.blink` 微動畫）。Prestige 儀式化重用 Phase 3 的 Epic overlay 通道（`Moment` 加 `Title` 欄位）。sim 層零改動。

**Tech Stack:** Go 1.25、charmbracelet bubbletea v1.3.10、lipgloss v1.1.0（既有依賴，禁止新增）。

**Spec:** `docs/superpowers/specs/2026-07-11-tui-warroom-gamification-design.md`（Phase 4-6 節）

## Global Constraints

- 零新依賴；sim 層（internal/sim、internal/model、internal/game、internal/balance）**本 plan 零改動**。
- 顏色一律 hex `lipgloss.Color`，colorprofile 自動降級；測試無 TTY 色碼被剝除——**測試只斷言純文字與寬度**。
- 所有玩家可見文案繁體中文。
- 寬度計算用 `lipgloss.Width`；動畫沿用 250ms tick 與既有 `m.blink`。
- 每個 commit 訊息尾端：`Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`
- 測試：`go test ./internal/tui/ ./internal/store/`；全量 `go test ./...`。
- Phase 1-3 已落地的介面（直接使用，勿重造）：`CardIn/CardKind`、`GoldBar`、`Moment/MomentLevel/pushBanner/m.epic`、`renderEpicOverlay`、`m.blink`、`techLabel`、`doctrineLabel`、`legacyChoiceLabel`。

---

### Task 1: Meta.Achievements 持久化與 Model 鏡像

**Files:**
- Modify: `internal/store/meta.go:17-34`（Meta 加欄位）
- Modify: `internal/tui/tui.go`（Model 加 `achievements` 鏡像；`newAtPaths` 載入；`saveMetaAt` 寫回）
- Test: `internal/store/store_test.go`（或既有 meta 測試檔，roundtrip）、`internal/tui/tui_test.go`

**Interfaces:**
- Consumes: 既有 `store.SaveMeta/LoadMeta`、`saveMetaAt`（tui.go:348）。
- Produces:
  - `store.Meta.Achievements map[string]int64`（json:"achievements,omitempty"；id → unlockedAt unix；舊檔載入為 nil = 全未解鎖）
  - Model 欄位 `achievements map[string]int64`（記憶體鏡像，saveMetaAt 寫回）

- [ ] **Step 1: 寫失敗測試**

store 側（放進既有 store 測試檔，依其命名慣例）：

```go
func TestMetaAchievementsRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.json")
	in := Meta{Achievements: map[string]int64{"first-online": 1700000000}}
	if err := SaveMeta(path, in); err != nil {
		t.Fatal(err)
	}
	out, ok, err := LoadMeta(path)
	if err != nil || !ok {
		t.Fatalf("load failed: %v ok=%v", err, ok)
	}
	if out.Achievements["first-online"] != 1700000000 {
		t.Fatalf("achievements lost in roundtrip: %+v", out.Achievements)
	}
}

func TestMetaOldFileNilAchievements(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.json")
	if err := os.WriteFile(path, []byte(`{"streakDays":3}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, ok, _ := LoadMeta(path)
	if !ok || out.Achievements != nil {
		t.Fatalf("old meta should load with nil achievements, got %+v", out.Achievements)
	}
}
```

tui 側（tui_test.go 追加）：

```go
func TestSaveMetaPersistsAchievements(t *testing.T) {
	dir := t.TempDir()
	m := newAt(filepath.Join(dir, "save.json"))
	m.achievements = map[string]int64{"streak-3": 42}
	m.saveMetaAt(100)
	meta, ok, _ := store.LoadMeta(filepath.Join(dir, "meta.json"))
	if !ok || meta.Achievements["streak-3"] != 42 {
		t.Fatalf("achievements not persisted: %+v", meta.Achievements)
	}
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/store/ ./internal/tui/ -run 'TestMeta|TestSaveMetaPersists' -v`
Expected: FAIL — `unknown field Achievements` / `m.achievements undefined`

- [ ] **Step 3: 實作**

`store/meta.go` 的 Meta 加欄位（LastCampaignCycle 之後）：

```go
	// Achievements maps achievement id → unlockedAt (unix seconds). Player-level
	// and cross-prestige: survives every run reset. Nil on old meta files.
	Achievements map[string]int64 `json:"achievements,omitempty"`
```

`tui.go` Model 加欄位（streakDays 鏡像區旁）：

```go
	achievements map[string]int64 // mirrors store.Meta.Achievements
```

`newAtPaths` 讀 meta 後（`streakDays: meta.StreakDays` 同一個建構區）加：

```go
		achievements:      meta.Achievements,
```

`saveMetaAt`（tui.go:348）的 store.Meta 組裝加：

```go
		Achievements:      m.achievements,
```

- [ ] **Step 4: 跑測試確認通過**

Run: `go test ./internal/store/ ./internal/tui/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/ internal/tui/
git commit -m "feat(tui): persist achievements in meta.json

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: 成就目錄（29 項）與檢查函式

**Files:**
- Create: `internal/tui/achievements_meta.go`
- Test: `internal/tui/achievements_meta_test.go`

**Interfaces:**
- Consumes: `model.GameState` 欄位、`m.streakDays`、`m.consumed`（`sumSourceTotals`）、`m.cfg.Stars`、`sim.MarketRank`。
- Produces:
  - `type achievement struct { ID, Name, Desc string; Check func(m Model) bool }`
  - `var achievementCatalog []achievement` — 26 項，順序即徽章牆顯示序
  - `var achievementCategories []struct{ Title string; From, To int }` — 分類切片索引（進度/習慣/經營/戰役/輪迴）

- [ ] **Step 1: 寫失敗測試**

```go
// internal/tui/achievements_meta_test.go
package tui

import (
	"path/filepath"
	"testing"

	"tokensmith/internal/model"
)

func TestAchievementCatalogWellFormed(t *testing.T) {
	if len(achievementCatalog) < 25 {
		t.Fatalf("catalog too small: %d", len(achievementCatalog))
	}
	seen := map[string]bool{}
	for _, a := range achievementCatalog {
		if a.ID == "" || a.Name == "" || a.Desc == "" || a.Check == nil {
			t.Fatalf("malformed achievement: %+v", a)
		}
		if seen[a.ID] {
			t.Fatalf("duplicate id %q", a.ID)
		}
		seen[a.ID] = true
	}
}

func TestAchievementChecksOnFreshGame(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	for _, a := range achievementCatalog {
		if a.Check(m) {
			t.Fatalf("fresh game should unlock nothing, but %q fired", a.ID)
		}
	}
}

func TestAchievementChecksFire(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	m.state.Models = []model.Model{{Gen: 2, Online: true}}
	m.state.MilestonesReached = 4
	m.streakDays = 7
	m.state.Campaign.Doctrine = model.DoctrineConsumer
	m.state.Prestige.Patents = 12
	fired := map[string]bool{}
	for _, a := range achievementCatalog {
		if a.Check(m) {
			fired[a.ID] = true
		}
	}
	for _, want := range []string{"first-online", "gen-2", "ms-1m", "ms-1b", "streak-3", "streak-7", "doctrine-chosen", "prestige-first", "patents-10"} {
		if !fired[want] {
			t.Fatalf("expected %q to fire, fired set: %v", want, fired)
		}
	}
	if fired["gen-5"] || fired["streak-10"] || fired["ms-1t"] {
		t.Fatalf("over-firing: %v", fired)
	}
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/tui/ -run TestAchievement -v`
Expected: FAIL — `undefined: achievementCatalog`

- [ ] **Step 3: 實作 achievements_meta.go**

```go
// internal/tui/achievements_meta.go
package tui

import (
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

// achievement is a player-level badge; Check is a pure read over Model.
type achievement struct {
	ID    string
	Name  string
	Desc  string
	Check func(m Model) bool
}

func maxTrainedGen(s model.GameState) int {
	g := 0
	for _, md := range s.Models {
		if md.Gen > g {
			g = md.Gen
		}
	}
	return g
}

func anyOnline(s model.GameState) bool {
	for _, md := range s.Models {
		if md.Online {
			return true
		}
	}
	return false
}

func totalTokens(m Model) int {
	in, out := sumSourceTotals(m.consumed)
	return in + out
}

func anyCountered(s model.GameState) bool {
	for _, r := range s.Campaign.Reports {
		for _, e := range r.Entries {
			if e.Countered {
				return true
			}
		}
	}
	return false
}

func distinctBadges(s model.GameState) int {
	seen := map[model.Doctrine]bool{}
	for _, d := range s.Prestige.RouteBadges {
		seen[d] = true
	}
	return len(seen)
}

func milestoneCheck(n int) func(Model) bool {
	return func(m Model) bool { return m.state.MilestonesReached >= n }
}

func genCheck(g int) func(Model) bool {
	return func(m Model) bool { return maxTrainedGen(m.state) >= g }
}

var achievementCatalog = []achievement{
	// 進度（0-11）
	{"first-online", "首航", "第一個模型上線", func(m Model) bool { return anyOnline(m.state) }},
	{"gen-2", "第二世代", "訓練出 Gen2 模型", genCheck(2)},
	{"gen-3", "第三世代", "訓練出 Gen3 模型", genCheck(3)},
	{"gen-4", "Gen4 大師", "訓練出 Gen4 模型", genCheck(4)},
	{"gen-5", "Gen5 神話", "訓練出 Gen5 模型", genCheck(5)},
	{"ms-1m", "百萬俱樂部", "估值達 $1M", milestoneCheck(1)},
	{"ms-10m", "千萬格局", "估值達 $10M", milestoneCheck(2)},
	{"ms-100m", "億級玩家", "估值達 $100M", milestoneCheck(3)},
	{"ms-1b", "獨角獸", "估值達 $1B", milestoneCheck(4)},
	{"ms-10b", "十倍獨角獸", "估值達 $10B", milestoneCheck(5)},
	{"ms-100b", "科技巨頭", "估值達 $100B", milestoneCheck(6)},
	{"ms-1t", "兆元傳說", "估值達 $1T", milestoneCheck(7)},
	// 習慣（12-17）
	{"streak-3", "三日連寫", "連續寫程式 3 天", func(m Model) bool { return m.streakDays >= 3 }},
	{"streak-7", "七日成習", "連續寫程式 7 天", func(m Model) bool { return m.streakDays >= 7 }},
	{"streak-10", "十日爐火", "連續寫程式 10 天（加成封頂）", func(m Model) bool { return m.streakDays >= 10 }},
	{"tokens-1m", "百萬鍛造", "累計收成 1M tokens", func(m Model) bool { return totalTokens(m) >= 1_000_000 }},
	{"tokens-10m", "千萬鍛造", "累計收成 10M tokens", func(m Model) bool { return totalTokens(m) >= 10_000_000 }},
	{"tokens-100m", "億級鍛造", "累計收成 100M tokens", func(m Model) bool { return totalTokens(m) >= 100_000_000 }},
	// 經營（18-21）
	{"star-first", "首位明星", "簽下第一位明星員工", func(m Model) bool { return len(m.state.HiredStars) >= 1 }},
	{"star-all", "全明星陣容", "簽下所有明星員工", func(m Model) bool {
		return len(m.cfg.Stars) > 0 && len(m.state.HiredStars) >= len(m.cfg.Stars)
	}},
	{"team-full", "四職能齊備", "研究/工程/營運/行銷都有人", func(m Model) bool {
		s := m.state
		r := 0
		for _, n := range s.Research.Researchers {
			r += n
		}
		return r > 0 && s.Engineers > 0 && s.Ops > 0 && s.Marketing > 0
	}},
	{"triple-crown", "三冠王", "三個市場同時排名第一", func(m Model) bool {
		for seg := 0; seg < model.NumSegments; seg++ {
			if rank, _ := sim.MarketRank(m.state, m.cfg, model.Segment(seg)); rank != 1 {
				return false
			}
		}
		return true
	}},
	// 戰役（22-25）
	{"doctrine-chosen", "定調", "選定第一個公司戰略", func(m Model) bool { return m.state.Campaign.Doctrine != model.DoctrineNone }},
	{"showdown-win", "決勝守成", "頂住宿敵攻勢贏得路線勝利", func(m Model) bool {
		return m.state.Campaign.Victory != model.DoctrineNone || m.state.Campaign.Stage == model.CampaignStageWon
	}},
	{"counter-hit", "反制奏效", "成功反制一次宿敵行動", func(m Model) bool { return anyCountered(m.state) }},
	{"endless", "無盡征途", "進入無盡模式", func(m Model) bool { return m.state.Campaign.Endless }},
	// 輪迴（26-28）
	{"prestige-first", "首次傳承", "完成第一次傳承重開", func(m Model) bool { return m.state.Prestige.Patents > 0 }},
	{"patents-10", "專利大戶", "累計專利達 10", func(m Model) bool { return m.state.Prestige.Patents >= 10 }},
	{"badge-grand-slam", "路線大滿貫", "三條路線徽章集滿", func(m Model) bool { return distinctBadges(m.state) >= 3 }},
}

// achievementCategories slices the catalog for the badge-wall page.
var achievementCategories = []struct {
	Title    string
	From, To int // [From, To)
}{
	{"進度", 0, 12},
	{"習慣", 12, 18},
	{"經營", 18, 22},
	{"戰役", 22, 26},
	{"輪迴", 26, 29},
}
```

- [ ] **Step 4: 跑測試確認通過**

Run: `go test ./internal/tui/ -run TestAchievement -v`
Expected: PASS（若 fresh-game 測試因 `doctrine-chosen` 等在新局誤觸發而失敗，檢查是 fixture 髒掉還是 Check 條件寫錯——新局 `Doctrine == DoctrineNone`、`Models` 空、`Patents == 0`，全部 Check 都應為 false）

- [ ] **Step 5: Commit**

```bash
git add internal/tui/achievements_meta.go internal/tui/achievements_meta_test.go
git commit -m "feat(tui): achievement catalog with 29 badges

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: 成就引擎（解鎖偵測 → 金色橫幅）

**Files:**
- Modify: `internal/tui/tui.go`（tickMsg 內每 8 tick 檢查；`checkAchievements` helper）
- Test: `internal/tui/achievements_meta_test.go`（追加）

**Interfaces:**
- Consumes: Task 1 `m.achievements`、Task 2 `achievementCatalog`、Phase 3 `pushBanner`。
- Produces: `func (m *Model) checkAchievements(nowUnix int64)` — 未解鎖項跑 Check，命中→寫 map + `pushBanner(Moment{Level: LevelMajor, Text: "🏆 成就解鎖：" + Name})`。持久化依靠既有 40-tick autosave 與離開時存檔（解鎖後最遲 10 秒落盤；重複解鎖冪等）。

- [ ] **Step 1: 寫失敗測試**

```go
func TestCheckAchievementsUnlocksAndBanners(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	m.streakDays = 3
	m.checkAchievements(1234)
	if m.achievements["streak-3"] != 1234 {
		t.Fatalf("streak-3 not unlocked: %+v", m.achievements)
	}
	if len(m.banners) == 0 || !strings.Contains(m.banners[0].Text, "成就解鎖") {
		t.Fatalf("unlock should push banner: %+v", m.banners)
	}
	// 冪等：再跑一次不重複解鎖、不重複 banner
	n := len(m.banners)
	m.checkAchievements(9999)
	if m.achievements["streak-3"] != 1234 || len(m.banners) != n {
		t.Fatal("re-check must be idempotent")
	}
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/tui/ -run TestCheckAchievements -v`
Expected: FAIL — `m.checkAchievements undefined`

- [ ] **Step 3: 實作**

`tui.go`（achievements 相關 helper 區）：

```go
// checkAchievements unlocks any newly satisfied achievements. Idempotent;
// persistence rides the regular autosave/quit meta writes.
func (m *Model) checkAchievements(nowUnix int64) {
	for _, a := range achievementCatalog {
		if _, done := m.achievements[a.ID]; done {
			continue
		}
		if !a.Check(*m) {
			continue
		}
		if m.achievements == nil {
			m.achievements = make(map[string]int64)
		}
		m.achievements[a.ID] = nowUnix
		m.pushBanner(Moment{Level: LevelMajor, Text: "🏆 成就解鎖：" + a.Name + "——" + a.Desc})
	}
}
```

tickMsg 內（`m.advanceDisplay()` 之後）：

```go
		if m.sparkTick%8 == 0 {
			m.checkAchievements(now.Unix())
		}
```

- [ ] **Step 4: 跑測試確認通過**

Run: `go test ./internal/tui/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): achievement unlock engine with gold banners

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: 第 7 頁「成就」徽章牆

**Files:**
- Create: `internal/tui/page_achievements.go`
- Modify: `internal/tui/tui.go:44-54`（Page 常數 + pageNames）、`tui.go` 按鍵 `case "1"..."6"` 加 `"7"`、`renderPage` switch、`pageKeys`
- Test: `internal/tui/page_achievements_test.go`

**Interfaces:**
- Consumes: Task 2 目錄與分類、Task 1 `m.achievements`、`GoldBar`、`CardIn`。
- Produces:
  - `PageAchievements Page`（插在 `PageTech` 之後、`numPages` 之前）
  - `func renderAchievements(m Model) string` — 頂部金色總進度條 + 五分類卡；已解鎖 `styleGold`＋🏆＋日期、未解鎖 `styleMuted`＋🔒＋條件

- [ ] **Step 1: 寫失敗測試**

```go
// internal/tui/page_achievements_test.go
package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestAchievementsPageRenders(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 110, Height: 40})
	m = mm.(Model)
	m.achievements = map[string]int64{"first-online": 1751500000}
	out := renderAchievements(m)
	for _, want := range []string{"成就進度", "1/29", "🏆 首航", "🔒", "進度", "輪迴"} {
		if !strings.Contains(out, want) {
			t.Fatalf("achievements page missing %q", want)
		}
	}
}

func TestAchievementsPageReachableByKey7(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'7'}})
	m = mm.(Model)
	if m.page != PageAchievements {
		t.Fatalf("key 7 should open achievements page, got %v", m.page)
	}
	if !strings.Contains(m.View(), "成就進度") {
		t.Fatal("View should render achievements page")
	}
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/tui/ -run TestAchievementsPage -v`
Expected: FAIL — `undefined: renderAchievements` / `undefined: PageAchievements`

- [ ] **Step 3: 實作**

`tui.go` Page 常數與 pageNames：

```go
const (
	PageOverview Page = iota
	PageModels
	PageMarket
	PageCompute
	PageTeam
	PageTech
	PageAchievements
	numPages
)

var pageNames = [numPages]string{"總覽", "模型", "市場", "算力", "團隊", "科技", "成就"}
```

按鍵 `case "1", "2", "3", "4", "5", "6":` 改為 `case "1", "2", "3", "4", "5", "6", "7":`。

`renderPage` switch 加：

```go
	case PageAchievements:
		return renderAchievements(m)
```

`pageKeys` switch 加：

```go
	case PageAchievements:
		return "[↑↓]捲動"
```

`page_achievements.go`：

```go
// internal/tui/page_achievements.go
package tui

import (
	"fmt"
	"time"
)

func renderAchievements(m Model) string {
	cw := m.contentWidth()
	total := len(achievementCatalog)
	done := 0
	for _, a := range achievementCatalog {
		if _, ok := m.achievements[a.ID]; ok {
			done++
		}
	}
	frac := 0.0
	if total > 0 {
		frac = float64(done) / float64(total)
	}
	header := CardIn(CardGold, cw, "成就進度",
		fmt.Sprintf("%s %d/%d", GoldBar(frac, 24), done, total))

	rows := []string{header}
	for _, cat := range achievementCategories {
		var lines []string
		for _, a := range achievementCatalog[cat.From:cat.To] {
			if at, ok := m.achievements[a.ID]; ok {
				day := time.Unix(at, 0).Format("2006-01-02")
				lines = append(lines, styleGold.Render(fmt.Sprintf("🏆 %s — %s（%s）", a.Name, a.Desc, day)))
			} else {
				lines = append(lines, styleMuted.Render(fmt.Sprintf("🔒 %s — %s", a.Name, a.Desc)))
			}
		}
		rows = append(rows, CardIn(CardDefault, cw, cat.Title, VStack(lines...)))
	}
	return VStack(rows...)
}
```

- [ ] **Step 4: 跑全包測試、修過時斷言**

Run: `go test ./internal/tui/`
Expected: PASS（nav/tab 測試若斷言 6 頁輪替或 tab bar 內容，更新為 7 頁）

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): achievements badge-wall page

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---
### Task 5: ascii_hq.go 總部藝術與階段選擇

**Files:**
- Create: `internal/tui/ascii_hq.go`
- Test: `internal/tui/ascii_hq_test.go`

**Interfaces:**
- Consumes: `m.state.MilestonesReached`、`m.state.HasTraining`、`m.blink`。
- Produces:
  - `func hqStage(milestones int) int` — 夾住 [0,7]
  - `var hqStageNames [8]string` — 車庫→太空電梯
  - `func hqArt(stage int, lit bool) string` — 5 行 ASCII；`lit` 控制機房燈 `●`/`○`
  - `func renderHQ(m Model, w int) string` — 寬 ≥100 全圖卡；<100 摺疊單行圖示列（當前階段金色）

- [ ] **Step 1: 寫失敗測試**

```go
// internal/tui/ascii_hq_test.go
package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestHQStageClamps(t *testing.T) {
	if hqStage(-1) != 0 || hqStage(0) != 0 || hqStage(3) != 3 || hqStage(99) != 7 {
		t.Fatal("hqStage must clamp to [0,7]")
	}
}

func TestHQArtShapes(t *testing.T) {
	for s := 0; s < 8; s++ {
		art := hqArt(s, false)
		lines := strings.Split(art, "\n")
		if len(lines) != 5 {
			t.Fatalf("stage %d: %d lines, want 5", s, len(lines))
		}
		for _, ln := range lines {
			if lipgloss.Width(ln) > 30 {
				t.Fatalf("stage %d art too wide: %q", s, ln)
			}
		}
	}
	if !strings.Contains(hqArt(2, true), "●") {
		t.Fatal("lit art should contain ●")
	}
	if strings.Contains(hqArt(2, false), "●") {
		t.Fatal("unlit art should not contain ●")
	}
}

func TestRenderHQWideAndNarrow(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	m.state.MilestonesReached = 4
	wide := renderHQ(m, 110)
	if !strings.Contains(wide, "摩天大樓") {
		t.Fatalf("wide HQ missing stage name: %q", wide)
	}
	narrow := renderHQ(m, 80)
	if strings.Count(narrow, "\n") > 4 {
		t.Fatalf("narrow HQ should be compact, got %d lines", strings.Count(narrow, "\n")+1)
	}
	if !strings.Contains(narrow, "🏙") {
		t.Fatalf("narrow HQ missing stage icons: %q", narrow)
	}
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/tui/ -run TestHQ -v`
Expected: FAIL — `undefined: hqStage`

- [ ] **Step 3: 實作 ascii_hq.go**

```go
// internal/tui/ascii_hq.go
package tui

import (
	"fmt"
	"strings"
)

var hqStageNames = [8]string{
	"車庫", "小辦公室", "辦公樓", "科技園區", "摩天大樓", "百層巨塔", "企業之城", "太空電梯",
}

var hqStageIcons = [8]string{"🏠", "🏢", "🏬", "🏞", "🏙", "🗼", "🌃", "🚀"}

// hqArts 每階段 5 行、寬 ≤ 30；'◇' 是機房燈占位符。
var hqArts = [8]string{
	`      _______
     /_______\
     | .-. ◇ |
     | |_|   |
     '-------'`,
	`     _________
    |  _   _  |
    | |_| |_| |
    | |_| |_|◇|
    '---------'`,
	`     _________
    | [] [] [] |
    | [] [] [] |
    | [] [] ◇[]|
    '----_----'`,
	`   ____   ______
  | [] |_| [][] |
  | [] |_| [][] |
  | []◇|_| [][] |
  '----' '------'`,
	`       _/\_
      | [] |
      | [] |    __
      | []◇|___|  |
      '----'---'--'`,
	`        /\
       |[]|  /\
       |[]| |[]|
       |[]◇||[]|
      _|--|_|--|_`,
	`   /\   _/\_   /\
  |[]| _|[][]| |[]|
  |[]||[][]◇| _|[]|
  |[]||[][][]||[][]|
  '--''------''----'`,
	`        .  ✦  .
        |     🌙
       ||| 
       |||◇
    ___|||_______`,
}

// hqStage clamps MilestonesReached into the art range.
func hqStage(milestones int) int {
	if milestones < 0 {
		return 0
	}
	if milestones > 7 {
		return 7
	}
	return milestones
}

// hqArt renders a stage; lit swaps the datacenter light on (訓練中閃爍用).
func hqArt(stage int, lit bool) string {
	lamp := "○"
	if lit {
		lamp = "●"
	}
	return strings.ReplaceAll(hqArts[hqStage(stage)], "◇", lamp)
}

// renderHQ is the overview headquarters card; narrow terminals collapse to
// a single icon progression row.
func renderHQ(m Model, w int) string {
	stage := hqStage(m.state.MilestonesReached)
	if w < 100 {
		var icons []string
		for i, ic := range hqStageIcons {
			if i == stage {
				icons = append(icons, styleGold.Bold(true).Render(ic+hqStageNames[i]))
			} else {
				icons = append(icons, styleMuted.Render(ic))
			}
		}
		return CardIn(CardDefault, w, "總部", strings.Join(icons, styleMuted.Render("→")))
	}
	lit := m.state.HasTraining && m.blink
	art := styleCyan.Render(hqArt(stage, lit))
	status := ""
	if m.state.HasTraining {
		status = styleAmber.Render("  訓練機房運轉中…")
	}
	title := fmt.Sprintf("總部 — %s %s", hqStageIcons[stage], hqStageNames[stage])
	return CardIn(CardDefault, w, title, art+status)
}
```

- [ ] **Step 4: 跑測試確認通過**

Run: `go test ./internal/tui/ -run TestHQ -v`
Expected: PASS（若藝術行寬超標，縮短該行——測試是規格）

- [ ] **Step 5: Commit**

```bash
git add internal/tui/ascii_hq.go internal/tui/ascii_hq_test.go
git commit -m "feat(tui): ascii headquarters growth art

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: 總覽頁掛上總部卡

**Files:**
- Modify: `internal/tui/page_overview.go`（renderOverview rows 第一列）
- Test: `internal/tui/page_overview_test.go`（追加）

**Interfaces:**
- Consumes: Task 5 `renderHQ`。
- Produces: 總覽頁頂部（Grid 之前）一張全寬總部卡。

- [ ] **Step 1: 寫失敗測試**

```go
func TestOverviewShowsHQ(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 110, Height: 42})
	m = mm.(Model)
	if out := renderOverview(m); !strings.Contains(out, "總部") {
		t.Fatal("overview should show HQ card")
	}
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/tui/ -run TestOverviewShowsHQ -v`
Expected: FAIL

- [ ] **Step 3: 實作**

`renderOverview` 的 `rows := []string{` 第一個元素改為 HQ 卡：

```go
	rows := []string{
		renderHQ(m, cw),
		Grid(cw, 2,
			// ...既有 cells 不動...
		),
		// ...
	}
```

- [ ] **Step 4: 跑測試確認通過**

Run: `go test ./internal/tui/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): headquarters card on overview

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 7: 勝利結算儀式化（統計＋獎盃＋Legacy 卡片化）

**Files:**
- Modify: `internal/tui/feedback.go`（Moment 加 Title 欄位；positional literal 改 named）
- Modify: `internal/tui/epic.go`（renderEpicOverlay 用 Moment.Title）
- Modify: `internal/tui/dialog_campaign_end.go`（victory 分支加統計/獎盃、Legacy 選項卡片化）
- Test: `internal/tui/dialog_campaign_end_test.go`（追加）、`internal/tui/feedback_test.go`（斷言不變、literal 改 named）

**Interfaces:**
- Consumes: Phase 1-3 `CardIn/CardGold/CardAccent`、`legacyChoiceLabel`、`m.cfg.PatentK`、`m.state.PeakValuation/GameTime/Prestige.RouteBadges`。
- Produces:
  - `Moment` 變為 `struct { Level MomentLevel; Text string; Title string }`（Title 空字串 → overlay 預設「🏆 榮耀時刻」；**所有既有 positional literal 改 named-field**，含 feedback.go 的 reportMoment、tests）
  - `func legacyChoiceDesc(leg model.LegacyChoice) string`
  - `const trophyArt`（10 行純 ASCII）
  - victory 結算畫面：CardGold 外框＋獎盃＋本局統計（天數/峰值估值/結算專利/路線徽章）＋並排 Legacy 卡（游標 CardAccent 高亮）
  - 偏差說明：spec 列的「總用戶峰值」無既有資料源（PeakUsers 未追蹤），依「資料源全部既有」原則略去，不加 sim 欄位

- [ ] **Step 1: 寫失敗測試**

追加到 `internal/tui/dialog_campaign_end_test.go`（fixture 寫法沿用該檔既有測試）：

```go
func TestVictoryDialogShowsRecapAndCards(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	m.state.Campaign.Doctrine = model.DoctrineConsumer
	m.state.Campaign.Victory = model.DoctrineConsumer
	m.state.PeakValuation = 4e8 // patents = floor(sqrt(4e8/1e8)) = 2
	m.state.GameTime = 86400 * 30
	d, ok := newCampaignEndDialog(m, campaignEndVictory)
	if !ok {
		t.Fatal("victory dialog should open")
	}
	out := renderCampaignEndDialog(d, m)
	for _, want := range []string{"本局天數", "30", "峰值估值", "$400.00M", "結算專利", "+2", "宿敵完整情報"} {
		if !strings.Contains(out, want) {
			t.Fatalf("victory recap missing %q:\n%s", want, out)
		}
	}
}

func TestMomentTitleDefault(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	mo := Moment{Level: LevelEpic, Text: "x"}
	if out := renderEpicOverlay(mo, m); !strings.Contains(out, "榮耀時刻") {
		t.Fatal("empty Title should fall back to default")
	}
	mo2 := Moment{Level: LevelEpic, Text: "x", Title: "🔄 傳承開局"}
	if out := renderEpicOverlay(mo2, m); !strings.Contains(out, "傳承開局") {
		t.Fatal("custom Title should be used")
	}
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/tui/ -run 'TestVictoryDialog|TestMomentTitle' -v`
Expected: FAIL — recap 缺失 / Moment 無 Title 欄位

- [ ] **Step 3: 實作**

`feedback.go` Moment 改為：

```go
// Moment is a detected feedback-worthy game event. Title is only used by the
// Epic overlay; empty falls back to the default celebration title.
type Moment struct {
	Level MomentLevel
	Text  string
	Title string
}
```

同檔所有 `Moment{LevelXXX, "..."}` positional literal 改 named（例 `Moment{Level: LevelMajor, Text: "..."}`）；`feedback_test.go`、`tui.go`（pushBanner 呼叫端、Task 3 的成就 banner）同步改 named。

`epic.go` renderEpicOverlay 標題行改：

```go
	title := mo.Title
	if title == "" {
		title = "🏆 榮耀時刻"
	}
	card := CardIn(CardGold, 0, title, inner)
```

`dialog_campaign_end.go`：檔頭 import 加 `"math"`；加：

```go
const trophyArt = `   ___________
  '._==_==_=_.'
  .-\:      /-.
 | (|:.     |) |
  '-|:.     |-'
    \::.    /
     '::. .'
       ) (
     _.' '._
    '-------'`

func legacyChoiceDesc(leg model.LegacyChoice) string {
	switch leg.Kind {
	case model.LegacySecondary:
		return "帶著副戰略與其\n一階能力開新局"
	case model.LegacyIntel:
		return "下一局宿敵行動\n情報全開"
	case model.LegacyTech:
		return "帶一項已解鎖科技\n開局（再選一項）"
	default:
		return ""
	}
}
```

`renderCampaignEndDialog` victory 非 choosingTech 分支改為（choosingTech 與 exit 分支不動）：

```go
	day := int(m.state.GameTime / 86400)
	patents := int(math.Floor(math.Sqrt(m.state.PeakValuation / m.cfg.PatentK)))
	var badges []string
	for _, doc := range m.state.Prestige.RouteBadges {
		badges = append(badges, doctrineLabel(doc))
	}
	badges = append(badges, doctrineLabel(m.state.Campaign.Victory)+"（本局）")
	recap := VStack(
		styleGold.Render(trophyArt),
		"",
		KV("本局天數", fmt.Sprintf("%d", day)),
		KV("峰值估值", "$"+human(m.state.PeakValuation)),
		KV("結算專利", fmt.Sprintf("+%d", patents)),
		KV("路線徽章", strings.Join(badges, "、")),
		"",
		"選擇 Legacy 帶入下一局，或繼續本局無盡模式。",
	)
	var cards []string
	for i, leg := range d.options {
		kind := CardDefault
		if d.cursor == i {
			kind = CardAccent
		}
		cards = append(cards, CardIn(kind, 26,
			fmt.Sprintf("[%d] %s", i+1, legacyShortLabel(leg)), legacyChoiceDesc(leg)))
	}
	row := HRow(1, cards...)
	contMarker := "  "
	contLine := fmt.Sprintf("[%d] 繼續本局（無盡模式）", len(d.options)+1)
	if d.cursor == len(d.options) {
		contMarker = "▸ "
		contLine = styleAccent.Render(contLine)
	}
	body := VStack(recap, row, contMarker+contLine)
	if m.campaignError != "" {
		body = VStack(body, styleWarn.Render(m.campaignError))
	}
	body = VStack(body, "", helpStyle.Render("[↑↓]選擇 [Enter]確認 [Esc]取消"))
	return CardIn(CardGold, 0, "🏆 路線勝利結算", body)
```

加短標籤 helper（卡片標題塞不下完整 label）：

```go
func legacyShortLabel(leg model.LegacyChoice) string {
	switch leg.Kind {
	case model.LegacySecondary:
		return "副戰略"
	case model.LegacyIntel:
		return "宿敵完整情報"
	case model.LegacyTech:
		return "起始科技"
	default:
		return string(leg.Kind)
	}
}
```

（注意 `TestVictoryDialogShowsRecapAndCards` 斷言 `"宿敵完整情報"` 來自卡片標題。原 `legacyChoiceLabel` 保留給 Task 8 的傳承開局文案使用。）

- [ ] **Step 4: 跑全包測試、修 literal 編譯錯**

Run: `go test ./internal/tui/`
Expected: 先出現 positional literal 編譯錯誤——全部改 named 後 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): ceremonial victory settlement with trophy and legacy cards

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 8: 傳承開局 Epic 卡

**Files:**
- Modify: `internal/tui/tui.go:892-914`（updateCampaignEndDialog 成功分支）
- Modify: `internal/tui/dialog_campaign_end.go`（newRunEpic helper）
- Test: `internal/tui/dialog_campaign_end_test.go`（追加）

**Interfaces:**
- Consumes: Task 7 `Moment.Title`、`legacyChoiceLabel`、`m.state.Prestige`。
- Produces: `func newRunEpic(m Model) *Moment` — 傳承/退場後以 Epic overlay 顯示新局開場（帶入專利、徽章、Legacy）；`CampaignContinue`（無盡模式）不觸發。

- [ ] **Step 1: 寫失敗測試**

```go
func TestNewRunEpicContent(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	m.state.Prestige.Patents = 12
	m.state.Prestige.RouteBadges = []model.Doctrine{model.DoctrineConsumer}
	m.state.Prestige.PendingLegacy = model.LegacyChoice{Kind: model.LegacyIntel}
	mo := newRunEpic(m)
	if mo == nil || mo.Title != "🔄 傳承開局" {
		t.Fatalf("bad epic: %+v", mo)
	}
	for _, want := range []string{"專利 ×12", "消費者霸主", "宿敵完整情報"} {
		if !strings.Contains(mo.Text, want) {
			t.Fatalf("epic text missing %q: %q", want, mo.Text)
		}
	}
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/tui/ -run TestNewRunEpic -v`
Expected: FAIL — `undefined: newRunEpic`

- [ ] **Step 3: 實作**

`dialog_campaign_end.go`：

```go
// newRunEpic builds the fresh-run opening overlay after prestige/exit.
func newRunEpic(m Model) *Moment {
	p := m.state.Prestige
	lines := []string{fmt.Sprintf("帶入專利 ×%.0f", p.Patents)}
	if len(p.RouteBadges) > 0 {
		var badges []string
		for _, d := range p.RouteBadges {
			badges = append(badges, doctrineLabel(d))
		}
		lines = append(lines, "徽章："+strings.Join(badges, "、"))
	}
	if p.PendingLegacy.Kind != "" {
		lines = append(lines, "Legacy："+legacyChoiceLabel(p.PendingLegacy))
	}
	lines = append(lines, "", "新的輪迴開始——祝這次更快。")
	mo := Moment{Level: LevelEpic, Text: strings.Join(lines, "\n"), Title: "🔄 傳承開局"}
	return &mo
}
```

`updateCampaignEndDialog` confirm 成功分支（`m.snapDisplay()` 前後）改為：

```go
	if confirm {
		cmd := d.command()
		ns, err := sim.Apply(m.state, cmd, m.cfg)
		if err != nil {
			m.campaignError = campaignErrorText(err)
			m.campaignEnd = &d
			return m, nil
		}
		m.state = ns
		m.campaignError = ""
		m.campaignEnd = nil
		m.snapDisplay()
		switch cmd.(type) {
		case model.CampaignPrestige, model.CampaignExit:
			m.epic = newRunEpic(m)
		}
		return m, nil
	}
```

- [ ] **Step 4: 跑測試確認通過**

Run: `go test ./internal/tui/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): fresh-run opening epic after prestige

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 9: 全量迴歸與收尾

**Files:**
- Modify: `docs/superpowers/specs/2026-07-11-tui-warroom-gamification-design.md`（狀態行）

- [ ] **Step 1: 全量測試與 vet**

Run: `go test ./... && go vet ./...`
Expected: 全綠。

- [ ] **Step 2: 視覺抽查**

建臨時 `internal/tui/zz_capture_test.go`（結尾刪除、不 commit）render 總覽頁（含 HQ 卡）與成就頁（key 7），確認：HQ 藝術不破版、成就頁金/灰徽章與總進度條正常、七個 tab 顯示。

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
	fmt.Printf("===== OVERVIEW =====\n%s\n", m.View())
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'7'}})
	m = mm.(Model)
	fmt.Printf("===== ACHIEVEMENTS =====\n%s\n", m.View())
}
```

Run: `go test ./internal/tui -run TestZZCapture -v | head -120`，檢查後 `rm internal/tui/zz_capture_test.go`。

- [ ] **Step 3: 更新 spec 狀態並 commit**

spec 狀態行改為：`狀態：Phase 1-6 全部已實作`

```bash
git add docs/superpowers/specs/2026-07-11-tui-warroom-gamification-design.md
git commit -m "docs: mark spec fully implemented (phases 1-6)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```
