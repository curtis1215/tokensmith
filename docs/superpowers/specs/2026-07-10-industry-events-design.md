# Industry Events System — Design

**Date:** 2026-07-10
**Status:** Approved (brainstorming), pending spec review
**Parent spec:** `2026-07-07-tokensmith-design.md` §8.5（機制）＋ §17.2（事件池內容）

## 1. Overview & Goals

把主 spec §8.5/§17.2 的產業事件系統接進現有架構：定期發生 AI 產業事件，對經濟系統有實質影響，部分帶玩家選擇。內容層（事件、觸發、效果、選項）主 spec 已定；本文件補齊 **wiring 層**——隨機性如何進純 sim、暫時性修飾符、待決選擇語意、TUI 呈現。

Goals:
- **中後期節奏**：打破放置期的單調，事件帶來波動與決策點。
- **對齊安全分支有實質價值**：事故機率吃 `TechEffects.IncidentMult`（`internal/model/types.go` 已預留）。
- **陪跑成立**：離線期間事件照發生，回來看摘要；玩家不因離線被懲罰。

Approved decisions (brainstorming):
1. **範圍**：精選 10 個事件，只收現有 sim 有直接掛鉤者；信任度、聲望、晶片 lead time、明星挖角等缺維度內容砍掉或以現有旋鈕近似。
2. **選擇語意**：遊戲照跑，待決事件掛在事件列；超過決策期限自動套用**不花現金**的保守預設選項；離線結算同規則。
3. **TUI**：總覽頁「產業動態」卡 + `e` 鍵選擇對話框；不加第七頁。
4. **架構**：seeded 確定性 RNG 放進 `GameState`（方案 A）；事件決策全在 sim 內。

## 2. Data Model（`internal/model`）

沿用現有 neutral-mult 慣例（比照 `TechEffects` / `StarEffects`）：

```go
// EventEffects：持續型修飾符效果，中性值全 1.0。
type EventEffects struct {
    BuildCostMult      float64 // 自建晶片/服務器成本
    PowerCostMult      float64 // 電價（ElectricityPerKWSec）
    RefPriceMult       float64 // 付費意願（RefPrice / SegmentRefPrice）
    UserGrowthMult     float64 // 用戶成長
    TechCostMult       float64 // 科技解鎖成本（配 Target 只作用單一分支）
    TAMMult            float64 // 市場規模（SegmentTargetScale）
    ValuationMult      float64 // 估值倍數
    SafetyWeightMult   float64 // 吸引力計算中安全維度的權重放大（玩家與對手一體適用）
    IncidentChanceMult float64 // 事故事件觸發權重（低調處理的後遺症）
}

func NeutralEventEffects() EventEffects // 全 1.0

type ActiveModifier struct {
    EventID   string
    ExpiresAt float64 // GameTime 秒；到期自動移除
    Target    int     // 事件目標（科技分支 / 對手 / 區隔 index；-1 無目標）
    Effects   EventEffects
}

type PendingEvent struct {
    EventID  string
    Target   int
    FiredAt  float64 // GameTime
    Deadline float64 // 超時自動套用目錄的 DefaultChoice
}

type EventRecord struct { // 事件歷史，環形保留最近 20 條
    EventID string
    At      float64
    Choice  int  // 決議的選項 index；無選項事件為 0
    Auto    bool // true = 超時/離線自動決議
}

type EventsState struct {
    RandState   uint64 // splitmix64 state；new-game 時在 sim 外播種
    NextCheckAt float64
    Pending     []PendingEvent
    Active      []ActiveModifier
    Log         []EventRecord
}
```

`GameState` 新增欄位 `Events EventsState`。存檔 JSON 快照自然帶上，無 schema 遷移需求（舊檔缺欄位 → 零值，首 tick 視同新啟用）。

新命令（走既有 `sim.Apply`）：

```go
type ResolveEvent struct {
    PendingIndex int // Events.Pending 的 index
    Choice       int // 事件選項 index
}
```

驗證：index / choice 超範圍回錯誤；對應事件已被自動決議（不在 Pending）回錯誤。

## 3. Sim Wiring（`internal/sim`）

### 3.1 確定性隨機

`sim.go` 頂部契約註解由「no randomness」改為「**無非確定性隨機**——所有隨機經由 `Events.RandState`（splitmix64）推進」。性質：同 seed + 同 dt 序列 + 同命令序列 → 同結果。測試注入固定 seed；離線結算（`settle.go` ≤1h chunk 快轉）不需任何特例。

