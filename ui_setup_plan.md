# Implementation Plan - Project Setup via UI & Basic Auth

## Goal Description
Fitur ini bertujuan untuk memindahkan manajemen konfigurasi aplikasi (yang sebelumnya dilakukan secara manual melalui `config.yaml`) ke dalam antarmuka web (UI) menggunakan **SQLite** dan **HTMX**. Pengguna akan dapat membuat (Create), mengedit (Edit), dan menghapus (Delete) konfigurasi proyek langsung dari dashboard web.

Selain itu, karena antarmuka web kini memiliki hak akses penuh untuk memodifikasi konfigurasi server, kita akan menambahkan sistem **Basic Authentication** (Username & Password) untuk melindungi seluruh rute UI. Rute Webhook akan dibiarkan tanpa Basic Auth karena sudah diamankan oleh HMAC-SHA256 signature rahasia.

## Proposed Changes

### 1. Konfigurasi (`internal/config/config.go`)
Struktur `config.yaml` akan disederhanakan. Bagian `apps` akan dihapus, dan diganti dengan konfigurasi kredensial login.

#### [MODIFY] internal/config/config.go
- Modifikasi `type Config struct`:
```go
type Config struct {
	Port     int    `yaml:"port"`
	Username string `yaml:"username"` // Username untuk Basic Auth UI
	Password string `yaml:"password"` // Password untuk Basic Auth UI
}
```
- Hapus struct `AppConfig` dan parsing bagian `Apps`.
- Berikan nilai default untuk `Username` (contoh: `admin`) dan `Password` (contoh: `admin`) jika kosong di file YAML.

---

### 2. Database & Store Layer (`internal/store/store.go`)
Membuat tabel baru untuk menyimpan proyek (Project) dan method CRUD-nya.

#### [MODIFY] internal/store/store.go
- Tambahkan struktur model `Project`:
```go
type Project struct {
	Name          string    `json:"name"`
	Path          string    `json:"path"`
	Branch        string    `json:"branch"`
	WebhookSecret string    `json:"webhook_secret"`
	DeployCmd     string    `json:"deploy_cmd"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
```
- Tambahkan pembuatan tabel `projects` saat inisialisasi:
```go
CREATE TABLE IF NOT EXISTS projects (
    name TEXT PRIMARY KEY,
    path TEXT NOT NULL,
    branch TEXT NOT NULL DEFAULT 'main',
    webhook_secret TEXT,
    deploy_cmd TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
```
- Tambahkan method CRUD:
  - `CreateProject(p *Project) error`
  - `GetProject(name string) (*Project, error)`
  - `ListProjects() ([]*Project, error)`
  - `UpdateProject(p *Project) error`
  - `DeleteProject(name string) error`

---

### 3. HTTP Handlers & Basic Auth Middleware (`internal/handler/handler.go`)
Menerapkan middleware untuk proteksi dan menambahkan endpoint baru untuk pemrosesan form via HTMX.

#### [MODIFY] internal/handler/handler.go
- **Basic Auth Middleware**:
  - Buat fungsi `h.basicAuth(next http.HandlerFunc) http.HandlerFunc` yang akan mencocokkan kredensial request dengan `cfg.Username` dan `cfg.Password`.
- **Registrasi Route**:
  - Terapkan `h.basicAuth` ke *semua* rute UI (`GET /`, `GET /app/...`, `GET /projects/...`, dll).
  - Rute `/webhook/{name}` **TIDAK** menggunakan Basic Auth (agar GitHub bisa menembak webhook).
- **Endpoint CRUD Proyek**:
  - `GET /projects/new` (Render form kosong via `project_form.html`)
  - `POST /projects/new` (Proses pembuatan proyek & redirect ke `/`)
  - `GET /app/{name}/edit` (Render form edit terisi via `project_form.html`)
  - `POST /app/{name}/edit` (Simpan perubahan)
  - `DELETE /app/{name}` (Hapus dari database dan kembalikan ke dashboard)
- Refaktor Handler lama (`handleDashboard`, `handleProject`, `handleTriggerDeploy`, dll) agar membaca `Project` dari `store.GetProject` dan `store.ListProjects()`.

---

### 4. Deployer Layer (`internal/deployer/deployer.go`)
#### [MODIFY] internal/deployer/deployer.go
- Ubah parameter struct di method `TriggerDeploy` dan `run` agar menerima pointer ke `store.Project` alih-alih `config.AppConfig`.

---

### 5. Frontend Templates
Menambahkan halaman form, menyesuaikan tombol, dan mengubah sumber data template.

#### [NEW] templates/project_form.html
- Halaman form HTML dengan elemen input yang bersih (Vanilla CSS).
- Satu form yang bisa menangani mode Create dan mode Edit (bergantung pada data yang di-passing).

#### [MODIFY] templates/dashboard.html
- Tambahkan tombol **"+ Add Project"** di bagian atas (Header section).
- Tampilkan form empty state jika `store.ListProjects()` kosong.

#### [MODIFY] templates/project.html
- Tambahkan tombol **"Edit Configuration"** dan **"Delete Project"** (menggunakan HTMX `hx-delete` + konfirmasi).

---

## Verification Plan

### Automated Tests
- Perbarui `handler_test.go`, `store_test.go`, dan `config_test.go` yang rusak akibat perubahan struct `AppConfig` dan tambahan parameter `Project`.
- Pastikan test webhook memintas Basic Auth, sedangkan test UI dashboard menerima status `401 Unauthorized` jika dikirim tanpa header kredensial.
- Command: `go test -v ./...`

### Manual Verification
1. Ubah `config.yaml` menjadi hanya memuat port, username, dan password.
2. Jalankan server lokal `go run cmd/gorun/main.go`.
3. Akses `http://localhost:8080/` dan pastikan browser memunculkan popup otentikasi login.
4. Login menggunakan kredensial yang dibuat.
5. Klik **"+ Add Project"**, masukkan rincian aplikasi dummy, lalu **Save**.
6. Verifikasi proyek baru muncul di halaman utama Dashboard.
7. Masuk ke halaman detil proyek dan lakukan test deploy manual.
8. Edit proyek (misal ganti webhook secret), lalu **Save**.
9. Hapus proyek menggunakan tombol Delete, verifikasi proyek menghilang dari dashboard.
