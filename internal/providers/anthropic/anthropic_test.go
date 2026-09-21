package anthropic

import (
	"testing"

	"contextos/internal/providers"
)

func TestAnthropicTranslateRequest(t *testing.T) {
	p := New(nil)

	req := providers.ProviderRequest{
		Model: "claude-3-7-sonnet",
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "Optimize search algorithm"},
		},
		Compute: providers.ComputePolicy{
			Effort:   providers.EffortHigh,
			Adaptive: false,
		},
	}

	params, err := p.TranslateRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if params.Thinking == nil {
		t.Fatalf("expected non-nil thinking config")
	}
	if params.Thinking.Type != "enabled" {
		t.Fatalf("expected thinking type 'enabled', got %q", params.Thinking.Type)
	}
	if params.Thinking.BudgetTokens != 16384 {
		t.Fatalf("expected 16384 thinking budget tokens, got %d", params.Thinking.BudgetTokens)
	}
}

func TestAnthropicAdaptiveThinking(t *testing.T) {
	p := New(nil)

	req := providers.ProviderRequest{
		Model: "claude-3-7-sonnet",
		Compute: providers.ComputePolicy{
			Adaptive: true,
		},
	}

	params, err := p.TranslateRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params.Thinking == nil || params.Thinking.Type != "adaptive" {
		t.Fatalf("expected thinking type 'adaptive', got %+v", params.Thinking)
	}
}

func TestAnthropicNormalizeUsage(t *testing.T) {
	p := New(nil)

	raw := map[string]any{
		"input_tokens":                float64(2000),
		"output_tokens":               float64(1500),
		"cache_read_input_tokens":     float64(1200),
		"cache_creation_input_tokens": float64(500),
		"thinking_tokens":             float64(800),
	}

	usage := p.NormalizeUsage(raw, "claude-3-7-sonnet")

	if usage.InputTokens != 2000 {
		t.Fatalf("expected 2000 input tokens, got %d", usage.InputTokens)
	}
	if usage.CachedInputTokens != 1200 {
		t.Fatalf("expected 1200 cached input tokens, got %d", usage.CachedInputTokens)
	}
	if usage.CacheWriteTokens != 500 {
		t.Fatalf("expected 500 cache write tokens, got %d", usage.CacheWriteTokens)
	}
	if usage.ReasoningTokens != 800 {
		t.Fatalf("expected 800 reasoning tokens, got %d", usage.ReasoningTokens)
	}
	if usage.VisibleOutputTokens != 700 {
		t.Fatalf("expected 700 visible output tokens, got %d", usage.VisibleOutputTokens)
	}
	if usage.EstimatedCostUSD <= 0 {
		t.Fatalf("expected positive estimated cost, got %f", usage.EstimatedCostUSD)
	}
}
