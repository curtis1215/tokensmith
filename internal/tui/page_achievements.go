package tui

import (
	"fmt"
	"time"
)

func renderAchievements(m Model) string {
	cw := m.contentWidth()
	total := len(achievementCatalog)
	done := 0
	for _, a := range achievementCatalog {
		if _, ok := m.achievements[a.ID]; ok {
			done++
		}
	}
	frac := 0.0
	if total > 0 {
		frac = float64(done) / float64(total)
	}
	header := CardIn(CardGold, cw, "成就進度",
		fmt.Sprintf("%s %d/%d", GoldBar(frac, 24), done, total))

	rows := []string{header}
	for _, cat := range achievementCategories {
		var lines []string
		for _, a := range achievementCatalog[cat.From:cat.To] {
			if at, ok := m.achievements[a.ID]; ok {
				day := time.Unix(at, 0).Format("2006-01-02")
				lines = append(lines, styleGold.Render(fmt.Sprintf("🏆 %s — %s（%s）", a.Name, a.Desc, day)))
			} else {
				lines = append(lines, styleMuted.Render(fmt.Sprintf("🔒 %s — %s", a.Name, a.Desc)))
			}
		}
		rows = append(rows, CardIn(CardDefault, cw, cat.Title, VStack(lines...)))
	}
	return VStack(rows...)
}
