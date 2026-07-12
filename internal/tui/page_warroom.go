package tui

func renderWarRoom(m Model) string {
	cw := m.contentWidth()
	gap := 2

	// Events + board: allow up to 6 body lines (spec §5.3).
	var top string
	if cw < minDashWidth {
		top = VStack(
			renderCampaignStatusCard(m, cw),
			renderRivalRoadmapCard(m, cw),
		)
	} else {
		colW := (cw - gap) / 2
		top = HRowEqual(gap,
			renderCampaignStatusCard(m, colW),
			renderRivalRoadmapCard(m, colW),
		)
	}

	return VStack(
		top,
		renderEventsCardMax(m, cw, 6),
		renderBoardReportCardMax(m, cw, 6),
	)
}
