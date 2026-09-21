package retrieval

import (
	"sort"
)

// OptimizerConfig sets trade-off multipliers for the decision-aware context optimizer (PR.md Section 25).
type OptimizerConfig struct {
	BudgetTokens   int     `json:"budget_tokens"`
	TargetAccuracy float64 `json:"target_accuracy"` // tau: minimum acceptable P(correct decision)
	LambdaTokens   float64 `json:"lambda_tokens"`
	LambdaCost     float64 `json:"lambda_cost"`
	LambdaRisk     float64 `json:"lambda_risk"`
	LambdaUtility  float64 `json:"lambda_utility"`
}

// DefaultOptimizerConfig provides balanced default penalties and utilities.
var DefaultOptimizerConfig = OptimizerConfig{
	BudgetTokens:   4000,
	TargetAccuracy: 0.85,
	LambdaTokens:   0.0001,
	LambdaCost:     0.05,
	LambdaRisk:     0.30,
	LambdaUtility:  1.00,
}

// OptimizationResult summarizes the outcome of decision-aware context selection.
type OptimizationResult struct {
	SelectedUnits        []ContextUnit `json:"selected_units"`
	TotalTokens          int           `json:"total_tokens"`
	PrunedTokens         int           `json:"pruned_tokens"`
	EstimatedCorrectness float64       `json:"estimated_correctness"` // P(correct decision | C, q)
	ObjectiveScore       float64       `json:"objective_score"`
}

// OptimizeContext selects the minimal decision-sufficient subset of ContextUnits (PR.md Section 25).
// Solves: min_C [ lambda_t * Tokens(C) + lambda_r * Risk(C) - lambda_u * Utility(C) ]
// subject to: P(correct decision | C, q) >= tau and Tokens(C) <= Budget.
func OptimizeContext(available []ContextUnit, cfg OptimizerConfig) OptimizationResult {
	if len(available) == 0 {
		return OptimizationResult{}
	}

	// Compute total available tokens for accounting
	var totalAvailableTokens int
	for _, u := range available {
		totalAvailableTokens += u.Tokens
	}

	// Sort candidates by efficiency ratio: marginal utility per token
	// Utility = DecisionValue * (1 + CacheReuse*0.2)
	type ratedUnit struct {
		unit      ContextUnit
		ratio     float64
		effTokens int
	}

	rated := make([]ratedUnit, len(available))
	for i, u := range available {
		effTokens := u.Tokens
		if effTokens < 1 {
			effTokens = 1
		}
		utility := u.DecisionValue * (1.0 + 0.20*u.CacheReuse)
		rated[i] = ratedUnit{
			unit:      u,
			ratio:     utility / float64(effTokens),
			effTokens: effTokens,
		}
	}

	sort.Slice(rated, func(i, j int) bool {
		return rated[i].ratio > rated[j].ratio
	})

	var selected []ContextUnit
	var currentTokens int
	// Noisy-OR representation of probability of missing critical evidence:
	// P(error) = prod(1 - u.DecisionValue)
	residualError := 1.0

	for _, ru := range rated {
		if currentTokens+ru.effTokens > cfg.BudgetTokens {
			// Cannot fit without violating budget
			continue
		}

		selected = append(selected, ru.unit)
		currentTokens += ru.effTokens
		residualError *= (1.0 - ru.unit.DecisionValue*0.8)

		pCorrect := 1.0 - residualError
		// If target accuracy reached and minimum token floor satisfied, we can stop
		if pCorrect >= cfg.TargetAccuracy && currentTokens >= 200 {
			break
		}
	}

	pCorrect := 1.0 - residualError
	if pCorrect > 1.0 {
		pCorrect = 1.0
	}

	var totalUtility float64
	var totalRisk float64
	for _, u := range selected {
		totalUtility += u.DecisionValue
		totalRisk += u.Uncertainty
	}

	objectiveScore := cfg.LambdaTokens*float64(currentTokens) +
		cfg.LambdaRisk*totalRisk -
		cfg.LambdaUtility*totalUtility

	return OptimizationResult{
		SelectedUnits:        selected,
		TotalTokens:          currentTokens,
		PrunedTokens:         totalAvailableTokens - currentTokens,
		EstimatedCorrectness: pCorrect,
		ObjectiveScore:       objectiveScore,
	}
}
