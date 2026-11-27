package video

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// FilmCraftManifest is written to project_job.json inside the job working directory.
type FilmCraftManifest struct {
	JobID          string   `json:"job_id"`
	OutputPath     string   `json:"output_path"`
	Width          int      `json:"width"`
	Height         int      `json:"height"`
	FPS            int      `json:"fps"`
	TargetDuration int      `json:"target_duration_sec"`
	HookText       string   `json:"hook_text"`
	Clips          []string `json:"clips"`
	BGM            string   `json:"bgm,omitempty"`
	VoiceoverAudio string   `json:"voiceover_audio,omitempty"`
}

// FilmCraftRenderer executes FilmCraft via CLI or JSON Control Server.
type FilmCraftRenderer struct {
	binPath   string
	mode      string
	serverURL string
	width     int
	height    int
	fps       int
	client    *http.Client
}

// NewFilmCraftRenderer constructs a FilmCraftRenderer.
func NewFilmCraftRenderer(binPath, mode, serverURL string, width, height, fps int) *FilmCraftRenderer {
	if strings.TrimSpace(binPath) == "" {
		binPath = "filmcraft"
	}
	if strings.TrimSpace(mode) == "" {
		mode = "cli"
	}
	return &FilmCraftRenderer{
		binPath:   binPath,
		mode:      strings.ToLower(strings.TrimSpace(mode)),
		serverURL: strings.TrimRight(serverURL, "/"),
		width:     width,
		height:    height,
		fps:       fps,
		client:    &http.Client{Timeout: 5 * time.Minute},
	}
}

// Name returns the renderer identifier.
func (r *FilmCraftRenderer) Name() string {
	return "filmcraft"
}

// BuildManifest creates a FilmCraftManifest from payload.
func (r *FilmCraftRenderer) BuildManifest(payload *JobPayload) FilmCraftManifest {
	hook := ""
	if payload.AIGenerated != nil {
		hook = payload.AIGenerated.HookText
	}
	return FilmCraftManifest{
		JobID:          payload.JobID,
		OutputPath:     payload.OutputVideo,
		Width:          r.width,
		Height:         r.height,
		FPS:            r.fps,
		TargetDuration: payload.TargetDurationSec,
		HookText:       hook,
		Clips:          payload.Assets.Clips,
		BGM:            payload.Assets.BGM,
		VoiceoverAudio: payload.Assets.VoiceoverAudio,
	}
}

// Render writes project_job.json and invokes FilmCraft CLI or JSON Control Server.
func (r *FilmCraftRenderer) Render(ctx context.Context, payload *JobPayload, workDir string) error {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("create filmcraft workdir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(payload.OutputVideo), 0o755); err != nil {
		return fmt.Errorf("create video output dir: %w", err)
	}

	manifest := r.BuildManifest(payload)
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal filmcraft manifest: %w", err)
	}
	manifestPath := filepath.Join(workDir, "project_job.json")
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		return fmt.Errorf("write %q: %w", manifestPath, err)
	}

	if r.mode == "server" {
		return r.renderViaServer(ctx, manifestBytes, payload.OutputVideo)
	}
	return r.renderViaCLI(ctx, manifestPath, payload.OutputVideo)
}

func (r *FilmCraftRenderer) renderViaCLI(ctx context.Context, manifestPath, outputPath string) error {
	resolved, err := exec.LookPath(r.binPath)
	if err != nil {
		return fmt.Errorf("filmcraft binary %q not found: %w", r.binPath, err)
	}
	cmd := exec.CommandContext(ctx, resolved, "render", "--job", manifestPath, "--output", outputPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("filmcraft cli failed: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func (r *FilmCraftRenderer) renderViaServer(ctx context.Context, manifestBytes []byte, outputPath string) error {
	if r.serverURL == "" {
		return fmt.Errorf("filmcraft server_url is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.serverURL+"/render", bytes.NewReader(manifestBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("filmcraft server request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("filmcraft server status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if _, err := os.Stat(outputPath); err != nil {
		return fmt.Errorf("filmcraft server did not produce %q: %w", outputPath, err)
	}
	return nil
}
