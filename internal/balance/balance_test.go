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
