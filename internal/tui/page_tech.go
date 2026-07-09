package tui

import (
	"fmt"
	"strings"

	"tokensmith/internal/model"
)

var branchNames = [model.NumBranches]string{"演算法", "硬體基建", "商業營運", "對齊安全"}

func renderTech(m Model) string {
	s := m.state

	// Group nodes by branch
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

	var rows []string
	rows = append(rows, fmt.Sprintf("科技樹  可用 R&D %s", human(s.Resources.RnD)))
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
			switch {
			case techUnlocked(s, node.ID):
				stateStr = "✓ 已解鎖"
			case !prereqsMet(s, node.Prereqs):
				var prereqNames []string
				for _, p := range node.Prereqs {
					prereqNames = append(prereqNames, techLabel(p).Name)
				}
				stateStr = "🔒 需 " + strings.Join(prereqNames, ",")
			}

			nameWithID := fmt.Sprintf("%s (%s)", meta.Name, node.ID)
			lines = append(lines, fmt.Sprintf("%s %-25s %-16s | %s",
				cursor, nameWithID, stateStr, meta.Effect))
		}
		rows = append(rows, Card(br.name, VStack(lines...)))
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
