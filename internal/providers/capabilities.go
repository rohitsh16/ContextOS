package providers

import (
	"sync"
	"time"
)

// EffortLevel defines the normalized reasoning effort levels supported across models.
type EffortLevel int

const (
	EffortMinimal EffortLevel = iota // T0 deterministic / lowest compute
	EffortLow                        // T1 trivial
	EffortMedium                     // T2 moderate
	EffortHigh                       // T3 difficult
	EffortMaximum                    // T4 critical / research
)

func (e EffortLevel) String() string {
	switch e {
	case EffortMinimal:
		return "minimal"
	case EffortLow:
		return "low"
	case EffortMedium:
		return "medium"
	case EffortHigh:
		return "high"
	case EffortMaximum:
		return "maximum"
	default:
		return "medium"
	}
}

// ReasoningCapabilities defines model-specific support for extended thinking / reasoning.
type ReasoningCapabilities struct {
	Supported                bool          `json:"supported"`
	SupportsEffort           bool          `json:"supports_effort"`
	SupportsThinkingBudget   bool          `json:"supports_thinking_budget"`
	SupportsAdaptiveThinking bool          `json:"supports_adaptive_thinking"`
	SupportsThinkingDisable  bool          `json:"supports_thinking_disable"`
	SupportsPerTurnEffort    bool          `json:"supports_per_turn_effort"`
	EffortLevels             []EffortLevel `json:"effort_levels"`
	MinThinkingTokens        int64         `json:"min_thinking_tokens"`
	MaxThinkingTokens        int64         `json:"max_thinking_tokens"`
}

// CacheCapabilities defines prompt caching support and cache-invalidation characteristics.
type CacheCapabilities struct {
	Supported                   bool          `json:"supported"`
	ExplicitCaching             bool          `json:"explicit_caching"`
	AutomaticCaching            bool          `json:"automatic_caching"`
	CacheTTL                    time.Duration `json:"cache_ttl"`
	EffortChangesBreakCache     bool          `json:"effort_changes_break_cache"`
	PerTurnEffortPreservesCache bool          `json:"per_turn_effort_preserves_cache"`
}

// ToolCapabilities defines tool invocation characteristics.
type ToolCapabilities struct {
	Supported      bool `json:"supported"`
	ParallelCalls  bool `json:"parallel_calls"`
	StructuredArgs bool `json:"structured_args"`
}

// UsageCapabilities defines what usage metrics the provider actually reports.
type UsageCapabilities struct {
	ReportsReasoningTokens bool `json:"reports_reasoning_tokens"`
	ReportsCachedTokens    bool `json:"reports_cached_tokens"`
	ReportsCacheWrite      bool `json:"reports_cache_write"`
}

// ModelCapabilities encapsulates all capabilities of a specific provider/model.
type ModelCapabilities struct {
	Provider      string                `json:"provider"`
	Model         string                `json:"model"`
	ContextWindow int64                 `json:"context_window"`
	Reasoning     ReasoningCapabilities `json:"reasoning"`
	Caching       CacheCapabilities     `json:"caching"`
	Tools         ToolCapabilities      `json:"tools"`
	Usage         UsageCapabilities     `json:"usage"`
}

// Registry stores immutable capability snapshots for reproducibility.
type Registry struct {
	mu     sync.RWMutex
	models map[string]ModelCapabilities
}

// NewRegistry creates a capability registry populated with known default models.
func NewRegistry() *Registry {
	r := &Registry{
		models: make(map[string]ModelCapabilities),
	}
	r.registerDefaults()
	return r
}

func (r *Registry) Register(cap ModelCapabilities) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := cap.Provider + ":" + cap.Model
	r.models[key] = cap
}

func (r *Registry) Lookup(provider, model string) (ModelCapabilities, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key := provider + ":" + model
	cap, ok := r.models[key]
	return cap, ok
}

// Snapshot returns a frozen copy of all registered capabilities.
func (r *Registry) Snapshot() map[string]ModelCapabilities {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]ModelCapabilities, len(r.models))
	for k, v := range r.models {
		out[k] = v
	}
	return out
}

