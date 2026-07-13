# Employee + Office Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace aggregate staff + fixed stars with procedural individual employees (6 ranks, 4 stats, skills), a cash-upgraded office (seats + ASCII), and a hybrid talent market; present pay as monthly salary.

**Architecture:** New pure types in `model`; generation/salary/power/skill tables in `balance`; deterministic market RNG + hire/fire/upgrade/reroll in `sim`; schema migrate in `store`; Team page + HQ art driven by `Office.Level` in `tui`. Old `HireStaff`/`FireStaff`/`SignStar` and aggregate counters are removed; tests that seeded researchers/engineers/stars are rewritten to seed `Employees`.

**Tech Stack:** Go (module `tokensmith`), std `testing`, existing packages `model` / `balance` / `sim` / `store` / `tui`. No new deps.

**Spec:** `docs/superpowers/specs/2026-07-13-employee-office-refactor-design.md`

## Global Constraints

- Spec hard rules: six ranks, four roles, primary+secondary (1.0 / 0.35), three skill tiers with **God skills never on Manager or Director ranks**, manager 1 / director 2 / god 3 skill slots, office cash upgrade drives ASCII + seats, reroll cost geometric until free refresh, monthly salary in UI, full replace of aggregate staff + stars.
- `internal/sim` stays pure: no wall clock, no `math/rand` global; market uses `TalentMarket.RandState` via existing `nextRand` (splitmix64).
- `SalaryPerSec = MonthlySalary / SecondsPerMonth` with `SecondsPerMonth = 600` (one game-month = 600s wall/game time for pay display conversion).
- TDD per task: failing test → implement → pass → commit.
- After each task: `gofmt` touched files; final gate `go test ./...` and `go build ./...`.
- Chinese display names for ranks/skills/office stages (code IDs stay ASCII).

## File map

| File | Role |
|---|---|
| `internal/model/types.go` | Remove old staff/star fields & cmds; add Rank, Employee, Office, TalentMarket, new commands |
| `internal/model/employee.go` | **Create** — Rank/SkillTier helpers, PrimaryRoleFromStats, MultiSpecMode |
| `internal/model/types_test.go` / `employee_test.go` | Type + helper tests |
| `internal/balance/employee.go` | **Create** — office table, rank weights, bands, salary, severance, market knobs |
| `internal/balance/skills.go` | **Create** — ~57 skill defs + lookup |
| `internal/balance/balance.go` | Wire Config fields; drop obsolete staff/star hire fields when unused |
| `internal/balance/employee_test.go`, `skills_test.go` | Balance tests |
| `internal/sim/employee.go` | **Create** — power, salary/sec, skill effects, seats |
| `internal/sim/market.go` | **Create** — generate candidate, refresh pool, reroll cost |
| `internal/sim/apply.go` | New commands; remove HireStaff/FireStaff/SignStar |
| `internal/sim/sim.go` | Tick uses employee R&D/salary/infra/ops/marketing; market auto-refresh |
| `internal/sim/star.go` | Delete or gut; starEffects → skill/employee effects |
| `internal/sim/prestige.go` | freshRun: Office Lv1, empty employees, seed market |
| `internal/store/store.go` | SchemaVersion → 2 |
| `internal/store/migrate.go` | migrate staff/stars → compensation + new office/market |
| `internal/tui/ascii_hq.go` | Stage from Office.Level |
| `internal/tui/page_team.go` | Roster + market + office UI |
| `internal/tui/tui.go` | Key bindings h/f/r/u for new commands |

---

### Task 1: Model — Rank, Employee, Office, Market, commands

**Files:**
- Create: `internal/model/employee.go`
- Create: `internal/model/employee_test.go`
- Modify: `internal/model/types.go`
- Modify: `internal/model/types_test.go`

**Interfaces:**
- Produces:
  - `type Rank int` — `RankGrunt=0 … RankGod=5`, `NumRanks=6`
  - `type SkillTier int` — `SkillTierManager=0, SkillTierDirector=1, SkillTierGod=2`
  - `type Employee struct { ID, Name string; Rank Rank; Stats [NumRoles]int; PrimaryRole Role; SkillIDs []string; HireCost, MonthlySalary float64 }`
  - `type Office struct { Level int }`
  - `type TalentMarket struct { Candidates []Employee; NextRefreshAt float64; RerollCount int; RandState uint64 }`
  - Commands: `UpgradeOffice{}`, `HireEmployee{CandidateID string}`, `FireEmployee{EmployeeID string}`, `RerollMarket{}`
  - `func PrimaryRoleFromStats(stats [NumRoles]int) Role`
  - `func MultiSpecCount(stats [NumRoles]int, highThreshold int) int` — count of stats ≥ threshold (caller supplies band floor for “high”)
- GameState changes:
  - **Add:** `Employees []Employee`, `Office Office`, `Market TalentMarket`
  - **Remove:** `Engineers, Ops, Marketing int`, `HiredStars []string`
  - **Research:** keep `EfficiencyMult`; set `Researchers` removed OR zero-only deprecated — **remove `Researchers [NumTiers]int`** from `Research` (only `EfficiencyMult` remains)
  - **Remove types/cmds:** `HireStaff`, `FireStaff`, `SignStar`, `Star`, `StarEffects` (move any NeutralStarEffects dependents later)
- Keep `Role`, `NumRoles`, `StaffTier` only if still needed elsewhere; if `StaffTier` unused after Research change, leave constants for now only if other code breaks — prefer delete unused after compile fixes in Task 7.

- [ ] **Step 1: Write failing tests**

`internal/model/employee_test.go`:

