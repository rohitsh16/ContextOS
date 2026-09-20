package temporal

import (
	"math"
	"strings"
	"time"

	"contextos/internal/model"
)

const (
	// Epsilon prevents division by zero in hazard calculations.
	Epsilon = 1e-4

	// BaseTTL defines baseline horizon for average hazard (7 days).
	BaseTTL = 7 * 24 * time.Hour
)

// HazardComponents breaks down the factors contributing to memory staleness risk.
type HazardComponents struct {
	BaseVolatility float64 `json:"base_volatility"`
	FileChurn      float64 `json:"file_churn"`
	CommitDrift    float64 `json:"commit_drift"`
	SymbolChanges  float64 `json:"symbol_changes"`
	TotalHazard    float64 `json:"total_hazard"`
}

// EstimateHazard calculates the empirical staleness hazard rate h_m(t) in [0, 1].
// h_m(t) = P(m becomes invalid | m valid at t)
func EstimateHazard(m model.Memory, fileChurn float64, commitsSinceValid int, symbolChanges int) HazardComponents {
	comp := HazardComponents{}

	// 1. Base volatility depends on memory kind and authority
	switch strings.ToLower(m.Kind) {
	case "decision", "architecture":
		comp.BaseVolatility = 0.05 // Very durable
	case "constraint":
		comp.BaseVolatility = 0.10
	case "failure", "negative_knowledge":
		comp.BaseVolatility = 0.15
	case "convention":
		comp.BaseVolatility = 0.20
	default: // fact, observation, temporary
		comp.BaseVolatility = 0.40
	}

	// Authority discount
	switch strings.ToLower(m.Authority) {
	case "user", "developer":
		comp.BaseVolatility *= 0.5 // Direct developer decision is twice as durable
	case "test", "verification":
		comp.BaseVolatility *= 0.7
	case "inference", "agent":
		comp.BaseVolatility *= 1.3
	}

	// 2. File churn penalty (if scope is empty or global, churn impact is lower than file-specific)
	if m.Scope == "" || m.Scope == "*" {
		comp.FileChurn = math.Min(0.3, fileChurn*0.5)
	} else {
		comp.FileChurn = math.Min(0.5, fileChurn)
	}

	// 3. Commit drift penalty: hazard increases with number of commits since memory creation
	comp.CommitDrift = 1.0 - math.Exp(-0.05*float64(commitsSinceValid))

	// 4. Symbol modifications in scope
	comp.SymbolChanges = math.Min(0.3, float64(symbolChanges)*0.08)

	// Combine components into total hazard rate [0, 1]
	rawHazard := comp.BaseVolatility + 0.35*comp.FileChurn + 0.25*comp.CommitDrift + 0.20*comp.SymbolChanges
	comp.TotalHazard = math.Min(1.0, math.Max(0.01, rawHazard))

	return comp
}

// CalculateAdaptiveTTL computes TTL_m = BaseTTL / (h_m + epsilon) bounded between MinTTL and MaxTTL.
func CalculateAdaptiveTTL(m model.Memory, hazard float64) time.Duration {
	if hazard <= 0 {
		hazard = Epsilon
	}

	// Scale duration inversely proportional to hazard
	// Lower hazard -> longer TTL (e.g. up to 90 days for architectural decisions)
	// Higher hazard -> shorter TTL (e.g. down to 2 hours for volatile observations)
	factor := 0.20 / (hazard + Epsilon)
	seconds := BaseTTL.Seconds() * factor

	minTTL := 2 * time.Hour
	maxTTL := 90 * 24 * time.Hour

	duration := time.Duration(seconds) * time.Second
	if duration < minTTL {
		return minTTL
	}
	if duration > maxTTL {
		return maxTTL
	}
	return duration
}

// IsStaleAdaptive determines whether a memory has exceeded its adaptive TTL.
func IsStaleAdaptive(m model.Memory, created time.Time, now time.Time, hazard float64) bool {
	if created.IsZero() {
		return false
	}
	ttl := CalculateAdaptiveTTL(m, hazard)
	return now.Sub(created) > ttl
}
