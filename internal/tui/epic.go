// internal/tui/epic.go
package tui

import "github.com/charmbracelet/lipgloss"

// renderEpicOverlay fills the content region with a centered gold celebration.
func renderEpicOverlay(mo Moment, m Model) string {
	inner := VStack(
		"",
		styleGold.Bold(true).Render(mo.Text),
		"",
		styleMuted.Render("按任意鍵繼續"),
		"",
	)
	card := CardIn(CardGold, 0, "🏆 榮耀時刻", inner)
	h := m.vp.Height
	if h < lipgloss.Height(card) {
		h = lipgloss.Height(card)
	}
	return lipgloss.Place(m.contentWidth(), h, lipgloss.Center, lipgloss.Center, card)
}
