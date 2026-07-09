# TUI Scroll + Restrained Motion — Design

- **日期**：2026-07-09
- **狀態**：設計已確認，待寫實作計畫
- **範圍**：全域 content viewport 捲動 + 克制 displayState 動效；市場頁結構不改
- **相關**：`docs/superpowers/specs/2026-07-09-tui-dashboard-redesign-design.md`（v0.6 儀表板）；`internal/tui/*`

---

## 1. 問題

1. **市場等頁內容變高**（三區隔卡 + 對手檔），但 TUI 只記 `width`、整頁一次渲染，**無法捲動**（alt-screen 下終端原生捲動通常也無效）。
2. 儀表板資訊變多後，數字/ bar **每 tick 硬跳**，缺少經營模擬的「在動」感；使用者希望 **更多動效**，但強度要克制。

## 2. 目標與決策

| 項 | 選擇 |
|---|---|
| 範圍 | **捲動系統 + 動效語言** 一次做完 |
| 動效強度 | **克制經營模擬感**（數字/bar 平滑、token 短 pulse、notice 短高亮） |
| 市場頁 | **結構不變**，靠 viewport 捲完 |
| 方案 | **A**：`bubbles/viewport` 內容區 + `displayState` lerp |

### 非目標

- 市場改為焦點區隔 / 雙欄導航重構
- 真 60fps 動畫迴圈、粒子、全屏轉場
- 改 sim 經濟公式
- 每頁獨立 scroll offset 記憶（切頁回頂即可）

---

## 3. 全域 content viewport

### 3.1 依賴

```text
github.com/charmbracelet/bubbles/viewport
```

### 3.2 殼層 vs 可捲區

```
┌ header (Tokensmith · Day)     ┐ fixed
│ notice?                       │ fixed
│ resource bar                  │ fixed
│ tabs                          │ fixed
├───────────────────────────────┤
│                               │
│  viewport  ← renderPage()     │ scrollable
│                               │
├───────────────────────────────┤
│ pressures? (短)               │ fixed（建議；過長可併入 viewport）
│ footer help                   │ fixed
└───────────────────────────────┘
```

Dialog（訓練/發佈）開時：viewport 內容改為 dialog（通常無需捲），且 **捲動鍵不生效**。

### 3.3 Model 狀態

```text
width, height int
vp            viewport.Model
```

`WindowSizeMsg`：更新 width/height，重算：

```text
contentH = max(3, height − chromeRows)
vp.Width, vp.Height = contentW, contentH
```

`chromeRows`：以實際固定區塊行數估算（或常數 + pressures 行數）。

### 3.4 餵內容

每次需要重畫時（tick、切頁、鍵處理後）：

```text
vp.SetContent(renderPageOrDialog(m))
```

`View()` = 固定頂 + `vp.View()` + 固定底。

### 3.5 捲動鍵

| 情境 | 鍵 | 行為 |
|---|---|---|
| 無 dialog，無 ↑↓ cursor 頁（總覽/市場/團隊） | `↑↓` / `j k`、`PgUp/PgDn` | 捲 viewport |
| 無 dialog，有 cursor 頁（模型/科技/算力） | `↑↓` 選取；`PgUp/PgDn`、`ctrl+u`/`ctrl+d` 捲 | 避免搶選取 |
| Dialog 開 | 僅 dialog 鍵 | 不捲 |
| 切頁 | — | `vp.GotoTop()` |

滑鼠滾輪：若 `WithMouseCellMotion` 成本低則做；否則可 defer。

### 3.6 測試

- chrome 高度為正、contentH ≥ 3
- 長 content 時 viewport 可 `YOffset > 0`（模擬 PgDn）
- dialog 開時捲動鍵不改變 offset（或測 handler 短路）

---

## 4. 克制動效（displayState）

### 4.1 原則

- `sim.GameState` = 真相；**不**為動畫改 tick 公式。
- 展示用 `displayState` 落後數 tick；破產/prestige/讀檔 **snap**。

### 4.2 欄位（示意）

| 欄位 | 用途 |
|---|---|
| `dispCash`, `dispRnD`, `dispValuation` | 資源條 |
| `dispTotalUsers` / per-model users | 總覽、模型 |
| `dispInfUtil`, `dispTrainUtil` | 資源條與算力 bar |
| `dispShares`（可簡化：消費者 top 條） | 總覽/市場 bar |
| `pulseToken`, `pulseNotice` | 剩餘高亮幀 |

### 4.3 每 tick

```text
α ≈ 0.3
disp += α * (truth − disp); near-zero snap
lastTokens > 0 → pulseToken = 3..4
new notice → pulseNotice = 4
pulse*--
```

渲染資源條/用戶/bar 時讀 **disp\***；token pulse 時 R&D 段用 accent 樣式 2–3 幀。

### 4.4 不做

- 全屏轉場、粒子、選中卡狂閃
- 獨立 60fps timer（沿用 250ms tick）

### 4.5 測試

- lerp helper 單元測
- 多 tick 後 disp 逼近 truth
- Restart/snap 後 disp == truth

---

## 5. 實作切分（建議）

1. **依賴 + viewport 殼**：height、chrome 切分、SetContent、PgUp/Dn；長頁可捲。
2. **鍵位衝突表**：有 cursor 頁 vs 無 cursor 頁。
3. **displayState lerp**：資源條數字 + util bar。
4. **擴展 lerp**：用戶數、市佔 bar；token/notice pulse。
5. **回歸**：全測 + 手動市場長頁捲動、數字平滑。

可單一 PR。

---

## 6. 風險

| 風險 | 緩解 |
|---|---|
| chrome 行數估錯 → 裁切錯 | 集中 `measureChrome`；偏低寧可多給 content |
| ↑↓ 與選取衝突 | 分頁策略寫死 + 測試 |
| 每 tick SetContent 成本 | content 字串已存在；viewport 可接受 |
| lerp 與精確會計混淆 | 僅展示層；存檔仍用 sim |

---

## 7. 成功標準

1. 市場頁在標準終端高度下可 **PgDn 看完對手列表**；殼層不跟著捲走。
2. 模型/科技頁 **↑↓ 仍選項目**；PgUp/Dn 捲內容。
3. 現金/用戶/主要 bar **視覺上平滑**，非每 tick 硬跳。
4. 有 token 流入時資源條有 **短暫** 可察覺 feedback。
5. `go test ./...` 綠；無經濟行為改變。

---

## 8. 決策摘要

| 決策 | 選擇 |
|---|---|
| 方案 | bubbles/viewport + displayState |
| 動效 | 克制 lerp + short pulse |
| 市場 | 結構不變，可捲 |
| 切頁 scroll | 回頂 |
