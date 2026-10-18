package dreamrsi

import (
	"math"
)

// ExplorationPolicy defines the parameterized orchestration and exploration policy.
// Under Dream-RSI (arXiv:2609.14858), this policy is iteratively refined offline
// inside historical replay simulators ("evolving worlds") while base model weights remain frozen.
type ExplorationPolicy struct {
	ID                  string   `json:"id"`
	Version             int      `json:"version"`
	Description         string   `json:"description,omitempty"`

	// 6-Pass Scorer Linear Weights (ASC-1)
	WeightSemantic      float64  `json:"weight_semantic"`
	WeightLexical       float64  `json:"weight_lexical"`
	WeightTaskAffinity  float64  `json:"weight_task_affinity"`
	WeightAuthority     float64  `json:"weight_authority"`
	WeightEvidence      float64  `json:"weight_evidence"`
	WeightReuse         float64  `json:"weight_reuse"`
	WeightGraph         float64  `json:"weight_graph"`
	WeightFreshnessRisk float64  `json:"weight_freshness_risk"`
	WeightLengthPenalty float64  `json:"weight_length_penalty"`

	// RRF (Reciprocal Rank Fusion) parameters
	RRFConstant         float64  `json:"rrf_constant"` // default 60.0

	// Exploration & Pruning Bounds
	PruneThreshold      float64  `json:"prune_threshold"`       // Min density to include
	MinAuthorityFloor   float64  `json:"min_authority_floor"`    // Hard floor below which rejected (default 0.60)
	BacktrackPenalty    float64  `json:"backtrack_penalty"`      // Exploration backtracking penalty
	ExplorationRatio    float64  `json:"exploration_ratio"`      // Context budget fraction dedicated to novel exploration
	StageEarlyExitK     int      `json:"stage_early_exit_k"`     // Max candidates before short-circuiting downstream stages
	RetrievalStages     []string `json:"retrieval_stages"`       // Active stage routing sequence
}

// DefaultBaselinePolicy returns the production baseline exploration policy,
// matching the hardcoded ASC-1 allocator constants exactly.
func DefaultBaselinePolicy() ExplorationPolicy {
	return ExplorationPolicy{
		ID:                  "asc1-baseline",
		Version:             1,
		Description:         "Standard deterministic ASC-1 6-pass allocator baseline",
		WeightSemantic:      1.15,
		WeightLexical:       1.00,
		WeightTaskAffinity:  0.90,
		WeightAuthority:     0.75,
		WeightEvidence:      0.60,
		WeightReuse:         0.45,
		WeightGraph:         0.35,
		WeightFreshnessRisk: 1.50,
		WeightLengthPenalty: 0.15,
		RRFConstant:         60.0,
		PruneThreshold:      0.0,
		MinAuthorityFloor:   0.60,
		BacktrackPenalty:    0.25,
		ExplorationRatio:    0.15,
		StageEarlyExitK:     25,
		RetrievalStages: []string{
			"exact_path",
			"symbol_ast",
			"entity",
			"bm25",
			"semantic_hash",
			"graph_expansion",
			"rrf_rerank",
		},
	}
}

// ScoreCandidate computes the preliminary linear utility score for a candidate
// under this exploration policy.
func (p ExplorationPolicy) ScoreCandidate(sem, lex, affinity, auth, evidence, reuse, graph, freshRisk float64, tokens int) (float64, float64) {
	tok := math.Max(1, float64(tokens))
	score := p.WeightSemantic*sem +
		p.WeightLexical*lex +
		p.WeightTaskAffinity*affinity +
		p.WeightAuthority*auth +
		p.WeightEvidence*evidence +
		p.WeightReuse*reuse +
		p.WeightGraph*graph -
		p.WeightFreshnessRisk*freshRisk -
		p.WeightLengthPenalty*math.Min(1, tok/4000.0)

	density := score / tok
	if auth < p.MinAuthorityFloor {
		density = math.Inf(-1)
	}
	return score, density
}

// RRFScore computes rank fusion score for this policy with its calibrated damping constant.
func (p ExplorationPolicy) RRFScore(semRank, lexRank, affRank int) float64 {
	k := p.RRFConstant
	if k <= 0 {
		k = 60.0
	}
	return 1.0/(k+float64(semRank)) + 1.0/(k+float64(lexRank)) + 1.0/(k+float64(affRank))
}

// Mutate generates a perturbed candidate policy for offline simulation in the Dream loop.
func (p ExplorationPolicy) Mutate(seed int64, perturbationScale float64) ExplorationPolicy {
	// Deterministic pseudo-random perturbation based on seed
	next := p
	next.Version++
	next.ID = "dream-mutant"

	clamp := func(val, delta, minVal, maxVal float64) float64 {
		res := val + delta*perturbationScale
		if res < minVal {
			return minVal
		}
		if res > maxVal {
			return maxVal
		}
		return res
	}

	// Pseudo-random deterministic deltas
	d1 := math.Sin(float64(seed)*1.13) * 0.20
	d2 := math.Cos(float64(seed)*2.27) * 0.20
	d3 := math.Sin(float64(seed)*3.41) * 0.15
	d4 := math.Cos(float64(seed)*4.59) * 0.15
	d5 := math.Sin(float64(seed)*5.73) * 0.15
	d6 := math.Cos(float64(seed)*6.89) * 0.25

	next.WeightSemantic = clamp(p.WeightSemantic, d1, 0.5, 2.5)
	next.WeightLexical = clamp(p.WeightLexical, d2, 0.5, 2.0)
	next.WeightGraph = clamp(p.WeightGraph, d3, 0.1, 1.5)
	next.WeightTaskAffinity = clamp(p.WeightTaskAffinity, d4, 0.2, 2.0)
	next.WeightFreshnessRisk = clamp(p.WeightFreshnessRisk, d5, 0.5, 3.0)
	next.PruneThreshold = clamp(p.PruneThreshold, d6*0.01, 0.0, 0.10)

	return next
}
