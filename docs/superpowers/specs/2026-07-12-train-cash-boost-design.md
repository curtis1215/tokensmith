# Train Cash Boosts — Design

- **Date:** 2026-07-12
- **Status:** Approved for implementation
- **Scope:** Per-training cash consumables that add quality on top of R&D allocation; non-exploitable revenue-anchored pricing; rival target-side investment (no unlock cliff)
- **Related:** `docs/superpowers/specs/2026-07-07-tokensmith-design.md` (train/quality); model publish flow; long-term progression (`QualityScale`, `GlobalFrontier`, rival catch-up)

---

## 0. Review amendments (must-fix)

This revision incorporates design review. **Do not implement the original Approved draft.**

| # | Issue | Fix in this doc |
|---|---|---|
| 1 | Global quality soft cap made more spend **reduce** dim bonuses / segment appeal (esp. Enterprise/Developer) | **Remove quality soft cap.** Full additive bonuses. Suppress full-buy via **progressive slot pricing** only. |
| 2 | Anchor used `Users×Price` → instant `$1` price exploit | Anchor uses **`Users × EffectiveRefPrice(segment)`**, never current sticker price. |
| 3 | “年營收” ignored `RevenueMult` / prestige / campaign mults | Anchor is **effective gross monthly cash at ref pricing** (single helper). |
| 4 | Rival overlay on `MaxUnlockedGen` → gen-unlock difficulty cliff | Rival bonus is **target-side only**, scaled from **global frontier**, applied through existing catch-up. |
| 5 | Rival quality read at many sites | Player models store final quality; rivals use **stored `Competitor.Quality` only**. Optional `EffectiveRivalQuality` = identity helper + consistency tests so future overlays cannot fork. |
| 6 | Tests only unit-level | Add **monotonicity, anti-exploit, economy, long-run** gates (§12). |
| 7 | Knobs / save invariants missing | §11 validation rules for config + training job fields. |

---

## 1. Problem

1. **Cash is weakly coupled to model quality.** Training spends R&D only. Allocation (`Alloc`) is zero-sum across four dimensions; money does not improve a run’s four-dim profile.
2. **At a fixed generation, the player is allocation-capped.** `Quality[d] = Alloc[d] × QualityScale × tech × star` with `Σ Alloc = 1`, so specializing one dim weakens others. Rivals approach a multi-dim global frontier via Skill bands and can look **healthier** without a zero-sum Alloc.
3. **Surplus cash lacks a sharp sink** on the train → publish → compete loop.
4. **Any cash sink must not be soft-cap’d into anti-synergy**, must not be **price-warpable**, and must not **punish gen unlocks** via free rival power spikes.

## 2. Goals

| Goal | Success criterion |
|---|---|
| Cash buys per-training consumables that raise quality dims | Train dialog: optional ZH investments, live prices, live **predicted appeal** |
| Alloc stays zero-sum; cash is **strictly non-zero-sum on top** | Adding any selected boost **never decreases** any `CashBonus[d]`, final `Quality[d]`, or segment **appeal** (fixed Alloc/tech/star) |
| Full-pack is expensive, not quality-toxic | Full select costs on the order of **gen years of effective annual cash-in**; 3rd/4th items cost progressively more; quality still rises |
| Pricing not warpable by instant `SetPrice` | Quote unchanged when only model `Price` changes (users fixed) |
| “年營收” matches cash that actually hits the ledger (gross) | Anchor includes `RevenueMult`, prestige `CashMult`, campaign segment `RevenueMult` |
| Rival health without unlock cliffs | Rival investment only raises **catch-up targets**; Quality moves at `CompetitorCatchupRate` |
| Single quality authority | Player: `Model.Quality` after complete. Rival: `Competitor.Quality` only (boost via targets) |
| Tunable + safe | Knobs in `balance.Config` with finite/non-neg invariants and store validation |

## 3. Non-goals

- Multi-tier items per dim (basic/flagship)
- Permanent company assets or mid-training top-ups
- Charging cash for base `TrainRnD`
- Rival cash ledgers / rival shop UI
- Net-of-opex (rent/salary) pricing anchor for v1
- Frontier-project cash, chip design

