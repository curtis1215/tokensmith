package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Card renders a styled box with a title and body.
func Card(title, body string) string {
	inner := styleTitle.Render(title) + "\n" + body
	return boxStyle.Render(inner)
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

// Bar renders a progress bar of a given width for a fraction.
func Bar(frac float64, width int) string {
	return progressBar(frac, width)
}

// Footer renders a unified page-level footer.
func Footer(pageKeys string) string {
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

// progressBar renders a fixed-width ▓/░ bar for frac in [0,1].
func progressBar(frac float64, width int) string {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	n := int(frac * float64(width))
	return strings.Repeat("▓", n) + strings.Repeat("░", width-n)
}
