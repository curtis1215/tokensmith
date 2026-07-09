package sim

import (
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// nextRand advances a splitmix64 state and returns the new state plus a
// uniform float64 in [0,1). All event randomness flows through this so the
// sim stays deterministic: same GameState → same rolls.
func nextRand(state uint64) (uint64, float64) {
	state += 0x9E3779B97F4A7C15
	z := state
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	z ^= z >> 31
	return state, float64(z>>11) / float64(1<<53)
}

// eventEffects folds all active event modifiers into one multiplier set
// (neutral when none). TechCostMult is branch-targeted and deliberately NOT
// aggregated here — use eventTechCostMult.
func eventEffects(ns model.GameState, b balance.Config) model.EventEffects {
	agg := model.NeutralEventEffects()
	for _, m := range ns.Events.Active {
		agg.BuildCostMult *= m.Effects.BuildCostMult
		agg.PowerCostMult *= m.Effects.PowerCostMult
		agg.RefPriceMult *= m.Effects.RefPriceMult
		agg.UserGrowthMult *= m.Effects.UserGrowthMult
		agg.TAMMult *= m.Effects.TAMMult
		agg.ValuationMult *= m.Effects.ValuationMult
		agg.SafetyWeightMult *= m.Effects.SafetyWeightMult
		agg.IncidentChanceMult *= m.Effects.IncidentChanceMult
	}
	return agg
}

// eventTechCostMult is the product of active TechCostMult modifiers that
// target the given tech branch.
func eventTechCostMult(ns model.GameState, branch model.TechBranch) float64 {
	mult := 1.0
	for _, m := range ns.Events.Active {
		if m.Effects.TechCostMult != 1 && m.Target == int(branch) {
			mult *= m.Effects.TechCostMult
		}
	}
	return mult
}
