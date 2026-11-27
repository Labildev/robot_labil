package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds the full application configuration loaded from YAML.
type Config struct {
	App      AppConfig      `yaml:"app"`
	Paths    PathsConfig    `yaml:"paths"`
	Ollama   OllamaConfig   `yaml:"ollama"`
	Document DocumentConfig `yaml:"document"`
	Video    VideoConfig    `yaml:"video"`
	TTS      TTSConfig      `yaml:"tts"`
	Watcher  WatcherConfig  `yaml:"watcher"`
	Web      WebConfig      `yaml:"web"`
}

// AppConfig defines core runtime behavior and concurrency limits.
type AppConfig struct {
	Name           string `yaml:"name"`
	LogLevel       string `yaml:"log_level"`
	MaxConcurrency int    `yaml:"max_concurrency"`
}

// PathsConfig defines local directories and SQLite database path.
type PathsConfig struct {
	Database        string `yaml:"database"`
	InputDocs       string `yaml:"input_docs"`
	OutputDocs      string `yaml:"output_docs"`
	AffiliateAssets string `yaml:"affiliate_assets"`
	OutputVideos    string `yaml:"output_videos"`
	ErrorLogs       string `yaml:"error_logs"`
	PromptsDir      string `yaml:"prompts_dir"`
}

// OllamaConfig defines local Ollama binary, model store, and inference settings.
type OllamaConfig struct {
	BinPath           string `yaml:"bin_path"`
	ModelsDir         string `yaml:"models_dir"`
	BaseURL           string `yaml:"base_url"`
	Model             string `yaml:"model"`
	NumCtx            int    `yaml:"num_ctx"`
	KeepAlive         string `yaml:"keep_alive"`
	RequestTimeoutSec int    `yaml:"request_timeout_sec"`
}

// DocumentConfig defines document extraction and chunking settings.
type DocumentConfig struct {
	ChunkWordLimit          int    `yaml:"chunk_word_limit"`
	MapReduceThresholdWords int    `yaml:"map_reduce_threshold_words"`
	PDFToTextBin            string `yaml:"pdftotext_bin"`
}

// VideoConfig defines FilmCraft and FFmpeg rendering settings.
type VideoConfig struct {
	FilmCraftBin       string `yaml:"filmcraft_bin"`
	FilmCraftMode      string `yaml:"filmcraft_mode"`
	FilmCraftServerURL string `yaml:"filmcraft_server_url"`
	FFmpegBin          string `yaml:"ffmpeg_bin"`
	Width              int    `yaml:"width"`
	Height             int    `yaml:"height"`
	FPS                int    `yaml:"fps"`
	FallbackToFFmpeg   bool   `yaml:"fallback_to_ffmpeg"`
}

// TTSConfig defines local offline Piper TTS settings.
type TTSConfig struct {
	Enabled   bool   `yaml:"enabled"`
	PiperBin  string `yaml:"piper_bin"`
	ModelPath string `yaml:"model_path"`
}

// WatcherConfig controls input folder monitoring.
type WatcherConfig struct {
	Enabled         bool `yaml:"enabled"`
	PollIntervalSec int  `yaml:"poll_interval_sec"`
}

// WebConfig defines local HTTP server binding.
type WebConfig struct {
	ListenAddr string `yaml:"listen_addr"`
}

