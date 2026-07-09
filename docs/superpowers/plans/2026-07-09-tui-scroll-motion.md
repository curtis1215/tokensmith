# TUI Content Scroll + Restrained Motion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make long TUI pages (especially market) scrollable inside a fixed shell, and add restrained display-state lerp/pulse so cash/users/bars animate smoothly without changing sim economy.

**Architecture:** Embed `bubbles/viewport` for the content region; track terminal `height` and recompute viewport size after chrome. Keep `sim.GameState` as truth; maintain parallel `displayState` fields on the TUI model, advanced each tick with exponential approach (α≈0.3) plus short token/notice pulse counters. Scroll keys respect list-cursor pages (PgUp/Dn only) vs browse pages (↑↓ scrolls).

**Tech Stack:** Go 1.22+, Bubble Tea, Lipgloss, **bubbles/viewport**. Spec: `docs/superpowers/specs/2026-07-09-tui-scroll-motion-design.md`.

## Global Constraints

- Do **not** change sim tick formulas, balance, or Apply commands.
- Key semantics for game actions (`t/p/$/r/i/…`) unchanged.
- List pages (Models, Tech, Compute): `↑↓` = selection; scroll via `PgUp`/`PgDn`/`ctrl+u`/`ctrl+d`.
- Browse pages (Overview, Market, Team): `↑↓`/`j`/`k` may scroll viewport when no dialog.
- Dialog open: no scroll handling.
- Page change: `vp.GotoTop()`.
- Snap display on NewGame/Restart/prestige/load.
- TDD for lerp helpers and scroll key routing where practical.
- Commit per task; `go test ./...` green.

---

## File Structure

| File | Role |
|---|---|
| `go.mod` / `go.sum` | Add `github.com/charmbracelet/bubbles` |
| `internal/tui/tui.go` | height, viewport, WindowSize, View shell, tick→lerp, scroll keys |
| `internal/tui/display.go` | displayState, lerp, snap, pulse |
| `internal/tui/display_test.go` | unit tests |
| `internal/tui/viewport.go` (optional) | chrome measure, SetContent helpers |
| Page files | unchanged structure; content only via render* |
| Tests | scroll routing, window size |

---

### Task 1: Add bubbles dependency

**Files:** `go.mod`, `go.sum`

- [ ] **Step 1:**
```bash
cd /path/to/tokensmith
go get github.com/charmbracelet/bubbles/viewport@latest
go mod tidy
```

- [ ] **Step 2:** Confirm import builds:
```bash
go build ./internal/tui/
```

- [ ] **Step 3: Commit**
```bash
git add go.mod go.sum
git commit -m "chore: add bubbles/viewport for TUI content scroll"
```

---

### Task 2: displayState lerp + pulse (no viewport yet)

**Files:**
- Create: `internal/tui/display.go`, `display_test.go`
- Modify: `internal/tui/tui.go` (fields + tick call + snap on restart)

**Interfaces:**
```go
type displayState struct {
	Cash, RnD, Valuation float64
	TotalUsers           float64
	TrainUtil, InfUtil   float64
	PulseToken           int
	PulseNotice          int
}

func lerp(a, b, α float64) float64
func (d *displayState) approach(truth displayState, α float64)
func (d *displayState) snap(truth displayState)
func truthDisplay(m Model) displayState // from m.state + m.cfg
```

- [ ] **Step 1: Tests**
```go
func TestLerpApproaches(t *testing.T) {
	x := 0.0
	for i := 0; i < 30; i++ {
		x = lerp(x, 100, 0.3)
	}
	if x < 99 {
		t.Fatalf("x=%v want ~100", x)
	}
}

func TestDisplaySnap(t *testing.T) {
	var d displayState
	d.snap(displayState{Cash: 50})
	if d.Cash != 50 {
		t.Fatal(d.Cash)
	}
}
```

- [ ] **Step 2: Implement display.go**

α default `0.3`. Epsilon snap e.g. `1e-6` relative or absolute for money.

- [ ] **Step 3: Wire Model**
```go
// on Model:
disp displayState
dispReady bool // false until first snap

// on tick after sim.Tick:
truth := truthDisplay(m)
if !m.dispReady {
	m.disp.snap(truth)
	m.dispReady = true
} else {
	m.disp.approach(truth, 0.3)
}
if m.lastTokens > 0 {
	m.disp.PulseToken = 4
} else if m.disp.PulseToken > 0 {
	m.disp.PulseToken--
}
// when setting m.notice from events, set PulseNotice = 4
```

On `sim.Restart` / prestige / New: `m.dispReady = false` or immediate snap.

- [ ] **Step 4: Resource bar uses disp**
`renderResourceBar` reads `m.disp.Cash` etc.; if `PulseToken > 0` style R&D segment with accent.

- [ ] **Step 5: Green + commit**
```bash
go test ./internal/tui/ -count=1
git commit -am "feat(tui): displayState lerp and token pulse for resource bar"
```

---

### Task 3: Viewport shell + height

**Files:**
- Modify: `internal/tui/tui.go` (`View`, `WindowSizeMsg`, Init)
- Optional: `internal/tui/viewport_chrome.go`

**Interfaces:**
```go
// Model fields:
height int
vp     viewport.Model

func (m *Model) resize(w, h int)
func (m *Model) refreshViewport()
func (m Model) chromeRows() int // estimate fixed shell lines
```

- [ ] **Step 1: Init viewport**
```go
vp := viewport.New(80, 20)
// Model: width:100, height:40, vp:vp
```

