package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Labildev/robot_labil/internal/queue"
)

func TestScanOnceIgnoresDuplicatesAndTempFiles(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	inputDir := filepath.Join(tmp, "input_docs")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatalf("mkdir input: %v", err)
	}

	// Supported files
	if err := os.WriteFile(filepath.Join(inputDir, "notes.txt"), []byte("Halo dunia"), 0o644); err != nil {
		t.Fatalf("write txt: %v", err)
	}
	// Ignored temp/unsupported files
	if err := os.WriteFile(filepath.Join(inputDir, "~$notes.docx"), []byte("lock"), 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "photo.jpg"), []byte("img"), 0o644); err != nil {
		t.Fatalf("write jpg: %v", err)
	}

	store, err := queue.Open(filepath.Join(tmp, "robot.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	w := New(inputDir, store, nil, 100*time.Millisecond)
	added, err := w.ScanOnce(ctx)
	if err != nil {
		t.Fatalf("ScanOnce 1 failed: %v", err)
	}
	if added != 1 {
		t.Fatalf("expected 1 enqueued file, got %d", added)
	}

	// Scanning again should not duplicate the queued file.
	added2, err := w.ScanOnce(ctx)
	if err != nil {
		t.Fatalf("ScanOnce 2 failed: %v", err)
	}
	if added2 != 0 {
		t.Fatalf("expected 0 duplicates on rescan, got %d", added2)
	}
}
