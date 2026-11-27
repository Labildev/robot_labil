package llm

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Labildev/robot_labil/internal/doc"
)

// DocumentAssistant summarizes local documents and answers questions using a local LLM Client.
type DocumentAssistant struct {
	client        Client
	summaryPrompt string
	qaPrompt      string
}

// NewDocumentAssistant builds a DocumentAssistant with externalized prompt templates.
func NewDocumentAssistant(client Client, summaryPrompt, qaPrompt string) *DocumentAssistant {
	if strings.TrimSpace(summaryPrompt) == "" {
		summaryPrompt = "Ringkas teks dokumen berikut dalam bahasa Indonesia secara terstruktur."
	}
	if strings.TrimSpace(qaPrompt) == "" {
		qaPrompt = "Jawab pertanyaan pengguna hanya berdasarkan konteks dokumen berikut."
	}
	return &DocumentAssistant{
		client:        client,
		summaryPrompt: summaryPrompt,
		qaPrompt:      qaPrompt,
	}
}

// Summarize produces a Markdown summary, automatically switching to Map-Reduce for long documents.
func (a *DocumentAssistant) Summarize(ctx context.Context, filename, text string, thresholdWords, chunkWordLimit int) (string, error) {
	if thresholdWords <= 0 {
		thresholdWords = 3000
	}
	if chunkWordLimit <= 0 {
		chunkWordLimit = 2000
	}

	words := doc.CountWords(text)
	if words <= thresholdWords {
		return a.client.Chat(ctx, []Message{
			{Role: "system", Content: a.summaryPrompt},
			{Role: "user", Content: fmt.Sprintf("Nama file: %s\n\nIsi dokumen:\n%s", filename, text)},
		}, ChatOptions{})
	}

	chunks := doc.ChunkByWords(text, chunkWordLimit)
	partials := make([]string, 0, len(chunks))
	for i, chunk := range chunks {
		partial, err := a.client.Chat(ctx, []Message{
			{Role: "system", Content: "Ringkas bagian dokumen ini dalam poin-poin penting berbahasa Indonesia secara faktual."},
			{Role: "user", Content: fmt.Sprintf("File: %s (Bagian %d dari %d)\n\n%s", filename, i+1, len(chunks), chunk)},
		}, ChatOptions{})
		if err != nil {
			return "", fmt.Errorf("summarize chunk %d/%d: %w", i+1, len(chunks), err)
		}
		partials = append(partials, fmt.Sprintf("### Bagian %d\n%s", i+1, partial))
	}

	combined := strings.Join(partials, "\n\n")
	return a.client.Chat(ctx, []Message{
		{Role: "system", Content: a.summaryPrompt},
		{Role: "user", Content: fmt.Sprintf("Gabungkan ringkasan per bagian dari dokumen %q menjadi satu ringkasan final yang utuh:\n\n%s", filename, combined)},
	}, ChatOptions{})
}

// AnswerQuestion answers a user question grounded in the extracted document text.
// For large documents, it selects the most relevant chunks to stay within the 4096-token context budget.
func (a *DocumentAssistant) AnswerQuestion(ctx context.Context, filename, text, question string, chunkWordLimit int) (string, error) {
	if strings.TrimSpace(question) == "" {
		return "", fmt.Errorf("question must not be empty")
	}
	if chunkWordLimit <= 0 {
		chunkWordLimit = 1800
	}

	contextText := selectRelevantContext(text, question, chunkWordLimit)
	return a.client.Chat(ctx, []Message{
		{Role: "system", Content: a.qaPrompt},
		{Role: "user", Content: fmt.Sprintf("Dokumen: %s\n\nKonteks:\n%s\n\nPertanyaan: %s", filename, contextText, question)},
	}, ChatOptions{})
}

func selectRelevantContext(text, question string, maxWords int) string {
	if doc.CountWords(text) <= maxWords {
		return text
	}
	chunks := doc.ChunkByWords(text, 600)
	if len(chunks) == 0 {
		return text
	}

	terms := extractQueryTerms(question)
	type scoredChunk struct {
		index int
		score int
		text  string
	}
	scored := make([]scoredChunk, 0, len(chunks))
	for i, c := range chunks {
		lower := strings.ToLower(c)
		score := 0
		for _, term := range terms {
			score += strings.Count(lower, term)
		}
		scored = append(scored, scoredChunk{index: i, score: score, text: c})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].index < scored[j].index
	})

	var selected []scoredChunk
	usedWords := 0
	for _, sc := range scored {
		w := doc.CountWords(sc.text)
		if usedWords+w > maxWords && len(selected) > 0 {
			continue
		}
		selected = append(selected, sc)
		usedWords += w
		if usedWords >= maxWords {
			break
		}
	}

	// Restore original document reading order for coherence.
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].index < selected[j].index
	})
	parts := make([]string, 0, len(selected))
	for _, sc := range selected {
		parts = append(parts, sc.text)
	}
	return strings.Join(parts, "\n\n---\n\n")
}

func extractQueryTerms(question string) []string {
	fields := strings.Fields(strings.ToLower(question))
	var terms []string
	for _, f := range fields {
		cleaned := strings.Trim(f, ".,?!:;\"'()[]{}")
		if len(cleaned) >= 3 {
			terms = append(terms, cleaned)
		}
	}
	return terms
}
