package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"tokensmith/internal/model"
)

type directiveDialog struct {
	options        []model.DirectiveKind
	cursor         int
	choosingTarget bool
	targetCursor   int
}

func newDirectiveDialog(m Model) (directiveDialog, bool) {
	if m.state.Campaign.Doctrine == model.DoctrineNone {
		return directiveDialog{}, false
	}
	return directiveDialog{
		options: []model.DirectiveKind{
			model.DirectiveRoutePush,
			model.DirectiveCounter,
			model.DirectiveIntel,
		},
	}, true
}

func directiveTargets(m Model) []string {
	var out []string
	if c := m.state.Campaign.Primary.Company; c != "" {
		out = append(out, c)
	}
	if c := m.state.Campaign.Wildcard.Company; c != "" {
		out = append(out, c)
	}
	return out
}

func (d directiveDialog) update(msg tea.KeyMsg) (directiveDialog, bool, bool) {
	switch msg.String() {
	case "esc":
		if d.choosingTarget {
			d.choosingTarget = false
			d.targetCursor = 0
			return d, false, false
		}
		return d, false, true
	case "enter":
		if !d.choosingTarget {
			kind := d.options[d.cursor]
			if kind == model.DirectiveCounter || kind == model.DirectiveIntel {
				d.choosingTarget = true
				d.targetCursor = 0
				return d, false, false
			}
			return d, true, false
		}
		return d, true, false
	case "up", "left":
		if d.choosingTarget {
			if d.targetCursor > 0 {
				d.targetCursor--
			}
		} else if d.cursor > 0 {
			d.cursor--
		}
	case "down", "right":
		if d.choosingTarget {
			// Primary + wildcard at most; bounds re-checked against live targets in command/render.
			if d.targetCursor < 1 {
				d.targetCursor++
			}
		} else if d.cursor < len(d.options)-1 {
			d.cursor++
		}
	}
	return d, false, false
}

func (d directiveDialog) command(m Model) model.IssueDirective {
	kind := d.options[d.cursor]
	cmd := model.IssueDirective{Kind: kind}
	if kind == model.DirectiveCounter || kind == model.DirectiveIntel {
		targets := directiveTargets(m)
		if d.targetCursor >= 0 && d.targetCursor < len(targets) {
			cmd.Target = targets[d.targetCursor]
		}
	}
	return cmd
}

func renderDirectiveDialog(d directiveDialog, m Model) string {
	var b strings.Builder
	title := "高層指令"
	if d.choosingTarget {
		title = "選擇目標宿敵"
		targets := directiveTargets(m)
		if d.targetCursor >= len(targets) && len(targets) > 0 {
			d.targetCursor = len(targets) - 1
		}
		b.WriteString(fmt.Sprintf("指令：%s\n\n", directiveLabel(d.options[d.cursor])))
		if len(targets) == 0 {
			b.WriteString(styleMuted.Render("目前沒有可選宿敵") + "\n")
		}
		for i, t := range targets {
			marker := "  "
			line := fmt.Sprintf("[%d] %s", i+1, t)
			if d.targetCursor == i {
				marker = "▸ "
				line = styleAccent.Render(line)
			}
			b.WriteString(marker + line + "\n")
		}
	} else {
		b.WriteString("本董事會週期限用一次。\n\n")
		for i, kind := range d.options {
			marker := "  "
			line := fmt.Sprintf("[%d] %s", i+1, directiveLabel(kind))
			if d.cursor == i {
				marker = "▸ "
				line = styleAccent.Render(line)
			}
			b.WriteString(marker + line + "\n")
		}
	}
	if m.campaignError != "" {
		b.WriteString("\n" + styleWarn.Render(m.campaignError) + "\n")
	}
	help := "[↑↓]選擇 [Enter]確認 [Esc]取消"
	if d.choosingTarget {
		help = "[↑↓]選擇目標 [Enter]確認 [Esc]返回"
	}
	b.WriteString("\n" + helpStyle.Render(help))
	return Card(title, b.String())
}
