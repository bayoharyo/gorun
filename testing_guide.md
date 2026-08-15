# Step-by-Step Testing Guide - Gorun

Dokumen ini menyediakan panduan lengkap dan terstruktur untuk menguji seluruh fitur **Gorun** (termasuk antarmuka manajemen proyek via UI dan proteksi Basic Authentication) secara lokal di komputer Anda.

---

## 1. Persiapan File Konfigurasi (`config.yaml`)

File `config.yaml` kini difokuskan untuk port server dan kredensial login Basic Auth:

```yaml
# Gorun Server Configuration
port: 8080
username: "admin"
password: "adminpassword"
```

---

## 2. Menjalankan Server Gorun

Jalankan server dari root direktori proyek di terminal Anda:

```bash
./gorun
```
*Atau jika ingin menjalankan langsung dari source code Go:*
```bash
go run cmd/gorun/main.go
```

**Output terminal yang diharapkan:**
```text
[INFO] Starting Gorun...
[INFO] Configuration loaded (port 8080, auth user "admin")
[INFO] SQLite store initialized at "gorun.db"
[INFO] Gorun server listening on http://localhost:8080 (Port 8080)
```

---

## 3. Skenario Pengujian

### Skenario A: Proteksi Basic Authentication
1. Buka browser dan kunjungi: **`http://localhost:8080`**.
2. **Hal yang perlu diverifikasi:**
   - Browser memunculkan prompt dialog login / Basic Auth ("Sign in to access this site").
   - Jika memasukkan username/password salah, akses ditolak (`401 Unauthorized`).
   - Masukkan username `admin` dan password `adminpassword` untuk masuk ke Dashboard.

---

### Skenario B: Membuat Proyek Baru via UI (Create Project)
1. Pada halaman Dashboard, klik tombol **`+ Add Project`**.
2. Anda akan diarahkan ke form pembuatan proyek di `http://localhost:8080/projects/new`.
3. Isi formulir dengan data simulasi:
   - **Project Name**: `demo-app`
   - **Project Working Directory (Path)**: `./`
   - **Git Branch**: `main`
   - **Webhook Secret Key**: `secret-key-12345`
   - **Deploy Command**:
     ```bash
     echo '==> Pulling dependencies...'; sleep 1; echo '==> Building binary...'; sleep 1; echo '==> Restarting services...'; sleep 1; echo '==> Deployment finished successfully!'
     ```
4. Klik tombol **`Create Project`**.
5. **Verifikasi:** Anda otomatis dialihkan kembali ke Dashboard dan kartu proyek `demo-app` sudah tampil.

---

### Skenario C: Detail Proyek & Live Terminal Log (HTMX Polling)
1. Klik tombol **View Details & Logs** pada kartu `demo-app` (atau buka `http://localhost:8080/app/demo-app`).
2. Klik tombol **Deploy Now** di sudut kanan atas.
3. **Hal yang terjadi secara otomatis:**
   - **Toast Notifikasi** muncul di sudut kanan bawah: *"Deployment triggered for demo-app"*.
   - Status badge langsung berubah menjadi **`Deploying...`** (dengan efek animasi pulse kuning).
   - Layar konsol terminal Unix (kotak hitam dengan 3 dots window) otomatis melakukan **polling setiap 2 detik**.
   - Teks log bertambah secara bertahap (`Pulling dependencies...` -> `Building binary...` -> `Restarting services...`).
   - Layar terminal otomatis **scroll ke bawah** saat log baru masuk.
   - Setelah 3-4 detik, status badge otomatis berubah menjadi **`Success`** (hijau), dan polling HTMX berhenti otomatis.

---

### Skenario D: Mengubah Konfigurasi (Edit Config) & Mengganti Nama Proyek (Rename)
1. Di halaman detail `demo-app`, klik tombol **`Edit Config`**.
2. Ubah Git Branch menjadi `develop` dan klik **`Save Configuration`**.
   - Verifikasi branch terupdate di halaman detail.
3. Kembali klik **`Edit Config`**, lalu pada bagian **Rename Project** (kotak berbingkai kuning di bawah):
   - Ubah nama menjadi `renamed-app` dan klik **`Rename Project`**.
4. **Verifikasi:** URL halaman detail otomatis berpindah ke `http://localhost:8080/app/renamed-app` dan riwayat deployment tetap tersimpan.

---

### Skenario E: Concurrency Protection (Locking)
1. Klik tombol **Deploy Now**.
2. Saat status masih **`Deploying...`**, segera klik tombol **Deploy Now** sekali lagi.
3. **Hal yang diverifikasi:**
   - Muncul **Toast Warning** berwarna kuning: *"Deployment is already running for this app!"*.
   - Tidak terjadi penumpukan proses atau *race condition*.

---

### Skenario F: Automated Deployment via GitHub Webhook
Webhook Gorun **tidak terhalang oleh Basic Auth** dan diamankan dengan signature HMAC-SHA256.

Buka jendela terminal baru dan jalankan perintah `curl` berikut:

```bash
SECRET="secret-key-12345"
PAYLOAD='{"ref":"refs/heads/main","repository":{"name":"renamed-app"}}'
SIGNATURE="sha256=$(printf '%s' "$PAYLOAD" | openssl dgst -sha256 -hmac "$SECRET" | sed 's/^.* //')"

curl -X POST "http://localhost:8080/webhook/renamed-app" \
  -H "Content-Type: application/json" \
  -H "X-Hub-Signature-256: $SIGNATURE" \
  -d "$PAYLOAD"
```

**Hasil respons yang diharapkan di terminal:**
```json
{"deployment_id":"<uuid>","message":"Deployment started","status":"accepted"}
```

**Verifikasi di Web UI:**
- Buka atau refresh halaman `http://localhost:8080/app/renamed-app`.
- Pada panel **Recent Deployments** di sebelah kanan, akan tercatat entri baru dengan keterangan `via webhook`.

---

### Skenario G: Menghapus Proyek (Cascade Delete)
1. Di halaman detail proyek `renamed-app`, klik tombol **`Delete`**.
2. Browser memunculkan dialog konfirmasi: *"Are you sure you want to delete 'renamed-app'? All deployment history will also be permanently deleted."*.
3. Klik **OK**.
4. **Verifikasi:** Browser otomatis dialihkan ke Dashboard utama dan proyek beserta seluruh riwayat deployment-nya telah terhapus bersih dari database.

---

## 4. Menjalankan Automated Tests Suite

Anda dapat memverifikasi seluruh komponen secara otomatis dengan menjalankan:

```bash
go test -v ./...
```
