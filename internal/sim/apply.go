package sim

import (
	"errors"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// ErrUnknownCommand is returned by Apply for an unrecognized command type.
var ErrUnknownCommand = errors.New("sim: unknown command")

// Apply validates and applies a single player command, returning the new
// state. Pure: it does not mutate s.
func Apply(s model.GameState, cmd model.Command, b balance.Config) (model.GameState, error) {
	switch c := cmd.(type) {
	case model.RentTrainingCompute:
		return applyRentTrainingCompute(s, c), nil
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
