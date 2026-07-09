package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestEffectiveComputeSumsProcesses(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Compute: model.Compute{
		RentedTraining:  map[string]int{"N7": 2, "N5": 4}, // 2*1 + 4*2 = 10
		RentedInference: map[string]int{"N7": 1},          // 1*1 = 1
	}}
	if got := EffectiveTraining(s, b); got != 10 {
		t.Fatalf("EffectiveTraining = %v, want 10", got)
	}
	if got := EffectiveInference(s, b); got != 1 {
		t.Fatalf("EffectiveInference = %v, want 1", got)
	}
	// nil maps → 0, no panic
	if EffectiveTraining(model.GameState{}, b) != 0 {
		t.Fatal("nil map should give 0")
	}
}

func TestRentComputeRespectsLockAndPool(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	// N5 locked at start
	if _, err := Apply(s, model.RentCompute{Process: "N5", Pool: model.PoolTraining, Delta: 1}, b); err != ErrProcessLocked {
		t.Fatalf("locked process: err = %v, want ErrProcessLocked", err)
	}
	// N7 available
	ns, err := Apply(s, model.RentCompute{Process: "N7", Pool: model.PoolInference, Delta: 3}, b)
	if err != nil || ns.Compute.RentedInference["N7"] != 3 {
		t.Fatalf("rent N7 inf: %+v err=%v", ns.Compute.RentedInference, err)
	}
	// floors at 0, input not mutated
	ns2, _ := Apply(ns, model.RentCompute{Process: "N7", Pool: model.PoolInference, Delta: -10}, b)
	if ns2.Compute.RentedInference["N7"] != 0 {
		t.Fatalf("should floor at 0, got %v", ns2.Compute.RentedInference["N7"])
	}
	if ns.Compute.RentedInference["N7"] != 3 {
		t.Fatal("Apply mutated input map")
	}
}

func TestRevenueMultScalesRevenue(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Models: []model.Model{{Online: true, Users: 1000, Price: 12}}}
	ns := Tick(s, 1, nil, b)
	// revenue = 1000*12/MonthSec*RevenueMult(2) ; just assert it doubled vs mult=1
	b1 := b
	b1.RevenueMult = 1
	ns1 := Tick(s, 1, nil, b1)
	gain2 := ns.Resources.Cash
	gain1 := ns1.Resources.Cash
	if gain2 <= gain1 {
		t.Fatalf("RevenueMult=2 should out-earn 1: %v vs %v", gain2, gain1)
	}
}
