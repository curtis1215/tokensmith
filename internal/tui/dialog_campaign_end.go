package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"tokensmith/internal/model"
)

type campaignEndMode int

const (
	campaignEndVictory campaignEndMode = iota
	campaignEndExit
)

type campaignEndDialog struct {
	mode         campaignEndMode
	options      []model.LegacyChoice
	cursor       int
	continueRun  bool
	choosingTech bool
	techOptions  []string
	techCursor   int
}

func newCampaignEndDialog(m Model, mode campaignEndMode) (campaignEndDialog, bool) {
	switch mode {
	case campaignEndExit:
		if m.state.Campaign.Cycle < m.cfg.Campaign.StrategyExitCycle &&
			m.state.Campaign.FinancialDistressCycles < 2 {
			return campaignEndDialog{}, false
		}
		return campaignEndDialog{mode: campaignEndExit}, true
	case campaignEndVictory:
		if m.state.Campaign.Victory == model.DoctrineNone {
			return campaignEndDialog{}, false
		}
		return campaignEndDialog{
			mode:        campaignEndVictory,
			options:     validLegacyOptions(m),
			techOptions: append([]string(nil), m.state.UnlockedTech...),
		}, true
	default:
		return campaignEndDialog{}, false
	}
}

func validLegacyOptions(m Model) []model.LegacyChoice {
	var out []model.LegacyChoice
	camp := m.state.Campaign
	// Offer only Legacy kinds that can pass sim.validateLegacy on confirm.
	if camp.Secondary != model.DoctrineNone && camp.SecondaryPerk != "" {
		out = append(out, model.LegacyChoice{
			Kind:     model.LegacySecondary,
			Doctrine: camp.Secondary,
			PerkID:   camp.SecondaryPerk,
		})
	}
	out = append(out, model.LegacyChoice{Kind: model.LegacyIntel})
	if len(m.state.UnlockedTech) > 0 {
		out = append(out, model.LegacyChoice{Kind: model.LegacyTech})
	}
	return out
}

func (d campaignEndDialog) update(msg tea.KeyMsg) (campaignEndDialog, bool, bool) {
	switch msg.String() {
	case "esc":
		if d.choosingTech {
			d.choosingTech = false
			d.techCursor = 0
			return d, false, false
		}
		return d, false, true
	case "enter":
		if d.mode == campaignEndExit {
			return d, true, false
		}
		// Victory: continue sits after last legacy option.
		if d.cursor == len(d.options) {
			d.continueRun = true
			return d, true, false
		}
		if d.cursor < 0 || d.cursor >= len(d.options) {
			return d, false, false
		}
		choice := d.options[d.cursor]
		if choice.Kind == model.LegacyTech {
			if !d.choosingTech {
				if len(d.techOptions) == 0 {
					return d, false, false
				}
				d.choosingTech = true
				d.techCursor = 0
				return d, false, false
			}
			return d, true, false
		}
		return d, true, false
	case "up", "left":
		if d.choosingTech {
			if d.techCursor > 0 {
				d.techCursor--
			}
			return d, false, false
		}
		if d.cursor > 0 {
			d.cursor--
		}
	case "down", "right":
		if d.choosingTech {
			if d.techCursor < len(d.techOptions)-1 {
				d.techCursor++
			}
			return d, false, false
		}
		// +1 for continue option in victory mode
		max := len(d.options)
		if d.mode == campaignEndVictory {
			// continue is an extra row at index len(options)
			if d.cursor < max {
				d.cursor++
			}
		} else if d.cursor < max-1 {
			d.cursor++
		}
	}
	return d, false, false
}

func (d campaignEndDialog) command() model.Command {
	if d.mode == campaignEndExit {
		return model.CampaignExit{}
	}
	if d.continueRun {
		return model.CampaignContinue{}
	}
	if d.cursor < 0 || d.cursor >= len(d.options) {
		return model.CampaignContinue{}
	}
	choice := d.options[d.cursor]
	if choice.Kind == model.LegacyTech {
		if d.techCursor >= 0 && d.techCursor < len(d.techOptions) {
			choice.TechID = d.techOptions[d.techCursor]
		}
	}
	return model.CampaignPrestige{Legacy: choice}
}

func renderCampaignEndDialog(d campaignEndDialog, m Model) string {
	var b strings.Builder
	if d.mode == campaignEndExit {
		b.WriteString("策略退出將以較低專利報酬結束本局，且不取得勝利徽章與 Legacy。\n\n")
		b.WriteString("▸ " + styleAccent.Render("[Enter] 確認策略退出") + "\n")
		if m.campaignError != "" {
			b.WriteString("\n" + styleWarn.Render(m.campaignError) + "\n")
		}
		b.WriteString("\n" + helpStyle.Render("[Enter]確認 [Esc]取消"))
		return Card("策略退出", b.String())
	}

	title := "路線勝利結算"
	if d.choosingTech {
		title = "選擇帶入科技"
		b.WriteString("選擇一項已解鎖科技帶入下一局。\n\n")
		for i, id := range d.techOptions {
			marker := "  "
			line := fmt.Sprintf("[%d] %s", i+1, techLabel(id).Name)
			if d.techCursor == i {
				marker = "▸ "
				line = styleAccent.Render(line)
			}
			b.WriteString(marker + line + "\n")
		}
		if m.campaignError != "" {
			b.WriteString("\n" + styleWarn.Render(m.campaignError) + "\n")
		}
		b.WriteString("\n" + helpStyle.Render("[↑↓]選擇 [Enter]確認 [Esc]返回"))
		return Card(title, b.String())
	}

	b.WriteString("選擇 Legacy 帶入下一局，或繼續本局無盡模式。\n\n")
	for i, leg := range d.options {
		marker := "  "
		line := fmt.Sprintf("[%d] %s", i+1, legacyChoiceLabel(leg))
		if d.cursor == i {
			marker = "▸ "
			line = styleAccent.Render(line)
		}
		b.WriteString(marker + line + "\n")
	}
	// Continue row.
	contMarker := "  "
	contLine := fmt.Sprintf("[%d] 繼續本局（無盡模式）", len(d.options)+1)
	if d.cursor == len(d.options) {
		contMarker = "▸ "
		contLine = styleAccent.Render(contLine)
	}
	b.WriteString(contMarker + contLine + "\n")
	if m.campaignError != "" {
		b.WriteString("\n" + styleWarn.Render(m.campaignError) + "\n")
	}
	b.WriteString("\n" + helpStyle.Render("[↑↓]選擇 [Enter]確認 [Esc]取消"))
	return Card(title, b.String())
}

func legacyChoiceLabel(leg model.LegacyChoice) string {
	switch leg.Kind {
	case model.LegacySecondary:
		return fmt.Sprintf("副戰略：%s / %s", doctrineLabel(leg.Doctrine), perkLabel(leg.PerkID))
	case model.LegacyIntel:
		return "宿敵完整情報"
	case model.LegacyTech:
		return "起始科技（再選一項）"
	default:
		return string(leg.Kind)
	}
}
