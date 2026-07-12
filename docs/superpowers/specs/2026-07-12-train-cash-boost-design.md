# Train Cash Boosts — Design

- **Date:** 2026-07-12
- **Status:** Approved (brainstorming)
- **Scope:** Per-training cash consumables that add quality on top of R&D allocation; revenue-anchored pricing; rival implicit boosts
- **Related:** `docs/superpowers/specs/2026-07-07-tokensmith-design.md` (train/quality); model publish flow; long-term progression (`QualityScale` per gen)

---

## 1. Problem

1. **Cash is weakly coupled to model quality.** Training spends R&D only. Allocation (`Alloc`) is zero-sum across four dimensions; money does not improve a run’s four-dim profile.
2. **At a fixed generation, the player is allocation-capped.** `Quality[d] = Alloc[d] × QualityScale × tech × star` with `Σ Alloc = 1`, so specializing one dim weakens others. Rivals use Skill + frontier catch-up and often look **higher and healthier** across dims without that zero-sum tradeoff.
3. **Surplus cash lacks a sharp sink tied to the core loop** (train → publish → compete).

## 2. Goals

| Goal | Success feel |
|---|---|
| Cash buys **per-training consumables** that raise quality dims | Opening train dialog: optional Chinese-named investments with live prices |
| Alloc stays zero-sum; cash is **non-zero-sum on top** | Player can fix a weak dim without stealing from the main Alloc focus |
| Full-pack spend is **painful and revenue-scaled** | Full select ≈ `gen` years of annual revenue (Gen1 = 1y, Gen2 = 2y, …) |
| Soft cap discourages mindless full-buy | Full buy costs full sticker price but quality bonuses compress |
| Rivals share the **same bonus math** (implicit) | Same-gen rivals look healthier without a shopping UI |
| Tunable without rewiring | β, σ, PainMult, floor, weights live in `balance.Config` |

## 3. Non-goals

- Multi-tier items per dim (basic/flagship)
- Permanent company assets or mid-training top-ups
- Charging cash for base `TrainRnD` (R&D gate unchanged)
- Simulating rival cash ledgers or rival shop UI
- Frontier project cash, campaign P&L, chip design

## 4. Player-facing catalog (Chinese names)

Exactly four consumables, one per quality dim. At most one per dim per training run.

| Dim | Internal ID (stable) | Display name (ZH) | Intent |
|---|---|---|---|
| Capability | `boost-data` | **優質語料** | Better training data → smarter model |
| Efficiency | `boost-efficiency` | **省算力改造** | Distill/compress → cheaper to serve |
| Safety | `boost-safety` | **安全評測** | Red-team / alignment review → enterprise-safe |
| Speed | `boost-speed` | **加速優化** | Inference engineering → faster responses |

Copy rules: plain Chinese; no API/SFT/RLHF jargon in the TUI.

## 5. Quality formula

### 5.1 Unchanged pieces

- `StartTraining` still requires `Σ Alloc ≈ 1`, non-negative entries.
- Base R&D cost: `TrainRnD(gen) × TrainRnDMult` (tech), unchanged.
- Work remaining / compute progression unchanged.

### 5.2 Completion formula (new)

At **training start**, compute and freeze `CashBonus[4]` on the job (see §7).  
At **completion**:

```text
Quality[d] = (Alloc[d] × QualityScale(gen) + CashBonus[d])
             × TechQualityMult[d] × StarQualityMult[d]
```

If no boosts were selected, `CashBonus` is all zero → **bit-identical** to today’s formula (regression target).

### 5.3 Bonus magnitude and soft cap

```text
rawBonus[d] = Boosts[d] ? TrainBoostBeta × QualityScale(gen) : 0

softCap = TrainBoostSoftCap × QualityScale(gen)

if sum(rawBonus) <= softCap or sum == 0:
  CashBonus = rawBonus
else:
  CashBonus[d] = rawBonus[d] × (softCap / sum(rawBonus))   // proportional compress
```

**v0 defaults**

| Knob | Symbol | Default | Meaning |
|---|---|---|---|
| `TrainBoostBeta` | β | `0.15` | One item ≈ 15% of `QualityScale` points |
| `TrainBoostSoftCap` | σ | `0.30` | Total cash points capped at 30% of scale |
| Max per dim | — | 1 | One consumable slot per dim |

**Implication:** selecting 1–2 dims keeps full per-item β; selecting all four compresses each item’s realized bonus. Sticker cash is **not** reduced by the soft cap.

## 6. Cash pricing (revenue anchor)