// Default returns a Config populated with safe local defaults.
func Default() *Config {
	return &Config{
		App: AppConfig{
			Name:           "robot_labil",
			LogLevel:       "info",
			MaxConcurrency: 1,
		},
		Paths: PathsConfig{
			Database:        "storage/robot.db",
			InputDocs:       "storage/input_docs",
			OutputDocs:      "storage/output_docs",
			AffiliateAssets: "storage/affiliate_assets",
			OutputVideos:    "storage/output_videos",
			ErrorLogs:       "storage/error_logs",
			PromptsDir:      "prompts",
		},
		Ollama: OllamaConfig{
			BinPath:           "tools/ollama/ollama.exe",
			ModelsDir:         "storage/models",
			BaseURL:           "http://127.0.0.1:11434",
			Model:             "qwen2.5:3b",
			NumCtx:            4096,
			KeepAlive:         "2m",
			RequestTimeoutSec: 180,
		},
		Document: DocumentConfig{
			ChunkWordLimit:          2000,
			MapReduceThresholdWords: 3000,
			PDFToTextBin:            "pdftotext",
		},
		Video: VideoConfig{
			FilmCraftBin:       "filmcraft",
			FilmCraftMode:      "cli",
			FilmCraftServerURL: "http://127.0.0.1:4123",
			FFmpegBin:          "tools/ffmpeg/ffmpeg.exe",
			Width:              1080,
			Height:             1920,
			FPS:                30,
			FallbackToFFmpeg:   true,
		},
		TTS: TTSConfig{
			Enabled:   true,
			PiperBin:  "tools/piper/piper.exe",
			ModelPath: "storage/models/piper/id_ID-news_tts-medium.onnx",
		},
		Watcher: WatcherConfig{
			Enabled:         true,
			PollIntervalSec: 5,
		},
		Web: WebConfig{
			ListenAddr: "127.0.0.1:8080",
		},
	}
}

// Load reads YAML configuration from path and applies defaults for omitted fields.
func Load(path string) (*Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate enforces hardware-safety and required runtime invariants.
func (c *Config) Validate() error {
	if c.App.MaxConcurrency != 1 {
		// Hardware budget (8 GB RAM / 4 GB VRAM) requires strict serial execution.
		c.App.MaxConcurrency = 1
	}
	if strings.TrimSpace(c.Ollama.BaseURL) == "" {
		return fmt.Errorf("ollama.base_url must not be empty")
	}
	if strings.TrimSpace(c.Ollama.Model) == "" {
		return fmt.Errorf("ollama.model must not be empty")
	}
	if c.Ollama.NumCtx <= 0 {
		c.Ollama.NumCtx = 4096
	}
	if c.Ollama.RequestTimeoutSec <= 0 {
		c.Ollama.RequestTimeoutSec = 180
	}
	if c.Document.ChunkWordLimit <= 0 {
		c.Document.ChunkWordLimit = 2000
	}
	if c.Document.MapReduceThresholdWords <= 0 {
		c.Document.MapReduceThresholdWords = 3000
	}
	if c.Video.Width <= 0 {
		c.Video.Width = 1080
	}
	if c.Video.Height <= 0 {
		c.Video.Height = 1920
	}
	if c.Video.FPS <= 0 {
		c.Video.FPS = 30
	}
	if strings.TrimSpace(c.Video.FilmCraftMode) == "" {
		c.Video.FilmCraftMode = "cli"
	}
	if c.Watcher.PollIntervalSec <= 0 {
		c.Watcher.PollIntervalSec = 5
	}
	return nil
}

// EnsureDirectories creates all configured local runtime folders if missing.
func (c *Config) EnsureDirectories() error {
	dirs := []string{
		filepath.Dir(c.Paths.Database),
		c.Paths.InputDocs,
		c.Paths.OutputDocs,
		c.Paths.AffiliateAssets,
		filepath.Join(c.Paths.AffiliateAssets, "jobs"),
		filepath.Join(c.Paths.AffiliateAssets, "templates"),
		c.Paths.OutputVideos,
		c.Paths.ErrorLogs,
		c.Paths.PromptsDir,
		c.Ollama.ModelsDir,
	}
	for _, dir := range dirs {
		if strings.TrimSpace(dir) == "" || dir == "." {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %q: %w", dir, err)
		}
	}
	return nil
}

// LoadPrompt reads a prompt template file from PromptsDir.
func (c *Config) LoadPrompt(filename string) (string, error) {
	path := filepath.Join(c.Paths.PromptsDir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read prompt %q: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}
