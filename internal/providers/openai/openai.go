package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"contextos/internal/providers"
	"contextos/internal/telemetry"
)

// Provider implements providers.Provider for OpenAI reasoning and chat completion models.
type Provider struct {
	registry *providers.Registry
}

// New creates a new OpenAI provider adapter.
func New(registry *providers.Registry) *Provider {
	if registry == nil {
		registry = providers.NewRegistry()
	}
	return &Provider{registry: registry}
}

func (p *Provider) Name() string {
	return "openai"
}

func (p *Provider) Capabilities(ctx context.Context, model string) (providers.ModelCapabilities, error) {
	cap, ok := p.registry.Lookup("openai", model)
	if !ok {
		// Default generic OpenAI capabilities
		return providers.ModelCapabilities{
			Provider:      "openai",
			Model:         model,
			ContextWindow: 128000,
			Reasoning: providers.ReasoningCapabilities{
				Supported:      strings.HasPrefix(model, "o1") || strings.HasPrefix(model, "o3"),
				SupportsEffort: strings.HasPrefix(model, "o1") || strings.HasPrefix(model, "o3"),
				EffortLevels:   []providers.EffortLevel{providers.EffortLow, providers.EffortMedium, providers.EffortHigh},
			},
			Caching: providers.CacheCapabilities{
				Supported:        true,
				AutomaticCaching: true,
			},
			Tools: providers.ToolCapabilities{
				Supported:     true,
				ParallelCalls: true,
			},
			Usage: providers.UsageCapabilities{
				ReportsReasoningTokens: true,
				ReportsCachedTokens:    true,
			},
		}, nil
	}
	return cap, nil
}

// NativeParams represents the translated OpenAI API request parameters.
type NativeParams struct {
	Model               string           `json:"model"`
	Messages            []map[string]any `json:"messages"`
	ReasoningEffort     string           `json:"reasoning_effort,omitempty"`
	MaxCompletionTokens *int64           `json:"max_completion_tokens,omitempty"`
	Tools               []map[string]any `json:"tools,omitempty"`
}

// TranslateRequest maps a provider-neutral ProviderRequest into OpenAI-native parameters.
func (p *Provider) TranslateRequest(req providers.ProviderRequest) (NativeParams, error) {
	caps, err := p.Capabilities(context.Background(), req.Model)
	if err != nil {
		return NativeParams{}, err
	}

	params := NativeParams{
		Model: req.Model,
	}

	if req.Compute.MaxOutputTokens != nil {
		params.MaxCompletionTokens = req.Compute.MaxOutputTokens
	}

	// Translate reasoning effort if model supports it
	if caps.Reasoning.SupportsEffort {
		switch req.Compute.Effort {
		case providers.EffortMinimal, providers.EffortLow:
			params.ReasoningEffort = "low"
		case providers.EffortMedium:
			params.ReasoningEffort = "medium"
		case providers.EffortHigh, providers.EffortMaximum:
			params.ReasoningEffort = "high"
		default:
			params.ReasoningEffort = "medium"
		}
	}

	// Translate messages
	for _, m := range req.Messages {
		msg := map[string]any{
			"role":    string(m.Role),
			"content": m.Content,
		}
		if m.Name != "" {
			msg["name"] = m.Name
		}
		if m.ToolCallID != "" {
			msg["tool_call_id"] = m.ToolCallID
		}
		params.Messages = append(params.Messages, msg)
	}

	// Translate tools
	for _, t := range req.Tools {
		params.Tools = append(params.Tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.Parameters,
			},
		})
	}

	return params, nil
}

// NormalizeUsage converts OpenAI-specific usage JSON/payload into standard telemetry.UsageMetrics.
func (p *Provider) NormalizeUsage(raw map[string]any, model string) telemetry.UsageMetrics {
	var usage telemetry.UsageMetrics

	if promptTokens, ok := raw["prompt_tokens"].(float64); ok {
		usage.InputTokens = int64(promptTokens)
	}
	if completionTokens, ok := raw["completion_tokens"].(float64); ok {
		usage.OutputTokens = int64(completionTokens)
	}
	if totalTokens, ok := raw["total_tokens"].(float64); ok {
		usage.TotalTokens = int64(totalTokens)
	}

	// Reasoning tokens
	if cDetails, ok := raw["completion_tokens_details"].(map[string]any); ok {
		if rTokens, ok := cDetails["reasoning_tokens"].(float64); ok {
			usage.ReasoningTokens = int64(rTokens)
			usage.VisibleOutputTokens = usage.OutputTokens - usage.ReasoningTokens
			if usage.VisibleOutputTokens < 0 {
				usage.VisibleOutputTokens = 0
			}
		}
	}

	// Prompt cached tokens
	if pDetails, ok := raw["prompt_tokens_details"].(map[string]any); ok {
		if cTokens, ok := pDetails["cached_tokens"].(float64); ok {
			usage.CachedInputTokens = int64(cTokens)
		}
	}

	// Calculate cost
	pricing, _ := telemetry.LookupPricing("openai", model)
	usage.EstimatedCostUSD = telemetry.CalculateUsageCost(pricing, usage)

	return usage
}