### 6.1 Reference monthly revenue

Use the same definition as `sim.MonthlyRevenue`: sum over online models of `Users × Price` (monthly).

```text
refMonthly = max(MonthlyRevenue(state), TrainBoostFloorMonthly)
```

**v0 floor:** `TrainBoostFloorMonthly = StartingCash / 12`  
(with `StartingCash = 100_000` → floor ≈ `8_333.33`, floor annual ≈ `100_000`).

Purpose: pre-revenue runs still pay a startup-scale pack; post-revenue runs scale with success (**richer → more painful**).

### 6.2 Full pack and per-item sticker

```text
annualRef     = 12 × refMonthly
fullPackPrice = gen × annualRef × TrainBoostPainMult
// Gen1 → 1× annual, Gen2 → 2× annual, …
```

**v0:** `TrainBoostPainMult = 1.0` (strict “full pack = gen years of annual revenue”).

Per-item price from role weights:

```text
Price(d) = fullPackPrice × RoleWeight[d] / Σ RoleWeight
cashCost = sum Price(d) for each selected Boosts[d]
```

**v0 role weights**

| Item | RoleWeight | Share of full pack |
|---|---|---|
| 優質語料 | 1.2 | ≈ 28.6% |
| 省算力改造 | 1.0 | ≈ 23.8% |
| 安全評測 | 1.1 | ≈ 26.2% |
| 加速優化 | 0.9 | ≈ 21.4% |
| **Σ** | **4.2** | 100% |

### 6.3 Evaluation checklist (for future tuning)

When changing knobs, verify:

1. **β / σ** — one purchase noticeably patches a weak dim; full-buy quality is worse value than 1–2 focused buys.
2. **PainMult / gen×years** — full select feels like multi-year revenue commitment at that gen.
3. **Floor** — Gen1 first train without users is expensive vs starting cash but not impossible for 1 item.
4. **High revenue** — mid/late game full pack still hurts (does not become pocket change relative to cash pile alone; it tracks income).

Pricing shape stays; only balance constants move.

### 6.4 Worked examples (PainMult=1, floor active)

**No users (floor annual = 100k)**

| Gen | Full pack | ~單件 (weight 1.0) |
|---|---|---|
| 1 | 100,000 | ~23,800 |
| 2 | 200,000 | ~47,600 |
| 5 | 500,000 | ~119,000 |

**Monthly revenue 50,000 → annual 600,000**

| Gen | Full pack |
|---|---|
| 1 | 600,000 |
| 2 | 1,200,000 |
| 3 | 1,800,000 |

## 7. Commands and state

### 7.1 `StartTraining`

```text
StartTraining {
  Gen     int
  Segment Segment
  Alloc   [NumQualityDims]float64
  Price   float64
  Boosts  [NumQualityDims]bool   // NEW: true = buy that dim’s consumable
}
```

### 7.2 `TrainingJob`

```text
TrainingJob {
  Gen, Segment, Alloc, Price, WorkRemaining  // existing
  Boosts        [NumQualityDims]bool         // NEW
  CashBonus     [NumQualityDims]float64      // NEW: post soft-cap, frozen at start
  BoostCashPaid float64                      // NEW: total cash charged for boosts
}
```

**Freeze rule:** `CashBonus` and `BoostCashPaid` are computed in `applyStartTraining` from state **at start**. Completion must not recompute from later revenue, tech, or stars (tech/star mults still apply at completion as today on the summed base+bonus).

### 7.3 `applyStartTraining` order

1. Existing validations: not already training; valid gen / unlock; Alloc sum ∈ [0.999, 1.001]; Price > 0; sufficient R&D.
2. `gen >= 1` (invalid gen already errors).
3. `refMonthly = RefMonthlyRevenue(s, b)`.
4. `cashCost` from selected boosts; if `Cash < cashCost` → `ErrInsufficientCash` (no partial mutate).
5. Deduct R&D cost and `cashCost`.
6. `CashBonus = ApplySoftCap(raw bonuses)`; store Boosts, CashBonus, BoostCashPaid on job.
7. Set `HasTraining` / `WorkRemaining` as today.

### 7.4 Persistence

- New fields on job / command: zero value = legacy behavior.
- No mandatory save migration if JSON omitempty / missing fields decode as false/0.
- Draft `Model.Quality` already stores final numbers; no extra model fields required for v1.

## 8. Rivals (implicit budget)

