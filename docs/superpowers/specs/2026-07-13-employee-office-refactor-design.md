# Tokensmith — 員工系統重構 + 辦公室總部 Design

- **日期**：2026-07-13
- **狀態**：設計已確認，待寫實作計畫
- **類型**：sim / balance / store / TUI 重構
- **一句話**：把聚合四職能 + 固定明星改成「程序生成個體員工 + 六階 + 四維 + 技能 + 工位」；總部 ASCII 改由花錢升級驅動，並決定工位與稀有招募機率；薪資以月薪為玩家體感單位。

---

## 0. 背景與目標

### 0.1 現況問題

| 現況 | 問題 |
|---|---|
| `Researchers[T1–T3]` + `Engineers/Ops/Marketing` 聚合計數 | 無個體差異、無招募「抽卡」感 |
| 固定 `Stars` 名冊 + `SignStar` | 內容一次看完，與「隨進度變強的市場」脫節 |
| 總部 ASCII 綁估值里程碑 | 純展示，不影響經營決策 |
| 薪資以 `/s` 為主 | 玩家體感弱 |

### 0.2 目標（需求對照）

1. **維持四類員工**：研發 / 工程 / 營運 / 行銷。
2. **招募介面**：呈現可招募候選人；每人有四維對應四類；六階由低至高：雜魚 → 職員 → 幹部 → 經理 → 總監 → 大神。
3. **專長與多維特高**：可單專精，也可 2 維／3 維／4 維特高。
4. **經理以上自帶被動技能**（與專精偏好關聯）：經理 1、總監 2、大神 3。
5. **技能分 Manager / Director / God 三池**；God 級技能不出現在經理員工。
6. **薪資與四維、技能正相關**；越高級越貴；**UI 以月薪呈現**。
7. **辦公室架構取代原總部 ASCII 驅動**：花錢升級 → 工位增加 + 視覺換階。
8. **可解雇**騰工位，付遣散費。
9. **稀有員工機率與總部等級正相關**。

### 0.3 已確認決策

| 面向 | 決策 |
|---|---|
| 與舊系統關係 | **全面取代**聚合四職能 + 固定明星 |
| 架構 | **方案 A：程序生成人才市場**（大神絕活可嵌少數具名模板） |
| 四維效益 | **主職 + 副加**（主職 1.0、副維 0.35） |
| 等級 | **六階全用** |
| 總部 | **Office.Level 花錢升級**，接管 ASCII；不再跟估值里程碑 |
| 市場 | **混合**：週期自動刷新 + 付費重抽；重抽費**等比上升**，自動刷新重置 |
| 技能 | 三池 + 豐富目錄（約 57）+ 少量 God Signature；v1 Signature 先做強常駐 |
| 解雇 | **遣散費 = 0.5 × 月薪** |
| 薪資顯示 | **月薪為主**；sim 換算 `SalaryPerSec = MonthlySalary / SecondsPerMonth` |
| 養成 | **不做**員工升階養成（招募時定階） |

### 0.4 非目標（本設計不做）

- 員工從雜魚養成到大神
- 完整對手挖角系統（僅預留抗性向技能效果）
- 技能觸發／CD 狀態機
- 玩家手動指定／更改主職
- 培訓樹、裝備、職位槽指派

---

## 1. 核心資料模型

### 1.1 退役與取代

| 移除 | 取代 |
|---|---|
| `Researchers[]`、`Engineers`、`Ops`、`Marketing` | `Employees []Employee` |
| `HireStaff` / `FireStaff` | `HireEmployee` / `FireEmployee` |
| `Stars` 目錄 + `SignStar` + `HiredStars` | 市場程序生成；高階／Signature 承載舊「明星」感 |
| 估值里程碑 → ASCII 總部 | `Office.Level` → 工位 + ASCII |

### 1.2 枚舉

```text
Rank: 雜魚, 職員, 幹部, 經理, 總監, 大神   // 6 階，低→高
Role: 研發, 工程, 營運, 行銷               // 4 類，維持現有語意
SkillTier: Manager, Director, God
```

