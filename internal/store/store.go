package store

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Status string

const (
	StatusDeploying Status = "Deploying"
	StatusSuccess   Status = "Success"
	StatusFailed    Status = "Failed"
)

// Project represents a deployment target configuration stored in the database.
type Project struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Path          string    `json:"path"`
	Branch        string    `json:"branch"`
	WebhookSecret string    `json:"webhook_secret"`
	DeployCmd     string    `json:"deploy_cmd"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Deployment represents a deployment run record.
type Deployment struct {
	ID            string    `json:"id"`
	ProjectID     int64     `json:"project_id"`
	ProjectName   string    `json:"project_name"`
	Status        Status    `json:"status"`
	Logs          string    `json:"logs"`
	TriggerSource string    `json:"trigger_source"` // "manual" or "webhook"
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ProjectEnv represents an environment variable key-value pair for a project.
type ProjectEnv struct {
	ID        int64     `json:"id"`
	ProjectID int64     `json:"project_id"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ProjectDomain represents a custom domain mapped to a project.
type ProjectDomain struct {
	ID        int64     `json:"id"`
	ProjectID int64     `json:"project_id"`
	Domain    string    `json:"domain"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewStore initializes a SQLite database connection and sets up tables with foreign keys enabled.
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool for SQLite single-writer safety
	db.SetMaxOpenConns(1)

	// Enable foreign keys enforcement in SQLite (default is OFF)
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS projects (
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		name           TEXT    NOT NULL UNIQUE,
		path           TEXT    NOT NULL,
		branch         TEXT    NOT NULL DEFAULT 'main',
		webhook_secret TEXT    NOT NULL DEFAULT '',
		deploy_cmd     TEXT    NOT NULL DEFAULT '',
		created_at     DATETIME NOT NULL,
		updated_at     DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS deployments (
		id             TEXT    PRIMARY KEY,
		project_id     INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		project_name   TEXT    NOT NULL,
		status         TEXT    NOT NULL,
		logs           TEXT    NOT NULL DEFAULT '',
		trigger_source TEXT    NOT NULL DEFAULT 'manual',
		created_at     DATETIME NOT NULL,
		updated_at     DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS project_envs (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		key        TEXT    NOT NULL,
		value      TEXT    NOT NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		UNIQUE(project_id, key)
	);

	CREATE TABLE IF NOT EXISTS project_domains (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		domain     TEXT    NOT NULL UNIQUE,
		created_at DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_deployments_project_id ON deployments(project_id);
	CREATE INDEX IF NOT EXISTS idx_deployments_created_at ON deployments(created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_project_envs_project_id ON project_envs(project_id);
	CREATE INDEX IF NOT EXISTS idx_project_domains_project_id ON project_domains(project_id);
	`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	return &Store{db: db}, nil
}

// --- Project CRUD ---

// CreateProject inserts a new project and sets the generated ID.
func (s *Store) CreateProject(p *Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now

	if p.Branch == "" {
		p.Branch = "main"
	}
	if p.DeployCmd == "" {
		p.DeployCmd = "docker compose up -d --build"
	}

	query := `
	INSERT INTO projects (name, path, branch, webhook_secret, deploy_cmd, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	res, err := s.db.Exec(query, p.Name, p.Path, p.Branch, p.WebhookSecret, p.DeployCmd, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create project: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to retrieve last insert id: %w", err)
	}
	p.ID = id
	return nil
}

// GetProject retrieves a project by unique name slug.
func (s *Store) GetProject(name string) (*Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT id, name, path, branch, webhook_secret, deploy_cmd, created_at, updated_at
	FROM projects
	WHERE name = ?
	`
	var p Project
	err := s.db.QueryRow(query, name).Scan(
		&p.ID,
		&p.Name,
		&p.Path,
		&p.Branch,
		&p.WebhookSecret,
		&p.DeployCmd,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get project by name: %w", err)
	}
	return &p, nil
}

