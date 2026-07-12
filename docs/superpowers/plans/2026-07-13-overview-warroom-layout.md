# Overview Slim + War Room Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Slim the overview page to KPI + token harvest with equal-height cards, and move all campaign UI onto a new page 2「戰情室」.

**Architecture:** Extend the pure Lipgloss layout kit (`EqualHeight`, `HRowEqual`, `GridN`, body pad/truncate helpers). Renumber TUI pages so 戰情室 is index 1. New `page_warroom.go` reuses existing campaign renderers from `campaign_meta.go` / `page_overview.go`. Rewrite `renderOverview` to skeleton B (HQ|公司, thin harvest, 3 status cards) with no campaign cards.

**Tech Stack:** Go 1.22+, Bubble Tea, Lipgloss. No new deps. Spec: `docs/superpowers/specs/2026-07-13-overview-warroom-layout-design.md`.

## Global Constraints

- **Do not change** tick formulas, balance numbers, campaign sim, or `Apply` command validation.
- **Keybindings:** page digits become `1–8` (insert 戰情室 at `2`); campaign keys `e`/`c`/`P`/`E`/`d` semantics unchanged; `t` still works from overview.
- **Chinese UI strings;** rune/width-safe truncate via existing `Truncate` / `TruncateWidth`.
- **Narrow terminals:** `< 80` stack; no EqualHeight when stacked. `80–99` overview Row3 is **2+1**. `≥ 100` full 2 + full-width harvest + 3-col.
- TDD: layout tests first; update overview tests that currently assert campaign/frontier detail/In-Out harvest; add war-room tests.
- Commit after each task; keep `go test ./internal/tui/ ...` and `go test ./...` green before finishing.

---

## File Structure

| File | Role |
|---|---|
| `internal/tui/layout.go` | Add `EqualHeight`, `HRowEqual`, `GridN`, `padBodyLines`, optional thin helpers |
| `internal/tui/layout_test.go` | Tests for new layout APIs |
| `internal/tui/tui.go` | Page constants, `pageNames`, digit keys `1–8`, `renderPage`, `pageKeys` for war room |
| `internal/tui/page_warroom.go` | **Create** `renderWarRoom` |
| `internal/tui/page_overview.go` | Skeleton B; remove campaign cards; thin harvest; slim train/frontier; pending strip |
| `internal/tui/campaign_meta.go` | Keep renderers; optional `maxLines` param on events/report if cleaner than wrappers |
| `internal/tui/page_overview_test.go` | Rewrite campaign/frontier/daily expectations |
| `internal/tui/page_warroom_test.go` | **Create** war-room tests |
| `internal/tui/nav_test.go` | Tab → war room; digit targets after renumber |
| `internal/tui/page_achievements_test.go` | Key `8` opens achievements |
| `internal/tui/scroll_test.go` | Include `PageWarRoom` if page list is exhaustive |

---

### Task 1: Layout kit — EqualHeight, GridN, body pad

**Files:**
- Modify: `internal/tui/layout.go`
- Modify: `internal/tui/layout_test.go`

**Interfaces:**
- Consumes: existing `HRow`, `VStack`, `Grid`, `CardIn`, `lipgloss.Height`
- Produces:
```go
// padBodyLines ensures body has exactly n lines (pad with "" or truncate from end).
func padBodyLines(body string, n int) string

// EqualHeight pads each part with trailing "\n" so lipgloss.Height matches the tallest.
// Prefer padding *card bodies* via padBodyLines before CardIn when equal borders are required.
func EqualHeight(parts ...string) []string

// HRowEqual runs EqualHeight then HRow(gap, ...).
func HRowEqual(gap int, parts ...string) string

// GridN is Grid generalized to `cols` columns (cols >= 1).
// Below minDashWidth stacks full-width. Odd trailing cells span full width.
func GridN(cw, gap, cols int, cells ...func(w int) string) string
```

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/layout_test.go`:

```go
func TestPadBodyLines(t *testing.T) {
	got := padBodyLines("a\nb", 4)
	if lipgloss.Height(got) != 4 {
		t.Fatalf("height=%d want 4 (%q)", lipgloss.Height(got), got)
	}
	short := padBodyLines("a\nb\nc\nd\ne", 2)
	if lipgloss.Height(short) != 2 || !strings.Contains(short, "a") {
		t.Fatalf("truncate failed: %q", short)
	}
}

