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
	s.Research.Researchers[model.Tier1] = b.StartingResearchersT1
	s.Competitors = balance.DefaultCompetitors()
	// Compute starts empty (nil maps → 0): rent on demand, no rent burn
	// before a product exists.
	return s
}
