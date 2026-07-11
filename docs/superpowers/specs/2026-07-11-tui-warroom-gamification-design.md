# Tokensmith「AI 作戰室」TUI 遊戲化改造設計

日期：2026-07-11
狀態：Phase 1-6 全部已實作

## 背景與目標

現況診斷（六頁全部實際 render 檢視）：

- 全遊戲只有 4 種樣式（粉 205 accent、紅 warn、faint、粗體標題），所有卡片同款圓角框，無視覺層級
- 卡片各自縮到內容寬度，右側大片空白、欄位不對齊
- 身為 idle game 卻近乎全靜態：無 sparkline、無漲跌色、進度條只有無色 `▓░`
- 幾乎所有成功時刻靜默：訓練完成、科技解鎖、里程碑達成、campaign 階段推進、決勝、勝利皆無回饋；決勝守成（ShowdownHeld）完全沒有 UI
- streak 只在 token pulse 的約 3 秒內可見

目標：**AI 作戰室 HUD** 風格——深色霍光青紫、金融終端資訊密度、綠漲紅跌；針對「掛在旁邊一直開著」的使用型態，持續性微動畫優先。

## 總原則（全 Phase 共用）

1. **零新依賴**：只用現有 lipgloss/bubbles；sparkline、慶祝橫幅、ASCII 建築全部手繪 unicode。
2. **不碰 sim 純度**：新增內容全在表現層；讀 state、永不寫回 sim。慶祝觸發沿用「tick 前後 state 比對」樣板（`tui.go:387-393` prevFired 模式）。
3. **不畫整面背景**：終端背景不受控；深色 HUD 用前景色＋局部色塊（badge/chip/選中列）實現。所有色彩以 truecolor 為主、附 256 色 fallback（`lipgloss.CompleteColor` 或等價寫法）。
4. **動畫節奏維持現有 250ms tick**；不新增計時器。
5. 每個 Phase 獨立可交付、可玩，附 render/偵測測試（延續現有 `newAt` + `Update`/`View` 斷言風格）。

## Phase 1｜HUD 主題系統

新檔 `internal/tui/theme.go`，`style.go` 舊名保留為別名過渡，逐頁遷移後移除。

### 調色盤

| 名稱 | 用途 | truecolor | 256 fallback |
|---|---|---|---|
| hudCyan | 主 HUD 色：重點卡邊、active tab、主進度條 | #00D7FF 系 | 45 |
| hudPurple | 次要：漸層尾端、次要標題 | #B48CFF 系 | 141 |
| gainGreen | 漲、成功 | #50FA7B 系 | 84 |
| lossRed | 跌、威脅、超載 | #FF5555 系 | 203 |
| warnAmber | 警告、倒數 | #FFB86C 系 | 215 |
| goldCeleb | 慶祝、里程碑、成就 | #FFD75F 系 | 221 |
| mutedGray | 輔助文字 | faint 維持 | — |

### 卡片四變體

`Card(title, body)` 擴充為變體家族（同一實作、參數化樣式）：

- `cardDefault`：灰細邊（現有 RoundedBorder）
- `cardAccent`：青色粗邊（`lipgloss.ThickBorder()`，`┏━┓`）——作戰室重點卡（公司戰略、決勝）
- `cardThreat`：紅邊——宿敵路線卡、財務危機
- `cardGold`：金邊——慶祝橫幅、成就解鎖

### 彩色進度條

`progressBar` 升級：

- 一般條：青→紫逐字元漸層（lipgloss 逐 rune 上色；寬度 10-12 字元成本可忽略）
- 負載條（推理/電力）：<70% 青、70-90% 琥珀、>90% 紅
- 里程碑/成就條：金色
- 字元集升級 `█▓░`，滿格用實心

### 版面對齊（等寬 grid）

- `Card` 家族增加 `width` 參數；`ResponsiveRow` 改為把可用寬度均分給各卡並強制填滿（`lipgloss.Style.Width`）
- 兩欄 grid：每卡寬 `(cw-gap)/2`；窄於 `minDashWidth` 仍退化為直排
- 消滅現在的鋸齒狀右緣

### 資源列與 tab bar

- 資源列改分段膠囊：各段以細分隔 `│` 或色塊 chip 呈現，值用對應色（cash 綠/紅、R&D 青、估值紫）
- Tab bar：active 頁用 hudCyan 背景色塊反白，其餘 muted

## Phase 2｜活起來（動態層）

### Sparkline（新檔 `internal/tui/spark.go`）

- TUI 記憶體 ring buffer（不進存檔）：估值、總用戶、R&D 速率三條序列
- 取樣：每 4 tick（約 1 秒）一點，容量約 60 點；`Model` 上掛 `sparks` struct
- 渲染 `▁▂▃▄▅▆▇█` 映射 min-max 正規化，尾端最新；估值卡、公司卡內嵌一行
- 全零/單點序列渲染為平線，不除零

