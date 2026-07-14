# Industry Clock Player Cap & Online Idle Throttle — Design

- **日期**：2026-07-15
- **狀態**：已實作
- **範圍**：TimeFrontier / IndustryTime 綁定玩家解鎖世代；線上怠速節流；離線 allowance 對齊；load soft-repair 救過熱存檔
- **相關**：`docs/superpowers/specs/2026-07-12-long-term-progression-core-design.md` §8–9；`internal/sim/sim.go`、`frontier.go`、`internal/tui/settle.go`、`internal/store`

---

## 1. 問題

線上 `Tick` 讓 `economyDT == industryDT` 全速壓縮（`RealSecCompression = 14400`）。掛機時：

- `IndustryTime` / `TimeFrontier` 無上限超車玩家 `MaxUnlockedGen`
- 對手硬地板 `0.85 × GlobalFrontier` 被時間前線抬高
- 玩家 Gen6+ 解鎖仍卡在 Frontier R&D／工時

離線已有 8h + 下一個 baseline 殘差 cap；**線上沒有對等政策**。實測存檔可達 Industry ~40k 天（≈ Gen18 時間尺度）而仍停在 Gen5。

---

## 2. 目標

| 面向 | 目標 |
|------|------|
| 時間前線 | 最多超前玩家 **1 代**（`MaxUnlockedGen + 1` 的 `TimeBaselineDay`） |
| IndustryTime | 不得超過上述 cap；到頂則 `industryDT = 0`，解鎖後 cap 上移再開 |
| 線上掛機 | 無 Frontier 且無訓練時 industry 以 **0.15×** 推進（仍受 cap） |
| 離線 | 既有 8h／elapsed 規則外，再 min「到 cap 的殘差」 |
| 舊存檔 | Load 時 soft-repair clamp IndustryTime + rival band reclamp（**不升 schema**） |

**非目標**：改 `CompetitorCatchupRate`、市佔公式、Gen `QualityScale`、大 UI、升 `schemaVersion`。

---

## 3. 常數（`balance`）

| 常數 | 值 | 說明 |
|------|-----|------|
| `IndustryPlayerLeadGens` | `1` | TimeFrontier / IndustryTime 上限世代 = `MaxUnlockedGen + Lead` |
| `IndustryIdleMult` | `0.15` | 無 active Frontier 且無訓練時的 industry 倍率 |

兩者放在 `balance` package，與 `RealSecCompression` 同層，方便調參與測試。

---

## 4. 核心 API（`sim`）

### 4.1 產業 cap

```text
capGen = max(1, MaxUnlockedGen(s,b) + IndustryPlayerLeadGens)
capDay = Generation(capGen).TimeBaselineDay   // 解析失敗 → 不封頂（保底）
capSec = capDay * 86400
```

`IndustryTimeCapSec(s, b) float64`：回傳 cap 秒；無法解析時回傳 `+Inf` 語意（實作可用極大值或負數約定「無 cap」——本設計採：**解析失敗則不限制**，回傳 `math.MaxFloat64`）。

`IndustryTimeResidualToCap(s, b) float64`：`max(0, capSec - IndustryTime)`。

### 4.2 有效產業日（TimeFrontier）

`TimeFrontier` 改為用：

```text
effectiveDay = min(IndustryTime/86400, capDay)
```

再丟進既有 `interpolatedQualityScale(effectiveDay)`。  
**不改** `PlayerFrontier` / `GlobalFrontier = max(player, time)` 結構。

說明用的 `equivalentGenFromFrontier` 自動受益（時間維不再虛高）。

### 4.3 有效 industryDT

`EffectiveIndustryDT(s, economyDT, b) float64`：

1. `economyDT <= 0` → `0`
2. `residual = IndustryTimeResidualToCap`；`residual == 0` → `0`
3. `dt = economyDT`
4. 若 `!Frontier.Active && !HasTraining` → `dt *= IndustryIdleMult`
5. `return min(dt, residual)`

### 4.4 Tick 路徑

```text
// 線上
func Tick(...) {
  return tickWithClocks(s, dt, EffectiveIndustryDT(s, dt, b), events, b)
}

// 離線（既有）
OfflineTick(s, economyDT, industryDT, ...)  // industryDT 由 Settle 算好後傳入
// tickWithClocks 內 IndustryTime += industryDT 後，再 clamp 一次到 cap（防禦性）
```

`tickWithClocks` 結尾（或加 industry 後）防禦：

```text
if IndustryTime > capSec { IndustryTime = capSec }
```

確保任何 caller 傳入過大 `industryDT` 也不破 invariant。

### 4.5 離線 allowance

`offlineIndustryAllowance`：

```text
out = min(
  elapsed * RealSecCompression,
  8h * RealSecCompression,
  SecondsUntilNextTimeGeneration(s, b),   // 既有：下一個 catalog baseline 殘差
  IndustryTimeResidualToCap(s, b),          // 新增：玩家 cap 殘差
)
```

當 Industry 已在 cap 上，離線 industry 增量為 0；經濟 clock 仍可走滿。

---

## 5. 存檔 soft-repair（`store`）

**不升** `CurrentSchemaVersion`（仍為 2）。

在 `LoadWithConfig` 的 current-schema 路徑與 migrate 成功後路徑，統一呼叫：

```text
s = sim.ClampIndustryToPlayerCap(s, b)
```

`ClampIndustryToPlayerCap`：

