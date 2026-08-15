package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorun/internal/config"
	"gorun/internal/deployer"
	"gorun/internal/store"
)

func setupTestEnvironment(t *testing.T) (*Handler, *http.ServeMux, *store.Store, *store.Project) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	templatesDir := filepath.Join(tmpDir, "templates")

	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		t.Fatalf("failed to create test templates dir: %v", err)
	}

	// Minimal mock templates
	_ = os.WriteFile(filepath.Join(templatesDir, "dashboard.html"), []byte(`Dashboard: {{range .Apps}}{{.Name}} {{end}}`), 0644)
	_ = os.WriteFile(filepath.Join(templatesDir, "project.html"), []byte(`Project: {{.Project.Name}}`), 0644)
	_ = os.WriteFile(filepath.Join(templatesDir, "status_fragment.html"), []byte(`<div id="deployment-status">{{.Project.Name}}</div>`), 0644)
	_ = os.WriteFile(filepath.Join(templatesDir, "project_form.html"), []byte(`Form: {{if .IsEdit}}Edit{{else}}Create{{end}} {{if .Error}}Error: {{.Error}}{{end}}`), 0644)

	cfg := &config.Config{
		Port:     8080,
		Username: "admin",
		Password: "testpassword",
	}

	s, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}
	t.Cleanup(func() {
		_ = s.Close()
	})

	// Seed one project
	proj := &store.Project{
		Name:          "demo-app",
		Path:          tmpDir,
		Branch:        "main",
		WebhookSecret: "test-secret",
		DeployCmd:     "echo 'deploying test'",
	}
	if err := s.CreateProject(proj); err != nil {
		t.Fatalf("failed to seed project: %v", err)
	}

	d := deployer.NewDeployer(s)
	h, err := NewHandler(cfg, s, d, templatesDir)
	if err != nil {
		t.Fatalf("failed to init handler: %v", err)
	}

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	return h, mux, s, proj
}

func newAuthRequest(method, path string, body *strings.Reader) *http.Request {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, body)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.SetBasicAuth("admin", "testpassword")
	return req
}

func TestBasicAuthProtection(t *testing.T) {
	_, mux, _, _ := setupTestEnvironment(t)

	// 1. Without credentials -> 401 Unauthorized
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 without basic auth, got %d", rr.Code)
	}

	// 2. With wrong credentials -> 401 Unauthorized
	req = httptest.NewRequest("GET", "/", nil)
	req.SetBasicAuth("admin", "wrongpassword")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 with wrong password, got %d", rr.Code)
	}

	// 3. With valid credentials -> 200 OK
	req = newAuthRequest("GET", "/", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 with valid auth, got %d", rr.Code)
	}
}

func TestDashboardAndProjectRoutes(t *testing.T) {
	_, mux, _, _ := setupTestEnvironment(t)

	// Test GET /
	req := newAuthRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "demo-app") {
		t.Fatalf("expected body to contain demo-app, got: %s", rr.Body.String())
	}

	// Test GET /app/demo-app
	req = newAuthRequest("GET", "/app/demo-app", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Project: demo-app") {
		t.Fatalf("expected body to contain Project: demo-app, got: %s", rr.Body.String())
	}

	// Test GET /app/unknown-app (404)
	req = newAuthRequest("GET", "/app/unknown-app", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404 for unknown app, got %d", rr.Code)
	}
}

