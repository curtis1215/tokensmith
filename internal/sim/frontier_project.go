package sim

import (
	"math"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// simEpsilon snaps near-complete frontier residuals together.
const simEpsilon = 1e-9

// diminishedFrontierCompute applies linear progress through recommended compute
// and square-root diminishing returns above it.
func diminishedFrontierCompute(allocated, recommended float64) float64 {
	if allocated <= 0 {
		return 0
	}
	if recommended <= 0 || allocated <= recommended {
		return allocated
	}
	return recommended + math.Sqrt((allocated-recommended)*recommended)
}

// advanceFrontierProject streams R&D in proportion to completed work using the
// allocated training-pool share. Insufficient R&D stalls work progress; idle
// reserved compute is never redirected. On completion, MaxUnlockedGen becomes
// TargetGen and the project is cleared.
func advanceFrontierProject(s model.GameState, dt, allocated float64) model.GameState {
	if !s.Progression.Frontier.Active {
		return s
	}
	fp := s.Progression.Frontier
	if fp.WorkRemaining <= 0 {
		return completeFrontierProject(s)
	}
	if fp.RnDRemaining <= simEpsilon || s.Resources.RnD <= simEpsilon {
		// Stall: neither work nor frontier R&D advances without wallet R&D.
		return s
	}
	rndPerWork := fp.RnDRemaining / fp.WorkRemaining
	if rndPerWork <= 0 {
		return s
	}
	eff := diminishedFrontierCompute(allocated, fp.RecommendedCompute)
	workByCompute := eff * dt
	workByRnD := s.Resources.RnD / rndPerWork
	workDone := workByCompute
	if workByRnD < workDone {
		workDone = workByRnD
	}
	if fp.WorkRemaining < workDone {
		workDone = fp.WorkRemaining
	}
	if workDone <= 0 {
		return s
	}
	// Snap near-complete residuals so R&D and work finish together.
	if fp.WorkRemaining-workDone <= simEpsilon || fp.RnDRemaining-workDone*rndPerWork <= simEpsilon {
		// Finish only if wallet R&D covers the remaining frontier R&D.
		if s.Resources.RnD+simEpsilon >= fp.RnDRemaining {
			workDone = fp.WorkRemaining
			rndPerWork = fp.RnDRemaining / fp.WorkRemaining
		}
	}
	rndSpend := workDone * rndPerWork
	if rndSpend > s.Resources.RnD {
		rndSpend = s.Resources.RnD
		workDone = rndSpend / rndPerWork
	}

	ns := s
	ns.Resources.RnD -= rndSpend
	ns.Progression.Frontier.RnDRemaining -= rndSpend
	ns.Progression.Frontier.WorkRemaining -= workDone
	if ns.Progression.Frontier.WorkRemaining < 0 {
		ns.Progression.Frontier.WorkRemaining = 0
	}
	if ns.Progression.Frontier.RnDRemaining < 0 {
		ns.Progression.Frontier.RnDRemaining = 0
	}
	if ns.Progression.Frontier.WorkRemaining <= simEpsilon {
		return completeFrontierProject(ns)
	}
	return ns
}

func completeFrontierProject(s model.GameState) model.GameState {
	ns := s
	target := ns.Progression.Frontier.TargetGen
	if target > ns.Progression.MaxUnlockedGen {
		ns.Progression.MaxUnlockedGen = target
	}
	ns.Progression.Frontier = model.FrontierProject{}
	return ns
}

// applyStartFrontierProject begins a single long-run generation research job.
// Target must be MaxUnlockedGen+1 and at least Gen6; later-era targets also
// require EraOpen. Totals are snapshotted from Generation(TargetGen).
func applyStartFrontierProject(s model.GameState, c model.StartFrontierProject, _ balance.Config) (model.GameState, error) {
	if s.Progression.Frontier.Active {
		return s, ErrFrontierActive
	}
	max := MaxUnlockedGen(s, balance.Config{})
	if c.TargetGen < 6 || c.TargetGen != max+1 {
		// Surface catalog errors for invalid gens; otherwise wrong-target.
		if _, err := balance.Generation(c.TargetGen); err != nil {
			return s, err
		}
		return s, ErrInvalidFrontierTarget
	}
	// Era gates: Gen8+ (Era IV and later) require the target era to be open.
	era, err := balance.EraForGen(c.TargetGen)
	if err != nil {
		return s, err
	}
	if era >= 4 && !EraOpen(s, era) {
		return s, ErrEraNotOpen
	}
	// Snapshot catalog row only after target and era validation.
	spec, err := balance.Generation(c.TargetGen)
	if err != nil {
		return s, err
	}

	ns := s
	ns.Progression.Frontier = model.FrontierProject{
		Active:             true,
		TargetGen:          c.TargetGen,
		RnDTotal:           spec.FrontierRnD,
		RnDRemaining:       spec.FrontierRnD,
		WorkTotal:          spec.FrontierWork,
		WorkRemaining:      spec.FrontierWork,
		RecommendedCompute: spec.RecommendedCompute,
		AllocationPct:      100,
	}
	return ns, nil
}

// applySetFrontierAllocation sets the training-pool share reserved for the
// active frontier project. Progress totals/remaining are preserved.
func applySetFrontierAllocation(s model.GameState, c model.SetFrontierAllocation) (model.GameState, error) {
	if !s.Progression.Frontier.Active {
		return s, ErrNoFrontierProject
	}
	if c.Percent < 0 || c.Percent > 100 {
		return s, ErrInvalidFrontierAllocation
	}
	ns := s
	ns.Progression.Frontier.AllocationPct = c.Percent
	return ns, nil
}
