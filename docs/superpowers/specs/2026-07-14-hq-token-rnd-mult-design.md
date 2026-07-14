# HQ Token→R&D Multiplier — Design

- **Date:** 2026-07-14
- **Status:** Approved design (brainstorming); pending user review of this file
- **Scope:** Scale real coding-token → R&D conversion by headquarters (`Office.Level`); show the mult on pulse and overview HQ card
- **Related:**
  - `2026-07-10-token-rnd-rebalance-design.md` (token as primary R&D source; streak / display)
  - `2026-07-13-employee-office-refactor-design.md` (Office levels 1–8, HQ art alignment)
  - `2026-07-07-tokensmith-design.md` §6.2 / §8.1 (token fuel feel)

---

## 1. Problem

Token→R&D conversion is flat across the run (`(in×1 + out×2) / TokenDivisor`, then streak / prestige / skill). Early game (garage / small office) feels acceptable. Mid-to-late game, Gen unlock and frontier R&D costs grow by orders of magnitude while HQ upgrades only buy seats and market weights — **coding does not feel faster when the company scales**. The signature “real usage drives R&D” loop weakens exactly when the player has invested in headquarters.

## 2. Goals

| Goal | Success criterion |
|------|-------------------|
| Mid/late token R&D is noticeably faster | At L6–L8, booked token R&D is about **×3.5–×5** vs L1 for the same raw tokens |
| Early game unchanged | L1 mult is exactly **1.0** |
| HQ upgrade rewards token efficiency | Mult is a pure function of `Office.Level` |
| Visible feedback | Pulse shows `總部 ×N`; HQ card always shows `Token→R&D ×N` |
| Booked == displayed (for HQ) | Pulse `lastTokenRnD` includes the same HQ mult as `sim.Tick` |

## 3. Non-goals

- Changing employee passive R&D, `TokenDivisor`, input/output weights, soft-cap, streak formula, prestige, or skill stacking rules (except inserting HQ on the token product)
- Changing Office upgrade costs, seat caps, or talent-market weights
- Persisting a separate mult field in save / meta (derived from level only)
- Fixing the pre-existing pulse gap where skill `TokenRnDMult` is not shown in the status bar (YAGNI for this change)

## 4. Decisions (brainstorming)

| Topic | Decision |
|-------|----------|
| Pain phase | **B** — mid/late slow, not early |
| Magnitude | **B** — ~×3–×5 at high HQ |
| Curve | **A** — linear table, L1=1.0 → L8=5.0 |
| UI | **B+C** — pulse badge + overview HQ card |
| Architecture | balance lookup table + multiply token term in `sim.Tick` |

## 5. Formula

Existing token booking path (simplified):

```text
raw     = TokenRawRnD(events)   // (in×TokenInputWeight + out×TokenOutputWeight) / TokenDivisor
booked  = raw × StreakMult × PrestigeRnDMult × SkillTokenRnDMult
```

**After this design:**

```text
booked  = raw × StreakMult × PrestigeRnDMult × SkillTokenRnDMult × OfficeTokenRnDMult(level)
```

- `level = effectiveOfficeLevel(state)` — already exists in `sim` (floor at 1).
- HQ mult applies **only** to the token term, never to staff / employee R&D.
- Staff path remains: `staffRnD × PrestigeRnDMult` (unchanged).

### 5.1 Multiplier table

Stored on `balance.Config` as `OfficeTokenRnDMult [9]float64` (index 0 unused; levels 1..`MaxOfficeLevel`).

| Level | Name (`OfficeNames`) | Mult |
|------:|----------------------|-----:|
| 1 | 車庫 | 1.0 |
| 2 | 小辦公室 | 1.3 |
| 3 | 開放式樓層 | 1.7 |
| 4 | 辦公樓 | 2.2 |
| 5 | 園區 | 2.8 |
| 6 | 摩天樓 | 3.5 |
| 7 | 巨塔 | 4.2 |
| 8 | 太空電梯 | 5.0 |

Helper: `balance.OfficeTokenRnDMultAt(level int, b Config) float64`

- If `level < 1`, treat as 1.
- If `level > MaxOfficeLevel` (or beyond table), clamp to `MaxOfficeLevel`.
- If the slot is missing or non-positive after clamp, return **1.0** (fail-safe neutral).

## 6. Architecture

