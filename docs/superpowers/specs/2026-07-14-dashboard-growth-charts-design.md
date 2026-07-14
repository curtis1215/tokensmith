# 儀表板增長圖表 — Design

- **日期**：2026-07-14
- **狀態**：設計已確認，待寫實作計畫
- **範圍**：新增 TUI「儀表板」頁；用戶／營收／R&D 增長線圖；混合時間窗（session 短窗 + 真實日曆長窗）；R&D 庫存主線 + 來源流入細分
- **相關**：
  - `docs/superpowers/specs/2026-07-09-tui-dashboard-redesign-design.md`（六頁儀表板殼）
  - `docs/superpowers/specs/2026-07-11-tui-warroom-gamification-design.md`（sparkline、HUD 色、零新依賴）
  - `docs/superpowers/specs/2026-07-12-daily-agent-token-stats-design.md`（真實日鍵、sidecar 模式）
  - `docs/superpowers/specs/2026-07-13-overview-warroom-layout-design.md`（總覽精簡、八頁導覽現況）
  - 現況：`internal/tui/spark.go`、`page_overview.go`、`display.go`、`tui.go`；`internal/dailyusage`；`internal/store`

---

## 1. 問題

總覽「公司」卡只有一行估值 spark；用戶與 R&D 雖有 session 取樣，幾乎沒有可讀的增長敘事。玩家無法回答：

- 用戶這幾分鐘／這幾天怎麼長？
- 月營收走勢如何？
- R&D 庫存是在囤還是在燒？誰（Claude／Codex／員工…）在餵入？

需要一頁專門的增長儀表板，而不是再把總覽堆高。

---

## 2. 已確認決策

| 項 | 選擇 |
|---|---|
| 放置 | **新頁「儀表板」**（非總覽擴充、非戰情室區塊） |
| 導覽位置 | **總覽後為鍵 `2`**；戰情室起頁碼 +1 |
| 時間模型 | **混合**：短窗即時 sample + 長窗 **真實日曆日** 快照 |
| 指標語意 | **存量快照**：`TotalUsers`、`MonthlyRevenue`、`Resources.RnD` |
| R&D 細分 | **主線庫存** + **各來源流入**（token 工具 + staff）；非庫存拆帳 |
| 實作路線 | **方案 2**：sidecar 日序列 + 多列 Unicode 線圖；不併入 dailyusage、不塞 GameState |
| 依賴 | **零新依賴**（lipgloss + Unicode） |
| sim 邊界 | 只讀 sim 視圖／既有入帳結果；不改經濟公式 |

---

## 3. 目標與非目標

### 目標

1. 導覽新增「儀表板」，鍵 `2` 可進；一屏（可 scroll）見三張增長圖。
2. 短窗：TUI 開啟期間高頻 sample，掛機可見平滑走勢。
3. 長窗：本地日一點，寫入 sidecar；重開存檔後仍可見歷史。
4. R&D 卡：庫存曲線 + 來源流入（claude-code / codex / grok / opencode / staff）。
5. 與資源列 token→R&D 數字 **同一歸因公式**（含 streak / prestige / HQ mult）。

### 非目標

- 改 tick／經濟／估值／用戶增長公式
- 圖表庫、web dashboard、Grafana
- 遊戲 Day 作為長窗軸（本輪固定真實日曆）
- 可點選 zoom、任意時間篩選、匯出 CSV
- 把 KPI 塞進 `daily-usage.json` 或 `GameState`
- 重做總覽為圖表主場（總覽迷你 spark 可保留不動）
- 拆解 R&D **庫存**所有權（混池不可還原）

---

## 4. 導覽與職責

### 九頁導覽

| 鍵 | 頁 |
|---|---|
| 1 | 總覽 |
| **2** | **儀表板（新）** |
| 3 | 戰情室（原 2） |
| 4 | 模型（原 3） |
| 5 | 市場（原 4） |
| 6 | 算力（原 5） |
| 7 | 團隊（原 6） |
| 8 | 科技（原 7） |
| 9 | 成就（原 8） |

- `numPages = 9`
- 頁常數：`PageOverview=0, PageDashboard=1, PageWarRoom=2, … PageAchievements=8`
- Tab 列、數字鍵 `1`–`9`、左右切頁皆跟 `pageNames`
- 儀表板為 **純檢視**；無獨佔快捷鍵；footer：`[Tab]切頁 [q]離開`

