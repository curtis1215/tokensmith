package balance

import (
	"testing"

	"tokensmith/internal/model"
)

func TestDefaultV0Values(t *testing.T) {
	c := Default()
	if c.TokenInputWeight != 1 || c.TokenOutputWeight != 2 || c.TokenDivisor != 1 {
		t.Errorf("token formula params wrong: %+v", c)
	}
	if c.StreakMult != 1.0 {
		t.Errorf("StreakMult default = %v, want 1.0 (neutral)", c.StreakMult)
	}
	// Employee R&D uses RnDPerPower × RolePower (see applyEmployeeDefaults).
	if c.RnDPerPower <= 0 {
		t.Errorf("RnDPerPower = %v, want > 0", c.RnDPerPower)
	}
}

func TestDefaultGenAndTrainValues(t *testing.T) {
	c := Default()
	if c.TrainRentPerGPUSec != 0.01 {
		t.Errorf("TrainRentPerGPUSec = %v, want 0.01", c.TrainRentPerGPUSec)
	}
	// Gen1–5 training values are owned by the generation catalog.
	want := []struct {
		gen                 int
		trainRnD, trainWork float64
		quality             float64
	}{
		{1, 20000, 900000, 25},
		{2, 150000, 3600000, 45},
		{3, 1000000, 14400000, 65},
		{4, 6000000, 57600000, 82},
		{5, 40000000, 230400000, 100},
	}
	for _, tc := range want {
		g, err := Generation(tc.gen)
		if err != nil {
			t.Fatalf("Generation(%d): %v", tc.gen, err)
		}
		if g.TrainRnD != tc.trainRnD || g.TrainWork != tc.trainWork || g.QualityScale != tc.quality {
			t.Errorf("catalog gen %d = TrainRnD/Work/Quality %v/%v/%v, want %v/%v/%v",
				tc.gen, g.TrainRnD, g.TrainWork, g.QualityScale, tc.trainRnD, tc.trainWork, tc.quality)
		}
	}
}

func TestDefaultUserRevenueValues(t *testing.T) {
	c := Default()
	if c.QualityWeights[model.DimCapability] != 0.4 {
		t.Errorf("QualityWeights[cap] = %v, want 0.4", c.QualityWeights[model.DimCapability])
	}
	var sum float64
	for _, w := range c.QualityWeights {
		sum += w
	}
	if sum < 0.999 || sum > 1.001 {
		t.Errorf("QualityWeights sum = %v, want 1", sum)
	}
	if c.UserTargetPerAppeal != 20000 || c.UserGrowthRate != 3.5e-5 {
		t.Errorf("user growth params wrong: %+v", c)
	}
	if c.RefPrice != 12 || c.PriceElasticity != 1.5 {
		t.Errorf("pricing params wrong: %+v", c)
	}
	if c.MonthSec != 2592000 {
		t.Errorf("MonthSec = %v, want 2592000", c.MonthSec)
	}
}

func TestDefaultCompetitors(t *testing.T) {
	cs := DefaultCompetitors()
	if len(cs) != 7 {
		t.Fatalf("competitors = %d, want 7", len(cs))
	}
	if cs[0].Name != "OpenAI" {
		t.Errorf("first competitor wrong: %+v", cs[0])
	}
	// every competitor has a name and a positive top skill
	for _, c := range cs {
		if c.Name == "" {
			t.Errorf("competitor missing name: %+v", c)
		}
		if c.Skill[model.DimCapability] <= 0 {
			t.Errorf("competitor missing skill: %+v", c)
		}
	}
}

func TestDefaultCompetitorSpecialties(t *testing.T) {
	const lo, hi = 0.92, 1.08
	for _, c := range DefaultCompetitors() {
		for d, sk := range c.Skill {
			if sk < lo || sk > hi {
				t.Errorf("%s skill[%d]=%v outside [%.2f, %.2f]", c.Name, d, sk, lo, hi)
			}
		}
	}
}

