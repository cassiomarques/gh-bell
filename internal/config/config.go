package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

const defaultCleanupDays = 15

const defaultConfigTemplate = `# gh-bell configuration
# See https://github.com/cassiomarques/gh-bell for documentation.

# GitHub classic Personal Access Token (required).
# Create one at https://github.com/settings/tokens/new
# with 'notifications' and 'repo' scopes.
token: ""

# Auto-refresh interval in seconds (default: 60).
# refresh_interval: 60

# Auto-cleanup: remove read notifications older than N days (default: 15).
# Set to 0 to disable cleanup.
# cleanup_days: 15

# Group notifications by repository (default: false).
# When enabled, notifications are visually grouped under repository headers
# instead of a flat chronological list. Groups are sorted by most recent item.
# group_by_repo: false

# Sort mode: "smart" (priority score) or "chronological" (updated_at).
# Smart mode surfaces actionable items first using exponential time decay.
# sort_mode: smart

# Preview order: when true (default), show the latest comment before the
# description in the preview pane. This puts the most relevant content
# (what triggered the notification) at the top.
# preview_comment_first: true

# Pinned repositories: these repos always appear at the top of the list
# (when group_by_repo is enabled) in the order listed here.
# pinned_repos:
#   - owner/repo-name
#   - owner/another-repo

# Auto-read closed: when enabled, notifications for closed (not merged)
# issues/PRs are automatically marked as read and hidden from the
# Unread tab. Merged PRs remain visible. (default: false)
# auto_read_closed: false
`

// Config holds all gh-bell configuration.
type Config struct {
	Token               string `yaml:"token"`
	RefreshInterval     int    `yaml:"refresh_interval,omitempty"`
	CleanupDays         int    `yaml:"cleanup_days,omitempty"`
	GroupByRepo         bool   `yaml:"group_by_repo,omitempty"`
	SortMode            string `yaml:"sort_mode,omitempty"`            // "smart" or "chronological"
	PreviewCommentFirst *bool    `yaml:"preview_comment_first,omitempty"` // show latest comment before description (default: true)
	PinnedRepos         []string `yaml:"pinned_repos,omitempty"`          // repos pinned to top of grouped list
	AutoReadClosed      bool     `yaml:"auto_read_closed,omitempty"`      // auto-mark closed (not merged) notifications as read
}

// Dir returns the gh-bell data/config directory (~/.gh-bell/).
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determining home directory: %w", err)
	}
	return filepath.Join(home, ".gh-bell"), nil
}

// Path returns the full path to the config file.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Load reads configuration from ~/.gh-bell/config.yaml, then overlays
// environment variables. Env vars always take precedence over the file.
// If the config file doesn't exist, it creates one with commented defaults.
func Load() (*Config, error) {
	cfg := &Config{}

	cfgPath, err := Path()
	if err != nil {
		return cfg, err
	}

	data, err := os.ReadFile(cfgPath)
	if os.IsNotExist(err) {
		if writeErr := createDefault(cfgPath); writeErr != nil {
			log.Printf("warning: could not create default config at %s: %v", cfgPath, writeErr)
		}
	} else if err != nil {
		return cfg, fmt.Errorf("reading config file: %w", err)
	} else {
		if parseErr := parseConfig(data, cfg); parseErr != nil {
			return cfg, parseErr
		}
	}

	applyEnvOverrides(cfg)

	// Apply defaults for fields not set by file or env
	if cfg.CleanupDays == 0 {
		cfg.CleanupDays = defaultCleanupDays
	}

	return cfg, nil
}

// parseConfig unmarshals YAML data into the Config struct.
func parseConfig(data []byte, cfg *Config) error {
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("parsing config file: %w", err)
	}
	return nil
}

// applyEnvOverrides overlays environment variable values onto a Config.
// Env vars always take precedence over file values.
func applyEnvOverrides(cfg *Config) {
	if token := os.Getenv("GH_BELL_TOKEN"); token != "" {
		cfg.Token = token
	}
	if s := os.Getenv("GH_BELL_REFRESH"); s != "" {
		if secs, err := strconv.Atoi(s); err == nil && secs > 0 {
			cfg.RefreshInterval = secs
		} else {
			log.Printf("ignoring invalid GH_BELL_REFRESH=%q", s)
		}
	}
	if s := os.Getenv("GH_BELL_CLEANUP_DAYS"); s != "" {
		if days, err := strconv.Atoi(s); err == nil && days >= 0 {
			cfg.CleanupDays = days
		} else {
			log.Printf("ignoring invalid GH_BELL_CLEANUP_DAYS=%q", s)
		}
	}
	if s := os.Getenv("GH_BELL_GROUP_BY_REPO"); s != "" {
		cfg.GroupByRepo = s == "1" || s == "true"
	}
	if s := os.Getenv("GH_BELL_SORT_MODE"); s != "" {
		cfg.SortMode = s
	}
	if s := os.Getenv("GH_BELL_PREVIEW_COMMENT_FIRST"); s != "" {
		v := s == "1" || s == "true"
		cfg.PreviewCommentFirst = &v
	}
	if s := os.Getenv("GH_BELL_AUTO_READ_CLOSED"); s != "" {
		cfg.AutoReadClosed = s == "1" || s == "true"
	}
}

// createDefault writes a config file with commented-out defaults.
func createDefault(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(defaultConfigTemplate), 0o600)
}
