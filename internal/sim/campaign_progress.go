package sim

import (
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// campaignGateMet reports whether the current stage's approved route gate is met.
// Shared by CampaignStatus progress and advanceCampaignProgress transitions.
func campaignGateMet(s model.GameState, b balance.Config, status CampaignStatusView) bool {
	switch s.Campaign.Stage {
	case model.CampaignStageEstablish:
		return status.Share >= b.Campaign.EstablishShare && hasOnlineModelInSegment(s, doctrineSegment(s.Campaign.Doctrine))
	case model.CampaignStageExpand:
		switch s.Campaign.Doctrine {
		case model.DoctrineConsumer:
			return status.Share >= b.Campaign.ConsumerExpandShare && campaignCapacityOK(s, b, 0.90)
		case model.DoctrineEnterprise:
			return enterpriseSafetyOK(s, b) && status.Share >= b.Campaign.EnterpriseExpandShare && status.PriceOK
		case model.DoctrineDeveloper:
			return status.QualityRank <= 3 && status.Share >= b.Campaign.DeveloperExpandShare && status.PriceOK
		}
	case model.CampaignStageShowdown:
		switch s.Campaign.Doctrine {
		case model.DoctrineConsumer:
			return status.QualityRank == 1 && status.Share >= b.Campaign.ConsumerWinShare && campaignCapacityOK(s, b, 1.0)
		case model.DoctrineEnterprise:
			return enterpriseSafetyOK(s, b) && status.Share >= b.Campaign.EnterpriseWinShare && status.PriceOK && s.Ops >= 1 && campaignCapacityOK(s, b, 0.80)
		case model.DoctrineDeveloper:
			return status.QualityRank == 1 && status.Share >= b.Campaign.DeveloperWinShare && status.PriceOK && status.CashflowOK && campaignCapacityOK(s, b, 0.80)
		}
	}
	return false
}

// advanceCampaignProgress applies deterministic stage / showdown / victory
// transitions. Task 5 calls this after rival actions each board cycle.
func advanceCampaignProgress(s model.GameState, b balance.Config) (model.GameState, []model.CampaignReportEntry) {
	if s.Campaign.Doctrine == model.DoctrineNone || s.Campaign.Victory != model.DoctrineNone || s.Campaign.Endless {
		return s, nil
	}
	ns := s
	status := CampaignStatus(ns, b)
	if ns.Campaign.Stage == model.CampaignStageEstablish && campaignGateMet(ns, b, status) {
		ns.Campaign.Stage, ns.Campaign.PerkTierPending = model.CampaignStageExpand, 1
		return ns, []model.CampaignReportEntry{{Kind: model.ReportStageAdvanced, SubjectID: string(ns.Campaign.Stage)}}
	}
	if ns.Campaign.Stage == model.CampaignStageExpand && len(ns.Campaign.Perks) >= 1 && campaignGateMet(ns, b, status) {
		ns.Campaign.Stage, ns.Campaign.PerkTierPending = model.CampaignStageShowdown, 2
		return ns, []model.CampaignReportEntry{{Kind: model.ReportStageAdvanced, SubjectID: string(ns.Campaign.Stage)}}
	}
	if ns.Campaign.Stage != model.CampaignStageShowdown || len(ns.Campaign.Perks) < 2 {
		return ns, nil
	}
	if !campaignGateMet(ns, b, status) {
		if ns.Campaign.ShowdownHeld > 0 {
			ns.Campaign.ShowdownAttempts++
		}
		ns.Campaign.ShowdownHeld = 0
		return ns, nil
	}
	if ns.Campaign.ShowdownStartedCycle == 0 {
		ns.Campaign.ShowdownStartedCycle = ns.Campaign.Cycle
		ns.Campaign.Primary.CyclesUntilAction = 1
		return ns, []model.CampaignReportEntry{{Kind: model.ReportShowdown, SubjectID: ns.Campaign.Primary.Company}}
	}
	if ns.Campaign.Primary.LastExecutedCycle < ns.Campaign.ShowdownStartedCycle {
		return ns, nil
	}
	ns.Campaign.ShowdownHeld++
	if ns.Campaign.ShowdownHeld < 2 {
		return ns, nil
	}
	ns.Campaign.Victory, ns.Campaign.Stage = ns.Campaign.Doctrine, model.CampaignStageWon
	return ns, []model.CampaignReportEntry{{Kind: model.ReportVictory, SubjectID: string(ns.Campaign.Doctrine)}}
}
