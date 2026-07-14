package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
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

func TestMultiLineChartPerSeriesStyles(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	s1 := lipgloss.NewStyle().Foreground(lipgloss.Color("#00D7FF"))
	s2 := lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B"))
	// Two rising series with different magnitudes so both paint visible cells.
	series := [][]float64{
		{1, 2, 3, 4},
		{0.5, 1, 1.5, 2},
	}
	out := multiLineChart(series, 4, 4, []lipgloss.Style{s1, s2})
	if strings.Contains(out, "資料累積中") {
		t.Fatalf("expected chart, got placeholder: %q", out)
	}
	// Distinct fill runes (monochrome fallback) for series 0 vs 1.
	if !strings.Contains(out, "█") || !strings.Contains(out, "▓") {
		t.Fatalf("expected distinct fill runes █ and ▓, got %q", out)
	}
	// TrueColor styles should emit ANSI CSI sequences.
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected ANSI CSI styling, got %q", out)
	}
	plain := multiLineChart(series, 4, 4, nil)
	if out == plain {
		t.Fatal("styled multi-line chart should differ from unstyled")
	}
	styled0 := s1.Render(string(multiLineFills[0])) // █
	styled1 := s2.Render(string(multiLineFills[1])) // ▓
	if styled0 == styled1 {
		t.Fatal("test styles must produce non-identical ANSI")
	}
	if !strings.Contains(out, styled0) {
		t.Fatalf("missing series-0 styled fill %q in output: %q", styled0, out)
	}
	if !strings.Contains(out, styled1) {
		t.Fatalf("missing series-1 styled fill %q in output: %q", styled1, out)
	}
}
