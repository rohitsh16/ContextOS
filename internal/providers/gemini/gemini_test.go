package gemini

import (
	"testing"

	"contextos/internal/providers"
)

func TestGeminiTranslateRequest(t *testing.T) {
	p := New(nil)

	req := providers.ProviderRequest{
		Model: "gemini-2.5-pro",
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "Diagnose latency spike"},
		},
		Compute: providers.ComputePolicy{
			Effort: providers.EffortHigh,
		},
	}

	params, err := p.TranslateRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if params.ThinkingConfig == nil {
		t.Fatalf("expected non-nil thinking config")
	}
	if params.ThinkingConfig.ThinkingBudget != 16384 {
		t.Fatalf("expected 16384 thinking budget, got %d", params.ThinkingConfig.ThinkingBudget)
	}
	if params.ThinkingConfig.ThinkingLevel != "high" {
		t.Fatalf("expected thinking level 'high', got %q", params.ThinkingConfig.ThinkingLevel)
	}
}

func TestGeminiNormalizeUsage(t *testing.T) {
	p := New(nil)

	raw := map[string]any{
		"promptTokenCount":        float64(3000),
		"candidatesTokenCount":    float64(1200),
		"cachedContentTokenCount": float64(1800),
		"thoughtsTokenCount":      float64(600),
		"totalTokenCount":         float64(4200),
	}

	usage := p.NormalizeUsage(raw, "gemini-2.5-pro")

	if usage.InputTokens != 3000 {
		t.Fatalf("expected 3000 input tokens, got %d", usage.InputTokens)
	}
	if usage.CachedInputTokens != 1800 {
		t.Fatalf("expected 1800 cached input tokens, got %d", usage.CachedInputTokens)
	}
	if usage.OutputTokens != 1200 {
		t.Fatalf("expected 1200 output tokens, got %d", usage.OutputTokens)
	}
	if usage.ReasoningTokens != 600 {
		t.Fatalf("expected 600 reasoning tokens, got %d", usage.ReasoningTokens)
	}
	if usage.VisibleOutputTokens != 600 {
		t.Fatalf("expected 600 visible output tokens, got %d", usage.VisibleOutputTokens)
	}
	if usage.EstimatedCostUSD <= 0 {
		t.Fatalf("expected positive estimated cost, got %f", usage.EstimatedCostUSD)
	}
}
