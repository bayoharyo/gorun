package deployer

import (
	"path/filepath"
	"testing"
	"time"

	"gorun/internal/config"
	"gorun/internal/store"
)

func TestDeployerConcurrency(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	s, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}
	defer s.Close()

	d := NewDeployer(s)

	app := config.AppConfig{
		Path:      tmpDir,
		DeployCmd: "echo 'building'; sleep 0.2; echo 'done'",
	}

	// First trigger
	deployID1, err := d.TriggerDeploy("myapp", app, "manual")
	if err != nil {
		t.Fatalf("unexpected error on first deploy: %v", err)
	}
	if deployID1 == "" {
		t.Fatalf("expected deployID to be generated")
	}

	// Immediate second trigger should return ErrDeployInProgress
	_, err = d.TriggerDeploy("myapp", app, "manual")
	if err != ErrDeployInProgress {
		t.Fatalf("expected ErrDeployInProgress, got %v", err)
	}

	// Wait for background process to finish
	time.Sleep(500 * time.Millisecond)

	if d.IsDeploying("myapp") {
		t.Errorf("expected deployer lock to be released")
	}

	// Check status in DB
	dep, err := s.GetLatestDeployment("myapp")
	if err != nil {
		t.Fatalf("failed to get deployment: %v", err)
	}
	if dep == nil {
		t.Fatalf("deployment was nil")
	}
	if dep.Status != store.StatusSuccess && dep.Status != store.StatusFailed {
		t.Errorf("expected final status, got %s", dep.Status)
	}
}
