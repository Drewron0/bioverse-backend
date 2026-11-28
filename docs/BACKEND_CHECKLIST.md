# Checklist Lengkap Pekerjaan Backend BioVerse (Post-Week 11)

Dokumen ini adalah daftar lengkap seluruh celah, utang teknis, sinkronisasi kontrak API, dan fitur yang masih perlu dikerjakan atau diverifikasi pada `bioverse-backend`. Disusun berdasarkan audit langsung terhadap kode Go saat ini (hingga commit `e692a21`), `docs/BACKEND_SPEC.md`, `api/openapi.yaml`, progress report Week 10 & Week 11, serta repo frontend `gchelsea01/BioVerse` (`docs/BACKEND_NOTES.md`, `lib/services/*.dart`, dan `test/api_contract_test.dart`).

---

## Ringkasan Status Modul Saat Ini

| Domain / Modul | Kondisi di Kode Saat Ini | Sisa Pekerjaan Utama |
|---|---|---|
| `identity` | Register, login, refresh rotation + reuse detection, forgot/reset password, email verify, `users/me`, `users/me/stats`, cursor `notifications`, FCM token, Google ID token crypto verify, admin MFA TOTP + lockout. | Multi-audience Google OAuth (Android + Web), endpoint hapus akun (`DELETE /v1/app/users/me`) + UU PDP anonymisation. |
| `conservation` | List/detail proyek (lokasi, koordinat, kontributor), `corals/my`, `units/my` (coral/mangrove/tree + milestone `scheduled`), timeline, impact summary, news, admin CRUD zona/proyek/frame/news, `AllocateFragments` (`FOR UPDATE SKIP LOCKED`). | Sinkronisasi OpenAPI `units/my` (`{items}` vs array), endpoint admin `project_incidents` & pemindahan status fragmen (`planted`/`monitoring`/`deceased`), pembaruan foto seed mangrove. |
| `donation` & `payment` | Checkout idempoten, points lock/spend, mixed payment, `mangrove_planting`, Xendit invoice & webhook dedupe (`webhook_events`), `simulate-paid` (terkunci di prod), history + detail ter-enrichment proyek. | Uji E2E Xendit di staging (`ALLOW_SIMULATE_PAID=true`) atau QRIS nominal kecil di prod. |
| `catalog` | Produk digital, `shop/buy` idempoten, inventory, streak freeze, plans listing, `subscriptions` pause/resume/cancel. | Recurring billing tokenization Xendit (`POST /v1/app/subscriptions` saat ini `503` di prod) & server-side IAP verify Apple/Google (`503` di prod). |
| `gamification` | Pets multi-unit, `pets/species` (9 spesies laut/mangrove/hutan), interact + streak, equip inventory guard, missions (`completed_today` tanggal lokal + `COALESCE`), submit + presigned URL, admin review (`GET`/`POST /admin/gamification/submissions`), leaderboard. | Verifikasi deploy `e692a21` di API live, set `CV_FALLBACK_AUTO_APPROVE=false` sebelum rilis publik (atau aktifkan CV service), update foto seed spesies. |
| `fieldops` | Operator KYC terenkripsi (AES-256-GCM + HMAC NIK), work logs, media evidence + parser GPS EXIF (`exif.go`), subsea telemetry (`POST`/`GET /admin/fieldops/telemetry`), payout draft -> approve -> disburse idempoten. | Tambahkan route telemetry ke `api/openapi.yaml`, lengkapi alur `survey_records` & approval oleh `site_manager` jika operator login dibuka. |
| `b2b` | Company/member/contract/quota CRUD, dashboard + SROI (`CalculateSROI`), visual proof, metrics CSV export, report PDF/CSV/ZIP generator (`export.go`), brand assets + PR kit ZIP, service requests + completion, settings, members. | Tambahkan 12 endpoint B2B Portal/Admin yang belum masuk `api/openapi.yaml`, audit RBAC antar-role B2B, dan opsional arsip file permanen ke S3. |
| `worker` | Ticker 1 menit `ProcessExpiredDonations` (unlock points) & ticker 1 jam `ProcessDueSubscriptions` (simulasi cycle). | Aktifkan real subscription charging + retry/pause, tambah job `decay_pet_health`, `cleanup_expired_tokens`, `refresh_exchange_rates`, dan `anonymise_deleted_users`. |