```go
package model

import "testing"

func TestRankConsts(t *testing.T) {
	if NumRanks != 6 || RankGod != 5 || RankGrunt != 0 {
		t.Fatalf("ranks: grunt=%d god=%d num=%d", RankGrunt, RankGod, NumRanks)
	}
}

func TestPrimaryRoleFromStats(t *testing.T) {
	var s [NumRoles]int
	s[RoleEngineer] = 80
	s[RoleResearcher] = 40
	if PrimaryRoleFromStats(s) != RoleEngineer {
		t.Fatalf("want engineer")
	}
	// tie → lower role index wins
	s = [NumRoles]int{50, 50, 10, 10}
	if PrimaryRoleFromStats(s) != RoleResearcher {
		t.Fatalf("tie want researcher, got %v", PrimaryRoleFromStats(s))
	}
}

func TestEmployeeCommandsAreCommand(t *testing.T) {
	var cs []Command = []Command{
		UpgradeOffice{},
		HireEmployee{CandidateID: "c1"},
		FireEmployee{EmployeeID: "e1"},
		RerollMarket{},
	}
	if len(cs) != 4 {
		t.Fatal("commands")
	}
}

func TestGameStateEmployeeFields(t *testing.T) {
	var s GameState
	s.Office.Level = 1
	s.Employees = append(s.Employees, Employee{ID: "e", Rank: RankStaff, MonthlySalary: 2500})
	s.Market.Candidates = append(s.Market.Candidates, Employee{ID: "c"})
	if s.Office.Level != 1 || len(s.Employees) != 1 || len(s.Market.Candidates) != 1 {
		t.Fatalf("%+v", s)
	}
}
```

Update `types_test.go`: remove/replace `TestRolesAndStaffCommands` and `TestStarTypes` that reference removed types — delete those tests or rewrite to only check `Role`/`NumRoles` still exist.

- [ ] **Step 2: Run tests — expect FAIL**

```bash
go test ./internal/model/ -count=1
```

Expected: undefined Rank / Employee / commands.

- [ ] **Step 3: Implement**

`internal/model/employee.go`:

```go
package model

// Rank is employee seniority (spec six ranks).
type Rank int

const (
	RankGrunt Rank = iota // 雜魚
	RankStaff             // 職員
	RankLead              // 幹部
	RankManager           // 經理
	RankDirector          // 總監
	RankGod               // 大神
	NumRanks  = 6
)

// SkillTier gates which skills can roll onto which ranks.
type SkillTier int

const (
	SkillTierManager SkillTier = iota
	SkillTierDirector
	SkillTierGod
)

// Employee is one hired or market-candidate person.
type Employee struct {
	ID            string
	Name          string
	Rank          Rank
	Stats         [NumRoles]int
	PrimaryRole   Role
	SkillIDs      []string
	HireCost      float64
	MonthlySalary float64
}

// Office is the headquarters upgrade track (seats + ASCII).
type Office struct {
	Level int // 1..Max; 0 treated as 1 by sim helpers
}

// TalentMarket is the hireable candidate pool.
type TalentMarket struct {
	Candidates     []Employee
	NextRefreshAt  float64
	RerollCount    int
	RandState      uint64
}

// UpgradeOffice spends cash to raise Office.Level by 1.
type UpgradeOffice struct{}

func (UpgradeOffice) commandMarker() {}

// HireEmployee hires a market candidate by ID.
type HireEmployee struct {
	CandidateID string
}

func (HireEmployee) commandMarker() {}

// FireEmployee fires a roster employee by ID (pays severance).
type FireEmployee struct {
	EmployeeID string
}

func (FireEmployee) commandMarker() {}

// RerollMarket pays escalating cost to regenerate the candidate pool.
type RerollMarket struct{}

func (RerollMarket) commandMarker() {}

// PrimaryRoleFromStats returns argmax role; ties → lowest Role index.
func PrimaryRoleFromStats(stats [NumRoles]int) Role {
	best := RoleResearcher
	bestV := stats[RoleResearcher]
	for r := RoleEngineer; r < NumRoles; r++ {
		if stats[r] > bestV {
			bestV = stats[r]
			best = r
		}
	}
	return best
}
```

In `types.go` `GameState`:
- Remove `Engineers`, `Ops`, `Marketing`, `HiredStars`
- Add `Employees []Employee`, `Office Office`, `Market TalentMarket`
- Change `Research` to:

```go
type Research struct {
	EfficiencyMult float64 // 1.0 = neutral; kept for tech hooks / future
}
```

Remove `HireStaff`, `FireStaff`, `SignStar`, `Star`, `StarEffects`, `NeutralStarEffects` from types.go.

Keep `Role` constants.

- [ ] **Step 4: Run tests — expect PASS for model package only if no other packages compiled**

```bash
go test ./internal/model/ -count=1
```

Expected: PASS. (Other packages will not compile until later tasks — that is OK if you only test model. Do **not** run `./...` until Task 7+.)

- [ ] **Step 5: Commit**

```bash
git add internal/model/
git commit -m "feat(model): individual employees, office, talent market types"
```

---

### Task 2: Balance — office, market weights, salary knobs

**Files:**
- Create: `internal/balance/employee.go`
- Create: `internal/balance/employee_test.go`
- Modify: `internal/balance/balance.go` (`Config` + `Default()`)

