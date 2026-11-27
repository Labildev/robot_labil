# Local AI Automation Robot (`robot_labil`)

Robot asisten AI **100% Full Local (Offline)** berbasis **Go (Golang)** + **Ollama** untuk perangkat lokal dengan alokasi memori ketat (≤ 8 GB RAM, GPU 4 GB VRAM). Seluruh binary portable, model LLM, database, aset, dan output tersimpan mandiri di dalam folder proyek ini.

Lihat spesifikasi lengkap di [prd_trd_local_ai_automation.md](prd_trd_local_ai_automation.md).

## Status Implementasi
- **Fase 1 (Selesai):** Fondasi Go, konfigurasi YAML, persistent SQLite serial queue (`Concurrency = 1`), folder watcher (`fsnotify`), ekstraktor `.pdf` / `.docx`, Map-Reduce summarizer & Q&A via Ollama lokal (`127.0.0.1:11434`).
- **Fase 2 (Selesai):** Generator naskah affiliate JSON (`Ollama`), active VRAM unload (`keep_alive=0`), perakitan & render video 9:16 via **FilmCraft** (CLI / JSON server) dengan fallback **FFmpeg**, serta ekspor paket lokal (`<job_id>.mp4` + `caption.txt` + `job_payload.json`).
- **Fase 3 (Selesai):** Local Web Dashboard offline (`http://127.0.0.1:8080`) untuk upload dokumen, Q&A interaktif, input job affiliate, pemantauan antrean real-time, serta integrasi **Piper TTS** offline (`tools/piper/piper.exe`).

## Prasyarat Lokal (Semua di dalam Folder Proyek)
1. **Go 1.23+**
2. **Ollama portable** di `tools/ollama/ollama.exe` dengan model di `storage/models/` (`qwen2.5:3b`)
3. **FFmpeg portable** di `tools/ffmpeg/ffmpeg.exe` (dan/atau **FilmCraft** di `PATH`) untuk rendering video affiliate
4. **Piper TTS portable** di `tools/piper/piper.exe` + model suara Indonesia di `storage/models/piper/id_ID-news_tts-medium.onnx`
5. *(Opsional)* `pdftotext` (Poppler) di `PATH` untuk ekstraksi PDF yang lebih akurat (ada fallback pure-Go bawaan)

## Cara Menjalankan Cepat (1-Klik)
Klik ganda **`start_mvp.bat`** atau jalankan di PowerShell:
```powershell
.\scripts\run_mvp.ps1
```
Skrip ini otomatis menyalakan Ollama lokal, membuka browser ke `http://127.0.0.1:8080`, dan menjalankan watcher + serial worker.

## Cara Menjalankan Manual per Perintah
```powershell
# 0. Setup otomatis Ollama portable + model qwen2.5:3b di dalam folder proyek
.\scripts\setup_ollama.ps1

# 1. Jalankan Ollama lokal (model tersimpan di storage/models/)
.\robot.exe ollama-serve
# atau:
.\scripts\start_ollama.ps1

# 2. Jalankan Web Dashboard + Watcher + Serial Worker (buka http://127.0.0.1:8080)
.\robot.exe serve

# 3. Cek koneksi Ollama & folder lokal
.\robot.exe health

# 4. Ringkas 1 file PDF/DOCX secara manual
.\robot.exe summarize .\storage\input_docs\contoh.docx

# 5. Tanya jawab berdasarkan dokumen yang sudah diringkas
.\robot.exe qa --doc contoh --q "Apa poin utamanya?"

# 6. Buat paket video affiliate lokal (.mp4 + caption.txt)
.\robot.exe affiliate --name "Mini Tripod 360" --points "Ringan,Remote Bluetooth" --clips "storage/affiliate_assets/clip1.mp4"

# 7. Lihat status antrean SQLite
.\robot.exe status
```

## Build Binary Native Windows
```powershell
go build -o robot.exe ./cmd/robot
```
