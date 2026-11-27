package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAndValidateDefaults(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")
	yamlContent := `
app:
  name: "test_robot"
  max_concurrency: 4
paths:
  database: "` + filepath.ToSlash(filepath.Join(tmp, "storage", "robot.db")) + `"
  input_docs: "` + filepath.ToSlash(filepath.Join(tmp, "storage", "input_docs")) + `"
  output_docs: "` + filepath.ToSlash(filepath.Join(tmp, "storage", "output_docs")) + `"
  affiliate_assets: "` + filepath.ToSlash(filepath.Join(tmp, "storage", "affiliate_assets")) + `"
  output_videos: "` + filepath.ToSlash(filepath.Join(tmp, "storage", "output_videos")) + `"
  error_logs: "` + filepath.ToSlash(filepath.Join(tmp, "storage", "error_logs")) + `"
  prompts_dir: "` + filepath.ToSlash(filepath.Join(tmp, "prompts")) + `"
ollama:
  base_url: "http://127.0.0.1:11434"
  model: "qwen2.5:3b"
`
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.App.MaxConcurrency != 1 {
		t.Fatalf("expected MaxConcurrency clamped to 1, got %d", cfg.App.MaxConcurrency)
	}
	if cfg.Ollama.NumCtx != 4096 {
		t.Fatalf("expected default NumCtx 4096, got %d", cfg.Ollama.NumCtx)
	}
	if err := cfg.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories() failed: %v", err)
	}
	if _, err := os.Stat(cfg.Paths.InputDocs); err != nil {
		t.Fatalf("expected input_docs directory to exist: %v", err)
	}
}

func TestLoadPrompt(t *testing.T) {
	tmp := t.TempDir()
	promptsDir := filepath.Join(tmp, "prompts")
	if err := os.MkdirAll(promptsDir, 0o755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(promptsDir, "summary.txt"), []byte("  Ringkas teks ini. \n"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	cfg := Default()
	cfg.Paths.PromptsDir = promptsDir
	got, err := cfg.LoadPrompt("summary.txt")
	if err != nil {
		t.Fatalf("LoadPrompt() error: %v", err)
	}
	if got != "Ringkas teks ini." {
		t.Fatalf("unexpected prompt content: %q", got)
	}
}
