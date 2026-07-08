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

func TestSetPriceIsCommand(t *testing.T) {
	var c Command = SetPrice{ModelIndex: 0, Price: 15}
	sp, ok := c.(SetPrice)
	if !ok || sp.Price != 15 || sp.ModelIndex != 0 {
		t.Fatalf("SetPrice command wrong: %+v ok=%v", c, ok)
	}
}

func TestCompetitorFields(t *testing.T) {
	c := Competitor{Name: "Rival"}
	c.Quality[DimCapability] = 55
	c.Skill[DimCapability] = 1.1
	if c.Name != "Rival" || c.Quality[DimCapability] != 55 || c.Skill[DimCapability] != 1.1 {
		t.Fatalf("competitor fields wrong: %+v", c)
	}
	var s GameState
	s.Competitors = append(s.Competitors, c)
	if len(s.Competitors) != 1 {
		t.Fatalf("GameState.Competitors not usable")
	}
}

func TestSegmentConstsAndModelField(t *testing.T) {
	if NumSegments != 3 || SegDeveloper != 2 {
		t.Fatalf("segment consts wrong: NumSegments=%d SegDeveloper=%d", NumSegments, SegDeveloper)
	}
	m := Model{Segment: SegEnterprise}
	if m.Segment != SegEnterprise {
		t.Fatalf("model segment field wrong: %v", m.Segment)
	}
	var zero Model
	if zero.Segment != SegConsumer {
		t.Fatalf("default segment should be SegConsumer(0), got %v", zero.Segment)
	}
}

func TestInferenceComputeAndCommand(t *testing.T) {
	var s GameState
	s.Compute.InferenceCapacity = 4
	s.Compute.InferenceLoad = 1.5
	if s.Compute.InferenceCapacity != 4 || s.Compute.InferenceLoad != 1.5 {
		t.Fatalf("inference compute fields wrong: %+v", s.Compute)
	}
	var c Command = RentInferenceCompute{Delta: 2}
	if _, ok := c.(RentInferenceCompute); !ok {
		t.Fatalf("RentInferenceCompute not a Command")
	}
}

func TestComputeInfraTypes(t *testing.T) {
	if PoolTraining != 0 || PoolInference != 1 {
		t.Fatalf("pool consts wrong")
	}
	ch := Chip{Name: "T", Pool: PoolTraining, Compute: 3, PowerKW: 5, Price: 18000}
	sv := Server{Pool: ch.Pool, Compute: 24, PowerKW: 40, Slots: 1}
	var s GameState
	s.Servers = append(s.Servers, sv)
	s.Datacenter = Datacenter{PowerCapacity: 800, SlotCapacity: 20}
	if len(s.Servers) != 1 || s.Datacenter.PowerCapacity != 800 {
		t.Fatalf("infra fields wrong: %+v", s)
	}
	var c1 Command = BuildServer{ChipName: "T"}
	var c2 Command = ExpandDatacenter{PowerDelta: 100, SlotDelta: 5}
	if _, ok := c1.(BuildServer); !ok {
		t.Fatalf("BuildServer not a Command")
	}
	if _, ok := c2.(ExpandDatacenter); !ok {
		t.Fatalf("ExpandDatacenter not a Command")
	}
}

func TestRolesAndStaffCommands(t *testing.T) {
	if NumRoles != 4 || RoleMarketing != 3 {
		t.Fatalf("role consts wrong")
	}
	var s GameState
	s.Engineers = 3
	s.Ops = 2
	s.Marketing = 1
	if s.Engineers != 3 || s.Ops != 2 || s.Marketing != 1 {
		t.Fatalf("staff fields wrong: %+v", s)
	}
	var c1 Command = HireStaff{Role: RoleResearcher, Tier: Tier2, Count: 3}
	var c2 Command = FireStaff{Role: RoleEngineer, Count: 1}
	if _, ok := c1.(HireStaff); !ok {
		t.Fatalf("HireStaff not a Command")
	}
	if _, ok := c2.(FireStaff); !ok {
		t.Fatalf("FireStaff not a Command")
	}
}

func TestTechTypes(t *testing.T) {
	if NumBranches != 4 || BranchAlignment != 3 {
		t.Fatalf("branch consts wrong")
	}
	e := NeutralTechEffects()
	if e.TrainRnDMult != 1 || e.InfraMult != 1 || e.QualityMult[DimCapability] != 1 {
		t.Fatalf("neutral effects not all 1: %+v", e)
	}
	n := TechNode{ID: "x", Branch: BranchAlgo, Cost: 100, Effects: e}
	var s GameState
	s.UnlockedTech = append(s.UnlockedTech, n.ID)
	if len(s.UnlockedTech) != 1 {
		t.Fatalf("UnlockedTech not usable")
	}
	var c Command = UnlockTech{NodeID: "x"}
	if _, ok := c.(UnlockTech); !ok {
		t.Fatalf("UnlockTech not a Command")
	}
}

func TestValuationFields(t *testing.T) {
	var s GameState
	s.PeakValuation = 1_500_000
	s.MilestonesReached = 1
	if s.PeakValuation != 1_500_000 || s.MilestonesReached != 1 {
		t.Fatalf("valuation fields wrong: %+v", s)
	}
}

func TestPrestigeTypes(t *testing.T) {
	e := NeutralPrestigeEffects()
	if e.RnDMult != 1 || e.CashMult != 1 || e.StartCash != 0 {
		t.Fatalf("neutral prestige effects wrong: %+v", e)
	}
	var s GameState
	s.Prestige.Patents = 3
	s.Prestige.UnlockedPrestige = append(s.Prestige.UnlockedPrestige, "x")
	if s.Prestige.Patents != 3 || len(s.Prestige.UnlockedPrestige) != 1 {
		t.Fatalf("prestige state wrong: %+v", s.Prestige)
	}
	var c1 Command = PrestigeReset{}
	var c2 Command = BuyPrestigeNode{NodeID: "x"}
	if _, ok := c1.(PrestigeReset); !ok {
		t.Fatalf("PrestigeReset not a Command")
	}
	if _, ok := c2.(BuyPrestigeNode); !ok {
		t.Fatalf("BuyPrestigeNode not a Command")
	}
}

func TestStarTypes(t *testing.T) {
	e := NeutralStarEffects()
	if e.QualityMult[DimCapability] != 1 || e.InfraMult != 1 || e.UserGrowthMult != 1 || e.RnDPerSec != 0 {
		t.Fatalf("neutral star effects wrong: %+v", e)
	}
	st := Star{ID: "x", Name: "X", SigningCost: 100, SalaryPerSec: 1, Effects: e}
	var s GameState
	s.HiredStars = append(s.HiredStars, st.ID)
	if len(s.HiredStars) != 1 {
		t.Fatalf("HiredStars not usable")
	}
	var c Command = SignStar{StarID: "x"}
	if _, ok := c.(SignStar); !ok {
		t.Fatalf("SignStar not a Command")
	}
}