### 1.3 Employee

```text
Employee {
  ID            string
  Name          string
  Rank          Rank
  Stats         [4]int        // 對應四 Role，建議 1–100
  PrimaryRole   Role          // argmax(Stats)；並列時 Role 枚舉序破平（確定性）
  SkillIDs      []string
  HireCost      float64       // 一次簽約現金
  MonthlySalary float64       // 設計與 UI 主單位
}
```

- 主職**不可**由玩家手動更改。
- 內部扣薪：`SalaryPerSec = MonthlySalary / SecondsPerMonth`（`SecondsPerMonth` 在 `balance`，實作計畫鎖定單一常數）。

### 1.4 Office

```text
Office {
  Level int   // 1..MaxOfficeLevel
}
```

- 工位上限由 `balance.OfficeSeats[Level]` 查表。
- `len(Employees) >= Seats` 時禁止雇用。
- ASCII 總部**只**讀 `Office.Level`。

### 1.5 TalentMarket

```text
TalentMarket {
  Candidates     []Employee  // 可招、尚未入職
  NextRefreshAt  float64     // 遊戲時間；到期免費整池刷新
  RerollCount    int         // 本週期已付費重抽次數
}
```

### 1.6 Skill

```text
Skill {
  ID, Name, Description string
  Tier                  SkillTier
  Signature             bool      // God 絕活；每員工最多 1
  Tags                  []string  // 主職偏好、效果族互斥
  Effects               SkillEffects
}
```

效果聚合規則：多數 mult 相乘或 add 後 soft-cap；每技標註 stack 方式。v1 全部為**被動常駐**。

---

## 2. 辦公室升級與視覺

### 2.1 等級表

| Level | 名稱（視覺） | 工位 | 升到下一級費用 |
|------:|---|-----:|---:|
| 1 | 車庫 | 3 | —（開局） |
| 2 | 小辦公室 | 5 | $25,000 |
| 3 | 開放式樓層 | 8 | $80,000 |
| 4 | 辦公樓 | 12 | $200,000 |
| 5 | 園區 | 16 | $500,000 |
| 6 | 摩天樓 | 22 | $1,200,000 |
| 7 | 巨塔 | 28 | $3,000,000 |
| 8 | 太空電梯 | 36 | $8,000,000 |

- 指令：`UpgradeOffice`：現金足夠且未滿級 → 扣費、`Level++`、視覺立即更新。
- 估值里程碑若仍存在：只服務成就／進度，**不管**總部長相與工位。
- 數值可整體縮放；節奏原則固定：**低級擠工位、升級換位與稀有度**。

### 2.2 ASCII

- `ascii_hq`（或等價）輸入改為 `Office.Level`（1–8 映射既有建築階）。
- 總覽頁可附：`工位 a/b · 月薪合計 $X/月`。

---

## 3. 人才市場

### 3.1 節奏參數

| 參數 | 值 | 說明 |
|---|---|---|
| 池大小 | 5 | 一屏可掃 |
| 自動刷新週期 | 600 遊戲秒 | 到期整池重 roll，`RerollCount = 0` |
| 開啟招募 UI | 免費 | 只檢視當前池 |
| 付費重抽 | 整池重 roll | `RerollCount++` |

### 3.2 重抽費用（等比、週期重置）

```text
RerollCost(n) = BaseRerollCost × (RerollGrowth ^ n)
```

- `n` = 本週期已重抽次數（0-based：第一次付 Base）
- 建議：`BaseRerollCost = 5000`，`RerollGrowth = 2`
- 序列：$5k → $10k → $20k → $40k …
- 自動刷新後 `n` 歸零，費用重置

### 3.3 等級權重（相對權重，正規化後抽）

