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
apps:
  myapp:
    path: "/var/www/myapp"
    webhook_secret: "supersecret"
  customapp:
    path: "/var/www/customapp"
    branch: "develop"
    deploy_cmd: "make deploy"
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

	if len(cfg.Apps) != 2 {
		t.Fatalf("expected 2 apps, got %d", len(cfg.Apps))
	}

	myApp, ok := cfg.Apps["myapp"]
	if !ok {
		t.Fatalf("expected 'myapp' to exist")
	}
	if myApp.Branch != "main" {
		t.Errorf("expected default branch 'main', got %s", myApp.Branch)
	}
	if myApp.DeployCmd != "docker compose up -d --build" {
		t.Errorf("expected default deploy command, got %s", myApp.DeployCmd)
	}
	if myApp.WebhookSecret != "supersecret" {
		t.Errorf("expected webhook secret 'supersecret', got %s", myApp.WebhookSecret)
	}

	customApp := cfg.Apps["customapp"]
	if customApp.Branch != "develop" {
		t.Errorf("expected branch 'develop', got %s", customApp.Branch)
	}
	if customApp.DeployCmd != "make deploy" {
		t.Errorf("expected deploy command 'make deploy', got %s", customApp.DeployCmd)
	}
}
