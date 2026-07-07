// Package model holds the shared value types used across the simulation.
// It has no dependencies and performs no I/O.
package model

import "time"

// StaffTier is a researcher skill tier. Values double as array indices.
type StaffTier int

const (
	TierNone StaffTier = iota // 0 — unused slot / no staff
	Tier1                     // 1
	Tier2                     // 2
	Tier3                     // 3
	NumTiers = 4              // size of tier-indexed arrays
)

// Resources are the fungible currencies the player accumulates.
type Resources struct {
	RnD  float64
	Cash float64
}

// Research is the R&D-generating workforce.
// Researchers is indexed by StaffTier (index 0 unused).
type Research struct {
	Researchers    [NumTiers]int
	EfficiencyMult float64 // infra bonus; 1.0 = no bonus
}

// TokenEvent is a normalized real-world AI-tool usage event.
type TokenEvent struct {
	Source       string
	Timestamp    time.Time
	InputTokens  int
	OutputTokens int
}

// GameState is the full simulation state (plan-01 subset).
// GameTime and WindowElapsed are in seconds.
type GameState struct {
	GameTime      float64
	Resources     Resources
	Research      Research
	WindowRnD     float64 // token-sourced R&D accrued in the current soft-cap window
	WindowElapsed float64 // seconds elapsed in the current soft-cap window
}
