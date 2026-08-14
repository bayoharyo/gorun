package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorun/internal/config"
	"gorun/internal/deployer"
	"gorun/internal/store"
)

func setupTestEnvironment(t *testing.T) (*Handler, *http.ServeMux, string) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	templatesDir := filepath.Join(tmpDir, "templates")

	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		t.Fatalf("failed to create test templates dir: %v", err)
	}

	// Minimal mock templates
	_ = os.WriteFile(filepath.Join(templatesDir, "dashboard.html"), []byte(`Dashboard: {{range .Apps}}{{.Name}}{{end}}`), 0644)
	_ = os.WriteFile(filepath.Join(templatesDir, "project.html"), []byte(`Project: {{.AppName}}`), 0644)
	_ = os.WriteFile(filepath.Join(templatesDir, "status_fragment.html"), []byte(`<div id="deployment-status">{{.AppName}}</div>`), 0644)

	cfg := &config.Config{
		Port: 8080,
		Apps: map[string]config.AppConfig{
			"demo-app": {
				Path:          tmpDir,
				Branch:        "main",
				WebhookSecret: "test-secret",
				DeployCmd:     "echo 'deploying test'",
			},
		},
	}

	s, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}

	d := deployer.NewDeployer(s)
	h, err := NewHandler(cfg, s, d, templatesDir)
	if err != nil {
		t.Fatalf("failed to init handler: %v", err)
	}

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	return h, mux, tmpDir
}

func TestDashboardAndProjectRoutes(t *testing.T) {
	_, mux, _ := setupTestEnvironment(t)

	// Test GET /
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "demo-app") {
		t.Fatalf("expected body to contain demo-app, got: %s", rr.Body.String())
	}

	// Test GET /app/demo-app
	req = httptest.NewRequest("GET", "/app/demo-app", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Project: demo-app") {
		t.Fatalf("expected body to contain Project: demo-app, got: %s", rr.Body.String())
	}

	// Test GET /app/unknown-app (404)
	req = httptest.NewRequest("GET", "/app/unknown-app", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404 for unknown app, got %d", rr.Code)
	}
}

func TestDeployTriggerAndStatus(t *testing.T) {
	_, mux, _ := setupTestEnvironment(t)

	// Test POST /app/demo-app/deploy
	req := httptest.NewRequest("POST", "/app/demo-app/deploy", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Header().Get("HX-Trigger"), "showToast") {
		t.Fatalf("expected HX-Trigger header to contain showToast, got: %s", rr.Header().Get("HX-Trigger"))
	}

	// Test GET /app/demo-app/status
	req = httptest.NewRequest("GET", "/app/demo-app/status", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
}

func TestWebhookHandler(t *testing.T) {
	_, mux, _ := setupTestEnvironment(t)

	secret := "test-secret"
	payload := `{"ref":"refs/heads/main"}`

	// 1. Invalid signature
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
