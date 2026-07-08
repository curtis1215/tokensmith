package tui

import (
	"fmt"
	"strings"

	"tokensmith/internal/model"
)

var branchNames = [model.NumBranches]string{"演算法", "硬體基建", "商業營運", "對齊安全"}

func renderTech(m Model) string {
	s := m.state
	var b strings.Builder
	b.WriteString(fmt.Sprintf("科技樹  可用 R&D %s\n\n", human(s.Resources.RnD)))
	for i, node := range m.cfg.TechNodes {
		cursor := " "
		if i == m.techCursor {
			cursor = "▸"
		}
		state := fmt.Sprintf("%s R&D", human(node.Cost))
		switch {
		case techUnlocked(s, node.ID):
			state = "✓ 已解鎖"
		case !prereqsMet(s, node.Prereqs):
			state = "🔒 需 " + strings.Join(node.Prereqs, ",")
		}
		b.WriteString(fmt.Sprintf("%s [%s] %-18s %s\n",
			cursor, branchNames[node.Branch], node.ID, state))
	}
	b.WriteString(helpStyle.Render("\n[↑↓]選節點 [Enter]解鎖 [Tab]切頁"))
	return b.String()
}

func techUnlocked(s model.GameState, id string) bool {
	for _, u := range s.UnlockedTech {
		if u == id {
			return true
		}
	}
	return false
}

func prereqsMet(s model.GameState, prereqs []string) bool {
	for _, p := range prereqs {
		if !techUnlocked(s, p) {
			return false
		}
	}
	return true
}
