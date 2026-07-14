package tui

import (
	"strings"
	"testing"

	"tokensmith/internal/metrics"
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

func TestDashboardLongWindowUsesDoc(t *testing.T) {
	m := testModel(t)
	doc := metrics.EmptyDocument()
	metrics.UpsertSnapshot(&doc, "2026-07-10", 10, 100, 1, 1)
	metrics.UpsertSnapshot(&doc, "2026-07-11", 20, 200, 2, 2)
	metrics.UpsertSnapshot(&doc, "2026-07-12", 40, 400, 3, 3)
	metrics.AddInflow(&doc, "2026-07-12", "claude-code", 12, 3)
	metrics.AddInflow(&doc, "2026-07-12", metrics.SourceStaff, 4, 3)
	m.metricsDoc = doc
	m.metricsDay = "2026-07-12"
	m.width, m.height = 120, 50
	m.resize(m.width, m.height)

	body := renderDashboard(m)
	for _, want := range []string{"近況", "近 90 日", "流入 by 來源", "庫存含消耗", "員工"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "尚無歷史") {
		t.Fatalf("should not show empty long-window copy with ≥2 days:\n%s", body)
	}
}

func TestDashboardLongWindowEmptyCopy(t *testing.T) {
	m := testModel(t)
	m.metricsDoc = metrics.EmptyDocument()
	m.width, m.height = 120, 50
	m.resize(m.width, m.height)
	body := renderDashboard(m)
	if !strings.Contains(body, "近 90 日") {
		t.Fatalf("missing long section title:\n%s", body)
	}
	if !strings.Contains(body, "尚無歷史") {
		t.Fatalf("expected empty long-window copy:\n%s", body)
	}
}

func TestDeltaTodayFormatting(t *testing.T) {
	if got := deltaToday(100, 200, false); got != "" {
		t.Fatalf("openSet=false want empty, got %q", got)
	}
	if got := deltaToday(1000, 2200, true); got != "(+1k 今日)" {
		t.Fatalf("positive delta: got %q", got)
	}
	if got := deltaToday(1000, 500, true); got != "(-500 今日)" {
		t.Fatalf("negative delta: got %q", got)
	}
	if got := deltaToday(100, 100, true); got != "(+0 今日)" {
		t.Fatalf("zero delta: got %q", got)
	}
	// Large negative should still use human compact form via abs.
	if got := deltaToday(3000, 1000, true); got != "(-2k 今日)" {
		t.Fatalf("large negative: got %q", got)
	}
}

func TestDashboardEmptyHistoryCopy(t *testing.T) {
	m := testModel(t)
	m.metricsDoc = metrics.EmptyDocument()
	body := renderDashboard(m)
	if !strings.Contains(body, "尚無歷史") {
		t.Fatalf("want empty history copy:\n%s", body)
	}
	if !strings.Contains(body, "掛機或跨日後會出現") {
		t.Fatalf("want full empty-history phrase:\n%s", body)
	}
}

func TestDashboardDeltaTodayOnStockCards(t *testing.T) {
	m := testModel(t)
	doc := metrics.EmptyDocument()
	// Open freezes at first upsert: users=1000, rev=100, rnd=50.
	metrics.UpsertSnapshot(&doc, "2026-07-14", 1000, 100, 50, 1)
	metrics.UpsertSnapshot(&doc, "2026-07-14", 1500, 200, 40, 2)
	m.metricsDoc = doc
	m.metricsDay = "2026-07-14"
	// Live stocks differ from open-of-day.
	m.state.Models = []model.Model{{Online: true, Users: 2200, Price: 10}} // rev = 22000
	m.state.Resources.RnD = 80
	m.width, m.height = 120, 50
	m.resize(m.width, m.height)

	body := renderDashboard(m)
	// users: 2200-1000 = +1k; rev: 22000-100 = +22k; rnd: 80-50 = +30
	for _, want := range []string{"(+1k 今日)", "(+22k 今日)", "(+30 今日)"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing stock Δ今日 %q:\n%s", want, body)
		}
	}
}
