package sim

import (
	"math"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
)

// CampaignStatusView is the read-only status of the active strategic route.
type CampaignStatusView struct {
	Active      bool
	Doctrine    model.Doctrine
	Stage       model.CampaignStage
	Share       float64
	QualityRank int
	PriceOK     bool
	CapacityOK  bool
	CashflowOK  bool
	Progress    float64
	Victory     bool
}

// RivalIntelView surfaces confirmed + rumored rival roadmap actions for the TUI.
type RivalIntelView struct {
	Company           string
	ConfirmedActionID string
	RumoredActionID   string
	CyclesUntilAction int
	IntelFull         bool
}

// CampaignStatus returns a pure read-only view of the player's route progress.
func CampaignStatus(s model.GameState, b balance.Config) CampaignStatusView {
	view := CampaignStatusView{
		Active:   s.Campaign.Doctrine != model.DoctrineNone,
		Doctrine: s.Campaign.Doctrine,
		Stage:    s.Campaign.Stage,
		Victory:  s.Campaign.Victory != model.DoctrineNone,
	}
	if s.Campaign.Doctrine == model.DoctrineNone {
		return view
	}
	seg := doctrineSegment(s.Campaign.Doctrine)
	view.Share = playerSegmentShare(s, b, seg)
	view.QualityRank = campaignQualityRank(s, b, seg)
	view.PriceOK = campaignPriceOK(s, b)
	view.CapacityOK = campaignCapacityOK(s, b, capacityLimitForStage(s))
	view.CashflowOK = NetCashPerSec(s, b) > 0
	view.Progress = campaignProgress(s, b, view)
	return view
}

// RouteVictoryStatus evaluates the showdown gate for doctrine d without mutating
// the source campaign. Used by the CEO war room in endless mode for the other
// two route goals; does not award badges or Prestige.
func RouteVictoryStatus(s model.GameState, b balance.Config, d model.Doctrine) CampaignStatusView {
	viewState := s
	viewState.Campaign.Doctrine = d
	viewState.Campaign.Stage = model.CampaignStageShowdown
	viewState.Campaign.Victory = model.DoctrineNone
	viewState.Campaign.Endless = false
	return CampaignStatus(viewState, b)
}

// CampaignRivalIntel resolves the primary (or wildcard) roadmap's current and
// next action IDs through RivalProfileByName. Unknown company/action → ok=false.
func CampaignRivalIntel(s model.GameState, b balance.Config, primary bool) (RivalIntelView, bool) {
	roadmap := s.Campaign.Primary
	if !primary {
		roadmap = s.Campaign.Wildcard
	}
	if roadmap.Company == "" {
		return RivalIntelView{}, false
	}
	profile, ok := balance.RivalProfileByName(b.Campaign, roadmap.Company)
	if !ok {
		return RivalIntelView{}, false
	}
	if roadmap.ActionIndex < 0 || roadmap.ActionIndex >= len(profile.Actions) {
		return RivalIntelView{}, false
	}
	confirmed := profile.Actions[roadmap.ActionIndex]
	if _, ok := balance.RivalActionByID(b.Campaign, confirmed); !ok {
		return RivalIntelView{}, false
	}
	// Match executeRivalAction: next action wraps via modulo. Empty/unknown
	// action IDs still fail closed (ok=false).
	if len(profile.Actions) == 0 {
		return RivalIntelView{}, false
	}
	next := profile.Actions[(roadmap.ActionIndex+1)%len(profile.Actions)]
	if _, ok := balance.RivalActionByID(b.Campaign, next); !ok {
		return RivalIntelView{}, false
	}
	return RivalIntelView{
		Company:           roadmap.Company,
		ConfirmedActionID: confirmed,
		RumoredActionID:   next,
		CyclesUntilAction: roadmap.CyclesUntilAction,
		IntelFull:         roadmap.IntelFull,
	}, true
}

func doctrineSegment(d model.Doctrine) model.Segment {
	switch d {
	case model.DoctrineEnterprise:
		return model.SegEnterprise
	case model.DoctrineDeveloper:
		return model.SegDeveloper
	default:
		return model.SegConsumer
	}
}

func bestRouteModel(s model.GameState, b balance.Config, seg model.Segment) (model.Model, float64, bool) {
	w := b.SegmentWeights[seg]
	ce := campaignEffects(s, b)
	if seg == model.SegEnterprise {
		w[model.DimSafety] *= ce.SafetyAppealMult
	}
	var best model.Model
	bestAppeal := 0.0
	found := false
	for _, m := range s.Models {
		if !m.Online || m.Segment != seg {
			continue
		}
		a := appealOf(m.Quality, w)
		if !found || a > bestAppeal {
			best, bestAppeal, found = m, a, true
		}
	}
	return best, bestAppeal, found
}

func campaignQualityRank(s model.GameState, b balance.Config, seg model.Segment) int {
	_, playerAppeal, found := bestRouteModel(s, b, seg)
	if !found {
		return len(s.Competitors) + 1
	}
	w := b.SegmentWeights[seg]
	rank := 1
	for _, c := range s.Competitors {
		if appealOf(EffectiveRivalQuality(s, c, b), w) > playerAppeal {
			rank++
		}
	}
	return rank
}

func enterpriseSafetyOK(s model.GameState, b balance.Config) bool {
	m, _, ok := bestRouteModel(s, b, model.SegEnterprise)
	if !ok {
		return false
	}
	threshold := 15.0
	for _, c := range s.Competitors {
		eq := EffectiveRivalQuality(s, c, b)
		if c.Name == s.Campaign.Primary.Company && eq[model.DimSafety]*0.9 > threshold {
			threshold = eq[model.DimSafety] * 0.9
		}
	}
	return m.Quality[model.DimSafety] >= threshold
}

