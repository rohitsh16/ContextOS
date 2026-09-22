package telemetry

import (
	"math"
)

// CostBreakdown decomposes end-to-end task cost into 10 explicit buckets (R16 Section 10).
// Ensures zero hidden subsidies from controller overhead, verification, retries, or tool calls.
type CostBreakdown struct {
	InputUSD         float64 `json:"input_usd"`
	CachedInputUSD   float64 `json:"cached_input_usd"`
	CacheWriteUSD    float64 `json:"cache_write_usd"`
	ReasoningUSD     float64 `json:"reasoning_usd"`
	VisibleOutputUSD float64 `json:"visible_output_usd"`
	ToolUSD          float64 `json:"tool_usd"`
	VerificationUSD  float64 `json:"verification_usd"`
	RetryUSD         float64 `json:"retry_usd"`
	EscalationUSD    float64 `json:"escalation_usd"`
	ControllerUSD    float64 `json:"controller_usd"`

	TotalUSD float64 `json:"total_usd"`
}

// ComputeCostBreakdown maps raw usage and execution activity into the 10-bucket decomposition.
func ComputeCostBreakdown(
	p PricingEntry,
	u UsageMetrics,
	toolCostUSD float64,
	verificationCostUSD float64,
	retryCostUSD float64,
	escalationCostUSD float64,
	controllerCostUSD float64,
) CostBreakdown {
	uncachedTokens := u.EffectiveInputTokens()
	inputUSD := (float64(uncachedTokens) / 1e6) * p.InputPerMillion
	cachedInputUSD := (float64(u.CachedInputTokens) / 1e6) * p.CachedInputPerMillion
	cacheWriteUSD := (float64(u.CacheWriteTokens) / 1e6) * p.CacheWritePerMillion

	reasoningUSD := (float64(u.ReasoningTokens) / 1e6) * p.ReasoningPerMillion
	visibleTokens := u.OutputTokens - u.ReasoningTokens
	if visibleTokens < 0 {
		visibleTokens = 0
	}
	visibleOutputUSD := (float64(visibleTokens) / 1e6) * p.OutputPerMillion

	cb := CostBreakdown{
		InputUSD:         inputUSD,
		CachedInputUSD:   cachedInputUSD,
		CacheWriteUSD:    cacheWriteUSD,
		ReasoningUSD:     reasoningUSD,
		VisibleOutputUSD: visibleOutputUSD,
		ToolUSD:          toolCostUSD,
		VerificationUSD:  verificationCostUSD,
		RetryUSD:         retryCostUSD,
		EscalationUSD:    escalationCostUSD,
		ControllerUSD:    controllerCostUSD,
	}

	cb.TotalUSD = cb.Sum()
	return cb
}

// Sum returns the exact mathematical sum of all 10 constituent buckets.
func (cb CostBreakdown) Sum() float64 {
	return cb.InputUSD +
		cb.CachedInputUSD +
		cb.CacheWriteUSD +
		cb.ReasoningUSD +
		cb.VisibleOutputUSD +
		cb.ToolUSD +
		cb.VerificationUSD +
		cb.RetryUSD +
		cb.EscalationUSD +
		cb.ControllerUSD
}

// Reconcile verifies that the reported TotalUSD matches the sum of the components within epsilon.
func (cb CostBreakdown) Reconcile(epsilon float64) bool {
	if epsilon <= 0 {
		epsilon = 1e-7
	}
	return math.Abs(cb.TotalUSD-cb.Sum()) <= epsilon
}

// Add aggregates another CostBreakdown into this one.
func (cb *CostBreakdown) Add(other CostBreakdown) {
	cb.InputUSD += other.InputUSD
	cb.CachedInputUSD += other.CachedInputUSD
	cb.CacheWriteUSD += other.CacheWriteUSD
	cb.ReasoningUSD += other.ReasoningUSD
	cb.VisibleOutputUSD += other.VisibleOutputUSD
	cb.ToolUSD += other.ToolUSD
	cb.VerificationUSD += other.VerificationUSD
	cb.RetryUSD += other.RetryUSD
	cb.EscalationUSD += other.EscalationUSD
	cb.ControllerUSD += other.ControllerUSD
	cb.TotalUSD = cb.Sum()
}