**Interfaces:**
- Produces on `Config` (or package funcs reading defaults):
  - `SecondsPerMonth float64` = 600
  - `MaxOfficeLevel int` = 8
  - `OfficeSeats [9]int` index by level (1..8); index 0 unused
  - `OfficeUpgradeCost [9]float64` cost to go **from** level i **to** i+1 (index = current level)
  - `OfficeNames [9]string` Chinese stage names
  - `MarketPoolSize int` = 5
  - `MarketRefreshSec float64` = 600
  - `MarketRerollBase float64` = 5000
  - `MarketRerollGrowth float64` = 2
  - `RankWeights [8][NumRanks]float64` — row = office level-1, col = rank
  - `MultiSpecWeights [4]float64` — single/dual/tri/quad base weights 70,22,7,1
  - `RankStatHigh [NumRanks][2]int` min,max for high dims
  - `RankStatNorm [NumRanks][2]int` min,max for normal dims
  - `RankStatFloor [NumRanks]int`
  - `RankBaseMonth [NumRanks]float64`
  - `SalaryStatFactor`, `SalarySkillFactor float64`
  - `MultiSpecSalaryMult [4]float64` — index = highCount-1 (0→single)
  - `HireMonths float64` = 2
  - `SeveranceMonths float64` = 0.5
  - `PrimaryWeight float64` = 1.0
  - `SecondaryWeight float64` = 0.35
  - `StaffPowerCap`, `StaffPowerK`, `StaffPowerRef float64` — diminishing bonus curve
  - `RnDPerPower float64` — R&D/sec per unit research RolePower before EfficiencyMult
- Helpers:
  - `func OfficeSeatsAt(level int, b Config) int`
  - `func OfficeUpgradeCostAt(level int, b Config) (cost float64, ok bool)` — ok false if maxed
  - `func RerollCost(rerollCount int, b Config) float64`
  - `func MonthlyToPerSec(monthly float64, b Config) float64`

- [ ] **Step 1: Failing test**

```go
package balance

import (
	"math"
	"testing"

	"tokensmith/internal/model"
)

func TestOfficeTable(t *testing.T) {
	b := Default()
	if b.MaxOfficeLevel != 8 {
		t.Fatalf("max=%d", b.MaxOfficeLevel)
	}
	if OfficeSeatsAt(1, b) != 3 || OfficeSeatsAt(8, b) != 36 {
		t.Fatalf("seats L1=%d L8=%d", OfficeSeatsAt(1, b), OfficeSeatsAt(8, b))
	}
	c, ok := OfficeUpgradeCostAt(1, b)
	if !ok || c != 25000 {
		t.Fatalf("upgrade L1→2: %v %v", c, ok)
	}
	if _, ok := OfficeUpgradeCostAt(8, b); ok {
		t.Fatal("L8 should not upgrade")
	}
}

func TestRerollCostGeometric(t *testing.T) {
	b := Default()
	if RerollCost(0, b) != 5000 || RerollCost(1, b) != 10000 || RerollCost(2, b) != 20000 {
		t.Fatalf("reroll: %v %v %v", RerollCost(0, b), RerollCost(1, b), RerollCost(2, b))
	}
}

func TestRankWeightsL1NoGod(t *testing.T) {
	b := Default()
	if b.RankWeights[0][model.RankGod] != 0 || b.RankWeights[0][model.RankDirector] != 0 {
		t.Fatalf("L1 weights: %+v", b.RankWeights[0])
	}
	if b.RankWeights[7][model.RankGod] <= 0 {
		t.Fatal("L8 must allow god")
	}
}

func TestMonthlyToPerSec(t *testing.T) {
	b := Default()
	if b.SecondsPerMonth != 600 {
		t.Fatal(b.SecondsPerMonth)
	}
	got := MonthlyToPerSec(6000, b)
	if math.Abs(got-10) > 1e-9 {
		t.Fatalf("got %v want 10", got)
	}
}
```

- [ ] **Step 2: Run — FAIL**

```bash
go test ./internal/balance/ -run 'TestOffice|TestReroll|TestRankWeights|TestMonthly' -count=1
```

- [ ] **Step 3: Implement `employee.go` + wire `Default()`**

Fill tables from spec §2.1–§2.4 and §4.2 exactly:

| L | seats | upgrade cost from L |
|--:|------:|---:|
| 1 | 3 | 25000 |
| 2 | 5 | 80000 |
| 3 | 8 | 200000 |
| 4 | 12 | 500000 |
| 5 | 16 | 1200000 |
| 6 | 22 | 3000000 |
| 7 | 28 | 8000000 |
| 8 | 36 | — |

`RankBaseMonth`: grunt 800, staff 2500, lead 6000, manager 15000, director 40000, god 100000.

`RankWeights` rows for L1..L8 as in spec table.

`RnDPerPower`: choose so ~staff stat 40 primary ≈ old T1 researcher order of magnitude — start `0.0002` (tune later); document in comment.

`StaffPowerCap=0.8`, `StaffPowerK=1.2`, `StaffPowerRef=200` as starting diminishing params for engineer/ops/marketing mults.

Office names align ASCII: `車庫, 小辦公室, 開放式樓層, 辦公樓, 園區, 摩天樓, 巨塔, 太空電梯`.

- [ ] **Step 4: PASS + commit**

```bash
go test ./internal/balance/ -run 'TestOffice|TestReroll|TestRankWeights|TestMonthly' -count=1
git add internal/balance/
git commit -m "feat(balance): office, market, and salary employee knobs"
```

---

### Task 3: Balance — skill catalog (~57)

**Files:**
- Create: `internal/balance/skills.go`
- Create: `internal/balance/skills_test.go`
- Modify: `internal/balance/balance.go` — `Skills []SkillDef`, `Default()` loads them

**Interfaces:**

