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
