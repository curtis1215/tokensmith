# Dashboard Growth Charts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a TUI「儀表板」page (nav key `2`) with user, revenue, and R&D stock growth charts, hybrid short-session rings + calendar-day history sidecar, and R&D inflow breakdown by source.

**Architecture:** New `internal/metrics` package owns the versioned `metrics-history.json` sidecar (daily stock snapshots + per-source R&D inflow). TUI keeps session-only `dash*` spark rings for short windows and renders multi-row Unicode line charts. Token/staff inflow is bookkeeping only (same formulas as the resource bar); sim economy is untouched.

**Tech Stack:** Go 1.25, `charmbracelet/bubbletea` / lipgloss, existing `internal/tui/spark.go` rune scale, `go test` only. Zero new dependencies.

**Spec:** `docs/superpowers/specs/2026-07-14-dashboard-growth-charts-design.md`

## Global Constraints

- Module path is `tokensmith` (see `go.mod`); imports are `tokensmith/internal/...`.
- Zero new third-party deps; Unicode charts only.
- Do **not** change sim economic formulas, `balance` constants (except if a pure helper is already there), or `dailyusage` schema.
- Do **not** put KPI history into `GameState` or `daily-usage.json`.
- Day keys: always `dailyusage.DayKey(time.Now())` / `dailyusage.DayKey(at)` — local `YYYY-MM-DD`.
- Metrics file: same directory as save, name `metrics-history.json`.
- Retain at most **90** calendar days with data (`metrics.MaxDays = 90`).
- R&D stock series uses `Resources.RnD`; inflow is positive credits only (token sources + `staff`).
- Token R&D attribution must match `lastTokenRnD`:  
  `TokenRawRnD × StreakMult × PrestigeRnDMult × OfficeTokenRnDMult`.
- Staff inflow: `sim.RnDRatePerSec(state, cfg) * dt` on economy ticks.
- Nav after change: `1`總覽 `2`儀表板 `3`戰情室 `4`模型 `5`市場 `6`算力 `7`團隊 `8`科技 `9`成就.
- Run `go test` for packages touched at end of each task; prefer `go test ./internal/metrics/... ./internal/tui/...` after TUI tasks.
- Commit messages: `type(scope): summary` (repo convention).
- TDD: failing test → implement → pass → commit.

## File map

| File | Responsibility |
|------|----------------|
| `internal/metrics/types.go` | Document, DayPoint, constants, SourceOrder, DayKey wrapper |
| `internal/metrics/apply.go` | UpsertSnapshot, AddInflow, EnsureDay, Prune, open-of-day tracking helpers |
| `internal/metrics/store.go` | Load/Save atomic JSON, corrupt recovery |
| `internal/metrics/*_test.go` | Unit tests for apply/store |
| `internal/tui/linechart.go` | Multi-row single/multi series charts |
| `internal/tui/linechart_test.go` | Chart edge cases |
| `internal/tui/page_dashboard.go` | Three cards: users, revenue, R&D |
| `internal/tui/page_dashboard_test.go` | Render smoke, empty state |
| `internal/tui/tui.go` | PageDashboard, paths, Model fields, flush hooks, View switch |
| `internal/tui/display.go` | Dash ring sampling |
| `internal/tui/nav_test.go` | Page count and key bindings |
| Existing token tick path in `tui.go` | Add inflow bookkeeping when `lastTokenRnD` is computed |

---

### Task 1: `internal/metrics` types + pure apply/prune

**Files:**
- Create: `internal/metrics/types.go`
- Create: `internal/metrics/apply.go`
- Create: `internal/metrics/apply_test.go`

**Interfaces:**
- Consumes: `dailyusage.DayKey` (import `tokensmith/internal/dailyusage`)
- Produces:
  - `const SchemaVersion = 1`
  - `const MaxDays = 90`
  - `const SourceStaff = "staff"`
  - `var SourceOrder = []string{"claude-code","codex","grok","opencode","staff"}`
  - `type DayPoint struct { Users, MonthlyRevenue, RnDStock float64; RnDInflow map[string]float64; OpenUsers, OpenRevenue, OpenRnD float64; OpenSet bool }`
  - `type Document struct { SchemaVersion int; UpdatedAt int64; Days map[string]DayPoint }`
  - `func EmptyDocument() Document`
  - `func EnsureDay(doc *Document, day string) *DayPoint` — returns pointer into map after ensure
  - `func UpsertSnapshot(doc *Document, day string, users, revenue, rndStock float64, nowUnix int64)` — overwrites stock fields; on first write of day sets Open* if `!OpenSet`
  - `func AddInflow(doc *Document, day, source string, amount float64, nowUnix int64)` — no-op if amount≤0 or source=="" or invalid day
  - `func Prune(doc *Document)` — keep newest MaxDays valid keys
  - `func ValidDayKey(day string) bool`
  - `func Series(doc Document, days []string, pick func(DayPoint) float64) []float64`
  - `func SortedDays(doc Document) []string` — ascending date keys

