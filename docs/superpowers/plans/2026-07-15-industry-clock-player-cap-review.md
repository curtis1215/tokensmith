# Code Review — PR #19 fix(sim): cap industry clock to player gen and throttle idle

- 日期：2026-07-15
- 分支：`fix/industry-clock-player-cap` → `main`
- 審查範圍：`internal/balance`、`internal/sim`、`internal/store`、`internal/tui`（不含 docs）
- 測試：`go test ./...` **全數通過**（sim 1.7s、store 1.5s、tui 2.7s，其餘綠）

## Verdict: APPROVE

核心邏輯正確、雙時鐘一致、load soft-repair 無副作用、無 campaign/longrun/prestige 回歸。僅有 minor/nit 級別發現，皆不擋合併。

## Findings

### Minor

1. **`IndustryTimeCapSec` 的 doc 與實作不一致 + dead code**（`internal/sim/industry_clock.go:13`）
   Doc 寫「On catalog failure returns +Inf」，實際回傳 `math.MaxFloat64`。下游 `IndustryTimeResidualToCap`、`EffectiveIndustryDay` 的 `math.IsInf(cap, 0)` 檢查因此永遠為 false，是 dead branch。功能上仍正確（`MaxFloat64` 語意等同無上限，減法不會 overflow，`>` 比較不會誤觸發），但語義漂移：日後若有人「修正」成真的 `math.Inf(1)` 或依 doc 假設 Inf，行為會微妙變化。建議一行修正：改回傳 `math.Inf(1)`，或把 doc 與 IsInf 檢查統一成 MaxFloat64 語意。

2. **測試硬編碼 catalog 常數**（`internal/sim/frontier_test.go` `TestTimeFrontierCappedByPlayerLead`、`internal/store/store_test.go` `TestLoadClampsOverheatedIndustryTime`）
   `40500 * 86400` 與「Gen10 baseline day 40000」假設寫死在測試裡。catalog 若調 TimeBaselineDay，測試意圖會漂移（雖有 `uncapped*0.9` 防呆與 err check 不至於誤過）。可改為從 `balance.Generation(10).TimeBaselineDay` 推導。

### Nit

3. **`max` / `cap` 遮蔽 Go 1.21 builtin**（`industry_clock.go:14`、`sim.go:79` 等）。合法且 codebase 既有慣例（`rivals.go:55`、`frontier_project.go:127` 同款），可不改。

4. **`Frontier.Active` 且 `AllocationPct==0` 也算 engaged**（`EffectiveIndustryDT`）：掛著 frontier project 不撥算力仍拿全速 industry。非 exploit —— industry 跑快對玩家不利（rival 變強），玩家沒有動機這樣做，列為已知邊界即可。

5. **Offline 不套 idle throttle 的不對稱**：線上 idle 0.15×，離線 settle 拿全速 allowance（受 8h × compression、oneGen、playerCap 三重上限）。理論上「關遊戲比掛機推 industry 快」，但兩者終點都被 playerCap 綁死，且設計文件（§93、§107）明示此結構，符合意圖。

## Checklist 驗證（皆通過）

- **IndustryTimeCapSec / EffectiveIndustryDT / EffectiveIndustryDay 正確性**：cap = `Generation(MaxUnlockedGen+1).TimeBaselineDay×86400`；`MaxUnlockedGen()` 已保證 ≥1，capGen ≥2；capGen 超出 catalog（endgame）→ 無上限，合理。idle 判定、residual clamp、NaN/負值防護齊全。
- **Tick vs OfflineTick 雙時鐘一致**：`Tick` 以 pre-tick state 算 `EffectiveIndustryDT`，`tickWithClocks` 內 defensive clamp 兜底且 OfflineTick 共用同一 clamp。同 tick 內 `MaxUnlockedGen` 只會在 clamp 之後（`advanceFrontierProject`）上升，cap 單調不減，不會誤砍。
- **TimeFrontier cap vs raw IndustryTime 顯示不變量**：raw `IndustryTime` 僅剩兩個消費者 —— `TimeFrontier`（改走 `EffectiveIndustryDay`）與 `SecondsUntilNextTimeGeneration`（settle allowance 與 playerCap 取 min）。TUI 無其他直接讀取。線上 tick 後 IndustryTime ≤ cap，raw 與 effective 收斂一致。
- **Load soft-repair 副作用**：三條路徑（legacy、schema 1→2、current）齊套 `ClampIndustryToPlayerCap`；順序正確（先 clamp IndustryTime，再以 post-clamp GlobalFrontier reband rivals）；玩家模型品質不動（store test 驗證）。無條件 reband 冪等 —— band 本就是每 tick / 每 board-cycle 的全域不變量。不 bump schema 正確（soft repair 冪等，舊版 binary 讀新 save 無害）。
- **Rival band reclamp**：`clampAllRivalsToBand` 以 post-clamp `GlobalFrontier` 計算，`clampRivalToBand` 處理 NaN/負值。✓
- **Prestige 互動**：`freshRun` 重建全新 state（IndustryTime=0、MaxUnlockedGen=1），defensive clamp 不會誤砍新局。✓
- **Campaign share**：campaign cycle 為 board-cycle 外部驅動，不吃 industry clock；rival roadmap 在 cycle 內推進並自行 clamp band。idle throttle 只影響 rubber-band 速率，band 不變量不受影響。✓
- **Longrun**：`longrun_test.go` 模擬主動玩家（frontier/training engaged），全速路徑不變，通過。✓
- **Balance 常數位置**：`IndustryPlayerLeadGens`、`IndustryIdleMult` 為 package const，與 `RealSecCompression` 既有模式一致；若未來需 per-save 調整再搬進 `balance.Config`。✓
- **測試覆蓋**：cap 計算、residual、idle/engaged/at-cap DT、負 dt、residual clamp、load 修復、settle 停在 cap、defensive clamp 皆有直接測試。設計文件 §178 的 `TestTickUsesEffectiveIndustryDT` 以 `TestTickIdleIndustryThrottle` 之名落地、§181 的 `TestOfflineAllowanceIncludesPlayerCap` 以端到端 `TestSettleIndustryStopsAtPlayerCap` 落地（`offlineIndustryAllowance` unexported，端到端覆蓋可接受）。

## Suggested follow-ups

1. （一行）統一 `IndustryTimeCapSec` 的 Inf/MaxFloat64 語義，消除 dead `IsInf` 分支。
2. （測試健壯性）`TestTimeFrontierCappedByPlayerLead` / `TestLoadClampsOverheatedIndustryTime` 的魔法天數改由 catalog 推導。
