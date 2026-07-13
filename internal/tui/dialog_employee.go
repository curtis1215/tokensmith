package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

// employeeDetailDialog is a read-only modal for roster employees or market
// candidates: stats, pay quotes, and full skill names/effects.
type employeeDetailDialog struct {
	emp        model.Employee
	fromMarket bool // true = candidate (hire quote); false = roster (severance quote)
}

// newEmployeeDetailDialog opens detail for the focused team selection.
func newEmployeeDetailDialog(m Model) (employeeDetailDialog, bool) {
	if m.teamFocusRoster {
		if len(m.state.Employees) == 0 {
			return employeeDetailDialog{}, false
		}
		i := m.rosterCursor
		if i < 0 || i >= len(m.state.Employees) {
			return employeeDetailDialog{}, false
		}
		return employeeDetailDialog{emp: m.state.Employees[i], fromMarket: false}, true
	}
	if len(m.state.Market.Candidates) == 0 {
		return employeeDetailDialog{}, false
	}
	i := m.marketCursor
	if i < 0 || i >= len(m.state.Market.Candidates) {
		return employeeDetailDialog{}, false
	}
	return employeeDetailDialog{emp: m.state.Market.Candidates[i], fromMarket: true}, true
}

func (d employeeDetailDialog) update(msg tea.KeyMsg) (next employeeDetailDialog, cancel bool) {
	switch msg.String() {
	case "esc", "escape", "q", "enter":
		return d, true
	default:
		return d, false
	}
}

func skillTierNameZH(t model.SkillTier) string {
	switch t {
	case model.SkillTierManager:
		return "經理級"
	case model.SkillTierDirector:
		return "總監級"
	case model.SkillTierGod:
		return "大神級"
	default:
		return "技能"
	}
}

// skillEffectBlurb builds a short Traditional Chinese description from SkillDef
// hooks (catalog has NameZH but no free-text Description field).
func skillEffectBlurb(sk balance.SkillDef) string {
	var parts []string
	if sk.Signature {
		parts = append(parts, "絕活")
	}
	if sk.SelfRolePowerMult > 0 && sk.HasPrefer {
		parts = append(parts, fmt.Sprintf("主職%s功率 ×%.2f", roleNameZH(sk.PreferRole), sk.SelfRolePowerMult))
	} else if sk.SelfRolePowerMult > 0 {
		parts = append(parts, fmt.Sprintf("本人主職功率 ×%.2f", sk.SelfRolePowerMult))
	}
	for r := model.Role(0); r < model.NumRoles; r++ {
		if sk.CompanyRolePower[r] != 0 {
			parts = append(parts, fmt.Sprintf("全公司%s +%.0f%%", roleNameZH(r), sk.CompanyRolePower[r]*100))
		}
	}
	if sk.SelfSalaryMult > 0 && sk.SelfSalaryMult != 1 {
		parts = append(parts, fmt.Sprintf("本人月薪 ×%.2f", sk.SelfSalaryMult))
	}
	if sk.CompanySalaryMult > 0 && sk.CompanySalaryMult != 1 {
		parts = append(parts, fmt.Sprintf("全公司月薪 ×%.2f", sk.CompanySalaryMult))
	}
	if sk.HireCostMult > 0 && sk.HireCostMult != 1 {
		parts = append(parts, fmt.Sprintf("簽約金 ×%.2f", sk.HireCostMult))
	}
	if sk.SeveranceMult > 0 && sk.SeveranceMult != 1 {
		if sk.Family == "severance_company" {
			parts = append(parts, fmt.Sprintf("全公司遣散 ×%.2f", sk.SeveranceMult))
		} else {
			parts = append(parts, fmt.Sprintf("本人遣散 ×%.2f", sk.SeveranceMult))
		}
	}
	if sk.TokenRnDMult > 0 && sk.TokenRnDMult != 1 {
		parts = append(parts, fmt.Sprintf("Token→R&D ×%.2f", sk.TokenRnDMult))
	}
	if sk.InfraMult > 0 && sk.InfraMult != 1 {
		parts = append(parts, fmt.Sprintf("算力效率 ×%.2f", sk.InfraMult))
	}
	if sk.UserGrowthMult > 0 && sk.UserGrowthMult != 1 {
		parts = append(parts, fmt.Sprintf("用戶成長 ×%.2f", sk.UserGrowthMult))
	}
	if sk.ChurnMult > 0 && sk.ChurnMult != 1 {
		parts = append(parts, fmt.Sprintf("服務流失 ×%.2f", sk.ChurnMult))
	}
	if sk.TrainQualityMult > 0 && sk.TrainQualityMult != 1 {
		parts = append(parts, fmt.Sprintf("訓練品質 ×%.2f", sk.TrainQualityMult))
	}
	if sk.RevenueMult > 0 && sk.RevenueMult != 1 {
		parts = append(parts, fmt.Sprintf("營收 ×%.2f", sk.RevenueMult))
	}
	if sk.SecondaryWeight > 0 {
		parts = append(parts, fmt.Sprintf("副加權重 %.2f", sk.SecondaryWeight))
	}
	if sk.ExtraSeats > 0 {
		parts = append(parts, fmt.Sprintf("工位 +%d", sk.ExtraSeats))
	}
	if sk.MarketRarityBonus > 0 {
		parts = append(parts, fmt.Sprintf("市場稀有度 +%.1f 階", sk.MarketRarityBonus))
	}
	if sk.RerollBaseMult > 0 && sk.RerollBaseMult != 1 {
		parts = append(parts, fmt.Sprintf("重抽費 ×%.2f", sk.RerollBaseMult))
	}
	if sk.SelfStatMult > 0 && sk.SelfStatMult != 1 {
		parts = append(parts, fmt.Sprintf("本人四維結算 ×%.2f", sk.SelfStatMult))
	}
	if sk.EventNegMult > 0 && sk.EventNegMult != 1 {
		parts = append(parts, fmt.Sprintf("負面事件 ×%.2f", sk.EventNegMult))
	}
	if len(parts) == 0 {
		return "被動效果"
	}
	return strings.Join(parts, " · ")
}

