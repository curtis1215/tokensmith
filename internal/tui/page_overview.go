package tui

import (
	"fmt"
	"strings"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

func renderOverview(m Model) string {
	cw := m.contentWidth()
	rows := []string{
		renderHQ(m, cw),
		Grid(cw, 2,
			func(w int) string { return renderCampaignStatusCard(m, w) },
			func(w int) string { return renderRivalRoadmapCard(m, w) },
			func(w int) string { return companyCard(m, w) },
			func(w int) string { return trainCard(m, w) },
			func(w int) string { return shareCard(m, w) },
			func(w int) string { return powerMilestoneCard(m, w) },
		),
		renderBoardReportCard(m, cw),
		renderEventsCard(m, cw),
	}
	if warns := pressures(m); len(warns) > 0 {
		rows = append(rows, CardIn(CardThreat, cw, "注意", VStack(warns...)))
	}
	return VStack(rows...)
}

func companyCard(m Model, w int) string {
	s := m.state
	rank, field := sim.MarketRank(s, m.cfg, model.SegConsumer)
	val := sim.Valuation(s, m.cfg)
	totalUsers := sim.TotalUsers(s)
	if m.dispReady {
		val = m.disp.Valuation
		totalUsers = m.disp.TotalUsers
	}
	lines := []string{
		KV("估值", "$"+human(val)),
		KV("總用戶", human(totalUsers)),
		KV("排名", fmt.Sprintf("#%d / %d", rank, field)),
		KV("月營收", "$"+human(sim.MonthlyRevenue(s))),
	}
	if tr := m.sparkValuation.Render(18); tr != "" {
		lines = append(lines, styleCyan.Render("趨勢 ")+stylePurple.Render(tr))
	}
	body := VStack(lines...)
	return CardIn(CardDefault, w, "公司", body)
}

func trainCard(m Model, w int) string {
	s := m.state
	var lines []string
	// Model training progress (catalog work total for the bar).
	if s.HasTraining {
		total := 0.0
		if spec, err := balance.Generation(s.Training.Gen); err == nil {
			total = spec.TrainWork
		}
		done := 1.0
		if total > 0 {
			done = 1.0 - s.Training.WorkRemaining/total
		}
		if done < 0 {
			done = 0
		}
		if done > 1 {
			done = 1
		}
		lines = append(lines, fmt.Sprintf("訓練 Gen%d %s %.0f%%", s.Training.Gen, Bar(done, 10), done*100))
		lines = append(lines, KV("區隔", segmentName(s.Training.Segment)))
	} else {
		drafts := 0
		for _, md := range s.Models {
			if sim.IsDraft(md) {
				drafts++
			}
		}
		if drafts > 0 {
			lines = append(lines, "訓練 無進行中")
			lines = append(lines, styleWarn.Render(fmt.Sprintf("待發佈 %d 個（模型頁 p）", drafts)))
		} else {
			lines = append(lines, "訓練 無進行中")
			lines = append(lines, styleMuted.Render("(模型頁 t 開訓)"))
		}
	}
	// Frontier research from pure sim view (no TUI math duplication).
	lines = append(lines, renderFrontierProgressLines(m)...)
	return CardIn(CardDefault, w, "訓練 / 前沿", VStack(lines...))
}

// renderFrontierProgressLines formats sim.FrontierProgressView for cards.
func renderFrontierProgressLines(m Model) []string {
	v := sim.FrontierProgressView(m.state, m.cfg)
	if !v.Active {
		return []string{styleMuted.Render("前沿 無進行中（科技頁啟動）")}
	}
	lines := []string{
		fmt.Sprintf("前沿 Gen%d %s %.0f%%", v.TargetGen, Bar(v.WorkFraction, 10), v.WorkFraction*100),
		fmt.Sprintf("分配 前沿%d%% / 訓練%d%%", v.AllocationPct, v.ModelAllocationPct),
		fmt.Sprintf("算力 有效%.0f → 折合%.0f（建議%.0f）",
			v.AllocatedCompute, v.DiminishedCompute, v.RecommendedCompute),
	}
	if v.UnavailableReason != "" {
		lines = append(lines, styleWarn.Render("停滯 · "+frontierStallCopy(v.UnavailableReason)))
	} else if v.ETASec > 0 {
		lines = append(lines, KV("ETA", formatETASec(v.ETASec)))
	}
	// R&D progress snapshot (secondary).
	lines = append(lines, fmt.Sprintf("R&D 進度 %.0f%%", v.RnDFraction*100))
	return lines
}

func frontierStallCopy(reason string) string {
	switch reason {
	case "no-rnd":
		return "R&D 不足"
	case "no-compute":
		return "無訓練算力"
	case "paused":
		return "分配 0%（已暫停）"
	default:
		return reason
	}
}

func formatETASec(sec float64) string {
	if sec <= 0 {
		return "—"
	}
	if sec >= 86400 {
		return fmt.Sprintf("%.1f 天", sec/86400)
	}
	if sec >= 3600 {
		return fmt.Sprintf("%.1f 時", sec/3600)
	}
	return fmt.Sprintf("%.0f 秒", sec)
}

func shareCard(m Model, w int) string {
	s := m.state
	var shareLines []string
	bars := sim.SegmentShareBars(s, m.cfg, model.SegConsumer)
	limit := 5
	if len(bars) < limit {
		limit = len(bars)
	}
	for i := 0; i < limit; i++ {
		bRow := bars[i]
		share := bRow.Share
		if m.dispReady && i < len(m.disp.ConsumerShares) {
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
	return CardIn(CardDefault, w, "市佔 (消費者)", VStack(shareLines...))
}

func powerMilestoneCard(m Model, w int) string {
	s := m.state
	trainUtil := 0.0
	if s.HasTraining {
		trainUtil = 1.0
	}
	infUtil := 0.0
	if cap := sim.EffectiveInference(s, m.cfg); cap > 0 {
		infUtil = s.Compute.InferenceLoad / cap
	}
	if m.dispReady {
		trainUtil, infUtil = m.disp.TrainUtil, m.disp.InfUtil
	}
	infBar := fmt.Sprintf("推理 %s %.0f%%", LoadBar(infUtil, 10), infUtil*100)
	milestoneStr := ""
	if target, prog, ok := sim.NextMilestone(s, m.cfg); ok {
		milestoneStr = fmt.Sprintf("里程碑 $%s %s %.0f%%", human(target), GoldBar(prog, 10), prog*100)
	} else {
		milestoneStr = styleGold.Render("里程碑 全部達成 ✓")
	}
	body := VStack(
		fmt.Sprintf("訓練 %s %.0f%%", LoadBar(trainUtil, 10), trainUtil*100),
		infBar,
		milestoneStr,
	)
	return CardIn(CardDefault, w, "里程碑 & 算力", body)
}

// renderEventsCard is the 產業動態 card: pending decisions first (highlighted
// with their remaining decision window), then recent history, max 4 lines.
func renderEventsCard(m Model, w int) string {
	ev := m.state.Events
	var lines []string
	for _, p := range ev.Pending {
		meta := eventLabel(p.EventID)
		days := (p.Deadline - m.state.GameTime) / 86400
		if days < 0 {
			days = 0
		}
		lines = append(lines, styleWarn.Render(
			fmt.Sprintf("⏳ %s — [e]決策（剩 %.0f 天）", meta.Name, days)))
	}
	for i := len(ev.Log) - 1; i >= 0 && len(lines) < 4; i-- {
		rec := ev.Log[i]
		meta := eventLabel(rec.EventID)
		result := ""
		if meta.Choices[0] != "" && rec.Choice >= 0 && rec.Choice < 2 {
			result = " · " + meta.Choices[rec.Choice]
		}
		if rec.Auto {
			result += "（自動）"
		}
		day := int(rec.At / 86400)
		lines = append(lines, fmt.Sprintf("· D%d %s%s", day, meta.Name, result))
	}
	if len(lines) == 0 {
		lines = append(lines, styleMuted.Render("風平浪靜——尚無產業事件"))
	}
	return CardIn(CardDefault, w, "產業動態", VStack(lines...))
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
