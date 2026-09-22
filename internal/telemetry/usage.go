package telemetry

import "time"

// UsageMetrics captures normalized token counts, latencies, and financial costs across LLM providers.
type UsageMetrics struct {
	InputTokens         int64          `json:"input_tokens"`
	CachedInputTokens   int64          `json:"cached_input_tokens"`
	CacheWriteTokens    int64          `json:"cache_write_tokens"`
	OutputTokens        int64          `json:"output_tokens"`
	ReasoningTokens     int64          `json:"reasoning_tokens"`
	VisibleOutputTokens int64          `json:"visible_output_tokens"`
	TotalTokens         int64          `json:"total_tokens"`

	ToolCalls           int            `json:"tool_calls"`
	Turns               int            `json:"turns"`

	RetrievalLatencyMS  int64          `json:"retrieval_latency_ms"`
	ModelLatencyMS      int64          `json:"model_latency_ms"`
	ToolLatencyMS       int64          `json:"tool_latency_ms"`
	TotalLatencyMS      int64          `json:"total_latency_ms"`

	EstimatedCostUSD        float64        `json:"estimated_cost_usd"`
	ProviderReportedCostUSD float64        `json:"provider_reported_cost_usd,omitempty"`

	// R16 Section 11: Requested vs Realized compute
	RequestedEffort          string  `json:"requested_effort,omitempty"`
	RequestedReasoningBudget int64   `json:"requested_reasoning_budget,omitempty"`
	ReasoningUtilization     float64 `json:"reasoning_utilization,omitempty"`

	UnknownFields       map[string]any `json:"unknown_fields,omitempty"`
}

// Add aggregates usage from another turn or sub-action into this UsageMetrics.
func (u *UsageMetrics) Add(other UsageMetrics) {
	u.InputTokens += other.InputTokens
	u.CachedInputTokens += other.CachedInputTokens
	u.CacheWriteTokens += other.CacheWriteTokens
	u.OutputTokens += other.OutputTokens
	u.ReasoningTokens += other.ReasoningTokens
	u.VisibleOutputTokens += other.VisibleOutputTokens
	u.TotalTokens += other.TotalTokens
	u.ToolCalls += other.ToolCalls
	u.Turns += other.Turns
	u.RetrievalLatencyMS += other.RetrievalLatencyMS
	u.ModelLatencyMS += other.ModelLatencyMS
	u.ToolLatencyMS += other.ToolLatencyMS
	u.TotalLatencyMS += other.TotalLatencyMS
	u.EstimatedCostUSD += other.EstimatedCostUSD
	u.ProviderReportedCostUSD += other.ProviderReportedCostUSD
	if other.RequestedEffort != "" {
		u.RequestedEffort = other.RequestedEffort
	}
	if other.RequestedReasoningBudget > 0 {
		u.RequestedReasoningBudget += other.RequestedReasoningBudget
	}
	if u.RequestedReasoningBudget > 0 {
		u.ReasoningUtilization = float64(u.ReasoningTokens) / float64(u.RequestedReasoningBudget)
	}
	if other.UnknownFields != nil {
		if u.UnknownFields == nil {
			u.UnknownFields = make(map[string]any)
		}
		for k, v := range other.UnknownFields {
			u.UnknownFields[k] = v
		}
	}
}

// Clone returns a deep copy of the UsageMetrics.
func (u UsageMetrics) Clone() UsageMetrics {
	out := u
	if u.UnknownFields != nil {
		out.UnknownFields = make(map[string]any, len(u.UnknownFields))
		for k, v := range u.UnknownFields {
			out.UnknownFields[k] = v
		}
	}
	return out
}

// EffectiveInputTokens returns uncached input tokens.
func (u UsageMetrics) EffectiveInputTokens() int64 {
	uncached := u.InputTokens - u.CachedInputTokens
	if uncached < 0 {
		return 0
	}
	return uncached
}

// UsageRecord represents a timestamped event wrapper for usage.
type UsageRecord struct {
	Timestamp time.Time    `json:"timestamp"`
	Usage     UsageMetrics `json:"usage"`
}
