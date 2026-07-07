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
