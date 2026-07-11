package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// TestLongRunCalibration runs the approved reference-economy fixture:
// day 7,000, Gen5, sufficient R&D, no optional multipliers, recommended
// compute, 100% frontier then 100% train sequentially.
func TestLongRunCalibration(t *testing.T) {
	b := balance.Default()
	s := longRunReferenceStart(b)

	const day = 86400.0
	checkpoints := []struct {
		day    float64
		minGen int
		maxGen int
		label  string
	}{
		{20000, 8, 10, "day 20,000"},
		{43200, 10, 13, "day 43,200"},
		{72000, 10, 13, "day 72,000"},
	}

	nextCP := 0
	// Drive until past the last checkpoint.
	for s.GameTime < 72000*day+day && nextCP < len(checkpoints) {
		s = longRunStep(s, b)
		for nextCP < len(checkpoints) && s.GameTime+1e-6 >= checkpoints[nextCP].day*day {
			cp := checkpoints[nextCP]
			g := MaxUnlockedGen(s, b)
			if g < cp.minGen || g > cp.maxGen {
				t.Fatalf("%s: MaxUnlockedGen=%d, want %d–%d (gameTime=%.0f days)",
					cp.label, g, cp.minGen, cp.maxGen, s.GameTime/day)
			}
			nextCP++
		}
	}
	if nextCP != len(checkpoints) {
		t.Fatalf("fixture stopped early at day %.0f with gen %d; missing checkpoints",
			s.GameTime/day, MaxUnlockedGen(s, b))
	}
}

func longRunReferenceStart(b balance.Config) model.GameState {
	var s model.GameState
	s.GameTime = 7000 * 86400
	s.Progression.MaxUnlockedGen = 5
	s.Resources.RnD = 1e30 // "sufficient R&D"
	s.Resources.Cash = 1e30
	// No tech / stars / prestige multipliers; no engineers (infra = 1).
	return s
}

// longRunStep advances one frontier+train cycle action or a large Tick chunk.
func longRunStep(s model.GameState, b balance.Config) model.GameState {
	// Keep wallet topped up so R&D never stalls the reference script.
	s.Resources.RnD = 1e30
	s.Resources.Cash = 1e30

	// Ensure era gates for the next frontier target without optional-effect grind:
	// two free breakthroughs on the previous era when needed.
	s = ensureLongRunEraGates(s)

	max := MaxUnlockedGen(s, b)
	next := max + 1

	// Active frontier: put exactly recommended compute, 100% allocation, finish in one Tick.
	if s.Progression.Frontier.Active {
		rec := s.Progression.Frontier.RecommendedCompute
		s.Servers = []model.Server{{Pool: model.PoolTraining, Compute: rec}}
		s.Compute.RentedTraining = nil
		s.Progression.Frontier.AllocationPct = 100
		// dt covers remaining work at full recommended (linear).
		dt := s.Progression.Frontier.WorkRemaining / rec
		if dt < 1 {
			dt = 1
		}
		// Small pad for float residuals.
		return Tick(s, dt*1.01, nil, b)
	}

	// Active training: 100% model compute at that gen's recommended amount.
	if s.HasTraining {
		spec, err := balance.Generation(s.Training.Gen)
		if err != nil {
			return s
		}
		s.Servers = []model.Server{{Pool: model.PoolTraining, Compute: spec.RecommendedCompute}}
		if s.Training.Gen >= 6 {
			// Gen6+ train work uses recommended×seconds×compression; rate = recommended.
			dt := s.Training.WorkRemaining / spec.RecommendedCompute
			if dt < 1 {
				dt = 1
			}
			return Tick(s, dt*1.01, nil, b)
		}
		// Gen1–5: RecommendedCompute is 0 in catalog; use enough raw compute.
		s.Servers = []model.Server{{Pool: model.PoolTraining, Compute: 1e9}}
		return Tick(s, s.Training.WorkRemaining/1e9*1.01+1, nil, b)
	}

	// Prefer training the just-unlocked generation once before the next frontier,
	// matching "frontier then train sequentially".
	if max >= 6 && !longRunHasTrainedGen(s, max) {
		spec, err := balance.Generation(max)
		if err != nil {
			return s
		}
		ns, err := Apply(s, model.StartTraining{
			Gen: max, Segment: model.SegConsumer,
			Alloc: [model.NumQualityDims]float64{0.4, 0.2, 0.2, 0.2},
			Price: 12,
		}, b)
		if err != nil {
			// If training cannot start (e.g. already at capacity), fall through to frontier.
			_ = spec
		} else {
			return ns
		}
	}

	// Start next frontier project when Gen6+.
	if next >= 6 {
		ns, err := Apply(s, model.StartFrontierProject{TargetGen: next}, b)
		if err != nil {
			// Cannot start (era/target): jump a day so the loop cannot spin forever.
			s.GameTime += 86400
			return s
		}
		return ns
	}

	s.GameTime += 86400
	return s
}

func longRunHasTrainedGen(s model.GameState, gen int) bool {
	for _, m := range s.Models {
		if m.Gen == gen {
			return true
		}
	}
	return false
}

// ensureLongRunEraGates grants two breakthroughs on the previous era so the
// next frontier target's era is open. Effects are irrelevant for work rates
// (no train-work mult); this only clears the gate.
func ensureLongRunEraGates(s model.GameState) model.GameState {
	max := MaxUnlockedGen(s, balance.Config{})
	next := max + 1
	if next < 6 {
		return s
	}
	era, err := balance.EraForGen(next)
	if err != nil || era < 4 {
		return s
	}
	if EraOpen(s, era) {
		return s
	}
	prev := era - 1
	// Two branches unlocked on previous era.
	ep := model.EraProgress{
		Era:          prev,
		HasPrimary:   true,
		Primary:      model.BranchAlgo,
		UnlockedMask: (1 << model.BranchAlgo) | (1 << model.BranchInfra),
	}
	// Replace or insert.
	found := false
	eras := append([]model.EraProgress(nil), s.Progression.Eras...)
	for i := range eras {
		if eras[i].Era == prev {
			eras[i] = ep
			found = true
			break
		}
	}
	if !found {
		eras = append(eras, ep)
	}
	s.Progression.Eras = eras
	return s
}
