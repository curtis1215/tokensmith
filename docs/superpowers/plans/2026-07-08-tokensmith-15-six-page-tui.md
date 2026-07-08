# Six-Page TUI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Grow the single-screen prototype into the full six-page Bubble Tea TUI (總覽 · 模型 · 市場 · 算力 · 團隊 · 科技) so every already-implemented sim system is reachable and playable.

**Architecture:** The sim core stays the single source of truth; the TUI is a view + command dispatcher over it (same in-process model as the current prototype — daemon/IPC split is deferred). A thin `internal/sim/view.go` exposes the derived values panels need (effective capacity, utilisation, market rank, milestone progress) as pure, tested functions. The Bubble Tea `Model` gains a current-page enum and an optional modal-dialog state; `Update` routes keys to the active page or the open dialog, and `View` renders a persistent resource bar + tab bar + the active page (or dialog overlay).

**Tech Stack:** Go 1.22+, Bubble Tea v1.3.10, Lipgloss v1.1.0. No new dependencies.

## Global Constraints

- Module `tokensmith`; Go 1.22+; pnpm is irrelevant (Go project).
- `internal/sim` stays pure: no wall-clock, no rand, no I/O; time advances only via `dt`. New `view.go` helpers must also be pure (derive from `GameState`+`Config` only).
- Non-disruptive: every existing test across all 7 packages must stay green. New sim fields/commands default to neutral (e.g. `Segment` zero value = `SegConsumer`, preserving current behaviour).
- TUI is tested through `Model.Update(msg)` (assert resulting state) and `Model.View()` (assert substring), hermetically: construct with `newAt(t.TempDir()+"/s.json")` and swap in `ingestEmptyPoller(t)` so no real `~/.claude` read and no real save write.
- Chinese double-width alignment is handled by Lipgloss at render time; tests assert on substrings/values, never exact column widths.
- Keep files focused: one render file per page under `internal/tui/`.

---

## File Structure

- `internal/sim/view.go` (new) — pure derived-value helpers for the UI: `EffectiveTraining`, `EffectiveInference`, `TotalUsers`, `MonthlyRevenue`, `MarketRank`, `NextMilestone`.
- `internal/sim/view_test.go` (new) — tests for the above.
- `internal/model/types.go` (modify) — add `Segment` to `StartTraining`.
- `internal/sim/apply.go` (modify) — carry `Segment` into the training job.
- `internal/sim/sim.go` (modify) — set `Segment` on the completed model.
- `internal/tui/tui.go` (modify) — `Model` gains `page` + `dialog`; `Update` routes; `View` = resource bar + tab bar + active page/dialog. Shared styles + helpers (progress bar, resource bar, tab bar).
- `internal/tui/page_overview.go`, `page_models.go`, `page_market.go`, `page_compute.go`, `page_team.go`, `page_tech.go` (new) — one `render<Page>(m Model) string` each.
- `internal/tui/dialog_train.go` (new) — the training budget-allocation modal (state + update + render).
- `internal/tui/*_test.go` (new/modify) — per-file tests.

---

## Task 1: Sim view-model helpers

**Files:**
- Create: `internal/sim/view.go`
- Test: `internal/sim/view_test.go`

**Interfaces:**
- Consumes: `model.GameState`, `balance.Config`, existing unexported `effectiveTraining`/`effectiveInference`.
- Produces:
  - `func EffectiveTraining(ns model.GameState, b balance.Config) float64`
  - `func EffectiveInference(ns model.GameState, b balance.Config) float64`
  - `func TotalUsers(ns model.GameState) float64`
  - `func MonthlyRevenue(ns model.GameState) float64` — Σ online `Users*Price`
  - `func MarketRank(ns model.GameState, b balance.Config, seg model.Segment) (rank, total int)` — player's 1-based rank by appeal in `seg` among {player-best-model, each competitor}; `total` = competitors+1
  - `func NextMilestone(ns model.GameState, b balance.Config) (target, progress float64, ok bool)` — next unreached `ValuationMilestones` entry, `progress`∈[0,1] = `PeakValuation/target`; `ok=false` if all reached

- [ ] **Step 1: Write failing tests**

```go
package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestEffectiveCapacityExported(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Compute.TrainingCapacity = 4
	s.Compute.InferenceCapacity = 2
	if EffectiveTraining(s, b) != 4 {
		t.Errorf("EffectiveTraining = %v, want 4", EffectiveTraining(s, b))
	}
	if EffectiveInference(s, b) != 2 {
		t.Errorf("EffectiveInference = %v, want 2", EffectiveInference(s, b))
	}
}

func TestTotalUsersAndRevenue(t *testing.T) {
	s := model.GameState{Models: []model.Model{
		{Online: true, Users: 100, Price: 12},
		{Online: false, Users: 999, Price: 99}, // offline excluded
		{Online: true, Users: 50, Price: 6},
	}}
	if TotalUsers(s) != 150 {
		t.Errorf("TotalUsers = %v, want 150", TotalUsers(s))
	}
	if MonthlyRevenue(s) != 100*12+50*6 {
		t.Errorf("MonthlyRevenue = %v, want 1500", MonthlyRevenue(s))
	}
}

func TestMarketRankBeatsWeakField(t *testing.T) {
	b := balance.Default()
	strong := onlineModel(80, b.RefPrice) // high capability
	s := model.GameState{
		Models:      []model.Model{strong},
		Competitors: []model.Competitor{{Name: "weak"}},
	}
	rank, total := MarketRank(s, b, model.SegConsumer)
	if rank != 1 || total != 2 {
		t.Errorf("rank=%d total=%d, want 1/2", rank, total)
	}
}

func TestNextMilestone(t *testing.T) {
	b := balance.Default()
	s := model.GameState{PeakValuation: 5e5} // below first milestone 1e6
	target, prog, ok := NextMilestone(s, b)
	if !ok || target != 1e6 || prog != 0.5 {
		t.Errorf("got target=%v prog=%v ok=%v, want 1e6/0.5/true", target, prog, ok)
	}
}
```

