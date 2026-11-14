bila membuat notebook:
- Posisikan cell 1 khusus install library luar jika diperlukan, atau gunakan terminal/environment manager lokal.
- Posisikan cell 2 khusus import library, global setup, dan set global random seed (numpy, torch, random) demi reproducibility.
- Posisikan cell 3 khusus semua constant, parameter, dan file path yang bisa disesuaikan dari awal agar mudah diatur.
- Bikin code secara modular per cell dan hindari penulisan code dalam satu single cell.
- Pisahkan proses data loading dari data processing agar tidak perlu memuat ulang dataset dari disk/network saat menjalankan ulang logic.
- Bebaskan 3 cell pertama dari aturan penomoran header.
- Print header di baris pertama setiap cell mulai cell 4 dengan format nomor cell dan description pekerjaan cell.
- Gunakan execution step tracker untuk memastikan cell dieksekusi berurutan.
- Jaga kerapatan baris code vertikal dan hindari blank space berlebihan ke bawah.
- Pastikan setiap cell menghasilkan output ringkas yang informatif dan menjelaskan proses riil, bukan sekadar print confirmation.
- Bedakan display output cell check data dengan cell execution yang mengubah isi global variable.
- Lakukan cleanup temporary variable berukuran besar dan jalankan garbage collection di akhir cell berat agar RAM tidak OOM.
- Check prerequisite cell sebelumnya sebelum berlanjut ke step berikutnya. Stop flow dan berikan alert langsung jika ditemukan issue.

bila membuat projek secara umum (berlaku untuk semua stack):
- Wajib buat .gitignore di awal. Pastikan secret, .env, dan folder dependencies (node_modules, venv, cache) tidak masuk ke version control.
- Sediakan README.md yang memuat langkah setup lokal, environment variables yang dibutuhkan, dan cara menjalankan project.
- Tulis seluruh code, log print, dan comment wajib menggunakan bahasa Inggris.
- Tempatkan comment hanya di bagian atas big section code untuk menjelaskan apa yang dilakukan blok tersebut. Dilarang menggunakan inline comments.
- Gunakan single-line comment syntax sesuai bahasa pemrograman (misal // atau #) dan dilarang menggunakan internal numbering system pada comment.
- Isolasi semua constant, parameter, secret, dan konfigurasi ke file environment (.env) atau config terpisah. Dilarang melakukan hardcode parameter di level logic.
- Bangun arsitektur modular berbasis file dan folder (misal: src/, utils/, config/, tests/). Jangan menumpuk logic di satu file raksasa.
- Tulis unit test untuk fungsi kritikal atau core logic agar code teruji secara terisolasi.
- Wajib gunakan strict type hinting/typing di semua function signature, parameter, dan return value (misal: module typing di Python, TypeScript untuk ekosistem JS) untuk memperjelas I/O data.
- Output code wajib mematuhi standar strict formatter dan linter bahasa terkait (misal: PEP8/Black/Ruff untuk Python, Prettier/ESLint untuk frontend/backend web, gofmt untuk Go).
- Pertahankan variable name dan logika lama saat refactoring, kecuali ada alasan mendasar untuk performance atau maintainability.
- Jika ada perbaikan code sederhana, tunjukkan lokasi spesifik dan replacement code-nya saja tanpa me-rewrite seluruh file atau cell.
- Tulis code yang future-proof, mengikuti best practice bahasa tersebut, memiliki error handling yang mencakup edge cases, dan efisien dalam execution time.
- Jika code output mengharuskan full copy-paste file, berikan code utuh siap pakai tanpa elipsis (...), tanpa baris kosong yang terpotong, dan tunjukkan path lokasi pasti di mana file tersebut harus diletakkan.

bila membuat projek backend atau scripting (Python, Node.js, Go, dll):
- Pisahkan dependency management ke file terpusat (requirements.txt, pyproject.toml, package.json, go.mod).
- Gunakan explicit entry point (misal if __name__ == "__main__": di Python atau index.js/main.go) untuk membedakan script eksekusi dari module pendukung.
- Gunakan standard logging library (level INFO, DEBUG, ERROR) untuk tracking. Dilarang menggunakan print sembarangan di level production code.
- Terapkan fail-fast mechanism. Validasi input, parameter, atau koneksi di awal function. Lempar error spesifik langsung jika tidak terpenuhi sebelum menjalankan logic berat.
- Hindari penggunaan global variable. Passing data melalui function arguments atau class attributes untuk menjaga memory scope terisolasi.
- Terapkan try-catch/except eksplisit pada blok operasi rawan gagal (I/O, database query, network request) dengan fallback atau log error yang spesifik.
- Gunakan connection pooling untuk akses database. Jangan buka-tutup koneksi mandiri di setiap request.
- Terapkan security middleware dasar jika mengekspos API, minimal CORS dan rate limiting.
- Sediakan dokumentasi API terstandardisasi (misal OpenAPI/Swagger) atau minimal file koleksi request (misal Postman collection) di dalam repository.

bila membuat projek frontend atau web (React, Vue, HTML/JS, dll):
- Pisahkan secara tegas antara UI component (presentation layer), business logic (hooks/services/controllers), dan state management.
- Ekstrak reusable UI elements menjadi komponen modular. Hindari single file component yang membengkak ribuan baris.
- Simpan semua assets (images, fonts) dan global styling di folder terdedikasi (public/ atau assets/).
- Hindari prop drilling di level hirarki yang dalam. Gunakan context atau state manager jika data digunakan secara global.
- Handle interaksi external API secara asynchronous dengan merender state UI yang jelas (loading, success, error fallback).
- Lakukan sanitasi data dari external input atau API sebelum di-render ke DOM untuk mencegah celah security seperti XSS.
- Terapkan lazy loading atau code splitting pada routing/komponen besar untuk mengoptimalkan performa load time.
- Gunakan semantic HTML dan penuhi standar accessibility (a11y) dasar, seperti atribut alt pada gambar dan struktur navigasi yang bisa diakses keyboard.

kamu bisa ignore bagian yang lain kalau tidak relevan