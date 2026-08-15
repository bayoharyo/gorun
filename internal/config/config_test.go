package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `
port: 9000
username: "superadmin"
password: "supersecretpassword"
`

	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.Port != 9000 {
		t.Errorf("expected port 9000, got %d", cfg.Port)
	}
	if cfg.Username != "superadmin" {
		t.Errorf("expected username 'superadmin', got %q", cfg.Username)
	}
	if cfg.Password != "supersecretpassword" {
		t.Errorf("expected password 'supersecretpassword', got %q", cfg.Password)
	}
}

func TestLoadDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config_empty.yaml")

	yamlContent := `{}`

	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Port)
	}
	if cfg.Username != "admin" {
		t.Errorf("expected default username 'admin', got %q", cfg.Username)
	}
	if cfg.Password != "admin" {
		t.Errorf("expected default password 'admin', got %q", cfg.Password)
	}
}
