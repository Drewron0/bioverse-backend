Step by step:
# Roadmap Lengkap Pengembangan Backend hingga Siap Pakai Frontend

Dokumen ini menyajikan gambaran utuh pengerjaan dari titik nol hingga sistem backend siap diintegrasikan dan dipakai oleh tim frontend. Seluruh estimasi waktu di dalam rencana ini bersifat kasar dan menggunakan asumsi pengerjaan oleh 1 hingga 2 orang developer backend.

## Prinsip Kerja

Proses pengembangan berpegang pada prinsip **kontrak API terlebih dahulu, baru kemudian penulisan kode**. Setiap fase selalu dimulai dengan menyusun spesifikasi bagian `openapi.yaml` yang relevan agar tim frontend dapat langsung membuat *client* serta menjalankan *mock server* tanpa harus menunggu layanan backend selesai dibangun. Pengerjaan dilakukan dengan menyelesaikan satu alur (*slice*) secara *end-to-end* terlebih dahulu sebelum melebar ke modul-modul lainnya. Suatu pekerjaan baru dinyatakan selesai apabila seluruh pengujian berstatus hijau, spesifikasi OpenAPI telah diperbarui, sistem sudah ter-deploy ke *environment* staging, dan koleksi Postman telah diperbarui sesuai kondisi terkini.

## Fase Kerja

**Fase 0: Persiapan (Minggu 0, berjalan paralel dengan Fase 1)**
Fase ini berfokus penuh pada pekerjaan non-kode (rincian lengkap dijabarkan pada bagian tersendiri di bawah). Pada tahap ini, belum ada ketergantungan pekerjaan yang bisa langsung dieksekusi oleh tim frontend.

**Fase 1: Fondasi (Minggu 1)**
Pada minggu pertama, pekerjaan backend mencakup penyiapan repositori beserta tata letak modul, konfigurasi `docker-compose` (untuk menjalankan Postgres 16 dan MinIO), penulisan *database migration* dari `00002` hingga tahap *seed*, penyusunan `sqlc.yaml`, pengaturan konfigurasi aplikasi, serta pembuatan *middleware* dasar (mencakup *request ID*, *logging*, *recover*, dan standar error `problem+json`). Selain itu, fase ini juga menyelesaikan *endpoint health check*, jalur *Continuous Integration* (CI), proses *deploy* pertama ke *environment* staging, serta penyusunan kerangka `openapi.yaml` beserta *mock server*. Dengan fondasi ini, tim frontend sudah dapat mulai membuat *client* dari OpenAPI dan melakukan *setup* proyek mereka.

**Fase 2: Auth dan Identitas (Minggu 2)**
Memasuki minggu kedua, backend membangun seluruh sistem autentikasi dan manajemen identitas yang meliputi registrasi, *login*, integrasi Google, mekanisme *refresh token* dengan rotasi, fitur lupa kata sandi, verifikasi email, dan pengelolaan profil. Untuk sisi internal, dibangun pula sistem *login* admin yang dilengkapi *Multi-Factor Authentication* (MFA) dan mekanisme *lockout*, *middleware* pencatatan *audit log*, serta layanan pengiriman email. Tim frontend dapat langsung menggarap tampilan dan alur *onboarding*, *login*, halaman profil, serta menu pengaturan.

**Fase 3: Katalog dan Konservasi (Minggu 2 hingga Minggu 3)**
Pada minggu kedua hingga ketiga, fokus beralih ke pengelolaan data inti konservasi dan katalog melalui pembuatan fitur CRUD untuk admin yang mencakup zona, proyek, *frame*, produk, *plan*, misi, dan berita (*news*). Di sisi lain, disediakan pula API baca publik untuk menampilkan daftar maupun detail proyek, berita, dan katalog toko, serta mekanisme *presigned upload* untuk unggahan media. Tim frontend dapat mengerjakan halaman daftar serta detail donasi, halaman berita, dan tampilan antarmuka toko.

