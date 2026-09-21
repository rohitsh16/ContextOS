package compute

import (
	"contextos/internal/telemetry"
)

// ReasoningCurvePoint represents estimated accuracy and cost at a given effort level for a model.
type ReasoningCurvePoint struct {
	EffortLevel      EffortLevel `json:"effort_level"`
	ReasoningTokens  int64       `json:"reasoning_tokens"`
	EstimatedSuccess float64     `json:"estimated_success"` // A_m(e)
	EstimatedCostUSD float64     `json:"estimated_cost_usd"` // C_m(e)
}

// ComputeEstimator evaluates marginal reasoning gains and token distributions.
type ComputeEstimator struct {
	lambda float64 // Minimum marginal value threshold
}

// NewComputeEstimator creates an estimator with a default marginal threshold lambda.
func NewComputeEstimator(lambda float64) *ComputeEstimator {
	if lambda <= 0 {
		lambda = 0.50 // Default threshold: $0.50 per 1.0 accuracy gain
	}
	return &ComputeEstimator{lambda: lambda}
}

// EstimateCurve generates the discrete accuracy and cost curve for a model across effort levels.
func (ce *ComputeEstimator) EstimateCurve(provider, model string, taskDifficulty float64) []ReasoningCurvePoint {
	levels := []EffortLevel{EffortMinimal, EffortLow, EffortMedium, EffortHigh, EffortMaximum}
	pricing, _ := telemetry.LookupPricing(provider, model)

	curve := make([]ReasoningCurvePoint, len(levels))
	for i, lvl := range levels {
		var rTokens int64
		switch lvl {
		case EffortMinimal:
			rTokens = 0
		case EffortLow:
			rTokens = 2048
		case EffortMedium:
			rTokens = 8192
		case EffortHigh:
			rTokens = 16384
		case EffortMaximum:
			rTokens = 32768
		}

		// Higher difficulty requires more reasoning to reach high success probability.
		// Success model: Base sigmoid-like saturation
		normTokens := float64(rTokens) / 16384.0
		baseProb := 0.40 * (1.0 - taskDifficulty*0.5)
		saturationGain := 0.55 / (1.0 + (taskDifficulty*1.5)/(normTokens+0.1))

		success := baseProb + saturationGain
		if success > 0.98 {
			success = 0.98
		}

		usage := telemetry.UsageMetrics{
			InputTokens:     2000,
			OutputTokens:    rTokens + 500,
			ReasoningTokens: rTokens,
		}
		cost := telemetry.CalculateUsageCost(pricing, usage)

		curve[i] = ReasoningCurvePoint{
			EffortLevel:      lvl,
			ReasoningTokens:  rTokens,
			EstimatedSuccess: success,
			EstimatedCostUSD: cost,
		}
	}
	return curve
}

// MarginalValueOfThinking computes MVTC(e) = (A(e+Δe) - A(e)) / (C(e+Δe) - C(e)).
func (ce *ComputeEstimator) MarginalValueOfThinking(current, next ReasoningCurvePoint) float64 {
	costDelta := next.EstimatedCostUSD - current.EstimatedCostUSD
	if costDelta <= 0 {
		return 0
	}
	accDelta := next.EstimatedSuccess - current.EstimatedSuccess
	if accDelta <= 0 {
		return 0
	}
	return accDelta / costDelta
}

// RecommendOptimalEffort finds the knee of the reasoning curve where MVTC falls below lambda.
func (ce *ComputeEstimator) RecommendOptimalEffort(curve []ReasoningCurvePoint) EffortLevel {
	if len(curve) == 0 {
		return EffortMedium
	}

	best := curve[0].EffortLevel
	for i := 0; i < len(curve)-1; i++ {
		mvtc := ce.MarginalValueOfThinking(curve[i], curve[i+1])
		if mvtc >= ce.lambda {
			best = curve[i+1].EffortLevel
		} else {
			// Knee of the curve reached
			break
		}
	}
	return best
}
