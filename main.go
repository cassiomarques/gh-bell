package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/cassiomarques/gh-bell/internal/github"
	"github.com/cassiomarques/gh-bell/internal/tui"
)

func main() {
	client, err := github.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	app := tui.NewApp(client)
	p := tea.NewProgram(app)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
