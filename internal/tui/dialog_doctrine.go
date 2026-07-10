package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

type doctrineDialogMode int

const (
	doctrineChoosePrimary doctrineDialogMode = iota
	doctrineChoosePerk
	doctrineChooseSecondary
	doctrineConfirmPivot
)

type doctrineDialog struct {
	mode    doctrineDialogMode
	options []string
	cursor  int
}

func newDoctrineDialog(m Model, pivot bool) (doctrineDialog, bool) {
	if pivot {
		if m.state.Campaign.Doctrine == model.DoctrineNone {
			return doctrineDialog{}, false
		}
		if m.state.Campaign.PivotUsed {
			return doctrineDialog{}, false
		}
		if m.state.Campaign.Stage == model.CampaignStageShowdown || m.state.Campaign.Stage == model.CampaignStageWon {
			return doctrineDialog{}, false
		}
		opts := nonPrimaryDoctrines(m.state.Campaign.Doctrine)
		if len(opts) == 0 {
			return doctrineDialog{}, false
		}
		return doctrineDialog{mode: doctrineConfirmPivot, options: opts}, true
	}

	camp := m.state.Campaign
	if camp.Doctrine == model.DoctrineNone {
		if !hasOnlineModel(m.state) {
			return doctrineDialog{}, false
		}
		return doctrineDialog{
			mode: doctrineChoosePrimary,
			options: []string{
				string(model.DoctrineConsumer),
				string(model.DoctrineEnterprise),
				string(model.DoctrineDeveloper),
			},
		}, true
	}
	if camp.PerkTierPending > 0 {
		perks := balance.PerksFor(m.cfg.Campaign, camp.Doctrine, camp.PerkTierPending)
		if len(perks) == 0 {
			return doctrineDialog{}, false
		}
		opts := make([]string, len(perks))
		for i, p := range perks {
			opts[i] = p.ID
		}
		return doctrineDialog{mode: doctrineChoosePerk, options: opts}, true
	}
	if camp.Stage == model.CampaignStageShowdown && camp.Secondary == model.DoctrineNone {
		opts := secondaryPerkOptions(m.cfg.Campaign, camp.Doctrine)
		if len(opts) != 2 {
			return doctrineDialog{}, false
		}
		return doctrineDialog{mode: doctrineChooseSecondary, options: opts}, true
	}
	return doctrineDialog{}, false
}

func nonPrimaryDoctrines(primary model.Doctrine) []string {
	all := []model.Doctrine{model.DoctrineConsumer, model.DoctrineEnterprise, model.DoctrineDeveloper}
	var out []string
	for _, d := range all {
		if d != primary {
			out = append(out, string(d))
		}
	}
	return out
}

// secondaryPerkOptions returns exactly one tier-1 perk ID per non-primary doctrine.
func secondaryPerkOptions(cfg balance.CampaignConfig, primary model.Doctrine) []string {
	all := []model.Doctrine{model.DoctrineConsumer, model.DoctrineEnterprise, model.DoctrineDeveloper}
	var out []string
	for _, d := range all {
		if d == primary {
			continue
		}
		perks := balance.PerksFor(cfg, d, 1)
		if len(perks) == 0 {
			continue
		}
		out = append(out, perks[0].ID)
	}
	return out
}

func hasOnlineModel(s model.GameState) bool {
	for _, md := range s.Models {
		if md.Online {
			return true
		}
	}
	return false
}

func (d doctrineDialog) update(msg tea.KeyMsg) (doctrineDialog, bool, bool) {
	switch msg.String() {
	case "esc":
		return d, false, true
	case "enter":
		return d, true, false
	case "up", "left":
		if d.cursor > 0 {
			d.cursor--
		}
	case "down", "right":
		if d.cursor < len(d.options)-1 {
			d.cursor++
		}
	}
	return d, false, false
}

func (d doctrineDialog) command(m Model) model.Command {
	if len(d.options) == 0 || d.cursor < 0 || d.cursor >= len(d.options) {
		return model.ChooseDoctrine{} // invalid; Apply will reject
	}
	choice := d.options[d.cursor]
	switch d.mode {
	case doctrineChoosePrimary:
		return model.ChooseDoctrine{Doctrine: model.Doctrine(choice)}
	case doctrineChoosePerk:
		return model.ChooseDoctrinePerk{PerkID: choice}
	case doctrineChooseSecondary:
		p, _ := balance.CampaignPerkByID(m.cfg.Campaign, choice)
		return model.ChooseSecondaryDoctrine{Doctrine: p.Doctrine, PerkID: p.ID}
	default:
		return model.PivotDoctrine{Doctrine: model.Doctrine(choice)}
	}
}

func renderDoctrineDialog(d doctrineDialog, m Model) string {
	var b strings.Builder
	title := "公司戰略"
	switch d.mode {
	case doctrineChoosePrimary:
		title = "選擇主要戰略"
		b.WriteString("選定後將開啟董事會週期與宿敵路線。\n\n")
	case doctrineChoosePerk:
		title = fmt.Sprintf("選擇第 %d 階能力", m.state.Campaign.PerkTierPending)
		b.WriteString("路線推進解鎖的能力（二選一）。\n\n")
	case doctrineChooseSecondary:
		title = "選擇副戰略"
		b.WriteString("決勝階段可附加一條副路線（含其一階能力）。\n\n")
	case doctrineConfirmPivot:
		title = "確認戰略轉型"
		b.WriteString("轉型將重置路線進度與能力，並消耗現金與 R&D。\n\n")
	}
	for i, opt := range d.options {
		label := doctrineOptionLabel(d.mode, opt)
		marker := "  "
		line := fmt.Sprintf("[%d] %s", i+1, label)
		if d.cursor == i {
			marker = "▸ "
			line = styleAccent.Render(line)
		}
		b.WriteString(marker + line + "\n")
	}
	if m.campaignError != "" {
		b.WriteString("\n" + styleWarn.Render(m.campaignError) + "\n")
	}
	help := "[↑↓]選擇 [Enter]確認 [Esc]取消"
	if d.mode == doctrineConfirmPivot {
		help = "[↑↓]目標 [Enter]確認轉型 [Esc]取消"
	}
	b.WriteString("\n" + helpStyle.Render(help))
	return Card(title, b.String())
}

func doctrineOptionLabel(mode doctrineDialogMode, opt string) string {
	switch mode {
	case doctrineChoosePrimary, doctrineConfirmPivot:
		return doctrineLabel(model.Doctrine(opt))
	case doctrineChoosePerk, doctrineChooseSecondary:
		p, ok := balance.CampaignPerkByID(balance.Default().Campaign, opt)
		if ok {
			return fmt.Sprintf("%s（%s）", perkLabel(opt), doctrineLabel(p.Doctrine))
		}
		return perkLabel(opt)
	default:
		return opt
	}
}

func campaignErrorText(err error) string {
	switch {
	case errors.Is(err, sim.ErrInsufficientCash):
		return "現金不足"
	case errors.Is(err, sim.ErrInsufficientRnD):
		return "R&D 不足"
	case errors.Is(err, sim.ErrDirectiveUsed):
		return "本週期已使用高層指令"
	case errors.Is(err, sim.ErrPerkChoiceNotReady):
		return "目前沒有可選的流派能力"
	case errors.Is(err, sim.ErrPivotLocked):
		return "決勝階段不可轉型"
	case errors.Is(err, sim.ErrStrategyExitLocked):
		return "第 18 週期後才能策略退出"
	default:
		return "此策略目前無法執行"
	}
}
