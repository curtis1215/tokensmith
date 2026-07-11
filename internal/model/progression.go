package model

// ProgressionState is run-scoped long-term generation / era / frontier progress.
// Prestige and permanent unlocks live outside this struct and survive restarts.
type ProgressionState struct {
	MaxUnlockedGen int
	IndustryTime   float64
	Frontier       FrontierProject
	Eras           []EraProgress
	Rivals         RivalEraState
}

// FrontierProject is the single active long-run generation research job.
// Totals are snapshotted at start so mid-run balance changes do not rewrite ETA.
type FrontierProject struct {
	Active             bool
	TargetGen          int
	RnDTotal           float64
	RnDRemaining       float64
	WorkTotal          float64
	WorkRemaining      float64
	RecommendedCompute float64
	AllocationPct      int
}

// EraProgress records procedural breakthroughs for one era (Era III+).
// UnlockedMask uses bit 1<<TechBranch per branch.
type EraProgress struct {
	Era          int
	HasPrimary   bool
	Primary      TechBranch
	UnlockedMask uint8
}

// RivalEraState tracks the current rival league era and its selected leaders.
type RivalEraState struct {
	Era     int
	Leaders []string
}

// StartFrontierProject begins research toward TargetGen.
type StartFrontierProject struct {
	TargetGen int
}

func (StartFrontierProject) commandMarker() {}

// SetFrontierAllocation sets the training-pool share reserved for frontier work.
// Percent is an integer in [0, 100].
type SetFrontierAllocation struct {
	Percent int
}

func (SetFrontierAllocation) commandMarker() {}

// UnlockEraBreakthrough spends R&D on a procedural era branch breakthrough.
type UnlockEraBreakthrough struct {
	Era    int
	Branch TechBranch
}

func (UnlockEraBreakthrough) commandMarker() {}
