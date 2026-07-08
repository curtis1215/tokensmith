package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"tokensmith/internal/tui"
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v" || os.Args[1] == "version") {
		fmt.Println("tokensmith", version)
		return
	}
	p := tea.NewProgram(tui.New(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "tokensmith error:", err)
		os.Exit(1)
	}
}
