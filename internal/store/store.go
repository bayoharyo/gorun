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

type Deployment struct {
	ID            string    `json:"id"`
	AppName       string    `json:"app_name"`
	Status        Status    `json:"status"`
	Logs          string    `json:"logs"`
	TriggerSource string    `json:"trigger_source"` // "manual" or "webhook"
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewStore initializes a SQLite database connection and sets up tables.
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool for SQLite
	db.SetMaxOpenConns(1)

	createTableQuery := `
	CREATE TABLE IF NOT EXISTS deployments (
		id TEXT PRIMARY KEY,
		app_name TEXT NOT NULL,
		status TEXT NOT NULL,
		logs TEXT NOT NULL DEFAULT '',
		trigger_source TEXT NOT NULL DEFAULT 'manual',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_deployments_app_name ON deployments(app_name);
	CREATE INDEX IF NOT EXISTS idx_deployments_created_at ON deployments(created_at DESC);
	`

	if _, err := db.Exec(createTableQuery); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	return &Store{db: db}, nil
}

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
	INSERT INTO deployments (id, app_name, status, logs, trigger_source, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, d.ID, d.AppName, string(d.Status), d.Logs, d.TriggerSource, d.CreatedAt, d.UpdatedAt)
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
	SELECT id, app_name, status, logs, trigger_source, created_at, updated_at
	FROM deployments
	WHERE id = ?
	`
	var d Deployment
	var statusStr string
	err := s.db.QueryRow(query, id).Scan(
		&d.ID,
		&d.AppName,
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

// GetLatestDeployment retrieves the most recent deployment for an application.
func (s *Store) GetLatestDeployment(appName string) (*Deployment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT id, app_name, status, logs, trigger_source, created_at, updated_at
	FROM deployments
	WHERE app_name = ?
	ORDER BY created_at DESC
	LIMIT 1
	`
	var d Deployment
	var statusStr string
	err := s.db.QueryRow(query, appName).Scan(
		&d.ID,
		&d.AppName,
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

// ListDeployments returns the history of deployments for an application.
func (s *Store) ListDeployments(appName string, limit int) ([]*Deployment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	query := `
	SELECT id, app_name, status, logs, trigger_source, created_at, updated_at
	FROM deployments
	WHERE app_name = ?
	ORDER BY created_at DESC
	LIMIT ?
	`
	rows, err := s.db.Query(query, appName, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}
	defer rows.Close()

	var list []*Deployment
	for rows.Next() {
		var d Deployment
		var statusStr string
		if err := rows.Scan(&d.ID, &d.AppName, &statusStr, &d.Logs, &d.TriggerSource, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan deployment row: %w", err)
		}
		d.Status = Status(statusStr)
		list = append(list, &d)
	}
	return list, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}