```go
// SkillDef is a passive skill definition.
type SkillDef struct {
	ID          string
	NameZH      string
	Tier        model.SkillTier
	Signature   bool
	Family      string // mutex family key
	PreferRole  model.Role // NumRoles or -1 if none; use Role(-1) or separate bool
	HasPrefer   bool
	// Numeric hooks (v1); zero = unused
	SelfRolePowerMult float64 // if >0 and primary matches PreferRole
	CompanyRolePower  [model.NumRoles]float64 // additive mult on company power, e.g. 0.06
	SelfSalaryMult    float64 // e.g. 0.92 for -8%
	CompanySalaryMult float64
	HireCostMult      float64
	SeveranceMult     float64 // company-wide or self — document per skill
	TokenRnDMult      float64
	InfraMult         float64
	UserGrowthMult    float64
	ChurnMult         float64 // <1 reduces churn
	TrainQualityMult  float64
	RevenueMult       float64
	SecondaryWeight   float64 // if >0, overrides secondary weight for holder
	ExtraSeats        int
	MarketRarityBonus float64 // added to effective office level for weights
	RerollBaseMult    float64
	SelfStatMult      float64 // multiply all stats in power calc
	EventNegMult      float64 // <1 softens negative events
}

func SkillByID(b Config, id string) (SkillDef, bool)
func SkillsByTier(b Config, tier model.SkillTier) []SkillDef
func DefaultSkills() []SkillDef
```

- [ ] **Step 1: Tests**

```go
func TestDefaultSkillsCountAndTiers(t *testing.T) {
	b := Default()
	if len(b.Skills) < 50 {
		t.Fatalf("skills=%d want >=50", len(b.Skills))
	}
	var m, d, g, sig int
	ids := map[string]bool{}
	for _, sk := range b.Skills {
		if ids[sk.ID] {
			t.Fatalf("dup id %s", sk.ID)
		}
		ids[sk.ID] = true
		switch sk.Tier {
		case model.SkillTierManager:
			m++
			if sk.Signature {
				t.Fatalf("manager signature %s", sk.ID)
			}
		case model.SkillTierDirector:
			d++
		case model.SkillTierGod:
			g++
			if sk.Signature {
				sig++
			}
		}
	}
	if m < 18 || d < 18 || g < 12 || sig < 9 {
		t.Fatalf("tier counts m=%d d=%d g=%d sig=%d", m, d, g, sig)
	}
}

func TestSkillByID(t *testing.T) {
	b := Default()
	sk, ok := SkillByID(b, "gs-token-oracle")
	if !ok || !sk.Signature || sk.TokenRnDMult < 1.1 {
		t.Fatalf("%+v ok=%v", sk, ok)
	}
}
```

- [ ] **Step 2: Implement full catalog**

