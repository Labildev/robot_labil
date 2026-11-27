package llm

import (
	"context"
	"fmt"
	"testing"
)

type chatStub struct {
	captured []Message
}

func (s *chatStub) Chat(_ context.Context, messages []Message, _ ChatOptions) (string, error) {
	s.captured = messages
	return "Tentu, ini jawabannya.", nil
}
func (s *chatStub) UnloadModel(_ context.Context) error { return nil }
func (s *chatStub) Health(_ context.Context) error      { return nil }

func TestGeneralChatCapsHistoryAndPrependsSystem(t *testing.T) {
	stub := &chatStub{}
	gc := NewGeneralChat(stub, "Asisten lokal")

	var history []Message
	for i := 0; i < 16; i++ {
		history = append(history, Message{Role: "user", Content: fmt.Sprintf("msg-%d", i)})
	}
	// Invalid role should be filtered out.
	history = append(history, Message{Role: "system", Content: "ignore-me"})

	reply, err := gc.Reply(context.Background(), history, "Pertanyaan terbaru?")
	if err != nil {
		t.Fatalf("Reply() failed: %v", err)
	}
	if reply != "Tentu, ini jawabannya." {
		t.Fatalf("unexpected reply: %q", reply)
	}
	// Expect 1 system + 10 capped history + 1 current message = 12 messages.
	if len(stub.captured) != 12 {
		t.Fatalf("expected 12 messages (system + 10 history + latest), got %d", len(stub.captured))
	}
	if stub.captured[0].Role != "system" || stub.captured[1].Content != "msg-6" || stub.captured[11].Content != "Pertanyaan terbaru?" {
		t.Fatalf("unexpected captured message sequence: %+v", stub.captured)
	}
}