// GetProjectByID retrieves a project by its primary key ID.
func (s *Store) GetProjectByID(id int64) (*Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT id, name, path, branch, webhook_secret, deploy_cmd, created_at, updated_at
	FROM projects
	WHERE id = ?
	`
	var p Project
	err := s.db.QueryRow(query, id).Scan(
		&p.ID,
		&p.Name,
		&p.Path,
		&p.Branch,
		&p.WebhookSecret,
		&p.DeployCmd,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get project by ID: %w", err)
	}
	return &p, nil
}

// ListProjects returns all configured projects ordered by creation time.
func (s *Store) ListProjects() ([]*Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT id, name, path, branch, webhook_secret, deploy_cmd, created_at, updated_at
	FROM projects
	ORDER BY created_at ASC
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}
	defer rows.Close()

	var list []*Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(
			&p.ID,
			&p.Name,
			&p.Path,
			&p.Branch,
			&p.WebhookSecret,
			&p.DeployCmd,
			&p.CreatedAt,
			&p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan project row: %w", err)
		}
		list = append(list, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate project rows: %w", err)
	}
	return list, nil
}

// UpdateProject updates an existing project's configurations (except id, name, and created_at).
func (s *Store) UpdateProject(p *Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p.UpdatedAt = time.Now().UTC()
	query := `
	UPDATE projects
	SET path = ?, branch = ?, webhook_secret = ?, deploy_cmd = ?, updated_at = ?
	WHERE id = ?
	`
	res, err := s.db.Exec(query, p.Path, p.Branch, p.WebhookSecret, p.DeployCmd, p.UpdatedAt, p.ID)
	if err != nil {
		return fmt.Errorf("failed to update project: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("project id %d not found", p.ID)
	}
	return nil
}

// RenameProject updates a project's unique name slug.
func (s *Store) RenameProject(id int64, newName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	query := `
	UPDATE projects
	SET name = ?, updated_at = ?
	WHERE id = ?
	`
	res, err := s.db.Exec(query, newName, now, id)
	if err != nil {
		return fmt.Errorf("failed to rename project: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("project id %d not found", id)
	}
	return nil
}

// DeleteProject deletes a project by its primary key ID (cascades to deployments).
func (s *Store) DeleteProject(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM projects WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}
	return nil
}

// --- Deployment Operations ---

// CreateDeployment inserts a new deployment record.
func (s *Store) CreateDeployment(d *Deployment) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	d.UpdatedAt = now

	query := `
	INSERT INTO deployments (id, project_id, project_name, status, logs, trigger_source, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, d.ID, d.ProjectID, d.ProjectName, string(d.Status), d.Logs, d.TriggerSource, d.CreatedAt, d.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert deployment: %w", err)
	}
	return nil
}

// AppendLog appends text to an existing deployment's logs.
func (s *Store) AppendLog(id string, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	query := `
	UPDATE deployments
	SET logs = logs || ?, updated_at = ?
	WHERE id = ?
	`
	_, err := s.db.Exec(query, text, now, id)
	if err != nil {
		return fmt.Errorf("failed to append logs: %w", err)
	}
	return nil
}

// UpdateStatus updates the final status of a deployment.
func (s *Store) UpdateStatus(id string, status Status) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	query := `
	UPDATE deployments
	SET status = ?, updated_at = ?
	WHERE id = ?
	`
	_, err := s.db.Exec(query, string(status), now, id)
	if err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}
	return nil
}

// GetDeployment retrieves a deployment by its ID.
func (s *Store) GetDeployment(id string) (*Deployment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT id, project_id, project_name, status, logs, trigger_source, created_at, updated_at
	FROM deployments
	WHERE id = ?
	`
	var d Deployment
	var statusStr string
	err := s.db.QueryRow(query, id).Scan(
		&d.ID,
		&d.ProjectID,
		&d.ProjectName,
		&statusStr,
		&d.Logs,
		&d.TriggerSource,
		&d.CreatedAt,
		&d.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	d.Status = Status(statusStr)
	return &d, nil
}

// GetLatestDeployment retrieves the most recent deployment for a project ID.
func (s *Store) GetLatestDeployment(projectID int64) (*Deployment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT id, project_id, project_name, status, logs, trigger_source, created_at, updated_at
	FROM deployments
	WHERE project_id = ?
	ORDER BY created_at DESC
	LIMIT 1
	`
	var d Deployment
	var statusStr string
	err := s.db.QueryRow(query, projectID).Scan(
		&d.ID,
		&d.ProjectID,
		&d.ProjectName,
		&statusStr,
		&d.Logs,
		&d.TriggerSource,
		&d.CreatedAt,
		&d.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get latest deployment: %w", err)
	}
	d.Status = Status(statusStr)
	return &d, nil
}

