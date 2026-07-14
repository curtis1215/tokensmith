# HQ Token→R&D Multiplier Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Scale real coding-token → R&D conversion by headquarters `Office.Level` (L1×1.0 … L8×5.0) so mid/late game coding feels faster, and surface the mult on the pulse bar and overview HQ card.

**Architecture:** `balance.Config` gains an `OfficeTokenRnDMult [9]float64` lookup (index = office level) plus `OfficeTokenRnDMultAt`. `sim.Tick` multiplies only the token R&D term by that helper (staff R&D unchanged). TUI composes the same mult into `lastTokenRnD` and shows `總部 ×N` on pulse plus `Token→R&D ×N` on the HQ card. No save schema change.

**Tech Stack:** Go 1.25, `charmbracelet/bubbletea` / lipgloss (TUI), `go test` only.

**Spec:** `docs/superpowers/specs/2026-07-14-hq-token-rnd-mult-design.md`

## Global Constraints

- Module path is `tokensmith` (see `go.mod`); imports are `tokensmith/internal/...`.
- `internal/model` has zero internal dependencies — do not add edges into `model`.
- `internal/balance` must not import `internal/tui` or `internal/sim`.
- `sim.Tick` signature must not change: `Tick(s model.GameState, dt float64, events []model.TokenEvent, b balance.Config) model.GameState`.
- HQ mult multiplies **token R&D only**, never employee/staff R&D.
- `TokenRawRnD` stays raw-only (no HQ mult inside it).
- Default table must match design §5.1 exactly: `1.0, 1.3, 1.7, 2.2, 2.8, 3.5, 4.2, 5.0` for levels 1..8.
- Run `go test` for packages touched at end of each task; full `go test ./...` before final commit if time allows.
- Commit messages: `type(scope): summary` (repo convention).

## File map

| File | Responsibility |
|------|----------------|
| `internal/balance/balance.go` | Add `Config.OfficeTokenRnDMult` field |
| `internal/balance/employee.go` | Seed table in `applyEmployeeDefaults`; implement `OfficeTokenRnDMultAt` |
| `internal/balance/employee_test.go` | Table + helper tests |
| `internal/sim/sim.go` | Multiply token term by HQ mult |
| `internal/sim/sim_test.go` | L8 vs L1 ratio; staff isolation; streak stack |
| `internal/tui/tui.go` | `lastTokenRnD` includes HQ; pulse `總部 ×` suffix |
| `internal/tui/ascii_hq.go` | HQ card `Token→R&D ×N` |
| `internal/tui/display_test.go` / `ascii_hq_test.go` | Pulse + HQ card assertions |

---

### Task 1: Balance table + `OfficeTokenRnDMultAt`

**Files:**
- Modify: `internal/balance/balance.go` (Config struct)
- Modify: `internal/balance/employee.go` (`applyEmployeeDefaults` + helper)
- Modify: `internal/balance/employee_test.go`

**Interfaces:**
- Consumes: existing `Config.MaxOfficeLevel`, office tables in `applyEmployeeDefaults`
- Produces:
  - `Config.OfficeTokenRnDMult [9]float64` (index 0 unused)
  - `func OfficeTokenRnDMultAt(level int, b Config) float64`

- [ ] **Step 1: Write the failing tests**

Append to `internal/balance/employee_test.go`:

```go
func TestOfficeTokenRnDMultTable(t *testing.T) {
	b := Default()
	want := [9]float64{0, 1.0, 1.3, 1.7, 2.2, 2.8, 3.5, 4.2, 5.0}
	if b.OfficeTokenRnDMult != want {
		t.Fatalf("OfficeTokenRnDMult = %v, want %v", b.OfficeTokenRnDMult, want)
	}
}

func TestOfficeTokenRnDMultAt(t *testing.T) {
	b := Default()
	if got := OfficeTokenRnDMultAt(1, b); got != 1.0 {
		t.Fatalf("L1 = %v, want 1.0", got)
	}
	if got := OfficeTokenRnDMultAt(8, b); got != 5.0 {
		t.Fatalf("L8 = %v, want 5.0", got)
	}
	if got := OfficeTokenRnDMultAt(0, b); got != 1.0 {
		t.Fatalf("level 0 → L1 = %v, want 1.0", got)
	}
	if got := OfficeTokenRnDMultAt(-3, b); got != 1.0 {
		t.Fatalf("negative → L1 = %v, want 1.0", got)
	}
	if got := OfficeTokenRnDMultAt(99, b); got != 5.0 {
		t.Fatalf("oversize clamps to L8 = %v, want 5.0", got)
	}
	// Fail-safe: non-positive slot after clamp returns 1.0
	b.OfficeTokenRnDMult[1] = 0
	if got := OfficeTokenRnDMultAt(1, b); got != 1.0 {
		t.Fatalf("non-positive mult fail-safe = %v, want 1.0", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/balance/ -run 'TestOfficeTokenRnDMult' -count=1`

