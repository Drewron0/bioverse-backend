# Panduan Tindakan Khusus Developer (Tugas Non-Kode & 100% Gratis)

Dokumen ini berisi langkah-langkah spesifik yang **hanya bisa dilakukan oleh Anda sebagai manusia** (karena memerlukan login ke akun pribadi/organisasi Anda). Semua langkah di bawah ini **100% GRATIS** (tidak perlu kartu kredit/bayar).

---

## Ringkasan Parameter yang Perlu Anda Dapatkan & Masukkan ke `.env`

| Parameter di `.env` | Layanan | Status Biaya | Kegunaan |
|---|---|---|---|
| `XENDIT_SECRET_KEY` | Xendit Sandbox | **Gratis** | Simulasi pembayaran QRIS & Virtual Account |
| `GOOGLE_CLIENT_ID` | Google Cloud Console | **Gratis** | Autentikasi "Login with Google" di Flutter & Web |
| `FCM_CREDENTIALS_JSON_PATH` | Firebase Console | **Gratis** | Pengiriman push notifikasi ke HP pengguna |
| `RESEND_API_KEY` | Resend.com | **Gratis** (3.000 email/bln) | Pengiriman email verifikasi & reset password |

---

## 1. Xendit Sandbox (Payment Gateway Simulasi)

Mode Sandbox memungkinkan pengujian checkout, pembuatan QRIS dinamis, nomor Virtual Account Bank Mandiri/BCA/BRI, dan simulasi pembayaran lunas tanpa uang riil.

### Langkah-langkah:
1. Buka [dashboard.xendit.co/register](https://dashboard.xendit.co/register) dan daftar akun (bisa pakai email pribadi/kampus).
2. Setelah masuk ke dashboard, lihat tombol toggle di **kanan atas**: pastikan posisinya berada di **Test Mode / Sandbox** (bukan Live).
3. Di menu sidebar kiri, buka **Settings** $\rightarrow$ sub-menu **API Keys**.
4. Klik tombol **Generate Secret Key**:
   - Berikan nama key: `BioVerse Local Dev`.
   - Permissions: Berikan akses **Read & Write** untuk *Money-in (Payments)* dan *Money-out (Disbursements)*.
5. Klik **Generate Key** $\rightarrow$ Xendit akan menampilkan Secret Key (awalan `xnd_development_...`).
6. **Salin key tersebut**, lalu buka file `.env` di root proyek Anda, isi pada baris:
   ```env
   XENDIT_SECRET_KEY=xnd_development_xxxxxxxxxxxxxxxxxxxx
   ```
7. *(Opsional)* Di menu **Settings $\rightarrow$ Callbacks (Webhooks)**, Anda dapat melihat kolom *Webhook verification token*. Salin string tersebut ke:
   ```env
   XENDIT_WEBHOOK_TOKEN=token_callback_anda
   ```

---

## 2. Google Cloud Console (OAuth 2.0 Client ID)

Dibutuhkan agar Flutter app dan web dashboard bisa memverifikasi token Google pengguna secara aman di backend.

### Langkah-langkah:
1. Buka [console.cloud.google.com](https://console.cloud.google.com) dan login dengan akun Google Anda.
2. Di pojok kiri atas (sebelah logo Google Cloud), klik pemilih project $\rightarrow$ klik **New Project**.
   - Project Name: `BioVerse`.
   - Klik **Create**.
3. Buka menu **APIs & Services** $\rightarrow$ **OAuth consent screen**:
   - User Type: Pilih **External** $\rightarrow$ klik **Create**.
   - App Name: `BioVerse`.
   - User support email: Email Anda.
   - Developer contact email: Email Anda.
   - Klik **Save and Continue** melewati tahapan Scopes & Test Users hingga selesai.
4. Buka menu **APIs & Services** $\rightarrow$ **Credentials**:
   - Klik tombol **+ CREATE CREDENTIALS** di bagian atas $\rightarrow$ pilih **OAuth client ID**.
   - Application type: Pilih **Web application**.
   - Name: `BioVerse Backend & Web`.
   - Authorized redirect URIs: Tambahkan `http://localhost:8080/v1/app/auth/google/callback` dan `http://localhost:3000`.
   - Klik **Create**.
5. Google akan menampilkan modal berisi **Client ID** (berakhiran `.apps.googleusercontent.com`).
6. **Salin Client ID**, lalu buka file `.env`, isi pada baris:
   ```env
   GOOGLE_CLIENT_ID=xxxxxxxxxxxx-xxxxxxxxxxxxxxxx.apps.googleusercontent.com
   ```

---

## 3. Firebase Cloud Messaging (FCM Push Notification)

Dibutuhkan agar backend bisa mengirim notifikasi ke aplikasi Flutter saat karang selesai ditanam, ada foto update milestone (bulan ke 1, 3, 6), atau streak mau putus.

### Langkah-langkah:
1. Buka [console.firebase.google.com](https://console.firebase.google.com).
2. Klik **Add project** $\rightarrow$ beri nama `BioVerse` $\rightarrow$ matikan Google Analytics (opsional) $\rightarrow$ klik **Create project**.
3. Setelah project selesai dibuat, klik ikon **Settings (Gerigi)** di samping "Project Overview" di sidebar kiri $\rightarrow$ pilih **Project settings**.
4. Masuk ke tab **Service accounts**.
5. Pastikan bahasa terpilih **Firebase Admin SDK** $\rightarrow$ klik tombol **Generate new private key** $\rightarrow$ klik **Generate key**.
6. Browser akan otomatis mengunduh satu file JSON (misal bernama `bioverse-firebase-adminsdk-xxxxx.json`).
7. Simpan file JSON ini di folder aman di komputer Anda (misal ditaruh di dalam folder user Anda atau folder config lokal).
8. Buka file `.env`, isi path lengkap ke file JSON tersebut:
   ```env
   FCM_CREDENTIALS_JSON_PATH=C:\Users\ASUS\Documents\bioverse-firebase-adminsdk.json
   ```

---

## 4. Resend.com (Email Transaksional - Verifikasi Akun)

Resend adalah layanan pengiriman email modern dan gratis hingga 3.000 email per bulan tanpa perlu kartu kredit.

### Langkah-langkah:
1. Buka [resend.com/signup](https://resend.com/signup) dan daftar akun (bisa login via GitHub).
2. Di dashboard Resend, buka menu **API Keys**.
3. Klik **Create API Key**:
   - Name: `BioVerse Dev`.
   - Permission: `Full access`.
4. Salin API Key yang dihasilkan (awalan `re_...`).
5. Buka file `.env`, isi:
   ```env
   RESEND_API_KEY=re_xxxxxxxxxxxxxxxxxxxx
   EMAIL_FROM=onboarding@resend.dev
   ```
*(Catatan: Domain `onboarding@resend.dev` adalah domain uji coba gratis bawaan Resend sehingga Anda tidak wajib punya domain sendiri untuk testing).*

---

## 5. Pertanyaan Non-Teknis ke Tim Figma (Desain)

Ada 2 hal dari desain Figma yang perlu Anda pastikan bersama tim UI/UX:
1. **Layar Mission Details**: Di bawah deskripsi misi ada 3 kotak info kecil. Tanyakan: *"Tiga kotak itu standarnya menampilkan apa? Apakah Target Waktu, Poin XP, dan Tingkat Kesulitan?"*
2. **Kategori Misi**: Tanyakan daftar kategori apa saja yang disepakati (misal: `beach_cleanup`, `waste_sorting`, `tree_care`, `energy_saving`).
