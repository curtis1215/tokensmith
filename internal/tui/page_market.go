package tui

import (
	"fmt"
	"strings"

	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

func renderMarket(m Model) string {
	s := m.state
	var b strings.Builder
	b.WriteString("三區隔市場\n")
	segs := []model.Segment{model.SegConsumer, model.SegEnterprise, model.SegDeveloper}
	for _, seg := range segs {
		rank, field := sim.MarketRank(s, m.cfg, seg)
		tam := m.cfg.SegmentTargetScale[seg]
		b.WriteString(fmt.Sprintf("  %-6s TAM %s · 你排名 #%d / %d\n",
			segmentName(seg), human(tam), rank, field))
	}

	b.WriteString("\n對手檔案（能力 / 專長維度）\n")
	for _, c := range s.Competitors {
		b.WriteString(fmt.Sprintf("  %-10s 能力 %.0f · 專長 %s\n",
			c.Name, c.Quality[model.DimCapability], topSkillDim(c)))
	}
	b.WriteString(helpStyle.Render("\n[Tab]切頁"))
	return b.String()
}

// topSkillDim names the quality dimension a competitor is strongest in.
func topSkillDim(c model.Competitor) string {
	best, bestDim := -1.0, 0
	for d := 0; d < model.NumQualityDims; d++ {
		if c.Skill[d] > best {
			best = c.Skill[d]
			bestDim = d
		}
	}
	return dimNames[bestDim]
}