Implement **all** IDs from spec §5.1–§5.4 with non-zero effect fields matching the listed directions (percentages as mults: +12% → `1.12` or additive `0.12` — **pick one convention and use everywhere**: use **multiplier fields default 1.0 for unused**, and additive pct fields default 0. Prefer:

- `*Mult` fields: neutral = **1.0**, buffs >1, reductions <1  
- `CompanyRolePower[r]`: neutral **0**, value `0.06` means +6% when applied as `1+sum`  
- `ExtraSeats`: integer  
- `MarketRarityBonus`: e.g. 0.5  
- `SecondaryWeight`: if >0 replace personal secondary weight  

Manager 18 + Director 18 + God 12 + Signature 9. Every `Family` string unique per exclusivity group (`salary_self`, `company_rnd`, `market_rarity`, `token_rnd`, `desk`, …).

- [ ] **Step 3: PASS + commit**

```bash
go test ./internal/balance/ -run TestDefaultSkills -count=1
git add internal/balance/
git commit -m "feat(balance): employee skill catalog (~57 passives)"
```

---

### Task 4: Sim — power, salary, seats, skill aggregate skeleton

**Files:**
- Create: `internal/sim/employee.go`
- Create: `internal/sim/employee_test.go`

**Interfaces:**
- `func effectiveOfficeLevel(ns model.GameState) int` — max(1, Office.Level)
- `func seatCap(ns model.GameState, b balance.Config) int` — seats + ExtraSeats from skills (cap +2 from desk skills per spec)
- `func rosterFull(ns model.GameState, b balance.Config) bool`
- `func employeeRolePower(e model.Employee, b balance.Config) [model.NumRoles]float64`
- `func totalRolePower(ns model.GameState, b balance.Config) [model.NumRoles]float64`
- `func staffRnDPerSecFromEmployees(ns model.GameState, b balance.Config) float64`
- `func totalSalaryPerSecFromEmployees(ns model.GameState, b balance.Config) float64`
- `func roleBonus(totalPower float64, b balance.Config) float64` — `Cap*(1-exp(-K*p/Ref))`
- `func infraEfficiency(ns, b)` **will be replaced in Task 7** — for now add `employeeInfraMult` etc. as separate funcs:
  - `employeeInfraMult`, `employeeMarketingMult`, `employeeOpsChurnFactor`

Power formula:

```text
for each role r:
  w = PrimaryWeight if r==Primary else SecondaryWeight (or skill override SecondaryWeight on that employee)
  raw = float64(Stats[r]) * w * selfStatMult * selfRolePowerMult(if applicable)
RolePower[r] += raw
```

Then company skill mults applied in `skillEffects` (Task 6) or partially here.

Salary: sum `MonthlyToPerSec(e.MonthlySalary * selfSalaryMult * companySalaryMult, b)`.

- [ ] **Step 1: Tests**

```go
func TestEmployeeRolePowerPrimarySecondary(t *testing.T) {
	b := balance.Default()
	e := model.Employee{
		PrimaryRole: model.RoleResearcher,
		Stats:       [model.NumRoles]int{80, 40, 40, 40},
	}
	p := employeeRolePower(e, b)
	if p[model.RoleResearcher] <= p[model.RoleEngineer] {
		t.Fatalf("primary should dominate: %+v", p)
	}
}

func TestSeatCapAndFull(t *testing.T) {
	b := balance.Default()
	ns := model.GameState{Office: model.Office{Level: 1}}
	if seatCap(ns, b) != 3 {
		t.Fatal(seatCap(ns, b))
	}
	ns.Employees = make([]model.Employee, 3)
	if !rosterFull(ns, b) {
		t.Fatal("full")
	}
}

func TestSalaryPerSec(t *testing.T) {
	b := balance.Default()
	ns := model.GameState{Employees: []model.Employee{{MonthlySalary: 6000}}}
	got := totalSalaryPerSecFromEmployees(ns, b)
	if math.Abs(got-10) > 1e-9 {
		t.Fatalf("got %v", got)
	}
}

func TestRoleBonusZeroNeutral(t *testing.T) {
	b := balance.Default()
	if roleBonus(0, b) != 0 {
		t.Fatal(roleBonus(0, b))
	}
}
```

- [ ] **Step 2–4: Implement, PASS, commit**

```bash
go test ./internal/sim/ -run 'TestEmployee|TestSeat|TestSalary|TestRoleBonus' -count=1
git add internal/sim/employee.go internal/sim/employee_test.go
git commit -m "feat(sim): employee power, seats, and salary helpers"
```

---

### Task 5: Sim — market generation (deterministic)

**Files:**
- Create: `internal/sim/market.go`
- Create: `internal/sim/market_test.go`

**Interfaces:**
- `func nextRand(state uint64) (uint64, float64)` — already in `events.go` same package; reuse
- `func rollRank(level int, r float64, b balance.Config) model.Rank`
- `func generateEmployee(randState uint64, officeLevel int, gameTime float64, seq int, b balance.Config) (model.Employee, uint64)`
- `func refreshMarket(ns model.GameState, b balance.Config) model.GameState` — fills pool, sets NextRefreshAt = GameTime+RefreshSec, RerollCount=0
- `func ensureMarket(ns model.GameState, b balance.Config) model.GameState` — if Candidates empty or expired, refresh
- `func computeMonthlySalary(rank, stats, skills, multiSpec, b) float64`
- `func computeHireCost(monthly float64, b) float64`

Generation algorithm (must match tests):

1. `randState, u = nextRand` → weighted rank from `RankWeights[level-1]`
2. Multi-spec mode weighted from `MultiSpecWeights`
3. Pick which dims are “high” (shuffle roles with rand)
4. Roll stats in bands
5. `PrimaryRole = PrimaryRoleFromStats`
6. Roll skills by rank rules (Task 5 includes skill pick with family mutex + prefer role ×2 weight)
7. Compute salary + hire cost
8. `ID = fmt.Sprintf("e-%d-%d", int(gameTime), seq)` or hash from rand bits — stable for seed
9. Name from small name pools (fixed arrays + index from rand)

Skill rules:
- Manager: 1 from Manager tier only
- Director: 2 from Manager+Director, ≥1 Director, no God
- God: 3 from all, ≥1 God tier, ≤1 Signature

- [ ] **Step 1: Tests**

```go
func TestGenerateL1NeverGodOrDirector(t *testing.T) {
	b := balance.Default()
	st := uint64(1)
	for i := 0; i < 200; i++ {
		var e model.Employee
		e, st = generateEmployee(st, 1, 0, i, b)
		if e.Rank == model.RankGod || e.Rank == model.RankDirector {
			t.Fatalf("L1 rolled %v", e.Rank)
		}
	}
}

func TestGenerateDeterministic(t *testing.T) {
	b := balance.Default()
	e1, _ := generateEmployee(42, 5, 100, 0, b)
	e2, _ := generateEmployee(42, 5, 100, 0, b)
	if e1.ID != e2.ID || e1.Rank != e2.Rank || e1.Stats != e2.Stats {
		t.Fatalf("%+v vs %+v", e1, e2)
	}
}

func TestManagerSkillsNoGod(t *testing.T) {
	b := balance.Default()
	// Force many rolls at L4+; filter RankManager
	st := uint64(7)
	found := 0
	for i := 0; i < 500 && found < 20; i++ {
		var e model.Employee
		e, st = generateEmployee(st, 6, 0, i, b)
		if e.Rank != model.RankManager {
			continue
		}
		found++
		for _, id := range e.SkillIDs {
			sk, ok := balance.SkillByID(b, id)
			if !ok || sk.Tier == model.SkillTierGod {
				t.Fatalf("manager has god skill %s", id)
			}
		}
		if len(e.SkillIDs) != 1 {
			t.Fatalf("manager skills=%d", len(e.SkillIDs))
		}
	}
	if found < 5 {
		t.Fatal("not enough managers sampled")
	}
}

func TestRefreshMarketPoolSize(t *testing.T) {
	b := balance.Default()
	ns := model.GameState{Office: model.Office{Level: 1}, GameTime: 10, Market: model.TalentMarket{RandState: 9}}
	ns = refreshMarket(ns, b)
	if len(ns.Market.Candidates) != b.MarketPoolSize {
		t.Fatal(len(ns.Market.Candidates))
	}
	if ns.Market.RerollCount != 0 || ns.Market.NextRefreshAt != 10+b.MarketRefreshSec {
		t.Fatalf("%+v", ns.Market)
	}
}
```

- [ ] **Step 2–4: Implement, PASS, commit**

```bash
go test ./internal/sim/ -run 'TestGenerate|TestManagerSkills|TestRefreshMarket' -count=1
git add internal/sim/market.go internal/sim/market_test.go
git commit -m "feat(sim): deterministic talent market generation"
```

---

### Task 6: Sim — Apply hire / fire / upgrade / reroll

**Files:**
- Modify: `internal/sim/apply.go`
- Create or extend: `internal/sim/apply_employee_test.go`
- Remove `applyHireStaff`, `applyFireStaff`, `applySignStar` bodies and switch cases

**Interfaces / errors:**

```go
var (
	ErrOfficeMaxed     = errors.New("sim: office already max level")
	ErrNoSeats         = errors.New("sim: no free office seats")
	ErrUnknownEmployee = errors.New("sim: unknown employee id")
	ErrUnknownCandidate = errors.New("sim: unknown market candidate")
)
```

Behaviors:
- `UpgradeOffice`: cost from `OfficeUpgradeCostAt`; deduct cash; Level++
- `HireEmployee`: find candidate; cash ≥ HireCost; !rosterFull; move to Employees; remove from Candidates; apply hire-cost mult from skills if any
- `FireEmployee`: find employee; cash ≥ severance (after severance mults); deduct; remove from slice (clone)
- `RerollMarket`: cost = RerollCost(RerollCount); deduct; refreshMarket but **preserve** and then set RerollCount = old+1 (refreshMarket zeros it — order carefully: generate new pool, set NextRefreshAt unchanged until free refresh, increment RerollCount)

Actually per spec: paid reroll regenerates pool and increments n; does **not** reset free timer. So:

```go
func applyRerollMarket(...) {
  cost := balance.RerollCost(ns.Market.RerollCount, b)
  // apply skill reroll base mult if any
  if cash < cost { return ErrInsufficientCash }
  ns.Resources.Cash -= cost
  keepRefresh := ns.Market.NextRefreshAt
  keepN := ns.Market.RerollCount
  ns = regenerateCandidatesOnly(ns, b) // new candidates, advance RandState
  ns.Market.NextRefreshAt = keepRefresh
  ns.Market.RerollCount = keepN + 1
  return ns
}
```

- [ ] **Step 1: Tests**

```go
func TestApplyUpgradeOffice(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Office: model.Office{Level: 1}, Resources: model.Resources{Cash: 100_000}}
	ns, err := Apply(s, model.UpgradeOffice{}, b)
	if err != nil || ns.Office.Level != 2 {
		t.Fatalf("err=%v level=%d", err, ns.Office.Level)
	}
	if ns.Resources.Cash != 100_000-25000 {
		t.Fatal(ns.Resources.Cash)
	}
}

func TestApplyHireAndFire(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Office: model.Office{Level: 1},
		Resources: model.Resources{Cash: 1e9},
		Market: model.TalentMarket{
			Candidates: []model.Employee{{
				ID: "c1", HireCost: 1000, MonthlySalary: 2000, Rank: model.RankStaff,
				Stats: [model.NumRoles]int{30, 10, 10, 10}, PrimaryRole: model.RoleResearcher,
			}},
			RandState: 1,
		},
	}
	ns, err := Apply(s, model.HireEmployee{CandidateID: "c1"}, b)
	if err != nil || len(ns.Employees) != 1 || len(ns.Market.Candidates) != 0 {
		t.Fatalf("hire err=%v emp=%d cand=%d", err, len(ns.Employees), len(ns.Market.Candidates))
	}
	// severance 0.5 * 2000 = 1000
	ns2, err := Apply(ns, model.FireEmployee{EmployeeID: "c1"}, b)
	if err != nil || len(ns2.Employees) != 0 {
		t.Fatalf("fire err=%v", err)
	}
}

func TestApplyHireNoSeats(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Office:    model.Office{Level: 1},
		Resources: model.Resources{Cash: 1e9},
		Employees: []model.Employee{{ID: "a"}, {ID: "b"}, {ID: "c"}},
		Market:    model.TalentMarket{Candidates: []model.Employee{{ID: "c1", HireCost: 1}}},
	}
	if _, err := Apply(s, model.HireEmployee{CandidateID: "c1"}, b); !errors.Is(err, ErrNoSeats) {
		t.Fatalf("got %v", err)
	}
}

func TestApplyRerollEscalates(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Office: model.Office{Level: 1}, Resources: model.Resources{Cash: 1e9},
		Market: model.TalentMarket{RandState: 3, NextRefreshAt: 999},
	}
	s = refreshMarket(s, b)
	ns, err := Apply(s, model.RerollMarket{}, b)
	if err != nil || ns.Market.RerollCount != 1 {
		t.Fatalf("err=%v n=%d", err, ns.Market.RerollCount)
	}
	if ns.Market.NextRefreshAt != 999 {
		t.Fatal("reroll must not reset free timer")
	}
}
```

- [ ] **Step 2–4: Implement switch cases + helpers, PASS, commit**

```bash
go test ./internal/sim/ -run 'TestApplyUpgrade|TestApplyHire|TestApplyReroll' -count=1
git add internal/sim/apply.go internal/sim/apply_employee_test.go
git commit -m "feat(sim): hire, fire, office upgrade, market reroll commands"
```

---

### Task 7: Sim — Tick integration + remove aggregate staff/stars

**Files:**
- Modify: `internal/sim/sim.go` — salary, R&D, infra, marketing, ops, auto market refresh
- Modify: `internal/sim/star.go` — delete starEffects usage; replace with `passiveEffects` from employees/skills
- Modify: all sim tests that set `Researchers` / `Engineers` / `HiredStars`
- Modify: `advanceUsers` / `advanceServing` for new mults
- Modify: token R&D path to multiply `TokenRnDMult` from skills
- Modify: training quality path for `TrainQualityMult`
- Modify: `events.go` headcount if it counted researchers

**Tick changes:**

```go
ns = ensureMarket(ns, b) // or only refresh when GameTime >= NextRefreshAt
if ns.GameTime >= ns.Market.NextRefreshAt && ns.Market.NextRefreshAt > 0 {
  ns = refreshMarket(ns, b)
}
// also if NextRefreshAt==0 on brand new, seed in freshRun

staffRnD := staffRnDPerSecFromEmployees(ns, b) * economyDT
// no starRnD
tokenMult := passiveTokenRnDMult(ns, b) // product of skill TokenRnDMult
ns.Resources.RnD += staffRnD*pe.RnDMult + tokenRnD*b.StreakMult*pe.RnDMult*tokenMult
ns.Resources.Cash -= totalSalaryPerSecFromEmployees(ns, b) * economyDT
```

```go
func infraEfficiency(ns, b) float64 {
  // 1 + roleBonus(totalRolePower engineer) + skill infra mults product
  return (1 + roleBonus(totalRolePower(ns,b)[RoleEngineer], b)) * passiveInfraMult(ns,b)
}
```

Marketing: `1 + roleBonus(marketingPower)` * skill user mult  
Ops churn: divide by `(1 + roleBonus(opsPower))` * skill churn mult  

Remove `starEffects` from training completion; fold train quality mults into passive effects.

- [ ] **Step 1: Rewrite `TestStaffRnD` style tests**

```go
func TestEmployeeStaffRnDInTick(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Research: model.Research{EfficiencyMult: 1},
		Employees: []model.Employee{{
			PrimaryRole: model.RoleResearcher,
			Stats:       [model.NumRoles]int{50, 0, 0, 0},
			MonthlySalary: 0,
		}},
		Office: model.Office{Level: 1},
		Market: model.TalentMarket{NextRefreshAt: 1e12, RandState: 1},
	}
	before := s.Resources.RnD
	ns := Tick(s, 10, nil, b)
	if ns.Resources.RnD <= before {
		t.Fatal("expected R&D from employee")
	}
}
```

Fix every broken test under `internal/sim` by either removing star/staff assumptions or seeding employees.

- [ ] **Step 2: Implement integration + delete dead star hire code**

- [ ] **Step 3:**

```bash
go test ./internal/sim/ -count=1
```

Expected: PASS (fix until green).

- [ ] **Step 4: Commit**

```bash
git add internal/sim/
git commit -m "feat(sim): wire employees into tick; remove aggregate staff and stars"
```

---

### Task 8: Prestige / freshRun / campaign restart seeding

**Files:**
- Modify: `internal/sim/prestige.go` — `freshRun`
- Modify: `internal/sim/prestige_test.go` and campaign tests referencing researchers

**freshRun must:**

```go
ns.Office.Level = 1
ns.Employees = nil
ns.Research.EfficiencyMult = 1
// remove StartingResearchersT1 assignment
ns.Market = model.TalentMarket{RandState: /* preserve events rand or new from p */ s.Events.RandState}
ns = refreshMarket(ns, b)
ns.Resources.Cash = b.StartingCash + pe.StartCash
// ...
```

If `StartingResearchersT1` unused, remove from balance Config in this task and fix `Default()`.

- [ ] **Step 1: Test**

```go
func TestFreshRunSeedsOfficeAndMarket(t *testing.T) {
	b := balance.Default()
	ns := freshRun(model.Prestige{}, b)
	if ns.Office.Level != 1 || len(ns.Employees) != 0 {
		t.Fatalf("%+v", ns)
	}
	if len(ns.Market.Candidates) != b.MarketPoolSize {
		t.Fatal(len(ns.Market.Candidates))
	}
}
```

- [ ] **Step 2–4: Implement, `go test ./internal/sim/ -run Prestige -count=1`, commit**

```bash
git commit -m "feat(sim): seed office and talent market on fresh run"
```

---

### Task 9: Store — schema 2 + migrate legacy staff/stars

**Files:**
- Modify: `internal/store/store.go` — `CurrentSchemaVersion = 2`
- Modify: `internal/store/migrate.go` — migration from v1 (and v0 path)
- Modify: `internal/store/store_test.go`, `migrate_test.go`

**Migration rules (spec §8):**

When loading state that has legacy JSON fields OR empty Office with old staff:

JSON unmarshalling: removed fields are ignored. Detect legacy via:
- `SchemaVersion < 2`, OR
- helper: if `Office.Level == 0` and (after unmarshal) we need defaults

On migrate to employee system:

```go
func migrateToEmployeeOffice(s model.GameState, b balance.Config) model.GameState {
	// Compensation if we can detect from a side channel — for v1 saves, Engineers etc. are LOST on unmarshal.
	// Approach: bump schema; for any load with Office.Level==0:
	s.Office.Level = 1
	if s.Employees == nil {
		s.Employees = []model.Employee{}
	}
	// Cash: add flat RestructuringBonus once if GameTime>0 && no employees && cash compensation flag
	// Spec: $2000 per head + $50k per star — without legacy fields, use fixed RestructuringGrant for mid-run saves:
	if s.GameTime > 0 && len(s.Employees) == 0 {
		s.Resources.Cash += b.RestructuringGrant // e.g. 25000 constant on Config
	}
	if len(s.Market.Candidates) == 0 {
		if s.Market.RandState == 0 {
			s.Market.RandState = 1
		}
		s = sim.RefreshMarketExported or duplicate refresh call
	}
	return s
}
```

To preserve compensation accuracy, add optional legacy DTO for one version:

```go
type legacyStaffFields struct {
	Engineers, Ops, Marketing int
	Researchers               [4]int
	HiredStars                []string
}
```

Decode raw JSON twice: full state + probe legacy fields before they were removed — **if types removed, probe with anonymous struct on raw message** in Load:

```go
var probe struct {
	State struct {
		Engineers  int      `json:"Engineers"`
		Ops        int      `json:"Ops"`
		Marketing  int      `json:"Marketing"`
		HiredStars []string `json:"HiredStars"`
		Research   struct {
			Researchers [4]int `json:"Researchers"`
		} `json:"Research"`
	} `json:"state"`
}
```

Compensation = 2000*(sum researchers+eng+ops+mkt) + 50000*len(stars).

- [ ] **Step 1: Tests for probe compensation + Office default**

- [ ] **Step 2–4: Implement, `go test ./internal/store/ -count=1`, commit**

```bash
git commit -m "feat(store): schema v2 migrate to employee office system"
```

---

### Task 10: TUI — ASCII HQ from Office.Level

**Files:**
- Modify: `internal/tui/ascii_hq.go`
- Modify: `internal/tui/ascii_hq_test.go`
- Modify: callers in `page_overview.go` that pass `MilestonesReached`

**Change:**

```go
// hqStageFromOffice maps Office.Level (1..8) to art index 0..7.
func hqStageFromOffice(level int) int {
	if level < 1 {
		level = 1
	}
	if level > 8 {
		level = 8
	}
	return level - 1
}
```

Update `hqStageNames` to match balance office names where possible.

Replace `hqStage(m.state.MilestonesReached)` with `hqStageFromOffice(m.state.Office.Level)`.

- [ ] **Step 1: Test office level 1 → 車庫 art index 0; level 8 → last art**

- [ ] **Step 2–4: Implement, test, commit**

```bash
git commit -m "feat(tui): drive ASCII HQ from office level"
```

---

### Task 11: TUI — Team page roster + market + keys

**Files:**
- Modify: `internal/tui/page_team.go`
- Modify: `internal/tui/page_team_test.go`
- Modify: `internal/tui/tui.go` (key handling for PageTeam)
- Modify: any notice strings for hire/fire

**UI content:**
1. Office card: art or name, `工位 a/b`, upgrade cost, key `u`
2. Roster: name, rank ZH, primary role ZH, `月薪 $X`, skill count; total monthly payroll
3. Market: up to 5 candidates with stats blurb, hire cost, monthly; refresh countdown; next reroll price

**Keys (PageTeam only):**
- `u` → `UpgradeOffice`
- `h` → hire focused candidate (focus index on model, default 0)
- `f` → fire focused employee
- `r` → `RerollMarket`
- `j`/`k` or left/right → move focus (minimal: market index 0..n-1, roster 0..m-1; start with market focus only if simpler)

Display money with existing `human()` helper; always label `/月`.

Rank ZH map: 雜魚/職員/幹部/經理/總監/大神.

- [ ] **Step 1: Tests for render contains 月薪 and 工位; key `u` dispatches upgrade when cash ok**

- [ ] **Step 2–4: Implement, `go test ./internal/tui/ -count=1`, commit**

```bash
git commit -m "feat(tui): team page for employees, market, office upgrade"
```

---

### Task 12: Full compile sweep + balance cleanup

**Files:** any remaining references to Engineers, HiredStars, HireStaff, Stars, StaffTier researchers, starEffects, StartingResearchersT1, DefaultStars, ResearcherHireCost, etc.

**Steps:**

- [ ] **Step 1:**

```bash
go test ./... -count=1 2>&1 | head -100
```

- [ ] **Step 2:** Fix every failure. Common fixes:
  - Tests: seed one researcher employee instead of `Researchers[Tier1]=n`
  - Remove star roster tests; replace with market/skill tests if needed
  - View layer R&D rate displays
  - Events that counted staff headcount → `len(Employees)`
  - balance_test old staff values → delete or replace

- [ ] **Step 3:**

```bash
gofmt -w internal/
go test ./... -count=1
go vet ./...
go build -o /dev/null .
```

Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "fix: finish employee office refactor compile and test sweep"
```

- [ ] **Step 5: Update design status line** (optional same commit or follow-up)

In spec front matter: `狀態：設計已確認，實作計畫已備妥` → after full implement `已實作` only when code done.

---

## Spec coverage checklist (self-review)

| Spec requirement | Task |
|---|---|
| Four roles maintained | 1, 4, 5 |
| Recruit UI + candidates | 5, 6, 11 |
| Six ranks 雜魚…大神 | 1, 2, 5 |
| 4 stats + multi-spec high | 5 |
| Skills manager1/dir2/god3 | 3, 5 |
| God skills not on manager | 5 tests |
| Skill tier pools ~57 | 3 |
| Salary ∝ stats+skills; monthly UI | 2, 4, 5, 11 |
| Office upgrade seats + ASCII | 2, 6, 10, 11 |
| Fire + severance | 2, 6 |
| Rarity ∝ office level | 2, 5 |
| Hybrid market + geometric reroll | 2, 5, 6 |
| Full replace staff+stars | 1, 7, 12 |
| Migrate saves | 9 |
| Deterministic sim RNG | 5 |

## Placeholder scan

No TBD steps; skill numeric fields are specified by convention (mult neutral 1.0). `RnDPerPower` initial value may be tuned in Task 12 if economy tests fail — adjust only that constant, not architecture.

## Type consistency

- `model.Rank` / `model.Employee` / `model.Office` / `model.TalentMarket` used uniformly
- Commands: `UpgradeOffice`, `HireEmployee`, `FireEmployee`, `RerollMarket`
- Helpers: `refreshMarket`, `generateEmployee`, `seatCap`, `totalSalaryPerSecFromEmployees`, `staffRnDPerSecFromEmployees`

---

## Execution handoff

Plan saved to `docs/superpowers/plans/2026-07-13-employee-office-refactor.md`.

**Two execution options:**

1. **Subagent-Driven (recommended)** — fresh subagent per task, review between tasks  
2. **Inline Execution** — execute in this session with checkpoints  

Which approach?
