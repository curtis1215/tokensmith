package sim

import (
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// legacyGenUnlockMax is the highest generation unlocked via fixed tech nodes
// (model-gen-2 .. model-gen-5). Gen6+ advances Progression via frontier research.
const legacyGenUnlockMax = 5

// isUnlocked reports whether a tech node ID has been unlocked.
func isUnlocked(ns model.GameState, id string) bool {
	for _, u := range ns.UnlockedTech {
		if u == id {
			return true
		}
	}
	return false
}

// MaxUnlockedGen is the highest model generation the player may train.
// Gen 1 is always available. Contiguous legacy model-gen-N tech nodes (2–5)
// and Progression.MaxUnlockedGen (frontier unlocks) are reconciled; the result
// is always at least 1.
func MaxUnlockedGen(ns model.GameState, _ balance.Config) int {
	legacy := 1
	for n := 2; n <= legacyGenUnlockMax; n++ {
		if !isUnlocked(ns, balance.GenUnlockNodeID(n)) {
			break
		}
		legacy = n
	}
	g := ns.Progression.MaxUnlockedGen
	if g < 1 {
		g = 1
	}
	if legacy > g {
		return legacy
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
