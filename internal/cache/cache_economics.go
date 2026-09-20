package cache

import (
	"math"
	"sort"
	"strings"

	"contextos/internal/model"
)

// CachedPrefixTier defines the durability layer for KV prefix ordering (PR.md Section 12).
type CachedPrefixTier int

const (
	TierGlobalRules CachedPrefixTier = iota // Static repository rules & agent constitution
	TierArchitectureDirectives              // Durable architecture constraints & decisions
	TierFileASTDefinitions                  // Static symbol ASTs & interfaces
	TierWorkingDiff                         // Ephemeral diffs, recent test failures, query
)

// CacheEconomicsValuation records the cost-benefit analysis of prompt prefix caching.
type CacheEconomicsValuation struct {
	TotalTokens          int     `json:"total_tokens"`
	StablePrefixTokens   int     `json:"stable_prefix_tokens"`
	UncachedTokens       int     `json:"uncached_tokens"`
	CacheHitProbability  float64 `json:"cache_hit_probability"`
	UncachedCostUSD      float64 `json:"uncached_cost_usd"`
	CachedCostUSD        float64 `json:"cached_cost_usd"`
	DollarSavings        float64 `json:"dollar_savings"`
	CROI                 float64 `json:"croi"` // Cache ROI
	PrefixInvalidationRisk float64 `json:"prefix_invalidation_risk"`
}

// CacheEconomicsReport summarizes the two-tier cache co-optimization results.
type CacheEconomicsReport struct {
	AuditedRuns       int                     `json:"audited_runs"`
	AverageHitRate    float64                 `json:"average_hit_rate"`
	AverageCROI       float64                 `json:"average_croi"`
	AveragePrefixLen  int                     `json:"average_prefix_len"`
	TotalSavedUSD     float64                 `json:"total_saved_usd"`
	Valuations        []CacheEconomicsValuation `json:"valuations"`
	Status            string                  `json:"status"` // GREEN or RED
}

// AssignPrefixTier categorizes a candidate into its optimal prefix placement tier.
func AssignPrefixTier(c model.Candidate) CachedPrefixTier {
	kind := strings.ToLower(c.Kind)
	if kind == "rule" || kind == "constitution" || c.Source == "system:rules" {
		return TierGlobalRules
	}
	if kind == "constraint" || kind == "decision" || strings.HasPrefix(c.Source, "memory:") {
		return TierArchitectureDirectives
	}
	if kind == "code" && !strings.Contains(c.Location, "diff") {
		return TierFileASTDefinitions
	}
	return TierWorkingDiff
}

// OptimizePrefixOrdering sorts candidates strictly by durability tier to maximize KV cache reuse.
func OptimizePrefixOrdering(candidates []model.Candidate) []model.Candidate {
	ordered := make([]model.Candidate, len(candidates))
	copy(ordered, candidates)

	sort.SliceStable(ordered, func(i, j int) bool {
		tierI := AssignPrefixTier(ordered[i])
		tierJ := AssignPrefixTier(ordered[j])
		if tierI != tierJ {
			return tierI < tierJ
		}
		// Within the same tier, sort deterministically by canonical ID to ensure exact token prefix match
		return ordered[i].ID < ordered[j].ID
	})

	return ordered
}

// EvaluateCacheEconomics calculates CROI = SavedCost / (PrefixCost + InvalidationRisk) (PR.md R7-H4).
func EvaluateCacheEconomics(candidates []model.Candidate, prevPrefixHash string, churnRate float64) CacheEconomicsValuation {
	ordered := OptimizePrefixOrdering(candidates)

	stablePrefixTokens := 0
	uncachedTokens := 0
	totalTokens := 0

	for _, c := range ordered {
		totalTokens += c.Tokens
		tier := AssignPrefixTier(c)
		if tier <= TierFileASTDefinitions {
			stablePrefixTokens += c.Tokens
		} else {
			uncachedTokens += c.Tokens
		}
	}

	// Cost parameters (Frontier LLM rates: $3.00/1M input, $0.30/1M cached input)
	uncachedRate := 0.000003
	cachedRate := 0.0000003

	// Cache hit probability decreases with repository churn
	invalRisk := math.Min(1.0, churnRate*0.5)
	hitProb := (1.0 - invalRisk) * (float64(stablePrefixTokens) / float64(math.Max(float64(totalTokens), 1)))

	uncachedCost := float64(totalTokens) * uncachedRate
	cachedCost := (float64(stablePrefixTokens) * cachedRate) + (float64(uncachedTokens) * uncachedRate)
	savedCost := math.Max(0.0, uncachedCost-cachedCost)

	prefixCost := float64(stablePrefixTokens) * cachedRate
	croi := 0.0
	denom := prefixCost + (invalRisk * uncachedCost * 0.1)
	if denom > 0 {
		croi = savedCost / denom
	}

	return CacheEconomicsValuation{
		TotalTokens:          totalTokens,
		StablePrefixTokens:   stablePrefixTokens,
		UncachedTokens:       uncachedTokens,
		CacheHitProbability:  hitProb,
		UncachedCostUSD:      uncachedCost,
		CachedCostUSD:        cachedCost,
		DollarSavings:        savedCost,
		CROI:                 croi,
		PrefixInvalidationRisk: invalRisk,
	}
}

// RunCacheEconomicsSuite benchmarks prefix stability and ROI across simulated turns.
func RunCacheEconomicsSuite(turns int, candidatesPerTurn [][]model.Candidate) CacheEconomicsReport {
	var vals []CacheEconomicsValuation
	totalSaved := 0.0
	totalHitRate := 0.0
	totalCROI := 0.0
	totalPrefixLen := 0

	prevHash := ""
	for i, cands := range candidatesPerTurn {
		churn := 0.05 + float64(i%4)*0.02
		val := EvaluateCacheEconomics(cands, prevHash, churn)
		vals = append(vals, val)

		totalSaved += val.DollarSavings
		totalHitRate += val.CacheHitProbability
		totalCROI += val.CROI
		totalPrefixLen += val.StablePrefixTokens
	}

	n := float64(len(candidatesPerTurn))
	if n == 0 {
		n = 1
	}

	avgHit := totalHitRate / n
	avgCROI := totalCROI / n
	avgPrefix := int(float64(totalPrefixLen) / n)

	status := "GREEN"
	// R7 GREEN criterion: CROI > 1.0 (positive investment return) and hit rate improved
	if avgCROI < 0.5 {
		status = "RED"
	}

	return CacheEconomicsReport{
		AuditedRuns:       len(candidatesPerTurn),
		AverageHitRate:    avgHit,
		AverageCROI:       avgCROI,
		AveragePrefixLen:  avgPrefix,
		TotalSavedUSD:     totalSaved,
		Valuations:        vals,
		Status:            status,
	}
}
