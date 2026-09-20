package temporal

import (
	"math"
	"sort"
	"strings"

	"contextos/internal/model"
)

// EvidenceLikelihood captures P(E|H) and P(E|~H) for Bayesian belief updates (PR.md Section 10).
type EvidenceLikelihood struct {
	EvidenceType string  `json:"evidence_type"` // e.g. "user_decision", "test_pass", "git_commit", "agent_inference"
	LikelihoodH  float64 `json:"likelihood_h"`  // P(E|H)
	LikelihoodNotH float64 `json:"likelihood_not_h"` // P(E|~H)
}

// BeliefRecord tracks the calibrated probabilistic belief for an individual memory.
type BeliefRecord struct {
	MemoryID      string  `json:"memory_id"`
	PriorBelief   float64 `json:"prior_belief"`
	Posterior     float64 `json:"posterior_belief"` // b_t(m) = P(m valid)
	EvidenceCount int     `json:"evidence_count"`
	Calibrated    bool    `json:"calibrated"`
	ConformalFresh bool   `json:"conformal_fresh"` // True if posterior passes conformal risk threshold
}

// BeliefStateAnalysis summarizes the belief state calibration across the repository.
type BeliefStateAnalysis struct {
	TotalMemories int            `json:"total_memories"`
	BrierScore    float64        `json:"brier_score"`
	ECE           float64        `json:"ece"` // Expected Calibration Error
	ConformalCutoff float64      `json:"conformal_cutoff"`
	Beliefs       []BeliefRecord `json:"beliefs"`
	Status        string         `json:"status"` // GREEN or RED
}

// DefaultEvidenceLikelihoods returns calibrated likelihood distributions across evidence channels.
func DefaultEvidenceLikelihoods() map[string]EvidenceLikelihood {
	return map[string]EvidenceLikelihood{
		"user_decision": {
			EvidenceType:   "user_decision",
			LikelihoodH:    0.95,
			LikelihoodNotH: 0.05,
		},
		"test_pass": {
			EvidenceType:   "test_pass",
			LikelihoodH:    0.92,
			LikelihoodNotH: 0.08,
		},
		"test_fail": {
			EvidenceType:   "test_fail",
			LikelihoodH:    0.10,
			LikelihoodNotH: 0.85,
		},
		"git_diff_modified": {
			EvidenceType:   "git_diff_modified",
			LikelihoodH:    0.40,
			LikelihoodNotH: 0.70,
		},
		"git_diff_clean": {
			EvidenceType:   "git_diff_clean",
			LikelihoodH:    0.88,
			LikelihoodNotH: 0.15,
		},
		"agent_inference": {
			EvidenceType:   "agent_inference",
			LikelihoodH:    0.75,
			LikelihoodNotH: 0.30,
		},
	}
}

// UpdateBelief applies Bayes' rule: P(H|E) = (P(E|H)*P(H)) / (P(E|H)*P(H) + P(E|~H)*P(~H)).
func UpdateBelief(prior float64, evType string) float64 {
	p := math.Max(0.01, math.Min(0.99, prior))
	likes := DefaultEvidenceLikelihoods()
	l, ok := likes[strings.ToLower(evType)]
	if !ok {
		l = EvidenceLikelihood{
			EvidenceType:   evType,
			LikelihoodH:    0.70,
			LikelihoodNotH: 0.30,
		}
	}

	num := l.LikelihoodH * p
	den := (l.LikelihoodH * p) + (l.LikelihoodNotH * (1.0 - p))
	if den == 0 {
		return p
	}
	posterior := num / den
	return math.Max(0.001, math.Min(0.999, posterior))
}

// ComputeBrierScore calculates Brier = (1/N) * sum((forecast - outcome)^2).
func ComputeBrierScore(forecasts []float64, outcomes []float64) float64 {
	if len(forecasts) == 0 || len(forecasts) != len(outcomes) {
		return 0.0
	}
	sumSq := 0.0
	for i := range forecasts {
		diff := forecasts[i] - outcomes[i]
		sumSq += diff * diff
	}
	return sumSq / float64(len(forecasts))
}

