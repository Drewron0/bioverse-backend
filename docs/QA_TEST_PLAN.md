# Dokumen QA Test Plan Lengkap - BioVerse Backend

Dokumen ini menjadi panduan Quality Assurance (QA) resmi untuk memverifikasi seluruh layanan BioVerse Backend (B2C Mobile API, Xendit Payments & Webhook, Admin Back-Office, FieldOps, B2B ESG Portal, dan Background Worker) baik secara otomatis maupun pengujian integrasi live/staging.

---

## 1. Lingkungan & Prasyarat Pengujian

### 1.1. Target Environment
- **Local:** `http://localhost:8080/v1` (PostgreSQL 16 + MinIO via `deploy/docker-compose.yml`, `APP_ENV=development`)
- **Staging (Disarankan untuk Uji Pembayaran):** `APP_ENV=staging`, `ALLOW_SIMULATE_PAID=true`
- **Production Live:** `https://api.ourprojects.my.id/v1` (`APP_ENV=production`, `ALLOW_SIMULATE_PAID=false`)

### 1.2. Akun & Data Prasyarat Uji
1. **Akun B2C User A:** Akun test utama (timezone `Asia/Jakarta`, email terverifikasi, memiliki minimal 1 pet).
2. **Akun B2C User B:** Akun test pembanding untuk menguji seluruh skenario proteksi IDOR (tidak boleh membaca/mengubah data User A).
3. **Akun Admin Back-Office:**
   - 1 akun `super_admin` (dengan TOTP MFA aktif).
   - 1 akun `support` / `content` untuk menguji pembatasan RBAC terhadap endpoint `finance` (`payouts`).
4. **Akun B2B Portal:**
   - 1 akun member Perusahaan A berizin `admin` / `edit`.
   - 1 akun member Perusahaan A berizin `view_only` / role `external_auditor`.
   - 1 akun user non-member (bukan bagian dari perusahaan mana pun) untuk menguji penolakan `403 Forbidden`.

### 1.3. Pengujian Otomatis Sebelum Deploy (Regression Gate)
Jalankan berurutan dari root `bioverse-backend`:
```powershell
npx @redocly/cli lint api/openapi.yaml
go vet ./...
go build ./...
go test -p 1 -v ./...
```
Pada repo frontend (`gchelsea01/BioVerse/bioverse_app`):
```powershell
flutter analyze
flutter test
```
Catatan: Gunakan `-p 1` pada `go test` jika terhubung ke Supabase session pooler (batas 15 koneksi).

---

## 2. Skenario QA Test Plan Per Modul

### Modul A: Health, Security Headers, Rate Limiting, & Upload S3

| ID | Target / Endpoint | Prasyarat | Langkah Pengujian | Ekspektasi Hasil | Status |
|---|---|---|---|---|---|
| QA-INF-01 | `GET /healthz`, `GET /livez`, `GET /v1/health` | Server & DB menyala | Panggil `GET /v1/health` 2 kali berjarak >= 5 detik; cek response body dan header. | `200 OK`, `services.database == "connected"`, `uptime` dan `timestamp` selalu bertambah, header `Cache-Control: no-store, no-cache, must-revalidate` ada. | |
| QA-INF-02 | Security Headers & Body Limit | Tidak perlu auth | Periksa response headers; coba POST payload JSON > 1 MB ke endpoint POST. | Header keamanan OWASP (`X-Content-Type-Options`, dll.) ada; body > 1 MB ditolak (`413`). | |
| QA-INF-03 | Rate Limiter Auth (Anti-Spoof IP) | Tidak perlu auth | Kirim > 30 request `POST /v1/app/auth/login` dalam 1 menit dari 1 IP sambil mengubah-ubah header `X-Forwarded-For` dan `X-Real-IP`. | Request ke-31 diblokir `429 Too Many Requests` karena limiter membaca IP koneksi langsung, bukan header spoofed. | |
| QA-INF-04 | `POST /v1/app/uploads/presign` | Login User A | 1. Minta presign `purpose: "mission_proof"` (`image/png`, <= 10 MB) lalu `PUT` file ke `upload_url`.<br>2. Minta `purpose: "field_evidence"` memakai token User biasa.<br>3. Kirim ukuran > 10 MB atau content-type tidak diizinkan. | Langkah 1 berhasil `200` + `PUT` ke storage `200/204`. Langkah 2 ditolak `403`. Langkah 3 ditolak `400`. | |

