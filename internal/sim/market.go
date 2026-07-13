package sim

import (
	"fmt"
	"math"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// Fixed name pools for deterministic candidate naming (index from nextRand).
var marketSurnames = []string{
	"林", "陳", "張", "李", "王", "黃", "吳", "劉", "蔡", "楊",
	"許", "鄭", "謝", "郭", "洪", "邱", "曾", "廖", "賴", "徐",
}

var marketGivenNames = []string{
	"明遠", "志豪", "雅婷", "佳穎", "建宏", "淑芬", "家豪", "怡君",
	"俊傑", "詩涵", "冠宇", "佩珊", "子軒", "心怡", "宇翔", "雨萱",
	"柏翰", "宜庭", "彥廷", "安琦", "承恩", "佳蓉", "浩宇", "思穎",
}

// rollRank maps a uniform r∈[0,1) onto RankWeights[level-1].
// Level is clamped to 1..8; zero-weight ranks are skipped.
func rollRank(level int, r float64, b balance.Config) model.Rank {
	if level < 1 {
		level = 1
	}
	if level > 8 {
		level = 8
	}
	weights := b.RankWeights[level-1]
	total := 0.0
	for _, w := range weights {
		if w > 0 {
			total += w
		}
	}
	if total <= 0 {
		return model.RankGrunt
	}
	if r < 0 {
		r = 0
	}
	if r >= 1 {
		r = 0.9999999999999999
	}
	pick := r * total
	acc := 0.0
	last := model.RankGrunt
	for i, w := range weights {
		if w <= 0 {
			continue
		}
		acc += w
		last = model.Rank(i)
		if pick < acc {
			return last
		}
	}
	return last
}

// computeMonthlySalary implements design §4.2.
// multiSpec is high-dim count 1..4 (single/dual/tri/quad).
func computeMonthlySalary(
	rank model.Rank,
	stats [model.NumRoles]int,
	skillIDs []string,
	multiSpec int,
	b balance.Config,
) float64 {
	if rank < 0 || int(rank) >= model.NumRanks {
		rank = model.RankGrunt
	}
	base := b.RankBaseMonth[rank]
	primary := model.PrimaryRoleFromStats(stats)
	ss := salaryStatScore(stats, primary, b.SecondaryWeight)
	sks := salarySkillScore(skillIDs, b)
	ms := 1.0
	if multiSpec >= 1 && multiSpec <= 4 {
		ms = b.MultiSpecSalaryMult[multiSpec-1]
	}
	return base * (1 + b.SalaryStatFactor*ss) * (1 + b.SalarySkillFactor*sks) * ms
}

// computeHireCost is MonthlySalary × HireMonths.
func computeHireCost(monthly float64, b balance.Config) float64 {
	return monthly * b.HireMonths
}

func salaryStatScore(stats [model.NumRoles]int, primary model.Role, secW float64) float64 {
	primaryStat := float64(stats[primary])
	sumSec := 0.0
	nSec := 0
	for r := model.Role(0); r < model.NumRoles; r++ {
		if r == primary {
			continue
		}
		sumSec += float64(stats[r])
		nSec++
	}
	meanSec := 0.0
	if nSec > 0 {
		meanSec = sumSec / float64(nSec)
	}
	return (primaryStat + secW*meanSec) / 100
}

func salarySkillScore(skillIDs []string, b balance.Config) float64 {
	var s float64
	for _, id := range skillIDs {
		sk, ok := balance.SkillByID(b, id)
		if !ok {
			continue
		}
		switch sk.Tier {
		case model.SkillTierManager:
			s += 1
		case model.SkillTierDirector:
			s += 2
		case model.SkillTierGod:
			s += 3.5
		}
		if sk.Signature {
			s += 2
		}
	}
	return s
}

// marketOfficeLevel is office level plus hired MarketRarityBonus (soft-cap +2),
// used only for talent-market rank weights. Clamped to 1..8.
func marketOfficeLevel(ns model.GameState, b balance.Config) int {
	base := effectiveOfficeLevel(ns)
	bonus := 0.0
	for _, e := range ns.Employees {
		for _, id := range e.SkillIDs {
			sk, ok := balance.SkillByID(b, id)
			if !ok || sk.MarketRarityBonus <= 0 {
				continue
			}
			bonus += sk.MarketRarityBonus
		}
	}
	if bonus > 2 {
		bonus = 2
	}
	level := base + int(math.Round(bonus))
	if level < 1 {
		level = 1
	}
	if level > 8 {
		level = 8
	}
	return level
}

// generateEmployee rolls one market candidate and advances randState.
// ID embeds gameTime, seq, and pre-roll RandState bits so hire+reroll at the
// same GameTime cannot collide with retained roster IDs.
func generateEmployee(
	randState uint64,
	officeLevel int,
	gameTime float64,
	seq int,
	b balance.Config,
) (model.Employee, uint64) {
	if officeLevel < 1 {
		officeLevel = 1
	}
	if officeLevel > 8 {
		officeLevel = 8
	}

	// Capture uniqueness bits before advancing RNG for this roll.
	idTag := randState

	var u float64
	randState, u = nextRand(randState)
	rank := rollRank(officeLevel, u, b)

	var multiSpec int
	multiSpec, randState = rollMultiSpec(randState, b)

	var stats [model.NumRoles]int
	stats, randState = rollStats(rank, multiSpec, randState, b)
	primary := model.PrimaryRoleFromStats(stats)

	var skillIDs []string
	skillIDs, randState = rollSkills(rank, primary, randState, b)

	monthly := computeMonthlySalary(rank, stats, skillIDs, multiSpec, b)
	hire := computeHireCost(monthly, b)

	var name string
	name, randState = rollName(randState)

	e := model.Employee{
		ID:            fmt.Sprintf("e-%d-%d-%x", int(gameTime), seq, idTag),
		Name:          name,
		Rank:          rank,
		Stats:         stats,
		PrimaryRole:   primary,
		SkillIDs:      skillIDs,
		HireCost:      hire,
		MonthlySalary: monthly,
	}
	return e, randState
}

// regenerateCandidatesOnly rolls a fresh candidate pool and advances RandState
// without touching NextRefreshAt or RerollCount (used by paid reroll).
func regenerateCandidatesOnly(ns model.GameState, b balance.Config) model.GameState {
	level := marketOfficeLevel(ns, b)
	st := ns.Market.RandState
	n := b.MarketPoolSize
	if n < 0 {
		n = 0
	}
	cands := make([]model.Employee, 0, n)
	for i := 0; i < n; i++ {
		var e model.Employee
		e, st = generateEmployee(st, level, ns.GameTime, i, b)
		cands = append(cands, e)
	}
	ns.Market.Candidates = cands
	ns.Market.RandState = st
	return ns
}

// RefreshMarket regenerates the candidate pool, resets RerollCount, and
// schedules the next free refresh at GameTime+MarketRefreshSec.
// Exported for store migration (schema v2 employee office seed).
func RefreshMarket(ns model.GameState, b balance.Config) model.GameState {
	ns = regenerateCandidatesOnly(ns, b)
	ns.Market.NextRefreshAt = ns.GameTime + b.MarketRefreshSec
	ns.Market.RerollCount = 0
	return ns
}

// ensureMarket refreshes when the pool is empty or the free timer has expired.
func ensureMarket(ns model.GameState, b balance.Config) model.GameState {
	if len(ns.Market.Candidates) == 0 || ns.GameTime >= ns.Market.NextRefreshAt {
		return RefreshMarket(ns, b)
	}
	return ns
}

func rollMultiSpec(st uint64, b balance.Config) (int, uint64) {
	total := 0.0
	for _, w := range b.MultiSpecWeights {
		if w > 0 {
			total += w
		}
	}
	st, u := nextRand(st)
	if total <= 0 {
		return 1, st
	}
	pick := u * total
	acc := 0.0
	last := 1
	for i, w := range b.MultiSpecWeights {
		if w <= 0 {
			continue
		}
		acc += w
		last = i + 1
		if pick < acc {
			return last, st
		}
	}
	return last, st
}

func rollStats(rank model.Rank, multiSpec int, st uint64, b balance.Config) ([model.NumRoles]int, uint64) {
	var stats [model.NumRoles]int
	if rank < 0 || int(rank) >= model.NumRanks {
		rank = model.RankGrunt
	}
	if multiSpec < 1 {
		multiSpec = 1
	}
	if multiSpec > model.NumRoles {
		multiSpec = model.NumRoles
	}

	order, st := shuffleRoles(st)
	highLo, highHi := b.RankStatHigh[rank][0], b.RankStatHigh[rank][1]
	normLo, normHi := b.RankStatNorm[rank][0], b.RankStatNorm[rank][1]
	floor := b.RankStatFloor[rank]

	for i, role := range order {
		lo, hi := normLo, normHi
		if i < multiSpec {
			lo, hi = highLo, highHi
		}
		var v int
		v, st = rollInBand(lo, hi, st)
		if v < floor {
			v = floor
		}
		stats[role] = v
	}
	return stats, st
}

func shuffleRoles(st uint64) ([model.NumRoles]model.Role, uint64) {
	var roles [model.NumRoles]model.Role
	for i := 0; i < model.NumRoles; i++ {
		roles[i] = model.Role(i)
	}
	for i := model.NumRoles - 1; i > 0; i-- {
		var u float64
		st, u = nextRand(st)
		j := int(u * float64(i+1))
		if j > i {
			j = i
		}
		roles[i], roles[j] = roles[j], roles[i]
	}
	return roles, st
}

func rollInBand(lo, hi int, st uint64) (int, uint64) {
	if hi < lo {
		hi = lo
	}
	span := hi - lo + 1
	st, u := nextRand(st)
	v := lo + int(u*float64(span))
	if v > hi {
		v = hi
	}
	if v < lo {
		v = lo
	}
	return v, st
}

func rollName(st uint64) (string, uint64) {
	var u float64
	st, u = nextRand(st)
	si := int(u * float64(len(marketSurnames)))
	if si >= len(marketSurnames) {
		si = len(marketSurnames) - 1
	}
	st, u = nextRand(st)
	gi := int(u * float64(len(marketGivenNames)))
	if gi >= len(marketGivenNames) {
		gi = len(marketGivenNames) - 1
	}
	return marketSurnames[si] + marketGivenNames[gi], st
}

// rollSkills applies rank skill-slot rules, family mutex, and PreferRole ×2.
func rollSkills(
	rank model.Rank,
	primary model.Role,
	st uint64,
	b balance.Config,
) ([]string, uint64) {
	switch rank {
	case model.RankManager:
		pool := balance.SkillsByTier(b, model.SkillTierManager)
		id, st2, ok := pickWeightedSkill(pool, nil, false, true, primary, st)
		st = st2
		if !ok {
			return nil, st
		}
		return []string{id}, st

	case model.RankDirector:
		// ≥1 Director: force first pick from Director tier; second from M+D.
		usedFamily := map[string]bool{}
		var ids []string
		dirPool := balance.SkillsByTier(b, model.SkillTierDirector)
		id, st, ok := pickWeightedSkill(dirPool, usedFamily, false, true, primary, st)
		if ok {
			ids = append(ids, id)
			if sk, found := balance.SkillByID(b, id); found && sk.Family != "" {
				usedFamily[sk.Family] = true
			}
		}
		md := append(
			balance.SkillsByTier(b, model.SkillTierManager),
			balance.SkillsByTier(b, model.SkillTierDirector)...,
		)
		id, st, ok = pickWeightedSkill(md, usedFamily, false, true, primary, st)
		if ok {
			ids = append(ids, id)
		}
		return ids, st

	case model.RankGod:
		// ≥1 God tier, ≤1 Signature, 3 total from all tiers.
		usedFamily := map[string]bool{}
		hasSig := false
		var ids []string

		godPool := balance.SkillsByTier(b, model.SkillTierGod)
		id, st, ok := pickWeightedSkill(godPool, usedFamily, hasSig, true, primary, st)
		if ok {
			ids = append(ids, id)
			if sk, found := balance.SkillByID(b, id); found {
				if sk.Family != "" {
					usedFamily[sk.Family] = true
				}
				if sk.Signature {
					hasSig = true
				}
			}
		}

		all := append([]balance.SkillDef(nil), b.Skills...)
		for len(ids) < 3 {
			id, st2, ok2 := pickWeightedSkill(all, usedFamily, hasSig, true, primary, st)
			st = st2
			if !ok2 {
				break
			}
			ids = append(ids, id)
			if sk, found := balance.SkillByID(b, id); found {
				if sk.Family != "" {
					usedFamily[sk.Family] = true
				}
				if sk.Signature {
					hasSig = true
				}
			}
		}
		return ids, st

	default:
		// Grunt / Staff / Lead: no skills.
		return nil, st
	}
}

// pickWeightedSkill selects one skill: base weight 1, PreferRole match ×2.
// usedFamily enforces effect-family mutex; hasSig + allowSig gate signatures.
func pickWeightedSkill(
	pool []balance.SkillDef,
	usedFamily map[string]bool,
	hasSig bool,
	allowSig bool,
	primary model.Role,
	st uint64,
) (id string, newState uint64, ok bool) {
	type cand struct {
		sk balance.SkillDef
		w  float64
	}
	cands := make([]cand, 0, len(pool))
	for _, sk := range pool {
		if usedFamily != nil && sk.Family != "" && usedFamily[sk.Family] {
			continue
		}
		if sk.Signature {
			if hasSig || !allowSig {
				continue
			}
		}
		w := 1.0
		if sk.HasPrefer && sk.PreferRole == primary {
			w = 2.0
		}
		cands = append(cands, cand{sk: sk, w: w})
	}
	if len(cands) == 0 {
		return "", st, false
	}
	total := 0.0
	for _, c := range cands {
		total += c.w
	}
	st, u := nextRand(st)
	pick := u * total
	acc := 0.0
	for _, c := range cands {
		acc += c.w
		if pick < acc {
			return c.sk.ID, st, true
		}
	}
	return cands[len(cands)-1].sk.ID, st, true
}
