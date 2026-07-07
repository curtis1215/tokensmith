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
	Compute       Compute
	Models        []Model
	HasTraining   bool
	Training      TrainingJob
}

// QualityDim indexes Model.Quality.
type QualityDim int

const (
	DimCapability  QualityDim = iota // 0 能力
	DimEfficiency                    // 1 成本效率
	DimSafety                        // 2 安全
	DimSpeed                         // 3 速度
	NumQualityDims = 4
)

// Model is a trained AI model.
type Model struct {
	Gen     int
	Quality [NumQualityDims]float64
	Users   float64
	Price   float64 // per user per month; player-set
	Online  bool
}

// TrainingJob is the single in-progress training (plan-02).
type TrainingJob struct {
	Gen           int
	Alloc         [NumQualityDims]float64 // budget fraction per dim; sums to ~1
	Price         float64
	WorkRemaining float64 // GPU-seconds of training work left
}

// Compute holds compute capacity (plan-02: training pool only).
type Compute struct {
	TrainingCapacity float64 // rented training GPUs
}

// Command is a validated player action applied via sim.Apply.
type Command interface{ commandMarker() }

// StartTraining begins training a new model of the given generation.
type StartTraining struct {
	Gen   int
	Alloc [NumQualityDims]float64
	Price float64
}

func (StartTraining) commandMarker() {}

// RentTrainingCompute adjusts rented training capacity by Delta (may be negative).
type RentTrainingCompute struct {
	Delta float64
}

func (RentTrainingCompute) commandMarker() {}

// SetPrice changes the monthly price of the model at ModelIndex.
type SetPrice struct {
	ModelIndex int
	Price      float64
}

func (SetPrice) commandMarker() {}
