package tui

import (
	"tokensmith/internal/sim"
)

func renderDashboard(m Model) string {
	cw := m.contentWidth()
	if cw < 20 {
		cw = 20
	}
	chartW := cw - 6
	if chartW < 8 {
		chartW = 8
	}
	chartH := 5
	if cw < 100 {
		chartH = 3
	}

	users := sim.TotalUsers(m.state)
	if m.dispReady {
		users = m.disp.TotalUsers
	}
	rev := sim.MonthlyRevenue(m.state)
	rnd := m.state.Resources.RnD

	userBody := VStack(
		KV("總用戶", human(users)),
		styleMuted.Render("近況"),
		styleCyan.Render(lineChart(m.dashUsers.values(), chartW, chartH)),
	)
	revBody := VStack(
		KV("月營收", "$"+human(rev)),
		styleMuted.Render("近況"),
		styleGain.Render(lineChart(m.dashRevenue.values(), chartW, chartH)),
	)
	rndBody := VStack(
		KV("庫存", human(rnd)),
		styleMuted.Render("近況"),
		stylePurple.Render(lineChart(m.dashRnDStock.values(), chartW, chartH)),
	)

	return VStack(
		CardIn(CardDefault, cw, "用戶增長", userBody),
		CardIn(CardDefault, cw, "營收增長", revBody),
		CardIn(CardDefault, cw, "R&D 增長", rndBody),
	)
}
