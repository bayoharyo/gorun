package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreProjectCRUDAndCascade(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}
	defer s.Close()

	// 1. Create Project
	p := &Project{
		Name:          "demo-app",
		Path:          "/var/www/demo",
		Branch:        "main",
		WebhookSecret: "secret123",
		DeployCmd:     "docker compose up -d",
	}

	if err := s.CreateProject(p); err != nil {
		t.Fatalf("failed to create project: %v", err)
	}

	if p.ID == 0 {
		t.Fatalf("expected project ID to be assigned, got 0")
	}

	// 2. Get Project by Name
	got, err := s.GetProject("demo-app")
	if err != nil {
		t.Fatalf("failed to get project: %v", err)
	}
	if got == nil || got.ID != p.ID || got.Path != p.Path {
		t.Fatalf("unexpected project fetched: %+v", got)
	}

	// 3. Get Project by ID
	gotByID, err := s.GetProjectByID(p.ID)
	if err != nil {
		t.Fatalf("failed to get project by ID: %v", err)
	}
	if gotByID == nil || gotByID.Name != "demo-app" {
		t.Fatalf("unexpected project by ID: %+v", gotByID)
	}

	// 4. Update Project
	p.Path = "/var/www/demo-updated"
	p.Branch = "develop"
	if err := s.UpdateProject(p); err != nil {
		t.Fatalf("failed to update project: %v", err)
	}

	updated, _ := s.GetProject("demo-app")
	if updated.Path != "/var/www/demo-updated" || updated.Branch != "develop" {
		t.Fatalf("update was not persisted: %+v", updated)
	}

	// 5. Create Deployment for Project
	d := &Deployment{
		ID:            "dep-123",
		ProjectID:     p.ID,
		ProjectName:   p.Name,
		Status:        StatusDeploying,
		Logs:          "Starting deployment...\n",
		TriggerSource: "manual",
		CreatedAt:     time.Now().UTC(),
	}
	if err := s.CreateDeployment(d); err != nil {
		t.Fatalf("failed to create deployment: %v", err)
	}

	// Append log & update status
	if err := s.AppendLog("dep-123", "Step 1 done\n"); err != nil {
		t.Fatalf("failed to append log: %v", err)
	}
	if err := s.UpdateStatus("dep-123", StatusSuccess); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	latest, err := s.GetLatestDeployment(p.ID)
	if err != nil || latest == nil {
		t.Fatalf("failed to get latest deployment: %v", err)
	}
	if latest.Status != StatusSuccess {
		t.Errorf("expected status %s, got %s", StatusSuccess, latest.Status)
	}

	// 6. Rename Project
	if err := s.RenameProject(p.ID, "renamed-app"); err != nil {
		t.Fatalf("failed to rename project: %v", err)
	}

	renamed, err := s.GetProject("renamed-app")
	if err != nil || renamed == nil {
		t.Fatalf("expected project to exist with new name")
	}
	old, err := s.GetProject("demo-app")
	if err != nil || old != nil {
		t.Fatalf("expected old name to no longer exist")
	}

	// Verify deployment history is still accessible by ID
	latestAfterRename, err := s.GetLatestDeployment(p.ID)
	if err != nil || latestAfterRename == nil {
		t.Fatalf("expected deployment history to persist with project ID")
	}

	// 7. Test Env Operations
	env1, err := s.SetEnv(p.ID, "PORT", "8080")
	if err != nil {
		t.Fatalf("failed to set env: %v", err)
	}
	if env1.ID == 0 || env1.Key != "PORT" || env1.Value != "8080" {
		t.Fatalf("unexpected env returned: %+v", env1)
	}

	// Update env (UPSERT)
	env1Updated, err := s.SetEnv(p.ID, "PORT", "9090")
	if err != nil {
		t.Fatalf("failed to update env: %v", err)
	}
	if env1Updated.Value != "9090" {
		t.Fatalf("expected updated value 9090, got %s", env1Updated.Value)
	}

	// Add second env
	_, err = s.SetEnv(p.ID, "DATABASE_URL", "postgres://localhost/db")
	if err != nil {
		t.Fatalf("failed to add second env: %v", err)
	}

	envs, err := s.ListEnvs(p.ID)
	if err != nil || len(envs) != 2 {
		t.Fatalf("expected 2 envs, got %d, err: %v", len(envs), err)
	}

	// Delete one env
	if err := s.DeleteEnv(p.ID, env1.ID); err != nil {
		t.Fatalf("failed to delete env: %v", err)
	}
	envsAfterDel, err := s.ListEnvs(p.ID)
	if err != nil || len(envsAfterDel) != 1 {
		t.Fatalf("expected 1 env after deletion, got %d", len(envsAfterDel))
	}

	// 8. Test Domain Operations
	dom1, err := s.AddDomain(p.ID, "api.example.com")
	if err != nil {
		t.Fatalf("failed to add domain: %v", err)
	}
	if dom1.ID == 0 || dom1.Domain != "api.example.com" {
		t.Fatalf("unexpected domain returned: %+v", dom1)
	}

	dom2, err := s.AddDomain(p.ID, "app.example.com")
	if err != nil {
		t.Fatalf("failed to add domain 2: %v", err)
	}

	doms, err := s.ListDomains(p.ID)
	if err != nil || len(doms) != 2 {
		t.Fatalf("expected 2 domains, got %d, err: %v", len(doms), err)
	}

	if err := s.DeleteDomain(p.ID, dom2.ID); err != nil {
		t.Fatalf("failed to delete domain: %v", err)
	}
	domsAfterDel, err := s.ListDomains(p.ID)
	if err != nil || len(domsAfterDel) != 1 {
		t.Fatalf("expected 1 domain after deletion, got %d", len(domsAfterDel))
	}

	// 9. Test Cascade Delete
	if err := s.DeleteProject(p.ID); err != nil {
		t.Fatalf("failed to delete project: %v", err)
	}

	// Verify project is deleted
	deletedProj, _ := s.GetProject("renamed-app")
	if deletedProj != nil {
		t.Fatalf("expected project to be deleted")
	}

	// Verify deployments are cascade deleted
	depAfterDelete, err := s.GetDeployment("dep-123")
	if err != nil {
		t.Fatalf("unexpected error querying deployment after delete: %v", err)
	}
	if depAfterDelete != nil {
		t.Fatalf("expected deployment to be deleted via foreign key cascade")
	}

	// Verify envs and domains are cascade deleted
	envsAfterProjDelete, err := s.ListEnvs(p.ID)
	if err != nil {
		t.Fatalf("unexpected error querying envs after delete: %v", err)
	}
	if len(envsAfterProjDelete) != 0 {
		t.Fatalf("expected 0 envs after project delete, got %d", len(envsAfterProjDelete))
	}

	domsAfterProjDelete, err := s.ListDomains(p.ID)
	if err != nil {
		t.Fatalf("unexpected error querying domains after delete: %v", err)
	}
	if len(domsAfterProjDelete) != 0 {
		t.Fatalf("expected 0 domains after project delete, got %d", len(domsAfterProjDelete))
	}
}
