# Step-by-Step Testing Guide - Gorun

Dokumen ini menyediakan panduan lengkap dan terstruktur untuk menguji seluruh fitur **Gorun** secara lokal di komputer Anda tanpa memerlukan VPS atau Docker sungguhan (menggunakan simulasi command).

---

## 1. Persiapan File Konfigurasi (`config.yaml`)

Gorun membaca daftar aplikasi dari file `config.yaml`. Pastikan file `config.yaml` sudah ada dan berisi konfigurasi simulasi berikut:

```yaml
# Gorun Server Configuration
port: 8080

apps:
  demo-app:
    path: "./"
    branch: "main"
    webhook_secret: "secret-key-12345"
    deploy_cmd: "echo '==> Pulling dependencies...'; sleep 1; echo '==> Building binary...'; sleep 1; echo '==> Restarting services...'; sleep 1; echo '==> Deployment finished successfully!'"
```

> **Catatan**: `deploy_cmd` di atas menggunakan perintah `sleep` dan `echo` sehingga Anda dapat melihat streaming log terminal HTMX secara langsung selama 3-4 detik.

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
[INFO] Loaded configuration with 1 application(s)
[INFO] SQLite store initialized at "gorun.db"
[INFO] Gorun server listening on http://localhost:8080 (Port 8080)
```

---

## 3. Skenario Pengujian

### Skenario A: Akses Dashboard & Tampilan Awal
1. Buka browser dan kunjungi: **`http://localhost:8080`**
2. **Hal yang perlu diverifikasi:**
   - Halaman menampilkan kartu **`demo-app`**.
   - Badge status awal menunjukkan `Idle` (atau `Never Deployed`).
   - Terdapat info path `./`, branch `main`, dan tombol **View Details & Logs** serta tombol **Deploy**.

---

### Skenario B: Detail Proyek & Live Terminal Log (HTMX Polling)
1. Klik tombol **View Details & Logs** pada kartu `demo-app` (atau langsung buka `http://localhost:8080/app/demo-app`).
2. Klik tombol **Deploy Now** di sudut kanan atas.
3. **Hal yang terjadi secara otomatis:**
   - **Toast Notifikasi** muncul di sudut kanan bawah: *"Deployment triggered for demo-app"*.
   - Status badge langsung berubah menjadi **`Deploying...`** (dengan efek animasi pulse kuning).
   - Layar konsol terminal Unix (kotak hitam dengan 3 dots window) otomatis melakukan **polling setiap 2 detik**.
   - Teks log bertambah secara bertahap (`Pulling dependencies...` -> `Building binary...` -> `Restarting services...`).
   - Layar terminal otomatis **scroll ke bawah** saat log baru masuk.
   - Setelah 3-4 detik, status badge otomatis berubah menjadi **`Success`** (hijau), dan polling HTMX berhenti otomatis.

---

### Skenario C: Concurrency Protection (Locking)
1. Klik tombol **Deploy Now**.
2. Saat status masih **`Deploying...`**, segera klik tombol **Deploy Now** sekali lagi.
3. **Hal yang diverifikasi:**
   - Muncul **Toast Warning** berwarna kuning: *"Deployment is already running for this app!"*.
   - Tidak terjadi penumpukan proses atau *race condition*.

---

### Skenario D: Automated Deployment via GitHub Webhook
Gorun menyediakan endpoint webhook yang diamankan dengan HMAC-SHA256 signature standar GitHub.

Buka jendela terminal baru dan jalankan perintah `curl` berikut:

```bash
SECRET="secret-key-12345"
PAYLOAD='{"ref":"refs/heads/main","repository":{"name":"demo-app"}}'
SIGNATURE="sha256=$(printf '%s' "$PAYLOAD" | openssl dgst -sha256 -hmac "$SECRET" | sed 's/^.* //')"

curl -X POST "http://localhost:8080/webhook/demo-app" \
  -H "Content-Type: application/json" \
  -H "X-Hub-Signature-256: $SIGNATURE" \
  -d "$PAYLOAD"
```

**Hasil respons yang diharapkan di terminal:**
```json
{"deployment_id":"<uuid>","message":"Deployment started","status":"accepted"}
```

**Verifikasi di Web UI:**
- Buka atau refresh halaman `http://localhost:8080/app/demo-app`.
- Pada panel **Recent Deployments** di sebelah kanan, akan tercatat entri baru dengan keterangan `via webhook`.

---

### Skenario E: Penanganan Error / Deploy Gagal
Jika ingin menguji bagaimana Gorun menangani perintah yang error/gagal:
1. Ubah sementara `deploy_cmd` di `config.yaml` menjadi:
   ```yaml
   deploy_cmd: "echo 'Starting build...'; sleep 1; echo 'Fatal syntax error!'; exit 1"
   ```
2. Restart `./gorun`.
3. Klik **Deploy Now** di UI.
4. Status badge akan berubah menjadi **`Failed`** (merah), dan pesan error tercatat di konsol terminal.

---

## 4. Reset Data Pengujian (Opsional)
Jika Anda ingin menghapus seluruh riwayat deployment dan memulai dari database kosong:
```bash
rm gorun.db
```
Server akan otomatis membuat file database SQLite baru yang bersih saat pertama kali dijalankan kembali.
