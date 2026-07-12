package tui

import (
	"fmt"
	"strings"
)

var hqStageNames = [8]string{
	"車庫", "小辦公室", "辦公樓", "科技園區", "摩天大樓", "百層巨塔", "企業之城", "太空電梯",
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

// hqStage clamps MilestonesReached into the art range.
func hqStage(milestones int) int {
	if milestones < 0 {
		return 0
	}
	if milestones > 7 {
		return 7
	}
	return milestones
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
	stage := hqStage(m.state.MilestonesReached)
	if compact {
		var icons []string
		for i, ic := range hqStageIcons {
			if i == stage {
				icons = append(icons, styleGold.Bold(true).Render(ic+hqStageNames[i]))
			} else {
				icons = append(icons, styleMuted.Render(ic))
			}
		}
		return cardContent{
			kind:  CardDefault,
			w:     w,
			title: "總部",
			body:  strings.Join(icons, styleMuted.Render("→")),
		}
	}
	lit := m.state.HasTraining && m.blink
	art := styleCyan.Render(hqArt(stage, lit))
	status := ""
	if m.state.HasTraining {
		status = styleAmber.Render("  訓練機房運轉中…")
	}
	return cardContent{
		kind:  CardDefault,
		w:     w,
		title: fmt.Sprintf("總部 — %s %s", hqStageIcons[stage], hqStageNames[stage]),
		body:  art + status,
	}
}