- [ ] **Step 2: Run to verify fail**

Run: `go test ./internal/sim -run 'TestEffectiveCapacityExported|TestTotalUsersAndRevenue|TestMarketRank|TestNextMilestone' -v`
Expected: FAIL (undefined: EffectiveTraining, …).

- [ ] **Step 3: Implement `internal/sim/view.go`**

```go
package sim

import "tokensmith/internal/model"

import "tokensmith/internal/balance"

// EffectiveTraining is the exported view of self-built + rented training compute.
func EffectiveTraining(ns model.GameState, b balance.Config) float64 {
	return effectiveTraining(ns, b)
}

// EffectiveInference is the exported view of self-built + rented inference compute.
func EffectiveInference(ns model.GameState, b balance.Config) float64 {
	return effectiveInference(ns, b)
}

// TotalUsers sums users across online models.
func TotalUsers(ns model.GameState) float64 {
	var u float64
	for _, m := range ns.Models {
		if m.Online {
			u += m.Users
		}
	}
	return u
}

// MonthlyRevenue is the aggregate per-month subscription revenue of online models.
func MonthlyRevenue(ns model.GameState) float64 {
	var r float64
	for _, m := range ns.Models {
		if m.Online {
			r += m.Users * m.Price
		}
	}
	return r
}

// MarketRank returns the player's 1-based rank by appeal in seg among the
// player's best online model and every competitor, plus the field size.
func MarketRank(ns model.GameState, b balance.Config, seg model.Segment) (rank, total int) {
	w := b.SegmentWeights[seg]
	best := 0.0
	for _, m := range ns.Models {
		if m.Online {
			if a := appealOf(m.Quality, w); a > best {
				best = a
			}
		}
	}
	rank = 1
	for _, c := range ns.Competitors {
		if appealOf(c.Quality, w) > best {
			rank++
		}
	}
	return rank, len(ns.Competitors) + 1
}

// NextMilestone returns the next unreached valuation milestone and progress
// toward it. ok is false when every milestone has been reached.
func NextMilestone(ns model.GameState, b balance.Config) (target, progress float64, ok bool) {
	if ns.MilestonesReached >= len(b.ValuationMilestones) {
		return 0, 0, false
	}
	target = b.ValuationMilestones[ns.MilestonesReached]
	progress = ns.PeakValuation / target
	if progress > 1 {
		progress = 1
	}
	return target, progress, true
}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/sim -run 'TestEffectiveCapacityExported|TestTotalUsersAndRevenue|TestMarketRank|TestNextMilestone' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/sim/view.go internal/sim/view_test.go
git commit -m "feat(sim): pure view-model helpers for the TUI"
```

---

## Task 2: StartTraining carries a target segment

**Files:**
- Modify: `internal/model/types.go` (StartTraining struct)
- Modify: `internal/sim/apply.go` (applyStartTraining → TrainingJob)
- Modify: `internal/sim/sim.go` (advanceTraining → built Model)
- Test: `internal/sim/apply_test.go`

**Interfaces:**
- Produces: `model.StartTraining{Gen int; Segment model.Segment; Alloc [4]float64; Price float64}`; a completed model's `Segment` equals the job's segment. Note `TrainingJob` already has no segment — add one.

- [ ] **Step 1: Write failing test** (append to `apply_test.go`)

```go
func TestStartTrainingCarriesSegment(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Resources.RnD = 50000
	s.Compute.TrainingCapacity = 100 // finish fast
	cmd := model.StartTraining{Gen: 1, Segment: model.SegEnterprise, Alloc: validAlloc(), Price: 180}
	ns, err := Apply(s, cmd, b)
	if err != nil {
		t.Fatal(err)
	}
	if ns.Training.Segment != model.SegEnterprise {
		t.Fatalf("job segment = %v, want Enterprise", ns.Training.Segment)
	}
	// tick to completion; the online model must keep the segment
	for i := 0; i < 100 && ns.HasTraining; i++ {
		ns = Tick(ns, 3600, nil, b)
	}
	if len(ns.Models) == 0 || ns.Models[len(ns.Models)-1].Segment != model.SegEnterprise {
		t.Fatalf("completed model segment wrong: %+v", ns.Models)
	}
}
```

- [ ] **Step 2: Run to verify fail**

Run: `go test ./internal/sim -run TestStartTrainingCarriesSegment -v`
Expected: FAIL (unknown field Segment in StartTraining / TrainingJob).

- [ ] **Step 3: Implement**

In `internal/model/types.go`, add `Segment` to `StartTraining` and `TrainingJob`:

```go
type StartTraining struct {
	Gen     int
	Segment Segment
	Alloc   [NumQualityDims]float64
	Price   float64
}
```
```go
type TrainingJob struct {
	Gen           int
	Segment       Segment
	Alloc         [NumQualityDims]float64
	Price         float64
	WorkRemaining float64
}
```

