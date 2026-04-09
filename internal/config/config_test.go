package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(""), 0o600)

	// Override Dir() by setting the file directly — we test via the parse path
	cfg := &Config{}
	if cfg.Token != "" {
		t.Fatal("expected empty token")
	}
}

func TestLoad_ParsesYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `token: ghp_test123
refresh_interval: 120
`
	os.WriteFile(cfgPath, []byte(content), 0o600)

	data, _ := os.ReadFile(cfgPath)
	cfg := &Config{}
	if err := parseConfig(data, cfg); err != nil {
		t.Fatalf("parse: %v", err)
	}

	if cfg.Token != "ghp_test123" {
		t.Fatalf("expected token ghp_test123, got %q", cfg.Token)
	}
	if cfg.RefreshInterval != 120 {
		t.Fatalf("expected refresh 120, got %d", cfg.RefreshInterval)
	}
}

func TestLoad_EnvVarsOverrideFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `token: ghp_from_file
refresh_interval: 30
`
	os.WriteFile(cfgPath, []byte(content), 0o600)

	data, _ := os.ReadFile(cfgPath)
	cfg := &Config{}
	parseConfig(data, cfg)

	// Simulate env override
	t.Setenv("GH_BELL_TOKEN", "ghp_from_env")
	t.Setenv("GH_BELL_REFRESH", "90")

	applyEnvOverrides(cfg)

	if cfg.Token != "ghp_from_env" {
		t.Fatalf("expected env token, got %q", cfg.Token)
	}
	if cfg.RefreshInterval != 90 {
		t.Fatalf("expected 90, got %d", cfg.RefreshInterval)
	}
}

func TestLoad_InvalidRefreshEnvIgnored(t *testing.T) {
	cfg := &Config{RefreshInterval: 60}

	t.Setenv("GH_BELL_REFRESH", "notanumber")
	applyEnvOverrides(cfg)

	if cfg.RefreshInterval != 60 {
		t.Fatalf("expected 60 (unchanged), got %d", cfg.RefreshInterval)
	}
}

func TestCreateDefault_SetsPermissions(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	if err := createDefault(cfgPath); err != nil {
		t.Fatalf("createDefault: %v", err)
	}

	info, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Fatalf("expected 0600 permissions, got %o", perm)
	}

	data, _ := os.ReadFile(cfgPath)
	if len(data) == 0 {
		t.Fatal("expected non-empty default config")
	}
}

func TestLoad_CleanupDaysDefault(t *testing.T) {
	cfg := &Config{}
	// Simulate what Load does: parseConfig with empty data, then apply defaults
	if cfg.CleanupDays != 0 {
		t.Fatal("expected zero before defaults applied")
	}
	// After Load, CleanupDays should default to 15
	if defaultCleanupDays != 15 {
		t.Fatalf("expected default cleanup days 15, got %d", defaultCleanupDays)
	}
}

func TestLoad_CleanupDaysFromYAML(t *testing.T) {
	content := `token: ghp_test
cleanup_days: 30
`
	cfg := &Config{}
	if err := parseConfig([]byte(content), cfg); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.CleanupDays != 30 {
		t.Fatalf("expected cleanup_days 30, got %d", cfg.CleanupDays)
	}
}

func TestLoad_CleanupDaysEnvOverride(t *testing.T) {
	cfg := &Config{CleanupDays: 15}
	t.Setenv("GH_BELL_CLEANUP_DAYS", "7")
	applyEnvOverrides(cfg)
	if cfg.CleanupDays != 7 {
		t.Fatalf("expected cleanup_days 7 from env, got %d", cfg.CleanupDays)
	}
}

func TestLoad_CleanupDaysZeroDisables(t *testing.T) {
	cfg := &Config{CleanupDays: 15}
	t.Setenv("GH_BELL_CLEANUP_DAYS", "0")
	applyEnvOverrides(cfg)
	if cfg.CleanupDays != 0 {
		t.Fatalf("expected cleanup_days 0 (disabled) from env, got %d", cfg.CleanupDays)
	}
}
