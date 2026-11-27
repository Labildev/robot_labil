# Product & Technical Requirements Document (PRD & TRD)
**Nama Proyek:** Local AI Automation Robot (100% Offline Document Assistant & Affiliate Video Pipeline)  
**Target Lingkungan:** Komputer Lokal Windows/Linux (Total RAM 12 GB, Alokasi Robot ≤ 8 GB, GPU NVIDIA RTX 2050 4 GB VRAM)  
**Pendekatan Pengembangan:** 100% Full Local Native Binary berbasis **Go (Golang)** + Local LLM ([Ollama](https://ollama.com/))  
**Tanggal Dokumen:** 27 November 2026  

---

## Ringkasan Eksekutif

Dokumen ini menggabungkan **Product Requirements Document (PRD)** dan **Technical Requirements Document (TRD)** untuk membangun sistem **AI Automation 100% Full Local (Offline)** yang berjalan di perangkat pribadi dengan alokasi memori ketat (**maksimal 8 GB RAM** dan **4 GB VRAM RTX 2050**). Sistem dirancang sebagai "robot asisten" modular berbasis **single binary Go (Golang)**—tanpa n8n, tanpa Docker/WSL2, dan tanpa ketergantungan layanan *cloud*—sehingga ringan, cepat, hemat memori, serta menjaga privasi data sepenuhnya.

Pada fase **Minimum Viable Product (MVP)**, sistem berjalan **100% offline** dengan dua kapabilitas utama:
1. **Asisten Dokumen Lokal:** Membaca, merangkum, dan menjawab pertanyaan dari file PDF/Word lokal secara *offline*.
2. **Pipeline Konten Affiliate Lokal:** Membuat naskah promosi, merakit dan merender video secara otomatis melalui [FilmCraft](https://github.com/storytold/filmcraft) (dengan cadangan FFmpeg), lalu mengemas hasil video `.mp4` dan `caption.txt` siap unggah ke folder output lokal agar kreator tinggal mengunggah dan menempelkan tautan produk (*keranjang kuning*) di TikTok.

---

## Bagian 1: Product Requirements Document (PRD)

### 1.1 Visi Produk & Tujuan MVP
- **Visi:** Memiliki sistem otomasi AI pribadi yang berjalan mandiri 100% di komputer lokal, bebas biaya langganan API AI bulanan, tanpa kebocoran data ke internet, serta mampu bekerja seperti asisten produksi konten harian dengan konsumsi resource minimal.
- **Tujuan MVP:**
  - Memangkas waktu riset dan perangkuman dokumen PDF/Word lokal hingga >70%.
  - Mengotomatisasi pembuatan naskah *hook*, *body*, *caption*, dan *hashtag* video affiliate secara lokal.
  - Mengotomatisasi perakitan dan rendering video pendek melalui CLI/server kontrol FilmCraft.
  - Menghasilkan paket konten lokal siap unggah (`.mp4` + `caption.txt`) yang aman dari risiko pemblokiran akun/bot platform.

### 1.2 Ruang Lingkup (Scope)

| Kategori | In-Scope (Masuk MVP — 100% Offline) | Out-of-Scope (Fase Lanjutan / Opsional Online) |
| :--- | :--- | :--- |
| **Olah Dokumen** | Ekstraksi teks, ringkasan, dan tanya-jawab PDF/DOCX lokal | OCR dokumen scan tulisan tangan massal atau indeks jutaan file |
| **Pembuatan Video** | *Automated video assembly* (gabung klip/foto produk, teks overlay, musik, voiceover lokal) via FilmCraft / FFmpeg | *AI Text-to-Video generation* dari nol (Sora/Wan/CogVideoX) karena batas 4 GB VRAM |
| **Output Konten Affiliate** | Paket lokal siap unggah di folder output (`video.mp4` + `caption.txt` + metadata job) untuk diunggah & ditempel *keranjang kuning* manual oleh kreator | Auto-upload via [TikTok Content Posting API](https://developers.tiktok.com/doc/content-posting-api-get-started-upload-content?enter_method=left_navigation), [TikTok Shop Affiliate Creator API](https://partner.tiktokshop.com/docv2/page/affiliate-creator-api-overview), atau bot klik browser |
| **Antarmuka** | CLI Go + Local Web Dashboard ringan (`http://localhost:8080`) untuk status antrean, Q&A dokumen, dan input produk | Aplikasi desktop berat, *workflow engine* eksternal, atau web SaaS multi-user |

### 1.3 Kebutuhan Fungsional (Functional Requirements)

| ID | Modul | Deskripsi Kebutuhan | Prioritas |
| :--- | :--- | :--- | :--- |
| **FR-01** | **Folder Watcher & Trigger** | Sistem dapat dipicu melalui perintah CLI, form di Web Dashboard lokal, penjadwal internal (*cron*), atau otomatis saat ada file baru masuk ke folder input (`fsnotify`). | Tinggi (P0) |
| **FR-02** | **Ekstraksi Dokumen Lokal** | Sistem mampu mengekstrak teks bersih dari dokumen berformat `.pdf` dan `.docx` yang disimpan di direktori lokal. | Tinggi (P0) |
| **FR-03** | **Ringkasan & Q&A Dokumen** | AI lokal ([Ollama](https://ollama.com/)) mampu menghasilkan ringkasan terstruktur (poin utama, kesimpulan) dan menjawab pertanyaan spesifik berdasarkan isi dokumen. | Tinggi (P0) |
| **FR-04** | **Generator Naskah Affiliate** | Berdasarkan input nama produk/deskripsi singkat atau dokumen produk, AI menghasilkan naskah video pendek (Hook 3 detik, Isi/Keunggulan, CTA ke keranjang kuning), *caption*, dan *hashtag*. | Tinggi (P0) |
| **FR-05** | **Perakitan & Render FilmCraft** | Orkestrator Go mengirimkan perintah ke [FilmCraft](https://github.com/storytold/filmcraft) (melalui `os/exec` CLI / JSON Control Server) untuk menggabungkan aset klip/gambar, menambahkan teks *subtitle/hook*, audio, dan mengekspor file `.mp4` rasio 9:16 (1080x1920). | Tinggi (P0) |
| **FR-06** | **Paket Output Siap Upload** | Sistem menyimpan hasil video `.mp4`, file `caption.txt` (siap *copy-paste*), dan ringkasan job ke folder `storage/output_videos/<job_id>/` dengan status `READY_FOR_UPLOAD`. | Tinggi (P0) |
| **FR-07** | **Antrean Tugas (Serial Queue)** | Sistem memproses pekerjaan berat (inferensi LLM dan rendering video) satu per satu secara berurutan melalui worker queue serial di Go agar tidak terjadi tabrakan memori. | Tinggi (P0) |
| **FR-08** | **Dashboard Status & Log Lokal** | Sistem mencatat status setiap pekerjaan (*Queued*, *Processing*, *Done*, *Failed*) ke SQLite & structured log lokal serta menampilkannya di Web Dashboard lokal `http://localhost:8080`. | Sedang (P1) |

### 1.4 Kebutuhan Non-Fungsional (Non-Functional Requirements)

| ID | Aspek | Target / Batasan |
| :--- | :--- | :--- |
| **NFR-01** | **Batas RAM Sistem Robot** | Total konsumsi RAM seluruh proses robot (binary Go + Ollama + FilmCraft) **wajib ≤ 8 GB** agar sisa ≥ 4 GB RAM tetap aman untuk OS Windows/host. |
| **NFR-02** | **Batas VRAM GPU** | Model AI lokal wajib menggunakan kuantisasi 4-bit (`Q4_K_M`) berkapasitas **≤ 3,5 GB VRAM** agar muat penuh di NVIDIA RTX 2050 (4 GB VRAM) tanpa *spill* berlebihan ke RAM ([FitMyLLM](https://www.fitmyllm.com/blog/gpu/geforce-rtx-2050-mobile)). |
| **NFR-03** | **Privasi & 100% Offline** | Seluruh komponen MVP berjalan **100% offline / full local** di komputer pribadi tanpa mengirim data apa pun ke server eksternal atau internet. |
| **NFR-04** | **Kemudahan Pemeliharaan & Konfigurasi** | Kode Go dibangun modular per domain (`watcher`, `doc`, `llm`, `video`, `queue`, `web`) dengan konfigurasi eksternal (`config.yaml`) dan template *prompt* terpisah (`.txt`/`.md`) agar aturan kerja mudah diubah tanpa menyentuh logika inti. |
| **NFR-05** | **Keandalan (Fault Tolerance)** | Jika terjadi kegagalan pada satu tahap (misal format file rusak atau proses render gagal), worker mencatat error jelas di SQLite/log dan memindahkan file bermasalah ke folder `storage/error_logs/` tanpa menghentikan antrean berikutnya. |

### 1.5 User Stories & Acceptance Criteria

1. **US-01 (Asisten Dokumen):** Sebagai pengguna, saya ingin menaruh file PDF/Word ke folder `storage/input_docs/` atau mengirimnya lewat CLI/web lokal, lalu mendapatkan ringkasan poin penting dalam bahasa Indonesia di file `.md`, sehingga saya tidak perlu membaca dokumen panjang secara manual.
   - *Acceptance Criteria:* File PDF/DOCX terbaca tanpa karakter rusak, ringkasan dihasilkan dalam <60 detik untuk dokumen ≤15 halaman, dan seluruh proses berjalan 100% tanpa koneksi internet.
2. **US-02 (Generator Konten Affiliate):** Sebagai kreator affiliate, saya ingin memasukkan nama produk dan deskripsi singkat (via CLI/form lokal/JSON), lalu robot menghasilkan naskah video 15–30 detik dan merakit aset video di `storage/affiliate_assets/` menggunakan FilmCraft menjadi file `.mp4` siap unggah.
   - *Acceptance Criteria:* Output berupa file `.mp4` (9:16, 1080p) beserta file `caption.txt` (berisi hook, caption, dan hashtag) di folder `storage/output_videos/<job_id>/`.
3. **US-03 (Pemantauan Antrean Lokal):** Sebagai pengguna, saya ingin melihat status antrean dokumen dan render video secara lokal melalui CLI atau browser di `http://localhost:8080`, sehingga saya tahu pekerjaan mana yang sedang diproses, selesai (`READY_FOR_UPLOAD`), atau gagal.
   - *Acceptance Criteria:* Status setiap job tersimpan persisten di database SQLite lokal dan langsung diperbarui saat worker berpindah tahap.

---

## Bagian 2: Technical Requirements Document (TRD)

### 2.1 Arsitektur Sistem & Aliran Data

Sistem menggunakan arsitektur **Modular Pipeline Native 100% Local** berbasis **single binary Go** yang berjalan langsung di *host* Windows/Linux dengan prinsip **eksekusi serial** (tanpa VM, tanpa Docker, dan tanpa ketergantungan layanan eksternal):

1. **Layer Input & Trigger:** *File system watcher* (`fsnotify`), *internal cron scheduler*, perintah CLI (`robot summarize`, `robot affiliate`, `robot serve`), dan *Local Web Server* (`net/http` di `http://localhost:8080`).
2. **Layer Orkestrasi (Go Core Engine):** Single binary Go yang mengatur antrean serial (`Concurrency = 1`), ekstraksi dokumen, chunking, pemanggilan REST API Ollama lokal, eksekusi FilmCraft/FFmpeg via `os/exec`, dan pengemasan output.
3. **Layer Inferensi AI ([Ollama](https://ollama.com/)):** Berjalan native di *host* Windows (`http://127.0.0.1:11434`) agar akses langsung ke CUDA RTX 2050 berjalan optimal.
4. **Layer Pemrosesan Media ([FilmCraft](https://github.com/storytold/filmcraft) + FFmpeg):** Berjalan native di *host* Windows untuk memanfaatkan akselerasi hardware GPU saat *compositing* dan ekspor video.
5. **Layer Penyimpanan & Log:** Sistem file lokal terstruktur, **Embedded SQLite** (untuk antrean job, riwayat status, dan metadata), serta *structured logging* (`log/slog`) ke stdout dan file log lokal.

### 2.2 Tech Stack Terpilih & Alokasi Memori (Budget ≤ 8 GB RAM + 4 GB VRAM)

| Layer | Teknologi | Peran Teknis | Estimasi RAM | Estimasi VRAM |
| :--- | :--- | :--- | :--- | :--- |
| **Orkestrator, API & Web UI** | **Go (Golang 1.23+)** (`net/http`, `html/template`, `fsnotify`, `log/slog`) | Single binary native untuk watcher, scheduler, queue serial, CLI, dan Web Dashboard lokal | **~20 – 80 MB** | 0 MB |
| **Database Lokal** | **Embedded SQLite** (`modernc.org/sqlite` / `go-sqlite3`) | Penyimpanan antrean pekerjaan (*persistent queue*), status job, dan histori Q&A | (Tercakup di Go) | 0 MB |
| **Local LLM Engine** | [Ollama](https://ollama.com/) (Native Windows + CUDA) | Menjalankan model AI lokal secara offline di `127.0.0.1:11434` | ~500 MB – 1,2 GB | ~2,5 – 3,5 GB |
| **Model AI Utama** | `qwen2.5:3b` atau `llama3.2:3b` (Quantization `Q4_K_M`) | Tugas perangkuman dokumen, Q&A, dan penulisan naskah affiliate berbahasa Indonesia | (Tercakup di Ollama) | ~2,2 – 2,8 GB |
| **Ekstraktor Dokumen** | Go `DOCX` parser (`archive/zip` + `encoding/xml`) & `pdftotext` (Poppler CLI) / Go PDF lib | Ekstraksi teks cepat & bersih dari `.docx` dan `.pdf` secara offline | ~20 – 100 MB (saat proses) | 0 MB |
| **Video Editor Engine** | [FilmCraft](https://github.com/storytold/filmcraft) (CLI / JSON Control Server) + FFmpeg | Perakitan timeline multitrack, teks overlay, audio, dan rendering MP4 ([SourceForge](https://sourceforge.net/projects/filmcraft.mirror/)) | ~1,5 – 2,5 GB (saat render) | ~1 – 2 GB (saat render) |
| **Helper Audio/TTS Lokal** | **Piper TTS** (binary lokal offline) | Menghasilkan suara *voiceover* offline dari naskah AI sebelum masuk timeline FilmCraft | ~150 – 300 MB (saat sintesis) | 0 MB |
| **Total Puncak (Peak Serial)** | **Berjalan bergantian (Ollama → Piper → FilmCraft)** | **Total alokasi jauh lebih lega di bawah batas 8 GB RAM** | **~2,2 – 4,0 GB** | **< 3,8 GB** |

### 2.3 Strategi & Konfigurasi Optimasi Hardware

Karena total RAM fisik adalah 12 GB dan **alokasi maksimal untuk robot adalah 8 GB**, konfigurasi berikut diterapkan:

1. **Eksekusi Native Murni (Tanpa Docker / Middleware Eksternal):**
   Dengan menggunakan binary **Go native**, tidak ada VM WSL2 atau kontainer yang mengikat RAM secara permanen (`~2–4 GB` RAM terhemat), sehingga memori laptop tetap lega untuk Ollama dan FilmCraft.
2. **Konfigurasi Environment Variables & Siklus VRAM Ollama:**
   Atur variabel lingkungan berikut di Windows agar Ollama tidak menahan VRAM saat tidak dipakai:
   - `OLLAMA_HOST=127.0.0.1:11434` (mengikat endpoint khusus loopback lokal).
   - `OLLAMA_MAX_LOADED_MODELS=1` (hanya 1 model aktif dalam satu waktu).
   - `OLLAMA_KEEP_ALIVE=2m` (model otomatis di-*unload* dari VRAM setelah 2 menit menganggur).
   - `OLLAMA_NUM_PARALLEL=1` (mencegah pemrosesan paralel yang menggandakan konsumsi VRAM).
   - Batas konteks (`num_ctx`) di payload Go ke Ollama diatur ke `4096` token agar penggunaan memori tetap stabil ([FitMyLLM](https://www.fitmyllm.com/blog/gpu/geforce-rtx-2050-mobile)).
   - **Optimasi Aktif dari Go:** Pada request Ollama terakhir tepat sebelum tahap *render* FilmCraft, Go mengirim `keep_alive: "0"` agar VRAM RTX 2050 langsung kosong seketika sebelum rendering dimulai.
3. **Aturan Eksekusi Serial (Single Worker Goroutine):**
   Di dalam engine Go, seluruh pekerjaan berat (inferensi LLM, sintesis TTS, dan rendering video) diproses oleh **1 worker goroutine** (`max_concurrency = 1`) yang mengambil tugas dari SQLite queue secara berurutan. Langkah *Generate Script (Ollama)* wajib selesai penuh sebelum proses *Render (FilmCraft)* dimulai.

### 2.4 Spesifikasi Integrasi Antar Komponen

#### A. Integrasi Go ↔ Ollama (Teks & Dokumen)
- **Endpoint Lokal:** `http://127.0.0.1:11434/api/chat` (dengan `http.Client` timeout yang memadai untuk inferensi lokal).
- **Alur Dokumen PDF/Word:**
  1. `watcher` (`fsnotify`) atau CLI mendeteksi file baru di `storage/input_docs/`.
  2. Paket `doc` mengekstrak teks mentah dari `.docx` (native Go ZIP/XML) atau `.pdf` (`pdftotext` via `os/exec` dengan encoding UTF-8).
  3. Jika dokumen panjang (>3.000 kata), paket `doc` memecah teks per *chunk* ~2.000 kata, lalu mengirimnya secara berurutan ke Ollama untuk dirangkum bertahap (*Map-Reduce summarization*).
  4. Hasil akhir disimpan otomatis sebagai file `.md` di `storage/output_docs/` dan status job diperbarui di SQLite.

#### B. Integrasi Go ↔ FilmCraft & Piper TTS (Otomatisasi Video Lokal)
FilmCraft mengekspos aksi penyuntingan melalui perintah CLI, JSON Control Server, dan MCP ([SourceForge / FilmCraft](https://sourceforge.net/projects/filmcraft.mirror/)).
- **Metode Integrasi:**
  1. (Opsional) Paket `video` memanggil **Piper TTS** secara lokal via `os/exec` untuk mengubah `voiceover_script` menjadi file `.wav` offline.
  2. Paket `video` menyiapkan file manifest/instruksi edit (`project_job.json`) di `storage/affiliate_assets/jobs/<job_id>/` berisi path aset video/foto produk, teks *hook* dari Ollama, file audio *voiceover*, dan durasi per klip.
  3. Go memanggil FilmCraft menggunakan `exec.CommandContext` (atau `http.Client` ke JSON Control Server lokal) untuk merakit proyek dan mengekspor video ke `storage/output_videos/<job_id>/<job_id>.mp4`.
  4. **Catatan Penting Implementasi:** Karena FilmCraft masih dalam pengembangan aktif, pastikan versi binary yang diinstal sudah diuji perintah CLI/JSON-nya terlebih dahulu pada 1 klip contoh sebelum diintegrasikan penuh. Paket `video` wajib menyediakan **FFmpeg fallback** (`os/exec`) untuk perakitan klip sederhana (concat + drawtext + audio mix).

#### C. Pengemasan Output Lokal & Local Web Dashboard (`http://127.0.0.1:8080`)
- **Output Paket Konten:** Setelah video selesai dirender, Go menuliskan `caption.txt` (hook, caption, hashtag, dan catatan nama produk) serta `job_payload.json` di folder `storage/output_videos/<job_id>/` dengan status `READY_FOR_UPLOAD`.
- **Web Dashboard Lokal:** Dibangun menggunakan `net/http` + `html/template` bawaan Go (tanpa bundler/Node.js) dan hanya di-bind ke `127.0.0.1:8080` untuk:
  - Memantau antrean job & log error lokal.
  - Mengunggah/memicu ringkasan dokumen serta Q&A berbasis isi dokumen.
  - Mengisi form produk affiliate untuk masuk ke antrean render lokal.

### 2.5 Struktur Direktori Proyek Go yang Direkomendasikan

```text
robot_labil/
├── cmd/
│   └── robot/
│       └── main.go                 # Entrypoint CLI & Local Web Server/Daemon
├── internal/
│   ├── config/                     # Loader config.yaml
│   ├── queue/                      # Persistent SQLite queue & serial worker (Concurrency=1)
│   ├── watcher/                    # Pemantau folder input_docs/ (fsnotify) & cron scheduler
│   ├── doc/                        # Ekstraktor PDF/DOCX + text chunker
│   ├── llm/                        # Client REST API Ollama (127.0.0.1:11434) + Map-Reduce
│   ├── video/                      # Piper TTS + manifest FilmCraft + exec runner + FFmpeg fallback
│   └── web/                        # Local HTTP server (127.0.0.1:8080) & dashboard HTML
├── configs/
│   └── config.yaml                 # Konfigurasi path, batas token, jadwal, dan model lokal
├── prompts/                        # Template system prompt (summary.txt, qa.txt, affiliate_script.txt)
├── storage/                        # Data runtime lokal (di-ignore dari Git)
│   ├── robot.db                    # Database SQLite antrean, histori job, & indeks dokumen
│   ├── input_docs/                 # Folder drop file PDF / Word untuk dirangkum
│   ├── output_docs/                # Hasil ringkasan dokumen (.md)
│   ├── affiliate_assets/           # Bahan mentah video/foto produk & musik bebas hak cipta
│   │   ├── templates/              # Template proyek FilmCraft
│   │   └── jobs/                   # Folder kerja per job_id (naskah, audio, project_job.json)
│   ├── output_videos/              # Folder hasil per job_id (<job_id>.mp4 + caption.txt)
│   └── error_logs/                 # File gagal proses & log stderr untuk inspeksi
├── go.mod
└── prd_trd_local_ai_automation.md
```

### 2.6 Contoh Skema Data Job Video Affiliate (`job_payload.json`)

```json
{
  "job_id": "aff_20261127_001",
  "product_name": "Mini Tripod Bluetooth 360",
  "product_selling_points": [
    "Ringan muat di tas kecil",
    "Sudah termasuk remote shutter Bluetooth",
    "Tinggi maksimal 120 cm bahan aluminium"
  ],
  "target_duration_sec": 20,
  "assets": {
    "clips": [
      "storage/affiliate_assets/tripod/clip_unboxing.mp4",
      "storage/affiliate_assets/tripod/clip_demo.mp4"
    ],
    "bgm": "storage/affiliate_assets/bgm_upbeat.mp3"
  },
  "ai_generated": {
    "hook_text": "Masih sering minta tolong orang lain buat fotoin?",
    "voiceover_script": "Bawa tripod mini ini aja! Muat di tas kecil, udah dapet remote Bluetooth, dan tingginya bisa sampai 120 senti. Cek keranjang kuning mumpung promo!",
    "caption": "Tripod wajib buat yang suka solo traveling! Cek keranjang kuning sekarang.",
    "hashtags": ["#tripodmini", "#racuntiktok", "#affiliate"]
  },
  "output_dir": "storage/output_videos/aff_20261127_001/",
  "output_video": "storage/output_videos/aff_20261127_001/aff_20261127_001.mp4",
  "output_caption": "storage/output_videos/aff_20261127_001/caption.txt",
  "status": "READY_FOR_UPLOAD"
}
```

### 2.7 Manajemen Risiko Teknis & Mitigasi

| Risiko | Dampak | Strategi Mitigasi |
| :--- | :--- | :--- |
| **Out of Memory (OOM) / VRAM penuh saat AI & Render** | Laptop hang/freeze atau render gagal | Jalankan antrean serial di Go (`max_concurrency = 1`), kirim `keep_alive: "0"` ke Ollama sebelum memanggil FilmCraft, dan tanpa VM Docker/WSL2. |
| **Ekstraksi PDF di Go tidak rapi pada file tertentu** | Teks terpotong atau karakter berantakan | Gunakan `pdftotext` (Poppler CLI) sebagai ekstraktor utama PDF yang dipanggil via `os/exec` dengan sanitasi UTF-8; sediakan fallback library Go murni. |
| **Model AI melantur / Bahasa Indonesia kaku** | Naskah affiliate kurang menarik | Gunakan `qwen2.5:3b`, simpan *System Prompt* eksternal di `prompts/` berisi 2–3 contoh gaya bahasa *copywriting* TikTok (*few-shot prompting*) + *structured JSON output* Ollama. |
| **Perintah CLI/JSON FilmCraft berubah atau gagal** | Video gagal dirender | Isolasi integrasi FilmCraft di paket `internal/video` di balik *interface* `Renderer`; simpan `stderr` lengkap ke `storage/error_logs/`; sediakan implementasi **FFmpeg** sebagai *fallback*. |
| **Konsumsi ruang disk menumpuk dari video sementara** | Storage lokal cepat penuh | Pisahkan folder kerja sementara (`affiliate_assets/jobs/`) dari hasil akhir (`output_videos/`) agar file perantara aman dibersihkan berkala. |

### 2.8 Roadmap Implementasi Bertahap

1. **Fase 1 — Fondasi Go & Asisten Dokumen Offline (Minggu 1):**
   - Inisialisasi modul Go (`go.mod`), kerangka direktori `cmd/` & `internal/`, loader `config.yaml`, logger (`slog`), dan antrean SQLite.
   - Pastikan Ollama native Windows berjalan + unduh model `qwen2.5:3b`.
   - Implementasi `internal/doc` (ekstraksi `.pdf`/`.docx` + *chunker*), `internal/llm` (client Ollama `127.0.0.1:11434` + *Map-Reduce*), dan `internal/watcher` (`input_docs/` → `output_docs/`).
2. **Fase 2 — Generator Naskah & Perakitan Video FilmCraft (Minggu 2):**
   - Uji coba CLI / JSON Control Server FilmCraft untuk 1 video contoh (gabung klip + teks + ekspor MP4).
   - Implementasi alur affiliate di Go: input produk → Ollama buat `job_payload.json` (JSON mode) → unload VRAM (`keep_alive: "0"`) → eksekusi FilmCraft/FFmpeg fallback → simpan `.mp4` + `caption.txt` di `storage/output_videos/<job_id>/`.
3. **Fase 3 — Local Web Dashboard, Q&A Dokumen & TTS Lokal (Minggu 3):**
   - Implementasi `internal/web` (`http://127.0.0.1:8080`) untuk status antrean, upload dokumen, Q&A interaktif berbasis teks dokumen lokal, dan form pembuatan konten affiliate.
   - Integrasi **Piper TTS** lokal offline untuk suara *voiceover* otomatis.
4. **Fase 4 — Ekspansi Opsional (Minggu 4+):**
   - Fitur tambahan sesuai kebutuhan (misal integrasi opsional *TikTok Content Posting API* untuk upload draft otomatis atau modul ekstensi lainnya jika suatu saat dibutuhkan).