Expected: FAIL — unknown field / undefined `OfficeTokenRnDMultAt`.

- [ ] **Step 3: Implement field, defaults, helper**

In `internal/balance/balance.go`, inside `Config` near the office block (after `OfficeNames` or with other office fields):

```go
// OfficeTokenRnDMult[level] multiplies token-sourced R&D only (never employee
// R&D). Index 0 unused; levels 1..MaxOfficeLevel. See design
// 2026-07-14-hq-token-rnd-mult.
OfficeTokenRnDMult [9]float64
```

In `internal/balance/employee.go`, inside `applyEmployeeDefaults` after `OfficeNames`:

```go
c.OfficeTokenRnDMult = [9]float64{
	0,
	1.0, // L1 車庫
	1.3, // L2 小辦公室
	1.7, // L3 開放式樓層
	2.2, // L4 辦公樓
	2.8, // L5 園區
	3.5, // L6 摩天樓
	4.2, // L7 巨塔
	5.0, // L8 太空電梯
}
```

Add helper in the same file (near `OfficeSeatsAt`):

```go
// OfficeTokenRnDMultAt returns the token→R&D multiplier for an office level.
// Level < 1 is treated as 1; level above MaxOfficeLevel clamps to max.
// Non-positive table entries return 1.0 (neutral fail-safe).
func OfficeTokenRnDMultAt(level int, b Config) float64 {
	if level < 1 {
		level = 1
	}
	max := b.MaxOfficeLevel
	if max < 1 {
		max = 1
	}
	if level > max {
		level = max
	}
	if level < 0 || level >= len(b.OfficeTokenRnDMult) {
		return 1.0
	}
	m := b.OfficeTokenRnDMult[level]
	if m <= 0 || math.IsNaN(m) {
		return 1.0
	}
	return m
}
```

