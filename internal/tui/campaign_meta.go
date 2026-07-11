package tui

import (
	"fmt"
	"strings"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

// labelOrID returns map[id] when present; otherwise the original id (unknown fallback).
func labelOrID(m map[string]string, id string) string {
	if s, ok := m[id]; ok && s != "" {
		return s
	}
	return id
}

var doctrineLabels = map[model.Doctrine]string{
	model.DoctrineConsumer:   "消費者霸主",
	model.DoctrineEnterprise: "企業信任",
	model.DoctrineDeveloper:  "開發者生態",
}

var campaignStageLabels = map[model.CampaignStage]string{
	model.CampaignStageEstablish: "立足",
	model.CampaignStageExpand:    "擴張",
	model.CampaignStageShowdown:  "決勝",
	model.CampaignStageWon:       "已勝利",
}

var perkLabels = map[string]string{
	"consumer-premium":       "消費高階品牌",
	"consumer-mass":          "消費大眾滲透",
	"consumer-resilience":    "消費品牌韌性",
	"consumer-scale":         "消費規模擴張",
	"enterprise-compliance":  "企業合規優勢",
	"enterprise-premium":     "企業高價定位",
	"enterprise-reliability": "企業服務可靠",
	"enterprise-sales":       "企業銷售衝刺",
	"developer-open":         "開發者開放生態",
	"developer-api":          "開發者 API 平台",
	"developer-efficient":    "開發者推理效率",
	"developer-usage":        "開發者用量營收",
}

var rivalActionLabels = map[string]string{
	"openai-flagship":            "OpenAI 消費旗艦",
	"openai-platform":            "OpenAI 平台攻勢",
	"anthropic-trust":            "Anthropic 企業信任攻勢",
	"anthropic-enterprise-suite": "Anthropic 企業套件",
	"xai-scale":                  "xAI 暴力擴張",
	"xai-compute-rush":           "xAI 算力突進",
	"deepseek-price-war":         "DeepSeek 開源價格戰",
	"deepseek-distill":           "DeepSeek 蒸餾突破",
	"qwen-ecosystem":             "Qwen 開發者生態攻勢",
	"qwen-release-wave":          "Qwen 模型機海",
	"zhipu-enterprise":           "Zhipu 企業市場攻勢",
	"zhipu-contract":             "Zhipu 大單攻勢",
	"gemini-balanced":            "Gemini 全面發表",
	"gemini-multimodal":          "Gemini 多模態發表",
}

var reportKindLabels = map[string]string{
	string(model.ReportDoctrineChosen): "選定戰略",
	string(model.ReportStageAdvanced):  "階段推進",
	string(model.ReportRivalAction):    "宿敵行動",
	string(model.ReportShowdown):       "決勝開始",
	string(model.ReportVictory):        "路線勝利",
	string(model.ReportFinancialRisk):  "財務風險",
}

var directiveLabels = map[string]string{
	string(model.DirectiveRoutePush): "市場衝刺",
	string(model.DirectiveCounter):   "反制宿敵",
	string(model.DirectiveIntel):     "深度情報",
}

func doctrineLabel(d model.Doctrine) string {
	if s, ok := doctrineLabels[d]; ok && s != "" {
		return s
	}
	return string(d)
}

func campaignStageLabel(st model.CampaignStage) string {
	if s, ok := campaignStageLabels[st]; ok && s != "" {
		return s
	}
	return string(st)
}

func perkLabel(id string) string {
	return labelOrID(perkLabels, id)
}

func rivalActionLabel(id string) string {
	return labelOrID(rivalActionLabels, id)
}

func reportKindLabel(k model.CampaignReportKind) string {
	return labelOrID(reportKindLabels, string(k))
}

func directiveLabel(d model.DirectiveKind) string {
	return labelOrID(directiveLabels, string(d))
}

// renderCampaignStatusCard is the CEO war-room status card (doctrine / stage / gate).
func renderCampaignStatusCard(m Model, w int) string {
	status := sim.CampaignStatus(m.state, m.cfg)
	if !status.Active {
		return CardIn(CardAccent, w, "公司戰略", "第一個模型上線後可選公司戰略")
	}
	camp := m.state.Campaign
	var lines []string
	lines = append(lines,
		KV("主要戰略", doctrineLabel(status.Doctrine)),
		KV("階段", campaignStageLabel(status.Stage)),
		KV("董事會週期", fmt.Sprintf("%d", camp.Cycle)),
		fmt.Sprintf("下一目標 %s %.0f%%", Bar(status.Progress, 12), status.Progress*100),
	)
	kind := CardAccent
	if status.Stage == model.CampaignStageShowdown {
		kind = CardThreat
		line := fmt.Sprintf("⚔ 決勝中——已頂住 %d/2 次宿敵攻勢", camp.ShowdownHeld)
		if camp.ShowdownAttempts > 0 {
			line += fmt.Sprintf("（第 %d 次嘗試）", camp.ShowdownAttempts+1)
		}
		st := styleLoss.Bold(true)
		if m.blink {
			st = styleAmber.Bold(true)
		}
		lines = append(lines, st.Render(line))
	}
	if len(camp.Perks) > 0 {
		var names []string
		for _, id := range camp.Perks {
			names = append(names, perkLabel(id))
		}
		lines = append(lines, KV("能力", strings.Join(names, "、")))
	}
	if camp.PerkTierPending > 0 {
		lines = append(lines, styleWarn.Render(fmt.Sprintf("待選能力（第 %d 階）— 按 c", camp.PerkTierPending)))
	}
	if camp.Secondary != model.DoctrineNone {
		lines = append(lines, KV("副戰略", doctrineLabel(camp.Secondary)))
		if camp.SecondaryPerk != "" {
			lines = append(lines, KV("副能力", perkLabel(camp.SecondaryPerk)))
		}
	}
	if camp.Endless {
		// Optional route goals for the two non-primary doctrines.
		for _, d := range []model.Doctrine{model.DoctrineConsumer, model.DoctrineEnterprise, model.DoctrineDeveloper} {
			if d == camp.Doctrine {
				continue
			}
			alt := sim.RouteVictoryStatus(m.state, m.cfg, d)
			lines = append(lines, fmt.Sprintf("可選 %s %s %.0f%%",
				doctrineLabel(d), Bar(alt.Progress, 8), alt.Progress*100))
		}
	}
	if status.Victory || camp.Victory != model.DoctrineNone {
		lines = append(lines, styleAccent.Render("✓ 路線勝利 — 按 P 結算"))
	}
	return CardIn(kind, w, "公司戰略", VStack(lines...))
}

// renderRivalRoadmapCard shows primary + wildcard confirmed/rumored actions.
func renderRivalRoadmapCard(m Model, w int) string {
	var blocks []string
	if primary, ok := sim.CampaignRivalIntel(m.state, m.cfg, true); ok {
		blocks = append(blocks, renderRivalIntelBlock("主要宿敵", primary, m.cfg, m.blink))
	} else {
		blocks = append(blocks, styleMuted.Render("主要宿敵：尚無情報"))
	}
	if wildcard, ok := sim.CampaignRivalIntel(m.state, m.cfg, false); ok {
		blocks = append(blocks, renderRivalIntelBlock("攪局者", wildcard, m.cfg, m.blink))
	} else {
		blocks = append(blocks, styleMuted.Render("攪局者：尚無情報"))
	}
	return CardIn(CardThreat, w, "宿敵路線", VStack(blocks...))
}

func renderRivalIntelBlock(role string, intel sim.RivalIntelView, cfg balance.Config, blink bool) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("%s %s", role, intel.Company))
	if intel.ConfirmedActionID != "" {
		line := fmt.Sprintf("  已確認 %s · %d 週期後",
			rivalActionLabel(intel.ConfirmedActionID), intel.CyclesUntilAction)
		if intel.CyclesUntilAction <= 1 {
			st := styleLoss.Bold(true)
			if blink {
				st = styleAmber.Bold(true)
			}
			line = st.Render(line + " ⚠")
		}
		lines = append(lines, line)
		lines = append(lines, formatActionIntelDetail(intel.ConfirmedActionID, intel.IntelFull, cfg, "  "))
	}
	if intel.RumoredActionID != "" {
		lines = append(lines, fmt.Sprintf("  下一步 %s", rivalActionLabel(intel.RumoredActionID)))
		lines = append(lines, formatActionIntelDetail(intel.RumoredActionID, intel.IntelFull, cfg, "  "))
	} else {
		lines = append(lines, "  下一步 —")
	}
	return VStack(lines...)
}