**Fase 4: Donasi, Pembayaran, dan Webhook (Minggu 3 hingga Minggu 5 - Bagian Paling Berisiko)**
Fase ini merupakan bagian dengan risiko teknis tertinggi dan berlangsung dari minggu ketiga hingga kelima. Backend mengimplementasikan proses *checkout* yang dilengkapi *idempotency*, penguncian poin, pencatatan *snapshot* kurs mata uang (*FX*), *adapter* integrasi Xendit (mendukung QRIS dan Virtual Account), *inbox* untuk menampung *webhook*, pemrosesan transaksi *"on paid"* yang bersifat atomik, *background job* untuk menangani transaksi kedaluwarsa, serta pencatatan riwayat donasi. Pada tahap ini, tim frontend membangun layar pembayaran, layar status sukses, dan halaman riwayat, yang bermuara pada demo *end-to-end* meliputi alur pendaftaran akun, simulasi donasi di *sandbox*, penerimaan *webhook paid*, hingga munculnya fragmen karang.

**Fase 5: Fragmen, Frame, dan Data Lapangan (Minggu 5 hingga Minggu 6)**
Berjalan pada minggu kelima hingga keenam, backend membangun logika penempatan fragmen karang ke dalam *frame* dengan mekanisme penguncian (*lock*), antarmuka bagi admin untuk menginput log penanaman lapangan, fitur unggah bukti foto, lini masa pencapaian (*milestone*) pada bulan ke-1, ke-3, dan ke-6, serta pencatatan insiden di lapangan. Bersamaan dengan itu, tim frontend dapat menyelesaikan halaman *My Adopted Corals* serta fitur *restoration updates*.

**Fase 6: Gamifikasi (Minggu 5 hingga Minggu 7)**
Dari minggu kelima hingga ketujuh, backend mengembangkan fitur keterlibatan pengguna yang terdiri dari sistem *pet* (mendukung lebih dari satu *pet* per pengguna), sistem misi berbasis *Computer Vision* (menggunakan *stub* terlebih dahulu sebelum diganti dengan model asli), sistem *streak* yang menyesuaikan zona waktu pengguna, buku besar (*ledger*) pencatatan poin, fitur *boost Bio-Shield*, papan peringkat (*ranking*), serta pengiriman notifikasi melalui Firebase Cloud Messaging (FCM). Tim frontend dapat menggarap layar beranda (*home*), interaksi *pet*, daftar misi, fitur kamera, papan peringkat, dan halaman notifikasi.

**Fase 7: Toko, IAP, dan Langganan (Minggu 7 hingga Minggu 9)**
Pada minggu ketujuh hingga kesembilan, backend menangani sistem transaksi komersial yang mencakup pembelian poin serta integrasi *In-App Purchase* (IAP) untuk Apple dan Google lengkap dengan verifikasi di sisi server, pengelolaan inventaris pengguna, serta pemrosesan langganan berulang oleh *worker* (termasuk logika *retry*, *pause*, dan *resume*). Tim frontend dapat membangun tampilan *Shop*, pilihan *plan*, dan fitur berlangganan *BioVerse Plus*.

**Fase 8: Operator dan Payout (Minggu 8 hingga Minggu 10)**
Memasuki minggu kedelapan hingga kesepuluh, pengembangan difokuskan pada operasional lapangan melalui fitur *Know Your Customer* (KYC) operator yang dilengkapi enkripsi untuk data NIK dan nomor rekening, pencatatan log kerja (*work log*), serta sistem agregasi *payout* dan pencairan dana. Karena sifatnya internal, pekerjaan pada fase ini hanya melibatkan antarmuka *back office* untuk admin.

**Fase 9: B2B dan ESG (Minggu 10 hingga Minggu 13)**
Pada minggu kesepuluh hingga ketiga belas, backend membangun ekosistem kemitraan korporat (B2B) yang mencakup manajemen anggota dan undangan, pengelolaan paket, kontrak, *invoice*, dan kuota sponsor. Selain itu, disediakan API untuk *dashboard* B2B, pengajuan permintaan layanan lapangan, generator laporan *Environmental, Social, and Governance* (ESG) dalam format PDF, CSV, dan ZIP, serta penyediaan aset untuk kebutuhan *Public Relations* (PR). Tim frontend dapat mengerjakan keseluruhan aplikasi web B2B pada fase ini.

