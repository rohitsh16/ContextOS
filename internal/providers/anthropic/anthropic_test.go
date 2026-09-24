package anthropic

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestAnthropicRealMode(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{
			"id": "msg_123",
			"type": "message",
			"role": "assistant",
			"content": [{"type": "text", "text": "Anthropic response"}],
			"model": "claude-3-7-sonnet-20250219",
			"stop_reason": "end_turn",
			"usage": {
				"input_tokens": 100,
				"output_tokens": 50,
				"thinking_tokens": 20
			}
		}`)
	})

	httpClient := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			return rec.Result(), nil
		}),
	}

	p := New(nil)
	req := providers.ProviderRequest{
		Model: "claude-3-7-sonnet",
		Execution: providers.ExecutionOptions{
			Mode:       providers.ProviderModeReal,
			APIKey:     "test-key",
			Endpoint:   "http://in-memory/v1/messages",
			HTTPClient: httpClient,
		},
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "Hello Claude"},
		},
	}

	resp, err := p.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	if resp.Text != "Anthropic response" {
		t.Errorf("expected 'Anthropic response', got %q", resp.Text)
	}
	if resp.Usage.ReasoningTokens != 20 {
		t.Errorf("expected 20 reasoning tokens, got %d", resp.Usage.ReasoningTokens)
	}
	if resp.ModelVersion != "claude-3-7-sonnet-20250219" {
		t.Errorf("expected version claude-3-7-sonnet-20250219, got %q", resp.ModelVersion)
	}
}