// formatActionIntelDetail: IntelFull → quality % / price mod / lead cycles;
// otherwise only direction (segment).
func formatActionIntelDetail(actionID string, full bool, cfg balance.Config, indent string) string {
	spec, ok := balance.RivalActionByID(cfg.Campaign, actionID)
	if !ok {
		return indent + styleMuted.Render(actionID)
	}
	if !full {
		return indent + fmt.Sprintf("方向 %s", segmentName(spec.Segment))
	}
	var dims []string
	dimNames := [model.NumQualityDims]string{"能力", "成本", "安全", "速度"}
	for i := 0; i < model.NumQualityDims; i++ {
		if spec.FrontierProgress[i] != 0 {
			dims = append(dims, fmt.Sprintf("%s追趕%.0f%%", dimNames[i], spec.FrontierProgress[i]*100))
		}
	}
	if len(dims) == 0 {
		dims = append(dims, "品質—")
	}
	// Duration of any market-effect modifier this action can apply.
	dur := ""
	if spec.DurationCycles > 0 && spec.RefPriceMult > 0 && spec.RefPriceMult != 1 {
		dur = fmt.Sprintf(" · 市況%d週期", spec.DurationCycles)
	}
	mom := ""
	if spec.MomentumCycles > 0 {
		mom = fmt.Sprintf(" · 動能%d週期", spec.MomentumCycles)
	}
	return indent + fmt.Sprintf("%s · 價格×%.2f · 前置%d週期 · %s%s%s",
		strings.Join(dims, " "),
		spec.RefPriceMult,
		spec.LeadCycles,
		segmentName(spec.Segment),
		dur,
		mom,
	)
}