In `internal/sim/apply.go` `applyStartTraining`, set it on the job:

```go
	ns.Training = model.TrainingJob{
		Gen:           c.Gen,
		Segment:       c.Segment,
		Alloc:         c.Alloc,
		Price:         c.Price,
		WorkRemaining: b.GenTrainWorkGPUSec[c.Gen] * te.TrainWorkMult,
	}
```

In `internal/sim/sim.go` `advanceTraining`, set it on the built model:

```go
	m := model.Model{Gen: job.Gen, Segment: job.Segment, Price: job.Price, Online: true}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/sim -run TestStartTrainingCarriesSegment -v` → PASS
Run: `go test ./... ` → all green (zero value keeps SegConsumer, so prior tests unaffected).

- [ ] **Step 5: Commit**

```bash
git add internal/model/types.go internal/sim/apply.go internal/sim/sim.go internal/sim/apply_test.go
git commit -m "feat(sim): StartTraining carries a target market segment"
```

---

## Task 3: Page navigation scaffold + persistent chrome

**Files:**
- Modify: `internal/tui/tui.go`
- Create: `internal/tui/page_overview.go` … `page_tech.go` (stubs)
- Test: `internal/tui/nav_test.go`

**Interfaces:**
- Produces:
  - `type Page int` with `PageOverview, PageModels, PageMarket, PageCompute, PageTeam, PageTech` and `numPages = 6`.
  - `Model.page Page` field.
  - `renderResourceBar(m Model) string`, `renderTabBar(p Page) string`, `progressBar(frac float64, width int) string`.
  - `render<Page>(m Model) string` for each page (stubs returning the page's title for now).
- Consumes: existing `Model`, `sim`, `balance`.

- [ ] **Step 1: Write failing tests** (`internal/tui/nav_test.go`)

```go
package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func testModel(t *testing.T) Model {
	m := newAt(filepath.Join(t.TempDir(), "s.json"))
	m.poller = ingestEmptyPoller(t)
	return m
}

func TestTabCyclesPages(t *testing.T) {
	m := testModel(t)
	if m.page != PageOverview {
		t.Fatalf("start page = %v, want overview", m.page)
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if nm.(Model).page != PageModels {
		t.Fatalf("after Tab = %v, want models", nm.(Model).page)
	}
}

func TestNumberKeyJumpsPage(t *testing.T) {
	m := testModel(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}})
	if nm.(Model).page != PageTeam {
		t.Fatalf("key 5 = %v, want team", nm.(Model).page)
	}
}

func TestViewHasChrome(t *testing.T) {
	m := testModel(t)
	v := m.View()
	if !strings.Contains(v, "Tokensmith") || !strings.Contains(v, "總覽") || !strings.Contains(v, "團隊") {
		t.Fatalf("view missing chrome:\n%s", v)
	}
}

func TestProgressBar(t *testing.T) {
	got := progressBar(0.5, 10)
	full := strings.Count(got, "▓")
	if full != 5 {
		t.Fatalf("progressBar(0.5,10) filled=%d, want 5 (%q)", full, got)
	}
}
```

- [ ] **Step 2: Run to verify fail**

Run: `go test ./internal/tui -run 'TestTabCyclesPages|TestNumberKeyJumpsPage|TestViewHasChrome|TestProgressBar' -v`
Expected: FAIL (undefined PageOverview / progressBar / page field).

- [ ] **Step 3: Implement scaffold**

Add to `internal/tui/tui.go` — the page enum, `page` field, key routing, and chrome. Replace the current `View` body to compose chrome + active page, and keep the `t/r/i` prototype keys working via the page dispatch until later tasks replace them.

```go
type Page int

const (
	PageOverview Page = iota
	PageModels
	PageMarket
	PageCompute
	PageTeam
	PageTech
	numPages
)

var pageNames = [numPages]string{"總覽", "模型", "市場", "算力", "團隊", "科技"}
```

Add `page Page` to the `Model` struct. In `Update`'s `tea.KeyMsg` switch, add navigation BEFORE the existing action keys:

```go
		case "tab", "right":
			m.page = (m.page + 1) % numPages
			return m, nil
		case "shift+tab", "left":
			m.page = (m.page + numPages - 1) % numPages
			return m, nil
		case "1", "2", "3", "4", "5", "6":
			m.page = Page(msg.String()[0] - '1')
			return m, nil
```

Chrome + dispatch helpers (in `tui.go`):

```go
func renderResourceBar(m Model) string {
	s := m.state
	trainUtil := 0.0
	if cap := sim.EffectiveTraining(s, m.cfg); cap > 0 && s.HasTraining {
		trainUtil = 1 // a job fully occupies the training pool in v0
	}
	infCap := sim.EffectiveInference(s, m.cfg)
	infUtil := 0.0
	if infCap > 0 {
		infUtil = s.Compute.InferenceLoad / infCap
	}
	bar := fmt.Sprintf("💰 $%s   ⚡R&D %.0f/s   🖥訓練%.0f%% 推理%.0f%%   📈估值 $%s",
		human(s.Resources.Cash), m.state.Resources.RnD, trainUtil*100, infUtil*100,
		human(sim.Valuation(s, m.cfg)))
	if m.lastTokens > 0 {
		bar += fmt.Sprintf("   ⚡token +%d", m.lastTokens)
	}
	return bar
}

func renderTabBar(p Page) string {
	var parts []string
	for i, name := range pageNames {
		label := fmt.Sprintf("[%d]%s", i+1, name)
		if Page(i) == p {
			label = tabActiveStyle.Render(label)
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, "  ")
}

func progressBar(frac float64, width int) string {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	n := int(frac * float64(width))
	return strings.Repeat("▓", n) + strings.Repeat("░", width-n)
}

// human formats large numbers as e.g. 1.8M, 340k.
func human(v float64) string {
	switch {
	case v >= 1e9:
		return fmt.Sprintf("%.2fB", v/1e9)
	case v >= 1e6:
		return fmt.Sprintf("%.2fM", v/1e6)
	case v >= 1e3:
		return fmt.Sprintf("%.0fk", v/1e3)
	default:
		return fmt.Sprintf("%.0f", v)
	}
}

func (m Model) renderPage() string {
	switch m.page {
	case PageModels:
		return renderModels(m)
	case PageMarket:
		return renderMarket(m)
	case PageCompute:
		return renderCompute(m)
	case PageTeam:
		return renderTeam(m)
	case PageTech:
		return renderTech(m)
	default:
		return renderOverview(m)
	}
}
```

Add `tabActiveStyle` near the other styles:

```go
	tabActiveStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Underline(true)
```

Replace `View` with chrome + page:

```go
func (m Model) View() string {
	body := lipgloss.JoinVertical(lipgloss.Left,
		renderResourceBar(m),
		strings.Repeat("─", 66),
		renderTabBar(m.page),
		strings.Repeat("─", 66),
		m.renderPage(),
	)
	return boxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, titleStyle.Render("Tokensmith"), body))
}
```

Create six stub page files, each e.g. `internal/tui/page_overview.go`:

```go
package tui

func renderOverview(m Model) string { return "總覽" }
```
…and `renderModels`→"模型", `renderMarket`→"市場", `renderCompute`→"算力", `renderTeam`→"團隊", `renderTech`→"科技" in their files. (Real bodies land in later tasks.)

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/tui -run 'TestTabCyclesPages|TestNumberKeyJumpsPage|TestViewHasChrome|TestProgressBar' -v` → PASS
Run: `go test ./...` → all green.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/tui.go internal/tui/page_*.go internal/tui/nav_test.go
git commit -m "feat(tui): six-page navigation scaffold + persistent chrome"
```

---

## Task 4: Overview page

**Files:**
- Modify: `internal/tui/page_overview.go`
- Test: `internal/tui/page_overview_test.go`

**Interfaces:**
- Consumes: `sim.Valuation`, `sim.TotalUsers`, `sim.MonthlyRevenue`, `sim.MarketRank`, `sim.NextMilestone`, `progressBar`, `human`.

- [ ] **Step 1: Write failing test**

```go
package tui

import (
	"strings"
	"testing"

	"tokensmith/internal/model"
)

func TestOverviewShowsKPIsAndTraining(t *testing.T) {
	m := testModel(t)
	m.state.HasTraining = true
	m.state.Training = model.TrainingJob{Gen: 4, WorkRemaining: 500}
	m.state.Models = []model.Model{{Online: true, Users: 1000, Price: 12}}
	m.page = PageOverview
	v := renderOverview(m)
	for _, want := range []string{"估值", "總用戶", "月營收", "排名", "進行中訓練", "Gen4", "里程碑"} {
		if !strings.Contains(v, want) {
			t.Errorf("overview missing %q:\n%s", want, v)
		}
	}
}

func TestOverviewNoTrainingHint(t *testing.T) {
	m := testModel(t)
	m.state.HasTraining = false
	if !strings.Contains(renderOverview(m), "無進行中訓練") {
		t.Errorf("expected idle-training hint")
	}
}
```

- [ ] **Step 2: Run to verify fail** — `go test ./internal/tui -run TestOverview -v` → FAIL.

- [ ] **Step 3: Implement `renderOverview`**

```go
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

func renderOverview(m Model) string {
	s := m.state
	rank, total := sim.MarketRank(s, m.cfg, model.SegConsumer)
	company := boxStyle.Render(fmt.Sprintf(
		"公司\n估值   $%s\n總用戶 %s\n排名   #%d / %d\n月營收 $%s",
		human(sim.Valuation(s, m.cfg)), human(sim.TotalUsers(s)),
		rank, total, human(sim.MonthlyRevenue(s))))

	var training string
	if s.HasTraining {
		total := m.cfg.GenTrainWorkGPUSec[s.Training.Gen]
		done := 1.0
		if total > 0 {
			done = 1 - s.Training.WorkRemaining/total
		}
		training = boxStyle.Render(fmt.Sprintf("進行中訓練\nGen%d  %s %.0f%%\n區隔 %s",
			s.Training.Gen, progressBar(done, 12), done*100, segmentName(s.Training.Segment)))
	} else {
		training = boxStyle.Render("進行中訓練\n無進行中訓練（到模型頁按 t 開訓）")
	}

	var milestone string
	if target, prog, ok := sim.NextMilestone(s, m.cfg); ok {
		milestone = boxStyle.Render(fmt.Sprintf("下個里程碑\n估值 $%s  %s %.0f%%",
			human(target), progressBar(prog, 10), prog*100))
	} else {
		milestone = boxStyle.Render("下個里程碑\n全部達成")
	}

	help := helpStyle.Render("[Tab]切頁 [q]離開")
	return lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Top, company, "  ", training),
		milestone, help)
}

func segmentName(seg model.Segment) string {
	switch seg {
	case model.SegEnterprise:
		return "企業"
	case model.SegDeveloper:
		return "開發者"
	default:
		return "消費者"
	}
}
```

- [ ] **Step 4: Run to verify pass** — `go test ./internal/tui -run TestOverview -v` → PASS; `go test ./...` green.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/page_overview.go internal/tui/page_overview_test.go
git commit -m "feat(tui): overview page KPIs, training progress, milestone"
```

---

## Task 5: Models page + training dialog (core interaction)

This is the signature interaction (§11.3). Split into 5a (models list + open dialog) and 5b (dialog editing + confirm).

**Files:**
- Modify: `internal/tui/page_models.go`, `internal/tui/tui.go` (dialog state + routing)
- Create: `internal/tui/dialog_train.go`
- Test: `internal/tui/page_models_test.go`, `internal/tui/dialog_train_test.go`

**Interfaces:**
- Produces:
  - `Model.dialog *trainDialog` (nil = no modal open).
  - `type trainDialog struct { gen int; segment model.Segment; alloc [model.NumQualityDims]float64; dim int }`.
  - `func newTrainDialog(m Model) trainDialog` — seeded Gen1, SegConsumer, even alloc `{0.4,0.2,0.2,0.2}`, dim 0.
  - `func (d trainDialog) update(msg tea.KeyMsg) (trainDialog, bool, bool)` — returns (next, confirm, cancel).
  - `func (d trainDialog) command(b balance.Config) model.StartTraining`.
  - `func renderTrainDialog(d trainDialog, m Model) string`.

### Task 5a: models list + open dialog

- [ ] **Step 1: Failing test** (`page_models_test.go`)

```go
package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"tokensmith/internal/model"
)

