# TUI Dashboard Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild all six Tokensmith TUI pages into a Lipgloss multi-column dashboard with a shared shell (resource bar, tabs, pressures, footer), without changing sim economy or keybinding semantics.

**Architecture:** Extract a small pure layout kit (`card` / `hrow` / `footer` / styles). Refactor `Model.View` to the shell skeleton. Add pure view helpers in `internal/sim` (share bars, servable users) and display meta maps in `internal/tui` (tech Chinese names). Rewrite each `render*` page to compose cards. Dialogs keep logic; optional light frame polish last.

**Tech Stack:** Go 1.22+, Bubble Tea, Lipgloss. No new deps. Spec: `docs/superpowers/specs/2026-07-09-tui-dashboard-redesign-design.md`.

## Global Constraints

- **Do not change** tick formulas, balance numbers, or `Apply` command validation (except if a pure view helper needs export of existing unexported calc).
- **Keybindings unchanged:** `Tab`/`1–6`, `t`, `p`, `$`, `r/R`, `i/I`, `b/B`, `e`, `h`, `o`, `k`, `s`, `Enter` (tech), `P` prestige, `X` restart, `q`.
- **No event system.** Pressures + notice only.
- **Narrow terminals:** if content looks broken below ~80 cols, stack cards with `vstack` instead of `hrow` (simple width check via `lipgloss.Width` of a row or fixed threshold constant `minDashWidth = 80`).
- TDD where practical: helper unit tests first; page tests assert stable keywords (`待發佈`, `可撐用戶`, branch names).
- Chinese UI strings; rune-safe truncate.
- Commit after each task; keep `go test ./...` green.

---

## File Structure

| File | Role |
|---|---|
| `internal/tui/style.go` | Lipgloss styles (accent/warn/ok/muted/title/tab) |
| `internal/tui/layout.go` | `Card`, `HRow`, `VStack`, `KV`, `Bar`, `Footer`, `Truncate` |
| `internal/tui/layout_test.go` | Layout unit tests |
| `internal/tui/tui.go` | Shell `View`, resource bar colors, notice placement |
| `internal/tui/page_overview.go` | Dashboard 2×2 + pressures |
| `internal/tui/page_models.go` | List + detail cards |
| `internal/tui/page_market.go` | Segment cards + rivals |
| `internal/tui/page_compute.go` | Capacity causal card + process table |
| `internal/tui/page_team.go` | Roles + stars with effect blurb |
| `internal/tui/page_tech.go` | Grouped tech with Chinese meta |
| `internal/tui/tech_meta.go` | `techMeta` map (id → name, effect) |
| `internal/tui/dialog_*.go` | Light visual align only |
| `internal/sim/view.go` | `SegmentShareBars`, `ThreatLevel`, `ServableUsers` |
| `internal/sim/view_test.go` | Helper tests |
| Existing `*_test.go` | Update Contains expectations |

---

### Task 1: Layout kit + styles

**Files:**
- Create: `internal/tui/style.go`
- Create: `internal/tui/layout.go`
- Create: `internal/tui/layout_test.go`

**Interfaces (Produces):**
```go
func Card(title, body string) string
func HRow(gap int, parts ...string) string
func VStack(parts ...string) string
func KV(label, value string) string
func Bar(frac float64, width int) string // wraps progressBar
func Footer(pageKeys string) string      // pageKeys + "  [Tab]切頁 [q]離開"
func Truncate(s string, maxRunes int) string
// styles used by kit: styleAccent, styleWarn, styleMuted, styleTitle, styleTabActive
```

- [ ] **Step 1: Write failing tests**

```go
func TestCardContainsTitleAndBody(t *testing.T) {
	s := Card("公司", "估值 $1M")
	if !strings.Contains(s, "公司") || !strings.Contains(s, "估值") {
		t.Fatalf("card missing content: %q", s)
	}
}

func TestTruncateRunes(t *testing.T) {
	if Truncate("你好世界", 2) != "你好" {
		t.Fatalf("got %q", Truncate("你好世界", 2))
	}
	if Truncate("abc", 10) != "abc" {
		t.Fatal("no-op truncate failed")
	}
}

func TestFooterIncludesGlobalKeys(t *testing.T) {
	f := Footer("[t]訓練")
	if !strings.Contains(f, "[t]訓練") || !strings.Contains(f, "[Tab]") || !strings.Contains(f, "[q]") {
		t.Fatalf("footer: %q", f)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/tui/ -run 'TestCard|TestTruncate|TestFooter' -count=1
```

