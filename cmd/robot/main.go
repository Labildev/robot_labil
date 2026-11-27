package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Labildev/robot_labil/internal/config"
	"github.com/Labildev/robot_labil/internal/doc"
	"github.com/Labildev/robot_labil/internal/llm"
	"github.com/Labildev/robot_labil/internal/pipeline"
	"github.com/Labildev/robot_labil/internal/queue"
	"github.com/Labildev/robot_labil/internal/video"
	"github.com/Labildev/robot_labil/internal/watcher"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	configPath := "configs/config.yaml"
	subCmd := args[0]
	rest := args[1:]

	if subCmd == "--config" && len(args) >= 3 {
		configPath = args[1]
		subCmd = args[2]
		rest = args[3:]
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if err := cfg.EnsureDirectories(); err != nil {
		return err
	}

	logger := newLogger(cfg.App.LogLevel)
	store, err := queue.Open(cfg.Paths.Database)
	if err != nil {
		return err
	}
	defer store.Close()

	summaryPrompt, _ := cfg.LoadPrompt("summary.txt")
	qaPrompt, _ := cfg.LoadPrompt("qa.txt")
	affiliatePrompt, _ := cfg.LoadPrompt("affiliate_script.txt")

	ollamaClient := llm.NewOllamaClient(
		cfg.Ollama.BaseURL,
		cfg.Ollama.Model,
		cfg.Ollama.NumCtx,
		cfg.Ollama.KeepAlive,
		time.Duration(cfg.Ollama.RequestTimeoutSec)*time.Second,
	)
	extractor := doc.NewExtractor(cfg.Document.PDFToTextBin)
	assistant := llm.NewDocumentAssistant(ollamaClient, summaryPrompt, qaPrompt)
	docHandler := pipeline.NewDocSummaryHandler(cfg, store, extractor, assistant)

	affiliateGen := llm.NewAffiliateGenerator(ollamaClient, affiliatePrompt)
	filmcraftRenderer := video.NewFilmCraftRenderer(
		cfg.Video.FilmCraftBin,
		cfg.Video.FilmCraftMode,
		cfg.Video.FilmCraftServerURL,
		cfg.Video.Width,
		cfg.Video.Height,
		cfg.Video.FPS,
	)
	ffmpegRenderer := video.NewFFmpegRenderer(
		cfg.Video.FFmpegBin,
		cfg.Video.Width,
		cfg.Video.Height,
		cfg.Video.FPS,
	)
	renderer := video.NewFallbackRenderer(filmcraftRenderer, ffmpegRenderer, cfg.Video.FallbackToFFmpeg, logger)
	affiliateHandler := pipeline.NewAffiliateVideoHandler(cfg, ollamaClient, affiliateGen, renderer)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch subCmd {
	case "health":
		return runHealth(ctx, cfg, ollamaClient)
	case "ollama-serve":
		return runOllamaServe(ctx, cfg)
	case "status":
		return runStatus(ctx, store)
	case "summarize":
		return runSummarize(ctx, rest, store, logger, docHandler)
	case "qa":
		return runQA(ctx, rest, cfg, store, assistant)
	case "affiliate":
		return runAffiliate(ctx, rest, store, logger, affiliateHandler)
	case "watch":
		return runWatch(ctx, cfg, store, logger, docHandler, affiliateHandler)
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", subCmd)
	}
}

func runHealth(ctx context.Context, cfg *config.Config, client llm.Client) error {
	fmt.Printf("App       : %s (concurrency=%d)\n", cfg.App.Name, cfg.App.MaxConcurrency)
	fmt.Printf("Database  : %s\n", cfg.Paths.Database)
	fmt.Printf("ModelsDir : %s\n", cfg.Ollama.ModelsDir)
	fmt.Printf("OllamaBin : %s\n", cfg.Ollama.BinPath)
	fmt.Printf("OllamaURL : %s (model=%s, num_ctx=%d)\n", cfg.Ollama.BaseURL, cfg.Ollama.Model, cfg.Ollama.NumCtx)
	if err := client.Health(ctx); err != nil {
		return fmt.Errorf("ollama health check failed: %w", err)
	}
	fmt.Println("Status    : OK (Ollama reachable)")
	return nil
}