**Fase 10: Hardening dan Rilis (Minggu 13 hingga Minggu 15)**
Fase penutup pada minggu ketiga belas hingga kelima belas mencakup peninjauan keamanan menyeluruh, pengujian beban (*load test*), penerapan *observability* dan sistem peringatan (*alert*), mekanisme *backup* beserta uji coba *restore* data, pemenuhan kepatuhan Undang-Undang Perlindungan Data Pribadi (UU PDP), *deployment* ke *environment* produksi, penyusunan *runbook* operasional, serta dukungan teknis untuk proses pengajuan (*submit*) aplikasi ke App Store dan Play Store. Tim frontend mendampingi fase ini dengan melakukan pengujian akhir dan persiapan rilis.

**Catatan Titik Demo:** Setelah Fase 6 hingga Fase 7 rampung (sekitar minggu ke-8 hingga ke-9), seluruh alur B2C dan sistem admin sudah siap untuk didemokan secara utuh. Apabila jadwal terasa terlalu ketat, pengerjaan modul B2B pada Fase 9 dapat digeser.

## Fase 0: Pekerjaan Non-Kode (Mulai Sekarang, Jangan Menunggu)

Seluruh persiapan non-teknis berikut harus segera dimulai dari sekarang tanpa menunggu pengembangan kode berjalan:

*   **Xendit:** Melakukan pendaftaran akun dan memulai proses verifikasi bisnis (*KYB*) karena tahap ini memakan waktu paling lama, sekaligus melakukan verifikasi fitur QRIS, Virtual Account (VA), dan *payout* pada mode *sandbox*.
*   **Apple dan Google:** Menyiapkan akun *developer* resmi, mendaftarkan daftar produk *In-App Purchase* (IAP), serta meninjau kebijakan Apple terkait aturan pembayaran adopsi karang di luar mekanisme IAP.
*   **Firebase:** Membuat *project* baru untuk keperluan layanan notifikasi *push* melalui FCM.
*   **AWS dan Domain:** Menyiapkan akun AWS, mengamankan nama domain, mengonfigurasi sertifikat SSL, serta menyiapkan layanan penyimpanan S3 dan basis data RDS.
*   **Email Transaksional:** Menyiapkan layanan pengiriman email transaksional (seperti Amazon SES atau layanan sejenisnya) untuk menangani pengiriman tautan verifikasi akun dan pengaturan ulang kata sandi. Kebutuhan ini belum dibahas sebelumnya, namun wajib tersedia saat memasuki Fase 2.
*   **Google OAuth:** Membuat *Client ID* untuk masing-masing platform yang didukung, yaitu Android, iOS, dan web.
*   **Tim Figma:** Melengkapi desain antarmuka untuk tiga kotak informasi pada halaman *Mission Details*, menyusun daftar kategori misi, serta melengkapi desain layar untuk operator dan layar admin apabila belum tersedia.
*   **Model CV:** Menetapkan pihak yang bertanggung jawab sebagai pemilik model *Computer Vision* (CV) beserta target tingkat akurasi yang harus dicapai.
*   **Hukum:** Menyusun dokumen Kebijakan Privasi dan Syarat Layanan yang selaras dengan ketentuan UU PDP.

## Kontrak Teknis dengan Frontend (Disepakati Sebelum Fase 2)

Untuk memastikan integrasi berjalan mulus tanpa hambatan komunikasi, poin-poin kesepakatan teknis berikut wajib dikunci sebelum memasuki Fase 2:

