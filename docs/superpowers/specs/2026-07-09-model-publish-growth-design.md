# Model Publish Flow & Gradual User Growth — Design

- **日期**：2026-07-09
- **狀態**：設計已確認，待寫實作計畫
- **範圍**：訓練完成 → 待發佈草稿 → 命名 + 定價後發佈；用戶逐漸成長；Gen1 封頂上修
- **相關**：`docs/superpowers/specs/2026-07-07-tokensmith-design.md` §6.4；現況 `internal/sim` / `internal/tui` / `internal/balance`

---

## 1. 問題

1. **訓練完成即自動上線**：無發佈儀式、不可命名，缺少產品 launch 感。
2. **用戶瞬滿**：`advanceUsers` 用 Euler `users += (target−users)×rate×dt`，且 `rate×dt` 在 TUI/離線常見 `dt=3600` 時被 clamp 成 1 → 一步貼齊目標。
3. **封頂過低**：Gen1 中度市佔均衡約 **1–2k**，手感不像「產品起來了」。

## 2. 目標手感

| 面向 | 目標 |
|---|---|
| 流程 | 訓練完成 → **草稿（待發佈）** → 玩家在模型頁 **命名 + 定價 + 發佈** 後才吸用戶 |
| 草稿期間 | **可繼續訓練**下一台（草稿不佔訓練槽） |
| 成長節奏 | 約 **8 模擬小時**到 63% 目標；~24 模擬小時接近封頂（數小時～半天陪跑感） |
| Gen1 封頂 | 消費者、ref 定價、中度市佔 → 均衡約 **2–5 萬**（調參中位 ~3 萬） |
| 定價 | 發佈時玩家自訂；給推薦價；高低透過既有需求彈性影響目標用戶與成長 |

非目標（本版不做）：launch 行銷爆量、獨立 S 曲線、草稿刪除、重命名已上線、禁重名、重調估值里程碑。

---

## 3. 狀態機

```
TrainingJob（進行中）
    │ WorkRemaining → 0
    ▼
Draft（草稿）
    Online=false, Users=0, Name=""（可空）
    Quality / Gen / Segment / Price(暫存) 已定
    │ PublishModel{Index, Name, Price}
    ▼
Live（營運中）
    Online=true, Name=玩家命名, Price=發佈價
    │ 既有 SetPrice / 老化 / 市佔…
    ▼
（下架等維持現狀，本設計不擴）
```

**草稿識別（v1，不加 Status enum）**：`Online==false && Users==0`。  
已下架且曾有用戶者 **不** 走 `PublishModel` 重上架（YAGNI）。

---

## 4. 資料模型

### 4.1 `Model` 新增

| 欄位 | 型別 | 說明 |
|---|---|---|
| `Name` | `string` | 玩家命名；草稿可為空；發佈後必有非空名 |

既有：`Gen`, `Segment`, `Quality`, `Users`, `Price`, `Online`。

**`Online` 語意（不變但釐清）**：僅 `Online==true` 參與市佔、營收、對手 frontier、推理負載。

### 4.2 訓練完成（改 `advanceTraining`）

完成時建立：

```text
Model{
  Gen, Segment, Quality: 依既有公式,
  Price: job.Price,   // 區隔 ref 等暫存，發佈可覆寫
  Users: 0,
  Online: false,
  Name: "",
}
```

清空 `HasTraining` / `Training` → 可立刻 `StartTraining`。

### 4.3 新指令 `PublishModel`

```text
PublishModel {
  ModelIndex int
  Name       string
  Price      float64
}
```

**驗證**：

| 條件 | 錯誤 |
|---|---|
| index 越界 | `ErrInvalidModelIndex` |
| 非草稿（`Online==true` 或 `Users!=0`） | `ErrNotDraft`（新） |
| `Name` trim 後空或 rune 長度 > 24 | `ErrInvalidName`（新） |
| `Price <= 0` | `ErrInvalidPrice` |

