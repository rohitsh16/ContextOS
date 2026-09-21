package compute

// Action represents an action choice by the adaptive compute engine.
type Action int

const (
	ActionStop Action = iota
	ActionRetrieve
	ActionThink
	ActionVerify
	ActionEscalate
)

func (a Action) String() string {
	switch a {
	case ActionStop:
		return "STOP"
	case ActionRetrieve:
		return "RETRIEVE"
	case ActionThink:
		return "THINK"
	case ActionVerify:
		return "VERIFY"
	case ActionEscalate:
		return "ESCALATE"
	default:
		return "STOP"
	}
}

// UtilityWeights defines parameter weights for the multi-objective utility function.
type UtilityWeights struct {
	LambdaCost    float64 // Penalizes monetary cost
	MuLatency     float64 // Penalizes latency (seconds)
	RhoRisk       float64 // Penalizes residual risk
}

// DefaultUtilityWeights provides balanced default trade-off constants.
func DefaultUtilityWeights() UtilityWeights {
	return UtilityWeights{
		LambdaCost: 1.0,
		MuLatency:  0.05,
		RhoRisk:    2.0,
	}
}

// ComputeUtility calculates U = Quality - λ*Cost - μ*Latency - ρ*Risk.
func ComputeUtility(quality, costUSD, latencySec, risk float64, w UtilityWeights) float64 {
	return quality - (w.LambdaCost * costUSD) - (w.MuLatency * latencySec) - (w.RhoRisk * risk)
}

// CacheState tracks prompt cache persistence and invalidation penalty.
type CacheState struct {
	Active                 bool    `json:"active"`
	PrefixTokens           int64   `json:"prefix_tokens"`
	HitProbability         float64 `json:"hit_probability"`
	ExpectedSavingsUSD     float64 `json:"expected_savings_usd"`
	InvalidationPenaltyUSD float64 `json:"invalidation_penalty_usd"`
}

// CandidateActionScore stores the evaluated VOI and expected costs for a given action.
type CandidateActionScore struct {
	Action       Action  `json:"action"`
	ExpectedGain float64 `json:"expected_gain"`
	ExpectedCost float64 `json:"expected_cost"`
	CachePenalty float64 `json:"cache_penalty"`
	VOI          float64 `json:"voi"`
}

// VOIEngine evaluates Value-of-Information across candidate actions.
type VOIEngine struct {
	weights UtilityWeights
}

// NewVOIEngine creates a new VOI engine with specified utility weights.
func NewVOIEngine(w UtilityWeights) *VOIEngine {
	return &VOIEngine{weights: w}
}

