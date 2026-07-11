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

// renderHQ is the overview headquarters card; narrow terminals collapse to
// a single icon progression row.
func renderHQ(m Model, w int) string {
	stage := hqStage(m.state.MilestonesReached)
	if w < 100 {
		var icons []string
		for i, ic := range hqStageIcons {
			if i == stage {
				icons = append(icons, styleGold.Bold(true).Render(ic+hqStageNames[i]))
			} else {
				icons = append(icons, styleMuted.Render(ic))
			}
		}
		return CardIn(CardDefault, w, "總部", strings.Join(icons, styleMuted.Render("→")))
	}
	lit := m.state.HasTraining && m.blink
	art := styleCyan.Render(hqArt(stage, lit))
	status := ""
	if m.state.HasTraining {
		status = styleAmber.Render("  訓練機房運轉中…")
	}
	title := fmt.Sprintf("總部 — %s %s", hqStageIcons[stage], hqStageNames[stage])
	return CardIn(CardDefault, w, title, art+status)
}
