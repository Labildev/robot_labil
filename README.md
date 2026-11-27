# Local AI Automation Robot (`robot_labil`)

Robot asisten AI **100% Full Local (Offline)** berbasis **Go (Golang)** + **Ollama** untuk perangkat lokal dengan alokasi memori ketat (≤ 8 GB RAM, GPU 4 GB VRAM).

Lihat spesifikasi lengkap di [prd_trd_local_ai_automation.md](prd_trd_local_ai_automation.md).

## Status Implementasi
- **Fase 1 (Selesai / Aktif):** Fondasi Go, konfigurasi YAML, persistent SQLite serial queue (`Concurrency = 1`), folder watcher (`fsnotify`), ekstraktor `.pdf` / `.docx`, Map-Reduce summarizer & Q&A via Ollama lokal (`127.0.0.1:11434`).
- **Fase 2 (Berikutnya):** Generator naskah affiliate & rendering video via FilmCraft / FFmpeg.

## Prasyarat Lokal
1. **Go 1.23+**
2. **Ollama** berjalan lokal di `http://127.0.0.1:11434` dengan model `qwen2.5:3b` (`ollama pull qwen2.5:3b`)
3. *(Opsional, disarankan)* `pdftotext` (Poppler) di `PATH` untuk ekstraksi PDF yang lebih akurat (ada fallback pure-Go bawaan jika belum terpasang).

## Cara Menjalankan
```powershell
# Cek koneksi Ollama & folder lokal
go run ./cmd/robot health

# Ringkas 1 file PDF/DOCX secara manual
go run ./cmd/robot summarize .\storage\input_docs\contoh.docx

# Tanya jawab berdasarkan dokumen yang sudah diringkas
go run ./cmd/robot qa --doc contoh --q "Apa poin utamanya?"

# Jalankan mode daemon (pantau folder storage/input_docs/ + worker serial)
go run ./cmd/robot watch

# Lihat status antrean SQLite
go run ./cmd/robot status
```

## Build Binary Native Windows
```powershell
go build -o robot.exe ./cmd/robot
.\robot.exe health
```
