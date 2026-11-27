package doc

import (
	"strings"
)

// CountWords returns the number of whitespace-delimited words in text.
func CountWords(text string) int {
	return len(strings.Fields(text))
}

// ChunkByWords splits text into chunks of at most maxWords words, preserving paragraph boundaries where possible.
func ChunkByWords(text string, maxWords int) []string {
	if maxWords <= 0 {
		maxWords = 2000
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}

	paragraphs := strings.Split(trimmed, "\n\n")
	var chunks []string
	var cur []string
	curWords := 0

	flush := func() {
		if len(cur) > 0 {
			chunks = append(chunks, strings.Join(cur, "\n\n"))
			cur = nil
			curWords = 0
		}
	}

	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		pw := CountWords(p)
		if pw > maxWords {
			flush()
			words := strings.Fields(p)
			for i := 0; i < len(words); i += maxWords {
				end := i + maxWords
				if end > len(words) {
					end = len(words)
				}
				chunks = append(chunks, strings.Join(words[i:end], " "))
			}
			continue
		}
		if curWords+pw > maxWords && curWords > 0 {
			flush()
		}
		cur = append(cur, p)
		curWords += pw
	}
	flush()
	return chunks
}