---

## 4. Player-facing catalog (Chinese names)

Exactly four consumables, one per quality dim. At most one per dim per training run.

| Dim | Internal ID | Display name (ZH) | Intent |
|---|---|---|---|
| Capability | `boost-data` | **優質語料** | Better training data |
| Efficiency | `boost-efficiency` | **省算力改造** | Cheaper to serve |
| Safety | `boost-safety` | **安全評測** | Enterprise-safe |
| Speed | `boost-speed` | **加速優化** | Faster responses |

Copy: plain Chinese; no API/SFT/RLHF jargon.

---

## 5. Quality formula (player)

### 5.1 Unchanged

- `Σ Alloc ≈ 1`, non-negative.
- R&D cost: `TrainRnD(gen) × TrainRnDMult`.
- Training work / compute progression unchanged.

### 5.2 Completion

At **training start**, freeze `CashBonus[4]` on the job (full additive; §5.3).  
At **completion**:

```text
Quality[d] = (Alloc[d] × QualityScale(gen) + CashBonus[d])
             × TechQualityMult[d] × StarQualityMult[d]
```

No boosts → bit-identical to today’s formula.

### 5.3 Bonus magnitude — **no global quality soft cap**

```text
CashBonus[d] = Boosts[d] ? TrainBoostBeta × QualityScale(gen) : 0
```

**v0:** `TrainBoostBeta = 0.15`.

**Monotonicity invariant (hard):**

For fixed `Alloc`, `gen`, tech, stars, and segment weights `w`:

- Let `S ⊂ S'` be selected boost sets (component-wise `Boosts` implies).
- Then `CashBonus_S[d] ≤ CashBonus_S'[d]` for all `d`.
- Predicted `Quality` and `appealOf(Quality, w)` are **non-decreasing** when any additional boost is enabled.
- **Corollary:** full pack cannot be an appeal trap relative to best-two or any subset.

**Full-buy suppression is price-only** (§6.3), never quality compression.

---

## 6. Cash pricing

### 6.1 Anchor definition (single authority)

Introduce pure helper (name illustrative):

```text
// Gross monthly cash inflow if every online model charged EffectiveRefPrice,
// including all revenue mults that Tick uses for cash accrual — except sticker Price.
BoostRefMonthlyCash(s, b) =
  Σ_{m online} m.Users
      × EffectiveRefPrice(s, m.Segment, b)
      × campaignEffects(s,b).RevenueMult[m.Segment]
      × PrestigeEffects(...).CashMult
      × b.RevenueMult

refMonthly = max(BoostRefMonthlyCash(s, b), b.TrainBoostFloorMonthly)
annualRef  = 12 × refMonthly
```

**Decisions:**

| Question | Choice |
|---|---|
| Book subscription (`Users×Price`) vs cash inflow | **Effective gross cash at ref price** |
| Include `b.RevenueMult` / prestige / campaign rev mult | **Yes** — same mults as `advanceUsers` cash line |
| Use current `Model.Price` in anchor | **Never** |
| Net of rent/salary | **No** (v1 gross only) |
| Floor | `TrainBoostFloorMonthly = StartingCash / 12` |

**Anti-exploit:** Changing only `Model.Price` (including temporary `$1`) must not change `BoostRefMonthlyCash` while `Users` and mults are fixed. Automated test required.

`EffectiveRefPrice` already folds tech / event / campaign **ref-price** mults (`sim.EffectiveRefPrice`).

### 6.2 Linear base share of “gen years”

```text
targetFullLinear = gen × annualRef × TrainBoostPainMult
// Gen1 = 1 year, Gen2 = 2 years, … of effective annual cash-in
// v0 PainMult = 1.0

basePrice[d] = targetFullLinear × RoleWeight[d] / Σ RoleWeight
```

**v0 role weights**

| Item | RoleWeight | Share |
|---|---|---|
| 優質語料 | 1.2 | ≈ 28.6% |
| 省算力改造 | 1.0 | ≈ 23.8% |
| 安全評測 | 1.1 | ≈ 26.2% |
| 加速優化 | 0.9 | ≈ 21.4% |
| **Σ** | **4.2** | 100% |