- Do **not** debit rival cash or persist rival purchases.
- Introduce pure helper, e.g. `RivalTrainBoostBonus(c Competitor, gen int, b Config) [4]float64`:
  - Use `QualityScale(gen)` with `gen = max(1, player MaxUnlockedGen)` (same ruler as player’s current gen ceiling).
  - Implicitly select **`TrainBoostRivalPicks` (v0 = 2)** dims with highest `Skill[d]` (stable tie-break: lower dim index).
  - Build raw bonuses with same β; apply same soft cap.
- **Effective rival quality** for appeal / market / campaign comparisons:

```text
effectiveQuality[d] = Competitor.Quality[d] + RivalTrainBoostBonus(...)[d]
```

- Do **not** write bonus into `Competitor.Quality` (avoids double application and save bloat).
- Narrative: rivals already spend on the order of multi-year revenue-class training investment; math is shared, ledger is not.

**v0:** `TrainBoostRivalPicks = 2`.

## 9. TUI

Train dialog (`dialog_train`):

1. Existing gen / segment / Alloc controls.
2. Section **訓練投資（可選）** listing four ZH names, checkbox/toggle, live unit price.
3. Summary lines:
   - 參考月營收 (show actual vs floor if floor binds)
   - 全滿預算 = gen × 年營收（× PainMult if ≠ 1）
   - 已選合計 + approximate years of revenue
   - Soft-cap meter: selected raw bonus sum vs softCap (warn when compression applies)
4. Confirm path surfaces insufficient cash clearly; selection must not soft-lock the dialog.

Exact keybindings left to implementation plan; requirement is: toggle each of four, see prices, submit via existing confirm.

## 10. Package boundaries

| Package | Responsibility |
|---|---|
| `internal/balance` | Catalog, knobs, pure helpers: ref floor constant wiring, full pack / item price, raw bonus, soft cap (preferred home for pure pricing math) |
| `internal/model` | `StartTraining.Boosts`, `TrainingJob` boost fields |
| `internal/sim` | `applyStartTraining` charges; completion quality; `MonthlyRevenue` reuse; rival effective quality in appeal paths |
| `internal/tui` | Train dialog UX and price display |
| `internal/store` | Rely on backward-compatible decode; only add migration if a format break appears |

Keep `internal/sim` pure/deterministic: pricing uses state + config only (no wall clock).

## 11. Testing

| Case | Expect |
|---|---|
| No boosts | Quality matches pre-change formula; cash unchanged by boosts |
| Single dim boost | That dim gains β×scale (pre tech/star); cash decreases by Price(d) |
| All four boosts | `BoostCashPaid == fullPackPrice`; `Σ CashBonus == σ×scale` (within float tol) |
| Zero users | Pricing uses floor monthly |
| High MonthlyRevenue | fullPack = gen×12×revenue×PainMult |
| Insufficient cash | Error; R&D and cash and training flags unchanged |
| Rival effective quality | Top Skill dims receive implicit picks; appeal uses effective quality |
| Legacy job / save | Missing boost fields → zeros; train still works |

## 12. Defaults summary

| Key | v0 value |
|---|---|
| β `TrainBoostBeta` | 0.15 |
| σ `TrainBoostSoftCap` | 0.30 |
| `TrainBoostPainMult` | 1.0 |
| `TrainBoostFloorMonthly` | `StartingCash / 12` |
| `TrainBoostRivalPicks` | 2 |
| Role weights | 1.2 / 1.0 / 1.1 / 0.9 |
| Display names | 優質語料、省算力改造、安全評測、加速優化 |

## 13. Implementation notes for planning

Suggested order:

1. `balance` catalog + pure price/bonus/softcap helpers + unit tests  
2. `model` field extensions  
3. `sim` start/complete + insufficient cash + regression  
4. Rival effective quality wiring (grep appeal/competitor comparison sites)  
5. TUI train dialog  
6. Manual feel pass on α-equivalents via real revenue scenarios  

---

## 14. Decisions log (brainstorming)

| Topic | Decision |
|---|---|
| Stacking model | Cash bonus **outside** Alloc (Alloc remains sum=1) |
| Lifecycle | Per-training **consumables** at start |
| Stacking limits | Max one item per dim + global soft cap σ |
| Rivals | Implicit same formula; K=2 skill-preferred dims; no shop |
| Approach | Catalog checkboxes in train dialog (approach A) |
| Naming | Chinese plain names (table §4) |
| Pricing anchor | `fullPack = gen × 12 × max(actual monthly, floor) × PainMult` |
| Floor | `StartingCash/12` |
| PainMult default | 1.0 |
