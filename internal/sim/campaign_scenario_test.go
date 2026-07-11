package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// winningCampaignFixture builds a late-showdown state that should win within
// cycles 9–21 for balance scenarios (fixed seed + route-ready model).
//
// Hard global-frontier band (85%–115%) means idle rivals cannot sit far below
// the player-defined frontier. With the full 7-rival roster snapped to the
// floor, player segment share tops out near 1/(1+7×0.85)≈14% — below win
// gates (30–35%). The win-path fixture therefore uses two in-band rivals so
// share and rank gates remain attainable while the band invariant still holds.
func winningCampaignFixture(d model.Doctrine, seed uint64, b balance.Config) model.GameState {
	seg := doctrineSegment(d)
	price := b.SegmentRefPrice[seg]
	if d == model.DoctrineDeveloper {
		price *= 0.85
	}
	// Player quality 100 → GF≈100; two rivals parked at the hard floor (85).
	// Share ≈ 100/(100+85+85) ≈ 37% > Consumer/Developer win share 0.35.
	floorQ := [4]float64{85, 85, 85, 85}
	comps := []model.Competitor{
		{Name: "OpenAI", Quality: floorQ, Skill: q(1.0, 1.0, 1.0, 1.0)},
		{Name: "DeepSeek", Quality: floorQ, Skill: q(1.0, 1.0, 1.0, 1.0)},
	}
	s := model.GameState{
		Resources:   model.Resources{Cash: 1e7, RnD: 1e7},
		Models:      []model.Model{{Online: true, Segment: seg, Price: price, Users: 100000, Quality: [4]float64{100, 100, 100, 100}}},
		Competitors: comps,
		Ops:         2,
	}
	s.Compute.RentedInference = map[string]int{balance.EntryProcessID: 1000}
	s.Campaign.RandState = seed
	s, err := Apply(s, model.ChooseDoctrine{Doctrine: d}, b)
	if err != nil {
		panic(err)
	}
	s.Campaign.Stage = model.CampaignStageShowdown
	s.Campaign.Cycle = 8
	perks1 := balance.PerksFor(b.Campaign, d, 1)
	perks2 := balance.PerksFor(b.Campaign, d, 2)
	s.Campaign.Perks = []string{perks1[0].ID, perks2[0].ID}
	s.Campaign.ShowdownStartedCycle = 8
	s.Campaign.Primary.CyclesUntilAction = 1
	// Route-specific showdown gates: developer needs price ≤ EffectiveRefPrice*0.9
	// (after perk RefPriceMult) and positive cashflow with capacity util ≤ 0.80.
	// Shared 0.85×base-ref + 1000 chips fails under developer-open (ref×0.9).
	if d == model.DoctrineDeveloper {
		ref := EffectiveRefPrice(s, seg, b)
		s.Models[0].Price = ref * 0.85
		s.Compute.RentedInference = map[string]int{balance.EntryProcessID: 50}
	}
	return s
}

func assertWinsWithinTarget(t *testing.T, d model.Doctrine, seed uint64) {
	t.Helper()
	b := balance.Default()
	s := winningCampaignFixture(d, seed, b)
	for s.Campaign.Cycle < 21 && s.Campaign.Victory == model.DoctrineNone {
		s = AdvanceCampaignCycle(s, b)
		if s.Campaign.Victory == model.DoctrineNone {
			t.Logf("cycle=%d status=%+v", s.Campaign.Cycle, CampaignStatus(s, b))
		}
	}
	if s.Campaign.Victory != d || s.Campaign.Cycle < 9 || s.Campaign.Cycle > 21 {
		t.Fatalf("doctrine=%s cycle=%d victory=%s status=%+v", d, s.Campaign.Cycle, s.Campaign.Victory, CampaignStatus(s, b))
	}
}

func TestConsumerCampaignWinsWithinTargetCycles(t *testing.T) {
	assertWinsWithinTarget(t, model.DoctrineConsumer, 101)
}

func TestEnterpriseCampaignWinsWithinTargetCycles(t *testing.T) {
	assertWinsWithinTarget(t, model.DoctrineEnterprise, 202)
}

func TestDeveloperCampaignWinsWithinTargetCycles(t *testing.T) {
	assertWinsWithinTarget(t, model.DoctrineDeveloper, 303)
}
