package sim

import (
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// BoostRefMonthlyCash is gross monthly cash at EffectiveRefPrice (not sticker),
// including campaign/prestige/global revenue mults used by Tick accrual.
func BoostRefMonthlyCash(s model.GameState, b balance.Config) float64 {
	pe := PrestigeEffects(s.Prestige.UnlockedPrestige, b)
	ce := campaignEffects(s, b)
	var total float64
	for _, m := range s.Models {
		if !m.Online {
			continue
		}
		if int(m.Segment) < 0 || int(m.Segment) >= model.NumSegments {
			continue
		}
		ref := EffectiveRefPrice(s, m.Segment, b)
		total += m.Users * ref * ce.RevenueMult[m.Segment] * pe.CashMult * b.RevenueMult
	}
	return total
}

func TrainBoostRefMonthly(s model.GameState, b balance.Config) float64 {
	ref := BoostRefMonthlyCash(s, b)
	if ref < b.TrainBoostFloorMonthly {
		return b.TrainBoostFloorMonthly
	}
	return ref
}

func QuoteTrainBoostCost(s model.GameState, gen int, boosts [model.NumQualityDims]bool, b balance.Config) (float64, error) {
	return balance.TrainBoostCashCost(gen, TrainBoostRefMonthly(s, b), boosts, b)
}

func PredictedTrainQuality(s model.GameState, gen int, alloc [model.NumQualityDims]float64, boosts [model.NumQualityDims]bool, b balance.Config) ([model.NumQualityDims]float64, error) {
	var out [model.NumQualityDims]float64
	spec, err := balance.Generation(gen)
	if err != nil {
		return out, err
	}
	bonus, err := balance.TrainBoostCashBonus(gen, boosts, b)
	if err != nil {
		return out, err
	}
	te := techEffects(s, b)
	se := starEffects(s, b)
	for d := range model.NumQualityDims {
		out[d] = (alloc[d]*spec.QualityScale + bonus[d]) * te.QualityMult[d] * se.QualityMult[d]
	}
	return out, nil
}

// EffectiveRivalQuality is the sole authority for rival quality reads (v1: stored).
func EffectiveRivalQuality(s model.GameState, rival model.Competitor, b balance.Config) [model.NumQualityDims]float64 {
	_ = s
	_ = b
	return rival.Quality
}
