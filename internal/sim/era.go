package sim

import (
	"math"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// firstFrontierGen is the generation whose FrontierRnD sets era breakthrough base cost.
// Era III → Gen6; Era IV → Gen8; Era V+ → the era's first generation.
func firstFrontierGen(era int) (int, error) {
	if era < 3 {
		return 0, ErrEraNotOpen
	}
	switch era {
	case 3:
		return 6, nil
	case 4:
		return 8, nil
	default:
		return balance.EraStartGen(era)
	}
}

// EraBreakthroughBaseCost is 0.25 × FrontierRnD of the era's first frontier gen.
func EraBreakthroughBaseCost(era int) (float64, error) {
	gen, err := firstFrontierGen(era)
	if err != nil {
		return 0, err
	}
	spec, err := balance.Generation(gen)
	if err != nil {
		return 0, err
	}
	return 0.25 * spec.FrontierRnD, nil
}

// EraBreakthroughCost returns the R&D cost for unlocking branch in era.
// The first breakthrough in an era costs 1.0× base; later ones cost 1.75×.
// Ownership is not checked here — Apply enforces duplicates.
func EraBreakthroughCost(s model.GameState, era int, branch model.TechBranch) (float64, error) {
	if branch < 0 || int(branch) >= model.NumBranches {
		return 0, ErrInvalidEraBranch
	}
	if !EraOpen(s, era) {
		return 0, ErrEraNotOpen
	}
	base, err := EraBreakthroughBaseCost(era)
	if err != nil {
		return 0, err
	}
	if ep, ok := findEraProgress(s, era); ok && ep.HasPrimary {
		return base * 1.75, nil
	}
	return base, nil
}

// EraOpen reports whether procedural breakthroughs for era may be purchased.
// Eras I–II stay fixed-tech history. Era III opens after Gen4 (end of Era II).
// Later eras require the previous era's final generation plus two breakthroughs.
func EraOpen(s model.GameState, era int) bool {
	if era < 3 {
		return false
	}
	prevEnd, err := balance.EraEndGen(era - 1)
	if err != nil {
		return false
	}
	if MaxUnlockedGen(s, balance.Config{}) < prevEnd {
		return false
	}
	if era == 3 {
		return true
	}
	return eraBreakthroughCount(s, era-1) >= 2
}

// EraEffects aggregates square-root diminishing multipliers from all era
// breakthroughs. Neutral when no era progress is recorded.
func EraEffects(s model.GameState) model.TechEffects {
	e := model.NeutralTechEffects()
	var counts [model.NumBranches]int
	for _, ep := range s.Progression.Eras {
		for b := 0; b < model.NumBranches; b++ {
			if ep.UnlockedMask&(1<<b) != 0 {
				counts[b]++
			}
		}
	}
	algo := math.Sqrt(float64(counts[model.BranchAlgo]))
	infra := math.Sqrt(float64(counts[model.BranchInfra]))
	biz := math.Sqrt(float64(counts[model.BranchBusiness]))
	align := math.Sqrt(float64(counts[model.BranchAlignment]))

	e.QualityMult[model.DimCapability] = 1 + 0.05*algo
	e.InfraMult = 1 + 0.08*infra
	e.UserGrowthMult = 1 + 0.06*biz
	e.RefPriceMult = 1 + 0.03*biz
	e.QualityMult[model.DimSafety] = 1 + 0.06*align
	e.IncidentMult = 1 / (1 + 0.10*align)
	return e
}

func findEraProgress(s model.GameState, era int) (model.EraProgress, bool) {
	for _, ep := range s.Progression.Eras {
		if ep.Era == era {
			return ep, true
		}
	}
	return model.EraProgress{}, false
}

func eraBreakthroughCount(s model.GameState, era int) int {
	ep, ok := findEraProgress(s, era)
	if !ok {
		return 0
	}
	return bitsSet(ep.UnlockedMask)
}

func bitsSet(m uint8) int {
	n := 0
	for m != 0 {
		n += int(m & 1)
		m >>= 1
	}
	return n
}

// upsertEraProgress inserts or replaces the entry for era, keeping Eras sorted.
func upsertEraProgress(eras []model.EraProgress, ep model.EraProgress) []model.EraProgress {
	out := make([]model.EraProgress, 0, len(eras)+1)
	inserted := false
	for _, cur := range eras {
		if cur.Era == ep.Era {
			out = append(out, ep)
			inserted = true
			continue
		}
		if !inserted && cur.Era > ep.Era {
			out = append(out, ep)
			inserted = true
		}
		out = append(out, cur)
	}
	if !inserted {
		out = append(out, ep)
	}
	return out
}

func applyUnlockEraBreakthrough(s model.GameState, c model.UnlockEraBreakthrough, _ balance.Config) (model.GameState, error) {
	if c.Branch < 0 || int(c.Branch) >= model.NumBranches {
		return s, ErrInvalidEraBranch
	}
	if !EraOpen(s, c.Era) {
		return s, ErrEraNotOpen
	}
	ep, ok := findEraProgress(s, c.Era)
	if !ok {
		ep = model.EraProgress{Era: c.Era}
	}
	bit := uint8(1 << c.Branch)
	if ep.UnlockedMask&bit != 0 {
		return s, ErrEraBreakthroughOwned
	}
	cost, err := EraBreakthroughCost(s, c.Era, c.Branch)
	if err != nil {
		return s, err
	}
	if s.Resources.RnD < cost {
		return s, ErrInsufficientRnD
	}
	ns := s
	ns.Resources.RnD -= cost
	if !ep.HasPrimary {
		ep.HasPrimary = true
		ep.Primary = c.Branch
	}
	ep.UnlockedMask |= bit
	// Clone eras slice for purity.
	ns.Progression.Eras = upsertEraProgress(append([]model.EraProgress(nil), s.Progression.Eras...), ep)
	return ns, nil
}
