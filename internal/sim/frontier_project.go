package sim

import (
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

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
