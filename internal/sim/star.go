package sim

import (
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// starBonus is the legacy star-effects aggregate shape retained so training,
// users, and view keep compiling. Stars catalog was removed; effects are neutral
// until Task 6–7 rewires callers onto employee/skill helpers.
type starBonus struct {
	QualityMult    [model.NumQualityDims]float64
	RnDPerSec      float64
	InfraMult      float64
	UserGrowthMult float64
}

// starEffects returns neutral multipliers (no stars). Temporary stub.
func starEffects(_ model.GameState, _ balance.Config) starBonus {
	var se starBonus
	for i := range se.QualityMult {
		se.QualityMult[i] = 1
	}
	se.InfraMult = 1
	se.UserGrowthMult = 1
	return se
}
