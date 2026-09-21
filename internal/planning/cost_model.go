package planning

// ContextReasoningCandidate represents a point on the joint context-compute frontier.
type ContextReasoningCandidate struct {
	ContextTokens       int     `json:"context_tokens"`
	ContextCostUSD      float64 `json:"context_cost_usd"`
	ExpectedReasoningTokens int `json:"expected_reasoning_tokens"`
	ExpectedReasoningCostUSD float64 `json:"expected_reasoning_cost_usd"`
	ResidualRisk        float64 `json:"residual_risk"`
	TotalCostUSD        float64 `json:"total_cost_usd"` // J(C) = C_context + C_reasoning
}

// JointCostOptimizer solves J(C) = C_context(C) + E[C_reasoning | C] subject to Risk(C) <= epsilon.
type JointCostOptimizer struct {
	riskTarget float64
}

// NewJointCostOptimizer creates an optimizer with target risk epsilon.
func NewJointCostOptimizer(riskTarget float64) *JointCostOptimizer {
	if riskTarget <= 0 {
		riskTarget = 0.05
	}
	return &JointCostOptimizer{riskTarget: riskTarget}
}

// OptimizeFrontier evaluates candidate context sizes and selects the joint minimum cost point.
func (jco *JointCostOptimizer) OptimizeFrontier(
	taskDifficulty float64,
	inputCostPerMillion float64,
	reasoningCostPerMillion float64,
) ContextReasoningCandidate {
	// Sample context token points from 1k to 32k
	contextPoints := []int{1000, 2500, 5000, 10000, 16000, 24000, 32000}

	var candidates []ContextReasoningCandidate
	for _, ctxTokens := range contextPoints {
		ctxCost := (float64(ctxTokens) / 1e6) * inputCostPerMillion

		// Context/Compute relationship: More context provides relevant facts, reducing reasoning needed.
		// Less context starves the model, forcing high thinking or failing.
		// Model: expected reasoning = Base * (1 + 8000 / (ctxTokens + 500)) * difficulty
		baseReasoning := 3000.0 * (0.5 + taskDifficulty)
		ctxFactor := 8000.0 / (float64(ctxTokens) + 500.0)
		expectedReasoning := int(baseReasoning * (0.8 + ctxFactor))

		reasoningCost := (float64(expectedReasoning) / 1e6) * reasoningCostPerMillion

		// Residual risk drops with both context and reasoning
		risk := (taskDifficulty * 0.4) * (2000.0 / (float64(ctxTokens) + 1000.0))
		if risk > 1.0 {
			risk = 1.0
		}

		totalCost := ctxCost + reasoningCost

		candidates = append(candidates, ContextReasoningCandidate{
			ContextTokens:            ctxTokens,
			ContextCostUSD:           ctxCost,
			ExpectedReasoningTokens:  expectedReasoning,
			ExpectedReasoningCostUSD: reasoningCost,
			ResidualRisk:             risk,
			TotalCostUSD:             totalCost,
		})
	}

	// Select candidate that minimizes totalCost subject to risk <= riskTarget
	// If none strictly meet riskTarget, select the one with minimum totalCost among lowest risk
	best := candidates[0]
	foundFeasible := false

	for _, c := range candidates {
		if c.ResidualRisk <= jco.riskTarget {
			if !foundFeasible || c.TotalCostUSD < best.TotalCostUSD {
				best = c
				foundFeasible = true
			}
		}
	}

	if !foundFeasible {
		// Fallback: Pick minimum total cost
		for _, c := range candidates {
			if c.TotalCostUSD < best.TotalCostUSD {
				best = c
			}
		}
	}

	return best
}
