package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Labildev/robot_labil/internal/config"
	"github.com/Labildev/robot_labil/internal/llm"
	"github.com/Labildev/robot_labil/internal/queue"
	"github.com/Labildev/robot_labil/internal/video"
)

// AffiliateJobRequest is stored as JSON inside queue.Job.PayloadJSON.
type AffiliateJobRequest struct {
	ProductName       string   `json:"product_name"`
	SellingPoints     []string `json:"selling_points"`
	TargetDurationSec int      `json:"target_duration_sec"`
	Clips             []string `json:"clips"`
	BGM               string   `json:"bgm,omitempty"`
	VoiceoverAudio    string   `json:"voiceover_audio,omitempty"`
	ExtraContext      string   `json:"extra_context,omitempty"`
}

// AffiliateVideoHandler generates copywriting via Ollama, unloads VRAM, renders video, and writes output bundle.
type AffiliateVideoHandler struct {
	cfg       *config.Config
	llmClient llm.Client
	generator *llm.AffiliateGenerator
	renderer  video.Renderer
}

// NewAffiliateVideoHandler constructs an AffiliateVideoHandler.
func NewAffiliateVideoHandler(cfg *config.Config, llmClient llm.Client, generator *llm.AffiliateGenerator, renderer video.Renderer) *AffiliateVideoHandler {
	return &AffiliateVideoHandler{
		cfg:       cfg,
		llmClient: llmClient,
		generator: generator,
		renderer:  renderer,
	}
}

// Handle executes the serial affiliate pipeline for a single queued job.
func (h *AffiliateVideoHandler) Handle(ctx context.Context, job *queue.Job) (string, error) {
	var req AffiliateJobRequest
	if err := json.Unmarshal([]byte(job.PayloadJSON), &req); err != nil {
		h.writeErrorLog(job.ID, err)
		return "", fmt.Errorf("parse affiliate job payload: %w", err)
	}

	script, err := h.generator.Generate(ctx, llm.AffiliateInput{
		ProductName:       req.ProductName,
		SellingPoints:     req.SellingPoints,
		TargetDurationSec: req.TargetDurationSec,
		ExtraContext:      req.ExtraContext,
	})
	if err != nil {
		h.writeErrorLog(job.ID, err)
		return "", err
	}

	// Explicitly release Ollama VRAM before FilmCraft/FFmpeg starts rendering on the 4 GB RTX 2050.
	if h.llmClient != nil {
		_ = h.llmClient.UnloadModel(ctx)
	}

	outDir := filepath.Join(h.cfg.Paths.OutputVideos, job.ID)
	workDir := filepath.Join(h.cfg.Paths.AffiliateAssets, "jobs", job.ID)
	outVideo := filepath.Join(outDir, job.ID+".mp4")

	payload := &video.JobPayload{
		JobID:                job.ID,
		ProductName:          req.ProductName,
		ProductSellingPoints: req.SellingPoints,
		TargetDurationSec:    req.TargetDurationSec,
		Assets: video.Assets{
			Clips:          req.Clips,
			BGM:            req.BGM,
			VoiceoverAudio: req.VoiceoverAudio,
		},
		AIGenerated:   script,
		OutputDir:     outDir,
		OutputVideo:   outVideo,
		OutputCaption: filepath.Join(outDir, "caption.txt"),
		Status:        video.StatusReadyForUpload,
	}

	if err := h.renderer.Render(ctx, payload, workDir); err != nil {
		h.writeErrorLog(job.ID, err)
		return "", err
	}
	if err := video.WriteOutputBundle(payload); err != nil {
		h.writeErrorLog(job.ID, err)
		return "", err
	}
	return outDir, nil
}

func (h *AffiliateVideoHandler) writeErrorLog(jobID string, cause error) {
	if strings.TrimSpace(h.cfg.Paths.ErrorLogs) == "" {
		return
	}
	_ = os.MkdirAll(h.cfg.Paths.ErrorLogs, 0o755)
	stamp := time.Now().Format("20060102_150405")
	logPath := filepath.Join(h.cfg.Paths.ErrorLogs, fmt.Sprintf("%s_%s.error.txt", stamp, jobID))
	_ = os.WriteFile(logPath, []byte(fmt.Sprintf("job_id: %s\nerror: %v\n", jobID, cause)), 0o644)
}