func TestModelsPageListsModels(t *testing.T) {
	m := testModel(t)
	m.state.Models = []model.Model{{Gen: 2, Segment: model.SegConsumer, Online: true, Users: 500, Price: 12}}
	m.page = PageModels
	v := renderModels(m)
	if !strings.Contains(v, "Gen2") || !strings.Contains(v, "消費者") {
		t.Fatalf("models list missing entries:\n%s", v)
	}
}

func TestTKeyOpensTrainDialog(t *testing.T) {
	m := testModel(t)
	m.page = PageModels
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if nm.(Model).dialog == nil {
		t.Fatalf("t should open the training dialog on models page")
	}
}
```

- [ ] **Step 2: Verify fail** — `go test ./internal/tui -run 'TestModelsPageListsModels|TestTKeyOpensTrainDialog' -v` → FAIL.

- [ ] **Step 3: Implement**

`internal/tui/dialog_train.go` (state + constructor + render; editing lands in 5b):

```go
package tui

import (
	"fmt"
	"strings"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

type trainDialog struct {
	gen     int
	segment model.Segment
	alloc   [model.NumQualityDims]float64
	dim     int
}

func newTrainDialog(m Model) trainDialog {
	return trainDialog{gen: 1, segment: model.SegConsumer, alloc: [model.NumQualityDims]float64{0.4, 0.2, 0.2, 0.2}}
}

func (d trainDialog) command(b balance.Config) model.StartTraining {
	return model.StartTraining{Gen: d.gen, Segment: d.segment, Alloc: d.alloc, Price: b.SegmentRefPrice[d.segment]}
}

var dimNames = [model.NumQualityDims]string{"能力", "成本", "安全", "速度"}

func renderTrainDialog(d trainDialog, m Model) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("訓練新模型\n世代 ‹ Gen%d ›   主打區隔 ‹ %s ›\n\n預算分配（可用 R&D %s）\n",
		d.gen, segmentName(d.segment), human(m.state.Resources.RnD)))
	for i := 0; i < model.NumQualityDims; i++ {
		cursor := " "
		if i == d.dim {
			cursor = "▸"
		}
		est := d.alloc[i] * m.cfg.GenQualityCap[d.gen]
		b.WriteString(fmt.Sprintf("%s %s %s %.0f%%  → 預估 %.0f\n",
			cursor, dimNames[i], progressBar(d.alloc[i], 10), d.alloc[i]*100, est))
	}
	b.WriteString(fmt.Sprintf("\n成本 %s R&D + %.0f GPU\n", human(m.cfg.GenRnDCost[d.gen]), m.cfg.GenTrainWorkGPUSec[d.gen]))
	b.WriteString(helpStyle.Render("[←→]世代 [Tab]區隔 [↑↓]維度 [+/-]分配 [Enter]開訓 [Esc]取消"))
	return boxStyle.Render(b.String())
}
```

`internal/tui/page_models.go`:

```go
package tui

