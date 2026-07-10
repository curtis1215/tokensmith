package tui

import (
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

// Summary describes what an offline settlement produced, for the return banner.
type Summary struct {
	RnDGained          float64
	SecondsSettled     float64
	TrainingCompleted  bool
	TokensIn           int
	TokensOut          int
	EventsFired        int
	EventsAutoResolved int
	// CampaignCycles is the number of board cycles advanced during offline
	// catch-up (capped). The banner only reports the count; board reports
	// hold the detailed source of truth.
	CampaignCycles int
}

const (
	settleChunkSec = 3600.0      // advance in ≤1h steps to bound Euler error
	settleMaxSec   = 7 * 86400.0 // cap absurd offline windows at 7 days
)

// Settle advances the pure sim by elapsedSec (clamped to [0, 7d]) in ≤1h chunks,
// distributing the offline token batch evenly across the chunks. It returns the
// settled state and a summary for display.
func Settle(s model.GameState, b balance.Config, elapsedSec float64, offIn, offOut int) (model.GameState, Summary) {
	if elapsedSec < 0 {
		elapsedSec = 0
	}
	if elapsedSec > settleMaxSec {
		elapsedSec = settleMaxSec
	}
	sum := Summary{SecondsSettled: elapsedSec, TokensIn: offIn, TokensOut: offOut}
	beforeRnD := s.Resources.RnD
	beforeFired := s.Events.FiredCount
	beforeAuto := s.Events.AutoCount
	wasTraining := s.HasTraining

	chunks := int(elapsedSec / settleChunkSec)
	if float64(chunks)*settleChunkSec < elapsedSec {
		chunks++
	}
	if chunks == 0 && (offIn > 0 || offOut > 0) {
		chunks = 1 // still apply the tokens even with ~0 elapsed
	}

	remaining := elapsedSec
	for i := 0; i < chunks; i++ {
		dt := settleChunkSec
		if remaining < dt {
			dt = remaining
		}
		remaining -= dt

		var ev []model.TokenEvent
		if offIn > 0 || offOut > 0 {
			ci := offIn / chunks
			co := offOut / chunks
			if i == chunks-1 { // last chunk absorbs the division remainder
				ci = offIn - ci*(chunks-1)
				co = offOut - co*(chunks-1)
			}
			ev = []model.TokenEvent{{InputTokens: ci, OutputTokens: co}}
		}
		s = sim.Tick(s, dt, ev, b)
	}

	sum.RnDGained = s.Resources.RnD - beforeRnD
	sum.EventsFired = s.Events.FiredCount - beforeFired
	sum.EventsAutoResolved = s.Events.AutoCount - beforeAuto
	sum.TrainingCompleted = wasTraining && !s.HasTraining
	return s, sum
}
