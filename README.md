# Gorun 🚀

**Gorun** adalah aplikasi web *single-node deployment manager* yang ringan untuk mempermudah proses deployment aplikasi (khususnya berbasis Docker / Go) langsung di server / VPS Anda. 

Gorun berjalan di server yang sama dengan aplikasi target, memantau/mengambil *source code* terbaru dari Git, mengeksekusi proses build/containerization, dan menyajikan log deployment secara *real-time* melalui antarmuka web modern berbasis **Go Templates + HTMX**.

---

## ✨ Fitur Utama

- **Project Management via Web UI**: Tambah, edit, ganti nama (rename), dan hapus konfigurasi proyek langsung dari browser tanpa perlu mengedit file YAML secara manual.
- **Proteksi Basic Authentication**: Mengamankan seluruh halaman dashboard dan manajemen proyek dengan kredensial username & password yang dapat dikonfigurasi.
- **Interaktivitas Cepat Tanpa SPA**: Menggunakan Server-Side Rendering (SSR) Go Templates + **HTMX** untuk polling status dan streaming log secara asinkron tanpa reload halaman.
- **Dukungan GitHub Webhooks**: Deployment otomatis yang aman saat ada event `git push`, divalidasi menggunakan HMAC-SHA256 signature (`X-Hub-Signature-256`) tanpa terhalang Basic Auth.
- **Concurrency Control**: Mencegah *race condition* dengan memastikan hanya ada **1 proses deployment aktif** per aplikasi dalam satu waktu.
- **Embedded / Pure-Go Database**: Menggunakan SQLite murni (`modernc.org/sqlite`) tanpa ketergantungan CGO, dengan dukungan relasi Foreign Key dan Cascade Delete.
- **Terminal Log Real-time**: Menampilkan output build (`stdout` & `stderr`) secara berkala di browser.

---

## 🏗️ Arsitektur & Alur Kerja

```
GitHub Push / UI Trigger 
         │
         ▼
┌──────────────────┐
│   Gorun Server   │
└────────┬─────────┘
         │ (Asinkron / Non-blocking)
         ▼
 1. Concurrency Check (Lock)
 2. Update status -> "Deploying" di SQLite
 3. Git Pull (git pull origin <branch>)
 4. Execute Deploy Command (docker compose up -d --build)
 5. Stream output -> Update log di SQLite
 6. Update status -> "Success" / "Failed" (Release Lock)
```

---

## 📋 Prasyarat

- **Go** (versi 1.22 atau lebih baru) untuk menjalankan atau mengompilasi Gorun.
- **Git** terinstall pada server/komputer.
- **Docker & Docker Compose** (jika target deployment menggunakan container).

---

## ⚙️ Konfigurasi (`config.yaml`)

Konfigurasi server Gorun difokuskan pada port dan kredensial login Basic Auth:

```yaml
# Gorun Server Configuration
port: 8080
username: "admin"
password: "your-secure-password"
```

---

## 🚀 Cara Menjalankan

### 1. Menjalankan Langsung (Development)
```bash
go run cmd/gorun/main.go
```

### 2. Build Binary (Production)
```bash
# Kompilasi binary
go build -o gorun ./cmd/gorun

# Jalankan binary
./gorun --config=config.yaml
```

Buka browser dan akses antarmuka Gorun di:
```text
http://localhost:8080
```
*(Gunakan username dan password yang telah ditentukan di `config.yaml` saat browser meminta otentikasi).*

---

## 🔗 Integrasi GitHub Webhook

Untuk mengaktifkan deployment otomatis saat melakukan push ke GitHub:

1. Buat proyek terlebih dahulu melalui antarmuka web Gorun (**+ Add Project**).
2. Tentukan **Webhook Secret Key** untuk proyek tersebut.
3. Buka repositori GitHub Anda &rarr; **Settings** &rarr; **Webhooks** &rarr; **Add webhook**.
4. **Payload URL**: `http://<IP-VPS-ANDA>:8080/webhook/<NAMA-PROJECT>` (contoh: `http://103.x.x.x:8080/webhook/my-app`).
5. **Content type**: `application/json`.
6. **Secret**: Masukkan secret yang sama dengan konfigurasi webhook secret di Gorun.
7. **Which events would you like to trigger this webhook?**: Pilih *Just the push event*.
8. Klik **Add webhook**.

---

## 🧪 Menjalankan Pengujian (Testing)

Jalankan seluruh *unit test* suite:

```bash
go test -v ./...
```

---

## 📁 Struktur Direktori

```text
.
├── cmd/
│   └── gorun/
│       └── main.go          # Entrypoint server Gorun
├── internal/
│   ├── config/              # Modul pembaca config.yaml & kredensial auth
│   ├── deployer/            # Engine eksekusi shell & concurrency lock
│   ├── handler/             # HTTP controller, auth middleware, CRUD & router
│   └── store/               # SQLite store untuk projects, deployments, & logs
├── static/
│   ├── css/
│   │   └── style.css        # Vanilla CSS (Dark mode modern UI)
│   └── js/
│       └── htmx.min.js      # Library HTMX
├── templates/
│   ├── dashboard.html       # Halaman utama (daftar aplikasi & tombol Add)
│   ├── project.html         # Halaman detail, logs, tombol Edit & Delete
│   ├── project_form.html    # Form Create, Edit, dan Rename project
│   └── status_fragment.html # Fragmen HTMX untuk auto-refresh polling
├── config.example.yaml      # Contoh file konfigurasi
├── go.mod
└── README.md
```

---

## 📄 Lisensi

Proyek ini dibuat untuk kebutuhan personal & open-source. Bebas digunakan dan dimodifikasi sesuai kebutuhan Anda.