- [ ] **Step 3: Implement `style.go` + `layout.go`**

Move or re-export existing styles from `tui.go` into `style.go` so pages import one place. Keep package `tui`.

```go
// layout.go sketch
func Card(title, body string) string {
	inner := titleStyle.Render(title) + "\n" + body
	return boxStyle.Render(inner)
}

func HRow(gap int, parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	sep := strings.Repeat(" ", gap)
	return lipgloss.JoinHorizontal(lipgloss.Top, joinWithSep(parts, sep)...)
}

func Footer(pageKeys string) string {
	return helpStyle.Render(pageKeys + "  [Tab]切頁 [q]離開")
}

func Truncate(s string, maxRunes int) string {
	r := []rune(s)
	if maxRunes < 0 || len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes])
}
```

- [ ] **Step 4: Green + commit**

```bash
go test ./internal/tui/ -count=1
git add internal/tui/style.go internal/tui/layout.go internal/tui/layout_test.go internal/tui/tui.go
git commit -m "feat(tui): layout kit and shared styles for dashboard"
```

---

### Task 2: Shell View refactor

**Files:**
- Modify: `internal/tui/tui.go` (`View`, `renderResourceBar`, maybe `pressures` placement)

**Interfaces:**
- Consumes: layout kit
- Produces: shell matching spec §3.2; pages still old content temporarily

- [ ] **Step 1: Failing test for shell pieces**

```go
func TestViewShellHasTabsAndFooterPattern(t *testing.T) {
	m := testModel(t)
	v := m.View()
	for _, want := range []string{"Tokensmith", "總覽", "模型", "Day"} {
		if !strings.Contains(v, want) {
			t.Fatalf("missing %q in view", want)
		}
	}
}
```

(Adjust if Day formatting differs — use `Day` from resource bar.)

- [ ] **Step 2: Rewrite `View`**

```go
func (m Model) View() string {
	var rows []string
	day := int(m.state.GameTime / 86400)
	header := styleTitle.Render(fmt.Sprintf("Tokensmith  ·  Day %d", day))
	rows = append(rows, header)
	if m.offlineSummary != nil {
		rows = append(rows, offlineBanner(*m.offlineSummary))
	}
	if m.notice != "" {
		rows = append(rows, styleAccent.Render(m.notice))
	}
	rows = append(rows, renderResourceBar(m))
	rows = append(rows, renderTabBar(m.page))

	page := m.renderPage()
	if m.publish != nil {
		page = renderPublishDialog(*m.publish, m)
	} else if m.dialog != nil {
		page = renderTrainDialog(*m.dialog, m)
	}
	rows = append(rows, page)

	// Page-level footer is rendered by each page via Footer(...).
	// Shell does not duplicate page keys.

	return boxStyle.Render(VStack(rows...))
}
```

- [ ] **Step 3: Color resource bar**

In `renderResourceBar`, if inference util ≥ 0.9 wrap the 推理 segment with `styleWarn`; if cash < 0 wrap 現金.

- [ ] **Step 4: Remove duplicate outer separators** if pages no longer need full-width `──` under tabs (optional cleanup).

- [ ] **Step 5: Fix tests that snapshot full View; green; commit**

```bash
go test ./internal/tui/ -count=1
git add internal/tui/tui.go internal/tui/*_test.go
git commit -m "feat(tui): dashboard shell with header notice and tinted resource bar"
```

---

### Task 3: Sim view helpers

**Files:**
- Modify: `internal/sim/view.go`
- Modify: `internal/sim/view_test.go`

**Interfaces (Produces):**
```go
// ShareRow is one entry for market/overview bars.
type ShareRow struct {
	Name  string
	Share float64 // 0..1 of (player best + all competitors) appeal in segment
	You   bool
}

// SegmentShareBars returns player + competitors sorted by share desc.
func SegmentShareBars(ns model.GameState, b balance.Config, seg model.Segment) []ShareRow

// ThreatLevel: 0 low, 1 mid, 2 high — rival appeal vs player's best in seg.
func ThreatLevel(ns model.GameState, b balance.Config, seg model.Segment, rival model.Competitor) int

// ServableUsers is max users inference capacity can support; 0 capacity → 0
// (caller displays grace copy when capacity==0).
func ServableUsers(ns model.GameState, b balance.Config) float64
```

Implementation notes:

- Reuse `appealOf` + segment weights (same as `advanceUsers` / `MarketRank`).
- Share: `playerBestAppeal / (playerBest + sumRival)`; each rival `appeal_i / total`.
- Threat: if `rival > player*1.1` → high; `>= player*0.9` → mid; else low (tune in test).
- ServableUsers: `EffectiveInference(ns,b) / b.InferenceLoadPerUser` when loadPerUser > 0.

