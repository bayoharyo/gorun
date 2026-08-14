## Goal Description
Membangun `Gorun`, sebuah aplikasi web ringan (single-node) untuk men-deploy aplikasi Go ke VPS. Gorun akan berjalan di VPS yang sama dengan target deployment, mengambil (pull) source code terbaru dari Git, melakukan build Docker image, dan menjalankan container. Aplikasi ini dikembangkan menggunakan Go untuk backend, dan Go Templates + HTMX + Vanilla CSS untuk frontend yang responsif dan interaktif tanpa kerumitan SPA. Mendukung trigger deployment secara manual via UI dan otomatis via Webhook.

## Kesepakatan Desain (Design Decisions)
- **Manajemen Proyek (YAML)**: Berdasarkan masukan terbaru Anda, konfigurasi proyek (daftar aplikasi yang akan di-deploy) akan didefinisikan menggunakan file `config.yaml` agar lebih statis dan simpel, bukan melalui input dinamis di UI web.
- **Prasyarat Sistem**: Aplikasi ini membutuhkan `git` dan `docker` terinstall di server target. Karena umumnya VPS baru masih kosong (hanya berisi OS bawaan), kita akan mengasumsikan Anda akan menginstall Git dan Docker nanti di VPS Anda. Untuk saat ini, kita akan melakukan pengembangan dan pengujian secara lokal di komputer Anda terlebih dahulu.
- **Keamanan Webhook**: Endpoint webhook akan diamankan menggunakan mekanisme validasi signature secret key (seperti header `X-Hub-Signature-256` standar dari GitHub).

## Proposed Architecture & Workflow

### 1. Struktur Direktori Proyek
```text
/gorun
├── cmd/
│   └── gorun/
│       └── main.go           # Entry point aplikasi
├── internal/
│   ├── config/               # Parsing config.yaml
│   ├── deployer/             # Logika eksekusi Git Pull & Docker Build/Run (os/exec)
│   ├── handler/              # HTTP Handlers (Render UI & Terima Webhook)
│   └── store/                # Operasi database SQLite (history log deployment)
├── templates/
│   ├── layout.html           # Base HTML layout
│   ├── dashboard.html        # Halaman utama (list proyek)
│   └── project.html          # Detail proyek dan log deployment
├── static/
│   ├── css/
│   │   └── style.css         # Vanilla CSS (Modern, Dark Theme)
│   └── js/
│       └── htmx.min.js       # HTMX library untuk interaktivitas
├── config.example.yaml       # Contoh file konfigurasi
├── go.mod
└── go.sum
```

### 2. Alur Deployment (The Deployer)
Ketika deployment di-trigger (via klik tombol di UI atau via Webhook GitHub/GitLab):
1. **Status Update**: Gorun mencatat di SQLite bahwa status proyek menjadi `Deploying`.
2. **Git Pull**: Gorun mengeksekusi `git pull origin <branch>` di direktori proyek target pada VPS.
3. **Docker Build & Run**: Gorun mengeksekusi `docker-compose up -d --build` (sangat direkomendasikan karena rapi dan konsisten) atau perintah docker run manual jika dikonfigurasi.
4. **Log Capture**: Output terminal dari proses build (`stdout` dan `stderr`) ditangkap dan disimpan ke SQLite.
5. **Status Update**: Status diperbarui menjadi `Success` atau `Failed`.

### 3. UI/UX Design (HTMX + Vanilla CSS)
- **Desain**: Dark mode modern, fungsional, dan minim distraksi (prinsip "Less, but better").
- **Interaksi HTMX**:
  - Tombol "Deploy" akan mengirim POST request di background.
  - UI akan menampilkan indikator spinner loading (menggunakan atribut `htmx-indicator`) tanpa me-reload halaman.
  - Halaman detail proyek akan melakukan *polling* otomatis mengambil log terbaru setiap beberapa detik (`hx-trigger="every 2s"`) selama statusnya masih `Deploying`.

## Proposed Changes

Kita akan membuat project ini dari nol di dalam direktori kerja Anda saat ini.

### [NEW] Setup & Inisialisasi
- Membuat `go.mod` dan mengatur struktur folder.
- Menginstal dependency seperti driver SQLite (`github.com/mattn/go-sqlite3`) dan router HTTP jika diperlukan (misal: `github.com/go-chi/chi/v5`).

### [NEW] Komponen Backend
- Mengimplementasikan `internal/config` untuk membaca spesifikasi proyek dari file YAML.
- Mengimplementasikan `internal/store` untuk log deployment menggunakan SQLite.
- Mengimplementasikan `internal/deployer` yang aman dan tidak memblokir UI saat proses docker build berjalan lambat.
- Membuat HTTP routes di `internal/handler` untuk merender UI dan endpoint Webhook.

### [NEW] Komponen Frontend
- Membangun `layout.html`, `dashboard.html` dengan styling CSS Vanilla yang estetik, premium, dan fungsional.

## Verification Plan

### Manual Verification
1. Kita akan build dan jalankan `Gorun` secara lokal terlebih dahulu.
2. Buka antarmuka web di browser.
3. Buat sebuah proyek "dummy" di `config.yaml` yang hanya menjalankan perintah simulasi (tanpa harus build docker sungguhan pada saat testing awal).
4. Klik tombol "Deploy" dan pastikan UI bereaksi, spinner muncul, log tercetak, dan UI melakukan polling.
5. Gunakan `curl` untuk mengirim mock webhook payload dan pastikan deployment otomatis berjalan.