### 6.3 Progressive slot multipliers (replace quality soft cap)

When `k` boosts are selected, order selected dims by **ascending dim index** (stable; independent of toggle order). Assign slot ranks `0..k-1`:

```text
SlotMult[0] = 1.00
SlotMult[1] = 1.00
SlotMult[2] = 1.80   // 3rd item
SlotMult[3] = 2.50   // 4th item

cashCost = Σ basePrice[d] × SlotMult[rank(d)]
```

**Properties:**

- 1–2 items: pay linear base shares only.
- 3rd/4th: steep surcharge → full pack **≥** `targetFullLinear` (typically ~1.3–1.6× depending on which dims), so full-buy is a **cash** trap, not a quality trap.
- Exact full-pack / linear ratio is determined by weights × slot mults; expose helper `FullPackCashCost(gen, refMonthly, b)` for TUI and tests.

**v0 slot mults** are balance knobs (`TrainBoostSlotMult [4]float64`).

### 6.4 Worked sketch (floor only, PainMult=1)

`annualRef = StartingCash = 100_000`, Gen1 `targetFullLinear = 100_000`.

| Selection | Quality | Cash (illustrative) |
|---|---|---|
| 1 item (weight 1.0) | +0.15×scale on that dim | ~23.8k |
| Best 2 for a segment | +β each on those dims | sum of two bases |
| All 4 | +β on **all four** (no compress) | bases × slot mults ≫ two-item cost |

Segment appeal for Enterprise/Developer **rises** when adding safety/efficiency/speed on top of capability — unlike the rejected soft-cap design.

### 6.5 TUI quote copy

Avoid lying “年營收” if confused with raw `Users×Price`:

- Label: **參考月現金（標竿價）** / **約 N 年有效現金流入**
- Show whether **floor** is binding.
- Per toggle: **預測區隔吸引力** before → after (must not drop).
- Show marginal cash of enabling the focused item and slot-rank surcharge if rank ≥ 2.

---

## 7. Commands and state

### 7.1 `StartTraining`

```text
StartTraining {
  Gen, Segment, Alloc, Price
  Boosts [NumQualityDims]bool
}
```

### 7.2 `TrainingJob`

```text
TrainingJob {
  Gen, Segment, Alloc, Price, WorkRemaining
  Boosts        [NumQualityDims]bool
  CashBonus     [NumQualityDims]float64  // frozen additive bonuses
  BoostCashPaid float64                  // total cash charged
}
```

Freeze at start: bonus points and cash charged. Completion applies tech/star mults to `(Alloc×scale + CashBonus)` as in §5.2; does not re-quote revenue.

### 7.3 `applyStartTraining`

1. Existing validations (busy, gen unlock, Alloc, Price, R&D).
2. `refMonthly = max(BoostRefMonthlyCash(s,b), Floor)`.
3. `cashCost` via §6.2–6.3; `Cash < cashCost` → `ErrInsufficientCash`, no mutate.
4. Deduct R&D + cashCost.
5. `CashBonus[d] = Boosts[d] ? β×QualityScale : 0` (no soft cap).
6. Write job fields; start work as today.

### 7.4 Persistence

- Missing fields → false/0 (legacy).
- Store validation (§11): non-negative `CashBonus`, `BoostCashPaid`; `CashBonus[d]==0` if `!Boosts[d]`; finite values; `BoostCashPaid` consistent within tolerance **or** treat paid as authoritative audit field and recompute bonus only from Boosts+gen at load if desired (pick one in plan; prefer **trust Boosts+gen for bonus, trust BoostCashPaid as historical charge**).

---

## 8. Rivals — target-side investment (no overlay cliff)

### 8.1 Rejected approach

- Instant `effectiveQuality = Quality + f(player MaxUnlockedGen)` — causes gen-unlock cliffs and desync vs intel/UI.

### 8.2 Chosen approach

Fold rival “train investment” into **`rivalTarget` only**, then existing `advanceRivalLeague` / catch-up (`CompetitorCatchupRate`) moves `Competitor.Quality`.