- [ ] **Step 1: Tests**

```go
func TestSegmentShareBarsSumsToOne(t *testing.T) {
	// one online model + default competitors
	// sum of Share ≈ 1
}

func TestServableUsers(t *testing.T) {
	// 10 N7 inference, loadPerUser 0.0001 → 100000
}

func TestThreatLevelOrdering(t *testing.T) { ... }
```

- [ ] **Step 2: Implement + green + commit**

```bash
go test ./internal/sim/ -count=1
git add internal/sim/view.go internal/sim/view_test.go
git commit -m "feat(sim): share bars, threat level, servable users view helpers"
```

---

### Task 4: Overview page

**Files:**
- Modify: `internal/tui/page_overview.go`
- Modify: `internal/tui/page_overview_test.go`

**Layout:**
```
HRow( companyCard, trainCard )
HRow( shareCard, powerMilestoneCard )
Card("注意", pressures) if any
Footer("[t]訓練 …")
```

- Train card copy:
  - HasTraining → Gen bar segment
  - else if drafts > 0 → `無進行中訓練 · 待發佈 N 個（模型頁 p）`
  - else → open train hint
- Share card: consumer `SegmentShareBars`, show top ~5 with `Bar(share, 10)` and ★ on You.
- Use `Card` / `HRow(2, ...)` / `VStack`.

- [ ] **Step 1: Update tests for new strings** (`待發佈`, `市佔`, `里程碑`)

- [ ] **Step 2: Implement renderOverview**

- [ ] **Step 3: Green + commit**

```bash
go test ./internal/tui/ -count=1
git commit -am "feat(tui): overview multi-column dashboard"
```

---

### Task 5: Models page

**Files:**
- Modify: `internal/tui/page_models.go`
- Modify: `internal/tui/page_models_test.go`

**Layout:**
- `HRow( listCard, detailCard )` when width ok; else `VStack`.
- List: existing draft/live sections with ▸.
- Detail for `m.state.Models[m.modelCursor]`:
  - Four dims with `Bar(q/cap, 8)` if cap>0 else raw
  - If online: users → `EstimateUserTarget`, monthly ≈ users*price, load contrib, company util
  - Price vs `EffectiveRefPrice`
  - Draft: warn line 待發佈

```go
func renderModelDetail(m Model, idx int) string {
	// guard idx
	md := m.state.Models[idx]
	// build body
}
```

- Footer: `[↑↓]選模型 [p]發佈 [t]訓練 [$]改價`

- [ ] **Step 1: Tests** — draft + live render; detail contains `用戶` or `待發佈`

- [ ] **Step 2: Implement + commit**

```bash
git commit -am "feat(tui): models list and detail dashboard"
```

---

### Task 6: Market page

**Files:**
- Modify: `internal/tui/page_market.go`, `page_market_test.go`

**Layout:**
- Three segment cards (vstack of cards or hrow if wide enough for 3 — usually vstack).
- Each: users, rank, bars from `SegmentShareBars` (truncate names).
- Rivals card: name, capability bar (cap/100 or vs 45), top skill dim, threat label 高/中/低 via `ThreatLevel` on consumer or per-segment best — **use consumer for summary threat** or show per-seg max threat; simplest: threat on **consumer** segment for one number.

```go
func threatLabel(level int) string {
	switch level {
	case 2:
		return "高"
	case 1:
		return "中"
	default:
		return "低"
	}
}
```

- Footer: `[Tab]切頁` only extras.

- [ ] **Step 1: Tests** contain `消費者`, `對手`, `威脅`

- [ ] **Step 2: Implement + commit**

```bash
git commit -am "feat(tui): market segment cards and rival threat"
```

---

### Task 7: Compute page

**Files:**
- Modify: `internal/tui/page_compute.go`, `page_compute_test.go`

**Layout:**
1. Causal `Card("池狀態", ...)`:
   - train/inference bars + effective compute
   - `可撐用戶 ~X` from `ServableUsers`
   - `現況用戶 Y` = `TotalUsers`
   - if capacity==0: `未配置推理 · grace（不因超載砍用戶）`
   - else if Y > X: warn `超載 · 建議加推理`
2. Existing process table (keep columns; optional Card wrap)
3. Datacenter power/space with bars
4. Footer with full key help

- [ ] **Step 1: Test** contains `可撐用戶`

