package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

func renderOverview(m Model) string {
	s := m.state
	rank, field := sim.MarketRank(s, m.cfg, model.SegConsumer)
	company := boxStyle.Render(fmt.Sprintf(
		"公司\n估值   $%s\n總用戶 %s\n排名   #%d / %d\n月營收 $%s",
		human(sim.Valuation(s, m.cfg)), human(sim.TotalUsers(s)),
		rank, field, human(sim.MonthlyRevenue(s))))

	var training string
	if s.HasTraining {
		total := m.cfg.GenTrainWorkGPUSec[s.Training.Gen]
		done := 1.0
		if total > 0 {
			done = 1 - s.Training.WorkRemaining/total
		}
		training = boxStyle.Render(fmt.Sprintf("進行中訓練\nGen%d  %s %.0f%%\n區隔 %s",
			s.Training.Gen, progressBar(done, 12), done*100, segmentName(s.Training.Segment)))
	} else {
		training = boxStyle.Render("進行中訓練\n無進行中訓練（到模型頁按 t 開訓）")
	}

	var milestone string
	if target, prog, ok := sim.NextMilestone(s, m.cfg); ok {
		milestone = boxStyle.Render(fmt.Sprintf("下個里程碑\n估值 $%s  %s %.0f%%",
			human(target), progressBar(prog, 10), prog*100))
	} else {
		milestone = boxStyle.Render("下個里程碑\n全部達成")
	}

	sections := []string{
		lipgloss.JoinHorizontal(lipgloss.Top, company, "  ", training),
		milestone,
	}
	if warns := pressures(m); len(warns) > 0 {
		sections = append(sections, boxStyle.Render("注意\n"+joinLines(warns)))
	}
	hint := "[Tab]切頁 [t]訓練 [q]離開"
	if s.PeakValuation >= m.cfg.PrestigeUnlockValuation {
		hint = "[Tab]切頁 [t]訓練 [P]傳承重開 [q]離開"
	}
	sections = append(sections, helpStyle.Render(hint))
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func joinLines(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += "\n"
		}
		out += s
	}
	return out
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
