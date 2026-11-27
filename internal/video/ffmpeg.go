package video

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// FFmpegRenderer assembles vertical 9:16 affiliate videos directly via FFmpeg CLI.
type FFmpegRenderer struct {
	binPath string
	width   int
	height  int
	fps     int
}

// NewFFmpegRenderer creates an FFmpegRenderer with 9:16 portrait defaults.
func NewFFmpegRenderer(binPath string, width, height, fps int) *FFmpegRenderer {
	if strings.TrimSpace(binPath) == "" {
		binPath = "ffmpeg"
	}
	if width <= 0 {
		width = 1080
	}
	if height <= 0 {
		height = 1920
	}
	if fps <= 0 {
		fps = 30
	}
	return &FFmpegRenderer{
		binPath: binPath,
		width:   width,
		height:  height,
		fps:     fps,
	}
}

// Name returns the renderer identifier.
func (r *FFmpegRenderer) Name() string {
	return "ffmpeg"
}

// Render builds and runs FFmpeg to export payload.OutputVideo.
func (r *FFmpegRenderer) Render(ctx context.Context, payload *JobPayload, _ string) error {
	resolved, err := r.resolveBin()
	if err != nil {
		return fmt.Errorf("ffmpeg binary %q not found: %w", r.binPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(payload.OutputVideo), 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	args, err := r.BuildArgs(payload)
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, resolved, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg failed: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func (r *FFmpegRenderer) resolveBin() (string, error) {
	if info, err := os.Stat(r.binPath); err == nil && !info.IsDir() {
		return r.binPath, nil
	}
	if p, err := exec.LookPath(r.binPath); err == nil {
		return p, nil
	}
	return exec.LookPath("ffmpeg")
}

// BuildArgs constructs deterministic FFmpeg CLI arguments for clips/images, hook overlay, and BGM.
func (r *FFmpegRenderer) BuildArgs(payload *JobPayload) ([]string, error) {
	if len(payload.Assets.Clips) == 0 {
		return nil, fmt.Errorf("at least one video clip or product image is required")
	}
	for _, clip := range payload.Assets.Clips {
		if _, err := os.Stat(clip); err != nil {
			return nil, fmt.Errorf("missing media asset %q: %w", clip, err)
		}
	}

	duration := payload.TargetDurationSec
	if duration <= 0 {
		duration = 20
	}
	perClipSec := duration / len(payload.Assets.Clips)
	if perClipSec < 2 {
		perClipSec = 2
	}

	args := []string{"-y"}
	for _, clip := range payload.Assets.Clips {
		if isImageFile(clip) {
			args = append(args, "-loop", "1", "-t", fmt.Sprintf("%d", perClipSec), "-i", clip)
		} else {
			args = append(args, "-i", clip)
		}
	}

	audioIdx := -1
	audioPath := strings.TrimSpace(payload.Assets.VoiceoverAudio)
	if audioPath == "" {
		audioPath = strings.TrimSpace(payload.Assets.BGM)
	}
	if audioPath != "" {
		if _, err := os.Stat(audioPath); err != nil {
			return nil, fmt.Errorf("missing audio asset %q: %w", audioPath, err)
		}
		audioIdx = len(payload.Assets.Clips)
		args = append(args, "-i", audioPath)
	}

	var filter strings.Builder
	var concatInputs strings.Builder
	for i := range payload.Assets.Clips {
		fmt.Fprintf(&filter,
			"[%d:v]scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black,setsar=1,fps=%d[v%d];",
			i, r.width, r.height, r.width, r.height, r.fps, i,
		)
		fmt.Fprintf(&concatInputs, "[v%d]", i)
	}
	fmt.Fprintf(&filter, "%sconcat=n=%d:v=1:a=0[vcat]", concatInputs.String(), len(payload.Assets.Clips))

	videoMap := "[vcat]"
	if payload.AIGenerated != nil && strings.TrimSpace(payload.AIGenerated.HookText) != "" {
		wrapped := wrapText(payload.AIGenerated.HookText, 24)
		escaped := escapeDrawtext(wrapped)
		fontOpt := ""
		if fontPath := detectSystemFont(); fontPath != "" {
			fontOpt = fmt.Sprintf("fontfile='%s':", escapeFilterPath(fontPath))
		}
		fmt.Fprintf(&filter,
			";[vcat]drawtext=%stext='%s':fontcolor=white:fontsize=54:line_spacing=12:box=1:boxcolor=black@0.65:boxborderw=20:x=(w-text_w)/2:y=h*0.12[vout]",
			fontOpt, escaped,
		)
		videoMap = "[vout]"
	}

	args = append(args, "-filter_complex", filter.String(), "-map", videoMap)
	if audioIdx >= 0 {
		args = append(args, "-map", fmt.Sprintf("%d:a", audioIdx), "-c:a", "aac", "-shortest")
	} else {
		args = append(args, "-an")
	}

	args = append(args,
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-pix_fmt", "yuv420p",
		"-movflags", "+faststart",
		payload.OutputVideo,
	)
	return args, nil
}

func isImageFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".bmp":
		return true
	default:
		return false
	}
}

func detectSystemFont() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	candidates := []string{
		`C:\Windows\Fonts\arialbd.ttf`,
		`C:\Windows\Fonts\arial.ttf`,
		`C:\Windows\Fonts\seguisb.ttf`,
		`C:\Windows\Fonts\segoeui.ttf`,
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c
		}
	}
	return ""
}

func escapeFilterPath(path string) string {
	slashed := filepath.ToSlash(path)
	return strings.ReplaceAll(slashed, ":", `\:`)
}

func wrapText(text string, maxCols int) string {
	words := strings.Fields(strings.TrimSpace(text))
	if len(words) == 0 || maxCols <= 0 {
		return ""
	}
	var lines []string
	var cur strings.Builder
	for _, w := range words {
		if cur.Len() > 0 && cur.Len()+1+len(w) > maxCols {
			lines = append(lines, cur.String())
			cur.Reset()
		}
		if cur.Len() > 0 {
			cur.WriteByte(' ')
		}
		cur.WriteString(w)
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	return strings.Join(lines, "\n")
}

func escapeDrawtext(text string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`:`, `\:`,
		`'`, `\'`,
		`%`, `\%`,
		"\r", "",
	)
	return r.Replace(strings.TrimSpace(text))
}