import (
	"fmt"
	"strings"

	"tokensmith/internal/model"
)

func renderModels(m Model) string {
	var b strings.Builder
	b.WriteString("營運中模型\n")
	if len(m.state.Models) == 0 {
		b.WriteString("  （無 — 按 t 訓練第一個模型）\n")
	}
	for _, md := range m.state.Models {
		b.WriteString(fmt.Sprintf("  Gen%d · %s · 用戶 %s · 價 $%.0f · 能力 %.0f\n",
			md.Gen, segmentName(md.Segment), human(md.Users), md.Price, md.Quality[model.DimCapability]))
	}
	b.WriteString(helpStyle.Render("\n[t]訓練新模型 [Tab]切頁"))
	return b.String()
}
```

In `tui.go` add `dialog *trainDialog` to `Model`. In `Update`, when a dialog is open route keys to it FIRST (implemented in 5b); for now open on `t` when `m.page == PageModels`. Restructure the `tea.KeyMsg` case:

```go
	case tea.KeyMsg:
		if m.dialog != nil {
			return m.updateDialog(msg) // implemented in 5b
		}
		switch msg.String() {
		// … nav keys …
		case "q", "ctrl+c":
			_ = store.Save(m.savePath, m.state)
			return m, tea.Quit
		case "t":
			if m.page == PageModels || m.page == PageOverview {
				d := newTrainDialog(m)
				m.dialog = &d
			}
			return m, nil
		}
