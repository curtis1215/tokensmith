package sim

import (
	"errors"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// ErrUnknownCommand is returned by Apply for an unrecognized command type.
var ErrUnknownCommand = errors.New("sim: unknown command")

var (
	ErrTrainingInProgress = errors.New("sim: training already in progress")
	ErrInsufficientRnD    = errors.New("sim: insufficient R&D")
	ErrInvalidGen         = errors.New("sim: invalid generation")
	ErrInvalidAlloc       = errors.New("sim: allocation must sum to 1")
	ErrInvalidModelIndex  = errors.New("sim: invalid model index")
	ErrInvalidPrice       = errors.New("sim: price must be positive")
)

// Apply validates and applies a single player command, returning the new
// state. Pure: it does not mutate s.
func Apply(s model.GameState, cmd model.Command, b balance.Config) (model.GameState, error) {
	switch c := cmd.(type) {
	case model.RentTrainingCompute:
		return applyRentTrainingCompute(s, c), nil
	case model.StartTraining:
		return applyStartTraining(s, c, b)
	case model.SetPrice:
		return applySetPrice(s, c)
	case model.RentInferenceCompute:
		return applyRentInferenceCompute(s, c), nil
	default:
		return s, ErrUnknownCommand
	}
}

func applyRentTrainingCompute(s model.GameState, c model.RentTrainingCompute) model.GameState {
	ns := s
	ns.Compute.TrainingCapacity += c.Delta
	if ns.Compute.TrainingCapacity < 0 {
		ns.Compute.TrainingCapacity = 0
	}
	return ns
}

func applyStartTraining(s model.GameState, c model.StartTraining, b balance.Config) (model.GameState, error) {
	if s.HasTraining {
		return s, ErrTrainingInProgress
	}
	if c.Gen < 1 || c.Gen > balance.MaxGen {
		return s, ErrInvalidGen
	}
	var sum float64
	for _, a := range c.Alloc {
		if a < 0 {
			return s, ErrInvalidAlloc
		}
		sum += a
	}
	if sum < 0.999 || sum > 1.001 {
		return s, ErrInvalidAlloc
	}
	cost := b.GenRnDCost[c.Gen]
	if s.Resources.RnD < cost {
		return s, ErrInsufficientRnD
	}
	ns := s
	ns.Resources.RnD -= cost
	ns.HasTraining = true
	ns.Training = model.TrainingJob{
		Gen:           c.Gen,
		Alloc:         c.Alloc,
		Price:         c.Price,
		WorkRemaining: b.GenTrainWorkGPUSec[c.Gen],
	}
	return ns, nil
}

func applySetPrice(s model.GameState, c model.SetPrice) (model.GameState, error) {
	if c.ModelIndex < 0 || c.ModelIndex >= len(s.Models) {
		return s, ErrInvalidModelIndex
	}
	if c.Price <= 0 {
		return s, ErrInvalidPrice
	}
	ns := s
	ns.Models = append([]model.Model(nil), s.Models...)
	ns.Models[c.ModelIndex].Price = c.Price
	return ns, nil
}

func applyRentInferenceCompute(s model.GameState, c model.RentInferenceCompute) model.GameState {
	ns := s
	ns.Compute.InferenceCapacity += c.Delta
	if ns.Compute.InferenceCapacity < 0 {
		ns.Compute.InferenceCapacity = 0
	}
	return ns
}