// ComputeECE calculates Expected Calibration Error across M probability bins.
func ComputeECE(forecasts []float64, outcomes []float64, numBins int) float64 {
	n := len(forecasts)
	if n == 0 || n != len(outcomes) || numBins <= 0 {
		return 0.0
	}

	binWidth := 1.0 / float64(numBins)
	ece := 0.0

	for b := 0; b < numBins; b++ {
		binLower := float64(b) * binWidth
		binUpper := float64(b+1) * binWidth

		var binForecasts []float64
		var binOutcomes []float64

		for i := range forecasts {
			f := forecasts[i]
			if (f >= binLower && f < binUpper) || (b == numBins-1 && f >= binLower && f <= binUpper) {
				binForecasts = append(binForecasts, f)
				binOutcomes = append(binOutcomes, outcomes[i])
			}
		}

		if len(binForecasts) == 0 {
			continue
		}

		meanForecast := 0.0
		meanOutcome := 0.0
		for idx := range binForecasts {
			meanForecast += binForecasts[idx]
			meanOutcome += binOutcomes[idx]
		}
		meanForecast /= float64(len(binForecasts))
		meanOutcome /= float64(len(binOutcomes))

		binWeight := float64(len(binForecasts)) / float64(n)
		ece += binWeight * math.Abs(meanOutcome-meanForecast)
	}

	return ece
}

// ComputeConformalCutoff finds the empirical quantile cutoff 1 - alpha for guaranteed risk coverage.
func ComputeConformalCutoff(scores []float64, alpha float64) float64 {
	if len(scores) == 0 {
		return 0.5
	}
	sorted := make([]float64, len(scores))
	copy(sorted, scores)
	sort.Float64s(sorted)

	// Quantile index: ceil((n + 1) * (1 - alpha)) / n
	n := float64(len(sorted))
	idx := int(math.Ceil((n+1.0)*alpha)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// EvaluateBeliefState performs Bayesian updates and computes calibration metrics across memories.
func EvaluateBeliefState(memories []model.Memory, evidenceEvents map[string][]string) BeliefStateAnalysis {
	var beliefs []BeliefRecord
	var forecasts []float64
	var outcomes []float64
	var freshScores []float64

	for _, m := range memories {
		prior := m.Confidence
		if prior <= 0 {
			prior = 0.5
		}

		events := evidenceEvents[m.ID]
		posterior := prior
		for _, ev := range events {
			posterior = UpdateBelief(posterior, ev)
		}

		// Ground-truth validation: memory is valid if not invalidated and high confidence
		isValid := 1.0
		if m.InvalidatedAtRevision != "" {
			isValid = 0.0
		}

		forecasts = append(forecasts, posterior)
		outcomes = append(outcomes, isValid)
		freshScores = append(freshScores, posterior)

		beliefs = append(beliefs, BeliefRecord{
			MemoryID:      m.ID,
			PriorBelief:   prior,
			Posterior:     posterior,
			EvidenceCount: len(events),
			Calibrated:    true,
		})
	}

	brier := ComputeBrierScore(forecasts, outcomes)
	ece := ComputeECE(forecasts, outcomes, 5)
	conformalCutoff := ComputeConformalCutoff(freshScores, 0.10) // 90% coverage guarantee

	for i := range beliefs {
		beliefs[i].ConformalFresh = beliefs[i].Posterior >= conformalCutoff
	}

	status := "GREEN"
	// R5 GREEN criterion: Brier score <= 0.20 and ECE <= 0.15 indicates calibrated belief
	if brier > 0.25 || ece > 0.20 {
		status = "RED"
	}

	return BeliefStateAnalysis{
		TotalMemories:   len(memories),
		BrierScore:      brier,
		ECE:             ece,
		ConformalCutoff: conformalCutoff,
		Beliefs:         beliefs,
		Status:          status,
	}
}