```

Add a placeholder `updateDialog` (real body in 5b) so it compiles:

```go
func (m Model) updateDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		m.dialog = nil
	}
	return m, nil
}
```

Show the dialog as an overlay: in `View`, when `m.dialog != nil`, render it below the page (simple stacked overlay is fine for v1):

```go
	page := m.renderPage()
	if m.dialog != nil {
		page = renderTrainDialog(*m.dialog, m)
	}
```
(Use `page` in the JoinVertical instead of `m.renderPage()`.)

- [ ] **Step 4: Verify pass** — targeted tests + `go test ./...` green.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/dialog_train.go internal/tui/page_models.go internal/tui/tui.go internal/tui/page_models_test.go
git commit -m "feat(tui): models page + open training dialog"
```

### Task 5b: dialog editing + confirm

- [ ] **Step 1: Failing test** (`dialog_train_test.go`)

```go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func key(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestDialogAdjustAndConfirm(t *testing.T) {
	m := testModel(t)
	m.state.Resources.RnD = 50000
	m.state.Compute.TrainingCapacity = 4
	d := newTrainDialog(m)
	// move to 'safety' dim (index 2) and bump it
	d, _, _ = d.update(tea.KeyMsg{Type: tea.KeyDown})
	d, _, _ = d.update(tea.KeyMsg{Type: tea.KeyDown})
	before := d.alloc[2]
	d, _, _ = d.update(key("+"))
	if d.alloc[2] <= before {
		t.Fatalf("+ did not raise alloc: %v→%v", before, d.alloc[2])
	}
	// segment cycle
	d, _, _ = d.update(tea.KeyMsg{Type: tea.KeyTab})
	if d.segment == model.SegConsumer {
		t.Fatalf("Tab should cycle segment")
	}
	// Enter confirms
	_, confirm, _ := d.update(tea.KeyMsg{Type: tea.KeyEnter})
	if !confirm {
		t.Fatalf("Enter should confirm")
	}
	cmd := d.command(balance.Default())
	if cmd.Gen != d.gen || cmd.Segment != d.segment {
		t.Fatalf("command mismatch: %+v", cmd)
	}
}

func TestDialogConfirmStartsTraining(t *testing.T) {
	m := testModel(t)
	m.state.Resources.RnD = 50000
	m.state.Compute.TrainingCapacity = 4
	d := newTrainDialog(m)
	m.dialog = &d
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := nm.(Model)
	if got.dialog != nil {
		t.Fatalf("dialog should close after confirm")
	}
	if !got.state.HasTraining {
		t.Fatalf("confirm should start training")
	}
}
```

- [ ] **Step 2: Verify fail** → FAIL.

- [ ] **Step 3: Implement**

`trainDialog.update` in `dialog_train.go`:

```go
const allocStep = 0.05

func (d trainDialog) update(msg tea.KeyMsg) (trainDialog, bool, bool) {
	switch msg.String() {
	case "esc":
		return d, false, true
	case "enter":
		return d, true, false
	case "up":
		d.dim = (d.dim + model.NumQualityDims - 1) % model.NumQualityDims
	case "down":
		d.dim = (d.dim + 1) % model.NumQualityDims
	case "left":
		if d.gen > 1 {
			d.gen--
		}
	case "right":
		if d.gen < balance.MaxGen {
			d.gen++
		}
	case "tab":
		d.segment = (d.segment + 1) % model.NumSegments
	case "+", "=":
		d.alloc[d.dim] += allocStep
		d.normalize()
	case "-", "_":
		d.alloc[d.dim] -= allocStep
		if d.alloc[d.dim] < 0 {
			d.alloc[d.dim] = 0
		}
		d.normalize()
	}
	return d, false, false
}

// normalize rescales alloc back to sum 1 (StartTraining requires sum≈1).
func (d *trainDialog) normalize() {
	var sum float64
	for _, a := range d.alloc {
		if a < 0 {
			a = 0
		}
		sum += a
	}
	if sum == 0 {
		for i := range d.alloc {
			d.alloc[i] = 1.0 / model.NumQualityDims
		}
		return
	}
	for i := range d.alloc {
		d.alloc[i] /= sum
	}
}
```

Add `tea` and keep `balance` imports in `dialog_train.go`.

Real `updateDialog` in `tui.go`:

```go
func (m Model) updateDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	d, confirm, cancel := m.dialog.update(msg)
	if cancel {
		m.dialog = nil
		return m, nil
	}
	if confirm {
		if ns, err := sim.Apply(m.state, d.command(m.cfg), m.cfg); err == nil {
			m.state = ns
		}
		m.dialog = nil
		return m, nil
	}
	m.dialog = &d
	return m, nil
}
```

