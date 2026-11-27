package pipeline

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Labildev/robot_labil/internal/config"
	"github.com/Labildev/robot_labil/internal/doc"
	"github.com/Labildev/robot_labil/internal/llm"
	"github.com/Labildev/robot_labil/internal/queue"
)

// DocSummaryHandler processes queued document summarization jobs end-to-end offline.
type DocSummaryHandler struct {
	cfg       *config.Config
	store     *queue.Store
	extractor *doc.Extractor
	assistant *llm.DocumentAssistant
}

// NewDocSummaryHandler wires configuration, SQLite store, extractor, and local LLM assistant.
func NewDocSummaryHandler(cfg *config.Config, store *queue.Store, extractor *doc.Extractor, assistant *llm.DocumentAssistant) *DocSummaryHandler {
	return &DocSummaryHandler{
		cfg:       cfg,
		store:     store,
		extractor: extractor,
		assistant: assistant,
	}
}

// Handle extracts text, summarizes via Ollama, writes a Markdown output, and saves text for Q&A.
func (h *DocSummaryHandler) Handle(ctx context.Context, job *queue.Job) (string, error) {
	text, err := h.extractor.Extract(ctx, job.SourcePath)
	if err != nil {
		h.quarantineOnError(job.SourcePath, err)
		return "", err
	}

	filename := filepath.Base(job.SourcePath)
	summary, err := h.assistant.Summarize(
		ctx,
		filename,
		text,
		h.cfg.Document.MapReduceThresholdWords,
		h.cfg.Document.ChunkWordLimit,
	)
	if err != nil {
		h.quarantineOnError(job.SourcePath, err)
		return "", err
	}

	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	outPath := filepath.Join(h.cfg.Paths.OutputDocs, stem+".md")
	content := fmt.Sprintf("# Ringkasan: %s\n\n_Diproses lokal pada %s_\n\n%s\n",
		filename,
		time.Now().Format("2006-01-02 15:04:05"),
		summary,
	)
	if err := os.WriteFile(outPath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write summary %q: %w", outPath, err)
	}

	if err := h.store.SaveDocument(ctx, queue.DocumentRecord{
		Filename:      filename,
		SourcePath:    job.SourcePath,
		SummaryPath:   outPath,
		WordCount:     doc.CountWords(text),
		ExtractedText: text,
	}); err != nil {
		return "", err
	}

	return outPath, nil
}

func (h *DocSummaryHandler) quarantineOnError(srcPath string, cause error) {
	if strings.TrimSpace(h.cfg.Paths.ErrorLogs) == "" {
		return
	}
	_ = os.MkdirAll(h.cfg.Paths.ErrorLogs, 0o755)
	stamp := time.Now().Format("20060102_150405")
	base := filepath.Base(srcPath)

	logPath := filepath.Join(h.cfg.Paths.ErrorLogs, fmt.Sprintf("%s_%s.error.txt", stamp, base))
	_ = os.WriteFile(logPath, []byte(fmt.Sprintf("source: %s\nerror: %v\n", srcPath, cause)), 0o644)

	// Move only files inside input_docs so external CLI paths remain untouched.
	absSrc, err1 := filepath.Abs(srcPath)
	absInput, err2 := filepath.Abs(h.cfg.Paths.InputDocs)
	if err1 == nil && err2 == nil && strings.HasPrefix(strings.ToLower(absSrc), strings.ToLower(absInput)) {
		dst := filepath.Join(h.cfg.Paths.ErrorLogs, fmt.Sprintf("%s_%s", stamp, base))
		if err := os.Rename(srcPath, dst); err != nil {
			_ = copyFile(srcPath, dst)
		}
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