func playerSegmentShare(s model.GameState, b balance.Config, seg model.Segment) float64 {
	w := b.SegmentWeights[seg]
	ce := campaignEffects(s, b)
	if seg == model.SegEnterprise {
		w[model.DimSafety] *= ce.SafetyAppealMult
	}
	_, playerAppeal, found := bestRouteModel(s, b, seg)
	if !found {
		return 0
	}
	total := playerAppeal
	for _, c := range s.Competitors {
		total += appealOf(EffectiveRivalQuality(s, c, b), w)
	}
	if total <= 0 {
		return 0
	}
	return playerAppeal / total
}

func campaignInferenceUtil(s model.GameState, b balance.Config) float64 {
	ce := campaignEffects(s, b)
	load := 0.0
	for _, m := range s.Models {
		if m.Online {
			load += m.Users * b.InferenceLoadPerUser * ce.InferenceLoadMult
		}
	}
	cap := EffectiveInference(s, b)
	if cap <= 0 {
		if load <= 0 {
			return 0
		}
		return math.Inf(1)
	}
	return load / cap
}

func campaignCapacityOK(s model.GameState, b balance.Config, maxUtil float64) bool {
	util := campaignInferenceUtil(s, b)
	if math.IsInf(util, 1) {
		return false
	}
	return util <= maxUtil
}

func hasOnlineModelInSegment(s model.GameState, seg model.Segment) bool {
	for _, m := range s.Models {
		if m.Online && m.Segment == seg {
			return true
		}
	}
	return false
}

func campaignPriceOK(s model.GameState, b balance.Config) bool {
	seg := doctrineSegment(s.Campaign.Doctrine)
	m, _, ok := bestRouteModel(s, b, seg)
	if !ok {
		return false
	}
	ref := EffectiveRefPrice(s, seg, b)
	switch s.Campaign.Doctrine {
	case model.DoctrineEnterprise:
		return m.Price >= ref
	case model.DoctrineDeveloper:
		if s.Campaign.Stage == model.CampaignStageShowdown || s.Campaign.Stage == model.CampaignStageWon {
			return m.Price <= ref*0.9
		}
		return m.Price <= ref
	default:
		return true
	}
}

func capacityLimitForStage(s model.GameState) float64 {
	switch s.Campaign.Stage {
	case model.CampaignStageExpand:
		if s.Campaign.Doctrine == model.DoctrineConsumer {
			return 0.90
		}
		return 1.0
	case model.CampaignStageShowdown, model.CampaignStageWon:
		switch s.Campaign.Doctrine {
		case model.DoctrineConsumer:
			return 1.0
		case model.DoctrineEnterprise, model.DoctrineDeveloper:
			return 0.80
		}
	}
	return 1.0
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func minFloat(vals ...float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func capacityProgress(s model.GameState, b balance.Config, maxUtil float64) float64 {
	util := campaignInferenceUtil(s, b)
	if math.IsInf(util, 1) {
		return 0
	}
	if util <= maxUtil {
		return 1
	}
	if util <= 0 {
		return 1
	}
	return clamp01(maxUtil / util)
}

func rankProgress(rank, target int) float64 {
	if rank <= 0 {
		return 0
	}
	if rank <= target {
		return 1
	}
	return clamp01(float64(target) / float64(rank))
}

func boolProgress(ok bool) float64 {
	if ok {
		return 1
	}
	return 0
}

func campaignProgress(s model.GameState, b balance.Config, status CampaignStatusView) float64 {
	if s.Campaign.Victory != model.DoctrineNone {
		return 1
	}
	switch s.Campaign.Stage {
	case model.CampaignStageEstablish:
		online := boolProgress(hasOnlineModelInSegment(s, doctrineSegment(s.Campaign.Doctrine)))
		share := clamp01(status.Share / b.Campaign.EstablishShare)
		return minFloat(online, share)
	case model.CampaignStageExpand:
		switch s.Campaign.Doctrine {
		case model.DoctrineConsumer:
			return minFloat(
				clamp01(status.Share/b.Campaign.ConsumerExpandShare),
				capacityProgress(s, b, 0.90),
			)
		case model.DoctrineEnterprise:
			return minFloat(
				boolProgress(enterpriseSafetyOK(s, b)),
				clamp01(status.Share/b.Campaign.EnterpriseExpandShare),
				boolProgress(status.PriceOK),
			)
		case model.DoctrineDeveloper:
			return minFloat(
				rankProgress(status.QualityRank, 3),
				clamp01(status.Share/b.Campaign.DeveloperExpandShare),
				boolProgress(status.PriceOK),
			)
		}
	case model.CampaignStageShowdown:
		switch s.Campaign.Doctrine {
		case model.DoctrineConsumer:
			return minFloat(
				rankProgress(status.QualityRank, 1),
				clamp01(status.Share/b.Campaign.ConsumerWinShare),
				capacityProgress(s, b, 1.0),
			)
		case model.DoctrineEnterprise:
			return minFloat(
				boolProgress(enterpriseSafetyOK(s, b)),
				clamp01(status.Share/b.Campaign.EnterpriseWinShare),
				boolProgress(status.PriceOK),
				boolProgress(totalRolePower(s, b)[model.RoleOps] > 0),
				capacityProgress(s, b, 0.80),
			)
		case model.DoctrineDeveloper:
			return minFloat(
				rankProgress(status.QualityRank, 1),
				clamp01(status.Share/b.Campaign.DeveloperWinShare),
				boolProgress(status.PriceOK),
				boolProgress(status.CashflowOK),
				capacityProgress(s, b, 0.80),
			)
		}
	}
	return 0
}
