package tui

import (
	"fmt"

	"tokensmith/internal/model"
)

// renderTeam is a minimal placeholder until Task 11 rewrites the team page
// for individual employees, talent market, and office upgrade.
func renderTeam(m Model) string {
	s := m.state
	cw := m.contentWidth()
	name := hqStageNames[hqStageFromOffice(s.Office.Level)]
	body := VStack(
		KV("辦公室", fmt.Sprintf("Lv%d %s", max(1, s.Office.Level), name)),
		KV("在職", fmt.Sprintf("%d 人", len(s.Employees))),
		KV("人才市場", fmt.Sprintf("%d 位候選人", len(s.Market.Candidates))),
		"",
		"完整團隊頁（雇用/解雇/升級）見後續任務",
	)
	return CardIn(CardDefault, cw, "團隊", body)
}

// primaryRoleCounts tallies employees by primary role (used by achievements).
func primaryRoleCounts(s model.GameState) [model.NumRoles]int {
	var c [model.NumRoles]int
	for _, e := range s.Employees {
		if e.PrimaryRole >= 0 && e.PrimaryRole < model.NumRoles {
			c[e.PrimaryRole]++
		}
	}
	return c
}
