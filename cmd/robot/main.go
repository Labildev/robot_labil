package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch subCmd {
	case "health":
		return runHealth(ctx, cfg, ollamaClient)
	case "status":
		return runStatus(ctx, store)
	case "summarize":
		return runSummarize(ctx, rest, store, logger, docHandler)
	case "qa":
		return runQA(ctx, rest, cfg, store, assistant)
	case "watch":
		return runWatch(ctx, cfg, store, logger, docHandler)
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
	fmt.Printf("Ollama    : %s (model=%s, num_ctx=%d)\n", cfg.Ollama.BaseURL, cfg.Ollama.Model, cfg.Ollama.NumCtx)
	if err := client.Health(ctx); err != nil {
		return fmt.Errorf("ollama health check failed: %w", err)
	}
	fmt.Println("Status    : OK (Ollama reachable)")
	return nil
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
		fmt.Printf("[%s] %-12s %-11s src=%s out=%s\n",
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
	worker := queue.NewWorker(store, logger, time.Second, map[string]queue.Handler{
		queue.JobTypeDocSummary: handler.Handle,
	})
	if _, err := worker.RunOnce(ctx); err != nil {
		return err
	}

	jobs, err := store.ListJobs(ctx, 10)
	if err != nil {
		return err
	}
	for _, j := range jobs {
		if j.ID == id {
			if j.Status == queue.StatusFailed {
				return fmt.Errorf("job %s failed: %s", id, j.ErrorMessage)
			}
			fmt.Printf("Ringkasan tersimpan: %s\n", j.ResultPath)
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

func runWatch(ctx context.Context, cfg *config.Config, store *queue.Store, logger *slog.Logger, handler *pipeline.DocSummaryHandler) error {
	fw := watcher.New(cfg.Paths.InputDocs, store, logger, time.Duration(cfg.Watcher.PollIntervalSec)*time.Second)
	worker := queue.NewWorker(store, logger, 2*time.Second, map[string]queue.Handler{
		queue.JobTypeDocSummary: handler.Handle,
	})

	logger.Info("starting local robot watcher & serial worker",
		"input_docs", cfg.Paths.InputDocs,
		"output_docs", cfg.Paths.OutputDocs,
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
  watch                         Pantau storage/input_docs/ & proses antrean secara serial
  summarize <file>              Ekstrak dan ringkas file .pdf/.docx/.txt ke storage/output_docs/
  qa --doc <nama> --q "<tanya>" Tanya jawab berdasarkan dokumen yang sudah diproses
  status                        Lihat riwayat dan status antrean job di SQLite`)
}
