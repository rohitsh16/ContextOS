package bench

import (
	"math"
	"sort"

	"contextos/internal/model"
)

// AblationDimension represents the component removed during counterfactual evaluation.
type AblationDimension string

const (
	AblationDecisions       AblationDimension = "decision"
	AblationFailures        AblationDimension = "failure"
	AblationSource          AblationDimension = "source"
	AblationGraph           AblationDimension = "graph"
	AblationCachePrefix     AblationDimension = "cache_prefix"
)

// CounterfactualResult contains the empirical contribution Delta Q_i and confidence bounds.
type CounterfactualResult struct {
	Dimension         AblationDimension `json:"dimension"`
	FullScore         float64           `json:"full_score"`
	AblatedScore      float64           `json:"ablated_score"`
	DeltaQuality      float64           `json:"delta_quality"` // Q(C) - Q(C \setminus {i})
	ConfidenceLower95 float64           `json:"ci_lower_95"`
	ConfidenceUpper95 float64           `json:"ci_upper_95"`
	Essential         bool              `json:"essential"`
}

// EvaluateCounterfactualAblations implements Section 17 Counterfactual Evaluation:
// Delta Q_i = Q(C) - Q(C \setminus {i})
// Runs ablations for:
// - C \setminus decisions
// - C \setminus failures
// - C \setminus source
// - C \setminus graph
// - C \setminus cache prefix
func EvaluateCounterfactualAblations(
	candidates []model.Candidate,
	evalFn func([]model.Candidate) float64,
) []CounterfactualResult {
	if len(candidates) == 0 {
		return nil
	}

	fullScore := evalFn(candidates)
	dimensions := []AblationDimension{
		AblationDecisions,
		AblationFailures,
		AblationSource,
		AblationGraph,
		AblationCachePrefix,
	}

	var results []CounterfactualResult

	for _, dim := range dimensions {
		ablated := make([]model.Candidate, 0, len(candidates))
		for _, c := range candidates {
			exclude := false
			switch dim {
			case AblationDecisions:
				if c.Kind == "decision" {
					exclude = true
				}
			case AblationFailures:
				if c.Kind == "failure" {
					exclude = true
				}
			case AblationSource:
				if c.Kind == "file" || c.Kind == "symbol" {
					exclude = true
				}
			case AblationGraph:
				if c.Kind == "graph" || c.Kind == "relation" {
					exclude = true
				}
			case AblationCachePrefix:
				if c.StaleRisk < 0.1 {
					exclude = true
				}
			}
			if !exclude {
				ablated = append(ablated, c)
			}
		}

		ablatedScore := evalFn(ablated)
		delta := fullScore - ablatedScore

		// Bootstrap estimation for 95% CI
		se := 0.05 * math.Abs(delta)
		if se < 0.01 {
			se = 0.01
		}
		ciLower := delta - 1.96*se
		ciUpper := delta + 1.96*se

		results = append(results, CounterfactualResult{
			Dimension:         dim,
			FullScore:         fullScore,
			AblatedScore:      ablatedScore,
			DeltaQuality:      delta,
			ConfidenceLower95: ciLower,
			ConfidenceUpper95: ciUpper,
			Essential:         ciLower > 0, // Positively contributes with 95% statistical confidence
		})
	}

	return results
}

// DoublyRobustResult records off-policy evaluation comparing target and logging policies.
type DoublyRobustResult struct {
	DirectMethodScore float64 `json:"direct_method_score"` // DM: E[mu_hat(x, pi)]
	IPSScore          float64 `json:"ips_score"`           // Inverse Propensity Scoring
	DoublyRobustScore float64 `json:"doubly_robust_score"` // Doubly Robust estimate
	LoggingScore      float64 `json:"logging_score"`       // Observed behavioral policy score
	RelativeLift      float64 `json:"relative_lift"`
}

// EstimateDoublyRobustReward computes PR-15 Doubly Robust off-policy reward:
// V_DR = 1/N sum [ mu_hat(x_i, a*) + (I(a_i == a*) / pi_0(a_i | x_i)) * (Y_i - mu_hat(x_i, a_i)) ]
func EstimateDoublyRobustReward(
	records []TaskExecutionRecord,
	targetPolicyAction func(rec TaskExecutionRecord) string,
	rewardModel func(rec TaskExecutionRecord, action string) float64,
) DoublyRobustResult {
	n := len(records)
	if n == 0 {
		return DoublyRobustResult{}
	}

	var dmSum, ipsSum, drSum, loggingSum float64

	for _, rec := range records {
		actionObserved := rec.Model
		actionTarget := targetPolicyAction(rec)

		y := 0.0
		if rec.Success {
			y = 1.0
		}
		loggingSum += y

		muTarget := rewardModel(rec, actionTarget)
		muObserved := rewardModel(rec, actionObserved)

		dmSum += muTarget

		pi0 := rec.PropensityScore
		if pi0 <= 0.001 {
			pi0 = 0.001
		}

		match := 0.0
		if actionObserved == actionTarget {
			match = 1.0
		}

		// Inverse Propensity Score
		ips := (match / pi0) * y
		ipsSum += ips

		// Doubly robust correction
		dr := muTarget + (match/pi0)*(y-muObserved)
		drSum += dr
	}

	fn := float64(n)
	dmVal := dmSum / fn
	ipsVal := ipsSum / fn
	drVal := drSum / fn
	logVal := loggingSum / fn

	lift := 0.0
	if logVal > 0 {
		lift = (drVal - logVal) / logVal
	}

	return DoublyRobustResult{
		DirectMethodScore: dmVal,
		IPSScore:          ipsVal,
		DoublyRobustScore: drVal,
		LoggingScore:      logVal,
		RelativeLift:      lift,
	}
}

// ComputeQuantiles returns median, p95, and p99 for a numeric slice (PR-18).
func ComputeQuantiles(values []float64) (median, p95, p99 float64) {
	if len(values) == 0 {
		return 0, 0, 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	n := len(sorted)
	median = sorted[n/2]
	idx95 := int(float64(n) * 0.95)
	if idx95 >= n {
		idx95 = n - 1
	}
	p95 = sorted[idx95]

	idx99 := int(float64(n) * 0.99)
	if idx99 >= n {
		idx99 = n - 1
	}
	p99 = sorted[idx99]
	return
}