---

### Modul B: Identitas, Autentikasi, Profil, Statistik, & Notifikasi

| ID | Target / Endpoint | Prasyarat | Langkah Pengujian | Ekspektasi Hasil | Status |
|---|---|---|---|---|---|
| QA-AUT-01 | Register & Verifikasi Email | Email baru belum terdaftar | 1. `POST /v1/app/auth/register` (`timezone: "Asia/Jakarta"`, opsional `referral_code`).<br>2. Verifikasi email memakai token (`POST /v1/app/auth/verify-email`). | `201 Created`, user mendapat `access_token`, `refresh_token`, `referral_code` unik, baris `user_streaks` terbentuk; verifikasi email mengubah `is_email_verified = true`. | |
| QA-AUT-02 | Refresh Token Rotation & Reuse Detection | Token login User A | 1. Panggil `POST /v1/app/auth/refresh` dengan `refresh_token` R1 -> dapat R2.<br>2. Coba panggil refresh lagi memakai R1 (token lama yang sudah terpakai). | Langkah 1 sukses mengembalikan R2. Langkah 2 ditolak `401` dan seluruh sesi/token milik User A dicabut untuk mencegah token replay. | |
| QA-AUT-03 | Forgot & Reset Password | User A terdaftar | 1. `POST /v1/app/auth/forgot-password`.<br>2. `POST /v1/app/auth/reset-password` dengan token valid + password baru.<br>3. Login pakai password lama vs baru. | Password lama ditolak `401`, password baru berhasil login `200`. | |
| QA-AUT-04 | Google Sign-In (`POST /v1/app/auth/google`) | `GOOGLE_CLIENT_ID` terisi | 1. Kirim `id_token` palsu / JWT expired / audience salah.<br>2. Kirim ID token Google asli dari Android/Web OAuth client. | Langkah 1 ditolak `401`. Langkah 2 berhasil login atau auto-register dan mengembalikan JWT BioVerse. | |
| QA-USR-01 | `GET` & `PATCH /v1/app/users/me` | Login User A | 1. Ambil profil.<br>2. Upload avatar -> update `full_name`, `preferred_currency_code` (`IDR`), `avatar_object_key`. | `200 OK`; `current_streak` tampil di respons `UserProfile`; perubahan tersimpan persisten. | |
| QA-USR-02 | `GET /v1/app/users/me/stats` | User A punya donasi `paid` & misi `verified` | Panggil `GET /v1/app/users/me/stats`. | `200 OK`; `missions_completed` hanya menghitung status `verified`, sedangkan `total_donations_usd`, `total_donations_idr`, `total_units_adopted`, `current_points`, dan streak akurat. | |
| QA-USR-03 | `GET /v1/app/notifications` & Read State | User A punya >= 5 notifikasi | 1. Panggil `?limit=2`.<br>2. Panggil `?limit=2&cursor=<next_cursor>`.<br>3. `PATCH /v1/app/notifications/{id}/read` & `POST /read-all`.<br>4. User B coba tandai baca notifikasi milik User A. | Langkah 1-2 mengembalikan `{items, next_cursor}` tanpa duplikat. Langkah 3 sukses. Langkah 4 ditolak `404` (IDOR check). | |

---

### Modul C: Konservasi, Proyek, Fragmen Karang, & Units

