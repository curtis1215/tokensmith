package sim

import (
	"fmt"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// campaignRand advances a SplitMix64 state and returns the next state plus a
// uniform float in [0, 1).
func campaignRand(state uint64) (uint64, float64) {
	state += 0x9e3779b97f4a7c15
	z := state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	z ^= z >> 31
	return state, float64(z>>11) / float64(uint64(1)<<53)
}

func pickRival(candidates []balance.RivalProfile, state uint64) (balance.RivalProfile, uint64) {
	next, roll := campaignRand(state)
	idx := int(roll * float64(len(candidates)))
	if idx >= len(candidates) {
		idx = len(candidates) - 1
	}
	return candidates[idx], next
}

func roadmapFor(p balance.RivalProfile, b balance.Config) model.RivalRoadmap {
	if len(p.Actions) == 0 {
		return model.RivalRoadmap{Company: p.Name, ActionIndex: 0, CyclesUntilAction: 0}
	}
	a, _ := balance.RivalActionByID(b.Campaign, p.Actions[0])
	return model.RivalRoadmap{Company: p.Name, ActionIndex: 0, CyclesUntilAction: a.LeadCycles}
}

// initCampaignRoadmaps seeds Primary and Wildcard roadmaps from the campaign
// catalog using Campaign.RandState. Missing catalog entries leave an inert
// roadmap rather than panicking.
func initCampaignRoadmaps(s model.GameState, doctrine model.Doctrine, b balance.Config) model.GameState {
	ns := s
	var primaryCands []balance.RivalProfile
	for _, p := range b.Campaign.Rivals {
		for _, d := range p.PrimaryFor {
			if d == doctrine {
				primaryCands = append(primaryCands, p)
				break
			}
		}
	}
	if len(primaryCands) == 0 {
		return ns
	}
	primary, state := pickRival(primaryCands, ns.Campaign.RandState)
	var wildCands []balance.RivalProfile
	for _, p := range b.Campaign.Rivals {
		if p.Name != primary.Name {
			wildCands = append(wildCands, p)
		}
	}
	if len(wildCands) == 0 {
		ns.Campaign.RandState = state
		ns.Campaign.Primary = roadmapFor(primary, b)
		return ns
	}
	wildcard, state := pickRival(wildCands, state)
	ns.Campaign.RandState = state
	ns.Campaign.Primary = roadmapFor(primary, b)
	ns.Campaign.Wildcard = roadmapFor(wildcard, b)
	return ns
}

// roadmapActionID returns the currently telegraphed action ID for a roadmap.
// Exposed for Task 6 directive counter/intel resolution.
func roadmapActionID(r model.RivalRoadmap, b balance.Config) (string, bool) {
	p, ok := balance.RivalProfileByName(b.Campaign, r.Company)
	if !ok || len(p.Actions) == 0 {
		return "", false
	}
	return p.Actions[r.ActionIndex%len(p.Actions)], true
}

// campaignRoadmapByCompany finds the primary or wildcard roadmap for company.
func campaignRoadmapByCompany(s model.GameState, company string) (model.RivalRoadmap, bool) {
	if s.Campaign.Primary.Company == company {
		return s.Campaign.Primary, true
	}
	if s.Campaign.Wildcard.Company == company {
		return s.Campaign.Wildcard, true
	}
	return model.RivalRoadmap{}, false
}

// executeRivalAction applies one due roadmap action: quality mults, optional
// ref-price modifier, counter consumption, action-index advance, and report.
// Clones Competitors and Campaign.Active before mutation. ok=false leaves the
// roadmap inert (no report) when the profile/action is missing.
func executeRivalAction(s model.GameState, roadmap model.RivalRoadmap, b balance.Config) (model.GameState, model.RivalRoadmap, model.CampaignReportEntry, bool) {
	if roadmap.Company == "" {
		return s, roadmap, model.CampaignReportEntry{}, false
	}
	profile, ok := balance.RivalProfileByName(b.Campaign, roadmap.Company)
	if !ok || len(profile.Actions) == 0 {
		return s, roadmap, model.CampaignReportEntry{}, false
	}
	actionID := profile.Actions[roadmap.ActionIndex%len(profile.Actions)]
	action, ok := balance.RivalActionByID(b.Campaign, actionID)
	if !ok {
		return s, roadmap, model.CampaignReportEntry{}, false
	}

	ns := s
	impact := campaignEffects(ns, b).RivalImpactMult
	matched := ns.Campaign.CounterTarget == roadmap.Company && ns.Campaign.CounterActionID == actionID
	if matched {
		impact *= 0.5
		ns.Campaign.CounterTarget = ""
		ns.Campaign.CounterActionID = ""
	}

	comps := append([]model.Competitor(nil), ns.Competitors...)
	for i := range comps {
		if comps[i].Name != roadmap.Company {
			continue
		}
		for d := range model.NumQualityDims {
			pct := action.QualityPct[d]
			if pct != 0 {
				comps[i].Quality[d] *= 1 + pct*impact
			}
		}
		break
	}
	ns.Competitors = comps

	if action.RefPriceMult > 0 {
		e := model.NeutralCampaignEffects()
		// Blend toward 1 by impact so counters also soften price-war effects.
		e.RefPriceMult[action.Segment] = 1 + (action.RefPriceMult-1)*impact
		ns.Campaign.Active = append(append([]model.CampaignModifier(nil), ns.Campaign.Active...), model.CampaignModifier{
			ID:              fmt.Sprintf("rival-%s-%d", actionID, ns.Campaign.Cycle),
			CyclesRemaining: action.DurationCycles,
			Effects:         e,
		})
	}

	roadmap.LastExecutedCycle = ns.Campaign.Cycle
	roadmap.ActionIndex = (roadmap.ActionIndex + 1) % len(profile.Actions)
	nextID := profile.Actions[roadmap.ActionIndex]
	if next, ok := balance.RivalActionByID(b.Campaign, nextID); ok {
		roadmap.CyclesUntilAction = next.LeadCycles
	} else {
		roadmap.CyclesUntilAction = 0
	}

	entry := model.CampaignReportEntry{
		Kind:      model.ReportRivalAction,
		SubjectID: roadmap.Company,
		DetailID:  actionID,
		Countered: matched,
	}
	return ns, roadmap, entry, true
}

// advanceRivalRoadmap decrements the primary (or wildcard) countdown and
// executes when it reaches zero. Appends a report entry only when execution
// succeeds.
func advanceRivalRoadmap(s model.GameState, primary bool, b balance.Config, entries []model.CampaignReportEntry) (model.GameState, []model.CampaignReportEntry) {
	ns := s
	roadmap := ns.Campaign.Primary
	if !primary {
		roadmap = ns.Campaign.Wildcard
	}
	if roadmap.Company == "" {
		return ns, entries
	}
	roadmap.CyclesUntilAction--
	if roadmap.CyclesUntilAction != 0 {
		if primary {
			ns.Campaign.Primary = roadmap
		} else {
			ns.Campaign.Wildcard = roadmap
		}
		return ns, entries
	}
	var entry model.CampaignReportEntry
	var ok bool
	ns, roadmap, entry, ok = executeRivalAction(ns, roadmap, b)
	if primary {
		ns.Campaign.Primary = roadmap
	} else {
		ns.Campaign.Wildcard = roadmap
	}
	if ok {
		entries = append(entries, entry)
	}
	return ns, entries
}
