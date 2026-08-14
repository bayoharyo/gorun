## Goal Description
Dokumen ini merincikan rencana implementasi khusus untuk sisi backend dari aplikasi **Gorun**. Backend akan bertanggung jawab untuk membaca konfigurasi proyek dari file YAML, menyediakan endpoint HTTP untuk UI dan Webhook, menyimpan riwayat deployment ke SQLite, dan mengeksekusi perintah shell (`git` dan `docker`) secara asinkron.

## Design Decisions
1. **Concurrency Control**: Kita akan membatasi agar satu proyek hanya bisa melakukan **satu proses deployment dalam satu waktu**. Jika ada request deploy baru saat proses lama masih berjalan, request tersebut akan ditolak atau diabaikan untuk mencegah *race condition*.
2. **Webhook Support**: Versi awal ini hanya akan mendukung webhook dari **GitHub** (menggunakan validasi `X-Hub-Signature-256`).
3. **Database Driver**: Kita akan menggunakan driver *pure Go* `modernc.org/sqlite` (tanpa CGO) agar lebih praktis dalam proses build dan *cross-compilation*.

---

## Proposed Changes

### Configuration
#### [NEW] internal/config/config.go
Bertugas membaca dan me-mapping file `config.yaml` ke dalam struct Go.
```go
package config

import (
	"os"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Port int                  `yaml:"port"`
	Apps map[string]AppConfig `yaml:"apps"`
}

type AppConfig struct {
	Path          string `yaml:"path"`
	WebhookSecret string `yaml:"webhook_secret"`
	DeployCmd     string `yaml:"deploy_cmd"`
}

func Load(filepath string) (*Config, error) {
	// 1. Read file yaml
	// 2. Unmarshal ke struct Config
	// 3. Set default DeployCmd jika kosong (default: "docker compose up -d --build")
}
```

---

### Database & Store
#### [NEW] internal/store/store.go
Mengelola koneksi SQLite dan operasi CRUD untuk histori deployment.
```go
package store

import (
	"database/sql"
	_ "modernc.org/sqlite"
	"time"
)

type Deployment struct {
	ID        string
	AppName   string
	Status    string // "Deploying", "Success", "Failed"
	Logs      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Store struct {
	db *sql.DB
}

func NewStore(dbPath string) (*Store, error) {
	// 1. Open DB dengan driver "sqlite"
	// 2. Buat tabel jika belum ada:
	/*
		CREATE TABLE IF NOT EXISTS deployments (
			id TEXT PRIMARY KEY,
			app_name TEXT,
			status TEXT,
			logs TEXT,
			created_at DATETIME,
			updated_at DATETIME
		);
	*/
}

func (s *Store) CreateDeployment(d *Deployment) error { /* Insert ke DB */ }
func (s *Store) AppendLog(id string, logLine string) error { /* Update string logs (concatenation) di DB */ }
func (s *Store) UpdateStatus(id string, status string) error { /* Update status Success/Failed */ }
func (s *Store) GetLatestDeployment(appName string) (*Deployment, error) { /* Ambil data terbaru berdasarkan app_name */ }
```

---

