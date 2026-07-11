package sim

import (
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// ProgressionView is a pure, read-only projection of the active frontier
// research project for TUI and status consumers.
//
// Fractions and ETA use the project's snapshotted totals/remaining — not a
// re-resolved Generation() row — so mid-run balance edits cannot rewrite ETA.
type ProgressionView struct {
	Active             bool
	TargetGen          int
	TargetEra          int
	RnDFraction        float64 // completed fraction in [0,1] from snapshots
	WorkFraction       float64
	AllocationPct      int
	ModelAllocationPct int
	AllocatedCompute   float64
	DiminishedCompute  float64
	RecommendedCompute float64
	ETASec             float64 // WorkRemaining / diminished when progressing; else 0
	// UnavailableReason is "no-compute", "no-rnd", "paused", or "" when active
	// and progressing (or when no project is active).
	UnavailableReason string
}

// FrontierProgressView builds a ProgressionView from the current state.
func FrontierProgressView(s model.GameState, b balance.Config) ProgressionView {
	fp := s.Progression.Frontier
	if !fp.Active {
		return ProgressionView{}
	}
	v := ProgressionView{
		Active:             true,
		TargetGen:          fp.TargetGen,
		AllocationPct:      fp.AllocationPct,
		ModelAllocationPct: 100 - fp.AllocationPct,
		RecommendedCompute: fp.RecommendedCompute,
	}
	if era, err := balance.EraForGen(fp.TargetGen); err == nil {
		v.TargetEra = era
	}
	if fp.RnDTotal > 0 {
		v.RnDFraction = 1 - fp.RnDRemaining/fp.RnDTotal
		if v.RnDFraction < 0 {
			v.RnDFraction = 0
		}
		if v.RnDFraction > 1 {
			v.RnDFraction = 1
		}
	}
	if fp.WorkTotal > 0 {
		v.WorkFraction = 1 - fp.WorkRemaining/fp.WorkTotal
		if v.WorkFraction < 0 {
			v.WorkFraction = 0
		}
		if v.WorkFraction > 1 {
			v.WorkFraction = 1
		}
	}

	trainEff := effectiveTraining(s, b)
	pct := fp.AllocationPct
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	v.AllocatedCompute = trainEff * float64(pct) / 100
	v.DiminishedCompute = diminishedFrontierCompute(v.AllocatedCompute, fp.RecommendedCompute)

	v.UnavailableReason = frontierUnavailableReason(s, v.AllocatedCompute, pct)
	if v.UnavailableReason == "" && v.DiminishedCompute > 0 && fp.WorkRemaining > 0 {
		v.ETASec = fp.WorkRemaining / v.DiminishedCompute
	}
	return v
}

func frontierUnavailableReason(s model.GameState, allocated float64, allocPct int) string {
	if allocPct == 0 {
		return "paused"
	}
	if allocated <= simEpsilon {
		return "no-compute"
	}
	// Wallet R&D only blocks when the project still owes frontier R&D.
	if s.Progression.Frontier.RnDRemaining > simEpsilon && s.Resources.RnD <= simEpsilon {
		return "no-rnd"
	}
	return ""
}