| ID | Target / Endpoint | Prasyarat | Langkah Pengujian | Ekspektasi Hasil | Status |
|---|---|---|---|---|---|
| QA-CON-01 | `GET /v1/app/conservation/projects` & `/{id}` | Data seed migrasi `00013` - `00016` aktif | Panggil tanpa filter serta dengan `category=coral`, `mangrove`, `tree`. | Tersedia proyek coral, mangrove, dan tree. Tanjung Lesung Mangrove Shield memakai `location_name` Pandeglang, Banten & koordinat `-6.4833, 105.6667` (bukan Bali), lengkap dengan `contributors_count`. | |
| QA-CON-02 | `GET /v1/app/impact/summary` & `/app/news` | Data seed aktif | Panggil kedua endpoint publik tanpa token. | `200 OK`; impact summary agregat valid; berita mengembalikan minimal 3 artikel lintas kategori. | |
| QA-CON-03 | `GET /v1/app/conservation/units/my` | User A punya unit adopsi (coral/mangrove/tree) tanpa foto lapangan operator | Panggil endpoint dengan token User A. | Mengembalikan daftar unit lintas ekosistem; placeholder milestone bulan 1/3/6 memiliki `health_status: "scheduled"` dan `is_verified_healthy: false` (tidak memalsukan laporan diterima). | |
| QA-CON-04 | `GET /v1/app/conservation/units/my` (Sesudah Foto Lapangan) | Admin menginput `POST /v1/admin/fieldops/media` bulan ke-1 untuk fragmen User A | Panggil ulang `units/my` dan `GET /v1/app/conservation/corals/{id}/timeline`. | Milestone bulan ke-1 menampilkan foto lapangan asli operator, `is_verified_healthy: true`, `health_status: "healthy"`, dan timestamp aktual. | |
| QA-CON-05 | Proteksi IDOR Coral | Fragmen milik User A | User B memanggil `GET /v1/app/conservation/corals/{id}` dan `/{id}/timeline` memakai ID fragmen User A. | Keduanya mengembalikan `404 Not Found`. | |

---

### Modul D: Donasi, Checkout, Points, Xendit Webhook, & Worker Expired

| ID | Target / Endpoint | Prasyarat | Langkah Pengujian | Ekspektasi Hasil | Status |
|---|---|---|---|---|---|
| QA-DON-01 | `POST /v1/app/donations/checkout` (`points_only`) | User A punya poin cukup; proyek `coral`, `mangrove`, `tree` terbuka | Checkout masing-masing tipe (`coral_adoption`, `mangrove_planting`, `tree_planting`) dengan `payment_method_type: "points_only"`. | `201 Created`, `status: "paid"`, poin terpotong di `point_ledgers`, fragmen coral teralokasi ke `spider_frames` (atau `funded_units_count` proyek naik untuk mangrove/tree), dan unit muncul di `units/my`. | |
| QA-DON-02 | Mixed Payment + Points Locking | User A punya poin > 0 | Checkout `qris` atau `virtual_account` dengan `points_to_burn > 0`. Cek `locked_points_balance` user; coba checkout lagi melebihi sisa poin bebas. | Checkout pertama mengunci poin (`locked_points_balance` naik). Checkout kedua yang melebihi poin bebas ditolak. | |
| QA-DON-03 | Checkout Idempotency Key | Token User A | 1. Checkout dengan `Idempotency-Key: X1`.<br>2. Ulangi persis sama dengan `X1`.<br>3. Kirim `X1` dengan `quantity` atau `donation_type` berbeda. | Langkah 2 mengembalikan transaksi awal tanpa membuat invoice Xendit ganda. Langkah 3 ditolak (`ErrIdempotencyKeyReused`). | |
| QA-DON-04 | Webhook Xendit / `simulate-paid` (Staging) | Environment staging (`ALLOW_SIMULATE_PAID=true`) atau QRIS kecil di prod | 1. Checkout QRIS/VA (`status: pending`).<br>2. Trigger `POST /v1/webhooks/xendit` valid (atau `/simulate-paid` di staging).<br>3. Kirim ulang webhook yang sama (duplicate delivery).<br>4. Cek `GET /v1/app/donations/{id}` & `/history`. | Langkah 2 melunasi donasi, membuka lock poin -> `spend_donation`, mengalokasi unit, dan mengirim notifikasi. Langkah 3 ter-dedupe (`webhook_events` tidak double-allocate). Langkah 4 menampilkan `project_id`, `project_title`, `project_category`, `image_url`, `location_name`, dan `quantity`. | |
| QA-DON-05 | Keamanan Webhook | Environment apa saja | 1. Kirim webhook dengan `x-callback-token` salah.<br>2. Kirim webhook dengan nominal `amount` lebih kecil dari `settlement_amount` donasi.<br>3. Cek `simulate-paid` di environment `production`. | Token salah ditolak; nominal kurang ditolak/dicatat gagal; `simulate-paid` mengembalikan `404` (tidak ter-mount) di production. | |
| QA-DON-06 | Coral-First Gating & Bio-Shield Boost | User baru (0 coral) vs User dengan coral aktif | 1. User baru coba beli `bio_shield_addon`.<br>2. User yang sudah punya coral membeli `bio_shield_addon` dengan `target_pet_id`. | Langkah 1 ditolak; langkah 2 melampirkan boost `x1.25` XP pada pet yang dipilih. | |
| QA-DON-07 | Worker Cleanup Expired Donations | Ada donasi `pending` mixed payment dengan `expired_at < NOW()` | Jalankan `bioverse-worker` (`ProcessExpiredDonations`). | Status donasi berubah ke `expired`, `locked_points_balance` dikembalikan, dan entri `unlock_donation` tercatat di `point_ledgers`. | |

