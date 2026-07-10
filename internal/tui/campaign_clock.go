package tui

import (
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

// campaignCyclesDue computes how many board cycles are due from wall-clock
// elapsed since last, capped so long offline windows do not dump a backlog.
// When last is uninitialized or inputs are invalid, due is 0 and nextLast is now
// (so the first live session can arm the clock without firing retroactively).
// When raw due exceeds cap, nextLast jumps to now (drops old backlog).
// Otherwise nextLast advances by whole cycles so cadence is preserved.
func campaignCyclesDue(last, now, cycleSec int64, cap int) (due int, nextLast int64) {
	if last <= 0 || now <= last || cycleSec <= 0 || cap <= 0 {
		return 0, now
	}
	raw := int((now - last) / cycleSec)
	if raw <= 0 {
		return 0, last
	}
	if raw > cap {
		return cap, now
	}
	return raw, last + int64(raw)*cycleSec
}

// advanceCampaignTo settles due board cycles against wall-clock now.
// Wall-clock stays entirely in the TUI; the pure sim only sees cycle steps.
// Cap/dropped-backlog semantics match campaignCyclesDue (Task 2).
// Before doctrine selection, arms lastCampaignUnix when uninitialized and
// advances zero cycles (no retroactive board reports).
func (m Model) advanceCampaignTo(now int64) (Model, int) {
	if m.state.Campaign.Doctrine == model.DoctrineNone {
		if m.lastCampaignUnix == 0 {
			m.lastCampaignUnix = now
		}
		return m, 0
	}
	due, next := campaignCyclesDue(m.lastCampaignUnix, now, m.cfg.Campaign.CycleSec, m.cfg.Campaign.MaxCatchupCycles)
	for i := 0; i < due; i++ {
		m.state = sim.AdvanceCampaignCycle(m.state, m.cfg)
	}
	m.lastCampaignUnix = next
	return m, due
}
