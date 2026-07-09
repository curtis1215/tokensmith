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
	var cards []string
	segs := []model.Segment{model.SegConsumer, model.SegEnterprise, model.SegDeveloper}
	for _, seg := range segs {
		rank, field := sim.MarketRank(s, m.cfg, seg)

		// Users, rank, scale
		headerInfo := fmt.Sprintf("你的用戶: %s  ·  排名: #%d / %d  ·  市場規模: %s",
			human(segmentUsers(s, seg)), rank, field, marketSizeLabel(m.cfg, seg))

		// Bars from SegmentShareBars (consumer top rows use approached display shares)
		bars := sim.SegmentShareBars(s, m.cfg, seg)
		var shareLines []string
		for i := 0; i < len(bars); i++ {
			bRow := bars[i]
			share := bRow.Share
			if seg == model.SegConsumer && m.dispReady && i < len(m.disp.ConsumerShares) {
				share = m.disp.ConsumerShares[i]
			}
			star := " "
			if bRow.You {
				star = "★"
			}
			name := Truncate(bRow.Name, 10)
			namePadding := strings.Repeat(" ", 10-len([]rune(name)))
			if len([]rune(name)) > 10 {
				namePadding = ""
			}
			shareLines = append(shareLines, fmt.Sprintf("%s %s%s %s %.0f%%", star, name, namePadding, Bar(share, 10), share*100))
		}

		cardBody := VStack(
			headerInfo,
			"",
			VStack(shareLines...),
		)
		cards = append(cards, Card(segmentName(seg)+"市場", cardBody))
	}

	var rivalLines []string
	for _, c := range s.Competitors {
		capVal := c.Quality[model.DimCapability]
		capFrac := capVal / 100.0
		if capFrac > 1 {
			capFrac = 1
		}
		level := sim.ThreatLevel(s, m.cfg, model.SegConsumer, c)
		label := threatLabel(level)

		rivalLines = append(rivalLines, fmt.Sprintf("%-10s 能力 %s (%.0f) · 專長 %-4s · 威脅 %s",
			c.Name, Bar(capFrac, 10), capVal, topSkillDim(c), label))
	}
	rivalsCard := Card("對手檔案", VStack(rivalLines...))

	leftColumn := VStack(cards...)
	rightColumn := rivalsCard

	return ResponsiveRow(m.contentWidth(), 2, leftColumn, rightColumn)
}

func threatLabel(level int) string {
	switch level {
	case 2:
		return styleWarn.Render("高")
	case 1:
		return styleAccent.Render("中")
	default:
		return "低"
	}
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