// Generate implements providers.Provider. In offline/mock mode it synthesizes response with normalized metrics.
func (p *Provider) Generate(ctx context.Context, req providers.ProviderRequest) (providers.ProviderResponse, error) {
	params, err := p.TranslateRequest(req)
	if err != nil {
		return providers.ProviderResponse{}, err
	}
	if req.Execution.Mode == providers.ProviderModeReal {
		return p.generateReal(ctx, req, params)
	}

	// If a synthetic execution or mock is needed:
	mockUsage := telemetry.UsageMetrics{
		InputTokens:       int64(len(params.Messages) * 200),
		CachedInputTokens: int64(len(params.Messages) * 100),
		OutputTokens:      350,
		ReasoningTokens:   250,
		TotalTokens:       int64(len(params.Messages)*200 + 350),
		Turns:             1,
	}
	pricing, _ := telemetry.LookupPricing("openai", req.Model)
	mockUsage.EstimatedCostUSD = telemetry.CalculateUsageCost(pricing, mockUsage)

	return providers.ProviderResponse{
		Text:         fmt.Sprintf("Executed task with OpenAI model %s at reasoning_effort: %s", req.Model, params.ReasoningEffort),
		FinishReason: providers.FinishReasonStop,
		Usage:        mockUsage,
	}, nil
}

// generateReal is deliberately narrow: it uses the Chat Completions-compatible
// endpoint and rejects missing usage instead of fabricating telemetry.
func (p *Provider) generateReal(ctx context.Context, req providers.ProviderRequest, params NativeParams) (providers.ProviderResponse, error) {
	key := req.Execution.APIKey
	if key == "" {
		key = os.Getenv("OPENAI_API_KEY")
	}
	if key == "" {
		return providers.ProviderResponse{}, fmt.Errorf("openai real mode requires OPENAI_API_KEY")
	}
	endpoint := req.Execution.Endpoint
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/chat/completions"
	}
	body, err := json.Marshal(params)
	if err != nil {
		return providers.ProviderResponse{}, err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return providers.ProviderResponse{}, err
	}
	hreq.Header.Set("Authorization", "Bearer "+key)
	hreq.Header.Set("Content-Type", "application/json")
	started := time.Now()
	hresp, err := http.DefaultClient.Do(hreq)
	if err != nil {
		return providers.ProviderResponse{}, fmt.Errorf("openai real request: %w", err)
	}
	defer hresp.Body.Close()
	rawBody, err := io.ReadAll(hresp.Body)
	if err != nil {
		return providers.ProviderResponse{}, err
	}
	if hresp.StatusCode < 200 || hresp.StatusCode >= 300 {
		return providers.ProviderResponse{}, fmt.Errorf("openai real request returned %s: %s", hresp.Status, string(rawBody))
	}
	var raw map[string]any
	if err := json.Unmarshal(rawBody, &raw); err != nil {
		return providers.ProviderResponse{}, err
	}
	usageRaw, ok := raw["usage"].(map[string]any)
	if !ok {
		return providers.ProviderResponse{}, fmt.Errorf("openai real response omitted usage; refusing to invent billed telemetry")
	}
	usage := p.NormalizeUsage(usageRaw, req.Model)
	usage.ModelLatencyMS = time.Since(started).Milliseconds()
	usage.TotalLatencyMS = usage.ModelLatencyMS
	usage.RequestedEffort = req.Compute.Effort.String()
	if req.Compute.MaxReasoningTokens != nil {
		usage.RequestedReasoningBudget = *req.Compute.MaxReasoningTokens
	}
	if usage.RequestedReasoningBudget > 0 {
		usage.ReasoningUtilization = float64(usage.ReasoningTokens) / float64(usage.RequestedReasoningBudget)
	}
	text := ""
	if choices, ok := raw["choices"].([]any); ok && len(choices) > 0 {
		if c, ok := choices[0].(map[string]any); ok {
			if msg, ok := c["message"].(map[string]any); ok {
				text, _ = msg["content"].(string)
			}
		}
	}
	id, _ := raw["id"].(string)
	fingerprint, _ := raw["system_fingerprint"].(string)
	return providers.ProviderResponse{Text: text, Usage: usage, FinishReason: providers.FinishReasonStop, ModelVersion: req.Execution.ModelVersion, ProviderState: providers.ProviderState{RawResponseID: id, SystemFingerprint: fingerprint, Metadata: map[string]any{"execution_mode": "real"}}}, nil
}
