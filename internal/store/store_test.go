package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStore(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}
	defer s.Close()

	d := &Deployment{
		ID:            "dep-123",
		AppName:       "webapp",
		Status:        StatusDeploying,
		Logs:          "Starting deployment...\n",
		TriggerSource: "manual",
		CreatedAt:     time.Now().UTC(),
	}

	if err := s.CreateDeployment(d); err != nil {
		t.Fatalf("failed to create deployment: %v", err)
	}

	// Append log
	if err := s.AppendLog("dep-123", "Git pull complete.\n"); err != nil {
		t.Fatalf("failed to append log: %v", err)
	}

	// Update status
	if err := s.UpdateStatus("dep-123", StatusSuccess); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	// Get latest
	latest, err := s.GetLatestDeployment("webapp")
	if err != nil {
		t.Fatalf("failed to get latest deployment: %v", err)
	}
	if latest == nil {
		t.Fatalf("expected latest deployment to not be nil")
	}

	if latest.Status != StatusSuccess {
		t.Errorf("expected status %s, got %s", StatusSuccess, latest.Status)
	}
	expectedLog := "Starting deployment...\nGit pull complete.\n"
	if latest.Logs != expectedLog {
		t.Errorf("expected logs %q, got %q", expectedLog, latest.Logs)
	}

	// List deployments
	list, err := s.ListDeployments("webapp", 10)
	if err != nil {
		t.Fatalf("failed to list deployments: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 deployment in list, got %d", len(list))
	}
}
