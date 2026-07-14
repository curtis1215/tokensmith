package sim

import (
	"math"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// IndustryTimeCapSec is the maximum IndustryTime (game seconds) allowed for
// the player's current unlock: Generation(MaxUnlockedGen + Lead).TimeBaselineDay.
// On catalog failure returns +Inf so callers do not falsely freeze the clock.
func IndustryTimeCapSec(s model.GameState, b balance.Config) float64 {
	max := MaxUnlockedGen(s, b)
	if max < 1 {
		max = 1
	}
	capGen := max + balance.IndustryPlayerLeadGens
	if capGen < 1 {
		capGen = 1
	}
	spec, err := balance.Generation(capGen)
	if err != nil {
		return math.Inf(1)
	}
	if spec.TimeBaselineDay < 0 || math.IsNaN(spec.TimeBaselineDay) || math.IsInf(spec.TimeBaselineDay, 0) {
		return math.Inf(1)
	}
	return spec.TimeBaselineDay * 86400
}

// IndustryTimeResidualToCap is max(0, capSec − IndustryTime).
func IndustryTimeResidualToCap(s model.GameState, b balance.Config) float64 {
	cap := IndustryTimeCapSec(s, b)
	if math.IsInf(cap, 0) {
		return math.MaxFloat64
	}
	r := cap - s.Progression.IndustryTime
	if r < 0 || math.IsNaN(r) {
		return 0
	}
	return r
}

// EffectiveIndustryDay is IndustryTime in days, clamped to the player-lead cap.
// Used by TimeFrontier so the industry scale cannot outrun unlock progress.
func EffectiveIndustryDay(s model.GameState, b balance.Config) float64 {
	day := s.Progression.IndustryTime / 86400
	if day < 0 || math.IsNaN(day) {
		day = 0
	}
	capSec := IndustryTimeCapSec(s, b)
	if !math.IsInf(capSec, 0) {
		capDay := capSec / 86400
		if day > capDay {
			day = capDay
		}
	}
	return day
}

// EffectiveIndustryDT converts an economy delta into the industry delta for
// online play: idle throttle when neither frontier nor training is active,
// then clamp to residual-to-cap.
func EffectiveIndustryDT(s model.GameState, economyDT float64, b balance.Config) float64 {
	if economyDT <= 0 || math.IsNaN(economyDT) {
		return 0
	}
	residual := IndustryTimeResidualToCap(s, b)
	if residual <= 0 {
		return 0
	}
	dt := economyDT
	if !s.Progression.Frontier.Active && !s.HasTraining {
		dt *= balance.IndustryIdleMult
	}
	if dt > residual {
		dt = residual
	}
	if dt < 0 {
		return 0
	}
	return dt
}

// ClampIndustryToPlayerCap soft-repairs overheated IndustryTime and re-applies
// the rival hard band against the post-clamp GlobalFrontier. Player model
// quality is never rewritten. Pure.
func ClampIndustryToPlayerCap(s model.GameState, b balance.Config) model.GameState {
	ns := s
	cap := IndustryTimeCapSec(ns, b)
	if !math.IsInf(cap, 0) && ns.Progression.IndustryTime > cap {
		ns.Progression.IndustryTime = cap
	}
	if ns.Progression.IndustryTime < 0 || math.IsNaN(ns.Progression.IndustryTime) {
		ns.Progression.IndustryTime = 0
	}
	return clampAllRivalsToBand(ns, b)
}