```text
Office.Level (save)
       │
       ▼
balance.OfficeTokenRnDMultAt(level, b)
       │
       ├─► sim.Tick / OfflineTick token term   (authority for cash-in of R&D)
       ├─► TUI lastTokenRnD computation        (pulse amount)
       └─► renderHQ / hqContent                (static label)
```

No new save fields. Offline settlement already calls the same tick path, so offline token batches automatically use the current office level.

### 6.1 Packages

| Package | Change |
|---------|--------|
| `internal/balance` | Field + Default table + `OfficeTokenRnDMultAt` |
| `internal/sim` | Multiply token term by helper; unit tests for isolation from staff R&D |
| `internal/tui` | Include HQ mult in `lastTokenRnD`; pulse suffix; HQ card line |

### 6.2 Why not fold into `StreakMult` / Config injection

Streak is wall-clock and is injected per tick via `Config.StreakMult` because `sim` must not read the wall clock. Office level is already pure game state; `sim` can read it directly (same pattern as skill / prestige effects).

### 6.3 Tick line (conceptual)

```go
hq := balance.OfficeTokenRnDMultAt(effectiveOfficeLevel(ns), b)
ns.Resources.RnD += staffRnD*pe.RnDMult +
    tokenRnD*b.StreakMult*pe.RnDMult*sk.TokenRnDMult*hq
```

## 7. TUI

### 7.1 Pulse / resource bar

When `PulseToken > 0` and there is a per-source delta:

```text
⚡Claude Code +2.9k R&D  ⚡Codex +1.1k R&D  ·  總部 ×3.50
```

Rules:

- Each source chip still shows that source’s R&D (already including HQ mult in the number).
- HQ mult is shown **once** after all source chips (not repeated per source).
- Format mult with two decimal places (aligned with streak `×1.06`).
- Show even at L1 (`總部 ×1.00`) so early game is explicitly neutral.

Display product for pulse amounts (this change):

```text
lastTokenRnD[src] = TokenRawRnD(src) × StreakMult × PrestigeRnDMult × HQMult
```

Skill `TokenRnDMult` remains omitted from pulse (pre-existing); do not expand scope here.

### 7.2 Overview HQ card

| Mode | Presentation |
|------|----------------|
| Wide (ASCII art) | Keep title `總部 — {icon} {name}`; below art, muted line `Token→R&D ×{mult}` |
| Compact (icon strip) | After stage icons, append ` · Token→R&D ×{mult}` (or second line if width forces wrap). Do not drop stage icons. |

Always use `OfficeTokenRnDMultAt` — same number as booking.

## 8. Edge cases

| Case | Behavior |
|------|----------|
| `Office.Level == 0` / unset | Effective L1 → ×1.0 |
| Level above max | Clamp to max table entry |
| Upgrade office mid-session with tokens same tick | New level applies from that tick’s state |
| No token events | No R&D from tokens; HQ card still shows static mult |
| Multiple sources same tick | Per-source raw; shared HQ mult |
| Prestige / restart | Office reset → mult returns to L1 with office |
| Unknown future max office | Helper clamps; Default must fill 1..MaxOfficeLevel |

## 9. Testing

Minimum gates:

1. **balance**
   - Default table matches §5.1
   - `OfficeTokenRnDMultAt(0) == 1.0`, `At(1) == 1.0`, `At(8) == 5.0`, oversize clamps
2. **sim**
   - Same token batch: L8 booked R&D / L1 booked R&D ≈ 5.0
   - Employee/staff R&D unchanged when only level changes
   - Stacking: `StreakMult` and HQ mult both apply to token term
3. **tui**
   - `lastTokenRnD` includes HQ mult
   - Resource bar contains `總部 ×` while pulsing
   - `renderHQ` wide and compact contain `Token→R&D`
4. **Regression**
   - Existing streak/prestige display tests: at default L1, numeric expectations stay valid after multiplying by 1.0; if tests construct higher office levels, update wants

Run `go test ./internal/balance ./internal/sim ./internal/tui` (or `./...`) green before merge.

## 10. Migration / compatibility

- No schema bump required.
- Old saves already have `Office.Level`; mult appears immediately on load.
- Balance-only change: players who already maxed HQ get the full mid/late speed-up without further action.

## 11. Implementation notes (for plan author)

- Prefer exporting `balance.OfficeTokenRnDMultAt` and calling it from both `sim` and `tui` to avoid duplicated clamp logic.
- Do not change `sim.Tick` signature.
- Keep `TokenRawRnD` pure (raw only); HQ mult stays at booking / display composition sites.
- Commits: follow repo `type(scope): summary` style.
