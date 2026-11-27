package queue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Job status constants stored in SQLite.
const (
	StatusQueued     = "QUEUED"
	StatusProcessing = "PROCESSING"
	StatusDone       = "DONE"
	StatusFailed     = "FAILED"
)

// Job types supported by the serial worker.
const (
	JobTypeDocSummary     = "doc_summary"
	JobTypeAffiliateVideo = "affiliate_video"
)

// Job represents a queued unit of work persisted in SQLite.
type Job struct {
	ID           string
	JobType      string
	SourcePath   string
	PayloadJSON  string
	Status       string
	ResultPath   string
	ErrorMessage string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// DocumentRecord stores extracted document text and its generated summary path for local Q&A.
type DocumentRecord struct {
	ID            string
	Filename      string
	SourcePath    string
	SummaryPath   string
	WordCount     int
	ExtractedText string
	CreatedAt     time.Time
}

// Store manages the local SQLite database for serial job queuing and document history.
type Store struct {
	db *sql.DB
}

// Open initializes the SQLite connection, pragmas, and schema migrations.
func Open(dbPath string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", filepath.ToSlash(dbPath))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", dbPath, err)
	}
	// Single-writer connection pool prevents SQLITE_BUSY under serial worker execution.
	db.SetMaxOpenConns(1)

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// Close closes the underlying SQLite database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS jobs (
	id TEXT PRIMARY KEY,
	job_type TEXT NOT NULL,
	source_path TEXT NOT NULL,
	payload_json TEXT NOT NULL,
	status TEXT NOT NULL,
	result_path TEXT NOT NULL DEFAULT '',
	error_message TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_jobs_status_created ON jobs(status, created_at);

CREATE TABLE IF NOT EXISTS documents (
	id TEXT PRIMARY KEY,
	filename TEXT NOT NULL,
	source_path TEXT NOT NULL,
	summary_path TEXT NOT NULL DEFAULT '',
	word_count INTEGER NOT NULL,
	extracted_text TEXT NOT NULL,
	created_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_documents_filename ON documents(filename);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate sqlite schema: %w", err)
	}
	return nil
}

// RecoverStuckJobs resets jobs left in PROCESSING after an unexpected shutdown back to QUEUED.
func (s *Store) RecoverStuckJobs(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET status = ?, updated_at = ? WHERE status = ?`,
		StatusQueued, time.Now().UTC(), StatusProcessing,
	)
	if err != nil {
		return 0, fmt.Errorf("recover stuck jobs: %w", err)
	}
	return res.RowsAffected()
}

// Enqueue inserts a new job into the SQLite queue with status QUEUED.
func (s *Store) Enqueue(ctx context.Context, jobType, sourcePath, payloadJSON string) (string, error) {
	now := time.Now().UTC()
	id := fmt.Sprintf("%s_%d", jobType, now.UnixNano())
	if strings.TrimSpace(payloadJSON) == "" {
		payloadJSON = "{}"
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO jobs (id, job_type, source_path, payload_json, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, jobType, sourcePath, payloadJSON, StatusQueued, now, now,
	)
	if err != nil {
		return "", fmt.Errorf("enqueue job: %w", err)
	}
	return id, nil
}

// HasActiveOrCompletedSource reports whether sourcePath is already queued, processing, or completed.
func (s *Store) HasActiveOrCompletedSource(ctx context.Context, sourcePath string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM jobs WHERE source_path = ? AND status IN (?, ?, ?)`,
		sourcePath, StatusQueued, StatusProcessing, StatusDone,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check source_path %q: %w", sourcePath, err)
	}
	return count > 0, nil
}

// ClaimNext atomically picks the oldest QUEUED job and transitions it to PROCESSING.
// Returns (nil, nil) when the queue is empty.
func (s *Store) ClaimNext(ctx context.Context) (*Job, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin claim tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var job Job
	row := tx.QueryRowContext(ctx,
		`SELECT id, job_type, source_path, payload_json, status, result_path, error_message, created_at, updated_at
		 FROM jobs
		 WHERE status = ?
		 ORDER BY created_at ASC
		 LIMIT 1`,
		StatusQueued,
	)
	err = row.Scan(
		&job.ID, &job.JobType, &job.SourcePath, &job.PayloadJSON,
		&job.Status, &job.ResultPath, &job.ErrorMessage, &job.CreatedAt, &job.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("select next job: %w", err)
	}

	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx,
		`UPDATE jobs SET status = ?, updated_at = ? WHERE id = ?`,
		StatusProcessing, now, job.ID,
	); err != nil {
		return nil, fmt.Errorf("claim job %q: %w", job.ID, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit claim %q: %w", job.ID, err)
	}

	job.Status = StatusProcessing
	job.UpdatedAt = now
	return &job, nil
}

// MarkDone updates a job status to DONE and records its output artifact path.
func (s *Store) MarkDone(ctx context.Context, id, resultPath string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET status = ?, result_path = ?, error_message = '', updated_at = ? WHERE id = ?`,
		StatusDone, resultPath, time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("mark done %q: %w", id, err)
	}
	return nil
}

// MarkFailed updates a job status to FAILED with the error message.
func (s *Store) MarkFailed(ctx context.Context, id, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET status = ?, error_message = ?, updated_at = ? WHERE id = ?`,
		StatusFailed, errMsg, time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("mark failed %q: %w", id, err)
	}
	return nil
}

// ListJobs returns recent jobs ordered newest first.
func (s *Store) ListJobs(ctx context.Context, limit int) ([]Job, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, job_type, source_path, payload_json, status, result_path, error_message, created_at, updated_at
		 FROM jobs
		 ORDER BY created_at DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(
			&j.ID, &j.JobType, &j.SourcePath, &j.PayloadJSON,
			&j.Status, &j.ResultPath, &j.ErrorMessage, &j.CreatedAt, &j.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// SaveDocument persists an extracted document and its summary path for local Q&A lookup.
func (s *Store) SaveDocument(ctx context.Context, rec DocumentRecord) error {
	if rec.ID == "" {
		rec.ID = fmt.Sprintf("doc_%d", time.Now().UTC().UnixNano())
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO documents (id, filename, source_path, summary_path, word_count, extracted_text, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		rec.ID, rec.Filename, rec.SourcePath, rec.SummaryPath, rec.WordCount, rec.ExtractedText, rec.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("save document %q: %w", rec.Filename, err)
	}
	return nil
}

// FindDocumentByName returns the latest document matching filename (case-insensitive substring or exact match).
func (s *Store) FindDocumentByName(ctx context.Context, name string) (*DocumentRecord, error) {
	var rec DocumentRecord
	var row *sql.Row
	if strings.TrimSpace(name) == "" {
		row = s.db.QueryRowContext(ctx,
			`SELECT id, filename, source_path, summary_path, word_count, extracted_text, created_at
			 FROM documents ORDER BY created_at DESC LIMIT 1`,
		)
	} else {
		row = s.db.QueryRowContext(ctx,
			`SELECT id, filename, source_path, summary_path, word_count, extracted_text, created_at
			 FROM documents
			 WHERE LOWER(filename) LIKE LOWER(?)
			 ORDER BY created_at DESC LIMIT 1`,
			"%"+name+"%",
		)
	}
	err := row.Scan(
		&rec.ID, &rec.Filename, &rec.SourcePath, &rec.SummaryPath,
		&rec.WordCount, &rec.ExtractedText, &rec.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find document %q: %w", name, err)
	}
	return &rec, nil
}
