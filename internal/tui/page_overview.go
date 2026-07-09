package tui

import (
	"fmt"
	"strings"

	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

func renderOverview(m Model) string {
	s := m.state
	rank, field := sim.MarketRank(s, m.cfg, model.SegConsumer)

	// 1. Company card
	companyBody := VStack(
		KV("估值", "$"+human(sim.Valuation(s, m.cfg))),
		KV("總用戶", human(sim.TotalUsers(s))),
		KV("排名", fmt.Sprintf("#%d / %d", rank, field)),
		KV("月營收", "$"+human(sim.MonthlyRevenue(s))),
	)
	companyCard := Card("公司", companyBody)

	// 2. Training / Publish card
	var trainBody string
	if s.HasTraining {
		total := m.cfg.GenTrainWorkGPUSec[s.Training.Gen]
		done := 1.0
		if total > 0 {
			done = 1.0 - s.Training.WorkRemaining/total
		}
		trainBody = fmt.Sprintf("Gen%d %s %.0f%%\n%s", s.Training.Gen, Bar(done, 12), done*100, KV("區隔", segmentName(s.Training.Segment)))
	} else {
		drafts := 0
		for _, md := range s.Models {
			if sim.IsDraft(md) {
				drafts++
			}
		}
		if drafts > 0 {
			trainBody = fmt.Sprintf("無進行中訓練\n%s", styleWarn.Render(fmt.Sprintf("待發佈 %d 個（模型頁 p）", drafts)))
		} else {
			trainBody = "無進行中訓練\n(到模型頁按 t 開訓)"
		}
	}
	trainCard := Card("訓練 / 發佈", trainBody)

	// 3. Share card
	var shareLines []string
	bars := sim.SegmentShareBars(s, m.cfg, model.SegConsumer)
	limit := 5
	if len(bars) < limit {
		limit = len(bars)
	}
	for i := 0; i < limit; i++ {
		bRow := bars[i]
		star := " "
		if bRow.You {
			star = "★"
		}
		name := Truncate(bRow.Name, 10)
		namePadding := strings.Repeat(" ", 10-len([]rune(name)))
		if len([]rune(name)) > 10 {
			namePadding = ""
		}
		shareLines = append(shareLines, fmt.Sprintf("%s %s%s %s %.0f%%", star, name, namePadding, Bar(bRow.Share, 10), bRow.Share*100))
	}
	shareCard := Card("市佔 (消費者)", VStack(shareLines...))

	// 4. Power & Milestone card
	trainUtil := 0.0
	if s.HasTraining {
		trainUtil = 1.0
	}
	infUtil := 0.0
	if cap := sim.EffectiveInference(s, m.cfg); cap > 0 {
		infUtil = s.Compute.InferenceLoad / cap
	}

	infBar := fmt.Sprintf("推理 %s %.0f%%", Bar(infUtil, 10), infUtil*100)
	if infUtil >= 0.9 {
		infBar = styleWarn.Render(infBar)
	}

	milestoneStr := ""
	if target, prog, ok := sim.NextMilestone(s, m.cfg); ok {
		milestoneStr = fmt.Sprintf("里程碑 $%s %s %.0f%%", human(target), Bar(prog, 10), prog*100)
	} else {
		milestoneStr = "里程碑 全部達成 ✓"
	}

	powerMilestoneBody := VStack(
		fmt.Sprintf("訓練 %s %.0f%%", Bar(trainUtil, 10), trainUtil*100),
		infBar,
		milestoneStr,
	)
	powerMilestoneCard := Card("里程碑 & 算力", powerMilestoneBody)

	// Combine into rows
	row1 := ResponsiveRow(m.width, 2, companyCard, trainCard)
	row2 := ResponsiveRow(m.width, 2, shareCard, powerMilestoneCard)

	var rows []string
	rows = append(rows, row1, row2)

	// 5. Pressures (footer lives in the fixed shell)
	if warns := pressures(m); len(warns) > 0 {
		rows = append(rows, Card("注意", styleWarn.Render(VStack(warns...))))
	}

	return VStack(rows...)
}

func segmentName(seg model.Segment) string {
	switch seg {
	case model.SegEnterprise:
		return "企業"
	case model.SegDeveloper:
		return "開發者"
	default:
		return "消費者"
	}
}
