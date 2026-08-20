package deployer

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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

	proj := &store.Project{
		Name:      "myapp",
		Path:      tmpDir,
		DeployCmd: "echo 'building'; sleep 0.2; echo 'done'",
	}
	if err := s.CreateProject(proj); err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	// First trigger
	deployID1, err := d.TriggerDeploy(proj, "manual")
	if err != nil {
		t.Fatalf("unexpected error on first deploy: %v", err)
	}
	if deployID1 == "" {
		t.Fatalf("expected deployID to be generated")
	}

	// Immediate second trigger should return ErrDeployInProgress
	_, err = d.TriggerDeploy(proj, "manual")
	if err != ErrDeployInProgress {
		t.Fatalf("expected ErrDeployInProgress, got %v", err)
	}

	// Wait for background process to finish
	time.Sleep(500 * time.Millisecond)

	if d.IsDeploying(proj.ID) {
		t.Errorf("expected deployer lock to be released")
	}

	// Check status in DB
	dep, err := s.GetLatestDeployment(proj.ID)
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

func TestGenerateDockerfile(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Initial generation
	created, err := GenerateDockerfile(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error generating Dockerfile: %v", err)
	}
	if !created {
		t.Errorf("expected Dockerfile to be created")
	}

	dockerfilePath := filepath.Join(tmpDir, "Dockerfile")
	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("failed to read generated Dockerfile: %v", err)
	}
	if len(content) == 0 {
		t.Errorf("expected Dockerfile to have content")
	}

	// 2. Second call should not overwrite
	createdAgain, err := GenerateDockerfile(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}
	if createdAgain {
		t.Errorf("expected created to be false for existing Dockerfile")
	}
}

func TestDockerNamingAndArgs(t *testing.T) {
	projectID := int64(42)
	img := ImageName(projectID)
	if img != "gorun-img-42" {
		t.Errorf("expected gorun-img-42, got %s", img)
	}

	cnt := ContainerName(projectID)
	if cnt != "gorun-app-42" {
		t.Errorf("expected gorun-app-42, got %s", cnt)
	}

	port := HostPort(projectID)
	if port != 8042 {
		t.Errorf("expected 8042, got %d", port)
	}

	envs := []*store.ProjectEnv{
		{Key: "PORT", Value: "8080"},
		{Key: "DB_URL", Value: "postgres://localhost:5432/db"},
	}

	args := BuildDockerRunArgs(projectID, envs)
	expectedArgs := []string{
		"run", "-d",
		"--name", "gorun-app-42",
		"-p", "8042:8080",
		"-e", "PORT=8080",
		"-e", "DB_URL=postgres://localhost:5432/db",
		"gorun-img-42",
	}

	if len(args) != len(expectedArgs) {
		t.Fatalf("expected %d args, got %d: %v", len(expectedArgs), len(args), args)
	}

	for i, arg := range args {
		if arg != expectedArgs[i] {
			t.Errorf("arg[%d] expected %s, got %s", i, expectedArgs[i], arg)
		}
	}
}
