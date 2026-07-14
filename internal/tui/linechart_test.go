package tui

import (
	"strings"
	"testing"
)

func TestLineChartEmpty(t *testing.T) {
	if lineChart(nil, 10, 5) != "資料累積中" {
		t.Fatal("empty")
	}
	if lineChart([]float64{1}, 10, 5) != "資料累積中" {
		t.Fatal("single")
	}
}

func TestLineChartMonotone(t *testing.T) {
	out := lineChart([]float64{0, 1, 2, 3}, 4, 4)
	lines := strings.Split(out, "\n")
	if len(lines) != 4 {
		t.Fatalf("rows=%d want 4\n%s", len(lines), out)
	}
	// last column should be tallest: bottom row all filled or last col filled
	bottom := lines[len(lines)-1]
	if !strings.Contains(bottom, "█") {
		t.Fatalf("bottom empty: %q", bottom)
	}
}

func TestLineChartFlat(t *testing.T) {
	out := lineChart([]float64{5, 5, 5}, 3, 3)
	if strings.Contains(out, "資料累積中") {
		t.Fatal("flat should render")
	}
}
