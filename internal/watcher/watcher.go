package watcher

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Labildev/robot_labil/internal/doc"
	"github.com/Labildev/robot_labil/internal/queue"
	"github.com/fsnotify/fsnotify"
)

// FolderWatcher monitors inputDir for supported documents and enqueues them serially.
type FolderWatcher struct {
	inputDir     string
	store        *queue.Store
	logger       *slog.Logger
	pollInterval time.Duration
}

// New creates a FolderWatcher for inputDir.
func New(inputDir string, store *queue.Store, logger *slog.Logger, pollInterval time.Duration) *FolderWatcher {
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &FolderWatcher{
		inputDir:     inputDir,
		store:        store,
		logger:       logger,
		pollInterval: pollInterval,
	}
}

// ScanOnce inspects inputDir and enqueues any unseen supported document files.
func (w *FolderWatcher) ScanOnce(ctx context.Context) (int, error) {
	entries, err := os.ReadDir(w.inputDir)
	if err != nil {
		return 0, err
	}
	enqueued := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(w.inputDir, entry.Name())
		added, err := w.maybeEnqueue(ctx, path)
		if err != nil {
			w.logger.Error("watcher scan error", "file", path, "error", err)
			continue
		}
		if added {
			enqueued++
		}
	}
	return enqueued, nil
}

// Run performs an initial scan and watches for new files until ctx is canceled.
func (w *FolderWatcher) Run(ctx context.Context) error {
	if _, err := w.ScanOnce(ctx); err != nil {
		return err
	}

	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer fsWatcher.Close()

	if err := fsWatcher.Add(w.inputDir); err != nil {
		return err
	}

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-fsWatcher.Events:
			if !ok {
				return nil
			}
			if event.Has(fsnotify.Create) || event.Has(fsnotify.Write) || event.Has(fsnotify.Rename) {
				// Brief settle delay so Windows file copies finish before extraction.
				time.Sleep(300 * time.Millisecond)
				if _, err := w.maybeEnqueue(ctx, event.Name); err != nil {
					w.logger.Error("watcher event error", "file", event.Name, "error", err)
				}
			}
		case watchErr, ok := <-fsWatcher.Errors:
			if !ok {
				return nil
			}
			w.logger.Error("fsnotify error", "error", watchErr)
		case <-ticker.C:
			if _, err := w.ScanOnce(ctx); err != nil {
				w.logger.Error("periodic scan error", "error", err)
			}
		}
	}
}

func (w *FolderWatcher) maybeEnqueue(ctx context.Context, path string) (bool, error) {
	base := filepath.Base(path)
	if strings.HasPrefix(base, ".") || strings.HasPrefix(base, "~$") {
		return false, nil
	}
	if !doc.IsSupportedExt(path) {
		return false, nil
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return false, nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	exists, err := w.store.HasActiveOrCompletedSource(ctx, absPath)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}

	id, err := w.store.Enqueue(ctx, queue.JobTypeDocSummary, absPath, "{}")
	if err != nil {
		return false, err
	}
	w.logger.Info("enqueued document", "job_id", id, "file", absPath)
	return true, nil
}