| Rank | L1 | L2 | L3 | L4 | L5 | L6 | L7 | L8 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| 雜魚 | 55 | 45 | 35 | 25 | 18 | 12 | 8 | 5 |
| 職員 | 30 | 32 | 32 | 30 | 28 | 25 | 22 | 18 |
| 幹部 | 12 | 16 | 20 | 24 | 26 | 26 | 25 | 24 |
| 經理 | 3 | 5 | 9 | 13 | 16 | 18 | 20 | 22 |
| 總監 | 0 | 2 | 3 | 6 | 9 | 13 | 16 | 18 |
| 大神 | 0 | 0 | 1 | 2 | 3 | 6 | 9 | 13 |

- L1 不可能總監／大神；大神自 L3 起極低機率。
- **稀有度與總部正相關**的主開關。

### 3.4 四維生成

1. 抽 `Rank`（上表）。
2. 抽**特高模式**（隨總部微調，基準約）：

| 模式 | 基準權重 |
|---|---:|
| 單專精 | 70 |
| 雙維特高 | 22 |
| 三維特高 | 7 |
| 四維全高 | 1 |

總部升高時略增多維權重、略減單專精。

3. 依 Rank 的數值 band：

| Rank | 特高維（主帶） | 普通維 | floor |
|---|---|---|---:|
| 雜魚 | 15–30 | 8–18 | 5 |
| 職員 | 30–50 | 15–30 | 10 |
| 幹部 | 45–65 | 25–40 | 15 |
| 經理 | 60–80 | 35–55 | 20 |
| 總監 | 75–92 | 45–65 | 30 |
| 大神 | 88–100 | 55–75 | 40 |

4. `PrimaryRole = argmax(Stats)`；並列 → Role 序 0→3。

### 3.5 技能抽取規則

| Rank | 技能數 | 可抽池 |
|---|---:|---|
| 雜魚～幹部 | 0 | — |
| 經理 | 1 | **僅 Manager** |
| 總監 | 2 | Manager + Director；**至少 1 Director**；**禁止 God** |
| 大神 | 3 | 三池；**至少 1 God**；Signature **最多 1** |

- **硬約束**：God 級技能永不進入經理員工；亦不可進入總監。
- 與 `PrimaryRole` 相符的技能權重 ×2。
- **效果族互斥**：同族（如 `salary_self_down`、`company_rnd_pct`、`market_rarity`）同人最多 1；Signature 不與同族常駐重複。

### 3.6 確定性

- 刷新／重抽使用 `GameState` 內 PRNG（或 `MarketSeed + refreshIndex`）。
- Sim 保持純函式、可重放；UI 只送指令。

---

## 4. 效益、薪資、解雇

### 4.1 主職 + 副加 → 公司功率

```text
RolePower[r] =
  PrimaryWeight * Stats[r]     if r == PrimaryRole
  SecondaryWeight * Stats[r]   otherwise
```

- `PrimaryWeight = 1.0`
- `SecondaryWeight = 0.35`

全公司：

```text
TotalPower[role] = Σ RolePower[role] over Employees
Bonus[role] = Cap * (1 - exp(-K * TotalPower[role] / RefPower))
```

| 職能 | 接進 sim（方向） |
|---|---|
| 研發 | R&D/秒 |
| 工程 | 算力效率 |
| 營運 | 降服務流失 |
| 行銷 | 用戶成長 |

- 0 員工 → 中性（與舊系統一致）。
- **等級不直接乘效益**（避免與 Stats band 雙重計算）；等級透過更高 Stats + 技能間接變強。

### 4.2 月薪公式

```text
MonthlySalary =
  RankBaseMonth[rank]
  * (1 + StatFactor * StatScore)
  * (1 + SkillFactor * SkillScore)
  * MultiSpecMult
```

| 項 | 定義 |
|---|---|
| `RankBaseMonth`（示意） | 雜魚 800、職員 2500、幹部 6000、經理 15000、總監 40000、大神 100000 |
| `StatScore` | `(primary + SecondaryWeight * mean(secondary)) / 100` |
| `StatFactor` | ~0.8 |
| `SkillScore` | Manager +1、Director +2、God +3.5、Signature 再 +2 |
| `SkillFactor` | ~0.12 |
| `MultiSpecMult` | 單 1.00、雙 1.08、三 1.18、四 1.30 |

