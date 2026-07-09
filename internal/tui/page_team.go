package tui

import (
	"fmt"
	"strings"

	"tokensmith/internal/model"
)

func renderTeam(m Model) string {
	s := m.state

	// Calculate salary sum
	salarySum := 0.0
	for t := 1; t < model.NumTiers; t++ {
		salarySum += float64(s.Research.Researchers[t]) * m.cfg.ResearcherSalaryPerSec[t]
	}
	salarySum += float64(s.Engineers) * m.cfg.EngineerSalaryPerSec
	salarySum += float64(s.Ops) * m.cfg.OpsSalaryPerSec
	salarySum += float64(s.Marketing) * m.cfg.MarketingSalaryPerSec
	for _, hID := range s.HiredStars {
		for _, st := range m.cfg.Stars {
			if st.ID == hID {
				salarySum += st.SalaryPerSec
			}
		}
	}

	// 1. Roles Card
	rolesBody := VStack(
		KV("研究員", fmt.Sprintf("T1 %d · T2 %d · T3 %d",
			s.Research.Researchers[model.Tier1], s.Research.Researchers[model.Tier2], s.Research.Researchers[model.Tier3])),
		KV("工程", fmt.Sprintf("%d 人 (算力效率 +%.0f%%)",
			s.Engineers, float64(s.Engineers)*m.cfg.EngineerInfraBonus*100)),
		KV("營運", fmt.Sprintf("%d 人 (降流失)", s.Ops)),
		KV("行銷", fmt.Sprintf("%d 人 (用戶成長 +%.0f%%)",
			s.Marketing, float64(s.Marketing)*m.cfg.MarketingBonus*100)),
		"",
		KV("薪資合計", fmt.Sprintf("$%.3f/s", salarySum)),
	)
	rolesCard := Card("團隊四職能", rolesBody)

	// 2. Stars Card — truncate long status/blurb lines to fit viewport content width
	inner := m.cardInnerWidth()
	var starLines []string
	for _, st := range m.cfg.Stars {
		status := ""
		if starHired(s, st.ID) {
			status = "✓ 已簽"
		} else {
			status = fmt.Sprintf("簽約 $%s · 薪 $%.3f/s", human(st.SigningCost), st.SalaryPerSec)
		}

		blurb := starBlurb(st)
		line := fmt.Sprintf("%-15s %-25s (%s)", st.Name, status, blurb)
		starLines = append(starLines, TruncateWidth(line, inner))
	}
	starsCard := Card("明星員工", VStack(starLines...))

	return ResponsiveRow(m.contentWidth(), 2, rolesCard, starsCard)
}

func starBlurb(st model.Star) string {
	var parts []string
	e := st.Effects
	if e.RnDPerSec != 0 {
		parts = append(parts, fmt.Sprintf("R&D+%.0f/s", e.RnDPerSec))
	}

	dimNames := [4]string{"能力", "成本", "安全", "速度"}
	for d := 0; d < 4; d++ {
		if e.QualityMult[d] > 0 && e.QualityMult[d] != 1.0 {
			parts = append(parts, fmt.Sprintf("%s×%.2f", dimNames[d], e.QualityMult[d]))
		}
	}

	if e.UserGrowthMult > 0 && e.UserGrowthMult != 1.0 {
		parts = append(parts, fmt.Sprintf("用戶成長×%.2f", e.UserGrowthMult))
	}

	if e.InfraMult > 0 && e.InfraMult != 1.0 {
		parts = append(parts, fmt.Sprintf("算力效率×%.2f", e.InfraMult))
	}

	if len(parts) == 0 {
		return "無特殊加成"
	}
	return strings.Join(parts, " · ")
}

// starHired reports whether the star id is already on the roster.
func starHired(s model.GameState, id string) bool {
	for _, h := range s.HiredStars {
		if h == id {
			return true
		}
	}
	return false
}

// firstUnhiredStar returns the id of the first star not yet signed, or "".
func firstUnhiredStar(m Model) string {
	for _, st := range m.cfg.Stars {
		if !starHired(m.state, st.ID) {
			return st.ID
		}
	}
	return ""
}