seed 來源：TUI new-game 路徑（sim 外）以 wall-clock 播種；sim 本身永不讀時鐘。

### 3.2 Tick 事件步驟（置於既有經濟步驟之前）

1. **到期清理**：移除 `ExpiresAt <= GameTime` 的 `Active`；`Deadline <= GameTime` 的 `Pending` 以 `DefaultChoice` 自動決議（套用選項效果、記 Log `Auto=true`）。
2. **觸發擲骰**：當 `GameTime >= NextCheckAt`：
   - 收集「當前可觸發」事件：gate 通過（時間、估值門檻等）、且該事件不在 `Pending` / `Active` 中（**同事件不重複觸發**）。
   - 權重可依 state 調整：事故權重 ∝ 上線模型安全低 × `IncidentMult` × `IncidentChanceMult`；泡沫論 gate `PeakValuation`；突破論文權重隨 R&D 投入升。
   - 加權挑一個或空手而回（命中率為 balance 參數）。
   - `NextCheckAt += 檢查間隔 ± jitter`（jitter 亦出自 RandState）。
3. **生效**：
   - 一次性效果（現金增減、用戶流失、對手品質變動）立即套用。
   - 持續型效果進 `Active`。
   - 帶選項者進 `Pending`（選項間的差異效果在決議時才套用）；無選項者直接記 Log。
4. **聚合**：`sim.eventEffects(state) EventEffects`（寫法比照 `star.go` 聚合）將 `Active` 折成單一效果組，乘進既有消費點：自建成本、電費、RefPrice、用戶成長、科技成本、TAM、估值、事故權重、吸引力安全權重。

### 3.3 效果套用策略

目錄數值**資料驅動**（balance），效果**套用邏輯在 sim 內按 EventID switch**——10 個事件的效果形態差異大（一次性 vs 持續、目標挑選各異），硬做全資料驅動反而繞。新增事件 = balance 加一條 spec + sim switch 加一個 case。

## 4. 事件目錄（10 個，v0）

超時預設一律為**不花現金**的選項（離線自動決議不得花玩家的錢）。★ = 預設。

| # | ID | 事件 | gate / 權重 | 效果 | 選項 |
|---|---|---|---|---|---|
| 1 | `chip-shortage` | 晶片短缺 | 隨機 | `BuildCostMult 1.18`，2–3 天 | 囤貨鎖價（花現金，免疫此次）/ ★轉租度過 |
| 2 | `energy-spike` | 能源價波動 | 隨機，骰漲跌 | `PowerCostMult 1.3` 或 `0.7`（利多，無選項） | 漲時：簽長約鎖價（花現金，鎖 1.0）/ ★觀望 |
| 3 | `rival-breakthrough` | 對手重大發表 | 挑能力維度最強對手 | 一次性該對手 `Quality[DimCapability]` +15%（rubber-band 撐住） | 限時促銷（花現金，`UserGrowthMult 1.25` 一段時間）/ ★觀望 |
| 4 | `open-source-war` | 開源價格戰 | 隨機 | `RefPriceMult 0.8` 一段時間 | 跟進降價（改吃 `RefPriceMult 0.75` + `UserGrowthMult 1.2`）/ ★守高階 |
| 5 | `rival-scandal` | 對手安全爭議 | 以 (1−對手安全) 加權挑目標 | 一次性該對手 `Quality[DimSafety]` −20% | 花錢搶客（花現金，`UserGrowthMult 1.3`）/ ★觀望（`1.1` 小加成） |
| 6 | `breakthrough-paper` | 突破論文 | 權重隨 R&D 投入升；隨機挑分支（Target） | —（效果全在選項） | 押注加碼（花 R&D，該分支 `TechCostMult 0.5`）/ ★常規吸收（`0.7`） |
| 7 | `model-incident` | 你的模型出事 | 權重 ∝ 上線模型安全低 × `IncidentMult` × `IncidentChanceMult` | 一次性上線模型用戶 −8%（企業段 −15%） | 公開道歉（花現金，流失補回一半、無後遺症）/ ★低調（`IncidentChanceMult 1.5` 一段時間） |
| 8 | `regulation` | AI 監管新法 | 時間 gate（中後期） | 期間 `SafetyWeightMult 1.5` | 投資合規（花現金，你全模型 `Quality[DimSafety]` +10% 一次性）/ ★硬扛 |
| 9 | `market-cycle` | 市場榮枯 | 隨機週期，骰方向 | `TAMMult 1.25` 或 `0.8`，較長時段 | 無選項（宏觀） |
| 10 | `bubble-talk` | AI 泡沫論 | gate：`PeakValuation` 達標 | `ValuationMult 0.75` 一段時間 | 釋出實績穩信心（花現金，改吃 `0.9`）/ ★觀望 |

