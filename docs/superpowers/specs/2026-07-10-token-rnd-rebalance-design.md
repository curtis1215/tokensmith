# Token→R&D 手感重製 — Design

**Date:** 2026-07-10
**Status:** Approved (brainstorming), pending spec review
**Parent spec:** `2026-07-07-tokensmith-design.md` §6.2（R&D 產生：兩條進料）＋ §8.1（token 手感）

## 1. Overview & Goals

主 spec §6.2 原訂節奏是「早期靠真實 token 起步，中後期研發團隊變主幹、token 轉為衝刺加成」。實測發現這個節奏被一個未預期的單位錯配放大到失控：被動收入（研究員、明星員工）的 `RnD/tick = 產出 × dt`，而 `dt` 每 tick 固定 3600（模擬秒）、每秒 4 tick，等於被動收入額外偷吃了 **14,400 倍**的時間壓縮加成；token 收入是照 daemon 輪詢到的真實 token 數直接算，完全不吃這個加成。結果是 20 個 Tier2 研究員 = 4,320 R&D/實際秒，遠遠蓋過一次認真 coding 的 token 爆發（幾百 R&D），玩家對「真實使用驅動遊戲」這個招牌機制完全無感。

同時，token 灌入的視覺回饋本身也偏弱：只顯示原始 token 數（不是實際換到的 R&D）、來源工具（Claude Code / Codex）不分、閃現只維持 1 秒、原設計文件 §8.1 提到的連續使用 streak 加成從未實作。

本文件推翻 §6.2 的節奏設計，改為：

1. **Token 永遠是 R&D 主要來源**，不因研發團隊/明星規模而反轉。
2. **修正被動收入的單位錯配**，讓研究員/明星保有離線進度意義，但量級遠低於 token。
3. **重製 token 灌入的即時回饋**：顯示真實 R&D 增量、標明來源工具、加上連續使用 streak 加成，並延長閃現時間。

Approved decisions（brainstorming）：
- 研究員/明星維持固定加法產出（不改成「token 放大器」相乘模式），只下修數值。
- Streak 中斷即歸零（不做衰減），倍率封頂 ×1.6（10 天）。
- soft cap（`SoftCapFull=200000`／24h 模擬窗）維持不動——新數值下正常使用量級碰不到。

## 2. 比重修正（`internal/balance/balance.go`）

`ResearcherRnDPerSec` 與 `balance.go` 內明星 `RnDPerSec`（`buildStar` 系列函式）全數除以 `gameSecPerRealSec`（= `tickDT × 4` = 14400，定義於 `internal/tui/tui.go`），把單位從「隱性吃了時間壓縮的模擬秒產出」改回「每個真實秒該有多少」：

| | 現在 | 修正後 |
|---|---|---|
| Tier1 研究員 | 0.005 /模擬秒 | 0.005 / 14400 /模擬秒 |
| Tier2 研究員 | 0.015 /模擬秒 | 0.015 / 14400 /模擬秒 |
| Tier3 研究員 | 0.04 /模擬秒 | 0.04 / 14400 /模擬秒 |
| 明星（範例 300/400） | 300 / 400 /模擬秒 | 300/14400、400/14400 /模擬秒 |

換算後 20 個 Tier2 研究員 ≈ 0.3 R&D/實際秒（離線一小時仍有約 1,080 R&D 進度），明星等比例縮小。`gameSecPerRealSec` 目前是 `tui` package 私有常數，需要 export（或在 `balance.Config` 建構時作為參數傳入）供 `balance.Default()` 或呼叫端使用——採用哪種視 plan 階段決定，設計層面只要求「新常數必須是舊常數的可推導縮放，不是憑空調的魔術數字」，方便未來 tickDT 調整時两邊連動。

Token 端公式（`tokenRawRnD` / `applySoftCap`，`internal/sim/sim.go`）不變。

## 3. Ledger 資料流：per-source 累計（`internal/ledger`, `internal/daemon`）

`ledger.Ledger` 的 `CumIn/CumOut` 拆成 per-source 累計：

```go
type SourceTotals struct {
    CumIn  int
    CumOut int
}

type Ledger struct {
    Sources   map[string]SourceTotals `json:"sources"` // key: TokenEvent.Source ("claude-code" / "codex")
    UpdatedAt int64                   `json:"updatedAt"`
    Cursors   []ingest.CursorState    `json:"cursors,omitempty"`
}
```

