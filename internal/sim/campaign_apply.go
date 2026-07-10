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
	if ns.Campaign.Legacy.Kind == model.LegacySecondary {
		ns.Campaign.Secondary, ns.Campaign.SecondaryPerk = ns.Campaign.Legacy.Doctrine, ns.Campaign.Legacy.PerkID
	}
	ns = initCampaignRoadmaps(ns, c.Doctrine, b)
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