主 spec §17.2 中砍掉 / 延後者：新製程上市（科技樹已涵蓋解鎖）、人才流動潮（需挖角機制）、對手融資合併（純壓力預警，價值低）、法規調查（區隔暫停過重）、開源框架釋出（與 #4/#6 重疊）。

## 5. Balance（`internal/balance`）

`Config` 新增：

```go
type EventChoice struct {
    Label    string  // TUI 顯示
    CashCost float64 // 一次性現金成本（0 = 免費）
    RnDCost  float64 // 一次性 R&D 成本（僅 #6）
    // 其餘差異效果數值以事件別欄位表達（duration、mult 等）
}

type EventSpec struct {
    ID            string
    Weight        float64 // 基礎觸發權重
    MinGameTime   float64 // 時間 gate；0 = 無
    MinValuation  float64 // 估值 gate；0 = 無
    DurationSec   float64 // 持續型效果長度（骰範圍以 ±jitter 實現）
    DeadlineSec   float64 // 決策期限
    DefaultChoice int
    Choices       []EventChoice
    // 效果數值欄位（各 mult、一次性百分比）依事件語意命名
}

Events            []EventSpec
EventCheckSec     float64 // 觸發擲骰間隔（均值）
EventHitChance    float64 // 每次擲骰命中率
EventLogCap       int     // 歷史保留條數
```

v0 初標（待 playtest）：`EventCheckSec = 6 sim 小時 ± jitter`、`EventHitChance` 調到平均每天 1–2 起、`DeadlineSec = 24 sim 小時`、持續型 2–4 天、`EventLogCap = 20`。現金成本以當前規模比例計（如月營收的百分比，floor 保底），避免後期事件變成零感知。

## 6. TUI（`internal/tui`）

- **總覽頁**新增「產業動態」卡：最近 4 條 Log（相對時間 + 事件標題 + 決議結果），有待決事件時高亮標題 + 決策倒數。
- **`e` 鍵**（總覽頁）開 `dialog_event.go`：顯示最舊待決事件的敘述、兩選項的效果說明與成本；選定送 `ResolveEvent` 走 `Apply`；多個待決逐一處理。鍵位沿用現有 dialog 慣例（`dialog_publish.go` 模式）。
- **觸發當下**：`setNotice` + `PulseNotice` 提示（事件標題一行）。
- **離線結算**：`settle.go` 的 `Summary` 加事件計數，歡迎回來橫幅顯示「離線期間發生 N 起事件（M 起已自動決議）」；細節看動態卡 Log。
- 事件中文名 / 敘述 / 選項文案放 TUI 層 meta map（比照 `tech_meta.go`），sim/balance 只認 ID。

## 7. 錯誤處理與邊界

- `ResolveEvent`：index / choice 範圍驗證；已自動決議回錯誤（TUI 顯示 notice）。
- 目錄查無 EventID（存檔與 balance 版本漂移）：跳過該記錄，不 panic。
- `Log` 超過 cap 丟最舊。
- 同事件在 `Pending` 或 `Active` 期間不重複觸發。
- 現金不足以支付選項成本：`Apply` 回錯誤（比照既有 Hire/Build 慣例），TUI 提示。
- 舊存檔 `EventsState` 零值：首 tick `NextCheckAt` 從當前 GameTime 起排，`RandState` 為 0 時由 TUI 載檔路徑補播種。

## 8. 測試策略

- **sim 表格測試**（固定 seed）：觸發擲骰命中 / 空手、gate 過濾、同事件不重複、超時自動決議走預設、修飾符到期移除、`eventEffects` 聚合值。
- **每事件單元測試**：兩選項各自的效果套用（現金扣除、mult 生效、一次性變動）。
- **settle 整合測試**：跨 chunk 快轉觸發事件 + 自動決議 + Summary 計數正確。
- **TUI 測試**：dialog 開關、選擇送命令、notice 顯示（比照 `dialog_publish_test.go`）。
- **確定性測試**：同 seed 同輸入雙跑結果相等。
