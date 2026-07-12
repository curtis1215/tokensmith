package tui

func renderWarRoom(m Model) string {
	cw := m.contentWidth()
	gap := 2

	// Events + board: allow up to 6 body lines (spec §5.3).
	var top string
	if cw < minDashWidth {
		top = VStack(
			CardInFrom(campaignStatusContent(m, cw)),
			CardInFrom(rivalRoadmapContent(m, cw)),
		)
	} else {
		colW := (cw - gap) / 2
		top = HRowEqualCards(gap,
			campaignStatusContent(m, colW),
			rivalRoadmapContent(m, colW),
		)
	}

	rows := []string{
		top,
		renderEventsCardMax(m, cw, 6),
		renderBoardReportCardMax(m, cw, 6),
	}
	// Campaign-facing pressures (doctrine/perk/distress) live here, not on overview.
	if warns := campaignPressures(m); len(warns) > 0 {
		rows = append(rows, CardIn(CardThreat, cw, "注意", VStack(warns...)))
	}
	return VStack(rows...)
}
