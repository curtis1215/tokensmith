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
	Engineers     int
	Ops           int
	Marketing     int
	WindowRnD     float64 // token-sourced R&D accrued in the current soft-cap window
	WindowElapsed float64 // seconds elapsed in the current soft-cap window
	Compute       Compute
	Models        []Model
	Competitors   []Competitor
	Servers       []Server
	Datacenter    Datacenter
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

// Segment indexes a market segment.
type Segment int

const (
	SegConsumer   Segment = iota // 0 消費者
	SegEnterprise                // 1 企業
	SegDeveloper                 // 2 開發者
	NumSegments   = 3
)

// Model is a trained AI model.
type Model struct {
	Gen     int
	Segment Segment
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
	TrainingCapacity  float64 // rented training GPUs
	InferenceCapacity float64 // rented inference GPUs
	InferenceLoad     float64 // current inference load (computed each tick)
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

// Competitor is a rival AI company competing for market share.
type Competitor struct {
	Name         string
	Quality      [NumQualityDims]float64
	GrowthPerSec [NumQualityDims]float64 // per-second quality growth by dim
}

// RentInferenceCompute adjusts rented inference capacity by Delta.
type RentInferenceCompute struct {
	Delta float64
}

func (RentInferenceCompute) commandMarker() {}

// ComputePool identifies which compute pool a chip/server feeds.
type ComputePool int

const (
	PoolTraining  ComputePool = iota // 0
	PoolInference                    // 1
)

// Chip is a catalog entry; owned compute is held as Servers.
type Chip struct {
	Name    string
	Pool    ComputePool
	Compute float64 // compute per chip
	PowerKW float64 // power draw per chip
	Price   float64 // price per chip
}

// Server is self-built compute: a bundle of chips feeding one pool.
type Server struct {
	Pool    ComputePool
	Compute float64 // total compute contributed
	PowerKW float64 // total power draw
	Slots   float64 // rack slots occupied
}

// Datacenter provides power and rack-space capacity limits (single-DC v0).
type Datacenter struct {
	PowerCapacity float64
	SlotCapacity  float64
}

// BuildServer builds one server from the named chip in the datacenter.
type BuildServer struct {
	ChipName string
}

func (BuildServer) commandMarker() {}

// ExpandDatacenter adds power / rack-space capacity for capex.
type ExpandDatacenter struct {
	PowerDelta float64
	SlotDelta  float64
}

func (ExpandDatacenter) commandMarker() {}

// Role identifies an aggregate staff function.
type Role int

const (
	RoleResearcher Role = iota // 0
	RoleEngineer               // 1
	RoleOps                    // 2
	RoleMarketing              // 3
	NumRoles       = 4
)

// HireStaff hires Count staff of Role (Tier used only for RoleResearcher).
type HireStaff struct {
	Role  Role
	Tier  StaffTier
	Count int
}

func (HireStaff) commandMarker() {}

// FireStaff removes Count staff of Role (Tier used only for RoleResearcher).
type FireStaff struct {
	Role  Role
	Tier  StaffTier
	Count int
}

func (FireStaff) commandMarker() {}
