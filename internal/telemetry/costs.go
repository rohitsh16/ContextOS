package telemetry

import (
	"strings"
	"time"
)

// ModelPricing defines per-million token costs in USD for a specific model version.
type ModelPricing struct {
	Provider        string    `json:"provider"`
	Model           string    `json:"model"`
	PricingVersion  string    `json:"pricing_version"`
	EffectiveAt     time.Time `json:"effective_at"`

	InputPerMillion       float64 `json:"input_per_million"`
	CachedInputPerMillion float64 `json:"cached_input_per_million"`
	CacheWritePerMillion  float64 `json:"cache_write_per_million"`
	OutputPerMillion      float64 `json:"output_per_million"`
	ReasoningPerMillion   float64 `json:"reasoning_per_million"` // If billed differently than standard output
}

var defaultPricingCatalog = map[string]ModelPricing{
	// OpenAI models
	"openai:o1": {
		Provider:              "openai",
		Model:                 "o1",
		PricingVersion:        "2024-12-17",
		EffectiveAt:           time.Date(2024, 12, 17, 0, 0, 0, 0, time.UTC),
		InputPerMillion:       15.00,
		CachedInputPerMillion: 7.50,
		OutputPerMillion:      60.00,
		ReasoningPerMillion:   60.00,
	},
	"openai:o3-mini": {
		Provider:              "openai",
		Model:                 "o3-mini",
		PricingVersion:        "2025-01-31",
		EffectiveAt:           time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC),
		InputPerMillion:       1.10,
		CachedInputPerMillion: 0.55,
		OutputPerMillion:      4.40,
		ReasoningPerMillion:   4.40,
	},
	"openai:gpt-4o": {
		Provider:              "openai",
		Model:                 "gpt-4o",
		PricingVersion:        "2024-11-20",
		EffectiveAt:           time.Date(2024, 11, 20, 0, 0, 0, 0, time.UTC),
		InputPerMillion:       2.50,
		CachedInputPerMillion: 1.25,
		OutputPerMillion:      10.00,
		ReasoningPerMillion:   10.00,
	},
	"openai:gpt-4o-mini": {
		Provider:              "openai",
		Model:                 "gpt-4o-mini",
		PricingVersion:        "2024-07-18",
		EffectiveAt:           time.Date(2024, 7, 18, 0, 0, 0, 0, time.UTC),
		InputPerMillion:       0.15,
		CachedInputPerMillion: 0.075,
		OutputPerMillion:      0.60,
		ReasoningPerMillion:   0.60,
	},

	// Anthropic models
	"anthropic:claude-3-7-sonnet": {
		Provider:              "anthropic",
		Model:                 "claude-3-7-sonnet",
		PricingVersion:        "2025-02-24",
		EffectiveAt:           time.Date(2025, 2, 24, 0, 0, 0, 0, time.UTC),
		InputPerMillion:       3.00,
		CachedInputPerMillion: 0.30,
		CacheWritePerMillion:  3.75,
		OutputPerMillion:      15.00,
		ReasoningPerMillion:   15.00, // Claude thinking tokens billed as standard output
	},
	"anthropic:claude-3-5-sonnet": {
		Provider:              "anthropic",
		Model:                 "claude-3-5-sonnet",
		PricingVersion:        "2024-10-22",
		EffectiveAt:           time.Date(2024, 10, 22, 0, 0, 0, 0, time.UTC),
		InputPerMillion:       3.00,
		CachedInputPerMillion: 0.30,
		CacheWritePerMillion:  3.75,
		OutputPerMillion:      15.00,
		ReasoningPerMillion:   15.00,
	},
	"anthropic:claude-3-5-haiku": {
		Provider:              "anthropic",
		Model:                 "claude-3-5-haiku",
		PricingVersion:        "2024-11-04",
		EffectiveAt:           time.Date(2024, 11, 4, 0, 0, 0, 0, time.UTC),
		InputPerMillion:       0.80,
		CachedInputPerMillion: 0.08,
		CacheWritePerMillion:  1.00,
		OutputPerMillion:      4.00,
		ReasoningPerMillion:   4.00,
	},
	"anthropic:claude-3-opus": {
		Provider:              "anthropic",
		Model:                 "claude-3-opus",
		PricingVersion:        "2024-03-01",
		EffectiveAt:           time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
		InputPerMillion:       15.00,
		CachedInputPerMillion: 1.50,
		CacheWritePerMillion:  18.75,
		OutputPerMillion:      75.00,
		ReasoningPerMillion:   75.00,
	},

	// Gemini models
	"gemini:gemini-2.5-pro": {
		Provider:              "gemini",
		Model:                 "gemini-2.5-pro",
		PricingVersion:        "2025-03-01",
		EffectiveAt:           time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
		InputPerMillion:       1.25,
		CachedInputPerMillion: 0.3125,
		OutputPerMillion:      5.00,
		ReasoningPerMillion:   5.00,
	},
	"gemini:gemini-2.5-flash": {
		Provider:              "gemini",
		Model:                 "gemini-2.5-flash",
		PricingVersion:        "2025-03-01",
		EffectiveAt:           time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
		InputPerMillion:       0.075,
		CachedInputPerMillion: 0.01875,
		OutputPerMillion:      0.30,
		ReasoningPerMillion:   0.30,
	},
	"gemini:gemini-2.0-flash": {
		Provider:              "gemini",
		Model:                 "gemini-2.0-flash",
		PricingVersion:        "2025-01-01",
		EffectiveAt:           time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		InputPerMillion:       0.10,
		CachedInputPerMillion: 0.025,
		OutputPerMillion:      0.40,
		ReasoningPerMillion:   0.40,
	},
}