### 職責邊界

| 層 | 負責 | 不負責 |
|---|---|---|
| **TUI 短窗 rings** | 高頻 sample、儀表板即時線 | 寫入日序列檔 |
| **metrics 日序列** | 真實日 bucket、flush、prune、讀長窗 | 改 sim 經濟 |
| **R&D 流入記帳** | 當日 per-source / staff 累加 | 重複加庫存、從 inflow 扣消耗 |
| **總覽** | 維持 KPI；既有 spark 可不改 | 三張大圖 |
| **daemon** | 既有 token harvest | 不獨寫 metrics（KPI 需活 sim；由 TUI 寫） |

### 成功標準

1. 總覽按 `2` 進儀表板，三張圖標題可見。
2. 掛機數分鐘：短窗有走勢（≥2 sample 後）。
3. 跨本地日或重開：長窗仍有歷史日點（曾 flush 過的日）。
4. R&D 卡能區分庫存曲線與各來源流入。

---

## 5. 資料模型與持久化

### 5.1 原則

- 短窗：僅 TUI 記憶體；重開可清空。
- 長窗：獨立 sidecar，**不**進 `GameState`、**不**混 `daily-usage.json`。
- 日鍵：`dailyusage.DayKey(now)` → `YYYY-MM-DD`（本地日）。
- 路徑：與 save 同目錄，`metrics-history.json`  
  （例：`…/tokensmith/metrics-history.json`，與 `ledger.json` / `daily-usage.json` 並列）。

### 5.2 為何 sidecar

| 選項 | 問題 |
|---|---|
| 塞 `GameState` | 污染 sim；prestige／重開語意不清；逼升 save schema |
| 塞 `daily-usage` | token 收成帳本語意不同、耦合難測 |
| **sidecar** | 與既有 sidecar 模式一致；可 prune；TUI 專用 |

### 5.3 Schema（v1）

```json
{
  "schemaVersion": 1,
  "updatedAt": 1721000000,
  "days": {
    "2026-07-14": {
      "users": 12500,
      "monthlyRevenue": 84000,
      "rndStock": 3200,
      "rndInflow": {
        "claude-code": 1200,
        "codex": 80,
        "grok": 40,
        "opencode": 15,
        "staff": 300
      },
      "openUsers": 11000,
      "openRevenue": 70000,
      "openRnd": 2800,
      "openSet": true
    }
  }
}
```

| 欄位 | 意義 |
|---|---|
| `updatedAt` | 最近一次寫入文件的 Unix 秒（文件層） |
| `users` | 當日**最後一次** flush 的 `sim.TotalUsers` |
| `monthlyRevenue` | 當日最後一次的 `sim.MonthlyRevenue` |
| `rndStock` | 當日最後一次的 `Resources.RnD` |
| `rndInflow.*` | 當日**累加**正流入（非庫存拆帳） |
| `openUsers` / `openRevenue` / `openRnd` | 當日**第一次** snapshot 的存量（Δ今日開盤） |
| `openSet` | 是否已寫入開盤點；之後覆寫存量不改 open* |

固定來源 key（缺則視為 0）：

- token：`claude-code` / `codex` / `grok` / `opencode`（與 harvest 一致）
- 員工：`staff`

### 5.4 寫入節奏

| 事件 | 行為 |
|---|---|
| TUI tick（短窗） | push dash rings |
| 同日週期 flush（autosave、約 30s、正常退出） | upsert 當日：覆寫三存量；**不**重設 inflow |
| Token 入帳 | `rndInflow[source] +=` 實際入庫 R&D（與 `lastTokenRnD` 同公式） |
| Economy tick | `rndInflow["staff"] += RnDRatePerSec × dt`（與 sim 入庫時段一致） |
| 本地日切換 | 先 flush 舊日 → 新日 bucket（存量 snapshot；inflow 從 0） |
| 載入 | 讀 sidecar；短窗 ring 從空開始（可選 seed 當日最後點，非必須） |

### 5.5 保留與失敗

- 預設保留最近 **90** 個有資料日；更舊 prune。
- 無資料日不強制補 0；圖表只畫有點的日，X 軸標首／中／尾日期。
- 檔不存在 → 空歷史。
- 損壞 → rename `metrics-history.json.corrupt-<unix>`，重建 v1（對齊 dailyusage 風格）。
- 寫入失敗不擋遊戲；可選單次 notice。