### 資源列漲跌

- `💰 $1.2M ▲+$340/s`：速率取 disp 平滑值的每秒差分（tick 間差 × 每秒 tick 數，再以既有 approach 平滑），不另開 sim 查詢介面
- ▲ gainGreen / ▼ lossRed；速率近零（|rate| < epsilon）不顯示箭頭

### Streak 常駐

- `🔥7天 ×1.42` 從 token pulse 段移出，常駐資源列（streakDays ≥ 2 才顯示）
- token 進帳 pulse 期間該段加亮（沿用 PulseToken）

### 市佔排行榜

- 你的列：hudCyan 背景色塊高亮整列
- 名次變化：與上一 board cycle 名次比較，`↑2`/`↓1`（gainGreen/lossRed）；名次歷史存 TUI 記憶體態（per-cycle 快照，cycle 變化時輪替）

### Token 進帳強化

- 現有 `⚡ Claude Code +X R&D` 升級為高亮 chip（青底深字），淡出行為沿用 tokenPulseTicks=12

## Phase 3｜遊戲化回饋

### Delta 偵測器（新檔 `internal/tui/feedback.go`）

tick 內比對前後 state，集中在一個 `detectMoments(prev, next GameState, prevMeta…) []Moment`：

| 時刻 | 偵測方式 |
|---|---|
| 訓練完成（線上） | `prev.HasTraining && !next.HasTraining` 且新增草稿 |
| 科技解鎖 | `len(UnlockedTech)` 增加（解鎖成功分支 `tui.go:514-515` 也可直接發） |
| 里程碑達成 | `MilestonesReached` 增加 |
| 階段推進 / 決勝開始 / 勝利 / 宿敵行動 / 財務危機 | `Campaign.Reports` 新增條目按 kind 分派 |
| 反制奏效 | 宿敵行動報告帶 countered 標記（sim 報告已有的資訊維持 read-only；若 kind 不可分辨，Phase 3 於 sim 報告 struct 加欄位——唯一允許的 sim 側改動，純資料欄位） |
| 成就解鎖（Phase 4） | 成就引擎回報 |

### 三級慶祝

| 等級 | 演出 | 實作 | 觸發 |
|---|---|---|---|
| Minor | 現有 notice + PulseNotice | `setNotice` | 科技解鎖、雇用、簽星、事件決議、改價 |
| Major | 金色橫幅（cardGold 單行，View 頂部 banner 疊層），約 12 tick 自動淡出，多事件排隊 | `Model.banners []banner`（TUI 態佇列） | 訓練完成、里程碑、階段推進、反制奏效、成就解鎖 |
| Epic | 全螢幕 overlay（取代 contentBody，按任意鍵關閉） | 佇列同上、等級最高者優先 | 戰役勝利、prestige 結算（Phase 6 接手美化） |

- Banner 插入點：`View()` 頂部疊層（`tui.go:1215-1236`，offlineSummary 之後）
- `applyOK()`（`tui.go:1012-1019`）改造：接受成功訊息參數，統一發 Minor 回饋——一處改動覆蓋所有靜默操作

### 決勝守成 UI（現在完全不可見）

- 作戰室卡（`renderCampaignStatusCard`）決勝階段顯示：`⚔ 決勝中——已頂住 1/2 次宿敵攻勢`，cardThreat 紅邊 + 每 tick 交替明暗的脈動標記
- 資料源：既有 `Campaign.ShowdownHeld / ShowdownAttempts`（已存在、無 UI）

### 威脅顯示

- 宿敵行動倒數 ≤1 週期：宿敵卡該行紅色 + 閃爍（tick 奇偶交替）
- 財務危機連續週期數：警告區升級為紅色橫幅

### 離線戰報

- `offlineBanner` 從單行升級為多行卡（cardAccent）：token 收穫、R&D、訓練完成、產業事件、董事會週期數、期間宿敵行動摘要（從 Reports 尾段截取本次結算新增者）
- 按任意鍵關閉行為不變

## Phase 4｜成就系統

### 存放（關鍵決策，已確認）

- 成就屬於玩家、跨 prestige 保留 → 存 `meta.json`：`store.Meta` 加 `Achievements map[string]int64`（id → unlockedAt unix）
- 舊 meta 載入 → nil map → 視為全未解鎖；零 migration 風險
- `saveMetaAt`（`tui.go:304-312`）同步組裝

### 目錄（新檔 `internal/tui/achievements_meta.go`，靜態表約 25-30 個）

類別示例：

