package gemini

import (
	"context"
	"fmt"
	"strings"

	"contextos/internal/providers"
	"contextos/internal/telemetry"
)

// Provider implements providers.Provider for Google Gemini models (e.g. Gemini 2.5 Pro, Flash, 2.0 Flash Thinking).
type Provider struct {
	registry *providers.Registry
}

// New creates a new Gemini provider adapter.
func New(registry *providers.Registry) *Provider {
	if registry == nil {
		registry = providers.NewRegistry()
	}
	return &Provider{registry: registry}
}

func (p *Provider) Name() string {
	return "gemini"
}

func (p *Provider) Capabilities(ctx context.Context, model string) (providers.ModelCapabilities, error) {
	cap, ok := p.registry.Lookup("gemini", model)
	if !ok {
		return providers.ModelCapabilities{
			Provider:      "gemini",
			Model:         model,
			ContextWindow: 1000000,
			Reasoning: providers.ReasoningCapabilities{
				Supported:                strings.Contains(model, "2.5") || strings.Contains(model, "thinking"),
				SupportsEffort:           true,
				SupportsThinkingBudget:   true,
				SupportsAdaptiveThinking: true,
				SupportsThinkingDisable:  true,
				MinThinkingTokens:        1,
				MaxThinkingTokens:        64000,
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

// ThinkingConfig represents Gemini's thinking configuration parameters.
type ThinkingConfig struct {
	ThinkingBudget int64  `json:"thinkingBudget,omitempty"`
	ThinkingLevel  string `json:"thinkingLevel,omitempty"`
}

// NativeParams represents the translated Gemini API request parameters.
type NativeParams struct {
	Model          string          `json:"model"`
	Contents       []any           `json:"contents"`
	ThinkingConfig *ThinkingConfig `json:"thinkingConfig,omitempty"`
	Tools          []any           `json:"tools,omitempty"`
}

// TranslateRequest maps a provider-neutral ProviderRequest into Gemini-native parameters.
func (p *Provider) TranslateRequest(req providers.ProviderRequest) (NativeParams, error) {
	caps, err := p.Capabilities(context.Background(), req.Model)
	if err != nil {
		return NativeParams{}, err
	}

	params := NativeParams{
		Model: req.Model,
	}

	if caps.Reasoning.Supported {
		var budget int64
		if req.Compute.MaxReasoningTokens != nil && *req.Compute.MaxReasoningTokens > 0 {
			budget = *req.Compute.MaxReasoningTokens
		} else {
			switch req.Compute.Effort {
			case providers.EffortMinimal:
				budget = 0 // Thinking disabled or lowest level
			case providers.EffortLow:
				budget = 1024
			case providers.EffortMedium:
				budget = 4096
			case providers.EffortHigh:
				budget = 16384
			case providers.EffortMaximum:
				budget = 32768
			default:
				budget = 4096
			}
		}

		if budget == 0 && caps.Reasoning.SupportsThinkingDisable {
			params.ThinkingConfig = &ThinkingConfig{
				ThinkingBudget: 0,
				ThinkingLevel:  "low",
			}
		} else {
			params.ThinkingConfig = &ThinkingConfig{
				ThinkingBudget: budget,
				ThinkingLevel:  req.Compute.Effort.String(),
			}
		}
	}

	// Translate messages to Gemini contents
	for _, m := range req.Messages {
		role := "user"
		if m.Role == providers.RoleAssistant {
			role = "model"
		}
		params.Contents = append(params.Contents, map[string]any{
			"role": role,
			"parts": []map[string]any{
				{"text": m.Content},
			},
		})
	}

	return params, nil
}

// NormalizeUsage converts Gemini usageMetadata into standard telemetry.UsageMetrics.
func (p *Provider) NormalizeUsage(raw map[string]any, model string) telemetry.UsageMetrics {
	var usage telemetry.UsageMetrics

	if promptTokens, ok := raw["promptTokenCount"].(float64); ok {
		usage.InputTokens = int64(promptTokens)
	}
	if candidatesTokens, ok := raw["candidatesTokenCount"].(float64); ok {
		usage.OutputTokens = int64(candidatesTokens)
	}
	if totalTokens, ok := raw["totalTokenCount"].(float64); ok {
		usage.TotalTokens = int64(totalTokens)
	}
	if cachedTokens, ok := raw["cachedContentTokenCount"].(float64); ok {
		usage.CachedInputTokens = int64(cachedTokens)
	}

	// Thoughts token count in Gemini
	if thoughtsTokens, ok := raw["thoughtsTokenCount"].(float64); ok {
		usage.ReasoningTokens = int64(thoughtsTokens)
	}

	usage.VisibleOutputTokens = usage.OutputTokens - usage.ReasoningTokens
	if usage.VisibleOutputTokens < 0 {
		usage.VisibleOutputTokens = 0
	}

	pricing, _ := telemetry.LookupPricing("gemini", model)
	usage.EstimatedCostUSD = telemetry.CalculateUsageCost(pricing, usage)

	return usage
}

// Generate implements providers.Provider. In offline/mock mode it synthesizes response with normalized metrics.
func (p *Provider) Generate(ctx context.Context, req providers.ProviderRequest) (providers.ProviderResponse, error) {
	params, err := p.TranslateRequest(req)
	if err != nil {
		return providers.ProviderResponse{}, err
	}

	var reasoningTokens int64 = 0
	if params.ThinkingConfig != nil && params.ThinkingConfig.ThinkingBudget > 0 {
		reasoningTokens = params.ThinkingConfig.ThinkingBudget / 2
	}

	mockUsage := telemetry.UsageMetrics{
		InputTokens:       int64(len(params.Contents) * 200),
		CachedInputTokens: int64(len(params.Contents) * 100),
		OutputTokens:      reasoningTokens + 300,
		ReasoningTokens:   reasoningTokens,
		TotalTokens:       int64(len(params.Contents)*200) + reasoningTokens + 300,
		Turns:             1,
	}
	pricing, _ := telemetry.LookupPricing("gemini", req.Model)
	mockUsage.EstimatedCostUSD = telemetry.CalculateUsageCost(pricing, mockUsage)

	return providers.ProviderResponse{
		Text:         fmt.Sprintf("Executed task with Gemini model %s with thinking config: %+v", req.Model, params.ThinkingConfig),
		FinishReason: providers.FinishReasonStop,
		Usage:        mockUsage,
	}, nil
}