### 5.6 短窗 memory（不入檔）

儀表板**專用** rings（避免與總覽 `sparkRnD` 速率語意混淆）：

| 序列 | 內容 | 容量 |
|---|---|---|
| `dashUsers` | 總用戶存量 | 120 點（約 2 分鐘，sample 間隔 ~1s） |
| `dashRevenue` | 月營收存量 | 同上 |
| `dashRnDStock` | R&D 庫存 | 同上 |
| 短窗流入（可選 ring 或當日累計視圖） | per-source 近窗／今日累計 | 實作可簡化為「今日累計」多線 |

既有：

- `sparkValuation`：**不動**
- `sparkUsers` / `sparkRnD`：總覽可保留；儀表板不依賴其語意

---

## 6. 取樣與 R&D 歸因

### 6.1 短窗

- 時機：display tick 路徑，約每 **4 tick（~1s）** 一點（可與現 spark 對齊）。
- 點值：存量（用戶／月營收／R&D 庫存），**不是** R&D 速率。
- 平滑：用戶可用 `m.disp.TotalUsers`；其餘 truth 或與總覽一致的 approach。

### 6.2 Token 流入（與資源列／`sim.Tick` 一致）

```
perSourceRnD = TokenRawRnD(events_of_source)
             × StreakMult × PrestigeRnDMult × TokenSkillRnDMult × OfficeTokenRnDMult
```

- `TokenSkillRnDMult`：`sim.TokenSkillRnDMult`（雇員被動 `token_rnd` 家族 skill 乘積；無技能 = 1）
- 入帳當下：`day.rndInflow[source] += perSourceRnD`
- metrics **只記帳**，不重複增加 `Resources.RnD`（sim／既有路徑已加）
- 與 `lastTokenRnD`／資源列 pulse **同一路徑**，避免雙公式漂移

### 6.3 Staff 流入

```
staffDelta = RnDRatePerSec(state, cfg) × dt_seconds
day.rndInflow["staff"] += staffDelta
```

- `dt` 對應實際讓員工 R&D 入庫的時段
- 只記 **正流入**；訓練／科技／前沿消耗只反映在 `rndStock` 曲線，**不**從 inflow 回扣

### 6.4 不單獨成線

prestige mult、HQ mult、streak — 已乘進 token／staff 數字。

### 6.5 圖例順序

`claude-code` → `codex` → `grok` → `opencode` → `staff`  
（顯示名：Claude Code / Codex / Grok / OpenCode / 員工）

---

## 7. 圖表渲染與版面

### 7.1 API（示意）

新檔 `internal/tui/linechart.go`：

| API | 行為 |
|---|---|
| `lineChart(series []float64, w, h int) string` | 高 `h` 列；min-max 正規化；`<2` 點 →「資料累積中」 |
| `multiLineChart(ordered series, w, h int) string` | 多序列共用 Y；每序列固定色 |

**v1 渲染策略**：將既有 spark 的 8 級 `▁▂▃▄▅▆▇█` **垂直堆成 h 列**（每欄一個高度），風格統一、易測。  
多線並列共用 Y 軸即可；堆疊圖可選，非 v1 硬性。

### 7.2 顏色

| 系列 | 色 |
|---|---|
| 用戶 | `hudCyan` |
| 營收 | `gainGreen` |
| R&D 庫存 | `hudPurple` |
| 來源流入 | 固定 palette，不與庫存撞色 |

### 7.3 標註

- 標題列：最新值 + **Δ今日**
  - 基準：當日 **第一次** 成功 snapshot 的存量（開盤）vs 當下；當日尚無開盤點則省略 Δ
  - 僅用於用戶／營收／R&D **庫存**；流入用「今日合計」數字，不算 Δ
- 短窗：可不標 X
- 長窗：底部少量日期（首／中／尾）
- 空資料：`尚無歷史 · 掛機或跨日後會出現`
- R&D 文案註明：庫存含消耗；流入為正入帳

### 7.4 頁面骨架（寬 ≥ 100）

