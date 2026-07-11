package balance

import (
	"testing"

	"tokensmith/internal/model"
)

func TestDefaultCampaignContract(t *testing.T) {
	c := DefaultCampaign()
	if c.CycleSec != 8*60*60 || c.MaxCatchupCycles != 3 || c.ReportCap != 20 {
		t.Fatalf("clock config = %+v", c)
	}
	if len(c.Perks) != 12 {
		t.Fatalf("perk count = %d, want 12", len(c.Perks))
	}
	if len(c.Rivals) != 7 {
		t.Fatalf("rival count = %d, want 7", len(c.Rivals))
	}
	for _, d := range []model.Doctrine{model.DoctrineConsumer, model.DoctrineEnterprise, model.DoctrineDeveloper} {
		if len(PerksFor(c, d, 1)) != 2 || len(PerksFor(c, d, 2)) != 2 {
			t.Fatalf("doctrine %q does not have two perks per tier", d)
		}
	}
}

func TestCampaignLookupsRejectUnknownIDs(t *testing.T) {
	c := DefaultCampaign()
	if _, ok := CampaignPerkByID(c, "missing"); ok {
		t.Fatal("unknown perk resolved")
	}
	if _, ok := RivalActionByID(c, "missing"); ok {
		t.Fatal("unknown action resolved")
	}
}

func TestRivalActionsUseFrontierProgress(t *testing.T) {
	c := DefaultCampaign()
	a, ok := RivalActionByID(c, "openai-flagship")
	if !ok {
		t.Fatal("openai-flagship missing")
	}
	if a.FrontierProgress[model.DimCapability] != 0.15 || a.MomentumCycles <= 0 {
		t.Fatalf("flagship contract: %+v", a)
	}
	// No legacy QualityPct field — compile-time check via FrontierProgress only.
	if a.RefPriceMult != 1 || a.LeadCycles != 2 {
		t.Fatalf("flagship meta: %+v", a)
	}
}
