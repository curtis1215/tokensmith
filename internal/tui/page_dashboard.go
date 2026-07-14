package tui

import (
	"fmt"
	"strings"
	"time"

	"tokensmith/internal/metrics"
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

	days := metrics.SortedDays(m.metricsDoc)
	hasLong := len(days) >= 2
	longEmpty := styleMuted.Render("尚無歷史 · 掛機或跨日後會出現")

	longUsers := metrics.Series(m.metricsDoc, days, func(p metrics.DayPoint) float64 { return p.Users })
	longRev := metrics.Series(m.metricsDoc, days, func(p metrics.DayPoint) float64 { return p.MonthlyRevenue })
	longRnD := metrics.Series(m.metricsDoc, days, func(p metrics.DayPoint) float64 { return p.RnDStock })

	userLong := longEmpty
	revLong := longEmpty
	rndLong := longEmpty
	if hasLong {
		userLong = styleCyan.Render(lineChart(longUsers, chartW, chartH))
		revLong = styleGain.Render(lineChart(longRev, chartW, chartH))
		rndLong = stylePurple.Render(lineChart(longRnD, chartW, chartH))
	}

	// R&D inflow multi-line by SourceOrder (positive booked amounts only).
	inflowSeries := make([][]float64, 0, len(metrics.SourceOrder))
	for _, src := range metrics.SourceOrder {
		src := src
		inflowSeries = append(inflowSeries, metrics.Series(m.metricsDoc, days, func(p metrics.DayPoint) float64 {
			if p.RnDInflow == nil {
				return 0
			}
			return p.RnDInflow[src]
		}))
	}
	inflowChart := longEmpty
	if hasLong {
		inflowChart = stylePurple.Render(multiLineChart(inflowSeries, chartW, chartH))
	}

	dayKey := m.metricsDay
	if dayKey == "" {
		dayKey = metrics.DayKey(time.Now())
	}
	todayPt := m.metricsDoc.Days[dayKey]
	legendParts := make([]string, 0, len(metrics.SourceOrder))
	for _, src := range metrics.SourceOrder {
		amt := 0.0
		if todayPt.RnDInflow != nil {
			amt = todayPt.RnDInflow[src]
		}
		legendParts = append(legendParts, fmt.Sprintf("%s %s", sourceLabel(src), human(amt)))
	}
	todayLegend := styleMuted.Render("今日 " + strings.Join(legendParts, " · "))

	userVal := human(users)
	if d := deltaToday(todayPt.OpenUsers, users, todayPt.OpenSet); d != "" {
		userVal += " " + d
	}
	revVal := "$" + human(rev)
	if d := deltaToday(todayPt.OpenRevenue, rev, todayPt.OpenSet); d != "" {
		revVal += " " + d
	}
	rndVal := human(rnd)
	if d := deltaToday(todayPt.OpenRnD, rnd, todayPt.OpenSet); d != "" {
		rndVal += " " + d
	}

	userBody := VStack(
		KV("總用戶", userVal),
		styleMuted.Render("近況"),
		styleCyan.Render(lineChart(m.dashUsers.values(), chartW, chartH)),
		styleMuted.Render("近 90 日"),
		userLong,
	)
	revBody := VStack(
		KV("月營收", revVal),
		styleMuted.Render("近況"),
		styleGain.Render(lineChart(m.dashRevenue.values(), chartW, chartH)),
		styleMuted.Render("近 90 日"),
		revLong,
	)
	rndBody := VStack(
		KV("庫存", rndVal),
		styleMuted.Render("近況"),
		stylePurple.Render(lineChart(m.dashRnDStock.values(), chartW, chartH)),
		styleMuted.Render("近 90 日"),
		rndLong,
		styleMuted.Render("流入 by 來源"),
		styleMuted.Render("庫存含消耗；流入為正入帳"),
		inflowChart,
		todayLegend,
	)

	return VStack(
		CardIn(CardDefault, cw, "用戶增長", userBody),
		CardIn(CardDefault, cw, "營收增長", revBody),
		CardIn(CardDefault, cw, "R&D 增長", rndBody),
	)
}

// deltaToday formats open-of-day stock delta for dashboard KPI headers.
// Empty when open is not yet set for the day (no snapshot yet).
func deltaToday(open, now float64, openSet bool) string {
	if !openSet {
		return ""
	}
	d := now - open
	sign := "+"
	if d < 0 {
		sign = "-"
		d = -d
	}
	return fmt.Sprintf("(%s%s 今日)", sign, human(d))
}
