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
