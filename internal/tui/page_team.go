package tui

import (
	"errors"
	"fmt"
	"strings"

	"tokensmith/internal/balance"
	"tokensmith/internal/model"
	"tokensmith/internal/sim"
)

// rankNameZH maps employee Rank to Traditional Chinese display labels.
func rankNameZH(r model.Rank) string {
	switch r {
	case model.RankGrunt:
		return "雜魚"
	case model.RankStaff:
		return "職員"
	case model.RankLead:
		return "幹部"
	case model.RankManager:
		return "經理"
	case model.RankDirector:
		return "總監"
	case model.RankGod:
		return "大神"
	default:
		return "？"
	}
}

// roleNameZH maps Role to Traditional Chinese labels (研發/工程/營運/行銷).
func roleNameZH(r model.Role) string {
	switch r {
	case model.RoleResearcher:
		return "研發"
	case model.RoleEngineer:
		return "工程"
	case model.RoleOps:
		return "營運"
	case model.RoleMarketing:
		return "行銷"
	default:
		return "？"
	}
}

// totalMonthlyPayroll sums roster MonthlySalary (UI primary unit).
func totalMonthlyPayroll(emps []model.Employee) float64 {
	var sum float64
	for _, e := range emps {
		sum += e.MonthlySalary
	}
	return sum
}

// employeeStatsBlurb is a compact four-stat line, e.g. "研40 工10 營10 行10".
func employeeStatsBlurb(e model.Employee) string {
	return fmt.Sprintf("研%d 工%d 營%d 行%d",
		e.Stats[model.RoleResearcher],
		e.Stats[model.RoleEngineer],
		e.Stats[model.RoleOps],
		e.Stats[model.RoleMarketing])
}

// skillCountLabel formats skill count for roster rows.
func skillCountLabel(n int) string {
	if n <= 0 {
		return "無技能"
	}
	return fmt.Sprintf("%d技能", n)
}

// officeUpgradeLine shows next upgrade cost or maxed status.
func officeUpgradeLine(level int, b balance.Config) string {
	if level < 1 {
		level = 1
	}
	cost, ok := balance.OfficeUpgradeCostAt(level, b)
	if !ok {
		return "升級 已滿級"
	}
	return fmt.Sprintf("升級 $%s  [u]", human(cost))
}

// marketRefreshRemainSec is seconds until free talent-market refresh.
func marketRefreshRemainSec(s model.GameState) float64 {
	rem := s.Market.NextRefreshAt - s.GameTime
	if rem < 0 {
		return 0
	}
	return rem
}

func renderTeam(m Model) string {
	cw := m.contentWidth()
	gap := 2

	officeCard := renderTeamOffice(m, cw)
	if cw < minDashWidth {
		return VStack(officeCard, renderTeamRoster(m, cw), renderTeamMarket(m, cw))
	}
	// Wide: office full width, then roster | market side by side.
	colW := (cw - gap) / 2
	row := HRow(gap, renderTeamRoster(m, colW), renderTeamMarket(m, colW))
	return VStack(officeCard, row)
}

func renderTeamOffice(m Model, w int) string {
	s := m.state
	level := s.Office.Level
	if level < 1 {
		level = 1
	}
	stage := hqStageFromOffice(level)
	name := hqStageNames[stage]
	seats := sim.SeatCap(s, m.cfg)
	filled := len(s.Employees)
	body := VStack(
		KV("等級", fmt.Sprintf("Lv%d %s", level, name)),
		KV("工位", fmt.Sprintf("%d/%d", filled, seats)),
		KV("月薪合計", fmt.Sprintf("$%s/月", human(totalMonthlyPayroll(s.Employees)))),
		officeUpgradeLine(level, m.cfg),
	)
	return CardIn(CardDefault, w, "辦公室", body)
}

func renderTeamRoster(m Model, w int) string {
	s := m.state
	var lines []string
	if len(s.Employees) == 0 {
		lines = append(lines, styleMuted.Render("（尚無員工 — 從人才市場雇用）"))
	} else {
		for i, e := range s.Employees {
			cur := "  "
			if m.teamFocusRoster && i == m.rosterCursor {
				cur = "▸ "
			}
			lines = append(lines, fmt.Sprintf("%s%s · %s · %s · 月薪 $%s/月 · %s",
				cur,
				e.Name,
				rankNameZH(e.Rank),
				roleNameZH(e.PrimaryRole),
				human(e.MonthlySalary),
				skillCountLabel(len(e.SkillIDs)),
			))
		}
	}
	lines = append(lines, "")
	lines = append(lines, KV("月薪合計", fmt.Sprintf("$%s/月", human(totalMonthlyPayroll(s.Employees)))))
	focusHint := ""
	if m.teamFocusRoster {
		focusHint = " · 焦點"
	}
	title := fmt.Sprintf("在職 %d%s", len(s.Employees), focusHint)
	return CardIn(CardDefault, w, title, VStack(lines...))
}

