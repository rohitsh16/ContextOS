package compute

// ControllerState represents the complete observation vector s_t of the adaptive compute loop.
type ControllerState struct {
	Difficulty          float64     `json:"difficulty"`
	Risk                float64     `json:"risk"`
	EvidenceCoverage    float64     `json:"evidence_coverage"`
	EvidenceConflict    float64     `json:"evidence_conflict"`
	EstimatedConfidence float64     `json:"estimated_confidence"`
	CalibratedRisk      float64     `json:"calibrated_risk"`

	ContextTokens   int64 `json:"context_tokens"`
	ReasoningTokens int64 `json:"reasoning_tokens"`
	TurnCount       int   `json:"turn_count"`
	Verified        bool  `json:"verified"`

	RemainingBudget BudgetState `json:"remaining_budget"`
	CacheState      CacheState  `json:"cache_state"`
}

// AdaptiveStopping determines when task execution has satisfied quality targets or exhausted budget.
type AdaptiveStopping struct {
	maxTurns   int
	targetRisk float64
}

// NewAdaptiveStopping initializes the adaptive stopping evaluator.
func NewAdaptiveStopping(targetRisk float64, maxTurns int) *AdaptiveStopping {
	if targetRisk <= 0 {
		targetRisk = 0.05
	}
	if maxTurns <= 0 {
		maxTurns = 15
	}
	return &AdaptiveStopping{
		targetRisk: targetRisk,
		maxTurns:   maxTurns,
	}
}

// ShouldStop evaluates termination conditions across risk, evidence coverage, marginal VOI, and safety limits.
func (s *AdaptiveStopping) ShouldStop(state ControllerState, maxActionVOI float64) (bool, string) {
	// Safety limit 1: Turn limit exceeded
	if state.TurnCount >= s.maxTurns {
		return true, "safety_limit: max turns reached"
	}

	// Safety limit 2: Monetary budget exhausted
	if state.RemainingBudget.RemainingCostUSD <= 0.01 {
		return true, "budget_limit: cost budget exhausted"
	}

	// Criterion 1: Success satisfied (risk below target AND critical evidence covered)
	if state.CalibratedRisk <= s.targetRisk && state.EvidenceCoverage >= 0.85 {
		if maxActionVOI <= 0 {
			return true, "target_satisfied: calibrated risk <= target and marginal VOI <= 0"
		}
	}

	// Criterion 2: Diminishing returns (all candidate actions have non-positive VOI)
	if maxActionVOI <= 0 && state.EstimatedConfidence >= 0.70 {
		return true, "diminishing_returns: all candidate actions have negative or zero marginal VOI"
	}

	return false, ""
}
