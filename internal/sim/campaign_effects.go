package sim

import (
	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

func multiplyCampaignEffects(dst *model.CampaignEffects, src model.CampaignEffects) {
	for i := 0; i < model.NumSegments; i++ {
		dst.UserGrowthMult[i] *= src.UserGrowthMult[i]
		dst.RefPriceMult[i] *= src.RefPriceMult[i]
		dst.RevenueMult[i] *= src.RevenueMult[i]
	}
	dst.InferenceLoadMult *= src.InferenceLoadMult
	dst.ServiceChurnMult *= src.ServiceChurnMult
	dst.SafetyAppealMult *= src.SafetyAppealMult
	dst.RivalImpactMult *= src.RivalImpactMult
}

func campaignEffects(s model.GameState, b balance.Config) model.CampaignEffects {
	out := model.NeutralCampaignEffects()
	for _, id := range s.Campaign.Perks {
		if p, ok := balance.CampaignPerkByID(b.Campaign, id); ok {
			multiplyCampaignEffects(&out, p.Effects)
		}
	}
	if p, ok := balance.CampaignPerkByID(b.Campaign, s.Campaign.SecondaryPerk); ok && p.Tier == 1 && p.Doctrine == s.Campaign.Secondary {
		multiplyCampaignEffects(&out, p.Effects)
	}
	for _, m := range s.Campaign.Active {
		if m.CyclesRemaining > 0 {
			multiplyCampaignEffects(&out, m.Effects)
		}
	}
	return out
}
