package web

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Labildev/robot_labil/internal/config"
	"github.com/Labildev/robot_labil/internal/llm"
	"github.com/Labildev/robot_labil/internal/queue"
	"github.com/Labildev/robot_labil/internal/video"
)

type stubClient struct{}

func (s *stubClient) Chat(_ context.Context, _ []llm.Message, _ llm.ChatOptions) (string, error) {
	return "Jawaban Q&A dari Ollama lokal.", nil
}
func (s *stubClient) UnloadModel(_ context.Context) error { return nil }
func (s *stubClient) Health(_ context.Context) error      { return nil }

func TestWebDashboardAPIs(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()

	cfg := config.Default()
	cfg.Paths.Database = filepath.Join(tmp, "storage", "robot.db")
	cfg.Paths.InputDocs = filepath.Join(tmp, "storage", "input_docs")
	cfg.Paths.OutputDocs = filepath.Join(tmp, "storage", "output_docs")
	cfg.Paths.AffiliateAssets = filepath.Join(tmp, "storage", "affiliate_assets")
	cfg.Paths.OutputVideos = filepath.Join(tmp, "storage", "output_videos")
	cfg.Paths.ErrorLogs = filepath.Join(tmp, "storage", "error_logs")
	cfg.Paths.PromptsDir = filepath.Join(tmp, "prompts")
	cfg.Ollama.ModelsDir = filepath.Join(tmp, "storage", "models")
	if err := cfg.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories failed: %v", err)
	}

	store, err := queue.Open(cfg.Paths.Database)
	if err != nil {
		t.Fatalf("Open store failed: %v", err)
	}
	defer store.Close()

	summaryPath := filepath.Join(cfg.Paths.OutputDocs, "manual.md")
	if err := os.WriteFile(summaryPath, []byte("## Ringkasan\nManual produk."), 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}
	if err := store.SaveDocument(ctx, queue.DocumentRecord{
		ID:            "doc_1",
		Filename:      "manual.docx",
		SourcePath:    filepath.Join(cfg.Paths.InputDocs, "manual.docx"),
		SummaryPath:   summaryPath,
		WordCount:     120,
		ExtractedText: "Garansi produk 12 bulan.",
	}); err != nil {
		t.Fatalf("SaveDocument failed: %v", err)
	}

	var mu sync.Mutex
	srv, err := NewServer(
		cfg,
		store,
		&stubClient{},
		llm.NewDocumentAssistant(&stubClient{}, "", ""),
		video.NewPiperTTS(false, "", ""),
		&mu,
		nil,
	)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	h := srv.Handler()

	// 1. Dashboard HTML
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Robot Labil") {
		t.Fatalf("unexpected index response: code=%d", rec.Code)
	}

	// 2. Health endpoint
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ollama_ok":true`) {
		t.Fatalf("unexpected health response: %s", rec.Body.String())
	}

	// 3. Summary + Q&A endpoint
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/docs/summary?id=doc_1", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Manual produk.") {
		t.Fatalf("unexpected summary response: %s", rec.Body.String())
	}

	qaBody := strings.NewReader(`{"doc_id":"doc_1","question":"Berapa garansinya?"}`)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/docs/qa", qaBody))
	var qaResp struct {
		Filename string `json:"filename"`
		Answer   string `json:"answer"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &qaResp); err != nil || rec.Code != http.StatusOK || qaResp.Answer != "Jawaban Q&A dari Ollama lokal." {
		t.Fatalf("unexpected QA response: code=%d body=%s err=%v", rec.Code, rec.Body.String(), err)
	}

	// 4. Multipart document upload
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "catatan.txt")
	_, _ = fw.Write([]byte("Isi catatan lokal"))
	_ = mw.Close()
	uploadReq := httptest.NewRequest(http.MethodPost, "/api/docs/upload", &buf)
	uploadReq.Header.Set("Content-Type", mw.FormDataContentType())
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, uploadReq)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 upload, got %d: %s", rec.Code, rec.Body.String())
	}

	// 5. Enqueue affiliate job
	affReq := httptest.NewRequest(http.MethodPost, "/api/affiliate", strings.NewReader(`{"product_name":"Tripod","selling_points":["Kuat"],"clips":["a.mp4"]}`))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, affReq)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 affiliate, got %d: %s", rec.Code, rec.Body.String())
	}

	// 6. Jobs list should contain both queued jobs
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/jobs", nil))
	var jobs []jobDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &jobs); err != nil || len(jobs) != 2 {
		t.Fatalf("expected 2 jobs in list, got len=%d err=%v", len(jobs), err)
	}
}
