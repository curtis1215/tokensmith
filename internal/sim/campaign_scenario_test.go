package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// winningCampaignFixture builds a late-showdown state that should win within
// cycles 9–21 for balance scenarios (fixed seed + full default roster).
//
// Uses balance.DefaultCompetitors() (7 rivals). Hard band floor snaps idle
// rivals to ≥0.85×GF, so raw share ≤ HardBandPlayerShareCeiling(7)≈0.1439.
// Campaign share gates are calibrated under that ceiling (see DefaultCampaign).
func winningCampaignFixture(d model.Doctrine, seed uint64, b balance.Config) model.GameState {
	seg := doctrineSegment(d)
	price := b.SegmentRefPrice[seg]
	if d == model.DoctrineDeveloper {
		price *= 0.85
	}
	comps := balance.DefaultCompetitors()
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
	if n := len(s.Competitors); n != len(balance.DefaultCompetitors()) {
		t.Fatalf("fixture roster size = %d, want full DefaultCompetitors (%d)", n, len(balance.DefaultCompetitors()))
	}
	for s.Campaign.Cycle < 21 && s.Campaign.Victory == model.DoctrineNone {
		s = AdvanceCampaignCycle(s, b)
		if s.Campaign.Victory == model.DoctrineNone {
			t.Logf("cycle=%d status=%+v", s.Campaign.Cycle, CampaignStatus(s, b))
		}
	}
	if s.Campaign.Victory != d || s.Campaign.Cycle < 9 || s.Campaign.Cycle > 21 {
		t.Fatalf("doctrine=%s cycle=%d victory=%s status=%+v", d, s.Campaign.Cycle, s.Campaign.Victory, CampaignStatus(s, b))
	}
	// Band invariant holds on the full roster after the win path.
	assertRivalsInsideBand(t, s, b, "post-victory full roster")
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

// TestCampaignShareCeilingWithDefaultRoster proves the hard-floor share math:
// after a pure band clamp on the default 7-roster, player share equals
// HardBandPlayerShareCeiling(7) and every default win/expand gate is ≤ that.
func TestCampaignShareCeilingWithDefaultRoster(t *testing.T) {
	b := balance.Default()
	comps := balance.DefaultCompetitors()
	if len(comps) != 7 {
		t.Fatalf("default roster = %d, want 7", len(comps))
	}
	// Player far ahead on all dims; rivals start near skill×8 (≪ floor).
	s := model.GameState{
		Models: []model.Model{{
			Online: true, Segment: model.SegConsumer, Price: b.SegmentRefPrice[model.SegConsumer],
			Users: 100000, Quality: [4]float64{1000, 1000, 1000, 1000},
		}},
		Competitors: comps,
		Campaign:    model.CampaignState{Doctrine: model.DoctrineConsumer, Stage: model.CampaignStageExpand},
	}
	// Tiny industry dt: catch-up ≈ 0, but advanceRivalLeague still hard-clamps.
	b.CompetitorCatchupRate = 0
	ns := advanceRivalLeague(s, 1e-6, b)
	assertRivalsInsideBand(t, ns, b, "after pure floor clamp")
	share := playerSegmentShare(ns, b, model.SegConsumer)
	ceiling := balance.HardBandPlayerShareCeiling(len(comps))
	if share < ceiling-1e-6 || share > ceiling+1e-6 {
		t.Fatalf("share after hard floor = %v, want theoretical ceiling %v", share, ceiling)
	}
	// Gates must remain reachable at the floor-clamped ceiling.
	if b.Campaign.ConsumerWinShare >= ceiling || b.Campaign.ConsumerExpandShare >= ceiling {
		t.Fatalf("consumer gates win=%v expand=%v not under ceiling %v",
			b.Campaign.ConsumerWinShare, b.Campaign.ConsumerExpandShare, ceiling)
	}
	if share < b.Campaign.ConsumerWinShare {
		t.Fatalf("floor-clamped share %v < win gate %v (unreachable)", share, b.Campaign.ConsumerWinShare)
	}
}

// TestCampaignWinnableWithDefaultRoster runs real board cycles on the full
// default roster and asserts victory remains attainable under recalibrated gates.
func TestCampaignWinnableWithDefaultRoster(t *testing.T) {
	b := balance.Default()
	s := winningCampaignFixture(model.DoctrineConsumer, 101, b)
	if len(s.Competitors) != 7 {
		t.Fatalf("roster = %d, want 7", len(s.Competitors))
	}
	maxShare := 0.0
	for s.Campaign.Cycle < 21 && s.Campaign.Victory == model.DoctrineNone {
		s = AdvanceCampaignCycle(s, b)
		st := CampaignStatus(s, b)
		if st.Share > maxShare {
			maxShare = st.Share
		}
	}
	if s.Campaign.Victory != model.DoctrineConsumer {
		t.Fatalf("no victory with full roster: cycle=%d status=%+v maxShare=%v ceiling=%v",
			s.Campaign.Cycle, CampaignStatus(s, b), maxShare, balance.HardBandPlayerShareCeiling(7))
	}
	if maxShare > balance.HardBandPlayerShareCeiling(7)+1e-3 {
		t.Fatalf("maxShare %v exceeded hard-band ceiling (share formula changed?)", maxShare)
	}
}
