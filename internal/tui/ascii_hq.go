package tui

import (
	"fmt"
	"strings"

	"tokensmith/internal/balance"
)

// hqStageNames aligned with balance.Config.OfficeNames levels 1..8.
var hqStageNames = [8]string{
	"車庫", "小辦公室", "開放式樓層", "辦公樓", "園區", "摩天樓", "巨塔", "太空電梯",
}

var hqStageIcons = [8]string{"🏠", "🏢", "🏬", "🏞", "🏙", "🗼", "🌃", "🚀"}

// hqArts 每階段 5 行、寬 ≤ 30；'◇' 是機房燈占位符。
var hqArts = [8]string{
	`      _______
     /_______\
     | .-. ◇ |
     | |_|   |
     '-------'`,
	`     _________
    |  _   _  |
    | |_| |_| |
    | |_| |_|◇|
    '---------'`,
	`     _________
    | [] [] [] |
    | [] [] [] |
    | [] [] ◇[]|
    '----_----'`,
	`   ____   ______
  | [] |_| [][] |
  | [] |_| [][] |
  | []◇|_| [][] |
  '----' '------'`,
	`       _/\_
      | [] |
      | [] |    __
      | []◇|___|  |
      '----'---'--'`,
	`        /\
       |[]|  /\
       |[]| |[]|
       |[]◇||[]|
      _|--|_|--|_`,
	`   /\   _/\_   /\
  |[]| _|[][]| |[]|
  |[]||[][]◇| _|[]|
  |[]||[][][]||[][]|
  '--''------''----'`,
	`        .  ✦  .
        |     🌙
       ||| 
       |||◇
    ___|||_______`,
}

// hqStageFromOffice maps Office.Level (1..8) to art index 0..7.
func hqStageFromOffice(level int) int {
	if level < 1 {
		level = 1
	}
	if level > 8 {
		level = 8
	}
	return level - 1
}

// hqStage clamps a raw art index into the art range.
func hqStage(stage int) int {
	if stage < 0 {
		return 0
	}
	if stage > 7 {
		return 7
	}
	return stage
}

// hqArt renders a stage; lit swaps the datacenter light on (訓練中閃爍用).
func hqArt(stage int, lit bool) string {
	lamp := "○"
	if lit {
		lamp = "●"
	}
	return strings.ReplaceAll(hqArts[hqStage(stage)], "◇", lamp)
}

// renderHQ is the headquarters card. Width < 100 → compact icon strip (legacy
// standalone threshold). Prefer hqContent with an explicit compact flag when the
// page breakpoint should decide art mode independently of column width.
func renderHQ(m Model, w int) string {
	c := hqContent(m, w, w < 100)
	return CardIn(c.kind, c.w, c.title, c.body)
}

// hqContent builds HQ card pieces. compact=true uses the single-line stage strip;
// compact=false always uses full ASCII art (art width ≤ 30 cells) regardless of w.
func hqContent(m Model, w int, compact bool) cardContent {
	stage := hqStageFromOffice(m.state.Office.Level)
	if compact {
		var icons []string
		for i, ic := range hqStageIcons {
			if i == stage {
				icons = append(icons, styleGold.Bold(true).Render(ic+hqStageNames[i]))
			} else {
				icons = append(icons, styleMuted.Render(ic))
			}
		}
		hq := balance.OfficeTokenRnDMultAt(m.state.Office.Level, m.cfg)
		body := strings.Join(icons, styleMuted.Render("→")) +
			styleMuted.Render(fmt.Sprintf(" · Token→R&D ×%.2f", hq))
		return cardContent{
			kind:  CardDefault,
			w:     w,
			title: "總部",
			body:  body,
		}
	}
	lit := m.state.HasTraining && m.blink
	art := styleCyan.Render(hqArt(stage, lit))
	status := ""
	if m.state.HasTraining {
		status = styleAmber.Render("  訓練機房運轉中…")
	}
	hq := balance.OfficeTokenRnDMultAt(m.state.Office.Level, m.cfg)
	multLine := styleMuted.Render(fmt.Sprintf("Token→R&D ×%.2f", hq))
	return cardContent{
		kind:  CardDefault,
		w:     w,
		title: fmt.Sprintf("總部 — %s %s", hqStageIcons[stage], hqStageNames[stage]),
		body:  art + status + "\n" + multLine,
	}
}
