package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// AffiliateInput holds product details used to generate short-form affiliate copywriting.
type AffiliateInput struct {
	ProductName       string   `json:"product_name"`
	SellingPoints     []string `json:"selling_points"`
	TargetDurationSec int      `json:"target_duration_sec"`
	ExtraContext      string   `json:"extra_context,omitempty"`
}

// AffiliateScript represents structured copywriting output returned by Ollama.
type AffiliateScript struct {
	HookText        string   `json:"hook_text"`
	VoiceoverScript string   `json:"voiceover_script"`
	Caption         string   `json:"caption"`
	Hashtags        []string `json:"hashtags"`
}

// AffiliateGenerator prompts a local LLM to produce structured TikTok affiliate scripts.
type AffiliateGenerator struct {
	client Client
	prompt string
}

// NewAffiliateGenerator creates an AffiliateGenerator using the configured prompt template.
func NewAffiliateGenerator(client Client, prompt string) *AffiliateGenerator {
	if strings.TrimSpace(prompt) == "" {
		prompt = `Kamu adalah copywriter konten video pendek TikTok Affiliate berbahasa Indonesia.
Balas HANYA JSON valid dengan field: hook_text, voiceover_script, caption, hashtags (array string).`
	}
	return &AffiliateGenerator{
		client: client,
		prompt: prompt,
	}
}

// Generate creates a structured AffiliateScript for input using JSON output mode.
func (g *AffiliateGenerator) Generate(ctx context.Context, input AffiliateInput) (*AffiliateScript, error) {
	if strings.TrimSpace(input.ProductName) == "" {
		return nil, fmt.Errorf("product_name must not be empty")
	}
	if input.TargetDurationSec <= 0 {
		input.TargetDurationSec = 20
	}

	var userMsg strings.Builder
	fmt.Fprintf(&userMsg, "Nama Produk: %s\n", input.ProductName)
	fmt.Fprintf(&userMsg, "Target Durasi Video: %d detik\n", input.TargetDurationSec)
	if len(input.SellingPoints) > 0 {
		userMsg.WriteString("Keunggulan Produk:\n")
		for _, pt := range input.SellingPoints {
			if strings.TrimSpace(pt) != "" {
				fmt.Fprintf(&userMsg, "- %s\n", strings.TrimSpace(pt))
			}
		}
	}
	if strings.TrimSpace(input.ExtraContext) != "" {
		fmt.Fprintf(&userMsg, "\nCatatan Tambahan:\n%s\n", strings.TrimSpace(input.ExtraContext))
	}

	raw, err := g.client.Chat(ctx, []Message{
		{Role: "system", Content: g.prompt},
		{Role: "user", Content: userMsg.String()},
	}, ChatOptions{Format: "json"})
	if err != nil {
		return nil, fmt.Errorf("generate affiliate script: %w", err)
	}

	script, err := parseAffiliateJSON(raw)
	if err != nil {
		return nil, err
	}
	return script, nil
}

func parseAffiliateJSON(raw string) (*AffiliateScript, error) {
	cleaned := stripJSONFence(raw)
	var script AffiliateScript
	if err := json.Unmarshal([]byte(cleaned), &script); err != nil {
		return nil, fmt.Errorf("parse affiliate script json: %w (raw=%q)", err, cleaned)
	}

	script.HookText = strings.TrimSpace(script.HookText)
	script.VoiceoverScript = strings.TrimSpace(script.VoiceoverScript)
	script.Caption = strings.TrimSpace(script.Caption)
	if script.HookText == "" || script.VoiceoverScript == "" || script.Caption == "" {
		return nil, fmt.Errorf("incomplete affiliate script fields in json response")
	}

	normalizedTags := make([]string, 0, len(script.Hashtags))
	for _, tag := range script.Hashtags {
		t := strings.TrimSpace(tag)
		if t == "" {
			continue
		}
		if !strings.HasPrefix(t, "#") {
			t = "#" + t
		}
		normalizedTags = append(normalizedTags, t)
	}
	script.Hashtags = normalizedTags
	return &script, nil
}

func stripJSONFence(s string) string {
	trimmed := strings.TrimSpace(s)
	if strings.HasPrefix(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		if len(lines) >= 3 {
			lines = lines[1:]
			if strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
				lines = lines[:len(lines)-1]
			}
			trimmed = strings.Join(lines, "\n")
		}
	}
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end > start {
		return trimmed[start : end+1]
	}
	return strings.TrimSpace(trimmed)
}