func TestProjectCRUD(t *testing.T) {
	_, mux, s, _ := setupTestEnvironment(t)

	// 1. GET /projects/new (form)
	req := newAuthRequest("GET", "/projects/new", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Create") {
		t.Fatalf("failed to load new project form: %d %s", rr.Code, rr.Body.String())
	}

	// 2a. POST /projects/new with invalid name -> should render form with error (200 OK)
	invalidFormData := url.Values{
		"name": {"Invalid Name with spaces!"},
		"path": {"/var/www/invalid-app"},
	}
	req = newAuthRequest("POST", "/projects/new", strings.NewReader(invalidFormData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Error:") {
		t.Fatalf("expected 200 OK with validation error for invalid project name, got %d", rr.Code)
	}

	// 2b. POST /projects/new (valid create)
	formData := url.Values{
		"name":           {"new-app"},
		"path":           {"/var/www/new-app"},
		"branch":         {"main"},
		"webhook_secret": {"sec123"},
		"deploy_cmd":     {"echo ok"},
	}
	req = newAuthRequest("POST", "/projects/new", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect after create, got %d", rr.Code)
	}

	created, err := s.GetProject("new-app")
	if err != nil || created == nil {
		t.Fatalf("expected project to be created in store")
	}

	// 3. GET /app/new-app/edit (edit form)
	req = newAuthRequest("GET", "/app/new-app/edit", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Edit") {
		t.Fatalf("failed to load edit form: %d %s", rr.Code, rr.Body.String())
	}

	// 4. POST /app/new-app/edit (update)
	updateData := url.Values{
		"path":           {"/var/www/new-app-updated"},
		"branch":         {"develop"},
		"webhook_secret": {"sec456"},
		"deploy_cmd":     {"echo updated"},
	}
	req = newAuthRequest("POST", "/app/new-app/edit", strings.NewReader(updateData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect after update, got %d", rr.Code)
	}

	updated, _ := s.GetProject("new-app")
	if updated.Path != "/var/www/new-app-updated" {
		t.Fatalf("expected updated path, got %s", updated.Path)
	}

	// 5a. POST /app/new-app/rename with invalid name -> should render form with error
	invalidRenameData := url.Values{
		"new_name": {"../invalid/slug"},
	}
	req = newAuthRequest("POST", "/app/new-app/rename", strings.NewReader(invalidRenameData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Error:") {
		t.Fatalf("expected 200 OK with validation error for invalid rename, got %d", rr.Code)
	}

	// 5b. POST /app/new-app/rename (valid rename)
	renameData := url.Values{
		"new_name": {"brand-new-app"},
	}
	req = newAuthRequest("POST", "/app/new-app/rename", strings.NewReader(renameData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect after rename, got %d", rr.Code)
	}

	renamed, _ := s.GetProject("brand-new-app")
	if renamed == nil {
		t.Fatalf("expected project with new name to exist")
	}

	// 6. DELETE /app/brand-new-app (delete)
	req = newAuthRequest("DELETE", "/app/brand-new-app", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on delete, got %d", rr.Code)
	}
	if rr.Header().Get("HX-Redirect") != "/" {
		t.Fatalf("expected HX-Redirect header to be '/', got %s", rr.Header().Get("HX-Redirect"))
	}

	deleted, _ := s.GetProject("brand-new-app")
	if deleted != nil {
		t.Fatalf("expected project to be deleted from store")
	}
}

func TestDeployTriggerAndStatus(t *testing.T) {
	_, mux, _, _ := setupTestEnvironment(t)

	// Test POST /app/demo-app/deploy
	req := newAuthRequest("POST", "/app/demo-app/deploy", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Header().Get("HX-Trigger"), "showToast") {
		t.Fatalf("expected HX-Trigger header to contain showToast, got: %s", rr.Header().Get("HX-Trigger"))
	}

	// Test GET /app/demo-app/status
	req = newAuthRequest("GET", "/app/demo-app/status", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
}

func TestWebhookHandler(t *testing.T) {
	_, mux, _, _ := setupTestEnvironment(t)

	secret := "test-secret"
	payload := `{"ref":"refs/heads/main"}`

	// 1. Invalid signature (no basic auth needed, should check signature)
	req := httptest.NewRequest("POST", "/webhook/demo-app", strings.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 for invalid signature, got %d", rr.Code)
	}

	// 2. Valid signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	validSignature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req = httptest.NewRequest("POST", "/webhook/demo-app", strings.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", validSignature)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status 202 for valid webhook, got %d", rr.Code)
	}
}