func renderEmployeeDetailDialog(d employeeDetailDialog, m Model) string {
	e := d.emp
	s := m.state
	var b strings.Builder
	src := "在職員工"
	if d.fromMarket {
		src = "人才市場候選人"
	}
	b.WriteString(fmt.Sprintf("【員工詳情】%s\n\n", src))
	b.WriteString(fmt.Sprintf("%s  ·  %s  ·  主職 %s\n", e.Name, rankNameZH(e.Rank), roleNameZH(e.PrimaryRole)))
	b.WriteString(fmt.Sprintf("四維  %s\n", employeeStatsBlurb(e)))

	if d.fromMarket {
		hire := sim.HireCostQuote(s, e, m.cfg)
		pay := sim.EffectiveMonthlySalaryForHire(e, s, m.cfg)
		b.WriteString(fmt.Sprintf("簽約  $%s    月薪  $%s/月\n", human(hire), human(pay)))
	} else {
		pay := sim.EffectiveMonthlySalary(e, s, m.cfg)
		sev := sim.SeveranceQuote(e, s, m.cfg)
		b.WriteString(fmt.Sprintf("月薪  $%s/月    遣散  $%s\n", human(pay), human(sev)))
	}

	b.WriteString("\n── 技能 ──────────────────────────────\n")
	if len(e.SkillIDs) == 0 {
		b.WriteString(styleMuted.Render("（此等級無被動技能）") + "\n")
	} else {
		for i, id := range e.SkillIDs {
			sk, ok := balance.SkillByID(m.cfg, id)
			if !ok {
				b.WriteString(fmt.Sprintf("%d. %s  （未知技能）\n", i+1, id))
				continue
			}
			sig := ""
			if sk.Signature {
				sig = " ★"
			}
			b.WriteString(fmt.Sprintf("%d. %s%s  [%s]\n", i+1, sk.NameZH, sig, skillTierNameZH(sk.Tier)))
			b.WriteString(fmt.Sprintf("   %s\n", skillEffectBlurb(sk)))
		}
	}
	b.WriteString("\n[Esc/Enter] 關閉")
	// Width roughly content width of the dashboard.
	w := m.contentWidth()
	if w < 40 {
		w = 40
	}
	if w > 72 {
		w = 72
	}
	return CardIn(CardDefault, w, "員工詳情", strings.TrimRight(b.String(), "\n"))
}
