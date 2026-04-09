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
	"github.com/cassiomarques/gh-bell/internal/search"
	"github.com/cassiomarques/gh-bell/internal/service"
	"github.com/cassiomarques/gh-bell/internal/storage"
	"github.com/cassiomarques/gh-bell/internal/tui"
)

func setupLogging() (*os.File, error) {
	dir, err := storage.DataDir()
	if err != nil {
		dir, _ = os.UserHomeDir()
		if dir == "" {
			dir = os.TempDir()
		}
	}
	// Ensure data directory exists
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	logPath := filepath.Join(dir, "gh-bell.log")
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

	// Initialize SQLite store for local persistence
	dbPath, err := storage.DefaultDBPath()
	if err != nil {
		log.Printf("warning: could not determine DB path: %v", err)
		fmt.Fprintf(os.Stderr, "Warning: local cache disabled: %v\n", err)
	}

	var svc *service.NotificationService
	if dbPath != "" {
		store, err := storage.Open(dbPath)
		if err != nil {
			log.Printf("warning: could not open database at %s: %v", dbPath, err)
			fmt.Fprintf(os.Stderr, "Warning: local cache disabled: %v\n", err)
		} else {
			svc = service.New(client, store)
			defer svc.Close()
			log.Printf("SQLite store opened at %s", dbPath)

			// Initialize Bleve search index
			dataDir, _ := storage.DataDir()
			indexPath := filepath.Join(dataDir, "search.bleve")
			searchIdx, err := search.Open(indexPath)
			if err != nil {
				log.Printf("warning: could not open search index: %v", err)
			} else {
				svc.SetSearch(searchIdx)
				log.Printf("Bleve search index opened at %s", indexPath)
			}
		}
	}

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

	opts := []tui.Option{tui.WithRefreshInterval(refreshInterval)}
	if svc != nil {
		opts = append(opts, tui.WithService(svc))
	}
	app := tui.NewApp(client, opts...)
	p := tea.NewProgram(app)

	log.Println("starting TUI")
	if _, err := p.Run(); err != nil {
		log.Printf("TUI error: %v", err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	log.Println("gh-bell exited normally")
}