*   **OpenAPI-First:** Pengembangan berpatokan penuh pada dokumen spesifikasi. Tim frontend akan menghasilkan kode *client* Dart langsung dari berkas `openapi.yaml` dan menggunakan *mock server* sebelum *endpoint* asli selesai dibangun. Setiap perubahan yang berpotensi merusak kompatibilitas (*breaking changes*) wajib diumumkan melalui *changelog*.
*   **Tiga Environment:** Infrastruktur dibagi menjadi tiga lingkungan kerja, yaitu **lokal** (berbasis `docker-compose`), **staging** (selalu aktif dengan *auto-deploy* dari *branch* `main`, serta sudah terisi *seed data*, akun uji coba, dan terhubung ke Xendit *sandbox*), dan **produksi**.
*   **Autentikasi:** Menggunakan skema *Bearer JWT* dengan masa berlaku 15 menit yang dipasangkan dengan *refresh token* berotasi. Apabila *client* menerima respons status `401`, aplikasi wajib memanggil *endpoint refresh* sebanyak satu kali lalu mengulang *request* sebelumnya; jika proses *refresh* tersebut gagal, pengguna otomatis dikeluarkan (*logout*).
*   **Format Umum:** Seluruh respons kesalahan (*error*) menggunakan standar `problem+json` dengan kode error yang stabil. Daftar data menggunakan metode *pagination cursor*, nilai uang direpresentasikan sebagai *string* desimal yang disertai kode mata uang, dan seluruh penanda waktu menggunakan format ISO 8601 dalam zona waktu UTC.
*   **Alur Pembayaran:** *Client* wajib menghasilkan UUID baru sebagai `Idempotency-Key` pada setiap upaya *checkout*. Backend akan mengembalikan *payload* QRIS atau nomor Virtual Account, yang kemudian ditampilkan oleh *client* lengkap dengan penghitung waktu mundur berdasarkan nilai `expired_at`. Status pembayaran dipantau melalui mekanisme *polling* ke `GET /donations/{id}` yang dikombinasikan dengan notifikasi *push* saat pembayaran lunas, sementara urusan *webhook* sepenuhnya ditangani di sisi backend.
*   **Alur Unggah Foto:** Proses unggah dilakukan dalam tiga langkah berurutan, dimulai dari meminta URL *presigned*, mengunggah berkas langsung ke S3, lalu mengirimkan kunci objek (*object key*) tersebut ke *endpoint submit*. Seluruh batasan ukuran berkas dan tipe file yang diizinkan dicatat secara eksplisit di dalam dokumen OpenAPI.
*   **Alur In-App Purchase (IAP):** *Client* melakukan transaksi pembelian langsung melalui *store* (App Store/Play Store), mengirimkan bukti transaksi ke server, dan server akan melakukan verifikasi sebelum menambahkan item ke inventaris pengguna. Aplikasi *client* tidak pernah diizinkan menambah isi inventaris secara mandiri.
*   **Notifikasi Push:** Backend menyediakan *endpoint* khusus untuk mendaftarkan token FCM perangkat, disertai dengan katalog jenis notifikasi lengkap beserta struktur *payload deep link* masing-masing.
*   **CORS dan Domain:** Pengaturan kebijakan CORS serta penentuan nama domain untuk aplikasi web B2B dan panel admin ditetapkan sejak Fase 1.
*   **Rutinitas Komunikasi:** Tim menyelenggarakan sesi sinkronisasi singkat setiap minggu dan menggunakan satu kanal komunikasi bersama khusus untuk membahas pertanyaan seputar kontrak API.

## Risiko Utama

Terdapat empat risiko utama yang perlu diantisipasi sepanjang proses pengembangan ini. Pertama, alur pengelolaan uang pada Fase 4 dan Fase 7 merupakan titik paling rawan kesalahan, sehingga pengujian terhadap duplikasi *webhook*, kegagalan transaksi sebagian, serta kondisi balapan (*race condition*) pada kapasitas *frame* wajib dipastikan lulus sebelum melangkah ke tahap berikutnya. Kedua, proses verifikasi bisnis (KYB) Xendit dan persetujuan produk IAP dari toko aplikasi berpotensi memakan waktu di luar kendali tim pengembang. Ketiga, ketidaksiapan model *Computer Vision* tidak boleh menghambat jalannya pengembangan aplikasi; sistem harus tetap berjalan menggunakan *stub* terlebih dahulu dan baru diganti ketika model aslinya sudah siap. Keempat, layar admin saat ini belum memiliki rancangan desain sehingga pengerjaannya harus dimulai dengan antarmuka pengguna (UI) yang sederhana mengingat seluruh data lapangan untuk versi MVP akan diinput melalui panel admin.
