// Package game seeds a fresh GameState for a new run.
package game

import (
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// NewGame returns the initial state for a new run.
func NewGame() model.GameState {
	var s model.GameState
	s.Resources.Cash = 100000
	s.Resources.RnD = 50000
	s.Research.EfficiencyMult = 1.0
	s.Research.Researchers[model.Tier1] = 2
	s.Competitors = balance.DefaultCompetitors()
	s.Compute.TrainingCapacity = 4
	s.Compute.InferenceCapacity = 2
	return s
}