- [ ] **Step 4: Verify pass** — targeted + `go test ./...` green.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/dialog_train.go internal/tui/tui.go internal/tui/dialog_train_test.go
git commit -m "feat(tui): training dialog editing, normalize, confirm→StartTraining"
```

---

## Task 6: Compute page

**Files:** Modify `internal/tui/page_compute.go`; Test `internal/tui/page_compute_test.go`.

**Interfaces:** Consumes `sim.EffectiveTraining/EffectiveInference`, `progressBar`. Keys on `PageCompute`: `r`/`R` rent/unrent training (`RentTrainingCompute{±1}`), `i`/`I` inference (`RentInferenceCompute{±1}`), `b` build server (`BuildServer` first chip), `e` expand datacenter (`ExpandDatacenter{PowerDelta:100,SlotDelta:5}`).

- [ ] **Step 1: Failing test**

```go
package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestComputePageShowsPools(t *testing.T) {
	m := testModel(t)
	m.page = PageCompute
	v := renderCompute(m)
	for _, w := range []string{"訓練", "推理", "機房", "晶片"} {
		if !strings.Contains(v, w) {
			t.Errorf("compute page missing %q:\n%s", w, v)
		}
	}
}

func TestComputeRentKeys(t *testing.T) {
	m := testModel(t)
	m.page = PageCompute
	before := m.state.Compute.TrainingCapacity
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if nm.(Model).state.Compute.TrainingCapacity != before+1 {
		t.Fatalf("r should add training capacity")
	}
}
```

- [ ] **Step 2: Verify fail** → FAIL.

- [ ] **Step 3: Implement** `renderCompute` (pools with util bars, rent/build ratio, datacenter power/space, chip list) and add a `PageCompute` key block in `Update`'s action switch:

```go
		case "r":
			if m.page == PageCompute {
				m.state = applyOK(m.state, model.RentTrainingCompute{Delta: 1}, m.cfg)
			}
			return m, nil
		case "R":
			if m.page == PageCompute {
				m.state = applyOK(m.state, model.RentTrainingCompute{Delta: -1}, m.cfg)
			}
			return m, nil
		case "i":
			if m.page == PageCompute {
				m.state = applyOK(m.state, model.RentInferenceCompute{Delta: 1}, m.cfg)
			}
			return m, nil
		case "I":
			if m.page == PageCompute {
				m.state = applyOK(m.state, model.RentInferenceCompute{Delta: -1}, m.cfg)
			}
			return m, nil
		case "b":
			if m.page == PageCompute && len(m.cfg.Chips) > 0 {
				m.state = applyOK(m.state, model.BuildServer{ChipName: m.cfg.Chips[0].Name}, m.cfg)
			}
			return m, nil
		case "e":
			if m.page == PageCompute {
				m.state = applyOK(m.state, model.ExpandDatacenter{PowerDelta: 100, SlotDelta: 5}, m.cfg)
			}
			return m, nil
```

Add a small helper in `tui.go` used by all pages (keeps failed commands a no-op):

```go
func applyOK(s model.GameState, cmd model.Command, b balance.Config) model.GameState {
	if ns, err := sim.Apply(s, cmd, b); err == nil {
		return ns
	}
	return s
}
```
Remove the now-duplicated prototype `r`/`i` handlers (their logic moved to `PageCompute`).

- [ ] **Step 4: Verify pass** — targeted + `go test ./...` green.

- [ ] **Step 5: Commit** — `feat(tui): compute page (pools, rent/build, datacenter, chips)`.

---

## Task 7: Team page

**Files:** Modify `internal/tui/page_team.go`; Test `internal/tui/page_team_test.go`.

**Interfaces:** Renders four roles (counts + salary), star roster (hired/available). Keys on `PageTeam`: `1`-style conflict avoided — use letters: `h` hire T1 researcher (`HireStaff{RoleResearcher,Tier1,1}`), `e` hire engineer, `o` hire ops, `k` hire marketing, `s` sign first unhired star (`SignStar`). (Number keys stay page-jumps; team uses letters.)

- [ ] **Step 1: Failing test**

```go
package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"tokensmith/internal/model"
)

func TestTeamPageShowsRoles(t *testing.T) {
	m := testModel(t)
	m.page = PageTeam
	v := renderTeam(m)
	for _, w := range []string{"研究員", "工程", "營運", "行銷", "明星"} {
		if !strings.Contains(v, w) {
			t.Errorf("team page missing %q:\n%s", w, v)
		}
	}
}

