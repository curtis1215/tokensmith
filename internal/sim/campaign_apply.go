package sim

import (
	"fmt"
	"math"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func validDoctrine(d model.Doctrine) bool {
	return d == model.DoctrineConsumer || d == model.DoctrineEnterprise || d == model.DoctrineDeveloper
}

func hasOnlineModel(s model.GameState) bool {
	for _, m := range s.Models {
		if m.Online {
			return true
		}
	}
	return false
}

func applyChooseDoctrine(s model.GameState, c model.ChooseDoctrine, b balance.Config) (model.GameState, error) {
	if !validDoctrine(c.Doctrine) {
		return s, ErrInvalidDoctrine
	}
	if s.Campaign.Doctrine != model.DoctrineNone {
		return s, ErrDoctrineAlreadyChosen
	}
	if !hasOnlineModel(s) {
		return s, ErrCampaignNeedsModel
	}
	ns := s
	ns.Campaign.Doctrine = c.Doctrine
	ns.Campaign.Stage = model.CampaignStageEstablish
	legacy := ns.Campaign.Legacy
	if legacy.Kind == model.LegacySecondary {
		ns.Campaign.Secondary, ns.Campaign.SecondaryPerk = legacy.Doctrine, legacy.PerkID
	}
	ns = initCampaignRoadmaps(ns, c.Doctrine, b)
	if legacy.Kind == model.LegacyIntel {
		ns.Campaign.Primary.IntelFull = true
		ns.Campaign.Wildcard.IntelFull = true
	}
	// Secondary and intel are one-run: consume after doctrine selection.
	if legacy.Kind == model.LegacySecondary || legacy.Kind == model.LegacyIntel {
		ns.Campaign.Legacy = model.LegacyChoice{}
	}
	return ns, nil
}

func applyChooseDoctrinePerk(s model.GameState, c model.ChooseDoctrinePerk, b balance.Config) (model.GameState, error) {
	p, ok := balance.CampaignPerkByID(b.Campaign, c.PerkID)
	if !ok || p.Doctrine != s.Campaign.Doctrine {
		return s, ErrInvalidDoctrinePerk
	}
	if s.Campaign.PerkTierPending == 0 || p.Tier != s.Campaign.PerkTierPending {
		return s, ErrPerkChoiceNotReady
	}
	for _, id := range s.Campaign.Perks {
		if id == p.ID {
			return s, ErrAlreadyUnlocked
		}
	}
	ns := s
	ns.Campaign.Perks = append(append([]string(nil), s.Campaign.Perks...), p.ID)
	ns.Campaign.PerkTierPending = 0
	return ns, nil
}

func applyChooseSecondaryDoctrine(s model.GameState, c model.ChooseSecondaryDoctrine, b balance.Config) (model.GameState, error) {
	if !validDoctrine(c.Doctrine) || c.Doctrine == s.Campaign.Doctrine {
		return s, ErrInvalidDoctrine
	}
	if s.Campaign.Stage != model.CampaignStageShowdown {
		return s, ErrSecondaryNotReady
	}
	p, ok := balance.CampaignPerkByID(b.Campaign, c.PerkID)
	if !ok || p.Doctrine != c.Doctrine || p.Tier != 1 {
		return s, ErrInvalidDoctrinePerk
	}
	ns := s
	ns.Campaign.Secondary, ns.Campaign.SecondaryPerk = c.Doctrine, c.PerkID
	return ns, nil
}

func applyPivotDoctrine(s model.GameState, c model.PivotDoctrine, b balance.Config) (model.GameState, error) {
	if !validDoctrine(c.Doctrine) || c.Doctrine == s.Campaign.Doctrine {
		return s, ErrInvalidDoctrine
	}
	if s.Campaign.PivotUsed {
		return s, ErrPivotAlreadyUsed
	}
	if s.Campaign.Stage == model.CampaignStageShowdown || s.Campaign.Stage == model.CampaignStageWon {
		return s, ErrPivotLocked
	}
	cashCost := math.Max(b.Campaign.PivotCashFloor, MonthlyRevenue(s)*b.Campaign.PivotRevenueMonths)
	rndCost := s.Resources.RnD * b.Campaign.PivotRnDFrac
	if s.Resources.Cash < cashCost {
		return s, ErrInsufficientCash
	}
	if s.Resources.RnD < rndCost {
		return s, ErrInsufficientRnD
	}
	ns := s
	ns.Resources.Cash -= cashCost
	ns.Resources.RnD -= rndCost
	ns.Campaign.Doctrine, ns.Campaign.Secondary, ns.Campaign.SecondaryPerk = c.Doctrine, model.DoctrineNone, ""
	ns.Campaign.Stage, ns.Campaign.Perks = model.CampaignStageEstablish, nil
	ns.Campaign.PerkTierPending, ns.Campaign.PivotUsed = 0, true
	ns.Campaign.ShowdownHeld, ns.Campaign.ShowdownStartedCycle = 0, 0
	// Drop stale executive counter pin so it cannot soft-lock against re-seeded
	// roadmaps. Preserve Active and DirectiveUsed: Task 3 keeps combat modifiers,
	// and one-directive-per-cycle still spans the pivot within the same cycle.
	ns.Campaign.CounterTarget, ns.Campaign.CounterActionID = "", ""
	ns.Campaign.Primary, ns.Campaign.Wildcard = model.RivalRoadmap{}, model.RivalRoadmap{}
	ns = initCampaignRoadmaps(ns, c.Doctrine, b)
	return ns, nil
}

func applyIssueDirective(s model.GameState, c model.IssueDirective, b balance.Config) (model.GameState, error) {
	if s.Campaign.Doctrine == model.DoctrineNone {
		return s, ErrInvalidDirective
	}
	if s.Campaign.DirectiveUsed {
		return s, ErrDirectiveUsed
	}
	ns := s
	switch c.Kind {
	case model.DirectiveRoutePush:
		cost := math.Max(5000, MonthlyRevenue(s)*0.25)
		if s.Resources.Cash < cost {
			return s, ErrInsufficientCash
		}
		e := model.NeutralCampaignEffects()
		e.UserGrowthMult[doctrineSegment(s.Campaign.Doctrine)] = 1.20
		ns.Resources.Cash -= cost
		ns.Campaign.Active = append(append([]model.CampaignModifier(nil), s.Campaign.Active...), model.CampaignModifier{
			ID:              fmt.Sprintf("directive-route-push-%d", s.Campaign.Cycle),
			CyclesRemaining: 1,
			Effects:         e,
		})
	case model.DirectiveCounter:
		r, ok := campaignRoadmapByCompany(s, c.Target)
		if !ok {
			return s, ErrInvalidRivalTarget
		}
		if s.Campaign.CounterTarget != "" {
			return s, ErrRivalAlreadyCountered
		}
		actionID, ok := roadmapActionID(r, b)
		if !ok {
			return s, ErrInvalidRivalTarget
		}
		ns.Campaign.CounterTarget, ns.Campaign.CounterActionID = c.Target, actionID
	case model.DirectiveIntel:
		if c.Target == s.Campaign.Primary.Company {
			ns.Campaign.Primary.IntelFull = true
		} else if c.Target == s.Campaign.Wildcard.Company {
			ns.Campaign.Wildcard.IntelFull = true
		} else {
			return s, ErrInvalidRivalTarget
		}
	default:
		return s, ErrInvalidDirective
	}
	ns.Campaign.DirectiveUsed = true
	return ns, nil
}

func addDoctrineUnique(in []model.Doctrine, d model.Doctrine) []model.Doctrine {
	for _, x := range in {
		if x == d {
			return in
		}
	}
	return append(append([]model.Doctrine(nil), in...), d)
}

func validateLegacy(s model.GameState, leg model.LegacyChoice, b balance.Config) error {
	switch leg.Kind {
	case model.LegacyNone:
		return ErrInvalidLegacy
	case model.LegacySecondary:
		if leg.Doctrine != s.Campaign.Secondary || leg.PerkID != s.Campaign.SecondaryPerk {
			return ErrInvalidLegacy
		}
		p, ok := balance.CampaignPerkByID(b.Campaign, leg.PerkID)
		if !ok || p.Doctrine != leg.Doctrine || p.Tier != 1 {
			return ErrInvalidLegacy
		}
		return nil
	case model.LegacyIntel:
		// No payload required.
		return nil
	case model.LegacyTech:
		if leg.TechID == "" {
			return ErrInvalidLegacy
		}
		for _, id := range s.UnlockedTech {
			if id == leg.TechID {
				return nil
			}
		}
		return ErrInvalidLegacy
	default:
		return ErrInvalidLegacy
	}
}

func applyCampaignContinue(s model.GameState) (model.GameState, error) {
	if s.Campaign.Victory == model.DoctrineNone {
		return s, ErrCampaignNotWon
	}
	ns := s
	ns.Campaign.Endless = true
	return ns, nil
}

func applyCampaignExit(s model.GameState, b balance.Config) (model.GameState, error) {
	if s.Campaign.Cycle < b.Campaign.StrategyExitCycle && s.Campaign.FinancialDistressCycles < 2 {
		return s, ErrStrategyExitLocked
	}
	p := s.Prestige
	p.Patents += math.Floor(patentsFor(s.PeakValuation, b) * 0.5)
	p.PendingLegacy = model.LegacyChoice{}
	ns := freshRun(p, b)
	ns.Events.RandState, ns.Campaign.RandState = s.Events.RandState, s.Campaign.RandState
	return ns, nil
}

func applyCampaignPrestige(s model.GameState, c model.CampaignPrestige, b balance.Config) (model.GameState, error) {
	if s.Campaign.Victory == model.DoctrineNone {
		return s, ErrCampaignNotWon
	}
	if err := validateLegacy(s, c.Legacy, b); err != nil {
		return s, err
	}
	p := s.Prestige
	p.Patents += patentsFor(s.PeakValuation, b)
	p.RouteBadges = addDoctrineUnique(p.RouteBadges, s.Campaign.Victory)
	p.PendingLegacy = c.Legacy
	ns := freshRun(p, b)
	ns.Events.RandState, ns.Campaign.RandState = s.Events.RandState, s.Campaign.RandState
	return ns, nil
}