`daemon.Harvester.Step()` 依 `TokenEvent.Source` 分別加總進對應的 `SourceTotals`，不再合併成單一總數。JSON 新結構對舊 ledger 檔案不相容（舊檔沒有 `sources` 欄位）——daemon 重啟時讀到舊格式視同空 ledger 起算，不影響正確性（`cursors` 仍在，不會重複計算歷史 token，只是短暫損失累計顯示，可接受）。

## 4. TUI 消費：per-source 事件與即時 R&D 換算（`internal/tui/tui.go`）

`pollTokens()`（daemon 模式）對每個 `Sources` 分別算 delta，有變動的來源各自組一筆 `model.TokenEvent{Source, InputTokens, OutputTokens}`，維持現有回傳 `[]TokenEvent` 的介面（獨立模式本來就是逐筆帶 Source，行為對齊）。

顯示層在既有 pulse 機制（`display.go` `PulseToken`）之上，改算「這個 tick 每個來源各自換到多少 R&D」（用 `(input×TokenInputWeight + output×TokenOutputWeight) / TokenDivisor`，跟 sim 內部同一條公式，避免顯示跟實際加值兜不起來），狀態列格式：

```
⚡ Claude Code +842 R&D   🔥連續3天 ×1.18
```

兩個來源同 tick 都有變動則各佔一段、以空格分隔。Pulse 持續時間從 4 tick（~1s）延長到約 12 tick（~3s），淡出而非硬消失（沿用 `display.go` 現有 `pulseT` 倒數邏輯，只調初始值與尾端淡出處理）。

## 5. Streak（`internal/store/meta.go` + TUI 層）

`Meta` 新增：

```go
type Meta struct {
    ConsumedIn   int
    ConsumedOut  int
    LastRealUnix int64
    LastActiveDate string // "2026-07-10"，本機時區，YAGNI：不存 UTC 額外欄位
    StreakDays     int
}
```

只在偵測到「這個 tick 真的有 token 進帳」時才檢查/更新（不是每 tick 都算日期）：
- 今天日期已等於 `LastActiveDate` → 不動。
- 昨天 == `LastActiveDate` → `StreakDays++`。
- 其他（含首次、或中斷過）→ `StreakDays = 1`。
- 更新 `LastActiveDate = 今天`。

倍率 `streakMult = 1 + 0.06 × min(StreakDays, 10)`（5 天 ×1.3 對齊原設計文件範例，10 天封頂 ×1.6）。

`streakMult` 只作用在 token 換算出的 R&D，不影響研究員/明星那條，套用點在 `sim.Tick()` 現有的 `ns.Resources.RnD += (staffRnD + tokenRnD + starRnD) * pe.RnDMult` 那行，改成 `tokenRnD*streakMult` 那一項單獨吃倍率。`streakMult` 由 TUI 層算好（讀 wall-clock 日期），當一般 float64 參數傳入 `Tick()`——**`sim` package 本身不讀時間**，維持純函式、確定性、可測試的既有原則（跟現有離線結算在 TUI 層讀 wall-clock 是同一套模式）。

離線結算（`Settle()`，`internal/tui/settle.go`）呼叫 `sim.Tick` 走同一個新簽名；離線期間的 streak 用「回來的當下日期」算一次，不逐日回溯模擬（YAGNI——離線結算本來就是整段快轉，逐日 streak 精算對這個小遊戲沒有實質意義）。

## 6. Edge Cases

- **`Sources` map 裡出現未知 key**（未來新 adapter）：顯示層對未知 source 用 `Source` 原字串直接顯示，不特別擋。
- **同一秒兩個來源都有數據**：各自獨立算、獨立顯示，不合併（用意是玩家能分清是 Claude 還是 Codex 在灌）。
- **`StreakDays` 溢位/長期不玩後回來**：中斷判斷只看「今天」與「LastActiveDate」的日曆差，不管中斷多久都是重置為 1，邏輯不需要特殊處理超長間隔。
- **舊存檔沒有 `LastActiveDate`/`StreakDays`**：零值視同「尚未有 streak」，第一次進帳直接記為 `StreakDays=1`，不補算歷史。

## 7. Testing

- `balance_test.go`：新增/調整既有數值斷言（研究員、明星 RnDPerSec 新值）。
- `sim_test.go`：`Tick()` 簽名變動後的既有測試補上 `streakMult` 參數（預設 1.0 case + 一個 streak>1 的 case 驗證只放大 token 那條、不放大 staff/star）。
- `daemon_test.go` / ledger 相關 test：per-source 累計正確性、舊格式 ledger 讀取不 crash。
- `tui` 顯示相關 test：per-source pulse 訊息格式化、streak 日期進位/重置邏輯（不依賴真實 wall clock，用注入的 now）。
