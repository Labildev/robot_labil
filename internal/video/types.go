package video

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Labildev/robot_labil/internal/llm"
)

// StatusReadyForUpload indicates the local video and caption bundle are ready for manual upload.
const StatusReadyForUpload = "READY_FOR_UPLOAD"

// Assets describes raw media inputs for an affiliate video job.
type Assets struct {
	Clips          []string `json:"clips"`
	BGM            string   `json:"bgm,omitempty"`
	VoiceoverAudio string   `json:"voiceover_audio,omitempty"`
}

// JobPayload mirrors the job_payload.json schema in the TRD for local video assembly.
type JobPayload struct {
	JobID                string               `json:"job_id"`
	ProductName          string               `json:"product_name"`
	ProductSellingPoints []string             `json:"product_selling_points"`
	TargetDurationSec    int                  `json:"target_duration_sec"`
	Assets               Assets               `json:"assets"`
	AIGenerated          *llm.AffiliateScript `json:"ai_generated"`
	OutputDir            string               `json:"output_dir"`
	OutputVideo          string               `json:"output_video"`
	OutputCaption        string               `json:"output_caption"`
	Status               string               `json:"status"`
}

// Renderer compiles an affiliate JobPayload into an exported .mp4 video file.
type Renderer interface {
	Name() string
	Render(ctx context.Context, payload *JobPayload, workDir string) error
}

// FallbackRenderer tries the primary renderer (FilmCraft) and falls back to FFmpeg when enabled.
type FallbackRenderer struct {
	primary         Renderer
	fallback        Renderer
	fallbackEnabled bool
	logger          *slog.Logger
}

// NewFallbackRenderer creates a Renderer that tries primary first and optionally falls back to secondary.
func NewFallbackRenderer(primary, fallback Renderer, fallbackEnabled bool, logger *slog.Logger) *FallbackRenderer {
	if logger == nil {
		logger = slog.Default()
	}
	return &FallbackRenderer{
		primary:         primary,
		fallback:        fallback,
		fallbackEnabled: fallbackEnabled,
		logger:          logger,
	}
}

// Name returns a composite renderer name.
func (r *FallbackRenderer) Name() string {
	return "filmcraft-with-ffmpeg-fallback"
}

// Render executes primary rendering and falls back to FFmpeg on failure if enabled.
func (r *FallbackRenderer) Render(ctx context.Context, payload *JobPayload, workDir string) error {
	var primaryErr error
	if r.primary != nil {
		primaryErr = r.primary.Render(ctx, payload, workDir)
		if primaryErr == nil {
			return nil
		}
		if !r.fallbackEnabled || r.fallback == nil {
			return primaryErr
		}
		r.logger.Warn("primary renderer failed, falling back to ffmpeg",
			"primary", r.primary.Name(),
			"error", primaryErr,
		)
	}

	if r.fallback == nil {
		return fmt.Errorf("no video renderer available")
	}
	if err := r.fallback.Render(ctx, payload, workDir); err != nil {
		if primaryErr != nil {
			return fmt.Errorf("primary (%s): %v; fallback (%s): %w", r.primary.Name(), primaryErr, r.fallback.Name(), err)
		}
		return err
	}
	return nil
}
