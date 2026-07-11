package tui

import (
	"fmt"
	"math"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"tokensmith/internal/model"
)

const trophyArt = `   ___________
  '._==_==_=_.'
  .-\:      /-.
 | (|:.     |) |
  '-|:.     |-'
    \::.    /
     '::. .'
       ) (
     _.' '._
    '-------'`

func legacyChoiceDesc(leg model.LegacyChoice) string {
	switch leg.Kind {
	case model.LegacySecondary:
		return "帶著副戰略與其\n一階能力開新局"
	case model.LegacyIntel:
		return "下一局宿敵行動\n情報全開"
	case model.LegacyTech:
		return "帶一項已解鎖科技\n開局（再選一項）"
	default:
		return ""
	}
}

func legacyShortLabel(leg model.LegacyChoice) string {
	switch leg.Kind {
	case model.LegacySecondary:
		return "副戰略"
	case model.LegacyIntel:
		return "宿敵完整情報"
	case model.LegacyTech:
		return "起始科技"
	default:
		return string(leg.Kind)
	}
}

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

	if d.choosingTech {
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
		return Card("選擇帶入科技", b.String())
	}

	day := int(m.state.GameTime / 86400)
	patents := int(math.Floor(math.Sqrt(m.state.PeakValuation / m.cfg.PatentK)))
	var badges []string
	for _, doc := range m.state.Prestige.RouteBadges {
		badges = append(badges, doctrineLabel(doc))
	}
	badges = append(badges, doctrineLabel(m.state.Campaign.Victory)+"（本局）")
	recap := VStack(
		styleGold.Render(trophyArt),
		"",
		KV("本局天數", fmt.Sprintf("%d", day)),
		KV("峰值估值", "$"+human(m.state.PeakValuation)),
		KV("結算專利", fmt.Sprintf("+%d", patents)),
		KV("路線徽章", strings.Join(badges, "、")),
		"",
		"選擇 Legacy 帶入下一局，或繼續本局無盡模式。",
	)
	var cards []string
	for i, leg := range d.options {
		kind := CardDefault
		if d.cursor == i {
			kind = CardAccent
		}
		cards = append(cards, CardIn(kind, 26,
			fmt.Sprintf("[%d] %s", i+1, legacyShortLabel(leg)), legacyChoiceDesc(leg)))
	}
	row := HRow(1, cards...)
	contMarker := "  "
	contLine := fmt.Sprintf("[%d] 繼續本局（無盡模式）", len(d.options)+1)
	if d.cursor == len(d.options) {
		contMarker = "▸ "
		contLine = styleAccent.Render(contLine)
	}
	body := VStack(recap, row, contMarker+contLine)
	if m.campaignError != "" {
		body = VStack(body, styleWarn.Render(m.campaignError))
	}
	body = VStack(body, "", helpStyle.Render("[↑↓]選擇 [Enter]確認 [Esc]取消"))
	return CardIn(CardGold, 0, "🏆 路線勝利結算", body)
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

// newRunEpic builds the fresh-run opening overlay after prestige/exit.
func newRunEpic(m Model) *Moment {
	p := m.state.Prestige
	lines := []string{fmt.Sprintf("帶入專利 ×%.0f", p.Patents)}
	if len(p.RouteBadges) > 0 {
		var badges []string
		for _, d := range p.RouteBadges {
			badges = append(badges, doctrineLabel(d))
		}
		lines = append(lines, "徽章："+strings.Join(badges, "、"))
	}
	if p.PendingLegacy.Kind != "" {
		lines = append(lines, "Legacy："+legacyChoiceLabel(p.PendingLegacy))
	}
	lines = append(lines, "", "新的輪迴開始——祝這次更快。")
	mo := Moment{Level: LevelEpic, Text: strings.Join(lines, "\n"), Title: "🔄 傳承開局"}
	return &mo
}
