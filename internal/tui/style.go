package tui

import "github.com/charmbracelet/lipgloss"

// 舊樣式名 rebind 到 theme.go 的 HUD 調色盤，呼叫端逐步遷移。
var (
	styleAccent    = styleCyan
	styleWarn      = styleLoss
	styleMuted     = lipgloss.NewStyle().Faint(true)
	styleTitle     = lipgloss.NewStyle().Bold(true).Foreground(colorCyan)
	styleTabActive = lipgloss.NewStyle().Bold(true).Foreground(colorCyan).Underline(true)

	titleStyle     = styleTitle
	boxStyle       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorDim).Padding(0, 1)
	helpStyle      = styleMuted
	tabActiveStyle = styleTabActive
)