---

## 1. Prioritas Kritis / P0 (Wajib Sebelum Rilis Publik B2C)

- [ ] **1.1. Verifikasi Deploy Commit `e692a21` di API Live (`https://api.ourprojects.my.id/v1`)**
  - *Status:* Sudah di-push di backend, perlu dipastikan aktif di server produksi dan dikonfirmasi oleh tim frontend agar `docs/BACKEND_NOTES.md` ikut diperbarui.
  - *Cek:*
    - `GET /v1/health` mengembalikan header `Cache-Control: no-store, no-cache, must-revalidate` dan `uptime`/`timestamp` selalu fresh.
    - `GET /v1/app/missions` mengembalikan `completed_today: true` jika misi sudah disubmit pada rentang `00:00-07:00 WIB` (`Asia/Jakarta`).
    - `GET /v1/admin/gamification/submissions` dan `POST /v1/admin/gamification/submissions/{id}/review` merespons normal.

- [ ] **1.2. Nonaktifkan Auto-Approve Fallback CV (`CV_FALLBACK_AUTO_APPROVE=false`)**
  - *Lokasi:* `cmd/api/main.go` (default `true` bila env tidak diset), `internal/gamification/service.go`.
  - *Masalah:* Selama service Python CV belum aktif di `CV_SERVICE_URL`, foto polos apa pun tetap lolos `verified` dan langsung memberi poin/XP.
  - *Tindakan:*
    1. Untuk production publik, set `CV_FALLBACK_AUTO_APPROVE=false` di `.env` server sehingga submission otomatis masuk `needs_review` dan tidak bisa dipakai farming poin.
    2. Pastikan staf/admin memakai endpoint `GET /v1/admin/gamification/submissions?status=needs_review` dan `POST /v1/admin/gamification/submissions/{id}/review`, atau jalankan service Python CV yang sebenarnya.

- [ ] **1.3. Uji End-to-End Checkout & Webhook Xendit**
  - *Lokasi:* `internal/platform/config/config.go` (`AllowSimulatePaid` dipaksa `false` di `production`), `internal/donation/service.go`, `internal/donation/handler.go`.
  - *Masalah:* Alur uang riil (`checkout` -> invoice/QRIS/VA Xendit -> webhook `paid` -> alokasi fragmen/unit -> `units/my` -> `donations/history`) belum terverifikasi di server live.
  - *Tindakan:*
    1. Siapkan environment staging dengan `APP_ENV=staging` dan `ALLOW_SIMULATE_PAID=true`, ATAU lakukan uji pembayaran QRIS nominal kecil di production.
    2. Verifikasi seluruh field proyek di `GET /v1/app/donations/{id}` & `/history` (`project_id`, `project_title`, `project_category`, `image_url`, `location_name`, `quantity`).
    3. Pastikan unit teradopsi langsung muncul di `GET /v1/app/conservation/units/my` untuk `coral_adoption`, `mangrove_planting`, dan `tree_planting`.

- [ ] **1.4. Aktivasi & Multi-Audience Google Sign-In Android (`POST /v1/app/auth/google`)**
  - *Lokasi:* `internal/identity/service.go` (`validateGoogleIDToken`), `.env` server (`GOOGLE_CLIENT_ID`).
  - *Masalah:*
    1. Tim frontend masih menunggu OAuth Client ID Android (SHA-1) sebelum memasang tombol Google Sign-In.
    2. `validateGoogleIDToken` saat ini hanya memvalidasi `payload.Audience` terhadap satu string `s.config.OAuth.GoogleClientID`. Di Flutter/Android, `idToken` bisa membawa `aud` berupa Web Client ID (`serverClientId`) atau Android Client ID.
  - *Tindakan:*
    1. Daftarkan SHA-1 debug & release di Google Cloud Console, buat Android Client ID + Web Client ID.
    2. Ubah `validateGoogleIDToken` agar mendukung daftar audience (comma-separated di `GOOGLE_CLIENT_ID`) sehingga token dengan audience Web maupun Android tetap tervalidasi aman.

