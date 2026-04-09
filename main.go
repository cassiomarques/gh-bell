package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/cassiomarques/gh-bell/internal/github"
	"github.com/cassiomarques/gh-bell/internal/tui"
)

func setupLogging() (*os.File, error) {
	dir, err := os.UserHomeDir()
	if err != nil {
		dir = os.TempDir()
	}
	logPath := filepath.Join(dir, ".gh-bell.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, err
	}
	log.SetOutput(f)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Println("gh-bell starting")
	return f, nil
}

func main() {
	logFile, err := setupLogging()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not set up logging: %v\n", err)
	} else {
		defer logFile.Close()
	}

	log.Println("creating GitHub API client")
	if os.Getenv("GH_BELL_TOKEN") != "" {
		log.Println("using GH_BELL_TOKEN (classic PAT)")
	} else {
		log.Println("using default gh auth token")
	}
	client, err := github.NewClient()
	if err != nil {
		log.Printf("client creation failed: %v", err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	log.Println("client created successfully")

	// Parse optional refresh interval from GH_BELL_REFRESH (seconds)
	var refreshInterval time.Duration
	if s := os.Getenv("GH_BELL_REFRESH"); s != "" {
		if secs, err := strconv.Atoi(s); err == nil && secs > 0 {
			refreshInterval = time.Duration(secs) * time.Second
			log.Printf("refresh interval set to %s (from GH_BELL_REFRESH)", refreshInterval)
		} else {
			log.Printf("ignoring invalid GH_BELL_REFRESH=%q, using default", s)
		}
	}

	app := tui.NewApp(client, tui.WithRefreshInterval(refreshInterval))
	p := tea.NewProgram(app)

	log.Println("starting TUI")
	if _, err := p.Run(); err != nil {
		log.Printf("TUI error: %v", err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	log.Println("gh-bell exited normally")
}