**成功**：寫入 `Name`、`Price`，設 `Online=true`。下一個 tick 起 `advanceUsers` 作用於該模型。

**重名**：v1 允許。

### 4.4 定價與需求（既有公式）

發佈 dialog 預填推薦價：

```text
refPrice = SegmentRefPrice[segment] × tech.RefPriceMult
```

需求：

```text
demandMult = (refPrice / price) ^ PriceElasticity   // 預設 1.5
target = appeal × SegmentTargetScale[seg] × demandMult × share
         × marketingMult × tech.UserGrowthMult × star.UserGrowthMult
```

| 定價 vs 推薦 | 效果 |
|---|---|
| 較低 | 目標用戶↑、爬坡絕對量較大、單客營收↓ |
| 等於 | 基準 |
| 較高 | 目標用戶↓、爬坡慢、單客營收↑ |

訓練 job 的 `Price` 僅作預設來源；**以發佈當下 `PublishModel.Price` 為準**。上線後仍可用既有 `SetPrice`。

---

## 5. 用戶成長（改公式 + 調參）

### 5.1 指數解（取代 Euler + clamp-to-1）

```text
users' = target + (users − target) × exp(−UserGrowthRate × dt)
```

- `users < 0` 仍 clamp 到 0。  
- 小 `dt` 與舊 Euler 接近；大 `dt`（TUI `tickDT=3600`、離線 chunk）平滑逼近，**不再一步貼齊**。

### 5.2 Balance 初值

| 參數 | 現值 | 新值 | 理由 |
|---|---|---|---|
| `UserGrowthRate` | `0.001` | **`3.5e-5`**（≈ `1/(8×3600)`） | ~8 模擬小時到 63% |
| `SegmentTargetScale` 消費者 | `1000` | **`20000`** | appeal≈10 × share≈0.15 → ~3 萬 |
| 企業 / 開發者 | `500` / `800` | **`10000` / `16000`** | 維持相對比例 |
| `UserTargetPerAppeal` | `1000` | **`20000`** | 與消費者 scale 鏡射（既有測試慣例） |
| `PriceElasticity` | `1.5` | 不變 | |

**驗算（Gen1 能力向、ref、share 15%）：**  
`10 × 20000 × 1.0 × 0.15 ≈ 30,000`。

**知悉**：用戶量級上修會拉高早期估值；本版 **不** 重調 `RevenueMultiple` / 里程碑。若 $1M 過鬆，另開 balance patch。

### 5.3 時間尺度說明

- 離線結算：`elapsed` 為真實秒 → 陪跑數小時應看到明顯成長。  
- TUI 開著：每 250ms 推進 1 模擬小時（既有加速）；新 rate 下約 **數秒真實時間可見爬坡**（約 8 tick 到 63%），而非單 tick 瞬滿。

---

## 6. TUI

### 6.1 模型頁

分兩區：

1. **待發佈**：`Online==false && Users==0`  
2. **營運中**：`Online==true`  

快捷鍵：

| 鍵 | 行為 |
|---|---|
| `↑↓` | 在可選列表游標移動（含草稿） |
| `p` | 對選中**草稿**開發佈 dialog |
| `t` | 訓練（既有） |
| `$` | 對選中**已上線**模型開最小改價 dialog（若尚無改價 UI 則本 feature 一併補） |
| `Tab` | 切頁 |

有草稿時總覽或模型頁提示：`有 N 個模型待發佈`。

營運中列表名稱優先：`「{Name}」 Gen{n} · …`；`Name==""` 的舊存檔顯示 `（未命名）`。

### 6.2 發佈 dialog

```
發佈模型
  Gen1 · 消費者 · 能力 … / 成本 … / …

  名稱  {可編輯，預設 Gen{n}-{區隔短名}}
  定價  ${n}   （推薦 ${ref}）

  預估  需求 ×{demandMult} · 封頂用戶 ~{estTarget}

[←→] 調價（±1，Shift ±5）  文字鍵編輯名稱  [Backspace] 刪字
[Enter] 發佈  [Esc] 取消
```