func runOllamaServe(ctx context.Context, cfg *config.Config) error {
	ollamaExe := cfg.Ollama.BinPath
	if strings.TrimSpace(ollamaExe) == "" {
		ollamaExe = "tools/ollama/ollama.exe"
	}
	absExe, err := filepath.Abs(ollamaExe)
	if err != nil {
		return err
	}
	if _, err := os.Stat(absExe); err != nil {
		return fmt.Errorf("local ollama binary not found at %q: %w", absExe, err)
	}

	absModels, err := filepath.Abs(cfg.Ollama.ModelsDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(absModels, 0o755); err != nil {
		return err
	}

	host := strings.TrimPrefix(strings.TrimPrefix(cfg.Ollama.BaseURL, "http://"), "https://")
	cmd := exec.CommandContext(ctx, absExe, "serve")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"OLLAMA_MODELS="+absModels,
		"OLLAMA_HOST="+host,
		"OLLAMA_MAX_LOADED_MODELS=1",
		"OLLAMA_KEEP_ALIVE="+cfg.Ollama.KeepAlive,
		"OLLAMA_NUM_PARALLEL=1",
	)
	fmt.Printf("Menjalankan Ollama lokal (%s) dengan OLLAMA_MODELS=%s\n", absExe, absModels)
	err = cmd.Run()
	if ctx.Err() != nil {
		return nil
	}
	return err
}

func runStatus(ctx context.Context, store *queue.Store) error {
	jobs, err := store.ListJobs(ctx, 20)
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		fmt.Println("Belum ada riwayat job di antrean.")
		return nil
	}
	for _, j := range jobs {
		fmt.Printf("[%s] %-16s %-11s src=%s out=%s\n",
			j.CreatedAt.Local().Format("2006-01-02 15:04"),
			j.JobType,
			j.Status,
			j.SourcePath,
			j.ResultPath,
		)
		if j.ErrorMessage != "" {
			fmt.Printf("   err: %s\n", j.ErrorMessage)
		}
	}
	return nil
}

func runSummarize(ctx context.Context, args []string, store *queue.Store, logger *slog.Logger, handler *pipeline.DocSummaryHandler) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: robot summarize <path-to-pdf-or-docx>")
	}
	absPath, err := filepath.Abs(args[0])
	if err != nil {
		return err
	}
	if _, err := os.Stat(absPath); err != nil {
		return err
	}

	id, err := store.Enqueue(ctx, queue.JobTypeDocSummary, absPath, "{}")
	if err != nil {
		return err
	}
	return runSingleWorkerJob(ctx, id, store, logger, map[string]queue.Handler{
		queue.JobTypeDocSummary: handler.Handle,
	}, "Ringkasan tersimpan")
}