func (r *Registry) registerDefaults() {
	// OpenAI o1
	r.Register(ModelCapabilities{
		Provider:      "openai",
		Model:         "o1",
		ContextWindow: 200000,
		Reasoning: ReasoningCapabilities{
			Supported:               true,
			SupportsEffort:          true,
			SupportsThinkingBudget:  false,
			SupportsThinkingDisable: false,
			SupportsPerTurnEffort:   true,
			EffortLevels:            []EffortLevel{EffortLow, EffortMedium, EffortHigh},
			MinThinkingTokens:       0,
			MaxThinkingTokens:       100000,
		},
		Caching: CacheCapabilities{
			Supported:                   true,
			AutomaticCaching:            true,
			CacheTTL:                    time.Hour,
			EffortChangesBreakCache:     false,
			PerTurnEffortPreservesCache: true,
		},
		Tools: ToolCapabilities{
			Supported:      true,
			ParallelCalls:  true,
			StructuredArgs: true,
		},
		Usage: UsageCapabilities{
			ReportsReasoningTokens: true,
			ReportsCachedTokens:    true,
		},
	})

	// OpenAI o3-mini
	r.Register(ModelCapabilities{
		Provider:      "openai",
		Model:         "o3-mini",
		ContextWindow: 200000,
		Reasoning: ReasoningCapabilities{
			Supported:               true,
			SupportsEffort:          true,
			SupportsThinkingBudget:  false,
			SupportsThinkingDisable: false,
			SupportsPerTurnEffort:   true,
			EffortLevels:            []EffortLevel{EffortLow, EffortMedium, EffortHigh},
			MinThinkingTokens:       0,
			MaxThinkingTokens:       100000,
		},
		Caching: CacheCapabilities{
			Supported:                   true,
			AutomaticCaching:            true,
			CacheTTL:                    time.Hour,
			EffortChangesBreakCache:     false,
			PerTurnEffortPreservesCache: true,
		},
		Tools: ToolCapabilities{
			Supported:      true,
			ParallelCalls:  true,
			StructuredArgs: true,
		},
		Usage: UsageCapabilities{
			ReportsReasoningTokens: true,
			ReportsCachedTokens:    true,
		},
	})

	// Anthropic Claude 3.7 Sonnet
	r.Register(ModelCapabilities{
		Provider:      "anthropic",
		Model:         "claude-3-7-sonnet",
		ContextWindow: 200000,
		Reasoning: ReasoningCapabilities{
			Supported:                true,
			SupportsEffort:           true,
			SupportsThinkingBudget:   true,
			SupportsAdaptiveThinking: true,
			SupportsThinkingDisable:  true,
			SupportsPerTurnEffort:    true,
			EffortLevels:             []EffortLevel{EffortMinimal, EffortLow, EffortMedium, EffortHigh, EffortMaximum},
			MinThinkingTokens:        1024,
			MaxThinkingTokens:        128000,
		},
		Caching: CacheCapabilities{
			Supported:                   true,
			ExplicitCaching:             true,
			CacheTTL:                    5 * time.Minute,
			EffortChangesBreakCache:     false,
			PerTurnEffortPreservesCache: true,
		},
		Tools: ToolCapabilities{
			Supported:      true,
			ParallelCalls:  true,
			StructuredArgs: true,
		},
		Usage: UsageCapabilities{
			ReportsReasoningTokens: true,
			ReportsCachedTokens:    true,
			ReportsCacheWrite:      true,
		},
	})

	// Gemini 2.5 Pro
	r.Register(ModelCapabilities{
		Provider:      "gemini",
		Model:         "gemini-2.5-pro",
		ContextWindow: 2000000,
		Reasoning: ReasoningCapabilities{
			Supported:                true,
			SupportsEffort:           true,
			SupportsThinkingBudget:   true,
			SupportsAdaptiveThinking: true,
			SupportsThinkingDisable:  true,
			SupportsPerTurnEffort:    true,
			EffortLevels:             []EffortLevel{EffortMinimal, EffortLow, EffortMedium, EffortHigh, EffortMaximum},
			MinThinkingTokens:        1,
			MaxThinkingTokens:        64000,
		},
		Caching: CacheCapabilities{
			Supported:                   true,
			ExplicitCaching:             true,
			AutomaticCaching:            true,
			CacheTTL:                    time.Hour,
			EffortChangesBreakCache:     false,
			PerTurnEffortPreservesCache: true,
		},
		Tools: ToolCapabilities{
			Supported:      true,
			ParallelCalls:  true,
			StructuredArgs: true,
		},
		Usage: UsageCapabilities{
			ReportsReasoningTokens: true,
			ReportsCachedTokens:    true,
		},
	})
}
