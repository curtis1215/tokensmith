package tui

import (
	"strings"
	"testing"

	"tokensmith/internal/model"
)

func TestDashRingsSampleStocks(t *testing.T) {
	m := testModel(t)
	m.state.Models = []model.Model{{Online: true, Users: 1000, Price: 10}}
	m.state.Resources.RnD = 500
	m.dispReady = true
	m.disp.TotalUsers = 1000
	// Drive the real display path: every 4 ticks samples once (need ≥2 samples).
	for i := 0; i < 9; i++ {
		m.advanceDisplay()
	}
	if m.dashUsers.n < 2 {
		t.Fatalf("dashUsers n=%d", m.dashUsers.n)
	}
	if m.dashRevenue.n < 1 || m.dashRnDStock.n < 1 {
		t.Fatal("revenue/rnd not sampled")
	}
}

func TestDashboardShortChartAfterSamples(t *testing.T) {
	m := testModel(t)
	for _, v := range []float64{1, 2, 3, 4, 5} {
		m.dashUsers.push(v)
		m.dashRevenue.push(v * 10)
		m.dashRnDStock.push(v * 100)
	}
	m.page = PageDashboard
	m.width, m.height = 120, 50
	m.resize(m.width, m.height)
	body := renderDashboard(m)
	if !strings.Contains(body, "近況") {
		t.Fatalf("missing 近況 label:\n%s", body)
	}
	if strings.Contains(body, "資料累積中") && !strings.Contains(body, "█") {
		t.Fatalf("expected chart blocks:\n%s", body)
	}
}