func runAffiliate(ctx context.Context, args []string, store *queue.Store, logger *slog.Logger, handler *pipeline.AffiliateVideoHandler) error {
	fs := flag.NewFlagSet("affiliate", flag.ContinueOnError)
	inputJSON := fs.String("input", "", "Path ke file JSON AffiliateJobRequest")
	name := fs.String("name", "", "Nama produk")
	points := fs.String("points", "", "Keunggulan produk dipisah koma")
	duration := fs.Int("duration", 20, "Target durasi video (detik)")
	clips := fs.String("clips", "", "Daftar path klip/foto produk dipisah koma")
	bgm := fs.String("bgm", "", "Path opsional musik latar (.mp3/.wav)")
	vo := fs.String("vo", "", "Path opsional file audio voiceover (.wav/.mp3)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var req pipeline.AffiliateJobRequest
	if strings.TrimSpace(*inputJSON) != "" {
		raw, err := os.ReadFile(*inputJSON)
		if err != nil {
			return fmt.Errorf("read affiliate input %q: %w", *inputJSON, err)
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			return fmt.Errorf("parse affiliate input %q: %w", *inputJSON, err)
		}
	} else {
		req = pipeline.AffiliateJobRequest{
			ProductName:       strings.TrimSpace(*name),
			SellingPoints:     splitCSV(*points),
			TargetDurationSec: *duration,
			Clips:             splitCSV(*clips),
			BGM:               strings.TrimSpace(*bgm),
			VoiceoverAudio:    strings.TrimSpace(*vo),
		}
	}

	if strings.TrimSpace(req.ProductName) == "" || len(req.Clips) == 0 {
		return fmt.Errorf("usage: robot affiliate --name \"<produk>\" --points \"p1,p2\" --clips \"clip1.mp4,clip2.jpg\" [--bgm bgm.mp3]")
	}

	payloadBytes, err := json.Marshal(req)
	if err != nil {
		return err
	}

	id, err := store.Enqueue(ctx, queue.JobTypeAffiliateVideo, req.ProductName, string(payloadBytes))
	if err != nil {
		return err
	}
	return runSingleWorkerJob(ctx, id, store, logger, map[string]queue.Handler{
		queue.JobTypeAffiliateVideo: handler.Handle,
	}, "Paket video affiliate siap upload")
}

func runSingleWorkerJob(ctx context.Context, jobID string, store *queue.Store, logger *slog.Logger, handlers map[string]queue.Handler, label string) error {
	worker := queue.NewWorker(store, logger, time.Second, handlers)
	if _, err := worker.RunOnce(ctx); err != nil {
		return err
	}

	jobs, err := store.ListJobs(ctx, 10)
	if err != nil {
		return err
	}
	for _, j := range jobs {
		if j.ID == jobID {
			if j.Status == queue.StatusFailed {
				return fmt.Errorf("job %s failed: %s", jobID, j.ErrorMessage)
			}
			fmt.Printf("%s: %s\n", label, j.ResultPath)
			return nil
		}
	}
	return nil
}

func runQA(ctx context.Context, args []string, cfg *config.Config, store *queue.Store, assistant *llm.DocumentAssistant) error {
	fs := flag.NewFlagSet("qa", flag.ContinueOnError)
	docName := fs.String("doc", "", "Nama file dokumen (kosongkan untuk dokumen terbaru)")
	question := fs.String("q", "", "Pertanyaan terkait isi dokumen")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*question) == "" {
		return fmt.Errorf("usage: robot qa --doc <nama_file> --q \"<pertanyaan>\"")
	}

	rec, err := store.FindDocumentByName(ctx, *docName)
	if err != nil {
		return err
	}
	if rec == nil {
		return fmt.Errorf("dokumen %q belum ditemukan di database; ringkas dokumen terlebih dahulu", *docName)
	}

	answer, err := assistant.AnswerQuestion(ctx, rec.Filename, rec.ExtractedText, *question, cfg.Document.ChunkWordLimit)
	if err != nil {
		return err
	}
	fmt.Printf("Dokumen: %s\n\nJawaban:\n%s\n", rec.Filename, answer)
	return nil
}

func runWatch(
	ctx context.Context,
	cfg *config.Config,
	store *queue.Store,
	logger *slog.Logger,
	docHandler *pipeline.DocSummaryHandler,
	affiliateHandler *pipeline.AffiliateVideoHandler,
) error {
	fw := watcher.New(cfg.Paths.InputDocs, store, logger, time.Duration(cfg.Watcher.PollIntervalSec)*time.Second)
	worker := queue.NewWorker(store, logger, 2*time.Second, map[string]queue.Handler{
		queue.JobTypeDocSummary:     docHandler.Handle,
		queue.JobTypeAffiliateVideo: affiliateHandler.Handle,
	})

	logger.Info("starting local robot watcher & serial worker",
		"input_docs", cfg.Paths.InputDocs,
		"output_docs", cfg.Paths.OutputDocs,
		"output_videos", cfg.Paths.OutputVideos,
		"model", cfg.Ollama.Model,
	)

	errCh := make(chan error, 2)
	go func() { errCh <- fw.Run(ctx) }()
	go func() { errCh <- worker.Run(ctx) }()

	select {
	case <-ctx.Done():
		logger.Info("shutting down gracefully")
		return nil
	case err := <-errCh:
		if err == context.Canceled {
			return nil
		}
		return err
	}
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}

func printUsage() {
	fmt.Println(`Local AI Automation Robot (100% Offline Go)

Usage:
  robot [--config configs/config.yaml] <command> [args]

Commands:
  health                        Cek konfigurasi & koneksi Ollama lokal
  ollama-serve                  Jalankan Ollama portable di tools/ollama/ dengan model di storage/models/
  watch                         Pantau storage/input_docs/ & proses semua antrean secara serial
  summarize <file>              Ekstrak dan ringkas file .pdf/.docx/.txt ke storage/output_docs/
  qa --doc <nama> --q "<tanya>" Tanya jawab berdasarkan dokumen yang sudah diproses
  affiliate [flags]             Buat naskah AI, render video 9:16, & kemas caption.txt lokal
  status                        Lihat riwayat dan status antrean job di SQLite`)
}
