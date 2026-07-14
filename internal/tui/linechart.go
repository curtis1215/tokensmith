package tui

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// lineChartRunes mirrors sparkRunes; kept local to avoid coupling.
var lineChartRunes = []rune("▁▂▃▄▅▆▇█")

const lineChartFill = '█'
const lineChartEmpty = ' '
const lineChartPlaceholder = "資料累積中"

// multiLineFills provides monochrome-distinguishable glyphs per series
// when colors are unavailable; cycles if more series than fills.
var multiLineFills = []rune{'█', '▓', '▒', '░', '▄'}

// lineChart renders a multi-row unicode bar chart from vals.
// Fewer than 2 points returns 資料累積中.
func lineChart(vals []float64, width, height int) string {
	width, height = clampChartDims(width, height)
	if len(vals) < 2 {
		return lineChartPlaceholder
	}
	if len(vals) > width {
		vals = vals[len(vals)-width:]
	}
	lo, hi := minMax(vals)
	heights := columnHeights(vals, lo, hi, height)
	return renderHeights(heights, height)
}

// multiLineChart renders multiple series on a shared Y scale.
// Later series overpaint earlier ones on the same cell.
// Each series uses styles[i%len(styles)] when styles is non-empty;
// fill runes cycle multiLineFills for monochrome fallback.
// Returns 資料累積中 when no series has at least 2 points.
func multiLineChart(series [][]float64, width, height int, styles []lipgloss.Style) string {
	width, height = clampChartDims(width, height)

	type painted struct {
		vals  []float64
		style lipgloss.Style
		fill  rune
		useSt bool
	}
	prepared := make([]painted, 0, len(series))
	maxLen := 0
	for i, s := range series {
		if len(s) < 2 {
			continue
		}
		if len(s) > width {
			s = s[len(s)-width:]
		}
		if len(s) > maxLen {
			maxLen = len(s)
		}
		p := painted{
			vals: s,
			fill: multiLineFills[i%len(multiLineFills)],
		}
		if len(styles) > 0 {
			p.style = styles[i%len(styles)]
			p.useSt = true
		}
		prepared = append(prepared, p)
	}
	if len(prepared) == 0 || maxLen < 2 {
		return lineChartPlaceholder
	}

	lo, hi := prepared[0].vals[0], prepared[0].vals[0]
	for _, s := range prepared {
		sLo, sHi := minMax(s.vals)
		if sLo < lo {
			lo = sLo
		}
		if sHi > hi {
			hi = sHi
		}
	}

	// grid cell: fill rune + index into prepared (-1 = empty)
	type cell struct {
		ch rune
		si int
	}
	grid := make([][]cell, height)
	for r := range grid {
		grid[r] = make([]cell, maxLen)
		for c := range grid[r] {
			grid[r][c] = cell{ch: lineChartEmpty, si: -1}
		}
	}

	for si, s := range prepared {
		// Right-align shorter series into maxLen columns.
		offset := maxLen - len(s.vals)
		hs := columnHeights(s.vals, lo, hi, height)
		for i, h := range hs {
			col := offset + i
			for row := 0; row < height; row++ {
				// row 0 is top → bottom index = height-1-row
				bottomIdx := height - 1 - row
				if h > bottomIdx {
					grid[row][col] = cell{ch: s.fill, si: si}
				}
			}
		}
	}

	lines := make([]string, height)
	for r := 0; r < height; r++ {
		var b strings.Builder
		c := 0
		for c < maxLen {
			cl := grid[r][c]
			if cl.si < 0 {
				b.WriteRune(lineChartEmpty)
				c++
				continue
			}
			// Contiguous run with same series style index.
			start := c
			for c < maxLen && grid[r][c].si == cl.si {
				c++
			}
			run := make([]rune, c-start)
			for i := start; i < c; i++ {
				run[i-start] = grid[r][i].ch
			}
			text := string(run)
			if prepared[cl.si].useSt {
				text = prepared[cl.si].style.Render(text)
			}
			b.WriteString(text)
		}
		lines[r] = b.String()
	}
	return strings.Join(lines, "\n")
}

func clampChartDims(width, height int) (int, int) {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	return width, height
}

func minMax(vals []float64) (lo, hi float64) {
	lo, hi = vals[0], vals[0]
	for _, v := range vals[1:] {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	return lo, hi
}

// columnHeights returns bar heights in 0..height for each value.
func columnHeights(vals []float64, lo, hi float64, height int) []int {
	out := make([]int, len(vals))
	if hi <= lo {
		mid := height / 2
		for i := range out {
			out[i] = mid
		}
		return out
	}
	span := hi - lo
	for i, v := range vals {
		h := int(math.Round((v - lo) / span * float64(height)))
		if h < 0 {
			h = 0
		}
		if h > height {
			h = height
		}
		out[i] = h
	}
	return out
}

func renderHeights(heights []int, height int) string {
	n := len(heights)
	lines := make([]string, height)
	for row := 0; row < height; row++ {
		// row 0 = top of chart
		bottomIdx := height - 1 - row
		buf := make([]rune, n)
		for col, h := range heights {
			if h > bottomIdx {
				buf[col] = lineChartFill
			} else {
				buf[col] = lineChartEmpty
			}
		}
		lines[row] = string(buf)
	}
	return strings.Join(lines, "\n")
}