```text
// After base target[d] = GlobalFrontier[d] × clamp(Skill+leader+momentum, 0.85, 1.15):

picks = top TrainBoostRivalPicks dims by Skill[d] (tie: lower dim index)
for d in picks:
  target[d] += TrainBoostBeta * GlobalFrontier[d]
// then catch-up toward target; clampRivalToBand still applies to Quality
```

**Why this is safe:**

- Scale tracks **`GlobalFrontier`** (player published frontier + time frontier), not “tech node just unlocked.”
- Unlocking Gen2 without a stronger online model / time frontier does **not** instantly buff all seven rivals.
- Target jumps only when GF jumps; Quality still has ~months half-life toward gap.
- Intel showing `Competitor.Quality` matches combat math.

**v0:** `TrainBoostRivalPicks = 2`.

**Band interaction:** Extra absolute target may sit above `1.15×GF` before clamp on Quality. Accept that **band clamp remains the hard ceiling** — boost improves approach toward the top of the band on specialty dims (healthier profile vs flat lag), without unbounded rival quality. If playtests show boost is too weak under band, prefer raising specialty via a **small skill-equivalent pct** inside the existing 0.85–1.15 clamp (balance-only), not a UI overlay.

### 8.3 Single authority for rival quality reads

```text
EffectiveRivalQuality(s, rival, b) [NumQualityDims]float64
  // v1: return rival.Quality unchanged (boost already in sim targets)
```

**Mandate:** every appeal, share, rank, threat, campaign gate, **and** rival intel projection calls this helper (or raw `Quality` only through it). Add a test that greps/reflects or table-drives known projection entry points so market rank ≡ tick user-growth rival terms ≡ campaign rank inputs.

Player trained models: **no** parallel effective layer — `Model.Quality` is final after complete.

---

## 9. TUI

Train dialog:

1. Existing gen / segment / Alloc.
2. **訓練投資（可選）** — four ZH rows, toggle, **marginal + total** cash.
3. Summary:
   - 參考月現金（標竿價）; floor badge if active
   - 全滿現金（含 3/4 件加成價）vs 約 gen 年有效現金
   - **預測吸引力** for selected segment: 目前配置 → 加上投資後 (must not fall when enabling items)
4. Insufficient cash: clear error; dialog stays usable.

No “已進入壓縮” quality warning (removed). Optional: “第 3/4 件溢價中” for slot mults.

---

## 10. Package boundaries

| Package | Responsibility |
|---|---|
| `balance` | Catalog, β, PainMult, floor, weights, slot mults, rival picks; pure basePrice math if state-free pieces live here |
| `model` | `StartTraining.Boosts`, job boost fields |
| `sim` | `BoostRefMonthlyCash`, pricing with state, apply/complete, `rivalTarget` extra, `EffectiveRivalQuality` |
| `tui` | Dialog, quotes, predicted appeal |
| `store` | Validate new job fields on load |

Sim stays pure/deterministic (state + config only).

---

## 11. Config and save invariants

### 11.1 `balance.Config` (validate in tests / `Default` sanity)

| Field | Invariant |
|---|---|
| `TrainBoostBeta` | finite, `≥ 0` |
| `TrainBoostPainMult` | finite, `> 0` |
| `TrainBoostFloorMonthly` | finite, `≥ 0` |
| `RoleWeight[d]` | finite, `> 0`; **Σ > 0** |
| `TrainBoostSlotMult[i]` | finite, `≥ 1` for i≥0; recommended non-decreasing |
| `TrainBoostRivalPicks` | integer in `0..NumQualityDims` |
| Catalog | exactly one entry per dim; unique IDs; non-empty ZH names |

**Monotonicity under knobs:** For any β≥0 and any boost set nest, appeal non-decreasing (property test). Slot mults must not be used to scale **quality**.

### 11.2 Active `TrainingJob` on load

In addition to existing Gen / WorkRemaining checks:

- `CashBonus[d]` finite and `≥ 0`
- `BoostCashPaid` finite and `≥ 0`
- If `!Boosts[d]` then `CashBonus[d] == 0` (or repair to 0)
- Optional: recompute expected bonus from `Boosts+Gen+β` and repair drift

---

## 12. Testing and acceptance gates

