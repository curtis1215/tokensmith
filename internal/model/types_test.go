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