// LookupPricing returns the model pricing for a given provider and model name.
func LookupPricing(provider, model string) (ModelPricing, bool) {
	key := strings.ToLower(provider) + ":" + strings.ToLower(model)
	if p, ok := defaultPricingCatalog[key]; ok {
		return p, true
	}
	// Fallback check by model name alone
	for _, p := range defaultPricingCatalog {
		if strings.EqualFold(p.Model, model) {
			return p, true
		}
	}
	// Default generic fallback
	return ModelPricing{
		Provider:              provider,
		Model:                 model,
		PricingVersion:        "generic-fallback",
		InputPerMillion:       3.00,
		CachedInputPerMillion: 0.75,
		OutputPerMillion:      15.00,
		ReasoningPerMillion:   15.00,
	}, false
}

// CalculateUsageCost computes the total USD cost given a model's pricing and usage metrics.
func CalculateUsageCost(pricing ModelPricing, usage UsageMetrics) float64 {
	uncachedInput := usage.EffectiveInputTokens()
	cachedInput := usage.CachedInputTokens

	inputCost := (float64(uncachedInput) / 1e6) * pricing.InputPerMillion
	cachedCost := (float64(cachedInput) / 1e6) * pricing.CachedInputPerMillion
	cacheWriteCost := (float64(usage.CacheWriteTokens) / 1e6) * pricing.CacheWritePerMillion

	// Output vs reasoning: If reasoning tokens are segregated, bill them via ReasoningPerMillion
	reasoningTokens := usage.ReasoningTokens
	visibleOutputTokens := usage.VisibleOutputTokens
	if visibleOutputTokens == 0 && usage.OutputTokens > 0 {
		visibleOutputTokens = usage.OutputTokens - reasoningTokens
		if visibleOutputTokens < 0 {
			visibleOutputTokens = 0
		}
	}

	outputCost := (float64(visibleOutputTokens) / 1e6) * pricing.OutputPerMillion
	reasoningCost := (float64(reasoningTokens) / 1e6) * pricing.ReasoningPerMillion

	return inputCost + cachedCost + cacheWriteCost + outputCost + reasoningCost
}