**簽約金**：

```text
HireCost = MonthlySalary * HireMonths
HireMonths = 2
```

**UI**：一律月薪（個人、合計、招募卡、解雇確認）；`/s` 僅可作次要除錯顯示。

### 4.3 解雇

```text
Severance = MonthlySalary * SeveranceMonths
SeveranceMonths = 0.5
```

- 現金不足 → 解雇失敗。
- 成功：扣遣散費、移出名冊、工位立刻釋放。
- 不退還 `HireCost`。

---

## 5. 技能目錄（約 ×3，roll 感）

目標規模：Manager **18** + Director **18** + God 常駐 **12** + God Signature **9** ≈ **57**。

### 5.1 Manager（18）— 個人向

| ID | 名 | 效果方向 |
|---|---|---|
| `m-deep-research` | 深潛研究 | 主職研發：研發功率 +12% |
| `m-sre-craft` | 穩健工程 | 主職工程：工程功率 +12% |
| `m-ops-playbook` | 營運手冊 | 主職營運：營運功率 +12% |
| `m-growth-hacks` | 成長黑客 | 主職行銷：行銷功率 +12% |
| `m-thrifty` | 精算師 | 本人月薪 -8% |
| `m-mentor` | 帶人 | 同主職低階員工功率 +3% |
| `m-night-owl` | 夜貓子 | 本人全維結算 +5% |
| `m-doc-driven` | 文件狂 | 主職研發：R&D 小額 flat |
| `m-perf-budget` | 效能預算 | 主職工程：算力效率微加 |
| `m-oncall` | 值班魂 | 主職營運：流失再降 |
| `m-copy-chief` | 文案手 | 主職行銷：用戶成長微加 |
| `m-cross-train` | 跨訓 | 本人副加權重 0.35→0.42 |
| `m-loyal` | 死忠 | 解雇本人遣散 -25% |
| `m-sprinter` | 衝刺型 | 訓練中本人功率 +10% |
| `m-frugal-stack` | 省雲費 | 主職工程：效率／成本向微加 |
| `m-pipeline` | 資料管線 | token→R&D 公司小加 +2% |
| `m-community` | 社群耳目 | 主職行銷：事件小減益 |
| `m-process-nerd` | 流程控 | 全公司營運功率 +2% |

### 5.2 Director（18）— 部門／公司級

| ID | 名 | 效果方向 |
|---|---|---|
| `d-lab-lead` | 實驗室主導 | 全公司研發 +6% |
| `d-infra-scale` | 基建擴張 | 算力效率 +5% |
| `d-sla-guard` | SLA 守護 | 服務流失再降 |
| `d-brand` | 品牌操盤 | 用戶成長 +6% |
| `d-talent-magnet` | 伯樂 | 市場稀有度視同總部 +0.5～1 階（cap） |
| `d-comp-opt` | 薪酬優化 | 全公司總月薪 -4% |
| `d-hiring-blitz` | 招聘衝刺 | 簽約金 -10% |
| `d-bench-strength` | 板凳深度 | 幹部及以下功率 +5% |
| `d-qa-gate` | 品質閘 | 訓練完成品質 +4% |
| `d-cost-ctrl` | 成本中心 | 現金流壓力事件傷害 -15% |
| `d-partner` | 生態合作 | 用戶 +3%、營收 +2% |
| `d-security` | 安全長視角 | 安全／事故抗性 + |
| `d-platform` | 平台化 | 全公司工程 +4% |
| `d-revops` | RevOps | 行銷+營運各 +3% |
| `d-research-ops` | ResearchOps | 研發+營運各 +3% |
| `d-market-sense` | 市場嗅覺 | 事件收益 +10% 或正面加權 |
| `d-desk-layout` | 工位配置 | 有效工位 +1（全公司最多疊 2） |
| `d-retention` | 留才 | 全公司遣散費 -20% |

