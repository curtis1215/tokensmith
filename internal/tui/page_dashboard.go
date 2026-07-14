package tui

func renderDashboard(m Model) string {
	cw := m.contentWidth()
	if cw < 20 {
		cw = 20
	}
	return VStack(
		CardIn(CardDefault, cw, "用戶增長", styleMuted.Render("資料累積中")),
		CardIn(CardDefault, cw, "營收增長", styleMuted.Render("資料累積中")),
		CardIn(CardDefault, cw, "R&D 增長", styleMuted.Render("資料累積中")),
	)
}
