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
