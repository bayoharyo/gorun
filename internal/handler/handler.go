package handler

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"gorun/internal/config"
	"gorun/internal/deployer"
	"gorun/internal/store"
)

type Handler struct {
	cfg          *config.Config
	store        *store.Store
	deployer     *deployer.Deployer
	templatesDir string
	tmpl         *template.Template
}

func NewHandler(cfg *config.Config, s *store.Store, d *deployer.Deployer, templatesDir string) (*Handler, error) {
	h := &Handler{
		cfg:          cfg,
		store:        s,
		deployer:     d,
		templatesDir: templatesDir,
	}

	if err := h.loadTemplates(); err != nil {
		return nil, fmt.Errorf("failed to load templates: %w", err)
	}

	return h, nil
}

func (h *Handler) loadTemplates() error {
	funcMap := template.FuncMap{
		"formatTime": func(t time.Time) string {
			if t.IsZero() {
				return "Never"
			}
			return t.Local().Format("2006-01-02 15:04:05")
		},
		"formatDuration": func(start, end time.Time) string {
			if start.IsZero() || end.IsZero() {
				return "-"
			}
			dur := end.Sub(start).Round(time.Second)
			return dur.String()
		},
		"statusClass": func(status store.Status) string {
			switch status {
			case store.StatusSuccess:
				return "status-success"
			case store.StatusFailed:
				return "status-failed"
			case store.StatusDeploying:
				return "status-deploying"
			default:
				return "status-idle"
			}
		},
	}

	pattern := filepath.Join(h.templatesDir, "*.html")
	tmpl, err := template.New("").Funcs(funcMap).ParseGlob(pattern)
	if err != nil {
		return err
	}
	h.tmpl = tmpl
	return nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// UI Pages
	mux.HandleFunc("GET /", h.handleDashboard)
	mux.HandleFunc("GET /app/{name}", h.handleProject)

	// HTMX Endpoints
	mux.HandleFunc("POST /app/{name}/deploy", h.handleTriggerDeploy)
	mux.HandleFunc("GET /app/{name}/status", h.handleGetStatus)

	// Webhook
	mux.HandleFunc("POST /webhook/{name}", h.handleWebhook)
}

type AppSummary struct {
	Name             string
	Config           config.AppConfig
	LatestDeployment *store.Deployment
	IsDeploying      bool
}

type DashboardData struct {
	Apps []AppSummary
}

func (h *Handler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	var apps []AppSummary
	for name, appCfg := range h.cfg.Apps {
		latest, err := h.store.GetLatestDeployment(name)
		if err != nil {
			log.Printf("[ERROR] failed to fetch latest deployment for %s: %v", name, err)
		}
		isDeploying := h.deployer.IsDeploying(name)

		apps = append(apps, AppSummary{
			Name:             name,
			Config:           appCfg,
			LatestDeployment: latest,
			IsDeploying:      isDeploying,
		})
	}

	data := DashboardData{Apps: apps}
	if err := h.tmpl.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
	}
}

type ProjectData struct {
	AppName          string
	Config           config.AppConfig
	LatestDeployment *store.Deployment
	History          []*store.Deployment
	IsDeploying      bool
	Host             string
}

func (h *Handler) handleProject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	appCfg, ok := h.cfg.Apps[name]
	if !ok {
		http.Error(w, "Application not found in configuration", http.StatusNotFound)
		return
	}

	latest, err := h.store.GetLatestDeployment(name)
	if err != nil {
		log.Printf("[ERROR] failed to get latest deployment: %v", err)
	}

	history, err := h.store.ListDeployments(name, 10)
	if err != nil {
		log.Printf("[ERROR] failed to get history: %v", err)
	}

	data := ProjectData{
		AppName:          name,
		Config:           appCfg,
		LatestDeployment: latest,
		History:          history,
		IsDeploying:      h.deployer.IsDeploying(name),
		Host:             r.Host,
	}

	if err := h.tmpl.ExecuteTemplate(w, "project.html", data); err != nil {
		http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handler) handleTriggerDeploy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	appCfg, ok := h.cfg.Apps[name]
	if !ok {
		http.Error(w, "Application not found", http.StatusNotFound)
		return
	}

	_, err := h.deployer.TriggerDeploy(name, appCfg, "manual")
	if err != nil {
		if err == deployer.ErrDeployInProgress {
			h.setToast(w, "Deployment is already running for this app!", "warning")
		} else {
			h.setToast(w, fmt.Sprintf("Deploy failed: %v", err), "error")
		}
	} else {
		h.setToast(w, fmt.Sprintf("Deployment triggered for %s", name), "info")
	}

	// Render status fragment
	h.renderStatusFragment(w, name)
}

func (h *Handler) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	_, ok := h.cfg.Apps[name]
	if !ok {
		http.Error(w, "Application not found", http.StatusNotFound)
		return
	}

	h.renderStatusFragment(w, name)
}

type StatusFragmentData struct {
	AppName          string
	LatestDeployment *store.Deployment
	IsDeploying      bool
}

func (h *Handler) renderStatusFragment(w http.ResponseWriter, appName string) {
	latest, _ := h.store.GetLatestDeployment(appName)
	isDeploying := h.deployer.IsDeploying(appName)

	data := StatusFragmentData{
		AppName:          appName,
		LatestDeployment: latest,
		IsDeploying:      isDeploying,
	}

	if err := h.tmpl.ExecuteTemplate(w, "status_fragment.html", data); err != nil {
		http.Error(w, fmt.Sprintf("Template fragment error: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handler) handleWebhook(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	appCfg, ok := h.cfg.Apps[name]
	if !ok {
		http.Error(w, "Application not found in config", http.StatusNotFound)
		return
	}

	signature := r.Header.Get("X-Hub-Signature-256")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	if err := VerifyGitHubSignature(appCfg.WebhookSecret, signature, body); err != nil {
		log.Printf("[SECURITY] Webhook signature verification failed for app %s: %v", name, err)
		http.Error(w, fmt.Sprintf("Signature verification failed: %v", err), http.StatusUnauthorized)
		return
	}

	deployID, err := h.deployer.TriggerDeploy(name, appCfg, "webhook")
	if err != nil {
		if err == deployer.ErrDeployInProgress {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status":  "ignored",
				"message": "A deployment is already in progress for this application",
			})
			return
		}
		http.Error(w, fmt.Sprintf("Failed to trigger deploy: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":        "accepted",
		"deployment_id": deployID,
		"message":       "Deployment started",
	})
}

type ToastMessage struct {
	Message string `json:"message"`
	Type    string `json:"type"` // "info", "success", "warning", "error"
}

func (h *Handler) setToast(w http.ResponseWriter, msg, toastType string) {
	payload := map[string]ToastMessage{
		"showToast": {
			Message: msg,
			Type:    toastType,
		},
	}
	bytes, err := json.Marshal(payload)
	if err == nil {
		// HTMX HX-Trigger header sends JSON event to client
		w.Header().Set("HX-Trigger", string(bytes))
	}
}
