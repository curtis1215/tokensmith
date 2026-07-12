// Package game seeds a fresh GameState for a new run.
package game

import (
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// NewGame returns the initial state for a new run. The starting baseline is
// sourced from balance.Config so prestige freshRun reseeds identical values.
func NewGame() model.GameState {
	b := balance.Default()
	var s model.GameState
	s.Resources.Cash = b.StartingCash
	s.Resources.RnD = b.StartingRnD
	s.Research.EfficiencyMult = 1.0
	s.Office.Level = 1
	s.Employees = nil
	s.Competitors = balance.DefaultCompetitors()
	s.Progression.MaxUnlockedGen = 1
	// Compute starts empty (nil maps → 0): rent on demand, no rent burn
	// before a product exists.
	// Talent market is seeded by sim.freshRun / store migrate on load paths;
	// NewGame leaves Market zero until the first Tick or explicit refresh.
	return s
}