- [ ] **Step 1: Write the failing tests**

Create `internal/metrics/apply_test.go`:

```go
package metrics

import (
	"math"
	"testing"
)

func TestUpsertSnapshotSetsOpenOnce(t *testing.T) {
	doc := EmptyDocument()
	UpsertSnapshot(&doc, "2026-07-14", 100, 50, 10, 1)
	UpsertSnapshot(&doc, "2026-07-14", 200, 80, 5, 2)
	p := doc.Days["2026-07-14"]
	if !p.OpenSet || p.OpenUsers != 100 || p.OpenRevenue != 50 || p.OpenRnD != 10 {
		t.Fatalf("open frozen wrong: %+v", p)
	}
	if p.Users != 200 || p.MonthlyRevenue != 80 || p.RnDStock != 5 {
		t.Fatalf("latest stock wrong: %+v", p)
	}
}

func TestAddInflowAccumulates(t *testing.T) {
	doc := EmptyDocument()
	AddInflow(&doc, "2026-07-14", "claude-code", 10, 1)
	AddInflow(&doc, "2026-07-14", "claude-code", 5, 2)
	AddInflow(&doc, "2026-07-14", "staff", 3, 3)
	AddInflow(&doc, "2026-07-14", "claude-code", -1, 4) // ignored
	p := doc.Days["2026-07-14"]
	if math.Abs(p.RnDInflow["claude-code"]-15) > 1e-9 {
		t.Fatalf("claude=%v want 15", p.RnDInflow["claude-code"])
	}
	if math.Abs(p.RnDInflow["staff"]-3) > 1e-9 {
		t.Fatalf("staff=%v want 3", p.RnDInflow["staff"])
	}
}

func TestPruneKeepsNewest90(t *testing.T) {
	doc := EmptyDocument()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 95; i++ {
		day := start.AddDate(0, 0, i).Format("2006-01-02")
		UpsertSnapshot(&doc, day, float64(i), 0, 0, int64(i))
	}
	Prune(&doc)
	if len(doc.Days) != MaxDays {
		t.Fatalf("len=%d want %d", len(doc.Days), MaxDays)
	}
	if _, ok := doc.Days["2026-01-01"]; ok {
		t.Fatal("oldest day should be pruned")
	}
	if _, ok := doc.Days[start.AddDate(0, 0, 94).Format("2006-01-02")]; !ok {
		t.Fatal("newest day should remain")
	}
}

func TestSortedDaysAscending(t *testing.T) {
	doc := EmptyDocument()
	UpsertSnapshot(&doc, "2026-07-12", 1, 0, 0, 1)
	UpsertSnapshot(&doc, "2026-07-10", 1, 0, 0, 1)
	UpsertSnapshot(&doc, "2026-07-11", 1, 0, 0, 1)
	got := SortedDays(doc)
	want := []string{"2026-07-10", "2026-07-11", "2026-07-12"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("SortedDays=%v want %v", got, want)
	}
}

func TestInvalidDayRejected(t *testing.T) {
	doc := EmptyDocument()
	UpsertSnapshot(&doc, "not-a-day", 1, 2, 3, 1)
	AddInflow(&doc, "2026/07/14", "staff", 9, 1)
	if len(doc.Days) != 0 {
		t.Fatalf("invalid days should be ignored: %+v", doc.Days)
	}
}
```

Add `import ("math"; "testing"; "time")` as needed.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/metrics/ -count=1`

Expected: FAIL (package not found or undefined symbols)

- [ ] **Step 3: Implement types + apply**

`internal/metrics/types.go`:

```go
package metrics

import (
	"tokensmith/internal/dailyusage"
	"time"
)

const (
	SchemaVersion = 1
	MaxDays       = 90
	SourceStaff   = "staff"
)

var SourceOrder = []string{"claude-code", "codex", "grok", "opencode", SourceStaff}

type DayPoint struct {
	Users           float64            `json:"users"`
	MonthlyRevenue  float64            `json:"monthlyRevenue"`
	RnDStock        float64            `json:"rndStock"`
	RnDInflow       map[string]float64 `json:"rndInflow,omitempty"`
	OpenUsers       float64            `json:"openUsers,omitempty"`
	OpenRevenue     float64            `json:"openRevenue,omitempty"`
	OpenRnD         float64            `json:"openRnd,omitempty"`
	OpenSet         bool               `json:"openSet,omitempty"`
}

