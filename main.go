package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/cassiomarques/gh-bell/internal/config"
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

	log.Println("loading configuration")
	cfg, err := config.Load()
	if err != nil {
		log.Printf("warning: config load error: %v", err)
	}

	log.Println("creating GitHub API client")
	if cfg.Token != "" {
		log.Println("using configured token (classic PAT)")
	} else {
		log.Println("using default gh auth token")
	}
	client, err := github.NewClient(cfg.Token)
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

	// Apply refresh interval from config
	var refreshInterval time.Duration
	if cfg.RefreshInterval > 0 {
		refreshInterval = time.Duration(cfg.RefreshInterval) * time.Second
		log.Printf("refresh interval set to %s", refreshInterval)
	}

	opts := []tui.Option{tui.WithRefreshInterval(refreshInterval)}
	if svc != nil {
		opts = append(opts, tui.WithService(svc))
	}

	// Pass log file path so the TUI can tail it in the log pane
	logPath := ""
	if dir, err := storage.DataDir(); err == nil {
		logPath = filepath.Join(dir, "gh-bell.log")
	}
	if logPath != "" {
		opts = append(opts, tui.WithLogFile(logPath))
	}

	// Pass cleanup config
	if cfg.CleanupDays > 0 {
		opts = append(opts, tui.WithCleanupDays(cfg.CleanupDays))
		log.Printf("auto-cleanup set to %d days", cfg.CleanupDays)
	}

	// Pass group-by-repo config
	if cfg.GroupByRepo {
		opts = append(opts, tui.WithGroupByRepo(true))
		log.Printf("group-by-repo enabled")
	}

	// Pass sort mode config (default is smart)
	if cfg.SortMode == "chronological" {
		opts = append(opts, tui.WithSmartSort(false))
		log.Printf("sort mode: chronological")
	} else {
		log.Printf("sort mode: smart")
	}

	// Pass preview comment-first config (default is true)
	if cfg.PreviewCommentFirst != nil && !*cfg.PreviewCommentFirst {
		opts = append(opts, tui.WithPreviewCommentFirst(false))
		log.Printf("preview: description first")
	} else {
		log.Printf("preview: comment first (default)")
	}

	// Pass pinned repos config
	if len(cfg.PinnedRepos) > 0 {
		opts = append(opts, tui.WithPinnedRepos(cfg.PinnedRepos))
		log.Printf("pinned repos: %v", cfg.PinnedRepos)
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
