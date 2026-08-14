## Goal Description
Dokumen ini merincikan rencana implementasi khusus untuk sisi frontend dari aplikasi **Gorun**. Frontend akan dibangun tanpa kerangka SPA (seperti React/Vue), melainkan menggunakan **HTML Server-Side Rendering (Go Templates)** yang diperkaya dengan **HTMX** untuk interaksi dinamis tanpa me-reload halaman, dan **Vanilla CSS** untuk desain visual yang modern, premium, dan responsif.

## Design Decisions
1. **Pewarnaan Log**: Output log build/docker akan menggunakan format teks **monokrom** dalam kotak kode standar agar desain tetap simpel dan elegan untuk versi awal.
2. **Notifikasi Global**: Kita akan menggunakan **Toast Notification** di sudut layar untuk menampilkan notifikasi global terkait status sistem (misalnya: "Deployment Started", "Deployment Successful").

---

## Proposed Changes

### 1. Struktur Layout & Templating
Menggunakan library template bawaan Go `html/template`.
- **`templates/layout.html`**:
  Berisi struktur dasar dokumen HTML5, integrasi library eksternal (htmx), meta tag responsif, dan `<main>` tag tempat konten lain disuntikkan.
- **`templates/dashboard.html`**:
  Halaman depan yang berisi *Grid* list semua aplikasi yang telah dikonfigurasi di `config.yaml`.
- **`templates/project.html`**:
  Halaman detail satu aplikasi, menampilkan riwayat deployment sebelumnya dan layar terminal/konsol log secara real-time.

---

### 2. Styling (Vanilla CSS)
Akan dibuat file `static/css/style.css` tanpa menggunakan TailwindCSS, sesuai panduan desain estetika web.
- **Palet Warna**: Menggunakan HSL. Fokus pada *Dark Mode* premium.
  - Background: Sangat gelap (misal: `#0a0a0a`).
  - Container/Card: Abu-abu gelap dengan sedikit transparansi/border (misal: `#171717`, border `#262626`).
  - Aksen Warna Utama (Tombol Deploy): Cyan/Biru modern atau warna brand Go (misal: `#00ADD8`).
  - Teks: Abu-abu terang kontras (misal: `#EDEDED`, sekunder `#A3A3A3`).
- **Tipografi**: Menggunakan sistem font *sans-serif* modern dan bersih seperti **Inter** atau system-ui, khusus untuk log terminal menggunakan font *monospace* seperti JetBrains Mono atau Fira Code.
- **Animasi & Interaksi**: Menambahkan transisi halus pada *hover state* tombol dan *fade-in* saat memuat konten.

---

### 3. Interaksi HTMX (The Magic)

Alih-alih menggunakan JavaScript berat, HTMX akan menangani permintaan asinkronus dengan markup HTML sederhana:

#### Toast Notification (Global)
Komponen Toast Notification diatur di dalam `layout.html`. Backend dapat memicu munculnya toast (contoh: "Deploying...") dengan memanfaatkan kapabilitas event HTMX (seperti mengirim response header `HX-Trigger`) setelah tombol deploy diklik.

#### Trigger Deployment (Tombol Deploy)
Di dalam `templates/project.html`:
```html
<button 
    hx-post="/app/{{.AppName}}/deploy" 
    hx-target="#deployment-status" 
    hx-swap="innerHTML"
    class="btn-primary">
    Deploy Now
    <span class="htmx-indicator loader"></span> <!-- Muncul saat loading -->
</button>
```

#### Live Status Polling (Melihat Log Realtime)
Ketika deployment sedang berjalan, kita perlu melihat output terminal. HTMX akan melakukan *polling* otomatis hanya ketika statusnya adalah "Deploying".

```html
<!-- Fragmen ini akan di-replace oleh respon backend secara berkala -->
<div id="deployment-status" 
     {{if eq .Status "Deploying"}} 
     hx-get="/app/{{.AppName}}/status" 
     hx-trigger="every 2s" 
     hx-swap="outerHTML" 
     {{end}}>
     
    <h3>Status: <span class="badge {{.Status}}">{{.Status}}</span></h3>
    <pre class="terminal-log"><code>{{.Logs}}</code></pre>
</div>
```
*Catatan*: Jika statusnya telah menjadi `Success` atau `Failed`, backend tidak akan lagi mengirim atribut `hx-trigger="every 2s"`, sehingga siklus *polling* akan berhenti otomatis dan menghemat resource (Server & Klien).

## Verification Plan
- **Automated Tests**: Tidak memerlukan pengujian otomatis khusus frontend untuk versi awal ini (karena logic dirender dari server).
- **Manual Verification**: 
  - Membuka halaman pada ukuran layar *desktop* dan *mobile* untuk memastikan responsivitas.
  - Menekan tombol **Deploy** dan memastikan indikator *loading* (spinner) muncul.
  - Memastikan teks log terminal secara otomatis ditambahkan/diperbarui (*auto-scroll* ke bawah sangat disarankan via sedikit JS kustom jika panjang).
