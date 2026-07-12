package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestEffectiveRivalQualityIdentityV1(t *testing.T) {
	b := balance.Default()
	c := model.Competitor{Quality: [4]float64{1, 2, 3, 4}}
	got := EffectiveRivalQuality(model.GameState{}, c, b)
	if got != c.Quality {
		t.Fatalf("%v != %v", got, c.Quality)
	}
}

// Smoke: MarketRank / SegmentShareBars still run after rival-quality rewiring.
func TestRivalQualityViewsSmoke(t *testing.T) {
	b := balance.Default()
	s := model.GameState{
		Models: []model.Model{{
			Name: "Player", Online: true, Segment: model.SegConsumer,
			Quality: [model.NumQualityDims]float64{20, 20, 20, 20},
		}},
		Competitors: []model.Competitor{{
			Name:    "Rival",
			Quality: [model.NumQualityDims]float64{10, 10, 10, 10},
		}},
	}
	rank, total := MarketRank(s, b, model.SegConsumer)
	if rank < 1 || total < 2 {
		t.Fatalf("MarketRank = %d/%d", rank, total)
	}
	bars := SegmentShareBars(s, b, model.SegConsumer)
	if len(bars) < 2 {
		t.Fatalf("SegmentShareBars len = %d", len(bars))
	}
}
