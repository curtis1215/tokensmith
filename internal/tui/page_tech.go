package tui

import (
	"fmt"
	"strings"

	"tokensmith/internal/model"
)

var branchNames = [model.NumBranches]string{"演算法", "硬體基建", "商業營運", "對齊安全"}

// techVisualOrder returns TechNodes indices in the order shown on the tech page
// (branch 0→NumBranches-1, catalog order within each branch). ↑↓ must walk this
// order — not the flat catalog index — or the cursor jumps between cards.
func techVisualOrder(nodes []model.TechNode) []int {
	order := make([]int, 0, len(nodes))
	for b := 0; b < model.NumBranches; b++ {
		for i, n := range nodes {
			if n.Branch == model.TechBranch(b) {
				order = append(order, i)
			}
		}
	}
	return order
}

func renderTech(m Model) string {
	s := m.state
	inner := m.cardInnerWidth()

	// Group nodes by branch (same order as techVisualOrder)
	type groupedBranch struct {
		name  string
		nodes []int // indices into m.cfg.TechNodes
	}

	branches := make([]groupedBranch, model.NumBranches)
	for b := 0; b < model.NumBranches; b++ {
		branches[b] = groupedBranch{
			name: branchNames[b],
		}
	}

	for i, node := range m.cfg.TechNodes {
		branches[node.Branch].nodes = append(branches[node.Branch].nodes, i)
	}

	cw := m.contentWidth()
	var rows []string
	rows = append(rows, TruncateWidth(fmt.Sprintf("科技樹  可用 R&D %s", human(s.Resources.RnD)), cw))
	for _, br := range branches {
		if len(br.nodes) == 0 {
			continue
		}
		var lines []string
		for _, idx := range br.nodes {
			node := m.cfg.TechNodes[idx]
			cursor := " "
			if idx == m.techCursor {
				cursor = "▸"
			}

			meta := techLabel(node.ID)

			stateStr := fmt.Sprintf("%s R&D", human(node.Cost))
			locked := false
			switch {
			case techUnlocked(s, node.ID):
				stateStr = styleGain.Render("✓") + " 已解鎖"
			case !prereqsMet(s, node.Prereqs):
				var prereqNames []string
				for _, p := range node.Prereqs {
					prereqNames = append(prereqNames, techLabel(p).Name)
				}
				stateStr = "🔒 需 " + strings.Join(prereqNames, ",")
				locked = true
			}

			nameWithID := fmt.Sprintf("%s (%s)", meta.Name, node.ID)
			line := fmt.Sprintf("%s %-25s %-16s | %s",
				cursor, nameWithID, stateStr, meta.Effect)
			line = TruncateWidth(line, inner)
			if locked {
				line = styleMuted.Render(line)
			}
			lines = append(lines, line)
		}
		rows = append(rows, CardIn(CardDefault, cw, br.name, VStack(lines...)))
	}
	return VStack(rows...)
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
