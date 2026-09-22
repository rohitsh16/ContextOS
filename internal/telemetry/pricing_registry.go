package telemetry

import (
	"fmt"
	"sync"
	"time"
)

// PricingEntry defines a pinned, immutable pricing schedule for a provider model version.
type PricingEntry struct {
	Provider    string    `json:"provider"`
	Model       string    `json:"model"`
	EffectiveAt time.Time `json:"effective_at"`
	Source      string    `json:"source"`
	Version     string    `json:"version"`

	InputPerMillion       float64 `json:"input_per_million"`
	CachedInputPerMillion float64 `json:"cached_input_per_million"`
	CacheWritePerMillion  float64 `json:"cache_write_per_million"`
	OutputPerMillion      float64 `json:"output_per_million"`
	ReasoningPerMillion   float64 `json:"reasoning_per_million"`
}

// PricingRegistry manages pinned model pricing schedules across providers and versions.
type PricingRegistry struct {
	mu      sync.RWMutex
	entries map[string]PricingEntry // Key: "provider:model@version" or "provider:model"
}

var (
	globalPricingRegistry     *PricingRegistry
	globalPricingRegistryOnce sync.Once
)

// DefaultPricingRegistry returns the global singleton pricing registry initialized with standard catalogs.
func DefaultPricingRegistry() *PricingRegistry {
	globalPricingRegistryOnce.Do(func() {
		globalPricingRegistry = NewPricingRegistry()
		globalPricingRegistry.populateDefaults()
	})
	return globalPricingRegistry
}

// NewPricingRegistry initializes an empty pricing registry.
func NewPricingRegistry() *PricingRegistry {
	return &PricingRegistry{
		entries: make(map[string]PricingEntry),
	}
}

// Register adds or updates a pinned pricing schedule.
func (pr *PricingRegistry) Register(entry PricingEntry) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	versionKey := fmt.Sprintf("%s:%s@%s", entry.Provider, entry.Model, entry.Version)
	latestKey := fmt.Sprintf("%s:%s", entry.Provider, entry.Model)

	pr.entries[versionKey] = entry

	// Update latest if newer or absent
	if existing, exists := pr.entries[latestKey]; !exists || entry.EffectiveAt.After(existing.EffectiveAt) {
		pr.entries[latestKey] = entry
	}
}

// LookupVersion retrieves an exact pinned pricing version.
func (pr *PricingRegistry) LookupVersion(provider, model, version string) (PricingEntry, error) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	key := fmt.Sprintf("%s:%s@%s", provider, model, version)
	if entry, ok := pr.entries[key]; ok {
		return entry, nil
	}
	return PricingEntry{}, fmt.Errorf("pricing not found for %s:%s version %s", provider, model, version)
}

// LookupLatest retrieves the most recent pricing entry for a provider model.
func (pr *PricingRegistry) LookupLatest(provider, model string) (PricingEntry, error) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", provider, model)
	if entry, ok := pr.entries[key]; ok {
		return entry, nil
	}
	return PricingEntry{}, fmt.Errorf("pricing not found for %s:%s", provider, model)
}

