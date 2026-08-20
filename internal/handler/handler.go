package handler

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gorun/internal/config"
	"gorun/internal/deployer"
	"gorun/internal/store"
)

var validProjectNameRegex = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

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

func (h *Handler) basicAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		userMatch := subtle.ConstantTimeCompare([]byte(username), []byte(h.cfg.Username)) == 1
		passMatch := subtle.ConstantTimeCompare([]byte(password), []byte(h.cfg.Password)) == 1
		if !ok || !userMatch || !passMatch {
			w.Header().Set("WWW-Authenticate", `Basic realm="Gorun Dashboard"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// UI Pages (protected with Basic Auth)
	mux.HandleFunc("GET /", h.basicAuth(h.handleDashboard))
	mux.HandleFunc("GET /app/{name}", h.basicAuth(h.handleProject))

	// HTMX Endpoints
	mux.HandleFunc("POST /app/{name}/deploy", h.basicAuth(h.handleTriggerDeploy))
	mux.HandleFunc("GET /app/{name}/status", h.basicAuth(h.handleGetStatus))

	// Environment Variables & Custom Domains Endpoints
	mux.HandleFunc("POST /app/{name}/env", h.basicAuth(h.handleAddEnv))
	mux.HandleFunc("DELETE /app/{name}/env/{id}", h.basicAuth(h.handleDeleteEnv))
	mux.HandleFunc("POST /app/{name}/domain", h.basicAuth(h.handleAddDomain))
	mux.HandleFunc("DELETE /app/{name}/domain/{id}", h.basicAuth(h.handleDeleteDomain))

	// Project CRUD Form Endpoints
	mux.HandleFunc("GET /projects/new", h.basicAuth(h.handleNewProjectForm))
	mux.HandleFunc("POST /projects/new", h.basicAuth(h.handleCreateProject))
	mux.HandleFunc("GET /app/{name}/edit", h.basicAuth(h.handleEditProjectForm))
	mux.HandleFunc("POST /app/{name}/edit", h.basicAuth(h.handleUpdateProject))
	mux.HandleFunc("POST /app/{name}/rename", h.basicAuth(h.handleRenameProject))
	mux.HandleFunc("DELETE /app/{name}", h.basicAuth(h.handleDeleteProject))

	// Webhook Route (NOT protected with Basic Auth, verified with HMAC signature)
	mux.HandleFunc("POST /webhook/{name}", h.handleWebhook)
}

type AppSummary struct {
	Name             string
	Project          *store.Project
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

	projects, err := h.store.ListProjects()
	if err != nil {
		log.Printf("[ERROR] failed to fetch projects: %v", err)
		http.Error(w, "Failed to load projects", http.StatusInternalServerError)
		return
	}

	var apps []AppSummary
	for _, proj := range projects {
		latest, err := h.store.GetLatestDeployment(proj.ID)
		if err != nil {
			log.Printf("[ERROR] failed to fetch latest deployment for %s: %v", proj.Name, err)
		}
		isDeploying := h.deployer.IsDeploying(proj.ID)

		apps = append(apps, AppSummary{
			Name:             proj.Name,
			Project:          proj,
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
	Project          *store.Project
	LatestDeployment *store.Deployment
	History          []*store.Deployment
	IsDeploying      bool
	Host             string
	Envs             []*store.ProjectEnv
	Domains          []*store.ProjectDomain
}

func (h *Handler) lookupProject(w http.ResponseWriter, name string) (*store.Project, bool) {
	proj, err := h.store.GetProject(name)
	if err != nil {
		log.Printf("[ERROR] failed to lookup project %s: %v", name, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return nil, false
	}
	if proj == nil {
		http.Error(w, fmt.Sprintf("Project %q not found", name), http.StatusNotFound)
		return nil, false
	}
	return proj, true
}

func (h *Handler) handleProject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	proj, ok := h.lookupProject(w, name)
	if !ok {
		return
	}

	latest, err := h.store.GetLatestDeployment(proj.ID)
	if err != nil {
		log.Printf("[ERROR] failed to get latest deployment for %s: %v", name, err)
	}

	history, err := h.store.ListDeployments(proj.ID, 10)
	if err != nil {
		log.Printf("[ERROR] failed to get history for %s: %v", name, err)
	}

	envs, err := h.store.ListEnvs(proj.ID)
	if err != nil {
		log.Printf("[ERROR] failed to get envs for %s: %v", name, err)
	}

	domains, err := h.store.ListDomains(proj.ID)
	if err != nil {
		log.Printf("[ERROR] failed to get domains for %s: %v", name, err)
	}

	data := ProjectData{
		Project:          proj,
		LatestDeployment: latest,
		History:          history,
		IsDeploying:      h.deployer.IsDeploying(proj.ID),
		Host:             r.Host,
		Envs:             envs,
		Domains:          domains,
	}

	if err := h.tmpl.ExecuteTemplate(w, "project.html", data); err != nil {
		http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handler) handleTriggerDeploy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	proj, ok := h.lookupProject(w, name)
	if !ok {
		return
	}

	_, err := h.deployer.TriggerDeploy(proj, "manual")
	if err != nil {
		if err == deployer.ErrDeployInProgress {
			h.setToast(w, "Deployment is already running for this app!", "warning")
		} else {
			h.setToast(w, fmt.Sprintf("Deploy failed: %v", err), "error")
		}
	} else {
		h.setToast(w, fmt.Sprintf("Deployment triggered for %s", proj.Name), "info")
	}

	// Render status fragment
	h.renderStatusFragment(w, proj)
}

func (h *Handler) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	proj, ok := h.lookupProject(w, name)
	if !ok {
		return
	}

	h.renderStatusFragment(w, proj)
}

type StatusFragmentData struct {
	Project          *store.Project
	LatestDeployment *store.Deployment
	IsDeploying      bool
}

func (h *Handler) renderStatusFragment(w http.ResponseWriter, proj *store.Project) {
	latest, _ := h.store.GetLatestDeployment(proj.ID)
	isDeploying := h.deployer.IsDeploying(proj.ID)

	data := StatusFragmentData{
		Project:          proj,
		LatestDeployment: latest,
		IsDeploying:      isDeploying,
	}

	if err := h.tmpl.ExecuteTemplate(w, "status_fragment.html", data); err != nil {
		http.Error(w, fmt.Sprintf("Template fragment error: %v", err), http.StatusInternalServerError)
	}
}

// --- Environment Variables & Custom Domains Handlers ---

type EnvFragmentData struct {
	Project *store.Project
	Envs    []*store.ProjectEnv
}

func (h *Handler) renderEnvFragment(w http.ResponseWriter, proj *store.Project) {
	envs, err := h.store.ListEnvs(proj.ID)
	if err != nil {
		log.Printf("[ERROR] failed to list envs for %s: %v", proj.Name, err)
	}
	data := EnvFragmentData{
		Project: proj,
		Envs:    envs,
	}
	if err := h.tmpl.ExecuteTemplate(w, "env_fragment.html", data); err != nil {
		http.Error(w, fmt.Sprintf("Template fragment error: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handler) handleAddEnv(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	proj, ok := h.lookupProject(w, name)
	if !ok {
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	key := strings.TrimSpace(r.FormValue("key"))
	val := strings.TrimSpace(r.FormValue("value"))

	if key == "" {
		h.setToast(w, "Environment variable key cannot be empty", "error")
		h.renderEnvFragment(w, proj)
		return
	}

	if _, err := h.store.SetEnv(proj.ID, key, val); err != nil {
		log.Printf("[ERROR] failed to set env for %s: %v", name, err)
		h.setToast(w, fmt.Sprintf("Failed to save environment variable: %v", err), "error")
		h.renderEnvFragment(w, proj)
		return
	}

	h.setToast(w, fmt.Sprintf("Saved environment variable %s", key), "success")
	h.renderEnvFragment(w, proj)
}

func (h *Handler) handleDeleteEnv(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	proj, ok := h.lookupProject(w, name)
	if !ok {
		return
	}

	idStr := r.PathValue("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		http.Error(w, "Invalid env ID", http.StatusBadRequest)
		return
	}

	if err := h.store.DeleteEnv(proj.ID, id); err != nil {
		log.Printf("[ERROR] failed to delete env %d for %s: %v", id, name, err)
		h.setToast(w, "Failed to delete environment variable", "error")
	} else {
		h.setToast(w, "Environment variable removed", "info")
	}

	h.renderEnvFragment(w, proj)
}

type DomainFragmentData struct {
	Project *store.Project
	Domains []*store.ProjectDomain
}

func (h *Handler) renderDomainFragment(w http.ResponseWriter, proj *store.Project) {
	domains, err := h.store.ListDomains(proj.ID)
	if err != nil {
		log.Printf("[ERROR] failed to list domains for %s: %v", proj.Name, err)
	}
	data := DomainFragmentData{
		Project: proj,
		Domains: domains,
	}
	if err := h.tmpl.ExecuteTemplate(w, "domain_fragment.html", data); err != nil {
		http.Error(w, fmt.Sprintf("Template fragment error: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handler) handleAddDomain(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	proj, ok := h.lookupProject(w, name)
	if !ok {
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	domain := strings.ToLower(strings.TrimSpace(r.FormValue("domain")))
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimRight(domain, "/")

	if domain == "" {
		h.setToast(w, "Domain name cannot be empty", "error")
		h.renderDomainFragment(w, proj)
		return
	}

	if _, err := h.store.AddDomain(proj.ID, domain); err != nil {
		log.Printf("[ERROR] failed to add domain for %s: %v", name, err)
		h.setToast(w, fmt.Sprintf("Failed to add domain: %v", err), "error")
		h.renderDomainFragment(w, proj)
		return
	}

	h.setToast(w, fmt.Sprintf("Added domain %s", domain), "success")
	h.renderDomainFragment(w, proj)
}

func (h *Handler) handleDeleteDomain(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	proj, ok := h.lookupProject(w, name)
	if !ok {
		return
	}

	idStr := r.PathValue("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		http.Error(w, "Invalid domain ID", http.StatusBadRequest)
		return
	}

	if err := h.store.DeleteDomain(proj.ID, id); err != nil {
		log.Printf("[ERROR] failed to delete domain %d for %s: %v", id, name, err)
		h.setToast(w, "Failed to delete domain", "error")
	} else {
		h.setToast(w, "Domain removed", "info")
	}

	h.renderDomainFragment(w, proj)
}

// --- CRUD Handlers ---

type ProjectFormData struct {
	IsEdit  bool
	Project *store.Project
	Error   string
}

func (h *Handler) handleNewProjectForm(w http.ResponseWriter, r *http.Request) {
	data := ProjectFormData{
		IsEdit: false,
		Project: &store.Project{
			Branch:    "main",
			DeployCmd: "docker compose up -d --build",
		},
	}
	if err := h.tmpl.ExecuteTemplate(w, "project_form.html", data); err != nil {
		http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handler) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	proj := &store.Project{
		Name:          strings.TrimSpace(r.FormValue("name")),
		Path:          strings.TrimSpace(r.FormValue("path")),
		Branch:        strings.TrimSpace(r.FormValue("branch")),
		WebhookSecret: strings.TrimSpace(r.FormValue("webhook_secret")),
		DeployCmd:     strings.TrimSpace(r.FormValue("deploy_cmd")),
	}

	if proj.Name == "" || proj.Path == "" {
		data := ProjectFormData{
			IsEdit:  false,
			Project: proj,
			Error:   "Project Name and Path are required.",
		}
		h.tmpl.ExecuteTemplate(w, "project_form.html", data)
		return
	}

	if !validProjectNameRegex.MatchString(proj.Name) {
		data := ProjectFormData{
			IsEdit:  false,
			Project: proj,
			Error:   "Project Name must contain only lowercase alphanumeric characters and hyphens (e.g. 'my-app').",
		}
		h.tmpl.ExecuteTemplate(w, "project_form.html", data)
		return
	}

	if err := h.store.CreateProject(proj); err != nil {
		data := ProjectFormData{
			IsEdit:  false,
			Project: proj,
			Error:   fmt.Sprintf("Failed to create project: %v", err),
		}
		h.tmpl.ExecuteTemplate(w, "project_form.html", data)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) handleEditProjectForm(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	proj, ok := h.lookupProject(w, name)
	if !ok {
		return
	}

	data := ProjectFormData{
		IsEdit:  true,
		Project: proj,
	}
	if err := h.tmpl.ExecuteTemplate(w, "project_form.html", data); err != nil {
		http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handler) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	proj, ok := h.lookupProject(w, name)
	if !ok {
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	proj.Path = strings.TrimSpace(r.FormValue("path"))
	proj.Branch = strings.TrimSpace(r.FormValue("branch"))
	proj.WebhookSecret = strings.TrimSpace(r.FormValue("webhook_secret"))
	proj.DeployCmd = strings.TrimSpace(r.FormValue("deploy_cmd"))

	if proj.Path == "" {
		data := ProjectFormData{
			IsEdit:  true,
			Project: proj,
			Error:   "Project Path is required.",
		}
		h.tmpl.ExecuteTemplate(w, "project_form.html", data)
		return
	}

	if err := h.store.UpdateProject(proj); err != nil {
		data := ProjectFormData{
			IsEdit:  true,
			Project: proj,
			Error:   fmt.Sprintf("Failed to update project: %v", err),
		}
		h.tmpl.ExecuteTemplate(w, "project_form.html", data)
		return
	}

	http.Redirect(w, r, "/app/"+proj.Name, http.StatusSeeOther)
}

func (h *Handler) handleRenameProject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	proj, ok := h.lookupProject(w, name)
	if !ok {
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	newName := strings.TrimSpace(r.FormValue("new_name"))
	if newName == "" {
		data := ProjectFormData{
			IsEdit:  true,
			Project: proj,
			Error:   "New project name cannot be empty.",
		}
		h.tmpl.ExecuteTemplate(w, "project_form.html", data)
		return
	}

	if !validProjectNameRegex.MatchString(newName) {
		data := ProjectFormData{
			IsEdit:  true,
			Project: proj,
			Error:   "New project name must contain only lowercase alphanumeric characters and hyphens (e.g. 'my-app').",
		}
		h.tmpl.ExecuteTemplate(w, "project_form.html", data)
		return
	}

	if newName == proj.Name {
		http.Redirect(w, r, "/app/"+proj.Name, http.StatusSeeOther)
		return
	}

	if err := h.store.RenameProject(proj.ID, newName); err != nil {
		data := ProjectFormData{
			IsEdit:  true,
			Project: proj,
			Error:   fmt.Sprintf("Failed to rename project (name may already exist): %v", err),
		}
		h.tmpl.ExecuteTemplate(w, "project_form.html", data)
		return
	}

	http.Redirect(w, r, "/app/"+newName, http.StatusSeeOther)
}

func (h *Handler) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	proj, ok := h.lookupProject(w, name)
	if !ok {
		return
	}

	if err := h.store.DeleteProject(proj.ID); err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete project: %v", err), http.StatusInternalServerError)
		return
	}

	// HX-Redirect header tells HTMX client to navigate to root
	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleWebhook(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	proj, ok := h.lookupProject(w, name)
	if !ok {
		return
	}

	signature := r.Header.Get("X-Hub-Signature-256")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	if err := VerifyGitHubSignature(proj.WebhookSecret, signature, body); err != nil {
		log.Printf("[SECURITY] Webhook signature verification failed for app %s: %v", name, err)
		http.Error(w, fmt.Sprintf("Signature verification failed: %v", err), http.StatusUnauthorized)
		return
	}

	deployID, err := h.deployer.TriggerDeploy(proj, "webhook")
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
