package game

import (
	"testing"

	"tokensmith/internal/model"
)

func TestNewGameSeed(t *testing.T) {
	s := NewGame()
	if s.Resources.Cash <= 0 {
		t.Errorf("cash should be positive, got %v", s.Resources.Cash)
	}
	if s.Resources.RnD < 20000 {
		t.Errorf("R&D should cover a Gen1 train, got %v", s.Resources.RnD)
	}
	if len(s.Competitors) != 7 {
		t.Errorf("competitors = %d, want 7", len(s.Competitors))
	}
	if s.Research.Researchers[model.Tier1] == 0 || s.Research.EfficiencyMult == 0 {
		t.Errorf("research not seeded: %+v", s.Research)
	}
	if s.Compute.TrainingCapacity <= 0 || s.Compute.InferenceCapacity <= 0 {
		t.Errorf("compute not seeded: %+v", s.Compute)
	}
}
