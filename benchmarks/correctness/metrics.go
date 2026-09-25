package correctness

import (
	"math"
	"math/rand"
	"sort"
	"strings"
)

// RetrievalMetrics aggregates the complete suite of retrieval correctness metrics (R18.1 §4, §16, §17).
type RetrievalMetrics struct {
	RecallAt1              float64 `json:"recall_at_1"`
	RecallAt5              float64 `json:"recall_at_5"`
	RecallAt10             float64 `json:"recall_at_10"`
	RecallAt20             float64 `json:"recall_at_20"`
	RecallAt50             float64 `json:"recall_at_50"`
	CandidateRecallAt100   float64 `json:"candidate_recall_at_100"`
	RequiredEvidenceRecall float64 `json:"required_evidence_recall"`
	MRR                    float64 `json:"mrr"`
	NDCG                   float64 `json:"ndcg"`
	PSIAt5                 float64 `json:"psi_at_5"`
	PSIAt10                float64 `json:"psi_at_10"`
	PSIAt20                float64 `json:"psi_at_20"`
	ParaphraseRecallRange  float64 `json:"paraphrase_recall_range"`
}

// ComputePSI calculates the Paraphrase Stability Index (mean pairwise Jaccard similarity) across paraphrases (R18.1 §17).
// J(qi, qj) = |TopK(qi) ∩ TopK(qj)| / |TopK(qi) ∪ TopK(qj)|
func ComputePSI(candidateSets [][]string, k int) float64 {
	if len(candidateSets) < 2 {
		return 1.0
	}

	// Slice each candidate set to Top-K
	topKSets := make([]map[string]bool, len(candidateSets))
	for i, cands := range candidateSets {
		m := make(map[string]bool)
		for j, c := range cands {
			if j >= k {
				break
			}
			m[strings.ToLower(c)] = true
		}
		topKSets[i] = m
	}

	totalJaccard := 0.0
	pairCount := 0

	for i := 0; i < len(topKSets); i++ {
		for j := i + 1; j < len(topKSets); j++ {
			intersection := 0
			unionMap := make(map[string]bool)

			for item := range topKSets[i] {
				unionMap[item] = true
				if topKSets[j][item] {
					intersection++
				}
			}
			for item := range topKSets[j] {
				unionMap[item] = true
			}

			if len(unionMap) > 0 {
				totalJaccard += float64(intersection) / float64(len(unionMap))
			} else {
				totalJaccard += 1.0
			}
			pairCount++
		}
	}

	if pairCount == 0 {
		return 1.0
	}
	return totalJaccard / float64(pairCount)
}

// ComputeRequiredEvidenceRecall computes the fraction of required evidence files retrieved in TopK (R18.1 §16).
func ComputeRequiredEvidenceRecall(retrieved []string, required []string) float64 {
	if len(required) == 0 {
		return 1.0
	}
	retMap := make(map[string]bool)
	for _, r := range retrieved {
		retMap[strings.ToLower(r)] = true
	}

	matched := 0
	for _, req := range required {
		if retMap[strings.ToLower(req)] {
			matched++
		}
	}
	return float64(matched) / float64(len(required))
}

// ComputeMRR computes Mean Reciprocal Rank for target evidence across queries.
func ComputeMRR(targetRanks []int) float64 {
	if len(targetRanks) == 0 {
		return 0.0
	}
	sumRR := 0.0
	for _, rank := range targetRanks {
		if rank > 0 {
			sumRR += 1.0 / float64(rank)
		}
	}
	return sumRR / float64(len(targetRanks))
}

// ComputeNDCG computes Normalized Discounted Cumulative Gain at K.
func ComputeNDCG(rankedMatches []bool, k int) float64 {
	if k <= 0 {
		k = len(rankedMatches)
	}
	dcg := 0.0
	idealMatches := make([]bool, len(rankedMatches))
	copy(idealMatches, rankedMatches)
	sort.Slice(idealMatches, func(i, j int) bool {
		return idealMatches[i] && !idealMatches[j]
	})

	idcg := 0.0
	for i := 0; i < len(rankedMatches) && i < k; i++ {
		if rankedMatches[i] {
			dcg += 1.0 / math.Log2(float64(i+2))
		}
		if idealMatches[i] {
			idcg += 1.0 / math.Log2(float64(i+2))
		}
	}
	if idcg == 0.0 {
		return 1.0
	}
	return dcg / idcg
}

// ComputeBootstrapCI computes 95% confidence intervals using 10,000 bootstrap resamples (R18.1 §41).
func ComputeBootstrapCI(scores []float64, iterations int) [2]float64 {
	if len(scores) == 0 {
		return [2]float64{0.0, 0.0}
	}
	if iterations <= 0 {
		iterations = 10000
	}

	means := make([]float64, iterations)
	n := len(scores)
	rng := rand.New(rand.NewSource(42))

	for b := 0; b < iterations; b++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += scores[rng.Intn(n)]
		}
		means[b] = sum / float64(n)
	}

	sort.Float64s(means)
	lowIdx := int(0.025 * float64(iterations))
	highIdx := int(0.975 * float64(iterations))

	return [2]float64{means[lowIdx], means[highIdx]}
}
