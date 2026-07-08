package tui

import (
	"fmt"
	"strings"

	"tokensmith/internal/balance"
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
		b.WriteString(fmt.Sprintf("  %-6s 你的用戶 %s · 排名 #%d / %d · 市場規模 %s\n",
			segmentName(seg), human(segmentUsers(s, seg)), rank, field, marketSizeLabel(m.cfg, seg)))
	}

	b.WriteString("\n對手檔案（能力 / 專長維度）\n")
	for _, c := range s.Competitors {
		b.WriteString(fmt.Sprintf("  %-10s 能力 %.0f · 專長 %s\n",
			c.Name, c.Quality[model.DimCapability], topSkillDim(c)))
	}
	b.WriteString(helpStyle.Render("\n[Tab]切頁"))
	return b.String()
}

// segmentUsers sums the player's online-model users in a segment.
func segmentUsers(s model.GameState, seg model.Segment) float64 {
	var u float64
	for _, md := range s.Models {
		if md.Online && md.Segment == seg {
			u += md.Users
		}
	}
	return u
}

// marketSizeLabel describes a segment's relative demand (大/中/小) from its
// target scale — a coefficient in the user-demand formula, not a user cap.
func marketSizeLabel(b balance.Config, seg model.Segment) string {
	bigger := 0
	for s := 0; s < model.NumSegments; s++ {
		if b.SegmentTargetScale[s] > b.SegmentTargetScale[seg] {
			bigger++
		}
	}
	return []string{"大", "中", "小"}[bigger]
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