---

### Modul E: Gamifikasi (Virtual Pet, Eco-Missions, Timezone, CV Fallback, & Shop)

| ID | Target / Endpoint | Prasyarat | Langkah Pengujian | Ekspektasi Hasil | Status |
|---|---|---|---|---|---|
| QA-GAM-01 | `GET /v1/app/pets/species` & Adopsi Pet | Login User A | 1. Panggil `GET /v1/app/pets/species`.<br>2. Adopsi 1 pet laut dan 1 pet hutan via `POST /v1/app/pets`. | `species` mengembalikan 9 spesies (laut, mangrove, hutan). Adopsi sukses membuat pet; pet pertama otomatis `is_primary: true`. | |
| QA-GAM-02 | `POST /v1/app/pets/{id}/interact` & Streak | Pet milik User A, streak hari ini belum aktif | Panggil `/interact` dengan `interaction_type` `"feed"`, `"clean"`, dan `"pet"`; lalu cek `GET /v1/app/streaks` & `/users/me`. | Respons mengembalikan `VirtualPet` lengkap (termasuk `xp_to_next_level`), XP naik, level/growth_stage ikut naik sesuai kurva `100*(L-1)^2`, dan `current_streak` bertambah 1. | |
| QA-GAM-03 | Submit Misi (`CV_FALLBACK_AUTO_APPROVE=true`) | CV service mati, flag `true` | Upload gambar PNG/JPG via presign -> `POST /v1/app/missions/{id}/submit`. | `202 Accepted`, `status: "verified"`, `message: "Proof photo verified successfully."`, poin user & XP pet langsung naik, muncul di `GET /v1/app/mission-submissions/my`. | |
| QA-GAM-04 | **Batas Timezone `completed_today` (00:00-07:00 WIB)** | User `Asia/Jakarta` | 1. Submit misi pada pukul 00:00-07:00 WIB (saat UTC masih tanggal kemarin).<br>2. Panggil `GET /v1/app/missions`.<br>3. Coba submit misi yang sama lagi di hari WIB tersebut. | Pada langkah 2, `completed_today == true` dan `my_last_submission_status != null`. Langkah 3 ditolak `409 Conflict`. | |
| QA-GAM-05 | **Strict CV Mode & Admin Review (`CV_FALLBACK_AUTO_APPROVE=false`)** | CV service mati, flag `false` | 1. User submit misi.<br>2. Cek poin user (harus belum bertambah).<br>3. Admin panggil `GET /v1/admin/gamification/submissions?status=needs_review`.<br>4. Admin panggil `POST /v1/admin/gamification/submissions/{id}/review` (`decision: "approve"`).<br>5. Panggil ulang `/review` dengan ID sama. | Langkah 1 mengembalikan `status: "needs_review"` dan pesan antrean review. Langkah 3 menampilkan URL foto presigned yang bisa dibuka. Langkah 4 mengubah status -> `verified` & mengkredit poin/XP. Langkah 5 ditolak `409 Conflict`. | |
| QA-GAM-06 | Admin Review Reject & Retry User | Submission `needs_review` | Admin review dengan `decision: "reject"`, lalu user submit ulang misi yang sama di hari yang sama dengan bukti baru. | Status jadi `rejected` (0 poin) dengan `rejection_reason`; user diizinkan mencoba submit ulang karena partial unique index mengabaikan `rejected`. | |
| QA-GAM-07 | Shop, Equip, & Leaderboard | User A punya poin; User B punya donasi anonim | 1. Beli item poin via `/shop/buy`, lalu equip ke pet.<br>2. Coba equip item yang tidak dimiliki.<br>3. Cek `GET /v1/app/leaderboard` untuk donasi `is_anonymous: true`. | Langkah 1 sukses; langkah 2 ditolak error inventori; langkah 3 menyembunyikan nama/avatar/ID user anonim menjadi `"Anonymous Supporter"`. | |