```
┌─ 用戶增長 ──────────────────────────────┐
│  12.5K  (+1.2K 今日)                      │
│  [短窗線 h≈5]                             │
│  ── 近 90 日 ──                           │
│  [長窗線 h≈5]                             │
└───────────────────────────────────────────┘
┌─ 營收增長 ──────────────────────────────┐
│  月營收 …  同上雙窗                       │
└───────────────────────────────────────────┘
┌─ R&D 增長 ──────────────────────────────┐
│  庫存 …                                   │
│  [庫存短窗 + 長窗]                        │
│  流入 by 來源                             │
│  [長窗多線：每日各來源 inflow]            │
│  今日：Claude … · 員工 …（KV／色點合計）  │
└───────────────────────────────────────────┘
```

**R&D 流入呈現（釘死）**：

- **長窗**：每來源一條日序列（當日 `rndInflow[source]` 一點），`multiLineChart` 並列
- **短窗／當日**：不另建高頻多線 ring；用 **今日累計 KV／色點圖例**（讀當日 bucket）
- 堆疊圖：非 v1

- 窄屏：三卡上下堆；圖高可減到 3
- 短／長窗 **同卡分區**，不另開 tab
- 內容過高：沿用既有 viewport scroll

---

## 8. 資料流

```
tick / token harvest
        │
        ├─► sim state（庫存／用戶／營收照舊）
        │
        ├─► TUI dash rings（短窗）
        │
        └─► metrics day bucket
              ├─ upsert users / revenue / rndStock
              └─ += rndInflow[source|staff]
                    │
                    └─► metrics-history.json
                              │
                              └─► page_dashboard 長窗
```

---

## 9. 檔案地圖

| 區域 | 檔案 | 內容 |
|---|---|---|
| 新 package | `internal/metrics/` | Document、Load/Save、snapshot upsert、AddInflow、prune；DayKey 委派 dailyusage |
| 圖表 | `internal/tui/linechart.go` (+ test) | 單線／多線 |
| 頁面 | `internal/tui/page_dashboard.go` (+ test) | 三卡、雙窗、R&D 雙區 |
| 導覽 | `internal/tui/tui.go` | PageDashboard、pageNames、numPages、View、數字鍵 |
| 取樣／歸因 | `internal/tui/display.go` 或 helper | dash rings、inflow 記帳、flush |
| 路徑 | `newAtPaths` 一帶 | 掛上 metrics 路徑 |
| 測試 | 導覽／鍵位相關 | 頁碼位移斷言 |

**不動**：`internal/sim` 經濟公式、`balance` 常數、`dailyusage` schema（僅共用 DayKey）。

---

## 10. 風險與對策

| 風險 | 對策 |
|---|---|
| 頁碼 +1 弄壞快捷鍵／測試 | 集中改常數；全 repo 更新戰情室等鍵位斷言 |
| Token 歸因雙公式 | 共用 helper／同一路徑 |
| Daemon 誰寫 metrics | 僅 TUI 寫 KPI 日序列 |
| 寫檔失敗 | 不擋遊戲；記憶體 bucket 仍更新 |
| R&D 消耗造成庫存大跌 | UI 註明；流入區只顯示正流入 |
| 一屏過高 | scroll + 窄屏減高 |

---

## 11. 實作分期

| Phase | 交付 | 可玩檢查 |
|---|---|---|
| **P1** | `internal/metrics` + 導覽空頁殼 | `2` 進「儀表板」 |
| **P2** | 短窗 sample + 三張單線（用戶／營收／R&D 庫存） | 掛機見曲線 |
| **P3** | 長窗 sidecar + flush／日切／prune | 重開仍有日點 |
| **P4** | R&D 流入 by source（記帳 + 多線） | 五來源有數 |
| **P5** | Δ今日、版面打磨、測試補齊 | 窄／寬 smoke |

每 phase 可獨立合併；P1–P2 即可感受價值。

---

## 12. 測試要點

- `lineChart`：空、單點、單調遞增、全相等（無除零）
- 日切換：舊日凍結、新日 inflow 0、存量接續 snapshot
- token 入帳 → 對應 source inflow；與 `lastTokenRnD` 公式一致
- staff dt 積分
- 90 日 prune
- 導覽：`2` = 儀表板，戰情室 = `3`
- render smoke：三標題、來源圖例、空狀態文案

---

## 13. 風格與約束（繼承）

1. 零新依賴；Unicode 手繪圖表。
2. 不碰 sim 純度：metrics 與 TUI 為表現／旁路帳本。
3. HUD 色系延續 warroom gamification（cyan／purple／gain／loss）。
4. 動畫／tick 節奏維持現有 250ms；不新增獨立計時器輪詢（flush 可掛在既有 tick 計數）。
