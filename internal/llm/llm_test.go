package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOllamaClientChatAndUnload(t *testing.T) {
	var sawUnload bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/chat":
			var req chatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode chat request: %v", err)
			}
			if req.Model != "qwen2.5:3b" || req.Stream {
				t.Fatalf("unexpected chat payload: %+v", req)
			}
			_ = json.NewEncoder(w).Encode(chatResponse{
				Message: Message{Role: "assistant", Content: "Ringkasan berhasil dibuat."},
			})
		case "/api/generate":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["keep_alive"] == float64(0) {
				sawUnload = true
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"done":true}`))
		case "/api/tags":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"models":[]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "qwen2.5:3b", 4096, "2m", 5*time.Second)
	if err := client.Health(context.Background()); err != nil {
		t.Fatalf("Health() failed: %v", err)
	}
	reply, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "Halo"}}, ChatOptions{})
	if err != nil {
		t.Fatalf("Chat() failed: %v", err)
	}
	if reply != "Ringkasan berhasil dibuat." {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if err := client.UnloadModel(context.Background()); err != nil || !sawUnload {
		t.Fatalf("UnloadModel() failed: err=%v sawUnload=%v", err, sawUnload)
	}
}

type fakeClient struct {
	calls []string
}

func (f *fakeClient) Chat(_ context.Context, messages []Message, _ ChatOptions) (string, error) {
	last := messages[len(messages)-1].Content
	f.calls = append(f.calls, last)
	if strings.Contains(last, "Gabungkan ringkasan") {
		return "## Ringkasan Eksekutif\nRingkasan gabungan.", nil
	}
	return "Poin ringkasan.", nil
}

func (f *fakeClient) UnloadModel(_ context.Context) error { return nil }
func (f *fakeClient) Health(_ context.Context) error      { return nil }

func TestDocumentAssistantMapReduceAndQA(t *testing.T) {
	fc := &fakeClient{}
	assistant := NewDocumentAssistant(fc, "prompt summary", "prompt qa")

	// Short document: single call.
	res, err := assistant.Summarize(context.Background(), "short.pdf", "satu dua tiga", 10, 5)
	if err != nil {
		t.Fatalf("Summarize short failed: %v", err)
	}
	if len(fc.calls) != 1 || res != "Poin ringkasan." {
		t.Fatalf("unexpected single-pass behavior: calls=%d res=%q", len(fc.calls), res)
	}

	// Long document: 2 map calls + 1 reduce call = 3 additional calls.
	fc.calls = nil
	longText := strings.Repeat("kata ", 12)
	resLong, err := assistant.Summarize(context.Background(), "long.pdf", longText, 5, 6)
	if err != nil {
		t.Fatalf("Summarize long failed: %v", err)
	}
	if len(fc.calls) != 3 || !strings.Contains(resLong, "Ringkasan gabungan") {
		t.Fatalf("unexpected map-reduce calls=%d res=%q", len(fc.calls), resLong)
	}
}
