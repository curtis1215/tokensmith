package sim

import (
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func isStarHired(ns model.GameState, id string) bool {
	for _, h := range ns.HiredStars {
		if h == id {
			return true
		}
	}
	return false
}

// starEffects aggregates the bonuses of all hired stars (neutral when none).
func starEffects(ns model.GameState, b balance.Config) model.StarEffects {
	agg := model.NeutralStarEffects()
	for _, st := range b.Stars {
		if !isStarHired(ns, st.ID) {
			continue
		}
		for d := range agg.QualityMult {
			agg.QualityMult[d] *= st.Effects.QualityMult[d]
		}
		agg.RnDPerSec += st.Effects.RnDPerSec
		agg.InfraMult *= st.Effects.InfraMult
		agg.UserGrowthMult *= st.Effects.UserGrowthMult
	}
	return agg
}
