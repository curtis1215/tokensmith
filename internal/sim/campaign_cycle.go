package sim

import (
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func ageCampaignModifiers(in []model.CampaignModifier) []model.CampaignModifier {
	out := make([]model.CampaignModifier, 0, len(in))
	for _, m := range in {
		m.CyclesRemaining--
		if m.CyclesRemaining > 0 {
			out = append(out, m)
		}
	}
	return out
}

func appendBoardReport(in []model.BoardReport, report model.BoardReport, cap int) []model.BoardReport {
	out := append(append([]model.BoardReport(nil), in...), report)
	if cap > 0 && len(out) > cap {
		out = out[len(out)-cap:]
	}
	return out
}

// AdvanceCampaignCycle applies one pure board-cycle transaction: age modifiers,
// execute due rival roadmap actions, advance campaign progress once, track
// financial distress, reset the per-cycle directive flag, and append a capped
// board report. No-op when no doctrine is selected.
func AdvanceCampaignCycle(s model.GameState, b balance.Config) model.GameState {
	if s.Campaign.Doctrine == model.DoctrineNone {
		return s
	}
	ns := s
	ns.Campaign.Cycle++
	ns.Campaign.Active = ageCampaignModifiers(ns.Campaign.Active)
	// Age roadmap momentum before new actions so a just-set cycle is not
	// immediately decayed on the same board tick that applied it.
	ns = ageRivalMomentum(ns)
	var entries []model.CampaignReportEntry
	ns, entries = advanceRivalRoadmap(ns, true, b, entries)
	ns, entries = advanceRivalRoadmap(ns, false, b, entries)
	// Board-cycle public update: re-assert the global-frontier band for all rivals.
	ns = clampAllRivalsToBand(ns, b)
	var progress []model.CampaignReportEntry
	ns, progress = advanceCampaignProgress(ns, b)
	entries = append(entries, progress...)
	if ns.Resources.Cash < 0 {
		ns.Campaign.FinancialDistressCycles++
		entries = append(entries, model.CampaignReportEntry{Kind: model.ReportFinancialRisk, Value: ns.Resources.Cash})
	} else {
		ns.Campaign.FinancialDistressCycles = 0
	}
	ns.Campaign.DirectiveUsed = false
	ns.Campaign.Reports = appendBoardReport(ns.Campaign.Reports, model.BoardReport{Cycle: ns.Campaign.Cycle, Entries: entries}, b.Campaign.ReportCap)
	return ns
}
