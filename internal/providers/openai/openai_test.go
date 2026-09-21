package openai

import (
	"context"
	"testing"

	"contextos/internal/providers"
)

func TestOpenAITranslateRequest(t *testing.T) {
	reg := providers.NewRegistry()
	p := New(reg)

	req := providers.ProviderRequest{
		Model: "o3-mini",
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "Refactor auth middleware"},
		},
		Compute: providers.ComputePolicy{
			Effort: providers.EffortHigh,
		},
	}

	params, err := p.TranslateRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if params.ReasoningEffort != "high" {
		t.Fatalf("expected reasoning_effort 'high', got %q", params.ReasoningEffort)
	}
	if len(params.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(params.Messages))
	}
}

func TestOpenAINormalizeUsage(t *testing.T) {
	p := New(nil)

	raw := map[string]any{
		"prompt_tokens":     float64(1000),
		"completion_tokens": float64(500),
		"total_tokens":      float64(1500),
		"completion_tokens_details": map[string]any{
			"reasoning_tokens": float64(300),
		},
		"prompt_tokens_details": map[string]any{
			"cached_tokens": float64(400),
		},
	}

	usage := p.NormalizeUsage(raw, "o3-mini")

	if usage.InputTokens != 1000 {
		t.Fatalf("expected 1000 input tokens, got %d", usage.InputTokens)
	}
	if usage.CachedInputTokens != 400 {
		t.Fatalf("expected 400 cached input tokens, got %d", usage.CachedInputTokens)
	}
	if usage.OutputTokens != 500 {
		t.Fatalf("expected 500 output tokens, got %d", usage.OutputTokens)
	}
	if usage.ReasoningTokens != 300 {
		t.Fatalf("expected 300 reasoning tokens, got %d", usage.ReasoningTokens)
	}
	if usage.VisibleOutputTokens != 200 {
		t.Fatalf("expected 200 visible output tokens, got %d", usage.VisibleOutputTokens)
	}
	if usage.EstimatedCostUSD <= 0 {
		t.Fatalf("expected positive estimated cost, got %f", usage.EstimatedCostUSD)
	}
}

func TestOpenAIGenerate(t *testing.T) {
	p := New(nil)
	req := providers.ProviderRequest{
		Model: "o1",
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "Debug race condition"},
		},
		Compute: providers.ComputePolicy{
			Effort: providers.EffortMedium,
		},
	}

	resp, err := p.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.FinishReason != providers.FinishReasonStop {
		t.Fatalf("expected finish reason stop, got %v", resp.FinishReason)
	}
	if resp.Usage.ReasoningTokens != 250 {
		t.Fatalf("expected 250 reasoning tokens, got %d", resp.Usage.ReasoningTokens)
	}
}
