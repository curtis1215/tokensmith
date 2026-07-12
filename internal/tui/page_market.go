package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

func renderMarket(m Model) string {
	s := m.state
	cw := m.contentWidth()
	colW := cw
	if cw >= minDashWidth {
		colW = (cw - 2) / 2
	}

	var cards []string
	segs := []model.Segment{model.SegConsumer, model.SegEnterprise, model.SegDeveloper}
	for _, seg := range segs {
		rank, field := sim.MarketRank(s, m.cfg, seg)

		// Users, rank, scale
		headerInfo := fmt.Sprintf("你的用戶: %s  ·  排名: #%d / %d%s  ·  市場規模: %s",
			human(segmentUsers(s, seg)), rank, field, rankArrow(m.prevRank[seg], rank), marketSizeLabel(m.cfg, seg))

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
			line := fmt.Sprintf("%s %s%s %s %.0f%%", star, name, namePadding, Bar(share, 10), share*100)
			if bRow.You {
				line = youRowStyle(line)
			}
			shareLines = append(shareLines, line)
		}

		cardBody := VStack(
			headerInfo,
			"",
			VStack(shareLines...),
		)
		cards = append(cards, CardIn(CardDefault, colW, segmentName(seg)+"市場", cardBody))
	}

	// Global frontier / era / band (ranking math unchanged).
	gf := sim.GlobalFrontier(s, m.cfg)
	era := sim.CurrentRivalEra(s, m.cfg)
	frontierHdr := fmt.Sprintf("全球前沿 能力%.0f · 時代 %s · 對手帶 85%%–115%%",
		gf[model.DimCapability], eraTitle(era))
	var rivalLines []string
	rivalLines = append(rivalLines, styleMuted.Render(frontierHdr))
	rivalLines = append(rivalLines, styleMuted.Render("品質相對前沿；排名仍依訴求計算"))
	for i, c := range s.Competitors {
		rv := sim.RivalFrontierView(s, i, m.cfg)
		capVal := rv.AbsoluteQuality[model.DimCapability]
		// Bar relative to global frontier (not a hard 100 scale).
		capFrac := 0.0
		if rv.GlobalFrontier[model.DimCapability] > 0 {
			capFrac = capVal / rv.GlobalFrontier[model.DimCapability]
			// Map 0.85–1.15 band roughly onto bar; clamp display.
			capFrac = (capFrac - 0.85) / 0.30
			if capFrac < 0 {
				capFrac = 0
			}
			if capFrac > 1 {
				capFrac = 1
			}
		}
		level := sim.ThreatLevel(s, m.cfg, model.SegConsumer, c)
		label := threatLabel(level)
		leader := ""
		if rv.IsLeader {
			leader = styleGold.Render("★領袖 ")
		}
		delta := rv.FrontierDeltaPct[model.DimCapability] * 100
		deltaStr := fmt.Sprintf("%+.0f%%", delta)
		spec := topSkillDim(c)
		mom := ""
		if rv.MomentumCycles > 0 {
			mom = fmt.Sprintf(" · 動能%d週期", rv.MomentumCycles)
		}
		rivalLines = append(rivalLines, fmt.Sprintf("%s%-8s 能力 %s %.0f (%s) · 專長 %s · 威脅 %s%s",
			leader, Truncate(c.Name, 8), Bar(capFrac, 8), capVal, deltaStr, spec, label, mom))
	}
	// Active campaign market-effect durations.
	if effects := activeMarketEffectLines(s); len(effects) > 0 {
		rivalLines = append(rivalLines, "")
		rivalLines = append(rivalLines, styleMuted.Render("進行中市況修正："))
		rivalLines = append(rivalLines, effects...)
	}
	rivalsCard := CardIn(CardThreat, colW, "對手檔案", VStack(rivalLines...))

	leftColumn := VStack(cards...)
	rightColumn := rivalsCard

	return ResponsiveRow(cw, 2, leftColumn, rightColumn)
}

// activeMarketEffectLines lists campaign Active modifiers still ticking.
func activeMarketEffectLines(s model.GameState) []string {
	var out []string
	for _, mod := range s.Campaign.Active {
		if mod.CyclesRemaining <= 0 {
			continue
		}
		// Summarize non-neutral ref-price effects.
		parts := []string{}
		for seg := 0; seg < model.NumSegments; seg++ {
			mult := mod.Effects.RefPriceMult[seg]
			if mult != 0 && mult != 1 {
				parts = append(parts, fmt.Sprintf("%s價×%.2f", segmentName(model.Segment(seg)), mult))
			}
		}
		if len(parts) == 0 {
			parts = append(parts, mod.ID)
		}
		out = append(out, fmt.Sprintf("· %s · 剩餘 %d 週期", strings.Join(parts, " "), mod.CyclesRemaining))
	}
	return out
}

// rankArrow shows rank movement since the previous snapshot (1-based ranks).
func rankArrow(prev, cur int) string {
	if prev == 0 || prev == cur {
		return ""
	}
	if cur < prev {
		return styleGain.Render(fmt.Sprintf(" ↑%d", prev-cur))
	}
	return styleLoss.Render(fmt.Sprintf(" ↓%d", cur-prev))
}

// youRowStyle inverts the player's row in share leaderboards.
func youRowStyle(line string) string {
	return lipgloss.NewStyle().Bold(true).Foreground(colorInk).Background(colorCyan).Render(line)
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
