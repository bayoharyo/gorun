# Gorun 🚀

**Gorun** is a lightweight, single-node deployment manager web application designed to simplify the deployment process for your applications (especially Docker / Go based) directly on your server / VPS.

Gorun runs on the same server as the target applications, monitors/pulls the latest source code from Git, executes build/containerization commands, and serves real-time deployment logs through a modern web UI powered by **Go Templates + HTMX**.

---

## Key Features

- **Project Management via Web UI**: Add, edit, rename, and delete project configurations directly from your browser without manually modifying YAML files.
- **Basic Authentication Protection**: Secure all dashboard and project management pages with configurable username and password credentials.
- **Fast Interactivity Without SPA**: Uses Server-Side Rendering (SSR) with Go Templates + **HTMX** for asynchronous status polling and log streaming without full page reloads.
- **GitHub Webhooks Support**: Secure automatic deployments triggered by `git push` events, validated using HMAC-SHA256 signatures (`X-Hub-Signature-256`) while bypassing Basic Auth restrictions.
- **Concurrency Control**: Prevents race conditions by ensuring only **1 active deployment process** runs per application at any given time.
- **Embedded / Pure-Go Database**: Uses pure SQLite (`modernc.org/sqlite`) without CGO dependencies, featuring foreign key constraints and cascade deletion support.
- **Real-time Terminal Logs**: Streams build output (`stdout` & `stderr`) periodically in the browser.

---

## Architecture & Workflow

```
GitHub Push / UI Trigger 
         │
         ▼
┌──────────────────┐
│   Gorun Server   │
└────────┬─────────┘
         │ (Asynchronous / Non-blocking)
         ▼
 1. Concurrency Check (Lock)
 2. Update status -> "Deploying" in SQLite
 3. Git Pull (git pull origin <branch>)
 4. Execute Deploy Command (docker compose up -d --build)
 5. Stream output -> Update log in SQLite
 6. Update status -> "Success" / "Failed" (Release Lock)
```

---

## Prerequisites

- **Go** (version 1.22 or newer) to run or build Gorun.
- **Git** installed on the server/machine.
- **Docker & Docker Compose** (if deployment targets use containers).

---

## Configuration (`config.yaml`)

Gorun server configuration focuses on the listening port and Basic Auth login credentials:

```yaml
# Gorun Server Configuration
port: 8080
username: "admin"
password: "your-secure-password"
```

---

## How to Run

### 1. Direct Execution (Development)
```bash
go run cmd/gorun/main.go
```

### 2. Build Binary (Production)
```bash
# Compile binary
go build -o gorun ./cmd/gorun

# Run binary
./gorun --config=config.yaml
```

Open your browser and navigate to:
```text
http://localhost:8080
```
*(Use the username and password configured in `config.yaml` when prompted for authentication).*

---

## GitHub Webhook Integration

To enable automated deployments upon pushing to GitHub:

1. Create a project via the Gorun web interface (**+ Add Project**).
2. Set a **Webhook Secret Key** for the project.
3. Open your GitHub repository &rarr; **Settings** &rarr; **Webhooks** &rarr; **Add webhook**.
4. **Payload URL**: `http://<YOUR-VPS-IP>:8080/webhook/<PROJECT-NAME>` (example: `http://103.x.x.x:8080/webhook/my-app`).
5. **Content type**: `application/json`.
6. **Secret**: Enter the same secret configured for the project in Gorun.
7. **Which events would you like to trigger this webhook?**: Select *Just the push event*.
8. Click **Add webhook**.

---

## Running Tests

Run the entire unit test suite:

```bash
go test -v ./...
```
