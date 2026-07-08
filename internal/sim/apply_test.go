package sim

import (
	"testing"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func TestApplyRentTrainingComputeAdds(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Compute.TrainingCapacity = 2
	ns, err := Apply(s, model.RentTrainingCompute{Delta: 3}, b)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ns.Compute.TrainingCapacity != 5 {
		t.Fatalf("capacity = %v, want 5", ns.Compute.TrainingCapacity)
	}
	// input not mutated
	if s.Compute.TrainingCapacity != 2 {
		t.Fatalf("Apply mutated input: %v", s.Compute.TrainingCapacity)
	}
}

func TestApplyRentTrainingComputeFloorsAtZero(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Compute.TrainingCapacity = 2
	ns, _ := Apply(s, model.RentTrainingCompute{Delta: -5}, b)
	if ns.Compute.TrainingCapacity != 0 {
		t.Fatalf("capacity = %v, want 0", ns.Compute.TrainingCapacity)
	}
}

func validAlloc() [model.NumQualityDims]float64 {
	return [model.NumQualityDims]float64{0.4, 0.2, 0.2, 0.2}
}

func TestApplyStartTrainingSuccess(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Resources.RnD = 50000 // > Gen1 cost 20000
	cmd := model.StartTraining{Gen: 1, Alloc: validAlloc(), Price: 12}
	ns, err := Apply(s, cmd, b)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ns.Resources.RnD != 30000 { // 50000 - 20000
		t.Errorf("RnD = %v, want 30000", ns.Resources.RnD)
	}
	if !ns.HasTraining || ns.Training.Gen != 1 || ns.Training.Price != 12 {
		t.Errorf("training not set: %+v", ns.Training)
	}
	if ns.Training.WorkRemaining != 1800 {
		t.Errorf("WorkRemaining = %v, want 1800", ns.Training.WorkRemaining)
	}
	if s.HasTraining {
		t.Errorf("Apply mutated input")
	}
}

func TestApplyStartTrainingErrors(t *testing.T) {
	b := balance.Default()
	base := model.GameState{}
	base.Resources.RnD = 50000

	// already training
	busy := base
	busy.HasTraining = true
	if _, err := Apply(busy, model.StartTraining{Gen: 1, Alloc: validAlloc()}, b); err != ErrTrainingInProgress {
		t.Errorf("busy: err = %v, want ErrTrainingInProgress", err)
	}
	// invalid gen
	if _, err := Apply(base, model.StartTraining{Gen: 9, Alloc: validAlloc()}, b); err != ErrInvalidGen {
		t.Errorf("gen: err = %v, want ErrInvalidGen", err)
	}
	// bad alloc (sums to 0.8)
	bad := [model.NumQualityDims]float64{0.4, 0.2, 0.1, 0.1}
	if _, err := Apply(base, model.StartTraining{Gen: 1, Alloc: bad}, b); err != ErrInvalidAlloc {
		t.Errorf("alloc: err = %v, want ErrInvalidAlloc", err)
	}
	// insufficient R&D
	poor := model.GameState{}
	poor.Resources.RnD = 100
	if _, err := Apply(poor, model.StartTraining{Gen: 1, Alloc: validAlloc()}, b); err != ErrInsufficientRnD {
		t.Errorf("poor: err = %v, want ErrInsufficientRnD", err)
	}
}

func TestApplySetPriceSuccess(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Models: []model.Model{{Online: true, Price: 12}}}
	ns, err := Apply(s, model.SetPrice{ModelIndex: 0, Price: 20}, b)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ns.Models[0].Price != 20 {
		t.Errorf("price = %v, want 20", ns.Models[0].Price)
	}
	if s.Models[0].Price != 12 {
		t.Errorf("Apply mutated input Models (price = %v)", s.Models[0].Price)
	}
}

func TestApplySetPriceErrors(t *testing.T) {
	b := balance.Default()
	s := model.GameState{Models: []model.Model{{Online: true, Price: 12}}}
	if _, err := Apply(s, model.SetPrice{ModelIndex: 5, Price: 20}, b); err != ErrInvalidModelIndex {
		t.Errorf("index: err = %v, want ErrInvalidModelIndex", err)
	}
	if _, err := Apply(s, model.SetPrice{ModelIndex: 0, Price: 0}, b); err != ErrInvalidPrice {
		t.Errorf("price: err = %v, want ErrInvalidPrice", err)
	}
}

func TestApplyRentInferenceCompute(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Compute.InferenceCapacity = 2
	ns, err := Apply(s, model.RentInferenceCompute{Delta: 3}, b)
	if err != nil || ns.Compute.InferenceCapacity != 5 {
		t.Fatalf("capacity = %v err=%v, want 5", ns.Compute.InferenceCapacity, err)
	}
	ns2, _ := Apply(s, model.RentInferenceCompute{Delta: -10}, b)
	if ns2.Compute.InferenceCapacity != 0 {
		t.Fatalf("should floor at 0, got %v", ns2.Compute.InferenceCapacity)
	}
	if s.Compute.InferenceCapacity != 2 {
		t.Fatalf("Apply mutated input")
	}
}

func TestApplyExpandDatacenter(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Resources.Cash = 1_000_000
	ns, err := Apply(s, model.ExpandDatacenter{PowerDelta: 800, SlotDelta: 20}, b)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ns.Datacenter.PowerCapacity != 800 || ns.Datacenter.SlotCapacity != 20 {
		t.Errorf("capacity wrong: %+v", ns.Datacenter)
	}
	wantCost := 800*b.PowerCostPerKW + 20*b.SlotCost
	if !approx(ns.Resources.Cash, 1_000_000-wantCost) {
		t.Errorf("cash = %v, want %v", ns.Resources.Cash, 1_000_000-wantCost)
	}
	if s.Datacenter.PowerCapacity != 0 {
		t.Errorf("Apply mutated input")
	}
}

func TestApplyExpandDatacenterInsufficientCash(t *testing.T) {
	b := balance.Default()
	s := model.GameState{}
	s.Resources.Cash = 100
	if _, err := Apply(s, model.ExpandDatacenter{PowerDelta: 800, SlotDelta: 20}, b); err != ErrInsufficientCash {
		t.Fatalf("err = %v, want ErrInsufficientCash", err)
	}
}
