package llm

import (
	"context"
	"fmt"
	"strings"
)

const maxChatHistoryTurns = 10

// GeneralChat handles general-purpose multi-turn Q&A with the local Ollama model.
type GeneralChat struct {
	client       Client
	systemPrompt string
}

// NewGeneralChat creates a GeneralChat assistant with an optional external system prompt.
func NewGeneralChat(client Client, systemPrompt string) *GeneralChat {
	if strings.TrimSpace(systemPrompt) == "" {
		systemPrompt = "Kamu adalah asisten AI lokal berbahasa Indonesia yang jelas, ringkas, dan membantu."
	}
	return &GeneralChat{
		client:       client,
		systemPrompt: systemPrompt,
	}
}

// Reply sends conversation history plus the new user message to the local LLM,
// capping retained history turns so context stays safely within num_ctx=4096.
func (g *GeneralChat) Reply(ctx context.Context, history []Message, userMessage string) (string, error) {
	msg := strings.TrimSpace(userMessage)
	if msg == "" {
		return "", fmt.Errorf("message must not be empty")
	}

	messages := []Message{{Role: "system", Content: g.systemPrompt}}

	// Filter to valid user/assistant turns and cap length for 4096-token budget.
	cleanHistory := make([]Message, 0, len(history))
	for _, h := range history {
		role := strings.ToLower(strings.TrimSpace(h.Role))
		content := strings.TrimSpace(h.Content)
		if (role == "user" || role == "assistant") && content != "" {
			cleanHistory = append(cleanHistory, Message{Role: role, Content: content})
		}
	}
	if len(cleanHistory) > maxChatHistoryTurns {
		cleanHistory = cleanHistory[len(cleanHistory)-maxChatHistoryTurns:]
	}
	messages = append(messages, cleanHistory...)
	messages = append(messages, Message{Role: "user", Content: msg})

	return g.client.Chat(ctx, messages, ChatOptions{})
}
