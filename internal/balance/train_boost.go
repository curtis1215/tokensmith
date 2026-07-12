package balance

import (
	"math"

	"tokensmith/internal/model"
)

// TrainBoost is one cash-paid consumable that boosts a quality dimension at train time.
type TrainBoost struct {
	Dim        model.QualityDim
	ID         string
	NameZH     string
	RoleWeight float64
}

// DefaultTrainBoosts returns the four Chinese-named consumables, one per quality dim.
func DefaultTrainBoosts() []TrainBoost {
	return []TrainBoost{
		{Dim: model.DimCapability, ID: "boost-data", NameZH: "優質語料", RoleWeight: 1.2},
		{Dim: model.DimEfficiency, ID: "boost-efficiency", NameZH: "省算力改造", RoleWeight: 1.0},
		{Dim: model.DimSafety, ID: "boost-safety", NameZH: "安全評測", RoleWeight: 1.1},
		{Dim: model.DimSpeed, ID: "boost-speed", NameZH: "加速優化", RoleWeight: 0.9},
	}
}

// TrainBoostRoleWeightSum returns the sum of RoleWeight across the catalog.
func TrainBoostRoleWeightSum(b Config) float64 {
	var s float64
	for _, tb := range b.TrainBoosts {
		s += tb.RoleWeight
	}
	return s
}

func weightForDim(b Config, dim model.QualityDim) float64 {
	for _, tb := range b.TrainBoosts {
		if tb.Dim == dim {
			return tb.RoleWeight
		}
	}
	return 0
}

func targetFullLinear(gen int, refMonthly float64, b Config) (float64, error) {
	if gen < 1 {
		return 0, ErrInvalidGenerationSpec
	}
	if refMonthly < 0 || math.IsNaN(refMonthly) || math.IsInf(refMonthly, 0) {
		return 0, ErrInvalidGenerationSpec
	}
	return float64(gen) * 12 * refMonthly * b.TrainBoostPainMult, nil
}

// TrainBoostBasePrice is the linear share of the full-pack target for one dim
// (no slot mult).
func TrainBoostBasePrice(gen int, refMonthly float64, dim model.QualityDim, b Config) (float64, error) {
	full, err := targetFullLinear(gen, refMonthly, b)
	if err != nil {
		return 0, err
	}
	sum := TrainBoostRoleWeightSum(b)
	if sum <= 0 {
		return 0, ErrInvalidGenerationSpec
	}
	return full * weightForDim(b, dim) / sum, nil
}

// TrainBoostCashCost sums base prices of selected boosts with slot mults by
// ascending dim index order among the selected set.
func TrainBoostCashCost(gen int, refMonthly float64, boosts [model.NumQualityDims]bool, b Config) (float64, error) {
	// Collect selected dims in ascending index order.
	var selected []model.QualityDim
	for d := model.QualityDim(0); d < model.NumQualityDims; d++ {
		if boosts[d] {
			selected = append(selected, d)
		}
	}
	var cost float64
	for rank, d := range selected {
		base, err := TrainBoostBasePrice(gen, refMonthly, d, b)
		if err != nil {
			return 0, err
		}
		mult := 1.0
		if rank >= 0 && rank < model.NumQualityDims {
			mult = b.TrainBoostSlotMult[rank]
		}
		if mult < 1 {
			mult = 1
		}
		cost += base * mult
	}
	return cost, nil
}

// TrainBoostCashBonus returns additive quality bonus per selected dim
// (beta * QualityScale); no soft-cap.
func TrainBoostCashBonus(gen int, boosts [model.NumQualityDims]bool, b Config) ([model.NumQualityDims]float64, error) {
	var out [model.NumQualityDims]float64
	spec, err := Generation(gen)
	if err != nil {
		return out, err
	}
	for d := model.QualityDim(0); d < model.NumQualityDims; d++ {
		if boosts[d] {
			out[d] = b.TrainBoostBeta * spec.QualityScale
		}
	}
	return out, nil
}
