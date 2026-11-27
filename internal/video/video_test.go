package video

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Labildev/robot_labil/internal/llm"
)

func TestFFmpegBuildArgsAndCaptionBundle(t *testing.T) {
	tmp := t.TempDir()
	imgPath := filepath.Join(tmp, "product.jpg")
	bgmPath := filepath.Join(tmp, "bgm.mp3")
	if err := os.WriteFile(imgPath, []byte("fake-jpg"), 0o644); err != nil {
		t.Fatalf("write img: %v", err)
	}
	if err := os.WriteFile(bgmPath, []byte("fake-mp3"), 0o644); err != nil {
		t.Fatalf("write bgm: %v", err)
	}

	outDir := filepath.Join(tmp, "output_videos", "aff_001")
	payload := &JobPayload{
		JobID:             "aff_001",
		ProductName:       "Mini Tripod 360",
		TargetDurationSec: 15,
		Assets: Assets{
			Clips: []string{imgPath},
			BGM:   bgmPath,
		},
		AIGenerated: &llm.AffiliateScript{
			HookText:        "Foto sendiri: ribet?",
			VoiceoverScript: "Pakai tripod ini!",
			Caption:         "Wajib punya buat solo traveling",
			Hashtags:        []string{"#tripodmini", "#racuntiktok"},
		},
		OutputDir:     outDir,
		OutputVideo:   filepath.Join(outDir, "aff_001.mp4"),
		OutputCaption: filepath.Join(outDir, "caption.txt"),
	}

	r := NewFFmpegRenderer("ffmpeg", 1080, 1920, 30)
	args, err := r.BuildArgs(payload)
	if err != nil {
		t.Fatalf("BuildArgs() failed: %v", err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-loop 1") || !strings.Contains(joined, "drawtext=text='Foto sendiri\\: ribet?'") {
		t.Fatalf("unexpected ffmpeg args: %s", joined)
	}
	if !strings.Contains(joined, "-map 1:a") {
		t.Fatalf("expected audio mapping in ffmpeg args: %s", joined)
	}

	if err := WriteOutputBundle(payload); err != nil {
		t.Fatalf("WriteOutputBundle() failed: %v", err)
	}
	capBytes, err := os.ReadFile(payload.OutputCaption)
	if err != nil {
		t.Fatalf("read caption.txt: %v", err)
	}
	if !strings.Contains(string(capBytes), "Mini Tripod 360") || !strings.Contains(string(capBytes), "#tripodmini") {
		t.Fatalf("unexpected caption.txt content: %s", string(capBytes))
	}
	if _, err := os.Stat(filepath.Join(outDir, "job_payload.json")); err != nil {
		t.Fatalf("expected job_payload.json: %v", err)
	}
}

type fakeRenderer struct {
	name string
	err  error
	ran  bool
}

func (f *fakeRenderer) Name() string { return f.name }
func (f *fakeRenderer) Render(_ context.Context, _ *JobPayload, _ string) error {
	f.ran = true
	return f.err
}

func TestFallbackRendererAndManifest(t *testing.T) {
	tmp := t.TempDir()
	primary := &fakeRenderer{name: "filmcraft", err: errors.New("missing binary")}
	fallback := &fakeRenderer{name: "ffmpeg"}

	fb := NewFallbackRenderer(primary, fallback, true, nil)
	if err := fb.Render(context.Background(), &JobPayload{}, tmp); err != nil {
		t.Fatalf("FallbackRenderer failed: %v", err)
	}
	if !primary.ran || !fallback.ran {
		t.Fatalf("expected both primary and fallback to run, got primary=%v fallback=%v", primary.ran, fallback.ran)
	}

	fc := NewFilmCraftRenderer("nonexistent-filmcraft", "cli", "", 1080, 1920, 30)
	payload := &JobPayload{
		JobID:       "aff_manifest",
		OutputVideo: filepath.Join(tmp, "out", "aff_manifest.mp4"),
		Assets:      Assets{Clips: []string{"clip1.mp4"}},
	}
	// Expect binary-not-found error, but project_job.json must still be written first.
	if err := fc.Render(context.Background(), payload, tmp); err == nil {
		t.Fatalf("expected error for missing filmcraft binary")
	}
	if _, err := os.Stat(filepath.Join(tmp, "project_job.json")); err != nil {
		t.Fatalf("expected project_job.json written: %v", err)
	}
}