- [ ] **Step 2: Implement + commit**

```bash
git commit -am "feat(tui): compute capacity causal card"
```

---

### Task 8: Team page

**Files:**
- Modify: `internal/tui/page_team.go`, `page_team_test.go`
- Optional small helper in same file: `starBlurb(st model.Star) string`

**Layout:**
- Card 四職能: T1/T2/T3 counts, engineer %, ops text, marketing %, optional salary/s sum using balance salary rates
- Card 明星: each line name, signed/cost, blurb from effects (RnDPerSec, QualityMult≠1, UserGrowthMult≠1, InfraMult≠1)

```go
func starBlurb(st model.Star) string {
	var parts []string
	e := st.Effects
	if e.RnDPerSec != 0 {
		parts = append(parts, fmt.Sprintf("R&D+%.0f/s", e.RnDPerSec))
	}
	// quality mults, growth, infra…
	return strings.Join(parts, " · ")
}
```

- Footer: `[h][e][o][k][s]`

- [ ] **Step 1: Test** contains `研究員`, `明星`, and a known star name from DefaultStars

- [ ] **Step 2: Implement + commit**

```bash
git commit -am "feat(tui): team roles and star effect cards"
```

---

### Task 9: Tech meta + tech page

**Files:**
- Create: `internal/tui/tech_meta.go`
- Create: `internal/tui/tech_meta_test.go` (every Default tech ID has meta)
- Modify: `internal/tui/page_tech.go`, `page_tech_test.go`

**tech_meta.go:**
```go
type techMeta struct{ Name, Effect string }

var techCatalog = map[string]techMeta{
	"algo-cap-1": {Name: "能力架構 I", Effect: "能力 +15%"},
	"algo-train-1": {Name: "訓練效率 I", Effect: "訓練 R&D/工作量優化"},
	// … every ID in balance.DefaultTechNodes()
	// gen unlocks: model-gen-2 → "解鎖 Gen2"
	// process-N5 → "解鎖製程 N5"
}

func techLabel(id string) techMeta {
	if m, ok := techCatalog[id]; ok {
		return m
	}
	return techMeta{Name: id, Effect: ""}
}
```

**page_tech.go:**
- Group nodes by `node.Branch` in catalog order
- Print branch header then lines: cursor, Chinese name, cost/✓/🔒, effect
- Footer: `[↑↓]選節點 [Enter]解鎖`

- [ ] **Step 1: Test all Default tech IDs present in catalog**

```go
func TestTechCatalogCoversDefaultNodes(t *testing.T) {
	for _, n := range balance.Default().TechNodes {
		if techLabel(n.ID).Name == n.ID && !strings.HasPrefix(n.ID, "x") {
			// allow fallback only if documented; prefer strict:
			if _, ok := techCatalog[n.ID]; !ok {
				t.Errorf("missing tech meta for %s", n.ID)
			}
		}
	}
}
```

- [ ] **Step 2: Render test contains `演算法` and a Chinese name not only raw id**

- [ ] **Step 3: Commit**

```bash
git commit -am "feat(tui): tech tree Chinese labels and grouped dashboard"
```

---

### Task 10: Dialog polish + full suite

**Files:**
- Modify: `dialog_train.go`, `dialog_publish.go` (use `Card` or same border style; keep logic)
- Fix any broken tests across package

- [ ] **Step 1:**
```bash
go test ./... -count=1
go vet ./...
go build ./...
```

- [ ] **Step 2:** Manual checklist (agent notes in commit body or PR):
  - 80-col and wide terminal
  - Overview shows share + draft CTA
  - Models detail shows target users
  - Compute shows 可撐用戶
  - Tech shows Chinese names
  - Keys: t, p, $, r, Enter tech still work

- [ ] **Step 3: Final commit if needed**

```bash
git commit -am "fix(tui): dashboard redesign integration fallout"
```

- [ ] **Step 4: Do not tag release unless user asks**

---

## Spec Coverage

| Spec item | Task |
|---|---|
| Layout kit | 1 |
| Shell / notice / tinted bar | 2 |
| Share / threat / servable helpers | 3 |
| Overview | 4 |
| Models list+detail | 5 |
| Market | 6 |
| Compute causal | 7 |
| Team | 8 |
| Tech Chinese meta | 9 |
| Dialog light polish | 10 |
| No events / no economy change | Constraints |
| Keybindings preserved | All page tasks + existing key tests |

## Self-Review

- No TBD steps; concrete APIs and test names.
- Helpers pure in sim; display maps in tui.
- Scope excludes event system and balance retune.
