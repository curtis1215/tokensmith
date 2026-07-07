package model

import (
	"testing"
	"time"
)

func TestResearchIndexedByTier(t *testing.T) {
	r := Research{EfficiencyMult: 1.0}
	r.Researchers[Tier1] = 2
	r.Researchers[Tier3] = 1
	if r.Researchers[Tier1] != 2 || r.Researchers[Tier3] != 1 {
		t.Fatalf("tier indexing wrong: %+v", r.Researchers)
	}
	if NumTiers != 4 {
		t.Fatalf("NumTiers = %d, want 4", NumTiers)
	}
}

func TestGameStateZeroValue(t *testing.T) {
	var s GameState
	if s.GameTime != 0 || s.Resources.RnD != 0 || s.WindowRnD != 0 {
		t.Fatalf("zero GameState not zero: %+v", s)
	}
}

func TestTokenEventFields(t *testing.T) {
	e := TokenEvent{Source: "claude-code", Timestamp: time.Unix(0, 0), InputTokens: 100, OutputTokens: 50}
	if e.Source != "claude-code" || e.InputTokens != 100 || e.OutputTokens != 50 {
		t.Fatalf("token event fields wrong: %+v", e)
	}
}

func TestModelAndComputeFields(t *testing.T) {
	m := Model{Gen: 2, Online: true, Price: 12}
	m.Quality[DimCapability] = 40
	m.Quality[DimSpeed] = 30
	if m.Quality[DimCapability] != 40 || m.Quality[DimSpeed] != 30 {
		t.Fatalf("quality dims wrong: %+v", m.Quality)
	}
	if NumQualityDims != 4 {
		t.Fatalf("NumQualityDims = %d, want 4", NumQualityDims)
	}
	var s GameState
	s.Compute.TrainingCapacity = 4
	s.Models = append(s.Models, m)
	s.HasTraining = true
	s.Training = TrainingJob{Gen: 2, Price: 12, WorkRemaining: 7200}
	if s.Compute.TrainingCapacity != 4 || len(s.Models) != 1 || !s.HasTraining {
		t.Fatalf("gamestate extension wrong: %+v", s)
	}
}

func TestCommandsImplementInterface(t *testing.T) {
	var cmds []Command
	cmds = append(cmds, StartTraining{Gen: 1}, RentTrainingCompute{Delta: 2})
	if len(cmds) != 2 {
		t.Fatalf("commands not assignable to Command interface")
	}
}