type Document struct {
	SchemaVersion int                 `json:"schemaVersion"`
	UpdatedAt     int64               `json:"updatedAt"`
	Days          map[string]DayPoint `json:"days"`
}

func EmptyDocument() Document {
	return Document{SchemaVersion: SchemaVersion, Days: map[string]DayPoint{}}
}

func DayKey(at time.Time) string { return dailyusage.DayKey(at) }
```

`internal/metrics/apply.go`: implement EnsureDay, ValidDayKey (`^\d{4}-\d{2}-\d{2}$` + parseable), UpsertSnapshot, AddInflow, Prune (sort keys descending, drop beyond MaxDays), SortedDays, Series.

EnsureDay must copy-on-write style for maps: when reading DayPoint from map, mutate, write back; init `RnDInflow` if nil.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/metrics/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/metrics/
git commit -m "feat(metrics): daily KPI document types and pure apply/prune"
```

---

### Task 2: metrics store Load/Save + corrupt recovery

**Files:**
- Create: `internal/metrics/store.go`
- Create: `internal/metrics/store_test.go`

**Interfaces:**
- Consumes: `Document`, `EmptyDocument`, `SchemaVersion`, `Prune`
- Produces:
  - `type Store struct` with `path string`
  - `func New(path string) *Store`
  - `func (s *Store) Load() (Document, bool, error)` — missing file → `(EmptyDocument(), false, nil)`
  - `func (s *Store) Save(doc Document) error` — mkdir parent, prune, atomic temp+rename, mode 0o600
  - Corrupt / wrong schema: on Load of bad data, rename to `path.corrupt-<unix>` and return empty doc + `ok=true`? **Prefer:** Load returns error for corrupt when called read-only; **Save path** uses recover. Match dailyusage pattern used by TUI:  
    - `Load()`: missing → ok=false; corrupt → rename aside, return EmptyDocument(), ok=true, err=nil (soft recover) OR return err.  
    **Decide for implementers:** soft-recover like dailyusage `loadOrRecover` so TUI never blocks: corrupt → rename + empty document, `ok=true`, `err=nil`.

- [ ] **Step 1: Write failing tests**

```go
func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "metrics-history.json")
	s := New(p)
	doc := EmptyDocument()
	UpsertSnapshot(&doc, "2026-07-14", 1, 2, 3, 10)
	if err := s.Save(doc); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Load()
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.Days["2026-07-14"].Users != 1 {
		t.Fatalf("users=%v", got.Days["2026-07-14"].Users)
	}
}

func TestStoreMissing(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "nope.json"))
	_, ok, err := s.Load()
	if err != nil || ok {
		t.Fatalf("want missing ok=false err=nil, got ok=%v err=%v", ok, err)
	}
}

func TestStoreCorruptRecovers(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "metrics-history.json")
	if err := os.WriteFile(p, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := New(p)
	doc, ok, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !ok || doc.SchemaVersion != SchemaVersion {
		t.Fatalf("recover: ok=%v schema=%d", ok, doc.SchemaVersion)
	}
	// original renamed to corrupt-*
	matches, _ := filepath.Glob(p + ".corrupt-*")
	if len(matches) != 1 {
		t.Fatalf("expected corrupt backup, got %v", matches)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

`go test ./internal/metrics/ -count=1 -run Store`

- [ ] **Step 3: Implement store.go**

Mirror `dailyusage` atomic write (CreateTemp in same dir, Sync, Rename). On decode failure or unsupported schemaVersion (≠1 and not empty fresh), rename corrupt and return EmptyDocument.

- [ ] **Step 4: Run — expect PASS**

`go test ./internal/metrics/ -count=1`

- [ ] **Step 5: Commit**

```bash
git add internal/metrics/
git commit -m "feat(metrics): atomic metrics-history.json store with corrupt recovery"
```

---

### Task 3: Multi-row line chart helper

**Files:**
- Create: `internal/tui/linechart.go`
- Create: `internal/tui/linechart_test.go`

**Interfaces:**
- Consumes: existing `sparkRunes` from `spark.go` (same package) — either export usage of package-level `sparkRunes` or duplicate the rune slice locally as `lineChartRunes` to avoid coupling. **Prefer:** unexported `lineChartRunes` copy of same 8 runes in `linechart.go`.
- Produces:
  - `func lineChart(vals []float64, width, height int) string`
  - `func multiLineChart(series [][]float64, width, height int) string` — shared min/max across all series; each column paints the **topmost** series that occupies that height cell (simple overpaint order: later series wins) OR paint only highest series per column with distinct runes. **v1 simple approach:** for each series, compute column heights independently on shared lo/hi; for each row/col, if any series' height reaches that row, draw that series' rune with preference to last series. For tests, single series is enough to pin behavior.
  - Empty or `<2` points → return placeholder string `資料累積中` (single line; caller may still put in card).
  - `height < 1` → treat as 1; `width < 1` → treat as 1.

**Algorithm for `lineChart`:**

1. If `len(vals) < 2` return `"資料累積中"`.
2. Truncate to last `width` samples if longer; if shorter, use all (chart width = len).
3. Find lo, hi; if hi==lo, all columns mid height `height/2` or full mid rune row.
4. For each column i, `level = int((v-lo)/(hi-lo) * float64(height*len(runes)-1))` mapped into row of solid blocks: for row `r` from top (height-1) to 0, if column height > r, put `█` else ` ` (space). Simpler v1 matching spec: **one spark rune per column on a single baseline is insufficient for h>1**. Use:

```
// colHeight in 0..height inclusive
h := 0
if hi > lo {
  h = int(math.Round((v-lo)/(hi-lo) * float64(height)))
}
// for row from top: empty or '█'
```

5. Join rows with `\n`. lipgloss width of each row must be equal (pad spaces).

- [ ] **Step 1: Failing tests**

```go
func TestLineChartEmpty(t *testing.T) {
	if lineChart(nil, 10, 5) != "資料累積中" {
		t.Fatal("empty")
	}
	if lineChart([]float64{1}, 10, 5) != "資料累積中" {
		t.Fatal("single")
	}
}

