package anthropic

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

// Provider implements providers.Provider for Anthropic Claude models (e.g. Claude 3.5 Sonnet, 3.7 Sonnet).
type Provider struct {
	registry *providers.Registry
}

// New creates a new Anthropic provider adapter.
func New(registry *providers.Registry) *Provider {
	if registry == nil {
		registry = providers.NewRegistry()
	}
	return &Provider{registry: registry}
}

func (p *Provider) Name() string {
	return "anthropic"
}

func (p *Provider) Capabilities(ctx context.Context, model string) (providers.ModelCapabilities, error) {
	cap, ok := p.registry.Lookup("anthropic", model)
	if !ok {
		return providers.ModelCapabilities{
			Provider:      "anthropic",
			Model:         model,
			ContextWindow: 200000,
			Reasoning: providers.ReasoningCapabilities{
				Supported:                strings.Contains(model, "3-7") || strings.Contains(model, "sonnet"),
				SupportsEffort:           true,
				SupportsThinkingBudget:   true,
				SupportsAdaptiveThinking: strings.Contains(model, "3-7"),
				SupportsThinkingDisable:  true,
				MinThinkingTokens:        1024,
				MaxThinkingTokens:        128000,
			},
			Caching: providers.CacheCapabilities{
				Supported:       true,
				ExplicitCaching: true,
			},
			Tools: providers.ToolCapabilities{
				Supported:     true,
				ParallelCalls: true,
			},
			Usage: providers.UsageCapabilities{
				ReportsReasoningTokens: true,
				ReportsCachedTokens:    true,
				ReportsCacheWrite:      true,
			},
		}, nil
	}
	return cap, nil
}

// ThinkingConfig represents Anthropic's native thinking request payload.
type ThinkingConfig struct {
	Type         string `json:"type"`                    // "enabled", "adaptive", or "disabled"
	BudgetTokens int64  `json:"budget_tokens,omitempty"` // For manual budget
}

// NativeParams represents the translated Anthropic API request parameters.
type NativeParams struct {
	Model       string           `json:"model"`
	MaxTokens   int64            `json:"max_tokens"`
	Messages    []map[string]any `json:"messages"`
	System      string           `json:"system,omitempty"`
	Thinking    *ThinkingConfig  `json:"thinking,omitempty"`
	Tools       []map[string]any `json:"tools,omitempty"`
}

// TranslateRequest maps a provider-neutral ProviderRequest into Anthropic-native parameters.
func (p *Provider) TranslateRequest(req providers.ProviderRequest) (NativeParams, error) {
	caps, err := p.Capabilities(context.Background(), req.Model)
	if err != nil {
		return NativeParams{}, err
	}

	maxTokens := int64(4096)
	if req.Compute.MaxOutputTokens != nil && *req.Compute.MaxOutputTokens > 0 {
		maxTokens = *req.Compute.MaxOutputTokens
	} else if caps.Reasoning.Supported {
		maxTokens = 8192 // Extended output window when thinking is enabled
	}

	params := NativeParams{
		Model:     req.Model,
		MaxTokens: maxTokens,
	}

	// Translate thinking / reasoning policy
	if caps.Reasoning.Supported {
		if caps.Reasoning.SupportsAdaptiveThinking && req.Compute.Adaptive {
			params.Thinking = &ThinkingConfig{Type: "adaptive"}
		} else if req.Compute.Effort == providers.EffortMinimal && caps.Reasoning.SupportsThinkingDisable {
			params.Thinking = &ThinkingConfig{Type: "disabled"}
		} else if caps.Reasoning.SupportsThinkingBudget {
			// Map effort level to token budget if not explicitly provided
			var budget int64
			if req.Compute.MaxReasoningTokens != nil && *req.Compute.MaxReasoningTokens > 0 {
				budget = *req.Compute.MaxReasoningTokens
			} else {
				switch req.Compute.Effort {
				case providers.EffortMinimal:
					budget = caps.Reasoning.MinThinkingTokens
				case providers.EffortLow:
					budget = 2048
				case providers.EffortMedium:
					budget = 8192
				case providers.EffortHigh:
					budget = 16384
				case providers.EffortMaximum:
					budget = 32768
				default:
					budget = 4096
				}
			}
			if budget < caps.Reasoning.MinThinkingTokens {
				budget = caps.Reasoning.MinThinkingTokens
			}
			if budget > caps.Reasoning.MaxThinkingTokens && caps.Reasoning.MaxThinkingTokens > 0 {
				budget = caps.Reasoning.MaxThinkingTokens
			}
			params.Thinking = &ThinkingConfig{
				Type:         "enabled",
				BudgetTokens: budget,
			}
			if params.MaxTokens <= budget {
				params.MaxTokens = budget + 4096
			}
		}
	}

	// Translate messages
	for _, m := range req.Messages {
		if m.Role == providers.RoleSystem {
			params.System = m.Content
			continue
		}
		msg := map[string]any{
			"role":    string(m.Role),
			"content": m.Content,
		}
		params.Messages = append(params.Messages, msg)
	}

	// Translate tools
	for _, t := range req.Tools {
		params.Tools = append(params.Tools, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"input_schema": t.Parameters,
		})
	}

	return params, nil
}

