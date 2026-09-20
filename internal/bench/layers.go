package bench

import (
	"math"
	"strings"

	"contextos/internal/model"
)

// LayerA captures retrieval-stage quality metrics.
type LayerA struct {
	RecallAt1  float64 `json:"recall_at_1"`
	RecallAt3  float64 `json:"recall_at_3"`
	RecallAt5  float64 `json:"recall_at_5"`
	RecallAt10 float64 `json:"recall_at_10"`
	MRR        float64 `json:"mrr"`
	NDCG       float64 `json:"ndcg"`
	MAP        float64 `json:"map"`
}

// LayerB captures context selection and budget packing efficiency metrics.
type LayerB struct {
	TokenUsage        float64 `json:"token_usage"`
	Coverage          float64 `json:"coverage"`
	Redundancy        float64 `json:"redundancy"`
	Utility           float64 `json:"utility"`
	BudgetUtilization float64 `json:"budget_utilization"`
}

// LayerC captures downstream agent performance and behavioral metrics.
type LayerC struct {
	TaskSuccess     float64 `json:"task_success"`
	TestPass        float64 `json:"test_pass"`
	RegressionRate  float64 `json:"regression_rate"`
	HandoffSuccess  float64 `json:"handoff_success"`
	RediscoveryRate float64 `json:"rediscovery_rate"`
}

// LayerD captures system economics, token caching, and operational costs.
type LayerD struct {
	InputTokens    float64 `json:"input_tokens"`
	CachedTokens   float64 `json:"cached_tokens"`
	UncachedTokens float64 `json:"uncached_tokens"`
	OutputTokens   float64 `json:"output_tokens"`
	LatencyMs      float64 `json:"latency_ms"`
	EstimatedCost  float64 `json:"estimated_cost_usd"`
}

// LayerMetrics bundles all 4 scientific evaluation layers for a strategy.
type LayerMetrics struct {
	A LayerA `json:"layer_a_retrieval"`
	B LayerB `json:"layer_b_selection"`
	C LayerC `json:"layer_c_agent_outcome"`
	D LayerD `json:"layer_d_economics"`
}

// ComputeRecallAtK computes Recall@K given ranked retrieved candidate IDs and ground truth relevant IDs.
func ComputeRecallAtK(rankedIDs []string, relevantIDs map[string]bool, k int) float64 {
	if len(relevantIDs) == 0 {
		return 1.0
	}
	limit := k
	if len(rankedIDs) < limit {
		limit = len(rankedIDs)
	}
	hits := 0
	for i := 0; i < limit; i++ {
		if relevantIDs[rankedIDs[i]] {
			hits++
		}
	}
	return float64(hits) / float64(len(relevantIDs))
}

// ComputeMRR computes Reciprocal Rank for the first relevant item found in rankedIDs.
func ComputeMRR(rankedIDs []string, relevantIDs map[string]bool) float64 {
	for i, id := range rankedIDs {
		if relevantIDs[id] {
			return 1.0 / float64(i+1)
		}
	}
	return 0.0
}

// ComputeNDCG computes Normalized Discounted Cumulative Gain at rank limit K.
func ComputeNDCG(rankedIDs []string, relevantIDs map[string]bool, k int) float64 {
	if len(relevantIDs) == 0 {
		return 1.0
	}
	limit := k
	if len(rankedIDs) < limit {
		limit = len(rankedIDs)
	}

	dcg := 0.0
	for i := 0; i < limit; i++ {
		rel := 0.0
		if relevantIDs[rankedIDs[i]] {
			rel = 1.0
		}
		dcg += (math.Pow(2, rel) - 1.0) / math.Log2(float64(i+2))
	}

	// Ideal DCG
	idcg := 0.0
	idealHits := len(relevantIDs)
	if limit < idealHits {
		idealHits = limit
	}
	for i := 0; i < idealHits; i++ {
		idcg += 1.0 / math.Log2(float64(i+2))
	}

	if idcg == 0.0 {
		return 0.0
	}
	return dcg / idcg
}

// ComputeMAP computes Average Precision for a single ranked list.
func ComputeMAP(rankedIDs []string, relevantIDs map[string]bool) float64 {
	if len(relevantIDs) == 0 {
		return 1.0
	}
	hits := 0
	sumP := 0.0
	for i, id := range rankedIDs {
		if relevantIDs[id] {
			hits++
			pAtI := float64(hits) / float64(i+1)
			sumP += pAtI
		}
	}
	if hits == 0 {
		return 0.0
	}
	return sumP / float64(len(relevantIDs))
}

// ComputeRedundancy computes average token overlap across all pairwise combinations of selected candidates.
func ComputeRedundancy(candidates []model.Candidate) float64 {
	if len(candidates) <= 1 {
		return 0.0
	}

	// Tokenize candidates into word sets
	sets := make([]map[string]bool, len(candidates))
	for i, c := range candidates {
		s := make(map[string]bool)
		for _, w := range strings.Fields(strings.ToLower(c.Content)) {
			s[w] = true
		}
		sets[i] = s
	}

	pairCount := 0
	simSum := 0.0
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			pairCount++
			simSum += jaccard(sets[i], sets[j])
		}
	}
	if pairCount == 0 {
		return 0.0
	}
	return simSum / float64(pairCount)
}

func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}
	intersection := 0
	for k := range a {
		if b[k] {
			intersection++
		}
	}
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0.0
	}
	return float64(intersection) / float64(union)
}

// ModelPricing represents token cost rates per million tokens.
type ModelPricing struct {
	UncachedInputPerM float64 // e.g. $3.00
	CachedInputPerM   float64 // e.g. $0.30
	OutputPerM        float64 // e.g. $15.00
}

// DefaultPricing returns standard tier pricing (Anthropic Sonnet / OpenAI GPT-4o style).
func DefaultPricing() ModelPricing {
	return ModelPricing{
		UncachedInputPerM: 3.00,
		CachedInputPerM:   0.30,
		OutputPerM:        15.00,
	}
}

// EstimateCostUSD calculates prompt and completion costs in USD.
func (p ModelPricing) EstimateCostUSD(uncachedInput, cachedInput, outputTokens float64) float64 {
	uncachedCost := (uncachedInput / 1_000_000.0) * p.UncachedInputPerM
	cachedCost := (cachedInput / 1_000_000.0) * p.CachedInputPerM
	outputCost := (outputTokens / 1_000_000.0) * p.OutputPerM
	return uncachedCost + cachedCost + outputCost
}
