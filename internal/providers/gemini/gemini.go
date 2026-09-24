package gemini

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

func (p *Provider) generateReal(ctx context.Context, req providers.ProviderRequest, params NativeParams) (providers.ProviderResponse, error) {
	apiKey := req.Execution.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}
	if apiKey == "" {
		apiKey = os.Getenv("GOOGLE_API_KEY")
	}
	if apiKey == "" {
		return providers.ProviderResponse{}, fmt.Errorf("GEMINI_API_KEY is required for ProviderModeReal")
	}

	endpoint := req.Execution.Endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", req.Model, apiKey)
	}

	bodyMap := map[string]any{
		"contents": params.Contents,
	}
	if params.ThinkingConfig != nil {
		bodyMap["generationConfig"] = map[string]any{
			"thinkingConfig": params.ThinkingConfig,
		}
	}

	bodyJSON, err := json.Marshal(bodyMap)
	if err != nil {
		return providers.ProviderResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(bodyJSON))
	if err != nil {
		return providers.ProviderResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	startTime := time.Now()
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	latency := time.Since(startTime)
	if err != nil {
		return providers.ProviderResponse{}, fmt.Errorf("gemini api error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return providers.ProviderResponse{}, fmt.Errorf("gemini api returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var respJSON struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		UsageMetadata map[string]any `json:"usageMetadata"`
		ModelVersion  string         `json:"modelVersion"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&respJSON); err != nil {
		return providers.ProviderResponse{}, fmt.Errorf("gemini decode response error: %w", err)
	}

	usage := p.NormalizeUsage(respJSON.UsageMetadata, req.Model)
	usage.TotalLatencyMS = latency.Milliseconds()
	usage.ModelLatencyMS = latency.Milliseconds()

	var text string
	if len(respJSON.Candidates) > 0 && len(respJSON.Candidates[0].Content.Parts) > 0 {
		text = respJSON.Candidates[0].Content.Parts[0].Text
	}

	return providers.ProviderResponse{
		Text:         text,
		FinishReason: providers.FinishReasonStop,
		Usage:        usage,
	}, nil
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