---

### Modul F: Catalog Subscriptions & In-App Purchase

| ID | Target / Endpoint | Prasyarat | Langkah Pengujian | Ekspektasi Hasil | Status |
|---|---|---|---|---|---|
| QA-CAT-01 | `GET /v1/app/catalog/products` & `/plans` | Data seed aktif | Panggil dengan `?currency=USD` dan `?currency=IDR`. | `200 OK`; daftar harga multi-currency dan array `benefits` tampil sesuai mata uang. | |
| QA-CAT-02 | `POST /v1/app/subscriptions` & Lifecycle | Staging vs Production | 1. Di `production`, coba buat subscription.<br>2. Di `staging`, buat subscription lalu `pause` -> `resume` -> `cancel`. | Di `production` sementara mengembalikan `503 NOT_AVAILABLE` hingga tokenization Xendit aktif. Di `staging` transisi status berjalan normal dan worker menggeser `next_billing_date`. | |
| QA-CAT-03 | `POST /v1/app/shop/iap/verify` | Production tanpa kredensial IAP vs Staging/Mock | Kirim payload receipt ke endpoint. | Di `production` tanpa kredensial mengembalikan `503 NOT_AVAILABLE` (tidak pernah lolos tanpa verifikasi store). | |

---

### Modul G: Admin Back-Office RBAC, FieldOps, Telemetry, & EXIF

| ID | Target / Endpoint | Prasyarat | Langkah Pengujian | Ekspektasi Hasil | Status |
|---|---|---|---|---|---|
| QA-ADM-01 | Admin Login MFA & Lockout | Akun admin aktif | 1. Login admin dengan password benar tanpa TOTP (jika MFA aktif).<br>2. Salah memasukkan password berkali-kali.<br>3. Setup & verify TOTP MFA ketika MFA sudah aktif. | Meminta `MFA_REQUIRED`; terkunci sementara (`ACCOUNT_LOCKED`) setelah gagal berulang; setup ulang saat MFA sudah aktif ditolak. | |
| QA-ADM-02 | Admin Role Enforcement | Token admin `support`/`content` vs `finance` vs `super_admin` | Coba akses `POST /v1/admin/fieldops/payouts` memakai role `support` atau `content`. | Ditolak `403 Forbidden`; hanya `finance` & `super_admin` yang diizinkan. | |
| QA-OPS-01 | Operator KYC & Response Sanitization | Token `operations` / `super_admin` | Daftarkan operator dengan NIK & nomor rekening (`POST /v1/admin/fieldops/operators`), lalu `GET .../operators/{id}`. | Response `toOperatorResponse` TIDAK membocorkan `nik_encrypted`, `nik_hmac`, `bank_account_encrypted`, atau `password_hash`. | |
| QA-OPS-02 | Media Evidence + Ekstraksi GPS EXIF & Telemetry | Foto JPEG ber-EXIF GPS (`image_base64`) & `zone_id` valid | 1. `POST /v1/admin/fieldops/media` dengan `image_base64` dan koordinat awal `0, 0`.<br>2. `POST` & `GET /v1/admin/fieldops/telemetry`. | Koordinat GPS & `exif_metadata` otomatis terekstrak dari JPEG dan tersimpan pada `media_evidences`; telemetry tersimpan dan bisa di-filter per `zone_id`. | |
| QA-OPS-03 | Operator Work Log & Payout State Machine | Operator punya work log `verified` belum dibayar | 1. `POST /v1/admin/fieldops/payouts` (`Idempotency-Key` wajib).<br>2. Coba langsung `disburse` sebelum `approve`.<br>3. `approve` -> `disburse` -> coba `disburse` kedua kali. | `draft` -> `disburse` ditolak; `approve` -> `disburse` sukses; `disburse` ulang ditolak oleh optimistic lock `expected_status`; `fee_earned_idr` dihitung dari tarif server. | |