func renderTeamMarket(m Model, w int) string {
	s := m.state
	var lines []string
	cands := s.Market.Candidates
	if len(cands) == 0 {
		lines = append(lines, styleMuted.Render("（市場空 — 等待刷新或按 r 重抽）"))
	} else {
		n := len(cands)
		if n > 5 {
			n = 5
		}
		for i := 0; i < n; i++ {
			e := cands[i]
			cur := "  "
			if !m.teamFocusRoster && i == m.marketCursor {
				cur = "▸ "
			}
			lines = append(lines, fmt.Sprintf("%s%s · %s · %s · %s",
				cur, e.Name, rankNameZH(e.Rank), roleNameZH(e.PrimaryRole), employeeStatsBlurb(e)))
			hire := sim.HireCostQuote(s, e, m.cfg)
			lines = append(lines, fmt.Sprintf("    簽約 $%s · 月薪 $%s/月 · %s",
				human(hire), human(e.MonthlySalary), skillCountLabel(len(e.SkillIDs))))
		}
	}
	lines = append(lines, "")
	remain := marketRefreshRemainSec(s)
	reroll := sim.RerollCostQuote(s, m.cfg)
	lines = append(lines, KV("自動刷新", formatETASec(remain)))
	lines = append(lines, KV("下次重抽", fmt.Sprintf("$%s  [r]", human(reroll))))
	focusHint := ""
	if !m.teamFocusRoster {
		focusHint = " · 焦點"
	}
	title := fmt.Sprintf("人才市場 %d%s", len(cands), focusHint)
	return CardIn(CardDefault, w, title, strings.TrimRight(VStack(lines...), "\n"))
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

// clampTeamCursors keeps market/roster cursors in range after hire/fire/reroll.
func clampTeamCursors(m *Model) {
	nc := len(m.state.Market.Candidates)
	if nc == 0 {
		m.marketCursor = 0
	} else if m.marketCursor >= nc {
		m.marketCursor = nc - 1
	} else if m.marketCursor < 0 {
		m.marketCursor = 0
	}
	ne := len(m.state.Employees)
	if ne == 0 {
		m.rosterCursor = 0
	} else if m.rosterCursor >= ne {
		m.rosterCursor = ne - 1
	} else if m.rosterCursor < 0 {
		m.rosterCursor = 0
	}
}

// teamMoveFocus advances market or roster cursor by delta (−1/+1).
func teamMoveFocus(m *Model, delta int) {
	if m.teamFocusRoster {
		n := len(m.state.Employees)
		if n == 0 {
			return
		}
		m.rosterCursor = (m.rosterCursor + delta + n) % n
		return
	}
	n := len(m.state.Market.Candidates)
	if n == 0 {
		return
	}
	m.marketCursor = (m.marketCursor + delta + n) % n
}

// teamToggleFocus switches between market and roster selection panes.
func teamToggleFocus(m *Model) {
	m.teamFocusRoster = !m.teamFocusRoster
	clampTeamCursors(m)
}

// applyTeamHire hires the focused market candidate.
func applyTeamHire(m *Model) {
	clampTeamCursors(m)
	cands := m.state.Market.Candidates
	if len(cands) == 0 || m.marketCursor < 0 || m.marketCursor >= len(cands) {
		m.setNotice("沒有可雇用的候選人")
		return
	}
	id := cands[m.marketCursor].ID
	name := cands[m.marketCursor].Name
	ns, err := sim.Apply(m.state, model.HireEmployee{CandidateID: id}, m.cfg)
	if err != nil {
		m.setNotice(teamCmdErrNotice(err))
		return
	}
	m.state = ns
	clampTeamCursors(m)
	m.setNotice(fmt.Sprintf("已雇用 %s", name))
}

// applyTeamFire fires the focused roster employee.
func applyTeamFire(m *Model) {
	clampTeamCursors(m)
	emps := m.state.Employees
	if len(emps) == 0 || m.rosterCursor < 0 || m.rosterCursor >= len(emps) {
		m.setNotice("沒有可解雇的員工")
		return
	}
	id := emps[m.rosterCursor].ID
	name := emps[m.rosterCursor].Name
	ns, err := sim.Apply(m.state, model.FireEmployee{EmployeeID: id}, m.cfg)
	if err != nil {
		m.setNotice(teamCmdErrNotice(err))
		return
	}
	m.state = ns
	clampTeamCursors(m)
	m.setNotice(fmt.Sprintf("已解雇 %s", name))
}

// applyTeamUpgrade upgrades the office by one level.
func applyTeamUpgrade(m *Model) {
	ns, err := sim.Apply(m.state, model.UpgradeOffice{}, m.cfg)
	if err != nil {
		m.setNotice(teamCmdErrNotice(err))
		return
	}
	m.state = ns
	level := m.state.Office.Level
	name := hqStageNames[hqStageFromOffice(level)]
	m.setNotice(fmt.Sprintf("辦公室升級 → Lv%d %s", level, name))
}

// applyTeamReroll pays to regenerate the talent market pool.
func applyTeamReroll(m *Model) {
	ns, err := sim.Apply(m.state, model.RerollMarket{}, m.cfg)
	if err != nil {
		m.setNotice(teamCmdErrNotice(err))
		return
	}
	m.state = ns
	m.marketCursor = 0
	clampTeamCursors(m)
	m.setNotice("人才市場已重抽")
}

func teamCmdErrNotice(err error) string {
	switch {
	case errors.Is(err, sim.ErrInsufficientCash):
		return "現金不足"
	case errors.Is(err, sim.ErrNoSeats):
		return "工位已滿"
	case errors.Is(err, sim.ErrOfficeMaxed):
		return "辦公室已滿級"
	case errors.Is(err, sim.ErrUnknownCandidate):
		return "找不到該候選人"
	case errors.Is(err, sim.ErrUnknownEmployee):
		return "找不到該員工"
	default:
		return "操作失敗"
	}
}
