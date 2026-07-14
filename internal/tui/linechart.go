package tui

import (
	"math"
	"strings"
)

// lineChartRunes mirrors sparkRunes; kept local to avoid coupling.
var lineChartRunes = []rune("▁▂▃▄▅▆▇█")

const lineChartFill = '█'
const lineChartEmpty = ' '
const lineChartPlaceholder = "資料累積中"

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
// Returns 資料累積中 when no series has at least 2 points.
func multiLineChart(series [][]float64, width, height int) string {
	width, height = clampChartDims(width, height)
	prepared := make([][]float64, 0, len(series))
	maxLen := 0
	for _, s := range series {
		if len(s) < 2 {
			continue
		}
		if len(s) > width {
			s = s[len(s)-width:]
		}
		if len(s) > maxLen {
			maxLen = len(s)
		}
		prepared = append(prepared, s)
	}
	if len(prepared) == 0 || maxLen < 2 {
		return lineChartPlaceholder
	}

	lo, hi := prepared[0][0], prepared[0][0]
	for _, s := range prepared {
		sLo, sHi := minMax(s)
		if sLo < lo {
			lo = sLo
		}
		if sHi > hi {
			hi = sHi
		}
	}

	// grid[row][col]; row 0 = top
	grid := make([][]rune, height)
	for r := range grid {
		grid[r] = make([]rune, maxLen)
		for c := range grid[r] {
			grid[r][c] = lineChartEmpty
		}
	}

	for _, s := range prepared {
		// Right-align shorter series into maxLen columns.
		offset := maxLen - len(s)
		hs := columnHeights(s, lo, hi, height)
		for i, h := range hs {
			col := offset + i
			for row := 0; row < height; row++ {
				// row 0 is top → bottom index = height-1-row
				bottomIdx := height - 1 - row
				if h > bottomIdx {
					grid[row][col] = lineChartFill
				}
			}
		}
	}

	lines := make([]string, height)
	for r := 0; r < height; r++ {
		lines[r] = string(grid[r])
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
