package deployer

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorun/internal/store"
)

var (
	ErrDeployInProgress = errors.New("deployment is already in progress for this application")
)

type Deployer struct {
	store *store.Store
	mu    sync.Mutex
	locks map[int64]bool
}

func NewDeployer(s *store.Store) *Deployer {
	return &Deployer{
		store: s,
		locks: make(map[int64]bool),
	}
}

// IsDeploying checks if a deployment is currently running for a project.
func (d *Deployer) IsDeploying(projectID int64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.locks[projectID]
}

// TriggerDeploy initiates an asynchronous deployment if not already running.
func (d *Deployer) TriggerDeploy(proj *store.Project, source string) (string, error) {
	d.mu.Lock()
	if d.locks[proj.ID] {
		d.mu.Unlock()
		return "", ErrDeployInProgress
	}
	d.locks[proj.ID] = true
	d.mu.Unlock()

	deployID := uuid.New().String()
	deployment := &store.Deployment{
		ID:            deployID,
		ProjectID:     proj.ID,
		ProjectName:   proj.Name,
		Status:        store.StatusDeploying,
		Logs:          fmt.Sprintf("[%s] Triggered deployment via %s\n", time.Now().Format("15:04:05"), source),
		TriggerSource: source,
		CreatedAt:     time.Now().UTC(),
	}

	if err := d.store.CreateDeployment(deployment); err != nil {
		d.mu.Lock()
		d.locks[proj.ID] = false
		d.mu.Unlock()
		return "", fmt.Errorf("failed to create deployment record: %w", err)
	}

	projSnapshot := *proj
	go d.run(deployID, &projSnapshot, source)

	return deployID, nil
}

func (d *Deployer) run(deployID string, proj *store.Project, source string) {
	defer func() {
		d.mu.Lock()
		d.locks[proj.ID] = false
		d.mu.Unlock()
	}()

	logWriter := func(line string) {
		_ = d.store.AppendLog(deployID, line+"\n")
	}

	logWriter(fmt.Sprintf("[%s] Working directory: %s", time.Now().Format("15:04:05"), proj.Path))

	// Step 1: Git pull if it is a git repository
	gitDir := filepath.Join(proj.Path, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		branch := proj.Branch
		if branch == "" {
			branch = "main"
		}

		logWriter(fmt.Sprintf("[%s] Executing: git pull origin %s", time.Now().Format("15:04:05"), branch))
		gitCmd := exec.Command("git", "pull", "origin", branch)
		gitCmd.Dir = proj.Path

		if err := runAndStreamOutput(gitCmd, logWriter); err != nil {
			logWriter(fmt.Sprintf("[%s] Git pull failed: %v", time.Now().Format("15:04:05"), err))
			_ = d.store.UpdateStatus(deployID, store.StatusFailed)
			return
		}
	} else {
		logWriter(fmt.Sprintf("[%s] No .git directory found, skipping git pull", time.Now().Format("15:04:05")))
	}

	// Step 2: Custom command fallback if specified (and not the default legacy value)
	if proj.DeployCmd != "" && proj.DeployCmd != "docker compose up -d --build" {
		logWriter(fmt.Sprintf("[%s] Executing custom deploy command: %s", time.Now().Format("15:04:05"), proj.DeployCmd))
		cmd := exec.Command("sh", "-c", proj.DeployCmd)
		cmd.Dir = proj.Path

		if err := runAndStreamOutput(cmd, logWriter); err != nil {
			logWriter(fmt.Sprintf("[%s] Custom deploy command failed: %v", time.Now().Format("15:04:05"), err))
			_ = d.store.UpdateStatus(deployID, store.StatusFailed)
			return
		}

		logWriter(fmt.Sprintf("[%s] Deployment finished successfully!", time.Now().Format("15:04:05")))
		_ = d.store.UpdateStatus(deployID, store.StatusSuccess)
		return
	}

	// Step 3: Zero-Config Go Docker Build & Run
	// 3a. Ensure Dockerfile exists (auto-generate if missing)
	created, err := GenerateDockerfile(proj.Path)
	if err != nil {
		logWriter(fmt.Sprintf("[%s] Dockerfile setup failed: %v", time.Now().Format("15:04:05"), err))
		_ = d.store.UpdateStatus(deployID, store.StatusFailed)
		return
	}
	if created {
		logWriter(fmt.Sprintf("[%s] No Dockerfile found. Auto-generated Go Dockerfile.", time.Now().Format("15:04:05")))
	} else {
		logWriter(fmt.Sprintf("[%s] Using existing Dockerfile.", time.Now().Format("15:04:05")))
	}

	// 3b. Fetch Environment Variables
	envs, err := d.store.ListEnvs(proj.ID)
	if err != nil {
		logWriter(fmt.Sprintf("[%s] Warning: Failed to list environment variables: %v", time.Now().Format("15:04:05"), err))
	} else if len(envs) > 0 {
		logWriter(fmt.Sprintf("[%s] Loaded %d environment variable(s)", time.Now().Format("15:04:05"), len(envs)))
	}

	// 3c. Docker build
	imgName := ImageName(proj.ID)
	logWriter(fmt.Sprintf("[%s] Building Docker image: %s", time.Now().Format("15:04:05"), imgName))
	buildCmd := exec.Command("docker", "build", "-t", imgName, ".")
	buildCmd.Dir = proj.Path
	if err := runAndStreamOutput(buildCmd, logWriter); err != nil {
		logWriter(fmt.Sprintf("[%s] Docker build failed: %v", time.Now().Format("15:04:05"), err))
		_ = d.store.UpdateStatus(deployID, store.StatusFailed)
		return
	}

	// 3d. Stop and remove existing container
	containerName := ContainerName(proj.ID)
	logWriter(fmt.Sprintf("[%s] Removing previous container (if exists): %s", time.Now().Format("15:04:05"), containerName))
	rmCmd := exec.Command("docker", "rm", "-f", containerName)
	_ = runAndStreamOutput(rmCmd, logWriter)

	// 3e. Docker run
	hostPort := HostPort(proj.ID)
	logWriter(fmt.Sprintf("[%s] Starting container %s on port %d...", time.Now().Format("15:04:05"), containerName, hostPort))
	runArgs := BuildDockerRunArgs(proj.ID, envs)
	runCmd := exec.Command("docker", runArgs...)
	runCmd.Dir = proj.Path
	if err := runAndStreamOutput(runCmd, logWriter); err != nil {
		logWriter(fmt.Sprintf("[%s] Docker run failed: %v", time.Now().Format("15:04:05"), err))
		_ = d.store.UpdateStatus(deployID, store.StatusFailed)
		return
	}

	logWriter(fmt.Sprintf("[%s] Deployment finished successfully! Application running on port %d", time.Now().Format("15:04:05"), hostPort))
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
