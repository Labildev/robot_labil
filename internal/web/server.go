package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Labildev/robot_labil/internal/config"
	"github.com/Labildev/robot_labil/internal/doc"
	"github.com/Labildev/robot_labil/internal/llm"
	"github.com/Labildev/robot_labil/internal/pipeline"
	"github.com/Labildev/robot_labil/internal/queue"
	"github.com/Labildev/robot_labil/internal/video"
)

//go:embed templates/index.html
var templateFS embed.FS

// Server exposes the 100% offline local HTTP dashboard and JSON API on 127.0.0.1.
type Server struct {
	cfg       *config.Config
	store     *queue.Store
	llmClient llm.Client
	assistant *llm.DocumentAssistant
	tts       video.TTSSynthesizer
	execMu    *sync.Mutex
	logger    *slog.Logger
	indexHTML []byte
}

// NewServer creates a local Web Dashboard Server.
func NewServer(
	cfg *config.Config,
	store *queue.Store,
	llmClient llm.Client,
	assistant *llm.DocumentAssistant,
	tts video.TTSSynthesizer,
	execMu *sync.Mutex,
	logger *slog.Logger,
) (*Server, error) {
	if logger == nil {
		logger = slog.Default()
	}
	htmlBytes, err := templateFS.ReadFile("templates/index.html")
	if err != nil {
		return nil, fmt.Errorf("read embedded dashboard template: %w", err)
	}
	return &Server{
		cfg:       cfg,
		store:     store,
		llmClient: llmClient,
		assistant: assistant,
		tts:       tts,
		execMu:    execMu,
		logger:    logger,
		indexHTML: htmlBytes,
	}, nil
}

// Handler returns the HTTP router for testing or serving.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/jobs", s.handleJobs)
	mux.HandleFunc("GET /api/docs", s.handleDocs)
	mux.HandleFunc("GET /api/docs/summary", s.handleDocSummary)
	mux.HandleFunc("POST /api/docs/upload", s.handleDocUpload)
	mux.HandleFunc("POST /api/docs/qa", s.handleDocQA)
	mux.HandleFunc("GET /api/assets", s.handleAssets)
	mux.HandleFunc("POST /api/assets/upload", s.handleAssetUpload)
	mux.HandleFunc("POST /api/affiliate", s.handleAffiliate)
	mux.HandleFunc("GET /api/outputs/caption", s.handleCaption)
	mux.HandleFunc("GET /api/outputs/payload", s.handleOutputPayload)
	mux.HandleFunc("GET /api/outputs/video", s.handleVideo)
	return mux
}

