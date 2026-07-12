package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"tokensmith/internal/balance"
	"tokensmith/internal/dailyusage"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

func renderOverview(m Model) string {
	cw := m.contentWidth()
	gap := 2
	rows := []string{}

	// Row1: HQ | 公司 — page width decides full ASCII vs compact (not column width).
	hqCompact := cw < 100
	if cw < minDashWidth {
		rows = append(rows,
			CardInFrom(hqContent(m, cw, true)),
			CardInFrom(companyContent(m, cw)),
		)
	} else {
		colW := (cw - gap) / 2
		rows = append(rows, HRowEqualCards(gap,
			hqContent(m, colW, hqCompact),
			companyContent(m, colW),
		))
	}

	// Row2: thin daily harvest full width
	rows = append(rows, renderDailyUsageCard(m, cw))

	// Row3: 訓練 | 市佔 | 里程碑
	rows = append(rows, renderOverviewStatusRow(m, cw, gap))

	// Pending strip (not a campaign card)
	if strip := overviewPendingStrip(m); strip != "" {
		rows = append(rows, strip)
	}

	// Operational pressures only (campaign prompts live on 戰情室).
	if warns := operationalPressures(m); len(warns) > 0 {
		rows = append(rows, CardIn(CardThreat, cw, "注意", VStack(warns...)))
	}
	return VStack(rows...)
}

func renderOverviewStatusRow(m Model, cw, gap int) string {
	if cw < minDashWidth {
		return VStack(
			CardInFrom(trainContent(m, cw)),
			CardInFrom(shareContent(m, cw)),
			CardInFrom(powerMilestoneContent(m, cw)),
		)
	}
	if cw < 100 {
		// 2+1
		colW := (cw - gap) / 2
		top := HRowEqualCards(gap, trainContent(m, colW), shareContent(m, colW))
		return VStack(top, CardInFrom(powerMilestoneContent(m, cw)))
	}
	return GridNCards(cw, gap, 3,
		func(w int) cardContent { return trainContent(m, w) },
		func(w int) cardContent { return shareContent(m, w) },
		func(w int) cardContent { return powerMilestoneContent(m, w) },
	)
}

// CardInFrom renders a cardContent.
func CardInFrom(c cardContent) string {
	return CardIn(c.kind, c.w, c.title, c.body)
}

func overviewPendingStrip(m Model) string {
	n := len(m.state.Events.Pending)
	if n == 0 {
		return ""
	}
	return styleWarn.Render(fmt.Sprintf("⚠ 產業待決策 %d · [2]戰情室  [e]決策", n))
}

// dailySourceOrder is the fixed display order for the four accounting identities.
var dailySourceOrder = []struct {
	key, wideLabel, narrowLabel string
	estimated                   bool
}{
	{"claude-code", "Claude Code", "Claude", false},
	{"codex", "Codex", "Codex", false},
	{"grok", "Grok（估算）", "Grok", true},
	{"opencode", "OpenCode", "OpenCode", false},
}

// renderDailyUsageCard shows today's per-source raw token harvest (thin overview).
// Both widths: compact "Claude 138K · Codex 97K …" segments, no per-source In/Out.
// Wide path adds a 合計 line. All four sources always appear; missing keys → zero.
// Midnight is handled by Model.dailyDay selecting the current local date bucket.
func renderDailyUsageCard(m Model, w int) string {
	day := m.dailyDay
	if day == "" {
		day = dailyusage.DayKey(time.Now())
	}
	var bucket map[string]dailyusage.SourceUsage
	if m.dailyDoc.Days != nil {
		bucket = m.dailyDoc.Days[day]
	}

	// Title: 今日 Token 收成 · MM/DD
	titleDay := day
	if t, err := time.Parse("2006-01-02", day); err == nil {
		titleDay = t.Format("01/02")
	}
	title := "今日 Token 收成 · " + titleDay

	var segs []string
	grand := 0
	for _, src := range dailySourceOrder {
		u := bucket[src.key]
		total := u.In + u.Out
		grand += total
		// Narrow labels keep the card thin; Grok is "Grok" (not fabricated Out).
		segs = append(segs, fmt.Sprintf("%s %s", src.narrowLabel, formatTokenCount(total)))
	}
	bodyW := w - 4 // account for card padding roughly
	if bodyW < 8 {
		bodyW = 8
	}
	// All widths: four source totals + 合計 (spec §4.2). Compact may wrap segments.
	body := wrapCompactSegments(segs, bodyW)
	body = VStack(body, fmt.Sprintf("合計 %s", formatTokenCount(grand)))
	return CardIn(CardDefault, w, title, body)
}