func TestLineChartMonotone(t *testing.T) {
	out := lineChart([]float64{0, 1, 2, 3}, 4, 4)
	lines := strings.Split(out, "\n")
	if len(lines) != 4 {
		t.Fatalf("rows=%d want 4\n%s", len(lines), out)
	}
	// last column should be tallest: bottom row all filled or last col filled
	bottom := lines[len(lines)-1]
	if !strings.Contains(bottom, "█") {
		t.Fatalf("bottom empty: %q", bottom)
	}
}

func TestLineChartFlat(t *testing.T) {
	out := lineChart([]float64{5, 5, 5}, 3, 3)
	if strings.Contains(out, "資料累積中") {
		t.Fatal("flat should render")
	}
}
```

- [ ] **Step 2: Run fail** — `go test ./internal/tui/ -count=1 -run LineChart`

- [ ] **Step 3: Implement** `linechart.go`

- [ ] **Step 4: Pass** — `go test ./internal/tui/ -count=1 -run LineChart`

- [ ] **Step 5: Commit**

```bash
git add internal/tui/linechart.go internal/tui/linechart_test.go
git commit -m "feat(tui): multi-row unicode lineChart helper"
```

---

### Task 4: Nav shell — PageDashboard + empty page

**Files:**
- Modify: `internal/tui/tui.go` (Page constants, `pageNames`, `renderPage`, `pageKeys`)
- Create: `internal/tui/page_dashboard.go` (stub render)
- Modify: `internal/tui/nav_test.go` (and any test hardcoding page indices / `numPages == 8`)

**Interfaces:**
- Produces:
  - `PageDashboard` between Overview and WarRoom
  - `pageNames = [9]string{"總覽","儀表板","戰情室","模型","市場","算力","團隊","科技","成就"}`
  - `func renderDashboard(m Model) string` — stub with three Card titles: `用戶增長`, `營收增長`, `R&D 增長`
  - `pageKeys` for dashboard: empty page-specific or `"檢視增長曲線"` muted style optional — keep minimal: return `""` or short hint without new shortcuts

- [ ] **Step 1: Update failing nav tests first**

In `nav_test.go`:

```go
func TestTabCyclesPages(t *testing.T) {
	m := testModel(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if nm.(Model).page != PageDashboard {
		t.Fatalf("after Tab = %v, want dashboard", nm.(Model).page)
	}
}

func TestNumberKeyJumpsPage(t *testing.T) {
	m := testModel(t)
	// 1總覽 2儀表板 3戰情 4模型 5市場 6算力 7團隊 8科技 9成就
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'7'}})
	if nm.(Model).page != PageTeam {
		t.Fatalf("key 7 = %v, want team", nm.(Model).page)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if nm.(Model).page != PageDashboard {
		t.Fatalf("key 2 = %v, want dashboard", nm.(Model).page)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	if nm.(Model).page != PageWarRoom {
		t.Fatalf("key 3 = %v, want war room", nm.(Model).page)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'9'}})
	if nm.(Model).page != PageAchievements {
		t.Fatalf("key 9 = %v, want achievements", nm.(Model).page)
	}
}

