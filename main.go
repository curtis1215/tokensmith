package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"tokensmith/internal/tui"
)

func main() {
	p := tea.NewProgram(tui.New(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "tokensmith error:", err)
		os.Exit(1)
	}
}
