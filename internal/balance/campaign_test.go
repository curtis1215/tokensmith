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

// TestDefaultCampaignShareGatesUnderHardBandCeiling locks the recalibrated
// establish < expand < win ladder inside the hard-band player-share ceiling
// for the default 7-rival roster. Gates above the ceiling make expand/win
// mathematically unreachable when rivals hard-clamp to 85% of GlobalFrontier.
func TestDefaultCampaignShareGatesUnderHardBandCeiling(t *testing.T) {
	c := DefaultCampaign()
	ceiling := HardBandPlayerShareCeiling(len(c.Rivals))
	wantCeil := 1 / (1 + 7*HardBandFloorPct)
	if ceiling != wantCeil {
		t.Fatalf("HardBandPlayerShareCeiling(7) = %v, want %v", ceiling, wantCeil)
	}
	// Absolute contract under ceiling.
	gates := []struct {
		name string
		v    float64
	}{
		{"EstablishShare", c.EstablishShare},
		{"ConsumerExpandShare", c.ConsumerExpandShare},
		{"EnterpriseExpandShare", c.EnterpriseExpandShare},
		{"DeveloperExpandShare", c.DeveloperExpandShare},
		{"ConsumerWinShare", c.ConsumerWinShare},
		{"EnterpriseWinShare", c.EnterpriseWinShare},
		{"DeveloperWinShare", c.DeveloperWinShare},
	}
	for _, g := range gates {
		if g.v <= 0 || g.v >= ceiling {
			t.Fatalf("%s = %v not in (0, ceiling=%v)", g.name, g.v, ceiling)
		}
	}
	// establish < expand < win per doctrine; consumer expand/win strictest.
	if !(c.EstablishShare < c.ConsumerExpandShare && c.ConsumerExpandShare < c.ConsumerWinShare) {
		t.Fatalf("consumer ladder: est=%v exp=%v win=%v", c.EstablishShare, c.ConsumerExpandShare, c.ConsumerWinShare)
	}
	if !(c.EstablishShare < c.EnterpriseExpandShare && c.EnterpriseExpandShare < c.EnterpriseWinShare) {
		t.Fatalf("enterprise ladder: est=%v exp=%v win=%v", c.EstablishShare, c.EnterpriseExpandShare, c.EnterpriseWinShare)
	}
	if !(c.EstablishShare < c.DeveloperExpandShare && c.DeveloperExpandShare < c.DeveloperWinShare) {
		t.Fatalf("developer ladder: est=%v exp=%v win=%v", c.EstablishShare, c.DeveloperExpandShare, c.DeveloperWinShare)
	}
	if c.ConsumerExpandShare < c.EnterpriseExpandShare || c.ConsumerExpandShare < c.DeveloperExpandShare {
		t.Fatalf("consumer expand should be strictest: c=%v e=%v d=%v",
			c.ConsumerExpandShare, c.EnterpriseExpandShare, c.DeveloperExpandShare)
	}
	if c.EnterpriseWinShare > c.ConsumerWinShare || c.EnterpriseWinShare > c.DeveloperWinShare {
		t.Fatalf("enterprise win should be easiest: c=%v e=%v d=%v",
			c.ConsumerWinShare, c.EnterpriseWinShare, c.DeveloperWinShare)
	}
	// Exact recalibrated values (document intentional balance table).
	if c.EstablishShare != 0.07 ||
		c.ConsumerExpandShare != 0.11 || c.EnterpriseExpandShare != 0.095 || c.DeveloperExpandShare != 0.095 ||
		c.ConsumerWinShare != 0.13 || c.EnterpriseWinShare != 0.12 || c.DeveloperWinShare != 0.13 {
		t.Fatalf("share gates drifted from hard-band table: %+v", c)
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
