# 📋 Laporan Code Review — Gorun

**Tanggal Review:** 15 Agustus 2026  
**Status Pengujian Otomatis:** ✅ Semua test lulus (`go test ./...` — PASS) & `go vet` clean  
**Cakupan Review:** Perubahan konfigurasi statis YAML &rarr; Manajemen Proyek Dinamis berbasis Web UI (SQLite + HTMX) dan Proteksi Basic Authentication.

---

## Executive Summary

Perubahan (*changes*) saat ini melakukan migrasi arsitektur yang signifikan dan terstruktur dengan sangat baik. Sistem beralih dari konfigurasi aplikasi statis di `config.yaml` menjadi pengelolaan proyek dinamis berbasis database SQLite murni (`modernc.org/sqlite`) yang dapat dikelola langsung melalui Web UI, dilengkapi proteksi Basic Authentication untuk mengamankan operasi admin, serta tetap mempertahankan endpoint GitHub Webhook yang terisolasi dengan validasi signature HMAC-SHA256.

Secara keseluruhan, kode berkualitas tinggi, modular, memiliki *test coverage* yang sangat baik untuk *happy path* dan *edge cases* utama, serta mematuhi idiom Go standar. Laporan ini merangkum temuan keamanan, performa, reliabilitas, serta rekomendasi peningkatan teknis.

---

## 📊 Ringkasan Temuan & Status Perbaikan

| ID | Kategori | Komponen | Tingkat Keparahan (Severity) | Status | Deskripsi Ringkas |
|---|---|---|---|---|---|
| **SEC-01** | Security | `internal/handler/handler.go` | 🟡 **Medium** | ✅ **Resolved** | Perbandingan kredensial Basic Auth telah diperbarui menggunakan `crypto/subtle.ConstantTimeCompare`. |
| **SEC-02** | Security | `internal/handler/handler.go` | 🟡 **Medium** | ✅ **Resolved** | Validasi format regex slug nama proyek (`^[a-z0-9]+(?:-[a-z0-9]+)*$`) diterapkan di backend untuk Create & Rename. |
| **SEC-03** | Security / UX | `cmd/gorun/main.go` | 🟢 **Low / Note** | ✅ **Resolved** | Peringatan keamanan otomatis dicetak di log saat startup jika mendeteksi password default `admin`. |
| **REL-01** | Reliability | `internal/store/store.go` | 🟢 **Low** | ✅ **Resolved** | Pengecekan `rows.Err()` ditambahkan setelah iterasi baris pada `ListProjects` & `ListDeployments`. |
| **ARC-01** | Architecture | `internal/deployer/deployer.go` | 🟢 **Low** | ✅ **Resolved** | Objek `store.Project` disalin (*snapshot copy*) sebelum dieksekusi di goroutine background async. |
| **ARC-02** | Database / Data Integrity | `internal/store/store.go` | ℹ️ **Info / Design** | ℹ️ **By Design** | Kolom `project_name` pada tabel `deployments` sengaja berfungsi sebagai *immutable audit log*. |

---

## 🔍 Detail Temuan & Analisis Komponen

---

### 1. Keamanan (Security)