func TestDefaultSegments(t *testing.T) {
	c := Default()
	// consumer(0) mirrors legacy scalars
	if c.SegmentWeights[model.SegConsumer] != c.QualityWeights {
		t.Errorf("consumer weights should mirror QualityWeights")
	}
	if c.SegmentTargetScale[model.SegConsumer] != c.UserTargetPerAppeal {
		t.Errorf("consumer scale should mirror UserTargetPerAppeal")
	}
	if c.SegmentRefPrice[model.SegConsumer] != c.RefPrice {
		t.Errorf("consumer ref price should mirror RefPrice")
	}
	// enterprise weights safety over capability
	ew := c.SegmentWeights[model.SegEnterprise]
	if ew[model.DimSafety] <= ew[model.DimCapability] {
		t.Errorf("enterprise should weight safety over capability: %+v", ew)
	}
	if c.SegmentRefPrice[model.SegEnterprise] != 180 || c.SegmentRefPrice[model.SegDeveloper] != 6 {
		t.Errorf("segment ref prices wrong: %+v", c.SegmentRefPrice)
	}
	// every segment's weights sum to 1
	for s, sw := range c.SegmentWeights {
		var sum float64
		for _, w := range sw {
			sum += w
		}
		if sum < 0.999 || sum > 1.001 {
			t.Errorf("segment %d weights sum = %v, want 1", s, sum)
		}
	}
}

func TestDefaultInferenceValues(t *testing.T) {
	c := Default()
	if c.InferenceRentPerGPUSec != 0.006 {
		t.Errorf("InferenceRentPerGPUSec = %v, want 0.006", c.InferenceRentPerGPUSec)
	}
	if c.InferenceLoadPerUser != 0.0001 {
		t.Errorf("InferenceLoadPerUser = %v, want 0.0001", c.InferenceLoadPerUser)
	}
	if c.ServiceChurnRate != 0.01 {
		t.Errorf("ServiceChurnRate = %v, want 0.01", c.ServiceChurnRate)
	}
}

func TestDefaultServerAndInfra(t *testing.T) {
	c := Default()
	// Self-build repoints onto the Processes catalog (plan-13); Chips is gone.
	if c.ChassisCost != 1000 {
		t.Errorf("server params wrong: %+v", c)
	}
	if c.ElectricityPerKWSec != 0.0002 || c.PowerCostPerKW != 400 || c.SlotCost != 4000 {
		t.Errorf("infra costs wrong: %+v", c)
	}
}

// TestSelfBuildCheaperThanRent locks in the buy-vs-rent invariant: a self-built
// chip's only ongoing cost is electricity, which must be below its rent so that
// paying capex to self-build actually pays back over time (spec §1 "rent OR buy
// = a real choice").
func TestSelfBuildCheaperThanRent(t *testing.T) {
	c := Default()
	for _, p := range c.Processes {
		elec := p.PowerKW * c.ElectricityPerKWSec
		if elec >= p.RentPerSec {
			t.Errorf("%s: self-build electricity %v must be < inference rent %v", p.ID, elec, p.RentPerSec)
		}
	}
}

func TestDefaultEmployeeEconomyKnobs(t *testing.T) {
	c := Default()
	if c.PrimaryWeight != 1.0 || c.SecondaryWeight != 0.35 {
		t.Errorf("role weights wrong: primary=%v secondary=%v", c.PrimaryWeight, c.SecondaryWeight)
	}
	wantRnD := 0.0002 / RealSecCompression
	if c.RnDPerPower != wantRnD {
		t.Errorf("RnDPerPower = %v, want %v", c.RnDPerPower, wantRnD)
	}
	if c.SecondsPerMonth != c.MonthSec {
		t.Errorf("SecondsPerMonth = %v, want MonthSec %v", c.SecondsPerMonth, c.MonthSec)
	}
	if c.HireMonths != 2 || c.SeveranceMonths != 0.5 {
		t.Errorf("hire/severance months wrong: hire=%v sev=%v", c.HireMonths, c.SeveranceMonths)
	}
	if c.MaxOfficeLevel != 8 || c.OfficeSeats[1] != 3 {
		t.Errorf("office defaults wrong: max=%d seatsL1=%d", c.MaxOfficeLevel, c.OfficeSeats[1])
	}
}