- 進度：首模型上線、Gen2/3/4/5 首訓、七個估值里程碑（$1M～$1T）
- 習慣：streak 3/7/10 天、累計 token 1M/10M/100M
- 經營：簽下第一位/全部明星、四職能滿編、三市場同時第一
- 戰役：首選 doctrine、決勝守成成功、三條路線各自勝利、反制奏效、無盡模式進入
- 輪迴：首次 prestige、專利累計 10/50

每項：`id`、名稱、描述、達成條件說明、檢查函式 `func(m Model) bool`（讀 state/meta，純函式）。

### 引擎與呈現

- tick 內（低頻：每 8 tick 檢查一次即可）跑未解鎖項的檢查函式；解鎖 → 寫 meta + Major 金色橫幅 `🏆 成就解鎖：Gen4 大師`
- **新增第 7 頁「成就」**（`page_achievements.go`，pageNames 加項）：徽章牆網格，已解鎖 goldCeleb 亮色＋日期、未解鎖 muted 剪影＋條件提示；頂部總進度條（金色）

## Phase 5｜ASCII 總部成長視覺

- 總覽頁「總部」卡（新檔 `internal/tui/ascii_hq.go`）：7 階段建築對應 7 個估值里程碑——車庫 → 小辦公室 → 辦公樓 → 園區 → 摩天樓 → 巨塔 → 太空電梯
- 素材：每階段一組 5-8 行 ASCII art（手繪，hudCyan/mutedGray 上色；金色點綴當前解鎖階段）
- 微動畫：訓練中 2-frame 交替（機房燈 `▪/▫` 閃爍），沿用 tick 奇偶
- 響應式：寬 < 100 欄摺疊為單行 `🏠 車庫 → 🏢 → … → 🚀`（當前階段高亮）

## Phase 6｜Prestige 儀式化

- 勝利/prestige 結算升級為 Epic 全螢幕回顧（接 Phase 3 的 Epic 通道）：
  - 本局統計：天數、峰值估值、總用戶峰值、獲得專利、路線徽章
  - 金色 ASCII 獎盃 + cardGold 邊框
  - Legacy 三選一改為並排卡片式（cardAccent 選中高亮），沿用現有 dialog 鍵位慣例
- 新局開場卡：第 N 輪 · 帶入專利 ×12 · 徽章列 · Legacy 效果說明（開場顯示一次，任意鍵關閉）
- 資料源全部既有：`Prestige.Patents / RouteBadges / PendingLegacy`、`PeakValuation`

## 錯誤處理與邊界

- 所有渲染對零值/空 slice 安全（新遊戲、舊存檔缺欄位）：sparkline 空序列畫平線；成就 nil map 視為未解鎖；名次歷史不足時不顯示變化箭頭
- 窄終端（<80 欄）：全部新元件遵循現有 ResponsiveRow 直排退化；ASCII 總部摺疊為單行
- 256 色終端：所有色彩自動 fallback；`NO_COLOR`/dumb terminal 由 lipgloss 既有機制處理
- 慶祝佇列上限 8 條（超出丟棄最舊的 Major）防離線結算灌爆；Epic 永遠優先、Major 依序播放

## 測試策略

- 每 Phase：render 測試（`newAt` + WindowSizeMsg + View 內容斷言）＋邏輯測試
- Phase 2：ring buffer 取樣/正規化單元測試
- Phase 3：`detectMoments` 表格測試（每種時刻一案例）；banner 佇列淡出測試
- Phase 4：成就檢查函式表格測試；meta 存讀 roundtrip
- 迴歸：現有測試全綠（`go test ./...`）

## 交付順序

`1 → 2 → 3 → 4 → 5 → 6`。1-3 是表現層遞進；4 依賴 3 的偵測與慶祝機制；5、6 錦上添花。

## 附錄：關鍵掛載點（機制掃描結論）

- `tui.go:366-412` tickMsg 主體——delta 偵測插入點；`tui.go:387-393` prevFired 樣板
- `display.go:125-141` advanceDisplay——pulse 衰減迴圈，新 pulse 欄位加這
- `display.go:150-155` setNotice——Minor 通道
- `tui.go:1012-1019` applyOK——成功回饋統一入口
- `tui.go:1209-1241` View 頂部 banner 疊層——Major 橫幅插入點
- `tui.go:1244-1262` offlineBanner——離線戰報升級處
- `campaign_meta.go:111-155 / 158-216 / 219-261`——作戰室三卡（狀態/宿敵/報告）
- `sim/campaign_progress.go:46,50,66,76`——階段推進/決勝/勝利報告產生源
- `model.CampaignState.ShowdownHeld/ShowdownAttempts`——已存在無 UI 的決勝資料
- `store/meta.go:17-34` Meta + `tui.go:304-312` saveMetaAt——成就持久化
- `sim.go:135-138` MilestonesReached——里程碑觸發
- `tui.go:262-292` streak 機制——streak 常駐顯示與成就資料源
