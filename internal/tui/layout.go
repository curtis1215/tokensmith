package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Card renders a styled box with a title and body.
func Card(title, body string) string {
	return CardIn(CardDefault, 0, title, body)
}

// cardFrameWidth is horizontal chrome of boxStyle: border + padding on each side.
const cardFrameWidth = 4

const minDashWidth = 80

// Grid lays cells out in two equal-width columns; below minDashWidth it
// stacks vertically with full-width cells. An odd trailing cell gets full width.
func Grid(cw, gap int, cells ...func(w int) string) string {
	if len(cells) == 0 {
		return ""
	}
	if cw < minDashWidth {
		parts := make([]string, len(cells))
		for i, c := range cells {
			parts[i] = c(cw)
		}
		return VStack(parts...)
	}
	colW := (cw - gap) / 2
	var rows []string
	for i := 0; i < len(cells); i += 2 {
		if i+1 < len(cells) {
			rows = append(rows, HRow(gap, cells[i](colW), cells[i+1](colW)))
		} else {
			rows = append(rows, cells[i](cw))
		}
	}
	return VStack(rows...)
}

// ResponsiveRow joins parts horizontally with a gap if width >= minDashWidth and the horizontal row width does not exceed the available width.
// Otherwise, it stacks them vertically using VStack.
func ResponsiveRow(width int, gap int, parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}
	if width < minDashWidth {
		return VStack(parts...)
	}
	hrow := HRow(gap, parts...)
	if lipgloss.Width(hrow) > width {
		return VStack(parts...)
	}
	return hrow
}

// DashRow is an alias for ResponsiveRow.
func DashRow(width int, gap int, parts ...string) string {
	return ResponsiveRow(width, gap, parts...)
}

// HRow joins multiple parts horizontally with a gap.
func HRow(gap int, parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}
	sep := strings.Repeat(" ", gap)
	return lipgloss.JoinHorizontal(lipgloss.Top, joinWithSep(parts, sep)...)
}

// joinWithSep is a helper to interleave a slice of strings with a separator.
func joinWithSep(parts []string, sep string) []string {
	var result []string
	for i, part := range parts {
		result = append(result, part)
		if i < len(parts)-1 {
			result = append(result, sep)
		}
	}
	return result
}

// VStack joins multiple parts vertically with left alignment.
func VStack(parts ...string) string {
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// KV renders a label and value key-value row.
func KV(label, value string) string {
	return label + ": " + value
}

// Bar renders the default cyan→purple gradient progress bar.
func Bar(frac float64, width int) string {
	return gradientBar(frac, width, "#00D7FF", "#B48CFF")
}

// Footer renders a unified page-level footer.
// When pageKeys is empty (dialog open), omit shell [Tab]/[q] so help matches
// dialog-only key handling.
func Footer(pageKeys string) string {
	if pageKeys == "" {
		return helpStyle.Render("[Esc]取消  [Enter]確認")
	}
	return helpStyle.Render(pageKeys + "  [Tab]切頁 [q]離開")
}

// Truncate truncates a string to a maximum number of runes.
func Truncate(s string, maxRunes int) string {
	r := []rune(s)
	if maxRunes < 0 || len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes])
}

// TruncateWidth truncates s to at most max display cells (ANSI/CJK-aware via lipgloss).
func TruncateWidth(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= max {
		return s
	}
	r := []rune(s)
	for len(r) > 0 {
		r = r[:len(r)-1]
		if lipgloss.Width(string(r)) <= max {
			return string(r)
		}
	}
	return ""
}