- [ ] **Step 2: resize**
```go
func (m *Model) resize(w, h int) {
	if w < 1 { w = 1 }
	if h < 1 { h = 1 }
	m.width, m.height = w, h
	ch := h - m.chromeRows()
	if ch < 3 { ch = 3 }
	// content width: leave margin for outer box (~2-4)
	cw := w - 4
	if cw < 20 { cw = 20 }
	m.vp.Width = cw
	m.vp.Height = ch
}
```

`chromeRows`: start with constant ~10–12; refine if clipping. Include notice/pressures when present (dynamic).

- [ ] **Step 3: refreshViewport**
```go
func (m *Model) refreshViewport() {
	var body string
	if m.publish != nil {
		body = renderPublishDialog(*m.publish, m)
	} else if m.dialog != nil {
		body = renderTrainDialog(*m.dialog, m)
	} else {
		body = m.renderPage()
	}
	m.vp.SetContent(body)
}
```

Call after tick, after key handling that changes page/state, and after resize.

- [ ] **Step 4: Rewrite View**
```go
func (m Model) View() string {
	// Do not call refreshViewport if it mutates — either use pointer receiver
	// pattern: Update always refreshViewport before View, View is pure.
	top := VStack(header, notice?, resourceBar, tabBar)
	mid := m.vp.View()
	bot := VStack(pressures?, /* footer is inside page currently */)
	// Prefer: pages stop embedding Footer; shell renders Footer(pageKeys(m))
	return boxStyle.Render(VStack(top, mid, bot))
}
```

**Footer ownership:** Move page-specific help into `pageKeys(m Page) string` on shell so footer stays fixed outside viewport. Update each `render*` to **not** append `Footer(...)` (breaking change for page tests — update Contains still OK if keys appear in shell).

- [ ] **Step 5: WindowSizeMsg**
```go
case tea.WindowSizeMsg:
	m.resize(msg.Width, msg.Height)
	m.refreshViewport()
	return m, nil
```

- [ ] **Step 6: Tests** — View still contains Tokensmith; after setting tall content and YOffset, structure ok.

- [ ] **Step 7: Commit**
```bash
git commit -am "feat(tui): content viewport with fixed shell chrome"
```

---

### Task 4: Scroll key routing

**Files:** `internal/tui/tui.go`, tests

- [ ] **Step 1: Helper**
```go
func (m Model) pageUsesListCursor() bool {
	switch m.page {
	case PageModels, PageTech, PageCompute:
		return true
	default:
		return false
	}
}
```

- [ ] **Step 2: In KeyMsg before page-specific handlers (and only if no dialog):**
```go
if m.publish == nil && m.dialog == nil {
	switch msg.String() {
	case "pgdown", "ctrl+d":
		m.vp.HalfViewDown() // or ViewDown
		return m, nil
	case "pgup", "ctrl+u":
		m.vp.HalfViewUp()
		return m, nil
	case "j", "down":
		if !m.pageUsesListCursor() {
			m.vp.LineDown(1)
			return m, nil
		}
	case "k", "up":
		if !m.pageUsesListCursor() {
			m.vp.LineUp(1)
			return m, nil
		}
	}
}
// existing up/down for list cursors unchanged
```

- [ ] **Step 3: On tab/number page change:** `m.vp.GotoTop(); m.refreshViewport()`

- [ ] **Step 4: Test** browse page down increases YOffset; models page down changes modelCursor not only scroll (or scroll keys pgdown still work).

- [ ] **Step 5: Commit**
```bash
git commit -am "feat(tui): scroll keys with list-cursor conflict rules"
```

---

### Task 5: Extend motion to users + bars + market/overview

**Files:** `display.go`, `page_overview.go`, `page_models.go`, `page_market.go`, `page_compute.go`, `renderResourceBar`

- [ ] **Step 1:** Extend `truthDisplay` / `displayState` with TotalUsers, InfUtil, TrainUtil; optional map of model users by index (rebuild each truth from state).

- [ ] **Step 2:** Overview total users + share bars use approached values where easy (if share lerp is heavy, lerp only total users + util bars first).

Minimum for task complete:
- Resource bar: cash, rnd, valuation, utils (task 2)
- Overview company card users from `disp.TotalUsers`
- Models detail users from disp if index matches
- Compute util bars from disp

- [ ] **Step 3:** Market share bars — optional lerp of each ShareRow.Share in a `[]float64` parallel array; or skip if time — prefer include simple consumer top shares.

- [ ] **Step 4: Commit**
```bash
git commit -am "feat(tui): smooth users and util bars via displayState"
```

---

### Task 6: Polish + full suite

- [ ] **Step 1:**
```bash
go test ./... -count=1
go vet ./...
go build ./...
```

- [ ] **Step 2: Manual checklist**
  - Market: PgDn reaches 對手檔案
  - Models: ↑↓ select; PgDn scrolls if detail long
  - Tick: cash/users ease; token flash
  - Open train dialog: no accidental scroll
  - Resize terminal: viewport reflows

- [ ] **Step 3: Fix fallout commit if needed**
```bash
git commit -am "fix(tui): scroll and motion integration fallout"
```

- [ ] **Step 4:** Do not release unless user asks.

---

## Spec Coverage

| Spec | Task |
|---|---|
| bubbles/viewport dependency | 1 |
| displayState lerp/pulse | 2, 5 |
| Shell chrome + contentH | 3 |
| Scroll keys + conflicts | 4 |
| Market structure unchanged | — (no page rewrite) |
| Snap on restart | 2 |
| Tests | 2, 4, 6 |

## Self-Review

- No economy changes.
- Footer moved to shell to keep fixed — pages must drop trailing Footer calls.
- Mouse wheel optional / not required for done.