### 5.3 God 常駐（12）

| ID | 名 | 效果方向 |
|---|---|---|
| `g-polymath` | 通才光環 | 本人副加權重 → 0.55 |
| `g-frontier` | 前沿直覺 | 訓練品質 +8% |
| `g-rainmaker` | 印鈔機 | 營收 +5% |
| `g-crisis` | 危機大腦 | 負面事件衝擊 -20% |
| `g-architect` | 系統架構師 | 算力效率 +8% |
| `g-scientist` | 首席科學家 | 研發功率 +10% |
| `g-operator` | 營運之神 | 流失大幅再降 |
| `g-evangelist` | 傳道者 | 用戶成長 +10% |
| `g-talent-blackhole` | 人才黑洞 | 市場高階權重再抬（與伯樂 soft-cap） |
| `g-equity-mind` | 股權思維 | 本人月薪 -15%，全公司研發 +3% |
| `g-compounder` | 複利腦 | 長期／R&D 效率小加成 |
| `g-full-stack-exec` | 全能高管 | 四職公司功率各 +3% |

### 5.4 God Signature（9）— 每人大神最多 1；僅大神

| ID | 名 | 效果方向（v1 強常駐） |
|---|---|---|
| `gs-token-oracle` | Token 神諭 | 真實 token→R&D +15% |
| `gs-poach-shield` | 挖角結界 | 本人遣散 -50%；另隨機 1 同事 -25% |
| `gs-moonshot` | 登月提案 | 訓練品質 +12% |
| `gs-open-source-halo` | 開源光環 | 用戶 +8%、品牌事件偏正 |
| `gs-chip-whisperer` | 晶片低語 | 算力效率 +12% |
| `gs-regulatory-sage` | 監管智者 | 安全／合規衝擊 -40% |
| `gs-viral-loop` | 病毒迴路 | 用戶成長 +12%（或 +15% 含 tradeoff） |
| `gs-war-chest` | 戰爭金庫 | 營收 +8%、重抽 Base -30% |
| `gs-one-person-army` | 一人成軍 | 本人四維結算 ×1.25 |

實作時每技需補齊：數值常數、stack 規則、效果族 tag、可接 sim 的 hook。上表為設計意圖，允許在 balance 微調幅度，**不可**破壞三池隔離與 Signature 上限。

---

## 6. 指令與套件邊界

### 6.1 指令

| 指令 | 行為 | 主要失敗條件 |
|---|---|---|
| `UpgradeOffice` | Level+1、扣升級費 | 滿級、現金不足 |
| `HireEmployee{CandidateID}` | 扣 HireCost、入名冊、移出池 | 無 ID、工位滿、現金不足 |
| `FireEmployee{EmployeeID}` | 扣 Severance、移出名冊 | 無 ID、現金不足 |
| `RerollMarket` | 付費整池重 roll | 現金 < RerollCost(n) |
| Tick 自動 | `now >= NextRefreshAt` → 免費刷新、n=0 | — |

退役：`HireStaff`、`FireStaff`、`SignStar`（建議直接移除並改測試，勿留 silent no-op）。

### 6.2 套件

| 層 | 職責 |
|---|---|
| `internal/model` | Rank、Employee、Office、TalentMarket、新 Command |
| `internal/balance` | 工位／升級費、權重、band、月薪係數、技能目錄、重抽參數、`SecondsPerMonth` |
| `internal/sim` | 確定性生成、指令、扣薪、功率與技能聚合、市場到期 |
| `internal/store` | 序列化、schema migrate |
| `internal/tui` | Team 頁、月薪 UI、按鍵、總覽工位摘要 |
| `ascii_hq` | 改讀 `Office.Level` |

---

## 7. TUI

### 7.1 團隊頁三區