func TestEqualHeight(t *testing.T) {
	a := "1\n2\n3"
	b := "x"
	out := EqualHeight(a, b)
	if lipgloss.Height(out[0]) != lipgloss.Height(out[1]) {
		t.Fatalf("heights %d vs %d", lipgloss.Height(out[0]), lipgloss.Height(out[1]))
	}
	if lipgloss.Height(out[0]) != 3 {
		t.Fatalf("want height 3, got %d", lipgloss.Height(out[0]))
	}
}

func TestHRowEqual(t *testing.T) {
	left := CardIn(CardDefault, 40, "L", "a\nb\nc")
	right := CardIn(CardDefault, 40, "R", "x")
	// Pad bodies then re-card for border-equal demo in later tasks;
	// HRowEqual still equalizes finished block heights.
	row := HRowEqual(2, left, right)
	if lipgloss.Height(row) < lipgloss.Height(left) {
		t.Fatalf("row shorter than tallest card")
	}
}

func TestGridNThreeColumns(t *testing.T) {
	cell := func(label string) func(int) string {
		return func(w int) string { return CardIn(CardDefault, w, label, "x") }
	}
	got := GridN(102, 2, 3, cell("A"), cell("B"), cell("C"))
	// First visual row should be ~102 wide (3 cols + 2 gaps).
	first := strings.Split(got, "\n")[0]
	if lipgloss.Width(first) != 102 {
		t.Fatalf("row width=%d want 102", lipgloss.Width(first))
	}
}

