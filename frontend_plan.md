# Frontend Implementation Plan - Gorun

Dokumen ini merincikan spesifikasi teknis lengkap untuk sisi frontend aplikasi **Gorun**. Frontend dibangun menggunakan **HTML Server-Side Rendering (Go Templates)** yang diperkaya dengan **HTMX** untuk interaksi dinamis tanpa me-reload halaman, dan **Vanilla CSS** untuk tampilan modern, premium, dan responsif.

---

## 1. Arsitektur & Data Contracts

Untuk memastikan sinkronisasi antara backend dan frontend tanpa ambiguitas, template frontend mengacu pada *data contracts* berikut:

### Struct View Model Backend
```go
// Data yang dikirim ke templates/dashboard.html
type DashboardData struct {
    Apps []AppSummary
}

type AppSummary struct {
    Name             string
    Config           config.AppConfig
    LatestDeployment *store.Deployment // nil jika belum pernah deploy
    IsDeploying      bool
}

// Data yang dikirim ke templates/project.html
type ProjectData struct {
    AppName          string
    Config           config.AppConfig
    LatestDeployment *store.Deployment
    History          []*store.Deployment
    IsDeploying      bool
    Host             string // misal "localhost:8080"
}

// Data yang dikirim ke templates/status_fragment.html (HTMX polling)
type StatusFragmentData struct {
    AppName          string
    LatestDeployment *store.Deployment
    IsDeploying      bool
}
```

### Template Helper Functions (`template.FuncMap`)
- `formatTime(t time.Time) string`: Format tanggal human-readable (`2006-01-02 15:04:05`) atau `"Never"` jika zero time.
- `formatDuration(start, end time.Time) string`: Menghitung durasi proses deployment.
- `statusClass(status store.Status) string`: Mengembalikan nama class CSS: `status-success`, `status-failed`, `status-deploying`, `status-idle`.

---

## 2. Struktur File Frontend

```text
static/
├── css/
│   └── style.css            # Vanilla CSS dengan Dark Slate Theme & Design Tokens
└── js/
    └── htmx.min.js          # Library HTMX v1.9.10+
templates/
├── dashboard.html           # Full page: Grid daftar aplikasi yang terkonfigurasi
├── project.html             # Full page: Detail proyek, webhook info, history & live console
└── status_fragment.html     # Fragment HTML: Status badge & console log untuk HTMX polling
```

---

## 3. Matriks Interaksi HTMX & Event

| Pemicu (Trigger) | Endpoint & Method | Jenis Request | Target DOM | Swap Type | Payload / Respons |
|---|---|---|---|---|---|
| Buka Dashboard | `GET /` | Full Page | `window` | Standard | Render `dashboard.html` |
| Buka Detail Proyek | `GET /app/{name}` | Full Page | `window` | Standard | Render `project.html` |
| Klik Tombol "Deploy" | `POST /app/{name}/deploy` | HTMX POST | `#deployment-status` | `outerHTML` | Render `status_fragment.html` + Header `HX-Trigger` |
| Live Polling Status & Log | `GET /app/{name}/status` | HTMX GET (every 2s) | `#deployment-status` | `outerHTML` | Render `status_fragment.html` |

### Toast Notification Protocol via `HX-Trigger`
Backend menyertakan header HTTP saat merespons trigger deploy:
```http
HX-Trigger: {"showToast": {"message": "Deployment triggered for my-app", "type": "info"}}
```

Client script di dalam template menangkap custom event `showToast`:
```javascript
document.body.addEventListener('showToast', function(evt) {
    const data = evt.detail;
    const container = document.getElementById('toast-container');
    if (!container) return;

    const toast = document.createElement('div');
    toast.className = 'toast toast-' + (data.type || 'info');
    toast.textContent = data.message;
    container.appendChild(toast);

    setTimeout(() => {
        toast.classList.add('toast-fadeout');
        setTimeout(() => toast.remove(), 400);
    }, 3000);
});
```

---

## 4. Desain Sistem (Vanilla CSS)

File `static/css/style.css` menggunakan CSS Variables:
- **Background Utama**: `--bg-primary: #0a0e17;`, `--bg-secondary: #121824;`, `--bg-surface: #1a2234;`
- **Warna Aksen**: Go Blue `--accent-blue: #00add8;` dengan hover `--accent-blue-hover: #0090b5;`
- **Status Colors**:
  - `Success`: Green (`#34d399`, bg `rgba(16, 185, 129, 0.15)`)
  - `Failed`: Red (`#f87171`, bg `rgba(239, 68, 68, 0.15)`)
  - `Deploying`: Yellow/Amber (`#fbbf24`, bg `rgba(245, 158, 11, 0.15)`, dengan animasi `pulse`)
  - `Idle`: Slate (`#94a3b8`, bg `rgba(148, 163, 184, 0.15)`)
- **Komponen Terminal**: Styling konsol Unix hitam pekat (`#0d1117`) dengan window control dots (merah, kuning, hijau) dan auto-scroll log.

---

## 5. Verification Plan

### Automated Verification
- Pastikan build Go dan asset handler melayani static file tanpa error:
  - `curl -I http://localhost:8080/static/css/style.css` (200 OK)
  - `curl -I http://localhost:8080/static/js/htmx.min.js` (200 OK)

### Manual Verification
1. **Dashboard UI**:
   - Buka `http://localhost:8080/` -> Memastikan kartu proyek tampil rapi dalam layout CSS grid.
   - Memastikan badge status awal tampil ("Idle" atau status deploy terakhir).
2. **Deploy Trigger & Polling**:
   - Buka `http://localhost:8080/app/{name}`.
   - Klik tombol **Deploy Now**.
   - Verifikasi Toast notifikasi muncul di pojok kanan bawah.
   - Verifikasi badge berubah menjadi `Deploying...` (berkedip/pulse) dan konsol log melakukan polling setiap 2 detik.
   - Pastikan teks log di terminal otomatis scroll ke baris paling bawah.
   - Setelah deploy selesai, pastikan atribut `hx-trigger="every 2s"` hilang sehingga polling berhenti.
3. **Responsivitas**:
   - Uji tampilan pada viewport mobile (375px) dan desktop (1200px).
