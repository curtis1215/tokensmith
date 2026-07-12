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
	return GridN(cw, gap, 2, cells...)
}

// padBodyLines ensures body has exactly n lines (pad with "" or truncate from end).
func padBodyLines(body string, n int) string {
	if n <= 0 {
		return ""
	}
	lines := strings.Split(body, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	for len(lines) < n {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// EqualHeight pads each part with trailing "\n" so lipgloss.Height matches the tallest.
// Note: padding finished bordered cards only extends the block below the border.
// For equal *card borders*, use HRowEqualCards (pad body before CardIn).
func EqualHeight(parts ...string) []string {
	maxH := 0
	for _, p := range parts {
		if h := lipgloss.Height(p); h > maxH {
			maxH = h
		}
	}
	out := make([]string, len(parts))
	for i, p := range parts {
		for lipgloss.Height(p) < maxH {
			p += "\n"
		}
		out[i] = p
	}
	return out
}

// HRowEqual runs EqualHeight then HRow(gap, ...). Prefer HRowEqualCards for bordered cards.
func HRowEqual(gap int, parts ...string) string {
	return HRow(gap, EqualHeight(parts...)...)
}

// cardContent is the pre-border pieces of a card so bodies can be height-equalized.
type cardContent struct {
	kind  CardKind
	w     int
	title string
	body  string
}

func bodyLineCount(body string) int {
	if body == "" {
		return 1
	}
	return lipgloss.Height(body)
}

// HRowEqualCards renders cards with equal visual height after width wrapping.
// It measures each CardIn height (so wrapped lines count), then pads shorter
// bodies with trailing newlines until all cards share the same lipgloss.Height.
func HRowEqualCards(gap int, cards ...cardContent) string {
	if len(cards) == 0 {
		return ""
	}
	if len(cards) == 1 {
		c := cards[0]
		return CardIn(c.kind, c.w, c.title, c.body)
	}
	maxH := 0
	for _, c := range cards {
		if h := lipgloss.Height(CardIn(c.kind, c.w, c.title, c.body)); h > maxH {
			maxH = h
		}
	}
	parts := make([]string, len(cards))
	for i, c := range cards {
		body := c.body
		// Pad by actual rendered height so wrap-induced lines are accounted for.
		for tries := 0; lipgloss.Height(CardIn(c.kind, c.w, c.title, body)) < maxH && tries < 256; tries++ {
			body += "\n"
		}
		parts[i] = CardIn(c.kind, c.w, c.title, body)
	}
	return HRow(gap, parts...)
}

// gridColWidths splits cw across n columns with (n-1)*gap gutters.
// Remainder cells go to the trailing columns so the row width equals cw.
func gridColWidths(cw, gap, n int) []int {
	if n < 1 {
		return nil
	}
	if n == 1 {
		return []int{cw}
	}
	inner := cw - gap*(n-1)
	base := inner / n
	if base < 1 {
		base = 1
	}
	// Recompute with floor so trailing columns absorb leftover cells.
	rem := cw - gap*(n-1) - base*(n-1)
	if rem < 1 {
		rem = 1
	}
	ws := make([]int, n)
	for i := 0; i < n-1; i++ {
		ws[i] = base
	}
	ws[n-1] = rem
	return ws
}

// GridN lays cells in `cols` equal-width columns.
// Below minDashWidth stacks full-width. Odd trailing cells span full width.
func GridN(cw, gap, cols int, cells ...func(w int) string) string {
	if len(cells) == 0 {
		return ""
	}
	if cols < 1 {
		cols = 1
	}
	if cw < minDashWidth || cols == 1 {
		parts := make([]string, len(cells))
		for i, c := range cells {
			parts[i] = c(cw)
		}
		return VStack(parts...)
	}
	var rows []string
	for i := 0; i < len(cells); i += cols {
		end := i + cols
		if end > len(cells) {
			end = len(cells)
		}
		chunk := cells[i:end]
		if len(chunk) < cols {
			// trailing incomplete row: if single cell, full width; else equal split among remaining
			if len(chunk) == 1 {
				rows = append(rows, chunk[0](cw))
				continue
			}
			widths := gridColWidths(cw, gap, len(chunk))
			parts := make([]string, len(chunk))
			for j, c := range chunk {
				parts[j] = c(widths[j])
			}
			// Finished-card EqualHeight: borders may still differ; prefer GridNCards.
			rows = append(rows, HRowEqual(gap, parts...))
			continue
		}
		widths := gridColWidths(cw, gap, cols)
		parts := make([]string, cols)
		for j, c := range chunk {
			parts[j] = c(widths[j])
		}
		rows = append(rows, HRowEqual(gap, parts...))
	}
	return VStack(rows...)
}

// GridNCards is GridN for cardContent cells so equal rows get true border-equal height.
func GridNCards(cw, gap, cols int, cells ...func(w int) cardContent) string {
	if len(cells) == 0 {
		return ""
	}
	if cols < 1 {
		cols = 1
	}
	if cw < minDashWidth || cols == 1 {
		parts := make([]string, len(cells))
		for i, c := range cells {
			cc := c(cw)
			parts[i] = CardIn(cc.kind, cc.w, cc.title, cc.body)
		}
		return VStack(parts...)
	}
	var rows []string
	for i := 0; i < len(cells); i += cols {
		end := i + cols
		if end > len(cells) {
			end = len(cells)
		}
		chunk := cells[i:end]
		if len(chunk) < cols {
			if len(chunk) == 1 {
				cc := chunk[0](cw)
				rows = append(rows, CardIn(cc.kind, cc.w, cc.title, cc.body))
				continue
			}
			widths := gridColWidths(cw, gap, len(chunk))
			contents := make([]cardContent, len(chunk))
			for j, c := range chunk {
				contents[j] = c(widths[j])
			}
			rows = append(rows, HRowEqualCards(gap, contents...))
			continue
		}
		widths := gridColWidths(cw, gap, cols)
		contents := make([]cardContent, cols)
		for j, c := range chunk {
			contents[j] = c(widths[j])
		}
		rows = append(rows, HRowEqualCards(gap, contents...))
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