### Shell Deployer
#### [NEW] internal/deployer/deployer.go
Core logic untuk mengeksekusi perintah shell (seperti Git Pull & Docker Build) dan mengelola *concurrency* (1 deploy per app).
```go
package deployer

import (
	"sync"
	"os/exec"
	"gorun/internal/store"
	"gorun/internal/config"
	"errors"
)

var ErrDeployInProgress = errors.New("deployment is already in progress for this app")

type Deployer struct {
	store *store.Store
	mu    sync.Mutex
	locks map[string]bool // Menyimpan status lock per app_name
}

func NewDeployer(s *store.Store) *Deployer {
	return &Deployer{
		store: s,
		locks: make(map[string]bool),
	}
}

// TriggerDeploy mengembalikan error jika sedang ada deployment berjalan
func (d *Deployer) TriggerDeploy(app config.AppConfig, appName string) (string, error) {
	d.mu.Lock()
	if d.locks[appName] {
		d.mu.Unlock()
		return "", ErrDeployInProgress
	}
	d.locks[appName] = true
	d.mu.Unlock()

	deployID := "generate-unique-id" // misal uuid atau string random
	
	// Simpan inisial status ke DB ("Deploying")
	// ...
	
	// Jalankan proses deploy di background agar HTTP response tidak terblokir
	go d.run(deployID, app, appName)
	return deployID, nil
}

func (d *Deployer) run(id string, app config.AppConfig, appName string) {
	defer func() {
		// Pastikan lock dilepas setelah proses background selesai
		d.mu.Lock()
		d.locks[appName] = false
		d.mu.Unlock()
	}()

	// PROSES 1: Git Pull
	// - exec.Command("git", "pull")
	// - cmd.Dir = app.Path
	// - cmd.StdoutPipe() / cmd.StderrPipe() -> dibaca menggunakan bufio.Scanner per baris
	// - Hasil teks dikirim ke `d.store.AppendLog(id, text)`

	// PROSES 2: Execute DeployCmd (misal: docker compose up -d --build)
	// - parse argumen command pakai sh -c
	// - tangkap output stream-nya dan log ke store juga
	
	// PROSES 3: Set status akhir
	// - Jika err != nil, update status jadi "Failed", jika aman jadi "Success"
}
```

---

### HTTP Handlers
#### [NEW] internal/handler/handler.go
Menyediakan HTTP endpoints untuk *UI rendering* dan API webhooks. Akan menggunakan `http.ServeMux` bawaan Go 1.22 yang sudah mendukung HTTP method dan path variables.
```go
package handler

import (
	"net/http"
	"gorun/internal/config"
	"gorun/internal/deployer"
	"gorun/internal/store"
)

type Handler struct {
	cfg      *config.Config
	store    *store.Store
	deployer *deployer.Deployer
}

func NewHandler(cfg *config.Config, s *store.Store, d *deployer.Deployer) *Handler {
	return &Handler{cfg: cfg, store: s, deployer: d}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// UI Routes
	mux.HandleFunc("GET /", h.handleDashboard)
	mux.HandleFunc("GET /app/{name}", h.handleProject)
	
	// HTMX API Routes
	mux.HandleFunc("POST /app/{name}/deploy", h.handleTriggerDeploy)
	mux.HandleFunc("GET /app/{name}/status", h.handleGetStatus)
	
	// Webhook Route
	mux.HandleFunc("POST /webhook/{name}", h.handleWebhook)
}

// Logic:
// - h.handleWebhook: mengecek `X-Hub-Signature-256` menggunakan `cfg.Apps[name].WebhookSecret`.
// - h.handleTriggerDeploy: memanggil `h.deployer.TriggerDeploy`. Jika mengembalikan ErrDeployInProgress, kirim header `HX-Trigger` dengan event Toast "Already deploying".
```

---

### App Entrypoint
#### [NEW] cmd/gorun/main.go
File utama untuk *wiring* dependensi dan menjalankan HTTP server.
```go
package main

import (
	"log"
	"net/http"
	"gorun/internal/config"
	"gorun/internal/store"
	"gorun/internal/deployer"
	"gorun/internal/handler"
)

func main() {
	cfg, _ := config.Load("config.yaml") // Jangan lupa error handling
	s, _ := store.NewStore("gorun.db")
	d := deployer.NewDeployer(s)
	h := handler.NewHandler(cfg, s, d)
	
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Serve folder static untuk CSS/JS
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	log.Printf("Server running on port %d", cfg.Port)
	http.ListenAndServe(":8080", mux) // Nanti port disesuaikan dengan cfg.Port
}
```

## Verification Plan
- **Automated Tests**: Membuat *unit test* minimal untuk:
  - `internal/config`: Parsing map YAML dan set fallback nilai default.
  - `internal/handler`: Validasi algoritma HMAC `X-Hub-Signature-256`.
- **Manual Verification**: 
  1. Menjalankan server menggunakan `go run cmd/gorun/main.go`.
  2. Melakukan klik tombol **Deploy** via UI dan memastikan respons HTML sesuai (trigger munculnya *Toast Notification* dan *polling* HTMX).
  3. Mengirimkan *mock payload* melalui terminal (`curl`) dengan menyertakan header `X-Hub-Signature-256` untuk memastikan validasi sekuriti pada webhook berfungsi.