func TestGridNStacksWhenNarrow(t *testing.T) {
	cell := func(w int) string { return CardIn(CardDefault, w, "T", "B") }
	got := GridN(60, 2, 3, cell, cell, cell)
	for _, ln := range strings.Split(got, "\n") {
		if lipgloss.Width(ln) > 60 {
			t.Fatalf("overflow %d > 60", lipgloss.Width(ln))
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run 'TestPadBodyLines|TestEqualHeight|TestHRowEqual|TestGridN' -count=1`

Expected: FAIL — undefined functions.

- [ ] **Step 3: Implement**

In `internal/tui/layout.go`:

```go
func padBodyLines(body string, n int) string {
	if n <= 0 {
		return ""
	}
	lines := strings.Split(body, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	for len(lines) < n {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func EqualHeight(parts ...string) []string {
	maxH := 0
	for _, p := range parts {
		if h := lipgloss.Height(p); h > maxH {
			maxH = h
		}
	}
	out := make([]string, len(parts))
	for i, p := range parts {
		for lipgloss.Height(p) < maxH {
			p += "\n"
		}
		out[i] = p
	}
	return out
}

func HRowEqual(gap int, parts ...string) string {
	return HRow(gap, EqualHeight(parts...)...)
}

// GridN lays cells in `cols` equal-width columns.
func GridN(cw, gap, cols int, cells ...func(w int) string) string {
	if len(cells) == 0 {
		return ""
	}
	if cols < 1 {
		cols = 1
	}
	if cw < minDashWidth || cols == 1 {
		parts := make([]string, len(cells))
		for i, c := range cells {
			parts[i] = c(cw)
		}
		return VStack(parts...)
	}
	// total gaps per row = cols-1
	colW := (cw - gap*(cols-1)) / cols
	if colW < 1 {
		colW = 1
	}
	var rows []string
	for i := 0; i < len(cells); i += cols {
		end := i + cols
		if end > len(cells) {
			end = len(cells)
		}
		chunk := cells[i:end]
		if len(chunk) < cols {
			// trailing incomplete row: if single cell, full width; else equal split among remaining
			if len(chunk) == 1 {
				rows = append(rows, chunk[0](cw))
				continue
			}
			subW := (cw - gap*(len(chunk)-1)) / len(chunk)
			parts := make([]string, len(chunk))
			for j, c := range chunk {
				parts[j] = c(subW)
			}
			rows = append(rows, HRowEqual(gap, parts...))
			continue
		}
		parts := make([]string, cols)
		for j, c := range chunk {
			parts[j] = c(colW)
		}
		rows = append(rows, HRowEqual(gap, parts...))
	}
	return VStack(rows...)
}
```

Refactor existing `Grid` to call `GridN(cw, gap, 2, cells...)` to avoid drift:

```go
func Grid(cw, gap int, cells ...func(w int) string) string {
	return GridN(cw, gap, 2, cells...)
}
```

**Note:** `Grid` previously did **not** EqualHeight. After this refactor, 2-col rows become equal-height. That is intended and matches the spec; re-run existing `TestGridEqualWidths` / page tests.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/ -run 'TestPadBodyLines|TestEqualHeight|TestHRowEqual|TestGridN|TestGrid' -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/layout.go internal/tui/layout_test.go
git commit -m "feat(tui): add EqualHeight, GridN, and body pad helpers"
```

---

### Task 2: Page constants, navigation 1–8, pageKeys

**Files:**
- Modify: `internal/tui/tui.go` (page enum ~62–72, key switch ~700, `pageKeys` ~1237, `renderPage` ~1493)
- Modify: `internal/tui/nav_test.go`
- Modify: `internal/tui/page_achievements_test.go`
- Modify: `internal/tui/scroll_test.go` (if it lists all pages)

**Interfaces:**
- Produces:
```go
const (
	PageOverview Page = iota
	PageWarRoom
	PageModels
	PageMarket
	PageCompute
	PageTeam
	PageTech
	PageAchievements
	numPages // 8
)
var pageNames = [numPages]string{"總覽", "戰情室", "模型", "市場", "算力", "團隊", "科技", "成就"}
```

- [ ] **Step 1: Write / update failing tests**

`nav_test.go`:

```go
func TestTabCyclesPages(t *testing.T) {
	m := testModel(t)
	if m.page != PageOverview {
		t.Fatalf("start page = %v, want overview", m.page)
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if nm.(Model).page != PageWarRoom {
		t.Fatalf("after Tab = %v, want war room", nm.(Model).page)
	}
}

func TestNumberKeyJumpsPage(t *testing.T) {
	m := testModel(t)
	// After renumber: 1總覽 2戰情 3模型 4市場 5算力 6團隊 7科技 8成就
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'6'}})
	if nm.(Model).page != PageTeam {
		t.Fatalf("key 6 = %v, want team", nm.(Model).page)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if nm.(Model).page != PageWarRoom {
		t.Fatalf("key 2 = %v, want war room", nm.(Model).page)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'8'}})
	if nm.(Model).page != PageAchievements {
		t.Fatalf("key 8 = %v, want achievements", nm.(Model).page)
	}
}

func TestNumPagesIsEight(t *testing.T) {
	if numPages != 8 {
		t.Fatalf("numPages=%d want 8", numPages)
	}
	if pageNames[1] != "戰情室" {
		t.Fatalf("pageNames[1]=%q", pageNames[1])
	}
}
```

`page_achievements_test.go`: change key `7` → `8` and assertion message.

- [ ] **Step 2: Run tests — expect fail**

Run: `go test ./internal/tui/ -run 'TestTabCyclesPages|TestNumberKeyJumpsPage|TestNumPagesIsEight|TestAchievements' -count=1`

Expected: FAIL on Tab target / key mapping / missing `PageWarRoom`.

- [ ] **Step 3: Implement navigation**

In `tui.go` constants:

```go
const (
	PageOverview Page = iota
	PageWarRoom
	PageModels
	PageMarket
	PageCompute
	PageTeam
	PageTech
	PageAchievements
	numPages
)

var pageNames = [numPages]string{"總覽", "戰情室", "模型", "市場", "算力", "團隊", "科技", "成就"}
```

Digit keys:

```go
case "1", "2", "3", "4", "5", "6", "7", "8":
	m.page = Page(msg.String()[0] - '1')
	m.vp.GotoTop()
	return m, nil
```

`pageKeys` — add war-room case **before** default overview:

```go
case PageWarRoom:
	hint := "[1]總覽"
	if len(m.state.Events.Pending) > 0 {
		hint = "[e]決策 " + hint
	}
	if m.state.Campaign.PerkTierPending > 0 {
		hint += " [c]能力"
	}
	if m.state.Campaign.Victory != model.DoctrineNone {
		hint += " [P]結算"
	}
	return hint
```

Keep overview `pageKeys` as today (including `[e]` when pending). Spec: overview still allows `e`; war room is the visual home.

`renderPage` — temporary stub until Task 3 (must compile):

```go
case PageWarRoom:
	return "戰情室" // replaced in Task 3
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/ -run 'TestTabCyclesPages|TestNumberKeyJumpsPage|TestNumPagesIsEight|TestAchievements|TestViewHasChrome' -count=1`

Expected: PASS (chrome should list 戰情室 once tabs render from `pageNames`).

Also run full package once to catch hard-coded page assumptions:

Run: `go test ./internal/tui/ -count=1`

Fix any digit-key or page-index failures in place (e.g. scroll tests listing pages — append `PageWarRoom`).

- [ ] **Step 5: Commit**

```bash
git add internal/tui/tui.go internal/tui/nav_test.go internal/tui/page_achievements_test.go internal/tui/scroll_test.go
git commit -m "feat(tui): insert 戰情室 as page 2 and renumber 1-8"
```

---

### Task 3: `page_warroom.go` — mount campaign cards

**Files:**
- Create: `internal/tui/page_warroom.go`
- Create: `internal/tui/page_warroom_test.go`
- Modify: `internal/tui/tui.go` (`renderPage` case)
- Modify: `internal/tui/page_overview.go` — move `renderEventsCard` if it lives only there (keep function, change caller); optionally bump line limits via parameters

**Interfaces:**
- Consumes: `renderCampaignStatusCard`, `renderRivalRoadmapCard`, `renderBoardReportCard`, `renderEventsCard`, `HRowEqual`, `VStack`, `CardIn`
- Produces: `func renderWarRoom(m Model) string`

- [ ] **Step 1: Write failing tests**

`page_warroom_test.go`:

```go
package tui

import (
	"strings"
	"testing"

	"tokensmith/internal/model"
)

func TestWarRoomShowsCampaignCards(t *testing.T) {
	m := testModel(t)
	m.state.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand, Cycle: 4,
		Primary:  model.RivalRoadmap{Company: "OpenAI", ActionIndex: 0, CyclesUntilAction: 1},
		Wildcard: model.RivalRoadmap{Company: "DeepSeek", ActionIndex: 0, CyclesUntilAction: 2},
		Reports: []model.BoardReport{{Cycle: 4, Entries: []model.CampaignReportEntry{
			{Kind: model.ReportRivalAction, SubjectID: "OpenAI", DetailID: "openai-flagship"},
		}}},
	}
	v := renderWarRoom(m)
	for _, want := range []string{"主要戰略", "消費者霸主", "OpenAI", "下一步", "董事會報告", "產業動態"} {
		if !strings.Contains(v, want) {
			t.Fatalf("missing %q:\n%s", want, v)
		}
	}
}

func TestWarRoomPreCampaignGuidance(t *testing.T) {
	m := testModel(t)
	m.state.Campaign = model.CampaignState{}
	v := renderWarRoom(m)
	if !strings.Contains(v, "第一個模型上線後可選公司戰略") {
		t.Fatalf("pre-campaign guidance missing:\n%s", v)
	}
}

func TestWarRoomPendingEventHighlighted(t *testing.T) {
	m := pendingChipShortage(testModel(t)) // helper in dialog_event_test.go (same package)
	v := renderWarRoom(m)
	if !strings.Contains(v, "決策") {
		t.Fatalf("expected pending decision highlight:\n%s", v)
	}
}
```

- [ ] **Step 2: Run — expect fail**

Run: `go test ./internal/tui/ -run 'TestWarRoom' -count=1`

Expected: FAIL — `renderWarRoom` undefined.

- [ ] **Step 3: Implement `renderWarRoom`**

`page_warroom.go`:

```go
package tui

func renderWarRoom(m Model) string {
	cw := m.contentWidth()
	gap := 2

	// Events + board: allow up to 6 body lines (spec §5.3).
	// Prefer adding maxLines params to renderEventsCard / renderBoardReportCard;
	// if keeping zero-param funcs, temporarily call as-is then raise limits inside those funcs
	// only when used from war room via:
	//   renderEventsCardMax(m, cw, 6)
	//   renderBoardReportCardMax(m, cw, 6)

	var top string
	if cw < minDashWidth {
		top = VStack(
			renderCampaignStatusCard(m, cw),
			renderRivalRoadmapCard(m, cw),
		)
	} else {
		colW := (cw - gap) / 2
		top = HRowEqual(gap,
			renderCampaignStatusCard(m, colW),
			renderRivalRoadmapCard(m, colW),
		)
	}

	return VStack(
		top,
		renderEventsCardMax(m, cw, 6),
		renderBoardReportCardMax(m, cw, 6),
	)
}
```

Refactor in `page_overview.go` / `campaign_meta.go`:

```go
// renderEventsCard keeps old name as max=4 for any residual callers, or delete after overview stops using it.
func renderEventsCard(m Model, w int) string { return renderEventsCardMax(m, w, 4) }

func renderEventsCardMax(m Model, w int, maxLines int) string {
	// same as current renderEventsCard but loop condition len(lines) < maxLines
}

func renderBoardReportCard(m Model, w int) string { return renderBoardReportCardMax(m, w, 4) }

func renderBoardReportCardMax(m Model, w int, maxEntries int) string {
	// take last maxEntries instead of hard-coded 4
}
```

Wire `renderPage`:

```go
case PageWarRoom:
	return renderWarRoom(m)
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/ -run 'TestWarRoom' -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/page_warroom.go internal/tui/page_warroom_test.go internal/tui/tui.go internal/tui/page_overview.go internal/tui/campaign_meta.go
git commit -m "feat(tui): add 戰情室 page with campaign cards"
```

---

### Task 4: Overview skeleton B — remove campaign, thin harvest, slim train

**Files:**
- Modify: `internal/tui/page_overview.go`
- Modify: `internal/tui/page_overview_test.go`

**Interfaces:**
- Consumes: `renderHQ`, `companyCard`, `trainCard`, `shareCard`, `powerMilestoneCard`, `renderDailyUsageCard`, `GridN`, `HRowEqual`, `padBodyLines`, `pressures`
- Produces: new `renderOverview` structure; `overviewPendingStrip(m) string`

- [ ] **Step 1: Rewrite tests first (they should fail on current overview)**

Replace / update in `page_overview_test.go`:

1. **Delete or relocate** `TestOverviewShowsCampaignWarRoom` and `TestOverviewPreCampaignGuidance` (already on war room in Task 3).

2. **Add:**

```go
func TestOverviewHasNoCampaignCards(t *testing.T) {
	m := testModel(t)
	m.state.Campaign = model.CampaignState{
		Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand, Cycle: 4,
		Primary: model.RivalRoadmap{Company: "OpenAI", ActionIndex: 0, CyclesUntilAction: 1},
		Reports: []model.BoardReport{{Cycle: 4, Entries: []model.CampaignReportEntry{
			{Kind: model.ReportRivalAction, SubjectID: "OpenAI", DetailID: "openai-flagship"},
		}}},
	}
	v := renderOverview(m)
	for _, ban := range []string{"公司戰略", "宿敵路線", "董事會報告", "產業動態", "主要戰略", "OpenAI"} {
		if strings.Contains(v, ban) {
			t.Fatalf("overview must not show campaign content %q:\n%s", ban, v)
		}
	}
}

func TestOverviewPendingStrip(t *testing.T) {
	m := pendingChipShortage(testModel(t))
	v := renderOverview(m)
	if !strings.Contains(v, "[2]戰情室") || !strings.Contains(v, "待決策") {
		t.Fatalf("expected pending strip pointing to war room:\n%s", v)
	}
	if strings.Contains(v, "產業動態") {
		t.Fatalf("overview must not show 產業動態 card title:\n%s", v)
	}
}
```

Strip copy (spec): `⚠ 產業待決策 N · [2]戰情室  [e]決策`.

3. **Update `TestOverviewShowsFrontier`:**

```go
// Overview only: progress + ETA/stall (≤2 frontier lines). No 分配/有效/折合/建議/R&D 進度.
for _, want := range []string{"訓練 Gen5", "前沿 Gen6", "ETA"} {
	// ...
}
for _, ban := range []string{"分配 前沿", "有效", "折合", "建議", "R&D 進度"} {
	if strings.Contains(v, ban) {
		t.Errorf("overview frontier too detailed: has %q", ban)
	}
}
```

Stall test still expects `停滯` / `R&D 不足` in card.

4. **Update `TestOverviewShowsDailyUsageBySource`:**

Wide layout is **thin**: source totals on one/two lines, **no** per-source `In 120K` / `Out 18K` requirement.

```go
for _, want := range []string{"今日 Token 收成", "Claude", "Codex", "Grok", "OpenCode", "316K"} {
	// labels may be narrowLabel or shortened wideLabel — accept "Claude" substring
}
// Must NOT require full multi-line In/Out rows.
// Still: Grok must not fabricate Out.
```

5. **Keep** `TestOverviewShowsHQ`, KPIs, share, drafts, align-flush, zeros.

- [ ] **Step 2: Run overview tests — expect fail**

Run: `go test ./internal/tui/ -run 'TestOverview' -count=1`

Expected: FAIL on no-campaign / frontier bans / pending strip / thin harvest.

- [ ] **Step 3: Implement overview**

`renderOverview`:

```go
func renderOverview(m Model) string {
	cw := m.contentWidth()
	gap := 2
	rows := []string{}

	// Row1: HQ | 公司
	if cw < minDashWidth {
		rows = append(rows, renderHQ(m, cw), companyCard(m, cw))
	} else {
		colW := (cw - gap) / 2
		// Equalize bodies before CardIn so borders match height.
		// renderHQ already returns a full card — for true border-equal, either:
		// (a) HRowEqual on finished cards, or
		// (b) split HQ art/body helpers.
		// Spec accepts EqualHeight on finished blocks; use HRowEqual:
		rows = append(rows, HRowEqual(gap, renderHQ(m, colW), companyCard(m, colW)))
	}

	// Row2: thin daily harvest full width
	rows = append(rows, renderDailyUsageCard(m, cw))

	// Row3: 訓練 | 市佔 | 里程碑
	rows = append(rows, renderOverviewStatusRow(m, cw, gap))

	// Pending strip (not a campaign card)
	if strip := overviewPendingStrip(m); strip != "" {
		rows = append(rows, strip)
	}

	// Operational pressures (non-campaign)
	if warns := pressures(m); len(warns) > 0 {
		rows = append(rows, CardIn(CardThreat, cw, "注意", VStack(warns...)))
	}
	return VStack(rows...)
}

func renderOverviewStatusRow(m Model, cw, gap int) string {
	if cw < minDashWidth {
		return VStack(trainCard(m, cw), shareCard(m, cw), powerMilestoneCard(m, cw))
	}
	if cw < 100 {
		// 2+1
		colW := (cw - gap) / 2
		top := HRowEqual(gap, trainCard(m, colW), shareCard(m, colW))
		return VStack(top, powerMilestoneCard(m, cw))
	}
	return GridN(cw, gap, 3,
		func(w int) string { return trainCard(m, w) },
		func(w int) string { return shareCard(m, w) },
		func(w int) string { return powerMilestoneCard(m, w) },
	)
}

func overviewPendingStrip(m Model) string {
	n := len(m.state.Events.Pending)
	if n == 0 {
		return ""
	}
	return styleWarn.Render(fmt.Sprintf("⚠ 產業待決策 %d · [2]戰情室  [e]決策", n))
}
```

**Thin `renderDailyUsageCard` (wide path):**

Replace per-source multi-line block with one compact line (or two if wrap):

```go
// Wide thin: "Claude 138K · Codex 97K · Grok 30K · OpenCode 51K" then "合計 316K"
// Reuse wrapCompactSegments; always show all four sources + 合計.
// Do not print In/Out on overview (spec §4.2).
// Grok label may keep "Grok" or "Grok*" — still no fabricated Out.
```

Keep `w < 100` compact path similar; both paths should look thin (≤3 body lines).

**Slim `renderFrontierProgressLines` for overview:**

```go
func renderFrontierProgressLines(m Model) []string {
	v := sim.FrontierProgressView(m.state, m.cfg)
	if !v.Active {
		return []string{styleMuted.Render("前沿 無進行中（科技頁啟動）")}
	}
	lines := []string{
		fmt.Sprintf("前沿 Gen%d %s %.0f%%", v.TargetGen, Bar(v.WorkFraction, 10), v.WorkFraction*100),
	}
	if v.UnavailableReason != "" {
		lines = append(lines, styleWarn.Render("停滯 · "+frontierStallCopy(v.UnavailableReason)))
	} else if v.ETASec > 0 {
		lines = append(lines, KV("ETA", formatETASec(v.ETASec)))
	}
	return lines // max 2 lines when active
}
```

**Share card:** change `limit := 5` to `limit := 4`.

**Train card body:** after building lines, optionally `padBodyLines(VStack(...), 5)` before `CardIn` — or rely on `HRowEqual`/`GridN` equalize.

Remove from overview any call to campaign renderers.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/ -run 'TestOverview' -count=1`

Expected: PASS

Run full: `go test ./internal/tui/ -count=1`

- [ ] **Step 5: Commit**

```bash
git add internal/tui/page_overview.go internal/tui/page_overview_test.go
git commit -m "feat(tui): slim overview to KPI harvest layout without campaign cards"
```

---

### Task 5: Footer polish, chrome, full regression

**Files:**
- Modify: `internal/tui/tui.go` (`pageKeys` overview — ensure pending still shows `[e]`; war room keys complete)
- Modify: any help string tests
- Optionally: `DEPLOYMENT.md` only if it documents page list as six/seven pages (update to eight + 戰情室)

- [ ] **Step 1: Manual checklist via tests**

Add `TestTabBarListsWarRoom` if not covered:

```go
func TestTabBarListsWarRoom(t *testing.T) {
	m := testModel(t)
	v := m.View()
	if !strings.Contains(v, "戰情室") {
		t.Fatalf("tab bar missing 戰情室:\n%s", v)
	}
}
```

Verify overview footer keys still include campaign action hints where relevant (`[c]`, `[e]` when pending) per existing `pressure_test.go` / `pageKeys` behavior.

- [ ] **Step 2: Run full suite**

```bash
go test ./internal/tui/ -count=1
go test ./... -count=1
```

Expected: all PASS.

- [ ] **Step 3: Spec acceptance smoke (manual or render asserts)**

| # | Check |
|---|---|
| 1 | Overview has no 公司戰略/宿敵/董事會/產業動態 cards |
| 2 | Width 110: HQ\|公司, harvest, 3 status cards |
| 3 | Same-row cards equal height (visual or lipgloss.Height on HRowEqual children) |
| 4 | Key `2` → war room with four campaign sections |
| 5 | Pending → overview strip with `[2]`/`[e]`, no 產業動態 card |
| 6 | `e`/`c`/`P` still open dialogs from war room / overview as before |

- [ ] **Step 4: Commit**

```bash
git add internal/tui/ docs/superpowers/plans/2026-07-13-overview-warroom-layout.md
# include DEPLOYMENT.md only if edited
git commit -m "test(tui): war-room chrome coverage and full regression green"
```

---

## Self-Review (plan vs spec)

| Spec requirement | Task |
|---|---|
| Layout EqualHeight / GridN / pad | Task 1 |
| 8 pages, 戰情室 = 2 | Task 2 |
| War room mounts 4 campaign cards, events/report max 6 | Task 3 |
| Overview skeleton B, no campaign cards, thin harvest, slim frontier, pending strip, pressures kept | Task 4 |
| Footer/keys, acceptance, full test | Task 5 |
| No sim/economy changes | All tasks (TUI only) |
| Narrow / 80–99 2+1 / ≥100 3-col | Task 4 `renderOverviewStatusRow` |

**Placeholder scan:** none intentional; implementers copy pending-event fixtures from `dialog_event_test.go` rather than inventing EventIDs.

**Type consistency:** `PageWarRoom` index 1; `renderWarRoom(m Model) string`; `renderEventsCardMax(m, w, maxLines int) string`; `GridN(cw, gap, cols int, cells ...func(int) string) string`.