// ListDeployments returns the history of deployments for a project ID.
func (s *Store) ListDeployments(projectID int64, limit int) ([]*Deployment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	query := `
	SELECT id, project_id, project_name, status, logs, trigger_source, created_at, updated_at
	FROM deployments
	WHERE project_id = ?
	ORDER BY created_at DESC
	LIMIT ?
	`
	rows, err := s.db.Query(query, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}
	defer rows.Close()

	var list []*Deployment
	for rows.Next() {
		var d Deployment
		var statusStr string
		if err := rows.Scan(
			&d.ID,
			&d.ProjectID,
			&d.ProjectName,
			&statusStr,
			&d.Logs,
			&d.TriggerSource,
			&d.CreatedAt,
			&d.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan deployment row: %w", err)
		}
		d.Status = Status(statusStr)
		list = append(list, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate deployment rows: %w", err)
	}
	return list, nil
}

// --- Environment Variables Operations ---

// ListEnvs returns all environment variables for a given project ID ordered by key.
func (s *Store) ListEnvs(projectID int64) ([]*ProjectEnv, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT id, project_id, key, value, created_at, updated_at
	FROM project_envs
	WHERE project_id = ?
	ORDER BY key ASC
	`
	rows, err := s.db.Query(query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list envs: %w", err)
	}
	defer rows.Close()

	var list []*ProjectEnv
	for rows.Next() {
		var e ProjectEnv
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.Key, &e.Value, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan env row: %w", err)
		}
		list = append(list, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate env rows: %w", err)
	}
	return list, nil
}

// SetEnv inserts or updates an environment variable for a project.
func (s *Store) SetEnv(projectID int64, key, value string) (*ProjectEnv, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	query := `
	INSERT INTO project_envs (project_id, key, value, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?)
	ON CONFLICT(project_id, key) DO UPDATE SET
		value = excluded.value,
		updated_at = excluded.updated_at
	RETURNING id, project_id, key, value, created_at, updated_at
	`
	var e ProjectEnv
	err := s.db.QueryRow(query, projectID, key, value, now, now).Scan(
		&e.ID, &e.ProjectID, &e.Key, &e.Value, &e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to set env: %w", err)
	}
	return &e, nil
}

// DeleteEnv removes an environment variable by ID for a project.
func (s *Store) DeleteEnv(projectID int64, envID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `DELETE FROM project_envs WHERE id = ? AND project_id = ?`
	res, err := s.db.Exec(query, envID, projectID)
	if err != nil {
		return fmt.Errorf("failed to delete env: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("env id %d not found for project %d", envID, projectID)
	}
	return nil
}

// --- Custom Domain Operations ---

// ListDomains returns all custom domains for a given project ID ordered by domain name.
func (s *Store) ListDomains(projectID int64) ([]*ProjectDomain, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT id, project_id, domain, created_at
	FROM project_domains
	WHERE project_id = ?
	ORDER BY domain ASC
	`
	rows, err := s.db.Query(query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list domains: %w", err)
	}
	defer rows.Close()

	var list []*ProjectDomain
	for rows.Next() {
		var d ProjectDomain
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.Domain, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan domain row: %w", err)
		}
		list = append(list, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate domain rows: %w", err)
	}
	return list, nil
}

// AddDomain adds a custom domain for a project.
func (s *Store) AddDomain(projectID int64, domain string) (*ProjectDomain, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	query := `
	INSERT INTO project_domains (project_id, domain, created_at)
	VALUES (?, ?, ?)
	RETURNING id, project_id, domain, created_at
	`
	var d ProjectDomain
	err := s.db.QueryRow(query, projectID, domain, now).Scan(
		&d.ID, &d.ProjectID, &d.Domain, &d.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to add domain: %w", err)
	}
	return &d, nil
}

// DeleteDomain removes a custom domain by ID for a project.
func (s *Store) DeleteDomain(projectID int64, domainID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `DELETE FROM project_domains WHERE id = ? AND project_id = ?`
	res, err := s.db.Exec(query, domainID, projectID)
	if err != nil {
		return fmt.Errorf("failed to delete domain: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("domain id %d not found for project %d", domainID, projectID)
	}
	return nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}