func (pr *PricingRegistry) populateDefaults() {
	defaults := []PricingEntry{
		// OpenAI
		{
			Provider:              "openai",
			Model:                 "o1",
			Version:               "2024-12-17",
			EffectiveAt:           time.Date(2024, 12, 17, 0, 0, 0, 0, time.UTC),
			Source:                "https://openai.com/pricing",
			InputPerMillion:       15.00,
			CachedInputPerMillion: 7.50,
			OutputPerMillion:      60.00,
			ReasoningPerMillion:   60.00,
		},
		{
			Provider:              "openai",
			Model:                 "o3-mini",
			Version:               "2025-01-31",
			EffectiveAt:           time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC),
			Source:                "https://openai.com/pricing",
			InputPerMillion:       1.10,
			CachedInputPerMillion: 0.55,
			OutputPerMillion:      4.40,
			ReasoningPerMillion:   4.40,
		},
		{
			Provider:              "openai",
			Model:                 "gpt-4o",
			Version:               "2024-11-20",
			EffectiveAt:           time.Date(2024, 11, 20, 0, 0, 0, 0, time.UTC),
			Source:                "https://openai.com/pricing",
			InputPerMillion:       2.50,
			CachedInputPerMillion: 1.25,
			OutputPerMillion:      10.00,
			ReasoningPerMillion:   10.00,
		},
		{
			Provider:              "openai",
			Model:                 "gpt-4o-mini",
			Version:               "2024-07-18",
			EffectiveAt:           time.Date(2024, 7, 18, 0, 0, 0, 0, time.UTC),
			Source:                "https://openai.com/pricing",
			InputPerMillion:       0.15,
			CachedInputPerMillion: 0.075,
			OutputPerMillion:      0.60,
			ReasoningPerMillion:   0.60,
		},

		// Anthropic
		{
			Provider:              "anthropic",
			Model:                 "claude-3-7-sonnet",
			Version:               "2025-02-24",
			EffectiveAt:           time.Date(2025, 2, 24, 0, 0, 0, 0, time.UTC),
			Source:                "https://anthropic.com/pricing",
			InputPerMillion:       3.00,
			CachedInputPerMillion: 0.30,
			CacheWritePerMillion:  3.75,
			OutputPerMillion:      15.00,
			ReasoningPerMillion:   15.00,
		},
		{
			Provider:              "anthropic",
			Model:                 "claude-3-5-sonnet",
			Version:               "2024-10-22",
			EffectiveAt:           time.Date(2024, 10, 22, 0, 0, 0, 0, time.UTC),
			Source:                "https://anthropic.com/pricing",
			InputPerMillion:       3.00,
			CachedInputPerMillion: 0.30,
			CacheWritePerMillion:  3.75,
			OutputPerMillion:      15.00,
			ReasoningPerMillion:   15.00,
		},
		{
			Provider:              "anthropic",
			Model:                 "claude-3-5-haiku",
			Version:               "2024-10-22",
			EffectiveAt:           time.Date(2024, 10, 22, 0, 0, 0, 0, time.UTC),
			Source:                "https://anthropic.com/pricing",
			InputPerMillion:       0.80,
			CachedInputPerMillion: 0.08,
			CacheWritePerMillion:  1.00,
			OutputPerMillion:      4.00,
			ReasoningPerMillion:   4.00,
		},

		// Google Gemini
		{
			Provider:              "gemini",
			Model:                 "gemini-2.0-flash-thinking",
			Version:               "2025-01-20",
			EffectiveAt:           time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC),
			Source:                "https://ai.google.dev/pricing",
			InputPerMillion:       0.10,
			CachedInputPerMillion: 0.025,
			OutputPerMillion:      0.40,
			ReasoningPerMillion:   0.40,
		},
		{
			Provider:              "gemini",
			Model:                 "gemini-2.0-flash",
			Version:               "2025-02-05",
			EffectiveAt:           time.Date(2025, 2, 5, 0, 0, 0, 0, time.UTC),
			Source:                "https://ai.google.dev/pricing",
			InputPerMillion:       0.10,
			CachedInputPerMillion: 0.025,
			OutputPerMillion:      0.40,
			ReasoningPerMillion:   0.40,
		},
		{
			Provider:              "gemini",
			Model:                 "gemini-2.5-flash",
			Version:               "2025-03-01",
			EffectiveAt:           time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
			Source:                "https://ai.google.dev/pricing",
			InputPerMillion:       0.075,
			CachedInputPerMillion: 0.01875,
			OutputPerMillion:      0.30,
			ReasoningPerMillion:   0.30,
		},
		{
			Provider:              "gemini",
			Model:                 "gemini-2.5-pro",
			Version:               "2025-03-01",
			EffectiveAt:           time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
			Source:                "https://ai.google.dev/pricing",
			InputPerMillion:       1.25,
			CachedInputPerMillion: 0.3125,
			OutputPerMillion:      5.00,
			ReasoningPerMillion:   5.00,
		},
	}

	for _, d := range defaults {
		pr.Register(d)
	}
}
