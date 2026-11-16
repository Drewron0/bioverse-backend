# Panduan Lengkap Setup & Checklist Developer BioVerse Backend

Dokumen ini berisi panduan langkah demi langkah yang **perlu Anda lakukan secara mandiri** untuk menghubungkan repositori ke GitHub, menyiapkan alat bantu lokal, menjalankan server backend, serta menyelesaikan persiapan eksternal (Fase 0).

---

## 1. Setup GitHub (Langkah Wajib Pertama)

Di folder lokal saat ini, Git sudah diinisialisasi dan commit fondasi awal sudah dibuat. Namun, Anda perlu menautkannya ke akun GitHub Anda.

### Langkah-langkah:
1. Buka [GitHub](https://github.com) dan login ke akun GitHub Anda.
2. Buat repositori baru:
   - **Repository name**: `bioverse-backend`
   - **Visibility**: `Private` (direkomendasikan karena backend memuat logika bisnis dan schema enterprise) atau `Public`.
   - **Initialize this repository with**: **JANGAN** centang README, .gitignore, atau license (karena kita sudah punya di lokal).
   - Klik **Create repository**.
3. Di terminal lokal (PowerShell pada root folder `bioverse-backend`), jalankan perintah berikut:
   ```powershell
   git remote add origin https://github.com/Drewron0/bioverse-backend.git
   git branch -M main
   git push -u origin main
   ```
4. **Pengaturan Rekomendasi di GitHub**:
   - Masuk ke **Settings** > **Branches** di repositori GitHub Anda.
   - Tambahkan branch protection rule untuk branch `main`:
     - Centang *Require a pull request before merging*.
     - Centang *Require status checks to pass before merging* (setelah CI GitHub Actions aktif).

---

## 2. Setup Perangkat Lunak Lokal (Local Tools)

Pastikan dependensi berikut terpasang dan aktif di komputer Anda:

### 2.1. Docker Desktop (Untuk PostgreSQL 16 & MinIO S3)
- Buka aplikasi **Docker Desktop** di Windows dan pastikan statusnya sudah **Running** (ikon paus berwarna hijau di taskbar).
- Verifikasi di terminal:
  ```powershell
  docker ps
  ```

### 2.2. Goose (CLI Migration Database)
Goose digunakan untuk mengeksekusi migration SQL (`db/migrations/`).
- Install goose via Go:
  ```powershell
  go install github.com/pressly/goose/v3/cmd/goose@latest
  ```
- Pastikan folder `$HOME/go/bin` sudah masuk ke dalam PATH Windows Anda:
  ```powershell
  goose -version
  ```

### 2.3. SQLC (Type-Safe Query Generator)
SQLC menghasilkan Go struct dan method query dari SQL mentah tanpa ORM.
- Install sqlc via Go:
  ```powershell
  go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
  ```
- Verifikasi instalasi:
  ```powershell
  sqlc version
  ```

### 2.4. Air (Opsional tapi Direkomendasikan untuk Live-Reload)
Memungkinkan server otomatis restart setiap kali Anda mengubah file `.go`:
```powershell
go install github.com/air-verse/air@latest
```

---

## 3. Cara Menjalankan Backend Secara Lokal

Setelah Docker Desktop berjalan dan tools terpasang:

### Langkah 1: Jalankan Database dan Object Storage
Jalankan PostgreSQL 16 dan MinIO via Docker Compose:
```powershell
docker compose -f deploy/docker-compose.yml up -d
```
- PostgreSQL berjalan di port `5432` (database: `bioverse`, user: `bioverse`, password: `bioverse_dev`).
- MinIO API berjalan di port `9000`, dan MinIO Web Console di `http://localhost:9001` (user: `bioverse_minio`, password: `bioverse_minio_secret`). Bucket `bioverse-media` akan dibuat otomatis.

### Langkah 2: Jalankan Migrasi Database
Jalankan migrasi enum (00001):
```powershell
goose -dir db/migrations postgres "postgres://bioverse:bioverse_dev@127.0.0.1:5432/bioverse?sslmode=disable" up
```

### Langkah 3: Jalankan API Server
Jalankan server HTTP:
```powershell
go run cmd/api/main.go
```
Server akan aktif di `http://localhost:8080`.

### Langkah 4: Uji Endpoint Health
Buka terminal baru atau browser untuk memastikan backend berjalan dengan baik:
```powershell
curl http://localhost:8080/healthz
curl http://localhost:8080/v1/health
```
Respons `/v1/health` akan menampilkan status `ok` dan database `connected`.

### Langkah 5: Jalankan Pengujian Otomatis (Unit Tests)
```powershell
go test -v ./...
```

---

## 4. Checklist Persiapan Non-Kode (Fase 0 - Mulai Sekarang)

Sesuai roadmap, beberapa integrasi eksternal memiliki masa tunggu verifikasi yang lama. Segera lakukan pendaftaran berikut:

- [ ] **1. Xendit (Payment Gateway MVP)**
  - Daftar akun di [Xendit Dashboard](https://dashboard.xendit.co/register).
  - Mulai proses verifikasi bisnis (**KYB**) karena membutuhkan waktu verifikasi beberapa hari hingga minggu.
  - Masuk ke menu **Settings > API Keys** pada mode **Test/Sandbox**, salin Secret Key dan masukkan ke `.env` (`XENDIT_SECRET_KEY`).
  - Uji simulasi QRIS dan Virtual Account di sandbox.

- [ ] **2. Firebase Cloud Messaging (FCM)**
  - Buat project baru di [Firebase Console](https://console.firebase.google.com) bernama `BioVerse`.
  - Masuk ke **Project Settings > Service Accounts**, generate private key dalam format `.json`.
  - Simpan file kredensial tersebut dengan aman dan catat lokasinya untuk disetel di `.env` (`FCM_CREDENTIALS_JSON_PATH`).

- [ ] **3. Google Cloud Console (OAuth 2.0)**
  - Masuk ke [Google Cloud Console](https://console.cloud.google.com).
  - Buat project `BioVerse`.
  - Buka **APIs & Services > Credentials** > Buat OAuth 2.0 Client ID untuk:
    - Web Application (digunakan juga untuk verifikasi token backend).
    - Android Client (menggunakan SHA-1 certificate dari Flutter).
    - iOS Client (menggunakan iOS Bundle ID).
  - Masukkan Client ID ke `.env` (`GOOGLE_CLIENT_ID`).

- [ ] **4. Akun Pengembang Apple & Google Play (Untuk IAP)**
  - Pastikan organisasi Anda memiliki akun **Apple Developer Program** dan **Google Play Console**.
  - Catat Bundle ID aplikasi Flutter (misal: `com.bioverse.app`).

- [ ] **5. Layanan Email Transaksional (Untuk Verifikasi Akun & Reset Password)**
  - Siapkan akun penyedia SMTP/API seperti Amazon SES, Resend, atau SendGrid untuk mengirimkan email aktivasi pada Fase 2.

- [ ] **6. Sinkronisasi dengan Tim UI/UX (Figma)**
  - Tanyakan ke tim Figma terkait 3 kotak informasi di halaman *Mission Details*.
  - Konfirmasikan daftar kategori misi yang akan digunakan.
  - Konfirmasikan alur layar Admin Back Office dan Operator Lapangan.

---

## 5. Ringkasan File Fondasi yang Sudah Dibuat

1. `.gitignore` & `.env.example` (Konfigurasi standar, proteksi rahasia).
2. `deploy/docker-compose.yml` (Postgres 16 + MinIO S3 + auto bucket creation).
3. `db/migrations/00001_enums.sql` (36 Enum sesuai ERD Section 7.3).
4. `sqlc.yaml` (Konfigurasi code generation SQL tanpa ORM).
5. `internal/platform/config` (Fail-fast environment parser dengan struct terisolasi).
6. `internal/platform/logger` (Structured logger berbasis `log/slog`).
7. `internal/platform/database` (Connection pool `pgxpool` + transaction helper `WithTx`).
8. `internal/platform/httperr` (Standard response error RFC 7807 `problem+json`).
9. `internal/platform/middleware` (RequestID, Slog Logger, Panic Recoverer, CORS).
10. `cmd/api/main.go` & `cmd/worker/main.go` (Entrypoint server HTTP & background worker).
11. `api/openapi.yaml` (Spesifikasi OpenAPI Contract-First untuk tim frontend).
12. `Makefile` (Shortcut command developer).
