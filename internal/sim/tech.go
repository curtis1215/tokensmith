package sim

import (
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// isUnlocked reports whether a tech node ID has been unlocked.
func isUnlocked(ns model.GameState, id string) bool {
	for _, u := range ns.UnlockedTech {
		if u == id {
			return true
		}
	}
	return false
}

// MaxUnlockedGen is the highest model generation the player may train: gen 1 is
// always available; higher gens require the chained model-gen tech nodes.
func MaxUnlockedGen(ns model.GameState, b balance.Config) int {
	g := 1
	for n := 2; n <= balance.MaxGen; n++ {
		if !isUnlocked(ns, balance.GenUnlockNodeID(n)) {
			break
		}
		g = n
	}
	return g
}

// techEffects aggregates the multipliers of all unlocked tech nodes.
// Iterates the catalog (deterministic order); neutral when nothing unlocked.
func techEffects(ns model.GameState, b balance.Config) model.TechEffects {
	agg := model.NeutralTechEffects()
	for _, node := range b.TechNodes {
		if !isUnlocked(ns, node.ID) {
			continue
		}
		for d := range agg.QualityMult {
			agg.QualityMult[d] *= node.Effects.QualityMult[d]
		}
		agg.TrainRnDMult *= node.Effects.TrainRnDMult
		agg.TrainWorkMult *= node.Effects.TrainWorkMult
		agg.InfraMult *= node.Effects.InfraMult
		agg.UserGrowthMult *= node.Effects.UserGrowthMult
		agg.RefPriceMult *= node.Effects.RefPriceMult
		agg.IncidentMult *= node.Effects.IncidentMult
	}
	return agg
}