// NormalizeUsage converts Anthropic usage payload into standard telemetry.UsageMetrics.
func (p *Provider) NormalizeUsage(raw map[string]any, model string) telemetry.UsageMetrics {
	var usage telemetry.UsageMetrics

	if inputTokens, ok := raw["input_tokens"].(float64); ok {
		usage.InputTokens = int64(inputTokens)
	}
	if outputTokens, ok := raw["output_tokens"].(float64); ok {
		usage.OutputTokens = int64(outputTokens)
	}
	if cacheRead, ok := raw["cache_read_input_tokens"].(float64); ok {
		usage.CachedInputTokens = int64(cacheRead)
	}
	if cacheWrite, ok := raw["cache_creation_input_tokens"].(float64); ok {
		usage.CacheWriteTokens = int64(cacheWrite)
	}

	// Anthropic thinking tokens are reported inside usage or content blocks
	if thinkingTokens, ok := raw["thinking_tokens"].(float64); ok {
		usage.ReasoningTokens = int64(thinkingTokens)
	} else if reasoningTokens, ok := raw["reasoning_tokens"].(float64); ok {
		usage.ReasoningTokens = int64(reasoningTokens)
	}

	usage.VisibleOutputTokens = usage.OutputTokens - usage.ReasoningTokens
	if usage.VisibleOutputTokens < 0 {
		usage.VisibleOutputTokens = 0
	}

	usage.TotalTokens = usage.InputTokens + usage.OutputTokens

	pricing, _ := telemetry.LookupPricing("anthropic", model)
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

	var reasoningTokens int64 = 0
	if params.Thinking != nil && params.Thinking.BudgetTokens > 0 {
		reasoningTokens = params.Thinking.BudgetTokens / 2
	}

	mockUsage := telemetry.UsageMetrics{
		InputTokens:       int64(len(params.Messages) * 250),
		CachedInputTokens: int64(len(params.Messages) * 125),
		OutputTokens:      reasoningTokens + 400,
		ReasoningTokens:   reasoningTokens,
		TotalTokens:       int64(len(params.Messages)*250) + reasoningTokens + 400,
		Turns:             1,
	}
	pricing, _ := telemetry.LookupPricing("anthropic", req.Model)
	mockUsage.EstimatedCostUSD = telemetry.CalculateUsageCost(pricing, mockUsage)

	return providers.ProviderResponse{
		Text:         fmt.Sprintf("Executed task with Anthropic model %s with thinking config: %+v", req.Model, params.Thinking),
		FinishReason: providers.FinishReasonStop,
		Usage:        mockUsage,
	}, nil
}

func (p *Provider) generateReal(ctx context.Context, req providers.ProviderRequest, params NativeParams) (providers.ProviderResponse, error) {
	apiKey := req.Execution.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if apiKey == "" {
		return providers.ProviderResponse{}, fmt.Errorf("ANTHROPIC_API_KEY is required for ProviderModeReal")
	}

	endpoint := req.Execution.Endpoint
	if endpoint == "" {
		endpoint = "https://api.anthropic.com/v1/messages"
	}

	bodyJSON, err := json.Marshal(params)
	if err != nil {
		return providers.ProviderResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyJSON))
	if err != nil {
		return providers.ProviderResponse{}, err
	}
	httpReq.Header.Set("x-api-key", apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("Content-Type", "application/json")

	startTime := time.Now()
	client := req.Execution.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := client.Do(httpReq)
	latency := time.Since(startTime)
	if err != nil {
		return providers.ProviderResponse{}, fmt.Errorf("anthropic api error: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return providers.ProviderResponse{}, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return providers.ProviderResponse{}, fmt.Errorf("anthropic api returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var respJSON struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content []struct {
			Type     string `json:"type"`
			Text     string `json:"text,omitempty"`
			Thinking string `json:"thinking,omitempty"`
		} `json:"content"`
		Model      string         `json:"model"`
		StopReason string         `json:"stop_reason"`
		Usage      map[string]any `json:"usage"`
	}

	if err := json.Unmarshal(respBody, &respJSON); err != nil {
		return providers.ProviderResponse{}, fmt.Errorf("anthropic decode error: %w", err)
	}

	if respJSON.Usage == nil {
		return providers.ProviderResponse{}, fmt.Errorf("anthropic real response omitted usage; refusing to invent billed telemetry")
	}

	usage := p.NormalizeUsage(respJSON.Usage, req.Model)
	usage.TotalLatencyMS = latency.Milliseconds()
	usage.ModelLatencyMS = latency.Milliseconds()
	usage.RequestedEffort = req.Compute.Effort.String()
	if req.Compute.MaxReasoningTokens != nil {
		usage.RequestedReasoningBudget = *req.Compute.MaxReasoningTokens
	}
	if usage.RequestedReasoningBudget > 0 {
		usage.ReasoningUtilization = float64(usage.ReasoningTokens) / float64(usage.RequestedReasoningBudget)
	}

	var textBuilder strings.Builder
	for _, block := range respJSON.Content {
		if block.Type == "text" {
			textBuilder.WriteString(block.Text)
		}
	}

	return providers.ProviderResponse{
		Text:         textBuilder.String(),
		FinishReason: providers.FinishReasonStop,
		Usage:        usage,
		ModelVersion: respJSON.Model,
		ProviderState: providers.ProviderState{
			RawResponseID: respJSON.ID,
			Metadata: map[string]any{
				"execution_mode": "real",
				"stop_reason":    respJSON.StopReason,
			},
		},
	}, nil
}