```
┌─ 總部／辦公室 ──────────────────────────────┐
│ ASCII(Level) · 名稱 · 工位 a/b               │
│ 升級預覽與費用 · [u]                         │
└────────────────────────────────────────────┘
┌─ 在職名冊 ─────────────┐ ┌─ 人才市場 ──────────┐
│ 姓名 階 主職 月薪 技能數 │ │ 5 張候選人卡         │
│ 月薪合計                 │ │ 四維／技能／簽約／月薪 │
│ [f] 解雇（預覽遣散）     │ │ [h] 雇 [r] 重抽      │
│                         │ │ 免費刷新倒數 · 重抽價  │
└─────────────────────────┘ └────────────────────┘
```

### 7.2 按鍵草案

| Key | 動作 |
|---|---|
| `u` | 升級總部 |
| `tab` / 方向鍵 | 區與卡片焦點 |
| `h` | 雇用焦點候選人 |
| `f` | 解雇焦點在職 |
| `r` | 付費重抽 |
| `enter` | 詳情（四維條、技能說明） |

### 7.3 文案單位

- 主：月薪、簽約金、遣散、公司總月薪、重抽費、升級費。
- 不把 `/s` 當主標。

---

## 8. 存檔遷移

**策略：補償清空 + Office Lv1**

1. 偵測舊欄位（聚合人數、`HiredStars`）。
2. 現金補償（建議）：每舊聚合人頭 **$2,000** + 每已簽明星 **$50,000**；並可顯示一次改制 notice。
3. 清空聚合與明星欄位。
4. `Office.Level = 1`，`Employees = []`，生成初始市場池。
5. Schema／版本 +1；單測覆蓋「含舊明星」存檔。

Prestige 重開：新局初始（Lv1、空名冊、新市場），不繼承個體。

---

## 9. 測試邊界

1. **生成**：固定 seed 穩定；L1 無大神；L8 大神權重顯著升高。
2. **技能門檻**：經理無 God；總監無 God；大神 Signature ≤1、至少 1 God 技。
3. **工位**：滿員拒雇；解雇後可雇；`d-desk-layout` 有效工位 cap。
4. **經濟**：月薪→/s；簽約 2 月；遣散 0.5 月；重抽等比與刷新重置。
5. **效益**：0 人中性；單專精 vs 四維通才方向正確；遞減不爆表。
6. **遷移**：舊檔可 Tick、狀態合法。
7. **TUI 煙測**：雇／解雇／升級／重抽 notice。

---

## 10. 與舊「明星員工」對照

| 舊概念 | 新落點 |
|---|---|
| 固定名冊簽約 | 市場刷總監／大神 |
| 高簽約金 + 高薪 | `HireCost` + 高 `MonthlySalary` |
| 維度／R&D／用戶 mult | Stats + 技能 Effects |
| 敘事名將 | God Signature（可選具名模板嵌生成） |

---

## 11. 實作分期建議（供 writing-plans 拆 PR）

| 階段 | 內容 |
|---|---|
| P1 | model + balance 骨架 + Office 升級 + 空名冊扣薪路徑 |
| P2 | 市場生成、Hire/Fire、工位、重抽／自動刷新 |
| P3 | 四維功率聚合接入 sim（取代舊 staff bonus） |
| P4 | 技能目錄與效果聚合 |
| P5 | TUI Team + ASCII 改 Office + 月薪文案 |
| P6 | store migrate + 全量測試與數值微調 |

---

## 12. 開放微調（實作期可調、不改設計意圖）

- `SecondsPerMonth` 絕對值
- 升級費／工位曲線整體倍率
- `RankBaseMonth` 與經濟通膨對齊
- 各技能具體百分點
- 市場 600s、池大小 5、BaseReroll 5k

**不可在實作期默默改掉的硬約束**：六階、四職、主職+副加、三池隔離、經理無 God 技、工位上限、重抽等比重置、月薪 UI、Office 驅動視覺、全面取代聚合與明星。
