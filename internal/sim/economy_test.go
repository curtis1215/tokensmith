package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestEarlyGameBreakEven(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Research.EfficiencyMult = 1
	s.Compute.RentedInference = map[string]int{"N7": 1}
	s.Models = []model.Model{{Online: true, Segment: model.SegConsumer, Price: 12, Users: 1000,
		Quality: [model.NumQualityDims]float64{25, 0, 0, 0}}}
	before := s.Resources.Cash
	s = Tick(s, 3600, nil, b) // one hour
	if s.Resources.Cash <= before {
		t.Fatalf("Gen1 ~1000 users + 1 N7 + 3 staff should be cash-positive, delta=%v", s.Resources.Cash-before)
	}
}
