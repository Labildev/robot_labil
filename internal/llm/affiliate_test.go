package llm

import (
	"context"
	"testing"
)

type affiliateStubClient struct {
	response string
	lastOpts ChatOptions
}

func (s *affiliateStubClient) Chat(_ context.Context, _ []Message, opts ChatOptions) (string, error) {
	s.lastOpts = opts
	return s.response, nil
}
func (s *affiliateStubClient) UnloadModel(_ context.Context) error { return nil }
func (s *affiliateStubClient) Health(_ context.Context) error      { return nil }

func TestAffiliateGeneratorParsesFenceAndNormalizesHashtags(t *testing.T) {
	stub := &affiliateStubClient{
		response: "```json\n{\"hook_text\":\"Stop foto buram!\",\"voiceover_script\":\"Pakai tripod ini aja, cek keranjang kuning!\",\"caption\":\"Tripod wajib solo traveler\",\"hashtags\":[\"tripodmini\",\"#racuntiktok\"]}\n```",
	}
	gen := NewAffiliateGenerator(stub, "")
	script, err := gen.Generate(context.Background(), AffiliateInput{
		ProductName:       "Mini Tripod",
		SellingPoints:     []string{"Ringan", "Remote Bluetooth"},
		TargetDurationSec: 20,
	})
	if err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}
	if stub.lastOpts.Format != "json" {
		t.Fatalf("expected JSON format option, got %q", stub.lastOpts.Format)
	}
	if len(script.Hashtags) != 2 || script.Hashtags[0] != "#tripodmini" || script.Hashtags[1] != "#racuntiktok" {
		t.Fatalf("unexpected normalized hashtags: %v", script.Hashtags)
	}
}