---

### Modul H: B2B ESG Portal & Tenant Isolation

| ID | Target / Endpoint | Prasyarat | Langkah Pengujian | Ekspektasi Hasil | Status |
|---|---|---|---|---|---|
| QA-B2B-01 | Tenant Isolation (Member vs Non-Member) | User non-member & Member Perusahaan A | User non-member panggil `GET /v1/b2b/dashboard` dan `/reports`; Member A panggil seluruh `/v1/b2b/*`. | Non-member ditolak `403 Forbidden`. Member A hanya melihat proyek, frame, metrik, dan laporan milik Perusahaan A. | |
| QA-B2B-02 | B2B Role Permissions (Write vs Read-Only) | Member `external_auditor` / `view_only` vs `corporate_admin` | Auditor mencoba `POST /v1/b2b/reports/generate`, `POST /v1/b2b/service-requests`, `PATCH /v1/b2b/settings`, atau `POST /v1/b2b/members`. | Auditor dan `view_only` ditolak `403 Forbidden` pada seluruh aksi mutasi; `corporate_admin` diizinkan. | |
| QA-B2B-03 | B2B Visual Proof & Export Downloads | Perusahaan A punya frame, telemetry, metrik, & laporan ESG | Panggil `/v1/b2b/visual-proof`, `/media-archive.zip`, `/metrics/export`, `/reports/{id}/download?format=pdf|csv|zip`, `/brand-assets`, `/pr-kit`. | Semua endpoint mengembalikan `200 OK` dengan `Content-Type` dan `Content-Disposition` valid (JSON, CSV, PDF, ZIP terbaca dan tidak corrupt) serta nilai SROI & POJK 51 readiness terhitung. | |
| QA-B2B-04 | Sponsored Quota Claim in B2C Checkout | Perusahaan A punya kontrak & kuota aktif | User mengklaim adopsi memakai `sponsored_quota_id` via `POST /v1/app/donations/checkout` hingga kuota habis. | Kuota berkurang atomik (`used_quota` bertambah); request setelah kuota habis ditolak dengan error kuota habis. | |

---

## 3. Definisi Lolos QA (Release Sign-Off Criteria)

- [ ] Seluruh regression suite backend (`go vet`, `go build`, `go test -p 1 ./...`, `redocly lint`) dan frontend (`flutter analyze`, `flutter test`) hijau 100%.
- [ ] Seluruh skenario P0 di Modul A, B, C, D, dan E (termasuk batas timezone 00:00-07:00 WIB, `needs_review` + admin review, serta alur donasi checkout -> webhook -> `units/my`) lulus tanpa error 500.
- [ ] `CV_FALLBACK_AUTO_APPROVE=false` aktif di production (atau service CV menyala) dan `ALLOW_SIMULATE_PAID=false` terverifikasi aktif di production.
- [ ] Seluruh route aktif pada kode sudah sinkron di `api/openapi.yaml`.