// ListenAndServe starts the local HTTP server and shuts down gracefully when ctx is canceled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	addr := strings.TrimSpace(s.cfg.Web.ListenAddr)
	if addr == "" {
		addr = "127.0.0.1:8080"
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("local web dashboard listening", "url", "http://"+addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	case err := <-errCh:
		return err
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(s.indexHTML)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ollamaOK := s.llmClient != nil && s.llmClient.Health(r.Context()) == nil
	piperOK := s.tts != nil && s.tts.Available()
	ffmpegOK := isBinaryAvailable(s.cfg.Video.FFmpegBin, "ffmpeg")
	writeJSON(w, http.StatusOK, map[string]any{
		"app":              s.cfg.App.Name,
		"model":            s.cfg.Ollama.Model,
		"ollama_ok":        ollamaOK,
		"piper_available":  piperOK,
		"ffmpeg_available": ffmpegOK,
		"max_concurrency":  s.cfg.App.MaxConcurrency,
	})
}

func isBinaryAvailable(configured, fallback string) bool {
	if strings.TrimSpace(configured) != "" {
		if info, err := os.Stat(configured); err == nil && !info.IsDir() {
			return true
		}
		if _, err := exec.LookPath(configured); err == nil {
			return true
		}
	}
	_, err := exec.LookPath(fallback)
	return err == nil
}

type jobDTO struct {
	ID             string `json:"id"`
	JobType        string `json:"job_type"`
	SourcePath     string `json:"source_path"`
	Status         string `json:"status"`
	ResultPath     string `json:"result_path"`
	ErrorMessage   string `json:"error_message"`
	CreatedAtLocal string `json:"created_at_local"`
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.store.ListJobs(r.Context(), 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]jobDTO, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, jobDTO{
			ID:             j.ID,
			JobType:        j.JobType,
			SourcePath:     j.SourcePath,
			Status:         j.Status,
			ResultPath:     j.ResultPath,
			ErrorMessage:   j.ErrorMessage,
			CreatedAtLocal: j.CreatedAt.Local().Format("2006-01-02 15:04:05"),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

type docDTO struct {
	ID             string `json:"id"`
	Filename       string `json:"filename"`
	WordCount      int    `json:"word_count"`
	CreatedAtLocal string `json:"created_at_local"`
}

func (s *Server) handleDocs(w http.ResponseWriter, r *http.Request) {
	docs, err := s.store.ListDocuments(r.Context(), 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]docDTO, 0, len(docs))
	for _, d := range docs {
		out = append(out, docDTO{
			ID:             d.ID,
			Filename:       d.Filename,
			WordCount:      d.WordCount,
			CreatedAtLocal: d.CreatedAt.Local().Format("2006-01-02 15:04"),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDocSummary(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "query parameter id is required")
		return
	}
	rec, err := s.store.GetDocumentByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rec == nil || rec.SummaryPath == "" {
		writeError(w, http.StatusNotFound, "summary not found")
		return
	}
	content, err := os.ReadFile(rec.SummaryPath)
	if err != nil {
		writeError(w, http.StatusNotFound, "summary file unreadable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":               rec.ID,
		"filename":         rec.Filename,
		"summary_markdown": string(content),
	})
}

func (s *Server) handleDocUpload(w http.ResponseWriter, r *http.Request) {
	base, absDst, err := s.saveUploadedFile(w, r, s.cfg.Paths.InputDocs, 50<<20, doc.IsSupportedExt)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	jobID, err := s.store.Enqueue(r.Context(), queue.JobTypeDocSummary, absDst, "{}")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"job_id":   jobID,
		"filename": base,
		"status":   queue.StatusQueued,
	})
}

func (s *Server) handleAssetUpload(w http.ResponseWriter, r *http.Request) {
	base, absDst, err := s.saveUploadedFile(w, r, s.cfg.Paths.AffiliateAssets, 200<<20, isSupportedAssetExt)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rel, relErr := filepath.Rel(".", absDst)
	if relErr != nil || strings.HasPrefix(rel, "..") {
		rel = absDst
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"filename": base,
		"path":     filepath.ToSlash(rel),
	})
}

func (s *Server) saveUploadedFile(w http.ResponseWriter, r *http.Request, targetDir string, maxBytes int64, allow func(string) bool) (string, string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return "", "", fmt.Errorf("invalid multipart upload: %w", err)
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return "", "", fmt.Errorf("missing form field 'file'")
	}
	defer file.Close()

	base := filepath.Base(header.Filename)
	if base == "" || base == "." || !allow(base) {
		return "", "", fmt.Errorf("unsupported file extension for %q", base)
	}

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", "", err
	}
	dstPath := filepath.Join(targetDir, base)
	absDst, err := filepath.Abs(dstPath)
	if err != nil {
		absDst = dstPath
	}

	out, err := os.Create(absDst)
	if err != nil {
		return "", "", err
	}
	if _, err := io.Copy(out, file); err != nil {
		_ = out.Close()
		return "", "", err
	}
	if err := out.Close(); err != nil {
		return "", "", err
	}
	return base, absDst, nil
}

func isSupportedAssetExt(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".mov", ".mkv", ".webm", ".avi", ".jpg", ".jpeg", ".png", ".webp", ".mp3", ".wav", ".m4a", ".aac":
		return true
	default:
		return false
	}
}