---

## 2. Prioritas Tinggi / P1 (Sinkronisasi Kontrak OpenAPI & Fitur B2C Lanjutan)

- [ ] **2.1. Sinkronkan `api/openapi.yaml` dengan Seluruh Route Aktual di Kode**
  - *Celah yang ditemukan saat audit:*
    - `GET /v1/app/conservation/units/my`: Handler (`internal/conservation/handler.go`) mengembalikan `{"items": [...]}`, sedangkan `api/openapi.yaml` masih menulis `type: array`. Ubah spec ke wrapper object `{"items": [...]}` agar OpenAPI 100% sesuai wire response.
    - 13 route yang **sudah ada di kode** (`internal/b2b/handler.go` dan `internal/fieldops/handler.go`) tetapi **belum terdokumentasi di `api/openapi.yaml`**:
      1. `GET /v1/b2b/visual-proof`
      2. `GET /v1/b2b/media-archive.zip`
      3. `GET /v1/b2b/metrics`
      4. `GET /v1/b2b/metrics/export`
      5. `POST /v1/b2b/reports/generate`
      6. `GET /v1/b2b/reports/{id}/download`
      7. `GET /v1/b2b/brand-assets`
      8. `GET /v1/b2b/pr-kit`
      9. `POST /v1/b2b/service-requests`
      10. `GET /v1/b2b/settings` & `PATCH /v1/b2b/settings`
      11. `GET /v1/b2b/members` & `POST /v1/b2b/members`
      12. `POST /v1/admin/b2b/service-requests/{id}/complete`
      13. `POST /v1/admin/fieldops/telemetry` & `GET /v1/admin/fieldops/telemetry`
    - Jalankan `npx @redocly/cli lint api/openapi.yaml` setelah update untuk memastikan 0 error.

- [ ] **2.2. Recurring Subscriptions Gateway Tokenization**
  - *Lokasi:* `internal/catalog/handler.go` (mengembalikan `503 NOT_AVAILABLE` di `production`), `internal/worker/jobs.go` (`ProcessDueSubscriptions`).
  - *Tindakan:*
    1. Integrasikan flow save payment token & charge saved token Xendit pada `CreateSubscription` dan `ProcessDueSubscriptions`.
    2. Implementasikan retry failed charge dengan backoff, auto-pause setelah N kali gagal (`pause_reason`, `resume_payment_url`), dan reminder notifikasi/email.

- [ ] **2.3. Server-Side In-App Purchase (IAP) Receipt Verification**
  - *Lokasi:* `internal/catalog/handler.go` (`handleVerifyIAPReceipt` mengembalikan `503 NOT_AVAILABLE` di `production` tanpa kredensial store), `internal/catalog/service.go`.
  - *Tindakan:* Pasang verifikasi nyata ke Apple App Store Server API & Google Play Developer API serta penanganan webhook refund/revocation agar `iap_transactions` dan `user_digital_inventories` sinkron.

- [ ] **2.4. Pembaruan Aset Foto Seed Mangrove & Spesies**
  - *Lokasi:* Migrasi seed baru (`00017_*`) & `internal/gamification/handler.go` (`getSpecies`).
  - *Tindakan:* Ganti URL Unsplash sementara pada proyek mangrove dan 9 spesies pet setelah tim desain menyediakan URL aset resmi.

---

## 3. Prioritas Menengah / P2 (Kelengkapan Spec & Operasional B2B/Admin)

