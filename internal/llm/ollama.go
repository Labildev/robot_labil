package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Message represents a single chat turn sent to or received from Ollama.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatOptions allows overriding per-request Ollama inference options.
type ChatOptions struct {
	KeepAlive string
	Format    string
	NumCtx    int
}

// Client defines the local LLM interaction contract.
type Client interface {
	Chat(ctx context.Context, messages []Message, opts ChatOptions) (string, error)
	UnloadModel(ctx context.Context) error
	Health(ctx context.Context) error
}

// OllamaClient calls the local Ollama REST API (127.0.0.1:11434).
type OllamaClient struct {
	baseURL    string
	model      string
	numCtx     int
	keepAlive  string
	httpClient *http.Client
}

// NewOllamaClient constructs an OllamaClient bound to a local Ollama instance.
func NewOllamaClient(baseURL, model string, numCtx int, keepAlive string, timeout time.Duration) *OllamaClient {
	if numCtx <= 0 {
		numCtx = 4096
	}
	if strings.TrimSpace(keepAlive) == "" {
		keepAlive = "2m"
	}
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	return &OllamaClient{
		baseURL:   strings.TrimRight(baseURL, "/"),
		model:     model,
		numCtx:    numCtx,
		keepAlive: keepAlive,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

type chatRequest struct {
	Model     string         `json:"model"`
	Messages  []Message      `json:"messages"`
	Stream    bool           `json:"stream"`
	Format    string         `json:"format,omitempty"`
	KeepAlive string         `json:"keep_alive,omitempty"`
	Options   map[string]any `json:"options,omitempty"`
}

type chatResponse struct {
	Message Message `json:"message"`
	Error   string  `json:"error,omitempty"`
}

// Chat executes a non-streaming /api/chat request against Ollama.
func (c *OllamaClient) Chat(ctx context.Context, messages []Message, opts ChatOptions) (string, error) {
	numCtx := c.numCtx
	if opts.NumCtx > 0 {
		numCtx = opts.NumCtx
	}
	keepAlive := c.keepAlive
	if strings.TrimSpace(opts.KeepAlive) != "" {
		keepAlive = opts.KeepAlive
	}

	payload := chatRequest{
		Model:     c.model,
		Messages:  messages,
		Stream:    false,
		Format:    opts.Format,
		KeepAlive: keepAlive,
		Options: map[string]any{
			"num_ctx": numCtx,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal ollama chat request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call ollama /api/chat: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read ollama response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("ollama status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBytes)))
	}

	var parsed chatResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return "", fmt.Errorf("decode ollama response: %w", err)
	}
	if parsed.Error != "" {
		return "", fmt.Errorf("ollama error: %s", parsed.Error)
	}
	return strings.TrimSpace(parsed.Message.Content), nil
}

// UnloadModel asks Ollama to release the active model from GPU VRAM immediately (keep_alive=0).
func (c *OllamaClient) UnloadModel(ctx context.Context) error {
	payload := map[string]any{
		"model":      c.model,
		"prompt":     "",
		"keep_alive": 0,
		"stream":     false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("unload ollama model: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unload ollama model status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// Health verifies that the local Ollama daemon responds on /api/tags.
func (c *OllamaClient) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ollama unreachable at %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("ollama health status %d", resp.StatusCode)
	}
	return nil
}