// formatTokenCount renders raw token counts compactly with capital K/M.
func formatTokenCount(n int) string {
	if n < 0 {
		n = 0
	}
	switch {
	case n >= 1_000_000:
		// One decimal when not an exact million.
		if n%1_000_000 == 0 {
			return fmt.Sprintf("%dM", n/1_000_000)
		}
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%dK", n/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// wrapCompactSegments joins segments with " · " and wraps so each visual line
// stays within maxW display cells (ANSI-aware via lipgloss.Width).
func wrapCompactSegments(segs []string, maxW int) string {
	if maxW < 8 {
		maxW = 8
	}
	var lines []string
	var cur string
	for _, s := range segs {
		candidate := s
		if cur != "" {
			candidate = cur + " · " + s
		}
		if lipgloss.Width(candidate) <= maxW {
			cur = candidate
			continue
		}
		if cur != "" {
			lines = append(lines, cur)
		}
		cur = s
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return VStack(lines...)
}

func companyCard(m Model, w int) string {
	return CardInFrom(companyContent(m, w))
}

func companyContent(m Model, w int) cardContent {
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
	return cardContent{kind: CardDefault, w: w, title: "公司", body: VStack(lines...)}
}

func trainCard(m Model, w int) string {
	return CardInFrom(trainContent(m, w))
}

func trainContent(m Model, w int) cardContent {
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
	return cardContent{kind: CardDefault, w: w, title: "訓練 / 前沿", body: VStack(lines...)}
}

// renderFrontierProgressLines formats sim.FrontierProgressView for the overview
// train card: progress + ETA/stall only (≤2 detail lines when active).
func renderFrontierProgressLines(m Model) []string {
	v := sim.FrontierProgressView(m.state, m.cfg)
	if !v.Active {
		return []string{styleMuted.Render("前沿 無進行中（科技頁啟動）")}
	}
	lines := []string{
		fmt.Sprintf("前沿 Gen%d %s %.0f%%", v.TargetGen, Bar(v.WorkFraction, 10), v.WorkFraction*100),
	}
	if v.UnavailableReason != "" {
		lines = append(lines, styleWarn.Render("停滯 · "+frontierStallCopy(v.UnavailableReason)))
	} else if v.ETASec > 0 {
		lines = append(lines, KV("ETA", formatETASec(v.ETASec)))
	}
	return lines // max 2 lines when active
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
	return CardInFrom(shareContent(m, w))
}

// pickShareRows returns at most limit bars, always including the player (You).
func pickShareRows(bars []sim.ShareRow, limit int) []sim.ShareRow {
	if limit < 1 || len(bars) == 0 {
		return nil
	}
	if len(bars) <= limit {
		return bars
	}
	var you *sim.ShareRow
	for i := range bars {
		if bars[i].You {
			row := bars[i]
			you = &row
			break
		}
	}
	out := make([]sim.ShareRow, 0, limit)
	othersBudget := limit
	if you != nil {
		othersBudget = limit - 1
	}
	takenOthers := 0
	youAdded := false
	for _, b := range bars {
		if b.You {
			if !youAdded && you != nil {
				out = append(out, *you)
				youAdded = true
			}
			continue
		}
		if takenOthers >= othersBudget {
			continue
		}
		out = append(out, b)
		takenOthers++
	}
	if !youAdded && you != nil {
		out = append(out, *you)
	}
	return out
}

func shareContent(m Model, w int) cardContent {
	s := m.state
	full := sim.SegmentShareBars(s, m.cfg, model.SegConsumer)
	bars := pickShareRows(full, 4)
	var shareLines []string
	for _, bRow := range bars {
		share := bRow.Share
		if m.dispReady {
			for j, fb := range full {
				if fb.Name == bRow.Name && fb.You == bRow.You && j < len(m.disp.ConsumerShares) {
					share = m.disp.ConsumerShares[j]
					break
				}
			}
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
	return cardContent{kind: CardDefault, w: w, title: "市佔 (消費者)", body: VStack(shareLines...)}
}

func powerMilestoneCard(m Model, w int) string {
	return CardInFrom(powerMilestoneContent(m, w))
}

func powerMilestoneContent(m Model, w int) cardContent {
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
	return cardContent{kind: CardDefault, w: w, title: "里程碑 & 算力", body: body}
}

// renderEventsCard is the 產業動態 card with the overview default of 4 body lines.
func renderEventsCard(m Model, w int) string {
	return renderEventsCardMax(m, w, 4)
}

// renderEventsCardMax is the 產業動態 card: pending decisions first (highlighted
// with their remaining decision window), then recent history, capped at maxLines.
func renderEventsCardMax(m Model, w int, maxLines int) string {
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
	for i := len(ev.Log) - 1; i >= 0 && len(lines) < maxLines; i-- {
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
