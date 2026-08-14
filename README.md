# Gorun 🚀

**Gorun** adalah aplikasi web *single-node deployment manager* yang ringan untuk mempermudah proses deployment aplikasi (khususnya berbasis Docker / Go) langsung di server / VPS Anda. 

Gorun berjalan di server yang sama dengan aplikasi target, memantau/mengambil *source code* terbaru dari Git, mengeksekusi proses build/containerization, dan menyajikan log deployment secara *real-time* melalui antarmuka web modern berbasis **Go Templates + HTMX**.

---

## ✨ Fitur Utama

- **Konfigurasi Statis via YAML**: Definisi aplikasi dan perintah deploy dikelola secara terpusat melalui file `config.yaml`.
- **Interaktivitas Cepat Tanpa SPA**: Menggunakan Server-Side Rendering (SSR) Go Templates + **HTMX** untuk polling status dan streaming log secara asinkron tanpa reload halaman.
- **Dukungan GitHub Webhooks**: Deployment otomatis yang aman saat ada event `git push`, divalidasi menggunakan HMAC-SHA256 signature (`X-Hub-Signature-256`).
- **Concurrency Control**: Mencegah *race condition* dengan memastikan hanya ada **1 proses deployment aktif** per aplikasi dalam satu waktu.
- **Embedded / Pure-Go Database**: Menggunakan SQLite murni (`modernc.org/sqlite`) tanpa ketergantungan CGO, sehingga mudah di-*compile* dan dipindahkan ke berbagai lingkungan.
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

Buat file `config.yaml` di root direktori aplikasi (atau salin dari `config.example.yaml`):

```yaml
port: 8080

apps:
  demo-app:
    path: "/var/www/demo-app"               # Path direktori proyek di server
    branch: "main"                         # Branch git yang di-pull (default: main)
    webhook_secret: "super-secret-key-123" # Secret untuk validasi GitHub webhook
    deploy_cmd: "docker compose up -d --build" # Perintah build & run (default)
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
./gorun --config=config.yaml --port=8080
```

Buka browser dan akses antarmuka Gorun di:
```text
http://localhost:8080
```

---

## 🔗 Integrasi GitHub Webhook

Untuk mengaktifkan deployment otomatis saat melakukan push ke GitHub:

1. Buka repositori GitHub Anda &rarr; **Settings** &rarr; **Webhooks** &rarr; **Add webhook**.
2. **Payload URL**: `http://<IP-VPS-ANDA>:8080/webhook/<NAMA-APP>` (contoh: `http://103.x.x.x:8080/webhook/demo-app`).
3. **Content type**: `application/json`.
4. **Secret**: Masukkan secret yang sama dengan `webhook_secret` di `config.yaml`.
5. **Which events would you like to trigger this webhook?**: Pilih *Just the push event*.
6. Klik **Add webhook**.

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
│   ├── config/              # Modul pembaca dan parser config.yaml
│   ├── deployer/            # Engine eksekusi shell & concurrency lock
│   ├── handler/             # HTTP controller, webhook validator, & router
│   └── store/               # SQLite store untuk history & logs
├── static/
│   ├── css/
│   │   └── style.css        # Vanilla CSS (Dark mode modern UI)
│   └── js/
│       └── htmx.min.js      # Library HTMX
├── templates/
│   ├── dashboard.html       # Halaman utama (daftar aplikasi)
│   ├── project.html         # Halaman detail & live terminal log
│   └── status_fragment.html # Fragmen HTMX untuk auto-refresh polling
├── config.example.yaml      # Contoh file konfigurasi
├── go.mod
└── README.md
```

---

## 📄 Lisensi

Proyek ini dibuat untuk kebutuhan personal & open-source. Bebas digunakan dan dimodifikasi sesuai kebutuhan Anda.