- 預估封頂：用當前 state + 輸入價呼叫 **純函式**（建議 `sim.EstimateUserTarget(state, modelIndex, price, balance)`），與 `advanceUsers` 同一套 appeal/share 邏輯。  
- 名稱：v1 簡單 line editor（可印字元 + backspace）；長度 1–24 rune。  
- Esc：不發佈，草稿保留。

### 6.3 訓練完成回饋

- **不**強制彈 dialog（可囤草稿）。  
- 總覽：訓練區改 `訓練完成 · 待發佈`。  
- 離線 banner：若本段有訓練完成 → 提示至模型頁發佈。  
- 發佈成功 banner：`「{Name}」已上線`。

---

## 7. 存檔相容

- JSON 新增 `name`；缺失 → `""`。  
- 舊檔 `Online==true`：視為已發佈，不強迫重走流程；列表可顯示 `（未命名）`。  
- 舊檔若出現 `Online==false && Users==0`：當草稿。  
- 無需獨立 migration。

---

## 8. 錯誤與邊界

| 情況 | 行為 |
|---|---|
| 已上線按發佈 | 拒 / 提示已發佈 |
| 名稱非法 | 拒 |
| Price ≤ 0 | 拒 |
| 無草稿按 `p` | 提示先訓練或選草稿 |
| 多草稿 | 游標選一個再 `p` |
| 草稿無限囤 | v1 允許 |

---

## 9. 測試策略

### Sim / balance

1. 訓練完成 → 模型 `Online==false`、`Users==0`；`HasTraining==false` 可再開訓。  
2. `PublishModel` 成功 → `Name`/`Price`/`Online` 正確。  
3. 非法 Publish（非草稿、空名、壞價）→ 對應 error，state 不變。  
4. 發佈後 1 模擬小時：`0 < Users < 0.25×target`（約）。  
5. 發佈後 8 模擬小時：`Users` 約在 `0.50–0.75×target`。  
6. 同模型低價 target / 路徑用戶 > 高價。  
7. 大 `dt`（3600）單 tick **不**瞬滿 target。

### TUI

1. 模型頁區分待發佈 / 營運中。  
2. 發佈 dialog 預填推薦價；confirm 送出正確 `PublishModel`。  
3. 渲染含名稱。

### 手動 smoke

訓 → 待發佈 → 命名定價 → 觀察用戶數逐 tick 上升；低價 vs 高價封頂差異。

---

## 10. 實作切分（建議 PR 順序）

1. **`model`**：`Name`；`PublishModel`；錯誤 sentinel 可放 `sim`。  
2. **`sim`**：草稿化 `advanceTraining`；`Apply(PublishModel)`；指數 `advanceUsers`；`EstimateUserTarget`。  
3. **`balance`**：三個數值 + 單元測試更新。  
4. **`tui`**：列表、發佈 dialog、提示、可選 `$` 改價。  
5. **`go test ./...`** 全綠。

---

## 11. 風險

| 風險 | 緩解 |
|---|---|
| 用戶↑ → 早期估值過高 | 本版接受；必要時 follow-up 調 `RevenueMultiple` |
| 推理負載隨用戶放大 | 既有 serving/churn；smoke 3 萬用戶 |
| 自由命名 TUI 手感 | 預設名 + 可編輯；實作卡住可再簡化 |

---

## 12. 決策摘要

| 決策 | 選擇 |
|---|---|
| 方案 | A：草稿 + Publish + 調成長參數 |
| 命名 / 定價時機 | **發佈時**（非開訓時） |
| 草稿時可訓練 | 是 |
| 成長曲線 | 指數貼近 target，非獨立 S 曲線 |
| 封頂 | scale 上修到 Gen1 ~2–5 萬 |
| Status enum | 不做（用 Online+Users） |
| 估值重平衡 | 不做（本版） |
