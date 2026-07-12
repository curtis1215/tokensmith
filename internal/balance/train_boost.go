package balance

import (
	"errors"
	"math"

	"tokensmith/internal/model"
)

// ErrInvalidTrainBoostConfig is returned when train-boost knobs or catalog
// violate design §11.1 invariants (fail closed — never emit NaN/negative cost).
var ErrInvalidTrainBoostConfig = errors.New("balance: invalid train boost config")

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

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// ValidateTrainBoostConfig enforces design §11.1: finite/non-neg knobs, legal
// catalog (exactly one entry per dim), positive role weights, slot mults ≥ 1,
// rival picks in 0..NumQualityDims.
func ValidateTrainBoostConfig(b Config) error {
	if !finite(b.TrainBoostBeta) || b.TrainBoostBeta < 0 {
		return ErrInvalidTrainBoostConfig
	}
	if !finite(b.TrainBoostPainMult) || b.TrainBoostPainMult <= 0 {
		return ErrInvalidTrainBoostConfig
	}
	if !finite(b.TrainBoostFloorMonthly) || b.TrainBoostFloorMonthly < 0 {
		return ErrInvalidTrainBoostConfig
	}
	if b.TrainBoostRivalPicks < 0 || b.TrainBoostRivalPicks > model.NumQualityDims {
		return ErrInvalidTrainBoostConfig
	}
	for i := 0; i < model.NumQualityDims; i++ {
		m := b.TrainBoostSlotMult[i]
		if !finite(m) || m < 1 {
			return ErrInvalidTrainBoostConfig
		}
	}
	if len(b.TrainBoosts) != model.NumQualityDims {
		return ErrInvalidTrainBoostConfig
	}
	seenDim := map[model.QualityDim]bool{}
	seenID := map[string]bool{}
	for _, tb := range b.TrainBoosts {
		if tb.ID == "" || tb.NameZH == "" {
			return ErrInvalidTrainBoostConfig
		}
		if int(tb.Dim) < 0 || int(tb.Dim) >= model.NumQualityDims {
			return ErrInvalidTrainBoostConfig
		}
		if seenDim[tb.Dim] || seenID[tb.ID] {
			return ErrInvalidTrainBoostConfig
		}
		seenDim[tb.Dim] = true
		seenID[tb.ID] = true
		if !finite(tb.RoleWeight) || tb.RoleWeight <= 0 {
			return ErrInvalidTrainBoostConfig
		}
	}
	for d := model.QualityDim(0); d < model.NumQualityDims; d++ {
		if !seenDim[d] {
			return ErrInvalidTrainBoostConfig
		}
	}
	if TrainBoostRoleWeightSum(b) <= 0 || !finite(TrainBoostRoleWeightSum(b)) {
		return ErrInvalidTrainBoostConfig
	}
	return nil
}

// TrainBoostRoleWeightSum returns the sum of RoleWeight across the catalog.
func TrainBoostRoleWeightSum(b Config) float64 {
	var s float64
	for _, tb := range b.TrainBoosts {
		s += tb.RoleWeight
	}
	return s
}

// TrainBoostByDim returns the catalog entry for dim, if present.
func TrainBoostByDim(b Config, dim model.QualityDim) (TrainBoost, bool) {
	for _, tb := range b.TrainBoosts {
		if tb.Dim == dim {
			return tb, true
		}
	}
	return TrainBoost{}, false
}

func weightForDim(b Config, dim model.QualityDim) (float64, error) {
	tb, ok := TrainBoostByDim(b, dim)
	if !ok {
		return 0, ErrInvalidTrainBoostConfig
	}
	return tb.RoleWeight, nil
}

func targetFullLinear(gen int, refMonthly float64, b Config) (float64, error) {
	if err := ValidateTrainBoostConfig(b); err != nil {
		return 0, err
	}
	if gen < 1 {
		return 0, ErrInvalidGenerationSpec
	}
	if !finite(refMonthly) || refMonthly < 0 {
		return 0, ErrInvalidTrainBoostConfig
	}
	full := float64(gen) * 12 * refMonthly * b.TrainBoostPainMult
	if !finite(full) || full < 0 {
		return 0, ErrInvalidTrainBoostConfig
	}
	return full, nil
}

// TrainBoostBasePrice is the linear share of the full-pack target for one dim
// (no slot mult).
func TrainBoostBasePrice(gen int, refMonthly float64, dim model.QualityDim, b Config) (float64, error) {
	full, err := targetFullLinear(gen, refMonthly, b)
	if err != nil {
		return 0, err
	}
	w, err := weightForDim(b, dim)
	if err != nil {
		return 0, err
	}
	sum := TrainBoostRoleWeightSum(b)
	price := full * w / sum
	if !finite(price) || price < 0 {
		return 0, ErrInvalidTrainBoostConfig
	}
	return price, nil
}

// TrainBoostCashCost sums base prices of selected boosts with slot mults by
// ascending dim index order among the selected set.
func TrainBoostCashCost(gen int, refMonthly float64, boosts [model.NumQualityDims]bool, b Config) (float64, error) {
	if err := ValidateTrainBoostConfig(b); err != nil {
		return 0, err
	}
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
		if rank < 0 || rank >= model.NumQualityDims {
			return 0, ErrInvalidTrainBoostConfig
		}
		mult := b.TrainBoostSlotMult[rank]
		// Invariants already require mult >= 1 and finite.
		line := base * mult
		if !finite(line) || line < 0 {
			return 0, ErrInvalidTrainBoostConfig
		}
		cost += line
	}
	if !finite(cost) || cost < 0 {
		return 0, ErrInvalidTrainBoostConfig
	}
	return cost, nil
}

// TrainBoostCashBonus returns additive quality bonus per selected dim
// (beta * QualityScale); no soft-cap.
func TrainBoostCashBonus(gen int, boosts [model.NumQualityDims]bool, b Config) ([model.NumQualityDims]float64, error) {
	var out [model.NumQualityDims]float64
	if err := ValidateTrainBoostConfig(b); err != nil {
		return out, err
	}
	spec, err := Generation(gen)
	if err != nil {
		return out, err
	}
	for d := model.QualityDim(0); d < model.NumQualityDims; d++ {
		if boosts[d] {
			v := b.TrainBoostBeta * spec.QualityScale
			if !finite(v) || v < 0 {
				return out, ErrInvalidTrainBoostConfig
			}
			out[d] = v
		}
	}
	return out, nil
}

// TrainBoostSlotRank returns the 0-based rank of dim among selected boosts
// ordered by ascending dim index, or -1 if dim is not selected.
func TrainBoostSlotRank(boosts [model.NumQualityDims]bool, dim model.QualityDim) int {
	if !boosts[dim] {
		return -1
	}
	rank := 0
	for d := model.QualityDim(0); d < dim; d++ {
		if boosts[d] {
			rank++
		}
	}
	return rank
}