func TestTeamHireResearcher(t *testing.T) {
	m := testModel(t)
	m.page = PageTeam
	m.state.Resources.Cash = 1e6
	before := m.state.Research.Researchers[model.Tier1]
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if nm.(Model).state.Research.Researchers[model.Tier1] != before+1 {
		t.Fatalf("h should hire a T1 researcher")
	}
}
```

- [ ] **Step 2: Verify fail** → FAIL.

- [ ] **Step 3: Implement** `renderTeam` + `PageTeam` key block (mirroring Task 6 with `applyOK`).

- [ ] **Step 4: Verify pass** — targeted + `go test ./...` green.

- [ ] **Step 5: Commit** — `feat(tui): team page (roles, stars, hire/fire/sign)`.

---

## Task 8: Tech page + Market page

**Files:** Modify `internal/tui/page_tech.go`, `internal/tui/page_market.go`; Tests `page_tech_test.go`, `page_market_test.go`.

**Interfaces:**
- Tech: list `cfg.TechNodes` grouped by branch with cost/locked/unlocked state; `↑↓` select a node (`Model.techCursor int`), `Enter` unlock (`UnlockTech`). Show R&D and branch bonus totals.
- Market: TAM per segment (from `SegmentTargetScale`), three-segment share ranking via `sim.MarketRank`, competitor profiles (name + capability + growth emphasis).

- [ ] **Step 1: Failing tests**

```go
// page_tech_test.go
func TestTechPageListsNodesAndUnlocks(t *testing.T) {
	m := testModel(t)
	m.page = PageTech
	m.state.Resources.RnD = 1e9
	v := renderTech(m)
	if len(m.cfg.TechNodes) > 0 && !strings.Contains(v, m.cfg.TechNodes[0].ID) {
		t.Fatalf("tech page should list node ids:\n%s", v)
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // unlock node under cursor
	if len(nm.(Model).state.UnlockedTech) == 0 {
		t.Fatalf("Enter should unlock the selected tech node")
	}
}

// page_market_test.go
func TestMarketPageShowsSegmentsAndRivals(t *testing.T) {
	m := testModel(t)
	m.page = PageMarket
	v := renderMarket(m)
	for _, w := range []string{"消費者", "企業", "開發者", "對手"} {
		if !strings.Contains(v, w) {
			t.Errorf("market page missing %q:\n%s", w, v)
		}
	}
}
```

- [ ] **Step 2: Verify fail** → FAIL.

- [ ] **Step 3: Implement** both render funcs; add `techCursor` to `Model` and a `PageTech` `up`/`down`/`enter` block in `Update` (guard the `enter`/nav so they only act on `PageTech` when no dialog is open). Market page is read-only.

- [ ] **Step 4: Verify pass** — targeted + `go test ./...` green.

- [ ] **Step 5: Commit** — `feat(tui): tech tree page (unlock) + market page (segments, rivals)`.

---

## Task 9: Pressure indicators, streak, prestige action

**Files:** Modify `internal/tui/tui.go` (resource bar + prestige key), `internal/tui/page_overview.go` (⚠ list); Test `internal/tui/pressure_test.go`.

**Interfaces:**
- `func pressures(m Model) []string` — returns ⚠ strings for: inference util ≥ 0.9; a star hired but cash < 0 trend (skip if complex — keep inference + "no online model" + "training idle with spare compute"). Shown on overview.
- Streak: display `s.GameTime` days in the resource bar (`Day N`); a real coding-streak counter is out of scope for v1 (note it).
- Prestige: on `PageOverview`/`PageTech`, key `P` issues `PrestigeReset` via `applyOK` (no-op when locked).

- [ ] **Step 1: Failing test**

```go
package tui

import (
	"strings"
	"testing"
)

func TestInferencePressureShown(t *testing.T) {
	m := testModel(t)
	m.state.Models = []model.Model{{Online: true, Users: 1e6, Price: 12}}
	m.state.Compute.InferenceCapacity = 1 // tiny → overloaded
	m.state.Compute.InferenceLoad = 100
	if !strings.Contains(strings.Join(pressures(m), "\n"), "推理") {
		t.Fatalf("expected inference pressure warning")
	}
}

func TestResourceBarShowsDay(t *testing.T) {
	m := testModel(t)
	m.state.GameTime = 3 * 86400
	if !strings.Contains(renderResourceBar(m), "Day 3") {
		t.Fatalf("resource bar should show Day 3")
	}
}
```

- [ ] **Step 2: Verify fail** → FAIL.

- [ ] **Step 3: Implement** `pressures`, add `Day N` to `renderResourceBar` (`int(s.GameTime/86400)`), render `⚠` list in overview when non-empty, add `P` prestige key. Note in code comment that coding-streak is deferred.

- [ ] **Step 4: Verify pass** — targeted + `go test ./...` green.

- [ ] **Step 5: Commit** — `feat(tui): pressure warnings, day counter, prestige action`.

---

## Self-Review

- **Spec coverage (§11.2):** 總覽=Task 4, 模型+訓練=Task 5, 市場=Task 8, 算力=Task 6, 團隊=Task 7, 科技=Task 8; resource bar/tab chrome=Task 3; pressure/streak/prestige=Task 9. Events池 (§8.5/§17.2) is a separate future plan — noted, not silently dropped.
- **Type consistency:** `render<Page>(m Model) string` used identically in stubs (Task 3) and bodies (Tasks 4/6/7/8); `applyOK(s, cmd, b)` signature fixed in Task 6 and reused in 7/8/9; `trainDialog.update` returns `(trainDialog, confirm, cancel)` consistently in 5a/5b; `Page`/`numPages`/`pageNames` defined once in Task 3.
- **Non-disruptive:** `StartTraining.Segment` zero value = SegConsumer (Task 2) → existing sim tests unaffected; nav keys added before action keys; prototype `t/r/i` behaviour is migrated (Task 5/6), not left duplicated.
- **Placeholders:** none — dialog editing, normalize, and every key handler carry real code.
- **Deferred (logged, not hidden):** industry-events engine, real coding-streak counter, star挖角/加薪 dialogs (basic sign only), daemon/IPC thin-client split.

## Execution Handoff

Plan saved to `docs/superpowers/plans/2026-07-08-tokensmith-15-six-page-tui.md`. Two execution options:

1. **Subagent-Driven (recommended)** — fresh subagent per task + review between tasks.
2. **Inline Execution** — execute here with checkpoints.