func TestDefaultTechNodes(t *testing.T) {
	c := Default()
	if len(c.TechNodes) < 8 {
		t.Fatalf("tech nodes = %d, want >= 8", len(c.TechNodes))
	}
	byID := map[string]model.TechNode{}
	for _, n := range c.TechNodes {
		byID[n.ID] = n
	}
	if n, ok := byID["algo-cap-1"]; !ok || n.Effects.QualityMult[model.DimCapability] != 1.15 {
		t.Errorf("algo-cap-1 wrong: %+v ok=%v", n, ok)
	}
	if n, ok := byID["infra-density-1"]; !ok || len(n.Prereqs) != 1 || n.Prereqs[0] != "infra-eff-1" {
		t.Errorf("infra-density-1 prereq wrong: %+v", n)
	}
	// unrelated fields stay neutral
	if byID["algo-cap-1"].Effects.InfraMult != 1 {
		t.Errorf("algo-cap-1 InfraMult should be neutral 1")
	}
}

func TestDefaultValuationValues(t *testing.T) {
	c := Default()
	if len(c.ValuationMilestones) != 7 {
		t.Fatalf("milestones = %d, want 7", len(c.ValuationMilestones))
	}
	if c.ValuationMilestones[0] != 1e6 || c.ValuationMilestones[3] != 1e9 {
		t.Errorf("milestone thresholds wrong: %v", c.ValuationMilestones)
	}
	if c.RevenueMultiple != 120 || c.UserValue != 10 || c.ServerAssetValue != 5000 {
		t.Errorf("valuation params wrong: %+v", c)
	}
	// milestones strictly increasing
	for i := 1; i < len(c.ValuationMilestones); i++ {
		if c.ValuationMilestones[i] <= c.ValuationMilestones[i-1] {
			t.Errorf("milestones must be increasing at %d", i)
		}
	}
}

func TestDefaultPrestige(t *testing.T) {
	c := Default()
	if c.PrestigeUnlockValuation != 1e9 || c.PatentK != 1e8 || c.StartingCash != 100000 {
		t.Errorf("prestige scalars wrong: %+v", c)
	}
	byID := map[string]model.PrestigeNode{}
	for _, n := range c.PrestigeNodes {
		byID[n.ID] = n
	}
	if n, ok := byID["start-cash-1"]; !ok || n.Cost != 1 || n.Effects.StartCash != 100000 {
		t.Errorf("start-cash-1 wrong: %+v ok=%v", n, ok)
	}
	if n, ok := byID["rnd-mult-1"]; !ok || n.Effects.RnDMult != 1.1 {
		t.Errorf("rnd-mult-1 wrong: %+v", n)
	}
}

func TestDefaultProcesses(t *testing.T) {
	c := Default()
	if len(c.Processes) != 4 {
		t.Fatalf("processes = %d, want 4", len(c.Processes))
	}
	n7, ok := ProcessByID(c.Processes, EntryProcessID)
	if !ok || n7.UnlockTech != "" || n7.Compute != 1 || n7.RentPerSec != 0.001 {
		t.Errorf("N7 entry wrong: %+v ok=%v", n7, ok)
	}
	n5, _ := ProcessByID(c.Processes, "N5")
	if n5.UnlockTech != "process-N5" || n5.Compute != 2 {
		t.Errorf("N5 wrong: %+v", n5)
	}
	// higher process = better compute-per-rent and compute-per-watt
	prev := 0.0
	for _, p := range c.Processes {
		if r := p.Compute / p.RentPerSec; r < prev {
			t.Errorf("compute/rent should be non-decreasing, %s broke it", p.ID)
		} else {
			prev = r
		}
	}
	if c.RevenueMult != 2 || c.TrainRentMult < 1.6 || c.TrainRentMult > 1.7 {
		t.Errorf("economy scalars wrong: rev=%v trainmult=%v", c.RevenueMult, c.TrainRentMult)
	}
	byID := map[string]model.TechNode{}
	for _, n := range c.TechNodes {
		byID[n.ID] = n
	}
	if n, ok := byID["process-N3"]; !ok || len(n.Prereqs) != 1 || n.Prereqs[0] != "process-N5" {
		t.Errorf("process-N3 prereq wrong: %+v", n)
	}
}
