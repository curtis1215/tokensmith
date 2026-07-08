package tui

import (
	"fmt"
	"strings"

	"tokensmith/internal/model"
)

func renderTeam(m Model) string {
	s := m.state
	var b strings.Builder
	b.WriteString("四職能\n")
	b.WriteString(fmt.Sprintf("  研究員  T1 %d · T2 %d · T3 %d\n",
		s.Research.Researchers[model.Tier1], s.Research.Researchers[model.Tier2], s.Research.Researchers[model.Tier3]))
	b.WriteString(fmt.Sprintf("  工程    %d 人（算力效率 +%.0f%%）\n",
		s.Engineers, float64(s.Engineers)*m.cfg.EngineerInfraBonus*100))
	b.WriteString(fmt.Sprintf("  營運    %d 人（降流失）\n", s.Ops))
	b.WriteString(fmt.Sprintf("  行銷    %d 人（用戶成長 +%.0f%%）\n",
		s.Marketing, float64(s.Marketing)*m.cfg.MarketingBonus*100))

	b.WriteString("\n明星員工\n")
	for _, st := range m.cfg.Stars {
		status := fmt.Sprintf("簽約 $%s · 薪 $%.3f/s", human(st.SigningCost), st.SalaryPerSec)
		if starHired(s, st.ID) {
			status = "✓ 已簽"
		}
		b.WriteString(fmt.Sprintf("  %-14s %s\n", st.Name, status))
	}

	b.WriteString(helpStyle.Render("\n[h]雇研究員 [e]雇工程 [o]雇營運 [k]雇行銷 [s]簽明星 [Tab]切頁"))
	return b.String()
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
