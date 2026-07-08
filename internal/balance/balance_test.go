package balance

import (
	"testing"

	"tokensmith/internal/model"
)

func TestDefaultV0Values(t *testing.T) {
	c := Default()
	if c.ResearcherRnDPerSec[model.Tier1] != 5 {
		t.Errorf("Tier1 R&D/s = %v, want 5", c.ResearcherRnDPerSec[model.Tier1])
	}
	if c.ResearcherRnDPerSec[model.Tier2] != 15 {
		t.Errorf("Tier2 R&D/s = %v, want 15", c.ResearcherRnDPerSec[model.Tier2])
	}
	if c.ResearcherRnDPerSec[model.Tier3] != 40 {
		t.Errorf("Tier3 R&D/s = %v, want 40", c.ResearcherRnDPerSec[model.Tier3])
	}
	if c.TokenInputWeight != 1 || c.TokenOutputWeight != 2 || c.TokenDivisor != 10 {
		t.Errorf("token formula params wrong: %+v", c)
	}
	if c.SoftCapFull != 200000 || c.SoftCapMult != 0.3 || c.SoftCapWindowSec != 86400 {
		t.Errorf("soft cap params wrong: %+v", c)
	}
}

func TestDefaultGenAndTrainValues(t *testing.T) {
	c := Default()
	if MaxGen != 5 {
		t.Fatalf("MaxGen = %d, want 5", MaxGen)
	}
	if c.GenRnDCost[1] != 20000 || c.GenRnDCost[5] != 40000000 {
		t.Errorf("GenRnDCost wrong: %v", c.GenRnDCost)
	}
	if c.GenTrainWorkGPUSec[1] != 1800 { // 0.5 GPU·hr * 3600
		t.Errorf("GenTrainWorkGPUSec[1] = %v, want 1800", c.GenTrainWorkGPUSec[1])
	}
	if c.GenTrainWorkGPUSec[4] != 108000 { // 30 GPU·hr * 3600
		t.Errorf("GenTrainWorkGPUSec[4] = %v, want 108000", c.GenTrainWorkGPUSec[4])
	}
	if c.GenQualityCap[1] != 25 || c.GenQualityCap[5] != 100 {
		t.Errorf("GenQualityCap wrong: %v", c.GenQualityCap)
	}
	if c.TrainRentPerGPUSec != 0.01 {
		t.Errorf("TrainRentPerGPUSec = %v, want 0.01", c.TrainRentPerGPUSec)
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
	if c.UserTargetPerAppeal != 1000 || c.UserGrowthRate != 0.001 {
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
	if cs[0].Name != "OpenAI" || cs[0].Quality[model.DimCapability] != 55 {
		t.Errorf("first competitor wrong: %+v", cs[0])
	}
	// every competitor has a name and some capability
	for _, c := range cs {
		if c.Name == "" {
			t.Errorf("competitor missing name: %+v", c)
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

func TestDefaultChipsAndInfra(t *testing.T) {
	c := Default()
	if len(c.Chips) != 2 {
		t.Fatalf("chips = %d, want 2", len(c.Chips))
	}
	if c.Chips[0].Name != "H-class G3" || c.Chips[0].Pool != model.PoolInference {
		t.Errorf("first chip wrong: %+v", c.Chips[0])
	}
	if c.Chips[1].Pool != model.PoolTraining || c.Chips[1].Price != 18000 {
		t.Errorf("second chip wrong: %+v", c.Chips[1])
	}
	if c.ChipsPerServer != 8 || c.ChassisCost != 5000 {
		t.Errorf("server params wrong: %+v", c)
	}
	if c.ElectricityPerKWSec != 0.001 || c.PowerCostPerKW != 400 || c.SlotCost != 30000 {
		t.Errorf("infra costs wrong: %+v", c)
	}
}

func TestDefaultStaffValues(t *testing.T) {
	c := Default()
	if c.ResearcherHireCost[model.Tier2] != 15000 {
		t.Errorf("ResearcherHireCost[T2] = %v, want 15000", c.ResearcherHireCost[model.Tier2])
	}
	if c.ResearcherSalaryPerSec[model.Tier3] != 0.005 {
		t.Errorf("ResearcherSalaryPerSec[T3] = %v, want 0.005", c.ResearcherSalaryPerSec[model.Tier3])
	}
	if c.EngineerHireCost != 8000 || c.OpsHireCost != 6000 || c.MarketingHireCost != 6000 {
		t.Errorf("hire costs wrong: %+v", c)
	}
	if c.EngineerInfraBonus != 0.02 || c.OpsChurnReduction != 0.1 || c.MarketingBonus != 0.03 {
		t.Errorf("staff bonuses wrong: %+v", c)
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