1. 若 `IndustryTime > capSec` → 設為 `capSec`
2. 對所有 competitor 依新 `GlobalFrontier` 做 band clamp（85%–115%）  
   - 實作：export 或包一層既有 `clampAllRivalsToBand` / 等價邏輯  
3. Pure：回傳新 state

Schema 0/1 migrate 完成後也跑一次，避免 migrate 後仍過熱。

**不重寫** 玩家模型 quality（絕對值保留）；僅對手被前線下降拉回 band。

---

## 6. 行為矩陣

| 狀態 | industry 推進 |
|------|----------------|
| 線上 + Frontier 進行中 + 未到 cap | 全速 `economyDT` |
| 線上 + 訓練中 + 未到 cap | 全速 |
| 線上 + 兩者皆無 + 未到 cap | `0.15 × economyDT` |
| 任一時鐘已到 cap | `0`（直到 `MaxUnlockedGen` 上升） |
| 離線 | min(既有三項, residual-to-cap) |
| Load 過熱存檔 | IndustryTime clamp + rival reclamp |

---

## 7. 玩家體感（本機 Gen5 過熱例）

| 項目 | 改前 | 改後（Gen5） |
|------|------|----------------|
| IndustryTime | ~40,500 天 | clamp → Gen6 baseline **10,000** 天 |
| TimeFrontier | ~71（Gen18 尺度） | ≤ Gen6 尺度（QualityScale 120 → TF ≈ 8×120/25 ≈ **38.4**） |
| 對手 appeal | ~95–110 | band 下降後明顯接近 Gen5 旗艦 |
| 再掛機無 frontier | 產業仍狂奔 | 0.15× 且到 10k 天即停 |

---

## 8. 測試計畫（TDD）

| 測試 | 斷言 |
|------|------|
| `TestTimeFrontierCappedByPlayerLead` | IndustryTime 極大、MaxUnlockedGen=5 → TF 等於 capDay（Gen6 baseline）插值結果 |
| `TestTimeFrontierBelowCapUnchanged` | Industry 低於 cap 時與舊插值一致 |
| `TestEffectiveIndustryDTIdleMult` | 無 frontier/訓練 → 0.15× |
| `TestEffectiveIndustryDTEngagedFull` | Frontier 或 Training → 1.0×（未到 cap） |
| `TestEffectiveIndustryDTAtCapZero` | residual=0 → 0 |
| `TestTickUsesEffectiveIndustryDT` | 線上 idle Tick：IndustryTime 增量 = 0.15×dt |
| `TestTickWithClocksStillHonorsExplicitIndustryDT` | 直接 `tickWithClocks` 仍可測雙鐘（回歸） |
| `TestIndustryTimeClampedInTickWithClocks` | 過大 industryDT 後 IndustryTime ≤ cap |
| `TestOfflineAllowanceIncludesPlayerCap` | 已在 cap 時 allowance=0 |
| `TestClampIndustryToPlayerCapOnLoad` | 過熱 IndustryTime + 過高 rival Q → load 後 clamp |
| 既有 | `longrun`、`settle`、`frontier` 測試保持綠（必要時補 MaxUnlockedGen） |

---

## 9. 檔案觸及

| 檔案 | 變更 |
|------|------|
| `internal/balance/balance.go` | 新增兩常數（或獨立小檔） |
| `internal/sim/frontier.go` | TimeFrontier 用 effective day |
| `internal/sim/sim.go` | Tick → EffectiveIndustryDT；防禦 clamp |
| `internal/sim/industry_clock.go`（新） | Cap / Residual / EffectiveIndustryDT / ClampIndustryToPlayerCap |
| `internal/sim/*_test.go` | 上表測試 |
| `internal/tui/settle.go` | allowance min residual-to-cap |
| `internal/tui/settle_test.go` | 對應案例 |
| `internal/store/store.go` | Load 呼叫 clamp |
| `internal/store/*_test.go` | load 過熱案例 |
| 若 `clampAllRivalsToBand` 未 export | 在 `industry_clock.go` 呼叫 package-private 或 export 最小 surface |

---

## 10. 實作順序

1. balance 常數  
2. `IndustryTimeCapSec` / Residual / EffectiveIndustryDT + 單元測試（RED→GREEN）  
3. TimeFrontier effective day + 測試  
4. Tick + tickWithClocks 防禦 clamp + 測試  
5. Settle allowance + 測試  
6. ClampIndustryToPlayerCap + store Load + 測試  
7. 全套 `go test ./...`

---

## 11. 風險與取捨

| 風險 | 緩解 |
|------|------|
| 認真玩時產業「停住」感 | 有 frontier/訓練仍全速；cap 僅 +1 代，解鎖即開 |
| longrun 依賴 IndustryTime | longrun 以 GameTime／解鎖為主；必要時補初始 IndustryTime |
| 對手被 clamp 後市佔跳變 | 預期且為修復；不 bank 舊過熱優勢 |
| idle 0.15 仍能爬到 cap | 可接受；硬 cap 才是安全網 |

---

## 12. 验收标准

1. Gen5 玩家無論 IndustryTime 存成多少，TimeFrontier ≤ Gen6 baseline 對應尺度。  
2. 線上純掛機 industry 速率為全速的 15%，且到 cap 後為 0。  
3. 離線不會把 IndustryTime 推過玩家 cap。  
4. 本機過熱 save load 後 IndustryTime ≤ cap，對手在 band 內，消費者市佔不再系統性墊底（若玩家模型已接近 Gen5 滿配）。  
5. `go test ./...` 全綠。
