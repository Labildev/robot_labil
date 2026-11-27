package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Labildev/robot_labil/internal/config"
	"github.com/Labildev/robot_labil/internal/doc"
	"github.com/Labildev/robot_labil/internal/llm"
	"github.com/Labildev/robot_labil/internal/queue"
)

type stubLLM struct{}

func (s *stubLLM) Chat(_ context.Context, _ []llm.Message, _ llm.ChatOptions) (string, error) {
	return "## Ringkasan Eksekutif\nDokumen uji berhasil dirangkum.", nil
}
func (s *stubLLM) UnloadModel(_ context.Context) error { return nil }
func (s *stubLLM) Health(_ context.Context) error      { return nil }

func TestDocSummaryHandlerSuccessAndQuarantine(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()

	cfg := config.Default()
	cfg.Paths.Database = filepath.Join(tmp, "storage", "robot.db")
	cfg.Paths.InputDocs = filepath.Join(tmp, "storage", "input_docs")
	cfg.Paths.OutputDocs = filepath.Join(tmp, "storage", "output_docs")
	cfg.Paths.ErrorLogs = filepath.Join(tmp, "storage", "error_logs")
	cfg.Paths.PromptsDir = filepath.Join(tmp, "prompts")
	if err := cfg.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories failed: %v", err)
	}

	store, err := queue.Open(cfg.Paths.Database)
	if err != nil {
		t.Fatalf("Open store failed: %v", err)
	}
	defer store.Close()

	srcFile := filepath.Join(cfg.Paths.InputDocs, "artikel.txt")
	if err := os.WriteFile(srcFile, []byte("Robot AI lokal bekerja tanpa internet."), 0o644); err != nil {
		t.Fatalf("write input doc: %v", err)
	}

	handler := NewDocSummaryHandler(
		cfg,
		store,
		doc.NewExtractor(""),
		llm.NewDocumentAssistant(&stubLLM{}, "", ""),
	)

	outPath, err := handler.Handle(ctx, &queue.Job{
		ID:         "job_1",
		JobType:    queue.JobTypeDocSummary,
		SourcePath: srcFile,
	})
	if err != nil {
		t.Fatalf("Handle() failed: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output md: %v", err)
	}
	if !strings.Contains(string(data), "Dokumen uji berhasil dirangkum.") {
		t.Fatalf("unexpected summary markdown: %s", string(data))
	}

	saved, err := store.FindDocumentByName(ctx, "artikel")
	if err != nil || saved == nil {
		t.Fatalf("expected saved document for Q&A, got saved=%+v err=%v", saved, err)
	}

	// Verify quarantine moves bad file from input_docs into error_logs.
	badFile := filepath.Join(cfg.Paths.InputDocs, "broken.png")
	if err := os.WriteFile(badFile, []byte("not-a-doc"), 0o644); err != nil {
		t.Fatalf("write bad file: %v", err)
	}
	if _, err := handler.Handle(ctx, &queue.Job{ID: "job_2", SourcePath: badFile}); err == nil {
		t.Fatalf("expected error for unsupported file")
	}
	if _, err := os.Stat(badFile); !os.IsNotExist(err) {
		t.Fatalf("expected bad file moved out of input_docs")
	}
	errEntries, _ := os.ReadDir(cfg.Paths.ErrorLogs)
	if len(errEntries) < 2 {
		t.Fatalf("expected quarantined file and error log in error_logs, got %d entries", len(errEntries))
	}
}
