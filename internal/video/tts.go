package video

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// TTSSynthesizer converts voiceover script text into a local audio file (.wav).
type TTSSynthesizer interface {
	Available() bool
	Synthesize(ctx context.Context, text, outWavPath string) error
}

// PiperTTS runs the offline Piper TTS binary against a local ONNX voice model.
type PiperTTS struct {
	enabled   bool
	binPath   string
	modelPath string
}

// NewPiperTTS creates a PiperTTS synthesizer.
func NewPiperTTS(enabled bool, binPath, modelPath string) *PiperTTS {
	return &PiperTTS{
		enabled:   enabled,
		binPath:   strings.TrimSpace(binPath),
		modelPath: strings.TrimSpace(modelPath),
	}
}

// Available reports whether Piper TTS is enabled and both binary and model file exist locally.
func (p *PiperTTS) Available() bool {
	if !p.enabled || p.binPath == "" || p.modelPath == "" {
		return false
	}
	if _, err := p.resolveBin(); err != nil {
		return false
	}
	info, err := os.Stat(p.modelPath)
	return err == nil && !info.IsDir()
}

// Synthesize generates outWavPath from text using local Piper TTS via stdin.
func (p *PiperTTS) Synthesize(ctx context.Context, text, outWavPath string) error {
	if !p.enabled {
		return fmt.Errorf("piper tts is disabled")
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("voiceover text must not be empty")
	}
	resolvedBin, err := p.resolveBin()
	if err != nil {
		return fmt.Errorf("piper binary %q not found: %w", p.binPath, err)
	}
	if _, err := os.Stat(p.modelPath); err != nil {
		return fmt.Errorf("piper model %q not found: %w", p.modelPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(outWavPath), 0o755); err != nil {
		return fmt.Errorf("create tts output dir: %w", err)
	}

	args := p.BuildArgs(outWavPath)
	cmd := exec.CommandContext(ctx, resolvedBin, args...)
	cmd.Stdin = strings.NewReader(strings.TrimSpace(text) + "\n")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("piper tts failed: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	if _, err := os.Stat(outWavPath); err != nil {
		return fmt.Errorf("piper did not produce %q: %w", outWavPath, err)
	}
	return nil
}

// BuildArgs builds CLI flags for Piper TTS.
func (p *PiperTTS) BuildArgs(outWavPath string) []string {
	return []string{"--model", p.modelPath, "--output_file", outWavPath}
}

func (p *PiperTTS) resolveBin() (string, error) {
	if info, err := os.Stat(p.binPath); err == nil && !info.IsDir() {
		return p.binPath, nil
	}
	return exec.LookPath(p.binPath)
}