- [ ] **3.1. Manajemen Siklus Hidup Fragmen & Pelaporan Insiden Lapangan (`project_incidents`)**
  - *Referensi:* `docs/BACKEND_SPEC.md` Bagian 8.3 & 8.10 (`db/migrations/00006_conservation.sql` sudah memiliki tabel `project_incidents` dan status fragmen `planted`/`monitoring`/`deceased`/`relocated`).
  - *Tindakan:*
    1. Tambahkan endpoint admin untuk menandai fragmen sudah ditanam (assign ke `spider_frames` dengan counter `fragments_planted` dan `planted_units_count`) serta mengupdate `health_status`.
    2. Tambahkan endpoint admin untuk mencatat `project_incidents` (bleaching, disease, death), mengonsumsi replant reserve bila perlu, dan mengirim notifikasi ke pemilik unit terdampak.

- [ ] **3.2. Kepatuhan UU PDP: Hapus Akun & Anonymisation Job**
  - *Referensi:* `docs/BACKEND_SPEC.md` Bagian 10 & 12 (tabel `users` sudah memiliki `deleted_at`).
  - *Tindakan:*
    1. Tambahkan endpoint `DELETE /v1/app/users/me` (soft-delete + cabut seluruh refresh token/FCM token).
    2. Tambahkan job worker `anonymise_deleted_users` untuk meng-anonimkan PII (nama, email, no HP, avatar) setelah masa tenggang berakhir.

- [ ] **3.3. Worker Jobs Terjadwal Tambahan (`cmd/worker/main.go`)**
  - *Lokasi:* `cmd/worker/main.go` & `internal/worker/jobs.go` (saat ini hanya menjalankan `ProcessExpiredDonations` dan `ProcessDueSubscriptions`).
  - *Tindakan:* Tambahkan job idempoten untuk:
    1. `decay_pet_health` (penurunan health pet harian jika tidak dirawat).
    2. `cleanup_expired_tokens` (pembersihan `user_tokens` dan `admin_tokens` yang sudah expired/revoked lama).
    3. `refresh_exchange_rates` (pembaruan kurs USD -> IDR ke tabel `exchange_rates`).
    4. Async mission verification / notification outbox jika nanti memakai River queue penuh.

- [ ] **3.4. Audit RBAC, B2B Portal Edge Cases, & Penyimpanan Arsip ESG ke S3**
  - *Lokasi:* `internal/b2b/handler.go`, `internal/b2b/service.go`.
  - *Tindakan:*
    1. Verifikasi lengkap setiap endpoint portal `/v1/b2b/*` (dashboard SROI, visual proof, download media archive ZIP, metrics CSV, generate & download report PDF/CSV/ZIP, PR kit, service requests, settings, member invites) terhadap 5 role B2B (`corporate_admin`, `esg_manager`, `finance`, `external_auditor`, `viewer`).
    2. Pastikan `external_auditor` dan `viewer` tidak pernah bisa memicu mutasi (`POST`/`PATCH`) apa pun.
    3. Opsional: upload hasil generate report PDF/CSV ke bucket S3 agar kolom `chapter_pdf_url` dan `raw_dataset_csv_url` juga memiliki tautan presigned permanen.

- [ ] **3.5. Endpoint Admin Audit Logs & Survey Records**
  - *Referensi:* `docs/BACKEND_SPEC.md` Bagian 2 & 8.9 (tabel `audit_logs` dan `survey_records` sudah ada).
  - *Tindakan:* Tambahkan endpoint baca `GET /v1/admin/audit-logs` untuk kebutuhan back-office compliance serta endpoint input/baca `survey_records` bila dibutuhkan tim lapangan dan B2B.

---

## 4. Prioritas DevOps & Infrastruktur Pengujian / P3

- [ ] **4.1. Isolasi Database untuk Pengujian Paralel**
  - *Masalah:* `go test ./...` secara paralel terhadap Supabase session pooler (limit 15 client) gagal dengan error `EMAXCONNSESSION`.
  - *Tindakan:* Gunakan `go test -p 1 ./...` untuk pengujian ke DB pooler, atau pisahkan environment `.env.test` menuju PostgreSQL lokal (`docker-compose.yml`).

- [ ] **4.2. Endpoint Admin Verification Smoke Test & Observability**
  - *Tindakan:* Tambahkan health alert / monitoring error rate 5xx dan pastikan log worker (`bioverse-worker`) dipantau di server produksi.