func (s *Server) handleDocQA(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DocID    string `json:"doc_id"`
		DocName  string `json:"doc_name"`
		Question string `json:"question"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json payload")
		return
	}
	if strings.TrimSpace(req.Question) == "" {
		writeError(w, http.StatusBadRequest, "question must not be empty")
		return
	}

	var rec *queue.DocumentRecord
	var err error
	if strings.TrimSpace(req.DocID) != "" {
		rec, err = s.store.GetDocumentByID(r.Context(), req.DocID)
	} else {
		rec, err = s.store.FindDocumentByName(r.Context(), req.DocName)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rec == nil {
		writeError(w, http.StatusNotFound, "document not found; summarize a document first")
		return
	}

	if s.execMu != nil {
		s.execMu.Lock()
		defer s.execMu.Unlock()
	}
	answer, err := s.assistant.AnswerQuestion(r.Context(), rec.Filename, rec.ExtractedText, req.Question, s.cfg.Document.ChunkWordLimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"filename": rec.Filename,
		"answer":   answer,
	})
}

func (s *Server) handleAssets(w http.ResponseWriter, _ *http.Request) {
	root := s.cfg.Paths.AffiliateAssets
	var assets []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (name == "jobs" || name == "templates") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == ".gitkeep" {
			return nil
		}
		assets = append(assets, filepath.ToSlash(path))
		return nil
	})
	if assets == nil {
		assets = []string{}
	}
	writeJSON(w, http.StatusOK, assets)
}

func (s *Server) handleAffiliate(w http.ResponseWriter, r *http.Request) {
	var req pipeline.AffiliateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json payload")
		return
	}
	if strings.TrimSpace(req.ProductName) == "" || len(req.Clips) == 0 {
		writeError(w, http.StatusBadRequest, "product_name and at least one clip are required")
		return
	}

	payloadBytes, err := json.Marshal(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jobID, err := s.store.Enqueue(r.Context(), queue.JobTypeAffiliateVideo, req.ProductName, string(payloadBytes))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"job_id": jobID,
		"status": queue.StatusQueued,
	})
}

func (s *Server) handleCaption(w http.ResponseWriter, r *http.Request) {
	jobID := filepath.Base(strings.TrimSpace(r.URL.Query().Get("job_id")))
	if jobID == "" || jobID == "." {
		writeError(w, http.StatusBadRequest, "job_id is required")
		return
	}
	captionPath := filepath.Join(s.cfg.Paths.OutputVideos, jobID, "caption.txt")
	data, err := os.ReadFile(captionPath)
	if err != nil {
		writeError(w, http.StatusNotFound, "caption.txt not found for job")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"job_id":  jobID,
		"caption": string(data),
	})
}

func (s *Server) handleOutputPayload(w http.ResponseWriter, r *http.Request) {
	jobID := filepath.Base(strings.TrimSpace(r.URL.Query().Get("job_id")))
	if jobID == "" || jobID == "." {
		writeError(w, http.StatusBadRequest, "job_id is required")
		return
	}
	payloadPath := filepath.Join(s.cfg.Paths.OutputVideos, jobID, "job_payload.json")
	data, err := os.ReadFile(payloadPath)
	if err != nil {
		writeError(w, http.StatusNotFound, "job_payload.json not found for job")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleVideo(w http.ResponseWriter, r *http.Request) {
	jobID := filepath.Base(strings.TrimSpace(r.URL.Query().Get("job_id")))
	if jobID == "" || jobID == "." {
		writeError(w, http.StatusBadRequest, "job_id is required")
		return
	}
	videoPath := filepath.Join(s.cfg.Paths.OutputVideos, jobID, jobID+".mp4")
	if _, err := os.Stat(videoPath); err != nil {
		writeError(w, http.StatusNotFound, "video not found for job")
		return
	}
	http.ServeFile(w, r, videoPath)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
