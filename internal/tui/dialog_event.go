package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"tokensmith/internal/balance"
	"tokensmith/internal/sim"
)

// eventDialog lets the player resolve the oldest pending industry event.
// cursor is the highlighted choice; it starts on the free default (1).
type eventDialog struct {
	cursor int
}

func newEventDialog(m Model) (eventDialog, bool) {
	if len(m.state.Events.Pending) == 0 {
		return eventDialog{}, false
	}
	return eventDialog{cursor: 1}, true
}

func (d eventDialog) update(msg tea.KeyMsg) (next eventDialog, confirm, cancel bool) {
	switch msg.String() {
	case "esc":
		return d, false, true
	case "enter":
		return d, true, false
	case "left", "up":
		d.cursor = 0
	case "right", "down":
		d.cursor = 1
	}
	return d, false, false
}

func renderEventDialog(d eventDialog, m Model) string {
	p := m.state.Events.Pending[0]
	meta := eventLabel(p.EventID)
	spec, ok := balance.EventByID(m.cfg.Events, p.EventID)

	var b strings.Builder
	b.WriteString(meta.Desc + "\n\n")
	if ok {
		days := (p.Deadline - m.state.GameTime) / 86400
		if days < 0 {
			days = 0
		}
		b.WriteString(fmt.Sprintf("決策期限：剩 %.0f 天（逾時自動選保守項）\n\n", days))
		cash, rnd := sim.EventChoiceCost(m.state, spec)
		cost := fmt.Sprintf("$%s", human(cash))
		if rnd > 0 {
			cost += fmt.Sprintf(" + %s R&D", human(rnd))
		}
		labels := [2]string{
			fmt.Sprintf("%s — 費用 %s", meta.Choices[0], cost),
			meta.Choices[1],
		}
		for i, label := range labels {
			marker := "  "
			line := fmt.Sprintf("[%d] %s", i+1, label)
			if d.cursor == i {
				marker = "▸ "
				line = styleAccent.Render(line)
			}
			b.WriteString(marker + line + "\n")
		}
	} else {
		b.WriteString("（此事件版本已不存在，確認後移除）\n")
	}
	b.WriteString("\n" + helpStyle.Render("[←→]選擇 [Enter]確認 [Esc]稍後再說"))
	return Card("📰 "+meta.Name, b.String())
}