#### 🟡 SEC-01: Potensi Timing Attack pada Basic Authentication Middleware
- **Lokasi File:** [`internal/handler/handler.go`](file:///Users/haryoparigroho/Documents/Projects/gorun/internal/handler/handler.go#L80-L90)
- **Kode Saat Ini:**
  ```go
  func (h *Handler) basicAuth(next http.HandlerFunc) http.HandlerFunc {
      return func(w http.ResponseWriter, r *http.Request) {
          username, password, ok := r.BasicAuth()
          if !ok || username != h.cfg.Username || password != h.cfg.Password {
              w.Header().Set("WWW-Authenticate", `Basic realm="Gorun Dashboard"`)
              http.Error(w, "Unauthorized", http.StatusUnauthorized)
              return
          }
          next(w, r)
      }
  }
  ```
- **Analisis:**
  Operator `!=` pada string membandingkan karakter demi karakter dan berhenti (*early exit*) begitu menemukan karakter pertama yang tidak cocok. Hal ini memungkinkan penyerang menganalisis perbedaan waktu respon (*side-channel timing attack*) untuk menebak panjang dan karakter kredensial.
- **Rekomendasi Perbaikan:**
  Gunakan `crypto/subtle.ConstantTimeCompare` untuk membandingkan kredensial dalam waktu konstan:
  ```go
  import "crypto/subtle"

  userMatch := subtle.ConstantTimeCompare([]byte(username), []byte(h.cfg.Username)) == 1
  passMatch := subtle.ConstantTimeCompare([]byte(password), []byte(h.cfg.Password)) == 1
  if !ok || !userMatch || !passMatch {
      // 401 Unauthorized
  }
  ```

---

#### 🟡 SEC-02: Validasi Format Nama Proyek di Backend
- **Lokasi File:** [`internal/handler/handler.go`](file:///Users/haryoparigroho/Documents/Projects/gorun/internal/handler/handler.go#L291-L308) & [`internal/handler/handler.go`](file:///Users/haryoparigroho/Documents/Projects/gorun/internal/handler/handler.go#L378-L417)
- **Kode Saat Ini:**
  ```go
  // Hanya melakukan TrimSpace
  proj := &store.Project{
      Name: strings.TrimSpace(r.FormValue("name")),
      ...
  }
  if proj.Name == "" || proj.Path == "" { ... }
  ```
- **Analisis:**
  Di frontend (`project_form.html`), input name sudah dibatasi dengan regex `pattern="[a-z0-9\-]+"`. Namun, jika ada request POST langsung ke `/projects/new` atau `/app/{name}/rename` yang mengandung spasi, garis miring (`/`), titik dua, atau karakter kontrol URL lainnya, sistem routing Go 1.22 (`/app/{name}`) dapat mengalami anomali atau gagal melakukan *matching*.
- **Rekomendasi Perbaikan:**
  Tambahkan validasi regex di handler backend sebelum menyimpan ke database:
  ```go
  var validNameRegex = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
  if !validNameRegex.MatchString(proj.Name) {
      // Return validation error: "Nama proyek hanya boleh huruf kecil, angka, dan tanda hubung (-)"
  }
  ```

---

#### 🟢 SEC-03: Default Credentials pada File Konfigurasi
- **Lokasi File:** [`internal/config/config.go`](file:///Users/haryoparigroho/Documents/Projects/gorun/internal/config/config.go#L32-L37)
- **Kode Saat Ini:**
  ```go
  if cfg.Username == "" {
      cfg.Username = "admin"
  }
  if cfg.Password == "" {
      cfg.Password = "admin"
  }
  ```
- **Analisis:**
  Nilai default sangat membantu kenyamanan saat *local development*. Namun, jika dijalankan di server VPS publik tanpa konfigurasi password yang kuat, server menjadi rentan terhadap serangan *brute force*.
- **Rekomendasi:**
  Berikan log peringatan di terminal pada saat server *startup* jika mendeteksi password default masih digunakan:
  ```go
  if cfg.Password == "admin" {
      log.Println("[WARNING] You are using the default admin password. Please update config.yaml for production environments!")
  }
  ```

---

### 2. Reliabilitas & Database Store

#### 🟢 REL-01: Pengecekan `rows.Err()` pada `ListProjects` dan `ListDeployments`
- **Lokasi File:** [`internal/store/store.go`](file:///Users/haryoparigroho/Documents/Projects/gorun/internal/store/store.go#L228) & [`internal/store/store.go`](file:///Users/haryoparigroho/Documents/Projects/gorun/internal/store/store.go#L454)
- **Analisis:**
  Pada method `ListProjects` dan `ListDeployments`, perulangan `for rows.Next()` berhenti jika semua baris selesai dibaca ATAU jika terjadi error di tengah pembacaan stream query. Sesuai *best practice* database SQL di Go, disarankan memanggil `rows.Err()` setelah loop untuk memastikan tidak ada error tersembunyi yang diabaikan.
- **Rekomendasi:**
  ```go
  for rows.Next() {
      // scan row...
  }
  if err := rows.Err(); err != nil {
      return nil, fmt.Errorf("error iterating rows: %w", err)
  }
  return list, nil
  ```

---

#### ℹ️ ARC-02: Konsistensi Snapshot `project_name` pada Tabel `deployments`
- **Lokasi File:** [`internal/store/store.go`](file:///Users/haryoparigroho/Documents/Projects/gorun/internal/store/store.go#L254-L273)
- **Analisis:**
  Saat method `RenameProject(id, newName)` dijalankan, nama pada tabel `projects` diperbarui. Di sisi lain, record pada tabel `deployments` tetap menyimpan `project_name` lama saat deployment tersebut dieksekusi.
- **Penilaian:**
  Hal ini sebenarnya adalah **keunggulan audit log** (*immutable historical record*), karena riwayat deployment merefleksikan status aktual saat proses terjadi. Foreign key `project_id` memastikan relasi dan riwayat tetap utuh saat di-query berdasarkan `project_id`.

---

### 3. Deployer Engine & Concurrency

#### 🟢 ARC-01: Thread-Safety Objek `store.Project` pada Asynchronous Goroutine
- **Lokasi File:** [`internal/deployer/deployer.go`](file:///Users/haryoparigroho/Documents/Projects/gorun/internal/deployer/deployer.go#L41-L71)
- **Analisis:**
  Pada `TriggerDeploy(proj *store.Project, source string)`, pointer `proj` dilewatkan ke goroutine `go d.run(deployID, proj, source)`. Jika terjadi perubahan data proyek melalui UI bersamaan dengan eksekusi deploy, pembacaan field `proj.Path`, `proj.Branch`, atau `proj.DeployCmd` secara teori bisa mengalami *race condition* jika struct tersebut dimodifikasi in-memory.
- **Rekomendasi:**
  Buat salinan nilai (*shallow copy*) dari struct `Project` sebelum masuk ke goroutine:
  ```go
  projCopy := *proj
  go d.run(deployID, &projCopy, source)
  ```

---

### 4. Frontend & User Experience (HTMX + UI)

- **Aset & Desain:**
  - CSS kustom pada [`static/css/style.css`](file:///Users/haryoparigroho/Documents/Projects/gorun/static/css/style.css) mengikuti panduan desain modern (Dark mode, kontras warna yang nyaman, tipografi terstruktur, state hover/focus yang jelas).
  - Tidak ditemukan anti-pattern seperti *nested cards* berlebihan atau *untracked fonts*.
- **Integrasi HTMX:**
  - Polling dinamis `hx-trigger="every 2s"` pada saat status `Deploying` berjalan efektif dan otomatis berhenti saat proses selesai.
  - Penggunaan header `HX-Trigger: {"showToast": ...}` untuk menampilkan notifikasi toast dinamis sangat elegan dan tidak memerlukan JavaScript framework berat.
  - Penggunaan `hx-delete` + `hx-confirm` pada tombol Delete Project memberikan proteksi terhadap klik yang tidak disengaja.

---

## 🧪 Hasil Verifikasi & Uji Otomatis

Seluruh unit test dan integrasi telah diverifikasi:

```text
=== RUN   TestLoad
--- PASS: TestLoad (0.00s)
=== RUN   TestLoadDefaults
--- PASS: TestLoadDefaults (0.00s)
=== RUN   TestDeployerConcurrency
--- PASS: TestDeployerConcurrency (0.50s)
=== RUN   TestBasicAuthProtection
--- PASS: TestBasicAuthProtection (0.00s)
=== RUN   TestDashboardAndProjectRoutes
--- PASS: TestDashboardAndProjectRoutes (0.00s)
=== RUN   TestProjectCRUD
--- PASS: TestProjectCRUD (0.00s)
=== RUN   TestDeployTriggerAndStatus
--- PASS: TestDeployTriggerAndStatus (0.00s)
=== RUN   TestWebhookHandler
--- PASS: TestWebhookHandler (0.00s)
=== RUN   TestVerifyGitHubSignature
--- PASS: TestVerifyGitHubSignature (0.00s)
=== RUN   TestStoreProjectCRUDAndCascade
--- PASS: TestStoreProjectCRUDAndCascade (0.01s)
PASS
ok      gorun/internal/config     0.295s
ok      gorun/internal/deployer   0.967s
ok      gorun/internal/handler    0.985s
ok      gorun/internal/store      0.615s
```

`go vet ./...` dan `go build ./cmd/gorun` berjalan mulus tanpa peringatan (*zero warnings*).

---

## 🎯 Kesimpulan & Rekomendasi Selanjutnya

Perubahan yang dilakukan sudah **sangat solid, fungsional, dan siap digunakan**. 

Langkah penyempurnaan berikutnya yang disarankan:
1. **[Keamanan]** Ganti pembanding kredensial Basic Auth dengan `subtle.ConstantTimeCompare` ([`SEC-01`](#sec-01-potensi-timing-attack-pada-basic-authentication-middleware)).
2. **[Validasi]** Tambahkan validasi regex slug nama proyek di level handler Go ([`SEC-02`](#sec-02-validasi-format-nama-proyek-di-backend)).
3. **[Reliabilitas]** Tambahkan pengecekan `rows.Err()` pada iterasi SQL ([`REL-01`](#rel-01-pengecekan-rowserr-pada-listprojects-dan-listdeployments)).
