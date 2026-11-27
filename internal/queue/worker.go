package queue

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Handler processes a claimed job and returns the resulting artifact path on success.
type Handler func(ctx context.Context, job *Job) (resultPath string, err error)

// Worker executes queued jobs strictly one at a time (Concurrency = 1).
type Worker struct {
	store        *Store
	logger       *slog.Logger
	pollInterval time.Duration
	handlers     map[string]Handler
}

// NewWorker creates a serial job worker backed by Store.
func NewWorker(store *Store, logger *slog.Logger, pollInterval time.Duration, handlers map[string]Handler) *Worker {
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{
		store:        store,
		logger:       logger,
		pollInterval: pollInterval,
		handlers:     handlers,
	}
}

// RunOnce processes at most one queued job. Returns (true, nil) if a job was processed.
func (w *Worker) RunOnce(ctx context.Context) (bool, error) {
	job, err := w.store.ClaimNext(ctx)
	if err != nil {
		return false, err
	}
	if job == nil {
		return false, nil
	}

	w.logger.Info("job started", "job_id", job.ID, "type", job.JobType, "source", job.SourcePath)

	handler, ok := w.handlers[job.JobType]
	if !ok {
		errMsg := fmt.Sprintf("no handler registered for job type %q", job.JobType)
		_ = w.store.MarkFailed(ctx, job.ID, errMsg)
		w.logger.Error("job failed", "job_id", job.ID, "error", errMsg)
		return true, nil
	}

	resultPath, execErr := w.safeExecute(ctx, handler, job)
	if execErr != nil {
		if markErr := w.store.MarkFailed(ctx, job.ID, execErr.Error()); markErr != nil {
			w.logger.Error("failed to mark job failed", "job_id", job.ID, "error", markErr)
		}
		w.logger.Error("job failed", "job_id", job.ID, "error", execErr)
		return true, nil
	}

	if err := w.store.MarkDone(ctx, job.ID, resultPath); err != nil {
		return true, err
	}
	w.logger.Info("job completed", "job_id", job.ID, "result", resultPath)
	return true, nil
}

func (w *Worker) safeExecute(ctx context.Context, handler Handler, job *Job) (res string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("worker panic: %v", r)
		}
	}()
	return handler(ctx, job)
}

// Run polls the SQLite queue serially until ctx is canceled.
func (w *Worker) Run(ctx context.Context) error {
	if recovered, err := w.store.RecoverStuckJobs(ctx); err != nil {
		return err
	} else if recovered > 0 {
		w.logger.Warn("recovered stuck jobs back to QUEUED", "count", recovered)
	}

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		processed, err := w.RunOnce(ctx)
		if err != nil {
			w.logger.Error("worker poll error", "error", err)
		}
		if processed {
			// Immediately check for the next queued item without waiting for the ticker.
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				continue
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