func TestNumPagesIsNine(t *testing.T) {
	if numPages != 9 {
		t.Fatalf("numPages=%d want 9", numPages)
	}
	if pageNames[1] != "儀表板" {
		t.Fatalf("pageNames[1]=%q", pageNames[1])
	}
	if pageNames[2] != "戰情室" {
		t.Fatalf("pageNames[2]=%q", pageNames[2])
	}
}

func TestDashboardPageRendersTitles(t *testing.T) {
	m := testModel(t)
	m.page = PageDashboard
	m.width, m.height = 120, 40
	m.resize(m.width, m.height)
	v := m.View()
	for _, want := range []string{"儀表板", "用戶增長", "營收增長", "R&D 增長"} {
		if !strings.Contains(v, want) {
			t.Fatalf("missing %q in:\n%s", want, v)
		}
	}
}
```

Rename `TestNumPagesIsEight` → `TestNumPagesIsNine`.

Grep whole `internal/tui` for hardcoded assumptions:

```bash
grep -n "numPages\|PageWarRoom\|key 2\|pageNames\[1\]\|\[2\]戰情\|8 成就\|numPages != 8" internal/tui/*_test.go internal/tui/*.go
```

Fix every hit that encodes old indices (overview pending strip mentions `[2]戰情室` must become `[3]戰情室` in `page_overview.go` if present).

- [ ] **Step 2: Run tests — expect FAIL**

`go test ./internal/tui/ -count=1 -run 'NumPages|NumberKey|TabCycles|DashboardPage'`

- [ ] **Step 3: Implement page constants + stub page**

```go
const (
	PageOverview Page = iota
	PageDashboard
	PageWarRoom
	PageModels
	PageMarket
	PageCompute
	PageTeam
	PageTech
	PageAchievements
	numPages
)

var pageNames = [numPages]string{"總覽", "儀表板", "戰情室", "模型", "市場", "算力", "團隊", "科技", "成就"}
```

`renderPage` add `case PageDashboard: return renderDashboard(m)`.

`page_dashboard.go` stub:

```go
func renderDashboard(m Model) string {
	cw := m.contentWidth()
	if cw < 20 {
		cw = 20
	}
	return VStack(
		CardIn(CardDefault, cw, "用戶增長", styleMuted.Render("資料累積中")),
		CardIn(CardDefault, cw, "營收增長", styleMuted.Render("資料累積中")),
		CardIn(CardDefault, cw, "R&D 增長", styleMuted.Render("資料累積中")),
	)
}
```

Update overview pending strip text from `[2]戰情室` → `[3]戰情室` wherever it appears.

Any `case PageOverview || PageWarRoom` key handlers that should also work from dashboard: **only if** they are global already; do not steal dashboard-only keys. Campaign `e` may stay overview/warroom only.

- [ ] **Step 4: Pass**

`go test ./internal/tui/ -count=1`

Fix all broken page-index tests in one go.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): add Dashboard page at nav key 2"
```

---

### Task 5: Session dash rings + short-window charts

**Files:**
- Modify: `internal/tui/tui.go` (Model fields, `newAt`/`newAtPaths` init)
- Modify: `internal/tui/display.go` (sample dash rings)
- Modify: `internal/tui/page_dashboard.go` (render short charts)
- Modify: `internal/tui/page_dashboard_test.go` / `display_test.go` as needed

**Interfaces:**
- Produces on `Model`:
  - `dashUsers, dashRevenue, dashRnDStock spark` (capacity 120)
  - Sample every 4 display ticks (same gate as existing sparks or share `sparkTick%4==0`)
  - Values: users from `m.disp.TotalUsers` if `dispReady` else `sim.TotalUsers`; revenue `sim.MonthlyRevenue(m.state)`; rnd `m.state.Resources.RnD`
- `renderDashboard` uses `lineChart(m.dashUsers.values(), chartW, 5)` etc. for short window section labeled `近況` or `短窗`

- [ ] **Step 1: Failing test**

```go
func TestDashRingsSampleStocks(t *testing.T) {
	m := testModel(t)
	m.state.Models = []model.Model{{Online: true, Users: 1000, Price: 10}}
	m.state.Resources.RnD = 500
	m.dispReady = true
	m.disp.TotalUsers = 1000
	// force enough display updates
	for i := 0; i < 20; i++ {
		m.sparkTick++ // if sampling is inside updateDisplay, call that instead
		// Prefer: drive through real tick path if accessible
		updateDisplayForTest(t, &m) // only if helper exists; else call m's unexported path via Update(tickMsg)
	}
	if m.dashUsers.n < 2 {
		t.Fatalf("dashUsers n=%d", m.dashUsers.n)
	}
	if m.dashRevenue.n < 1 || m.dashRnDStock.n < 1 {
		t.Fatal("revenue/rnd not sampled")
	}
}
```

If `updateDisplay` is unexported method on `*Model`, use `Update(tickMsg(...))` loop with fixed state. Inspect `display.go` for the function name (`stepDisplay` / etc.) and call the same path production uses.

Also:

```go
func TestDashboardShortChartAfterSamples(t *testing.T) {
	m := testModel(t)
	for _, v := range []float64{1, 2, 3, 4, 5} {
		m.dashUsers.push(v)
		m.dashRevenue.push(v * 10)
		m.dashRnDStock.push(v * 100)
	}
	m.page = PageDashboard
	m.width, m.height = 120, 50
	m.resize(m.width, m.height)
	body := renderDashboard(m)
	if strings.Contains(body, "資料累積中") && !strings.Contains(body, "█") {
		// at least one chart should have blocks
		t.Fatalf("expected chart blocks:\n%s", body)
	}
}
```

- [ ] **Step 2: Run fail**

- [ ] **Step 3: Implement sampling + render**

In `newAtPaths` after other sparks:

```go
m.dashUsers = newSpark(120)
m.dashRevenue = newSpark(120)
m.dashRnDStock = newSpark(120)
```

In display sample block (alongside valuation spark):

```go
if m.sparkTick%4 == 0 {
	// existing sparks...
	users := sim.TotalUsers(m.state)
	if m.dispReady {
		users = m.disp.TotalUsers
	}
	m.dashUsers.push(users)
	m.dashRevenue.push(sim.MonthlyRevenue(m.state))
	m.dashRnDStock.push(m.state.Resources.RnD)
}
```

`renderDashboard`: compute `chartW := cw - 6`; if chartW < 8 { chartW = 8 }; height 5 wide / 3 narrow (`cw < 100`).

Card body pattern:

```
KV latest
lineChart short
```

- [ ] **Step 4: Pass**

`go test ./internal/tui/ -count=1 -run 'Dash|Dashboard'`

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): session dash rings and short-window dashboard charts"
```

---

### Task 6: Wire metrics store into Model — load, flush, day roll

**Files:**
- Modify: `internal/tui/tui.go` (`newAtPaths`, tick/autosave, quit save)
- Create helpers in `internal/tui/metrics_sync.go` (optional, keeps tui.go smaller) OR inline methods on Model
- Test: `internal/tui/metrics_sync_test.go`

**Interfaces:**
- Model fields:
  - `metricsPath string`
  - `metricsStore *metrics.Store` (nil-safe if path empty)
  - `metricsDoc metrics.Document`
  - `metricsDay string` // last seen local day key
  - `metricsFlushTicks int`
  - `metricsDirty bool`
- Constants: `metricsFlushEveryTicks = 120` // 120 * 250ms ≈ 30s
- Methods:
  - `func (m *Model) metricsSnapshotNow(now time.Time)` — UpsertSnapshot for current day
  - `func (m *Model) metricsMaybeRollDay(now time.Time)` — if day changed, flush old, reset day
  - `func (m *Model) metricsFlush()` — Save if store non-nil; clear dirty on success; on error setNotice once optional
  - Call `metricsMaybeRollDay` + accumulate flush ticks on each game tick; flush on autosave and on graceful quit paths that already `store.Save`

Load in `newAtPaths`:

```go
metricsPath := filepath.Join(filepath.Dir(savePath), "metrics-history.json")
ms := metrics.New(metricsPath)
doc := metrics.EmptyDocument()
if d, ok, err := ms.Load(); err == nil && ok {
	doc = d
}
// assign to model
m.metricsDay = metrics.DayKey(time.Now())
```

- [ ] **Step 1: Failing integration-style test**

```go
func TestMetricsFlushPersistsSnapshot(t *testing.T) {
	dir := t.TempDir()
	save := filepath.Join(dir, "s.json")
	m := newAt(save)
	m.poller = ingestEmptyPoller(t)
	m.state.Models = []model.Model{{Online: true, Users: 42, Price: 12}}
	m.state.Resources.RnD = 7
	m.metricsSnapshotNow(time.Now())
	m.metricsDirty = true
	m.metricsFlush()
	s := metrics.New(filepath.Join(dir, "metrics-history.json"))
	doc, ok, err := s.Load()
	if err != nil || !ok {
		t.Fatalf("load: %v ok=%v", err, ok)
	}
	day := metrics.DayKey(time.Now())
	p := doc.Days[day]
	if p.Users != 42 {
		t.Fatalf("users=%v want 42", p.Users)
	}
}

func TestMetricsDayRollResetsInflowOnly(t *testing.T) {
	// Set metricsDay to yesterday, add inflow, roll to today, ensure new day empty inflow and old day retained.
}
```

- [ ] **Step 2: Run fail**

- [ ] **Step 3: Implement sync methods + hooks**

On each tick (where daily usage day is updated):

```go
now := time.Now()
m.metricsMaybeRollDay(now)
m.metricsSnapshotNow(now) // keep latest stock; Open* freezes first write
m.metricsDirty = true
m.metricsFlushTicks++
if m.metricsFlushTicks >= metricsFlushEveryTicks {
	m.metricsFlushTicks = 0
	m.metricsFlush()
}
```

Also flush when existing autosave succeeds and on quit.

**Important:** Snapshot every tick is fine (in-memory); disk only on flush interval.

- [ ] **Step 4: Pass**

`go test ./internal/tui/ -count=1 -run Metrics`

- [ ] **Step 5: Commit**

```bash
git add internal/tui/ internal/metrics/
git commit -m "feat(tui): persist dashboard KPI daily snapshots to metrics-history"
```

---

### Task 7: R&D inflow attribution (token + staff) + long-window multi-line

**Files:**
- Modify: `internal/tui/tui.go` (where `lastTokenRnD` is filled ~line 587–595)
- Modify: economy tick path (same tick function after `sim.Tick`) for staff inflow
- Modify: `internal/tui/page_dashboard.go` (long charts + R&D inflow section)
- Test: `internal/tui/metrics_inflow_test.go`, extend dashboard render tests

**Interfaces:**
- When computing per-source `rnd` map for `lastTokenRnD`, also:

```go
day := metrics.DayKey(time.Now())
for src, amt := range rnd {
	metrics.AddInflow(&m.metricsDoc, day, src, amt, time.Now().Unix())
}
m.metricsDirty = true
```

- After sim tick advances economy by `dt` real/game seconds that grant staff R&D, add:

```go
staff := sim.RnDRatePerSec(prevOrNewState, cfg) * dt
// Use the same state/dt the tick used so the integral matches Resources.RnD gain from staff.
metrics.AddInflow(&m.metricsDoc, day, metrics.SourceStaff, staff, nowUnix)
```

Find exact `dt` variable in the tick handler (likely `gameSecPerRealSec * tickInterval.Seconds()` or clock-split). Match staff contribution to what sim actually applied this tick — if tick uses economy clock dt, use that.

- Long window series:

```go
days := metrics.SortedDays(m.metricsDoc)
// optionally filter last 90 already pruned
users := metrics.Series(m.metricsDoc, days, func(p metrics.DayPoint) float64 { return p.Users })
// labels: first/mid/last day strings under chart
```

- R&D inflow multi-line:

```go
var series [][]float64
for _, src := range metrics.SourceOrder {
	series = append(series, metrics.Series(m.metricsDoc, days, func(p metrics.DayPoint) float64 {
		if p.RnDInflow == nil { return 0 }
		return p.RnDInflow[src]
	}))
}
chart := multiLineChart(series, chartW, 5)
```

- Today KV legend (always show SourceOrder):

```go
p := m.metricsDoc.Days[m.metricsDay]
for _, src := range metrics.SourceOrder {
  lines = append(lines, fmt.Sprintf("%s %s", sourceLabel(src), human(p.RnDInflow[src])))
}
```

Reuse existing `sourceLabel` from tui if present.

- [ ] **Step 1: Failing tests**

```go
func TestTokenInflowMatchesLastTokenRnD(t *testing.T) {
	// Arrange model with prestige/streak/hq known; inject one token event via tick
	// Assert metricsDoc day inflow for claude-code equals lastTokenRnD[claude-code]
}

func TestStaffInflowPositiveWhenResearchers(t *testing.T) {
	// Seed employee researcher power; tick without tokens; staff inflow > 0
}

func TestDashboardLongWindowUsesDoc(t *testing.T) {
	m := testModel(t)
	doc := metrics.EmptyDocument()
	metrics.UpsertSnapshot(&doc, "2026-07-10", 10, 100, 1, 1)
	metrics.UpsertSnapshot(&doc, "2026-07-11", 20, 200, 2, 2)
	metrics.UpsertSnapshot(&doc, "2026-07-12", 40, 400, 3, 3)
	m.metricsDoc = doc
	body := renderDashboard(m)
	if !strings.Contains(body, "近 90 日") && !strings.Contains(body, "歷史") {
		// use whatever section title you pick in render — pin string in test to actual title
	}
	_ = body
}
```

Pick fixed section titles in render:
- short: `近況`
- long: `近 90 日`
- inflow: `流入 by 來源`

- [ ] **Step 2: Run fail**

- [ ] **Step 3: Implement attribution + long render**

R&D card structure:

```
庫存 <val> (Δ今日 ...)
近況: lineChart dashRnDStock
近 90 日: lineChart long rndStock
流入 by 來源（僅正入帳；庫存含消耗）
multiLineChart long inflows
今日: Claude … · Staff …
```

- [ ] **Step 4: Pass**

`go test ./internal/tui/ -count=1`

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): R&D inflow by source and long-window dashboard charts"
```

---

### Task 8: Δ今日, polish, empty copy, full test sweep

**Files:**
- Modify: `internal/tui/page_dashboard.go`
- Modify tests as needed
- Grep/fix any remaining `[2]戰情室` / page index docs in comments only if user-facing

**Interfaces:**
- Helper:

```go
func deltaToday(open, now float64, openSet bool) string {
	if !openSet {
		return ""
	}
	d := now - open
	sign := "+"
	if d < 0 {
		sign = "" // human() may not include sign; use fmt with sign
	}
	return fmt.Sprintf("(%s%s 今日)", sign, human(d)) // fix sign for negative
}
```

Use Open* from **today's** DayPoint for stock cards.

Empty long history: if `len(SortedDays) < 2` show `尚無歷史 · 掛機或跨日後會出現` instead of chart.

Narrow height: `h := 5; if cw < 100 { h = 3 }`

- [ ] **Step 1: Tests**

```go
func TestDeltaTodayFormatting(t *testing.T) { ... }

func TestDashboardEmptyHistoryCopy(t *testing.T) {
	m := testModel(t)
	m.metricsDoc = metrics.EmptyDocument()
	body := renderDashboard(m)
	if !strings.Contains(body, "尚無歷史") {
		t.Fatalf("want empty history copy:\n%s", body)
	}
}
```

- [ ] **Step 2–4: Implement, pass**

`go test ./internal/metrics/ ./internal/tui/ -count=1`

Then: `go test ./... -count=1` (fix any stragglers outside tui)

- [ ] **Step 5: Commit**

```bash
git add internal/tui/ internal/metrics/
git commit -m "feat(tui): dashboard delta-today labels and empty-state polish"
```

---

## Spec coverage checklist (plan self-review)

| Spec requirement | Task |
|---|---|
| New page 儀表板 at key 2; war room shifts | Task 4 |
| Users / MonthlyRevenue / RnD stock metrics | Task 5–6 |
| Hybrid short + real calendar long | Task 5 + 6 |
| Sidecar metrics-history.json, not GameState/dailyusage | Task 2, 6 |
| 90-day prune | Task 1 |
| Open-of-day for Δ今日 | Task 1 Open*, Task 8 |
| R&D stock main + inflow by source | Task 7 |
| Token formula = lastTokenRnD | Task 7 |
| Staff inflow | Task 7 |
| multi-row unicode charts, zero deps | Task 3 |
| Today inflow KV legend | Task 7 |
| Corrupt recovery | Task 2 |
| Daemon does not sole-write metrics | Task 6 (TUI only) |
| Nav tests / overview `[3]戰情室` | Task 4 |
| Empty copy | Task 8 |

## Placeholder / consistency notes (resolved in plan)

- `DayPoint` JSON includes `openUsers`/`openRevenue`/`openRnd`/`openSet` so Δ今日 survives restart (stronger than in-memory-only open; aligns with spec intent).
- `multiLineChart` v1: shared scale, overpaint by series order (SourceOrder).
- Section titles pinned: `近況`, `近 90 日`, `流入 by 來源`.

---

## Execution handoff

Plan complete and saved to `docs/superpowers/plans/2026-07-14-dashboard-growth-charts.md`.

**Two execution options:**

1. **Subagent-Driven (recommended)** — fresh subagent per task, review between tasks  
2. **Inline Execution** — this session with executing-plans and checkpoints  

Which approach?
