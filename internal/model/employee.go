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
	Candidates    []Employee
	NextRefreshAt float64
	RerollCount   int
	RandState     uint64
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

// MultiSpecCount returns how many role stats are ≥ highThreshold.
func MultiSpecCount(stats [NumRoles]int, highThreshold int) int {
	n := 0
	for _, v := range stats {
		if v >= highThreshold {
			n++
		}
	}
	return n
}
