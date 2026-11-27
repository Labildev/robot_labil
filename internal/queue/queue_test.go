package queue

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestStoreLifecycleAndSerialWorker(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "robot.db")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer store.Close()

	id1, err := store.Enqueue(ctx, JobTypeDocSummary, "doc1.pdf", "{}")
	if err != nil {
		t.Fatalf("Enqueue 1 failed: %v", err)
	}
	id2, err := store.Enqueue(ctx, JobTypeDocSummary, "doc2.docx", "{}")
	if err != nil {
		t.Fatalf("Enqueue 2 failed: %v", err)
	}

	var active int32
	var maxConcurrent int32
	var order []string

	worker := NewWorker(store, nil, 10*time.Millisecond, map[string]Handler{
		JobTypeDocSummary: func(_ context.Context, job *Job) (string, error) {
			cur := atomic.AddInt32(&active, 1)
			if cur > atomic.LoadInt32(&maxConcurrent) {
				atomic.StoreInt32(&maxConcurrent, cur)
			}
			order = append(order, job.ID)
			atomic.AddInt32(&active, -1)

			if job.ID == id1 {
				return "storage/output_docs/doc1.md", nil
			}
			return "", errors.New("corrupted docx")
		},
	})

	for i := 0; i < 2; i++ {
		processed, err := worker.RunOnce(ctx)
		if err != nil || !processed {
			t.Fatalf("RunOnce %d failed: processed=%v err=%v", i, processed, err)
		}
	}
	if processed, err := worker.RunOnce(ctx); err != nil || processed {
		t.Fatalf("expected empty queue, got processed=%v err=%v", processed, err)
	}
	if maxConcurrent != 1 {
		t.Fatalf("expected strict serial execution, maxConcurrent=%d", maxConcurrent)
	}
	if len(order) != 2 || order[0] != id1 || order[1] != id2 {
		t.Fatalf("unexpected FIFO execution order: %v", order)
	}

	jobs, err := store.ListJobs(ctx, 10)
	if err != nil {
		t.Fatalf("ListJobs() failed: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
	statusByID := map[string]Job{}
	for _, j := range jobs {
		statusByID[j.ID] = j
	}
	if statusByID[id1].Status != StatusDone || statusByID[id1].ResultPath != "storage/output_docs/doc1.md" {
		t.Fatalf("unexpected job1 state: %+v", statusByID[id1])
	}
	if statusByID[id2].Status != StatusFailed || statusByID[id2].ErrorMessage != "corrupted docx" {
		t.Fatalf("unexpected job2 state: %+v", statusByID[id2])
	}
}

func TestDocumentStorageAndLookup(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "robot.db"))
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer store.Close()

	err = store.SaveDocument(ctx, DocumentRecord{
		Filename:      "Laporan_Penjualan.pdf",
		SourcePath:    "storage/input_docs/Laporan_Penjualan.pdf",
		SummaryPath:   "storage/output_docs/Laporan_Penjualan.md",
		WordCount:     420,
		ExtractedText: "Total omzet bulan ini naik 30 persen.",
	})
	if err != nil {
		t.Fatalf("SaveDocument() failed: %v", err)
	}

	found, err := store.FindDocumentByName(ctx, "laporan")
	if err != nil {
		t.Fatalf("FindDocumentByName() failed: %v", err)
	}
	if found == nil || found.WordCount != 420 {
		t.Fatalf("expected document match, got %+v", found)
	}
}