### 12.1 Unit / formula

| Case | Expect |
|---|---|
| No boosts | Quality regression-identical; cash unchanged by boost path |
| Single / multi boost | Additive β×scale per selected dim |
| Nested sets | Appeal and all Quality dims non-decreasing as sets grow |
| Full pack quality | Strictly ≥ best-2 quality componentwise on boosted dims; appeal ≥ best-2 for all three segment weight vectors |
| Floor | Zero users → floor monthly |
| Price exploit | Same users; `Price→1` then quote **unchanged** |
| Mult alignment | Doubling `b.RevenueMult` doubles anchor (users>0, above floor) |
| Insufficient cash | No R&D/cash/job mutate |
| Slot mults | 3- and 4-item costs match rank rule; independent of toggle order |
| Legacy save | Missing fields OK |

### 12.2 Rival / progression

| Case | Expect |
|---|---|
| Gen unlock cliff | Unlock gen tech only (no new online model, fixed time): rival Quality **unchanged** on next tick beyond normal catch-up to **previous** target path; no discontinuous jump larger than one catch-up step allows |
| Target includes boost | Top-K skill dims have higher `rivalTarget` than without boost flag (unit on `rivalTarget`) |
| Consistency | MarketRank / share / tick rival appeal / campaign rank / intel all use `EffectiveRivalQuality` |

### 12.3 Economy / strategy gates (automated where feasible)

| Gate | Spec |
|---|---|
| Monotonic spend | Enabling any extra item never lowers predicted segment appeal (Consumer, Enterprise, Developer weights) |
| Anti-arbitrage | Instant `SetPrice` cannot reduce boost quote without `Users` change |
| Affordance sketch | Document expected ballpark: Gen1 at floor, 1 item ≤ ~30% starting cash; full pack ≥ 1× floor annual before slot mults, higher after — assert numeric ranges in balance tests |
| Long-run smoke | Consumer / Enterprise / Developer scenarios: with boosts available, campaign or share progress remains finite; enabling rival target boost does not make win-time worse than **pre-feature baseline × agreed factor** (plan sets factor, e.g. 1.25) once catch-up settles |

Subjective “feels painful” is **not** a substitute for §12.1–12.2.

---

## 13. Defaults summary (v0)

| Key | Value |
|---|---|
| β | 0.15 |
| PainMult | 1.0 |
| FloorMonthly | `StartingCash / 12` |
| Role weights | 1.2 / 1.0 / 1.1 / 0.9 |
| SlotMult | 1.0 / 1.0 / 1.8 / 2.5 |
| RivalPicks | 2 |
| Quality soft cap σ | **removed** |
| Names | 優質語料、省算力改造、安全評測、加速優化 |
| Anchor | `Users × EffectiveRefPrice × campaign rev × prestige cash × RevenueMult` |

---

## 14. Implementation order (for writing-plans)

1. `BoostRefMonthlyCash` + price-exploit tests  
2. balance catalog, base + slot pricing, monotonicity property tests  
3. model fields; apply/complete additive bonus  
4. `rivalTarget` investment + unlock-cliff test  
5. `EffectiveRivalQuality` wiring + consistency test  
6. store validation  
7. TUI dialog (prices, predicted appeal, no compress UI)  
8. Segment long-run smokes  

---

## 15. Decisions log

| Topic | Decision |
|---|---|
| Stacking | Cash **outside** Alloc; full additive β per selected dim |
| Anti full-buy (quality) | **None** — monotonic quality/appeal required |
| Anti full-buy (cash) | Progressive **slot multipliers** on 3rd/4th selected dims |
| Lifecycle | Per-training consumables at start |
| Naming | ZH catalog §4 |
| Anchor | Effective gross cash at **ref price**, with Tick revenue mults; never sticker `Price` |
| Floor | `StartingCash/12` |
| Years rule | `targetFullLinear = gen × 12 × refMonthly × PainMult` |
| Rivals | Target-side `+ β×GF[d]` on top-K skill dims; catch-up only; no MaxUnlockedGen overlay |
| Rival reads | Single helper; v1 identity on stored Quality |
| Status | Revised after review; re-approve before implementation plan |
