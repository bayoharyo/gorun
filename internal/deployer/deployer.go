package deployer

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorun/internal/config"
	"gorun/internal/store"
)

var (
	ErrDeployInProgress = errors.New("deployment is already in progress for this application")
)

type Deployer struct {
	store *store.Store
	mu    sync.Mutex
	locks map[string]bool
}

func NewDeployer(s *store.Store) *Deployer {
	return &Deployer{
		store: s,
		locks: make(map[string]bool),
	}
}

// IsDeploying checks if a deployment is currently running for an application.
func (d *Deployer) IsDeploying(appName string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.locks[appName]
}

// TriggerDeploy initiates an asynchronous deployment if not already running.
func (d *Deployer) TriggerDeploy(appName string, app config.AppConfig, source string) (string, error) {
	d.mu.Lock()
	if d.locks[appName] {
		d.mu.Unlock()
		return "", ErrDeployInProgress
	}
	d.locks[appName] = true
	d.mu.Unlock()

	deployID := uuid.New().String()
	deployment := &store.Deployment{
		ID:            deployID,
		AppName:       appName,
		Status:        store.StatusDeploying,
		Logs:          fmt.Sprintf("[%s] Triggered deployment via %s\n", time.Now().Format("15:04:05"), source),
		TriggerSource: source,
		CreatedAt:     time.Now().UTC(),
	}

	if err := d.store.CreateDeployment(deployment); err != nil {
		d.mu.Lock()
		d.locks[appName] = false
		d.mu.Unlock()
		return "", fmt.Errorf("failed to create deployment record: %w", err)
	}

	go d.run(deployID, appName, app)

	return deployID, nil
}

func (d *Deployer) run(deployID, appName string, app config.AppConfig) {
	defer func() {
		d.mu.Lock()
		d.locks[appName] = false
		d.mu.Unlock()
	}()

	logWriter := func(line string) {
		_ = d.store.AppendLog(deployID, line+"\n")
	}

	logWriter(fmt.Sprintf("[%s] Working directory: %s", time.Now().Format("15:04:05"), app.Path))

	// Step 1: Git pull if path is set and git is intended
	branch := app.Branch
	if branch == "" {
		branch = "main"
	}

	logWriter(fmt.Sprintf("[%s] Executing: git pull origin %s", time.Now().Format("15:04:05"), branch))
	gitCmd := exec.Command("git", "pull", "origin", branch)
	gitCmd.Dir = app.Path

	if err := runAndStreamOutput(gitCmd, logWriter); err != nil {
		logWriter(fmt.Sprintf("[%s] Git pull failed: %v", time.Now().Format("15:04:05"), err))
		_ = d.store.UpdateStatus(deployID, store.StatusFailed)
		return
	}

	// Step 2: Run deploy command
	deployCmdStr := app.DeployCmd
	if deployCmdStr == "" {
		deployCmdStr = "docker compose up -d --build"
	}

	logWriter(fmt.Sprintf("[%s] Executing: %s", time.Now().Format("15:04:05"), deployCmdStr))
	cmd := exec.Command("sh", "-c", deployCmdStr)
	cmd.Dir = app.Path

	if err := runAndStreamOutput(cmd, logWriter); err != nil {
		logWriter(fmt.Sprintf("[%s] Deploy command failed: %v", time.Now().Format("15:04:05"), err))
		_ = d.store.UpdateStatus(deployID, store.StatusFailed)
		return
	}

	logWriter(fmt.Sprintf("[%s] Deployment finished successfully!", time.Now().Format("15:04:05")))
	_ = d.store.UpdateStatus(deployID, store.StatusSuccess)
}

func runAndStreamOutput(cmd *exec.Cmd, logFunc func(string)) error {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	reader := io.MultiReader(stdout, stderr)
	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		logFunc(scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		logFunc(fmt.Sprintf("[STREAM ERROR] %v", err))
	}

	return cmd.Wait()
}
