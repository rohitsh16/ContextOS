package retrieval

import (
	"math"
	"sort"
)

// ComputeEntropy calculates Shannon entropy H = -sum(p_i * ln(p_i)) over candidate scores (PR.md Section 18).
func ComputeEntropy(scores []float64, temperature float64) float64 {
	if len(scores) <= 1 {
		return 0.0
	}
	if temperature <= 0 {
		temperature = 1.0
	}

	// Softmax probabilities
	var maxScore float64
	for _, s := range scores {
		if s > maxScore {
			maxScore = s
		}
	}

	var sumExp float64
	expScores := make([]float64, len(scores))
	for i, s := range scores {
		expScores[i] = math.Exp((s - maxScore) / temperature)
		sumExp += expScores[i]
	}

	if sumExp == 0 {
		return 0.0
	}

	var entropy float64
	for _, exp := range expScores {
		p := exp / sumExp
		if p > 1e-12 {
			entropy -= p * math.Log(p)
		}
	}

	return entropy
}

// DetermineAdaptiveK scales K between kMin and kMax based on candidate score entropy (PR.md Section 18).
// Low entropy -> confident distribution -> small K
// High entropy -> uncertain distribution -> larger K
func DetermineAdaptiveK(scores []float64, kMin, kMax int, temperature float64) int {
	if len(scores) == 0 {
		return kMin
	}
	if kMin >= kMax {
		return kMin
	}

	h := ComputeEntropy(scores, temperature)
	hMax := math.Log(float64(len(scores)))
	if hMax <= 0 {
		return kMin
	}

	normalizedEntropy := h / hMax
	if normalizedEntropy > 1.0 {
		normalizedEntropy = 1.0
	}

	k := kMin + int(math.Round(float64(kMax-kMin)*normalizedEntropy))
	if k < kMin {
		k = kMin
	}
	if k > kMax {
		k = kMax
	}
	return k
}

// AdaptiveStopIndex determines where to truncate candidate list based on marginal gain and confidence (PR.md Section 19).
// Stop when: confidence >= targetConfidence AND marginal gain delta_k = S_k - S_{k+1} < epsilon.
func AdaptiveStopIndex(
	candidates []Candidate,
	minFloor int,
	targetConfidence float64,
	epsilon float64,
) int {
	n := len(candidates)
	if n <= minFloor {
		return n
	}

	for k := minFloor; k < n-1; k++ {
		currScore := candidates[k].Score
		nextScore := candidates[k+1].Score
		marginalGain := currScore - nextScore

		// Confidence proxy based on absolute score and distance from noise floor
		confidence := currScore
		if confidence > 1.0 {
			confidence = 1.0
		}

		if confidence >= targetConfidence && marginalGain < epsilon {
			return k + 1
		}
	}

	return n
}

// HistoricalBoost applies query-history-aware score adjustments from reuse frequency (PR.md Section 20).
func HistoricalBoost(
	candidates []Candidate,
	reuseCounts map[string]int,
	boostWeight float64,
) []Candidate {
	if len(reuseCounts) == 0 || boostWeight <= 0 {
		return candidates
	}

	boosted := make([]Candidate, len(candidates))
	copy(boosted, candidates)

	for i := range boosted {
		nodeID := boosted[i].NodeID
		if count, ok := reuseCounts[nodeID]; ok && count > 0 {
			// Sub-linear boost: log(1 + count)
			factor := math.Log1p(float64(count))
			boosted[i].Score += boostWeight * factor
		}
	}

	// Re-sort descending
	sort.Slice(boosted, func(i, j int) bool {
		return boosted[i].Score > boosted[j].Score
	})

	return boosted
}
