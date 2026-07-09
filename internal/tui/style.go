package tui

import "github.com/charmbracelet/lipgloss"

var (
	styleAccent    = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	styleWarn      = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleMuted     = lipgloss.NewStyle().Faint(true)
	styleTitle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	styleTabActive = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Underline(true)

	// Existing styles in tui.go re-exported here to avoid duplication
	titleStyle     = styleTitle
	boxStyle       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	helpStyle      = styleMuted
	tabActiveStyle = styleTabActive
)