// renderBoardReportCard shows the latest board report (newest four entries).
func renderBoardReportCard(m Model, w int) string {
	reports := m.state.Campaign.Reports
	if len(reports) == 0 {
		return CardIn(CardDefault, w, "董事會報告", styleMuted.Render("尚無董事會報告"))
	}
	latest := reports[len(reports)-1]
	entries := latest.Entries
	if len(entries) == 0 {
		return CardIn(CardDefault, w, "董事會報告", styleMuted.Render(fmt.Sprintf("第 %d 週期：無事項", latest.Cycle)))
	}
	// Newest four: take from the end.
	start := 0
	if len(entries) > 4 {
		start = len(entries) - 4
	}
	var lines []string
	lines = append(lines, fmt.Sprintf("第 %d 週期", latest.Cycle))
	for _, e := range entries[start:] {
		lines = append(lines, formatReportEntry(e))
	}
	return CardIn(CardDefault, w, "董事會報告", VStack(lines...))
}

func formatReportEntry(e model.CampaignReportEntry) string {
	kind := reportKindLabel(e.Kind)
	subject := e.SubjectID
	// Subject may be a doctrine or stage id — map when known.
	if s := doctrineLabel(model.Doctrine(e.SubjectID)); s != e.SubjectID {
		subject = s
	} else if s := campaignStageLabel(model.CampaignStage(e.SubjectID)); s != e.SubjectID {
		subject = s
	}
	detail := e.DetailID
	if detail != "" {
		if s := rivalActionLabel(detail); s != detail {
			detail = s
		} else if s := perkLabel(detail); s != detail {
			detail = s
		}
		return fmt.Sprintf("· %s %s · %s", kind, subject, detail)
	}
	return fmt.Sprintf("· %s %s", kind, subject)
}
