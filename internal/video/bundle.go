package video

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteOutputBundle saves caption.txt and job_payload.json alongside the rendered video.
func WriteOutputBundle(payload *JobPayload) error {
	if payload == nil {
		return fmt.Errorf("payload must not be nil")
	}
	if err := os.MkdirAll(payload.OutputDir, 0o755); err != nil {
		return fmt.Errorf("create output bundle dir: %w", err)
	}

	payload.Status = StatusReadyForUpload
	if payload.OutputCaption == "" {
		payload.OutputCaption = filepath.Join(payload.OutputDir, "caption.txt")
	}

	var capBuf strings.Builder
	fmt.Fprintf(&capBuf, "=== PAKET KONTEN AFFILIATE SIAP UPLOAD ===\n")
	fmt.Fprintf(&capBuf, "Job ID   : %s\n", payload.JobID)
	fmt.Fprintf(&capBuf, "Produk   : %s\n", payload.ProductName)
	fmt.Fprintf(&capBuf, "Video    : %s\n\n", payload.OutputVideo)
	if payload.AIGenerated != nil {
		fmt.Fprintf(&capBuf, "[HOOK OVERLAY]\n%s\n\n", payload.AIGenerated.HookText)
		fmt.Fprintf(&capBuf, "[CAPTION SIAP COPY-PASTE]\n%s\n\n", payload.AIGenerated.Caption)
		fmt.Fprintf(&capBuf, "[HASHTAGS]\n%s\n\n", strings.Join(payload.AIGenerated.Hashtags, " "))
		fmt.Fprintf(&capBuf, "[NASKAH VOICEOVER]\n%s\n\n", payload.AIGenerated.VoiceoverScript)
	}
	fmt.Fprintf(&capBuf, "[CHECKLIST KERANJANG KUNING]\n")
	fmt.Fprintf(&capBuf, "1. Upload %s ke TikTok (atau Draft).\n", filepath.Base(payload.OutputVideo))
	fmt.Fprintf(&capBuf, "2. Klik Tambah Tautan -> Produk -> pilih %s.\n", payload.ProductName)
	fmt.Fprintf(&capBuf, "3. Tempel caption & hashtag di atas, lalu Posting.\n")

	if err := os.WriteFile(payload.OutputCaption, []byte(capBuf.String()), 0o644); err != nil {
		return fmt.Errorf("write caption.txt: %w", err)
	}

	jsonBytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal job_payload.json: %w", err)
	}
	jsonPath := filepath.Join(payload.OutputDir, "job_payload.json")
	if err := os.WriteFile(jsonPath, jsonBytes, 0o644); err != nil {
		return fmt.Errorf("write job_payload.json: %w", err)
	}
	return nil
}
