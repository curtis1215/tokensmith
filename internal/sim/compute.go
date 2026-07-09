package sim

import (
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// ProcessUnlocked reports whether the player may rent/build a process: the
// entry node is always available; others require their tech node.
func ProcessUnlocked(ns model.GameState, b balance.Config, id string) bool {
	p, ok := balance.ProcessByID(b.Processes, id)
	if !ok {
		return false
	}
	return p.UnlockTech == "" || isUnlocked(ns, p.UnlockTech)
}

// poolCompute sums chip counts × per-process compute for a rented-pool map.
func poolCompute(rented map[string]int, b balance.Config) float64 {
	var c float64
	for id, n := range rented {
		if p, ok := balance.ProcessByID(b.Processes, id); ok {
			c += float64(n) * p.Compute
		}
	}
	return c
}

// poolRentPerSec is the aggregate rent per game-second across both pools
// (training pays ×TrainRentMult).
func poolRentPerSec(ns model.GameState, b balance.Config) float64 {
	var r float64
	for id, n := range ns.Compute.RentedInference {
		if p, ok := balance.ProcessByID(b.Processes, id); ok {
			r += float64(n) * p.RentPerSec
		}
	}
	for id, n := range ns.Compute.RentedTraining {
		if p, ok := balance.ProcessByID(b.Processes, id); ok {
			r += float64(n) * p.RentPerSec * b.TrainRentMult
		}
	}
	return r
}

// cloneCounts copies a rented-pool map for pure mutation.
func cloneCounts(m map[string]int) map[string]int {
	out := make(map[string]int, len(m)+1)
	for k, v := range m {
		out[k] = v
	}
	return out
}