(`math` is already imported in `employee.go`.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/balance/ -run 'TestOfficeTokenRnDMult|TestOfficeTable' -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/balance/balance.go internal/balance/employee.go internal/balance/employee_test.go
git commit -m "feat(balance): office-level token→R&D multiplier table"
```

---

### Task 2: Apply HQ mult in `sim.Tick` (token term only)

**Files:**
- Modify: `internal/sim/sim.go` (`tickWithClocks` R&D accrual line)
- Modify: `internal/sim/sim_test.go`

**Interfaces:**
- Consumes: `balance.OfficeTokenRnDMultAt`, `effectiveOfficeLevel` (already in `internal/sim/employee.go`)
- Produces: token R&D booking scaled by office level; staff path unchanged

- [ ] **Step 1: Write the failing tests**

Append to `internal/sim/sim_test.go`:

```go
func TestTickOfficeTokenRnDMultScalesTokenOnly(t *testing.T) {
	b := balance.Default()
	events := []model.TokenEvent{{OutputTokens: 1000}} // raw = 2000 with default weights/divisor

	l1 := model.GameState{Office: model.Office{Level: 1}}
	l8 := model.GameState{Office: model.Office{Level: 8}}
	ns1 := Tick(l1, 1, events, b)
	ns8 := Tick(l8, 1, events, b)
	if !approx(ns1.Resources.RnD, 2000) {
		t.Fatalf("L1 token R&D = %v, want 2000", ns1.Resources.RnD)
	}
	if !approx(ns8.Resources.RnD, 2000*5.0) {
		t.Fatalf("L8 token R&D = %v, want %v", ns8.Resources.RnD, 2000*5.0)
	}

	// Staff/employee R&D must not change with office level when no tokens.
	emp := []model.Employee{{
		PrimaryRole: model.RoleResearcher,
		Stats:       [model.NumRoles]int{40, 0, 0, 0},
	}}
	noTok1 := Tick(model.GameState{
		Office:    model.Office{Level: 1},
		Employees: emp,
		Research:  model.Research{EfficiencyMult: 1.0},
		Market:    model.TalentMarket{NextRefreshAt: 1e12, RandState: 1},
	}, 10, nil, b)
	noTok8 := Tick(model.GameState{
		Office:    model.Office{Level: 8},
		Employees: emp,
		Research:  model.Research{EfficiencyMult: 1.0},
		Market:    model.TalentMarket{NextRefreshAt: 1e12, RandState: 1},
	}, 10, nil, b)
	if noTok1.Resources.RnD <= 0 {
		t.Fatal("expected positive staff R&D with researcher")
	}
	if !approx(noTok1.Resources.RnD, noTok8.Resources.RnD) {
		t.Fatalf("office level must not affect staff R&D: L1=%v L8=%v",
			noTok1.Resources.RnD, noTok8.Resources.RnD)
	}
}

func TestTickOfficeAndStreakStackOnToken(t *testing.T) {
	b := balance.Default()
	b.StreakMult = 2.0
	events := []model.TokenEvent{{OutputTokens: 1000}} // raw 2000
	s := model.GameState{Office: model.Office{Level: 4}} // mult 2.2
	ns := Tick(s, 1, events, b)
	want := 2000 * 2.0 * 2.2 // streak × HQ
	if !approx(ns.Resources.RnD, want) {
		t.Fatalf("RnD = %v, want %v (streak×HQ)", ns.Resources.RnD, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/sim/ -run 'TestTickOffice' -count=1`

Expected: FAIL — L8 still books 2000 (no HQ mult yet).

- [ ] **Step 3: Multiply token term in `tickWithClocks`**

In `internal/sim/sim.go`, replace the R&D accrual block (~lines 88–92) with:

```go
	staffRnD := staffRnDPerSecFromEmployees(ns, b) * economyDT
	tokenRnD := TokenRawRnD(events, b)

	pe := PrestigeEffects(ns.Prestige.UnlockedPrestige, b)
	hq := balance.OfficeTokenRnDMultAt(effectiveOfficeLevel(ns), b)
	ns.Resources.RnD += staffRnD*pe.RnDMult + tokenRnD*b.StreakMult*pe.RnDMult*sk.TokenRnDMult*hq
```

Do not change any other lines in that function.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/sim/ -run 'TestTickOffice|TestTickAddsToken|TestTickStreak' -count=1`

Expected: PASS (including existing `TestTickAddsTokenRnD` — zero Office.Level floors to L1 via `effectiveOfficeLevel`).

Also run: `go test ./internal/sim/ -count=1`

Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/sim/sim.go internal/sim/sim_test.go
git commit -m "feat(sim): scale token R&D by office level"
```

---

### Task 3: TUI pulse badge + HQ card label

**Files:**
- Modify: `internal/tui/tui.go` (`tickMsg` lastTokenRnD composition; `renderResourceBar` pulse suffix)
- Modify: `internal/tui/ascii_hq.go` (`hqContent`)
- Modify: `internal/tui/display_test.go`
- Modify: `internal/tui/ascii_hq_test.go`

**Interfaces:**
- Consumes: `balance.OfficeTokenRnDMultAt(level, cfg)`
- Produces: pulse amounts include HQ; bar shows `總部 ×N.NN`; HQ card shows `Token→R&D ×N.NN`

- [ ] **Step 1: Write the failing tests**

**A.** In `internal/tui/display_test.go`, update prestige test wants to multiply by HQ (default L1 = 1.0 so numbers stay same if Office.Level is 1). Add an explicit HQ case:

```go
func TestPerSourceRnDDisplayIncludesOfficeMult(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, "ledger.json")
	mp := filepath.Join(dir, "meta.json")
	ledger.Save(lp, ledger.Ledger{
		Sources:   map[string]model.SourceTotals{"claude-code": {In: 1000, Out: 500}},
		UpdatedAt: 9_000_000_000,
	})
	store.SaveMeta(mp, store.Meta{LastRealUnix: 9_000_000_000})

	m := newAtPaths(filepath.Join(dir, "s.json"), lp, mp)
	m.daemonMode = true
	m.state.Office.Level = 6 // mult 3.5

	nm, _ := m.Update(tickMsg(time.Unix(0, 0)))
	got := nm.(Model)

	raw := sim.TokenRawRnD([]model.TokenEvent{{
		Source: "claude-code", InputTokens: 1000, OutputTokens: 500,
	}}, got.cfg)
	pe := sim.PrestigeEffects(got.state.Prestige.UnlockedPrestige, got.cfg)
	hq := balance.OfficeTokenRnDMultAt(got.state.Office.Level, got.cfg)
	want := raw * got.currentStreakMult() * pe.RnDMult * hq
	if math.Abs(got.lastTokenRnD["claude-code"]-want) > 1e-9 {
		t.Fatalf("lastTokenRnD=%v want %v (raw*streak*prestige*hq)",
			got.lastTokenRnD["claude-code"], want)
	}

	got.disp.PulseToken = 5
	bar := renderResourceBar(got)
	if !strings.Contains(bar, "總部 ×3.50") {
		t.Fatalf("expected HQ mult badge in bar, got:\n%s", bar)
	}
	wantSeg := "Claude Code +" + human(want) + " R&D"
	if !strings.Contains(bar, wantSeg) {
		t.Fatalf("expected HQ-scaled segment %q in bar, got:\n%s", wantSeg, bar)
	}
}
```

Ensure imports in that file include `tokensmith/internal/balance` if not already present (check existing imports; add if missing).

Also update `TestPerSourceRnDDisplayIncludesPrestigeMult` comment/want formula to document `* hq` with L1=1:

```go
	hq := balance.OfficeTokenRnDMultAt(got.state.Office.Level, got.cfg)
	want := raw * got.currentStreakMult() * pe.RnDMult * hq
```

(and use `want` in the fatal / bar segment as today). If `Office.Level` is 0 on that model, helper still returns 1.0.

**B.** In `internal/tui/ascii_hq_test.go`, extend `TestRenderHQWideAndNarrow` (or add):

```go
func TestRenderHQShowsTokenRnDMult(t *testing.T) {
	m := newAt(filepath.Join(t.TempDir(), "save.json"))
	m.state.Office.Level = 6 // 3.5
	wide := renderHQ(m, 110)
	if !strings.Contains(wide, "Token→R&D ×3.50") {
		t.Fatalf("wide HQ missing token mult: %q", wide)
	}
	narrow := renderHQ(m, 80)
	if !strings.Contains(narrow, "Token→R&D ×3.50") {
		t.Fatalf("narrow HQ missing token mult: %q", narrow)
	}
	// Stage icons must remain on narrow
	if !strings.Contains(narrow, "🗼") {
		t.Fatalf("narrow HQ dropped stage icons: %q", narrow)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run 'TestPerSourceRnDDisplayIncludesOfficeMult|TestRenderHQShowsTokenRnDMult' -count=1`

Expected: FAIL — no HQ in `lastTokenRnD` / missing strings.

- [ ] **Step 3: Implement TUI composition + render**

**3a.** In `internal/tui/tui.go` tick handler where `lastTokenRnD` is built (~lines 588–594), multiply by HQ:

```go
		if m.tokensThisTick {
			pe := sim.PrestigeEffects(m.state.Prestige.UnlockedPrestige, cfgTick)
			hq := balance.OfficeTokenRnDMultAt(m.state.Office.Level, cfgTick)
			rnd := make(map[string]float64, len(events))
			for _, e := range events {
				rnd[e.Source] += sim.TokenRawRnD([]model.TokenEvent{e}, cfgTick) * cfgTick.StreakMult * pe.RnDMult * hq
			}
			m.lastTokenRnD = rnd
		}
```

Note: use `m.state.Office.Level` (pre-tick state is fine; upgrade commands apply on other messages). `OfficeTokenRnDMultAt` already floors level &lt; 1.

**3b.** In `renderResourceBar`, after building source chips, append HQ mult when pulsing with token deltas:

```go
	if m.disp.PulseToken > 0 && len(m.lastTokenRnD) > 0 {
		parts := make([]string, 0, len(m.lastTokenRnD))
		for _, src := range sourceKeysOrdered(m.lastTokenRnD) {
			chip := fmt.Sprintf(" ⚡%s +%s R&D ", sourceLabel(src), human(m.lastTokenRnD[src]))
			parts = append(parts, lipgloss.NewStyle().Bold(true).Foreground(colorInk).Background(colorCyan).Render(chip))
		}
		hq := balance.OfficeTokenRnDMultAt(m.state.Office.Level, m.cfg)
		hqChip := styleMuted.Render(fmt.Sprintf(" ·  總部 ×%.2f", hq))
		bar += "  " + strings.Join(parts, " ") + hqChip
	}
```

(Replace the existing block that only joins `parts` without the HQ suffix.)

**3c.** In `internal/tui/ascii_hq.go` `hqContent`:

Compact branch — append mult after icons:

```go
	if compact {
		var icons []string
		for i, ic := range hqStageIcons {
			if i == stage {
				icons = append(icons, styleGold.Bold(true).Render(ic+hqStageNames[i]))
			} else {
				icons = append(icons, styleMuted.Render(ic))
			}
		}
		hq := balance.OfficeTokenRnDMultAt(m.state.Office.Level, m.cfg)
		body := strings.Join(icons, styleMuted.Render("→")) +
			styleMuted.Render(fmt.Sprintf(" · Token→R&D ×%.2f", hq))
		return cardContent{
			kind:  CardDefault,
			w:     w,
			title: "總部",
			body:  body,
		}
	}
```

Wide branch — art + status + mult line:

```go
	lit := m.state.HasTraining && m.blink
	art := styleCyan.Render(hqArt(stage, lit))
	status := ""
	if m.state.HasTraining {
		status = styleAmber.Render("  訓練機房運轉中…")
	}
	hq := balance.OfficeTokenRnDMultAt(m.state.Office.Level, m.cfg)
	multLine := styleMuted.Render(fmt.Sprintf("Token→R&D ×%.2f", hq))
	return cardContent{
		kind:  CardDefault,
		w:     w,
		title: fmt.Sprintf("總部 — %s %s", hqStageIcons[stage], hqStageNames[stage]),
		body:  art + status + "\n" + multLine,
	}
```

Add import `tokensmith/internal/balance` to `ascii_hq.go` if not present (`fmt` and `strings` already there).

- [ ] **Step 4: Run tests to verify they pass**

Run:

```bash
go test ./internal/tui/ -run 'TestPerSourceRnDDisplay|TestRenderHQ|TestRenderResourceBar' -count=1
go test ./internal/balance/ ./internal/sim/ ./internal/tui/ -count=1
```

Expected: PASS

- [ ] **Step 5: Full package check + commit**

```bash
go test ./... -count=1
git add internal/tui/tui.go internal/tui/ascii_hq.go internal/tui/display_test.go internal/tui/ascii_hq_test.go
git commit -m "feat(tui): show HQ token→R&D mult on pulse and HQ card"
```

---

## Spec coverage checklist

| Spec section | Task |
|--------------|------|
| §5 formula + table | Task 1 + 2 |
| §5.1 helper clamp / fail-safe | Task 1 |
| §6 sim booking | Task 2 |
| §7.1 pulse `總部 ×` + scaled amount | Task 3 |
| §7.2 HQ card wide/compact | Task 3 |
| §8 edges (L0, oversize, multi-source shared mult) | Task 1 helper + Task 3 bar once | 
| §9 tests | All tasks |
| §10 no schema migration | N/A (no code) |
| Non-goal: skill pulse gap | Not implemented (intentional) |

## Self-review notes

- No TBD/placeholder steps; full code snippets for each implementation step.
- Names consistent: `OfficeTokenRnDMult`, `OfficeTokenRnDMultAt`.
- Default L1 keeps existing `TestTickAddsTokenRnD` and prestige display numbers valid when multiplied by 1.0.
- Pulse still omits skill `TokenRnDMult` per spec non-goals.
