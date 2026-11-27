package pipeline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Labildev/robot_labil/internal/config"
	"github.com/Labildev/robot_labil/internal/llm"
	"github.com/Labildev/robot_labil/internal/queue"
	"github.com/Labildev/robot_labil/internal/video"
)

type spyAffiliateLLM struct {
	unloaded bool
}

func (s *spyAffiliateLLM) Chat(_ context.Context, _ []llm.Message, _ llm.ChatOptions) (string, error) {
	return `{"hook_text":"Jangan skip!","voiceover_script":"Tripod mini keren.","caption":"Cek keranjang kuning","hashtags":["#tripod"]}`, nil
}
func (s *spyAffiliateLLM) UnloadModel(_ context.Context) error {
	s.unloaded = true
	return nil
}
func (s *spyAffiliateLLM) Health(_ context.Context) error { return nil }

type spyRenderer struct {
	sawUnloadedBeforeRender bool
	spyLLM                  *spyAffiliateLLM
}

func (r *spyRenderer) Name() string { return "spy" }
func (r *spyRenderer) Render(_ context.Context, payload *video.JobPayload, _ string) error {
	r.sawUnloadedBeforeRender = r.spyLLM.unloaded
	if err := os.MkdirAll(filepath.Dir(payload.OutputVideo), 0o755); err != nil {
		return err
	}
	return os.WriteFile(payload.OutputVideo, []byte("mp4-bytes"), 0o644)
}

func TestAffiliateVideoHandlerUnloadsVRAMAndWritesBundle(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.Default()
	cfg.Paths.Database = filepath.Join(tmp, "storage", "robot.db")
	cfg.Paths.InputDocs = filepath.Join(tmp, "storage", "input_docs")
	cfg.Paths.OutputDocs = filepath.Join(tmp, "storage", "output_docs")
	cfg.Paths.AffiliateAssets = filepath.Join(tmp, "storage", "affiliate_assets")
	cfg.Paths.OutputVideos = filepath.Join(tmp, "storage", "output_videos")
	cfg.Paths.ErrorLogs = filepath.Join(tmp, "storage", "error_logs")
	cfg.Paths.PromptsDir = filepath.Join(tmp, "prompts")
	cfg.Ollama.ModelsDir = filepath.Join(tmp, "storage", "models")
	if err := cfg.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories failed: %v", err)
	}

	spyLLM := &spyAffiliateLLM{}
	spyRend := &spyRenderer{spyLLM: spyLLM}
	gen := llm.NewAffiliateGenerator(spyLLM, "")
	handler := NewAffiliateVideoHandler(cfg, spyLLM, gen, spyRend)

	reqBytes, _ := json.Marshal(AffiliateJobRequest{
		ProductName:       "Mini Tripod",
		SellingPoints:     []string{"Ringan"},
		TargetDurationSec: 20,
		Clips:             []string{"clip1.mp4"},
	})

	outDir, err := handler.Handle(context.Background(), &queue.Job{
		ID:          "aff_test_001",
		JobType:     queue.JobTypeAffiliateVideo,
		PayloadJSON: string(reqBytes),
	})
	if err != nil {
		t.Fatalf("Handle() failed: %v", err)
	}
	if !spyRend.sawUnloadedBeforeRender {
		t.Fatalf("expected Ollama VRAM unload before video rendering")
	}
	if _, err := os.Stat(filepath.Join(outDir, "caption.txt")); err != nil {
		t.Fatalf("expected caption.txt in %s: %v", outDir, err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "aff_test_001.mp4")); err != nil {
		t.Fatalf("expected mp4 in %s: %v", outDir, err)
	}
}