// EvaluateActions evaluates VOI for STOP, RETRIEVE, THINK, VERIFY, and ESCALATE given current state.
func (v *VOIEngine) EvaluateActions(
	currentConfidence float64,
	currentRisk float64,
	evidenceCoverage float64,
	difficulty float64,
	cache CacheState,
	budget BudgetState,
) []CandidateActionScore {
	currentUtility := ComputeUtility(currentConfidence, budget.SpentCostUSD, 0, currentRisk, v.weights)

	candidates := []CandidateActionScore{
		// STOP: Baseline no-op action
		{
			Action:       ActionStop,
			ExpectedGain: 0,
			ExpectedCost: 0,
			CachePenalty: 0,
			VOI:          0,
		},
	}

	// 1. RETRIEVE: High gain if evidence coverage is low or ambiguity is high
	if evidenceCoverage < 0.90 && budget.RemainingCostUSD >= 0.02 {
		missingEvidence := 1.0 - evidenceCoverage
		expectedGain := missingEvidence * 0.35
		cost := 0.015
		latency := 0.2
		projConf := currentConfidence + expectedGain
		if projConf > 0.99 {
			projConf = 0.99
		}
		projRisk := currentRisk * (1.0 - missingEvidence*0.6)
		uAfter := ComputeUtility(projConf, budget.SpentCostUSD+cost, latency, projRisk, v.weights)
		voi := uAfter - currentUtility - cost

		candidates = append(candidates, CandidateActionScore{
			Action:       ActionRetrieve,
			ExpectedGain: expectedGain,
			ExpectedCost: cost,
			CachePenalty: 0,
			VOI:          voi,
		})
	}

	// 2. THINK: High gain if task is difficult, evidence is acquired, but reasoning is needed
	if budget.RemainingReasoningTokens >= 2048 && budget.RemainingCostUSD >= 0.05 {
		// If missing evidence is high, thinking provides diminishing returns!
		// But if evidence is high and difficulty is high, thinking provides high gain!
		thinkingEffectiveness := evidenceCoverage * (0.10 + 0.30*difficulty)
		cost := 0.08
		latency := 2.5
		projConf := currentConfidence + thinkingEffectiveness
		if projConf > 0.99 {
			projConf = 0.99
		}
		projRisk := currentRisk * 0.7
		uAfter := ComputeUtility(projConf, budget.SpentCostUSD+cost, latency, projRisk, v.weights)
		voi := uAfter - currentUtility - cost

		// Cache disruption check: If thinking effort alters prefix, apply cache penalty
		var cachePenalty float64
		if cache.Active && cache.InvalidationPenaltyUSD > 0 {
			cachePenalty = (1.0 - cache.HitProbability) * cache.InvalidationPenaltyUSD
			voi -= cachePenalty
		}

		candidates = append(candidates, CandidateActionScore{
			Action:       ActionThink,
			ExpectedGain: thinkingEffectiveness,
			ExpectedCost: cost,
			CachePenalty: cachePenalty,
			VOI:          voi,
		})
	}

	// 3. VERIFY: High gain if evidence is largely acquired, current risk is high, and validation is needed
	if evidenceCoverage >= 0.65 && currentRisk > 0.10 && budget.RemainingCostUSD >= 0.03 {
		riskReduction := currentRisk * 0.75
		cost := 0.035
		latency := 0.8
		projRisk := currentRisk - riskReduction
		projConf := currentConfidence + 0.05
		if projConf > 0.99 {
			projConf = 0.99
		}
		uAfter := ComputeUtility(projConf, budget.SpentCostUSD+cost, latency, projRisk, v.weights)
		voi := uAfter - currentUtility - cost

		candidates = append(candidates, CandidateActionScore{
			Action:       ActionVerify,
			ExpectedGain: riskReduction,
			ExpectedCost: cost,
			CachePenalty: 0,
			VOI:          voi,
		})
	}

	// 4. ESCALATE: Model escalation when difficulty is high, risk is high, or repeated failures occurred
	if difficulty > 0.60 && budget.RemainingCostUSD >= 0.20 {
		expectedGain := 0.30 * difficulty
		cost := 0.25
		latency := 4.0
		projConf := currentConfidence + expectedGain
		if projConf > 0.99 {
			projConf = 0.99
		}
		projRisk := currentRisk * 0.4
		uAfter := ComputeUtility(projConf, budget.SpentCostUSD+cost, latency, projRisk, v.weights)
		voi := uAfter - currentUtility - cost

		candidates = append(candidates, CandidateActionScore{
			Action:       ActionEscalate,
			ExpectedGain: expectedGain,
			ExpectedCost: cost,
			CachePenalty: 0,
			VOI:          voi,
		})
	}

	return candidates
}

// BestAction selects the action with the maximum non-negative VOI, or STOP.
func (v *VOIEngine) BestAction(scores []CandidateActionScore) CandidateActionScore {
	if len(scores) == 0 {
		return CandidateActionScore{Action: ActionStop}
	}

	best := scores[0]
	for _, s := range scores[1:] {
		if s.VOI > best.VOI {
			best = s
		}
	}

	if best.VOI <= 0 {
		return CandidateActionScore{Action: ActionStop, VOI: 0}
	}
	return best
}
