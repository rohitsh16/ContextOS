package allocator

import (
	"context"
	"math"
	"sort"

	"contextos/internal/model"
	"contextos/internal/textutil"
)

// InfoGainWeights configures the parameters for PR-11 Information-Gain estimation.
type InfoGainWeights struct {
	AlphaNovelty     float64 // a
	BetaBoundary     float64 // b
	GammaUncertainty float64 // c
	DeltaRedundancy  float64 // d
}

// DefaultInfoGainWeights returns standard parameters for information gain.
func DefaultInfoGainWeights() InfoGainWeights {
	return InfoGainWeights{
		AlphaNovelty:     0.35,
		BetaBoundary:     0.25,
		GammaUncertainty: 0.25,
		DeltaRedundancy:  0.15,
	}
}

// ComputeInformationGain implements the formal signal:
// IG(m; q) = a*Novelty(m) + b*BoundaryCoverage(m) + c*UncertaintyReduction(m) - d*Redundancy(m, C)
func ComputeInformationGain(cand model.Candidate, selected []model.Candidate, w InfoGainWeights) float64 {
	// 1. Novelty: penalize overlap with already selected tokens
	overlapWithSelected := 0.0
	for _, s := range selected {
		ov := textutil.Overlap(cand.Content, s.Content)
		if ov > overlapWithSelected {
			overlapWithSelected = ov
		}
	}
	novelty := 1.0 - overlapWithSelected

	// 2. Boundary Coverage: code definitions, types, and architectural boundaries
	boundaryCoverage := 0.2
	if cand.Kind == "decision" || cand.Kind == "constraint" {
		boundaryCoverage = 1.0
	} else if cand.Kind == "symbol" || cand.Kind == "type" {
		boundaryCoverage = 0.8
	} else if cand.Kind == "file" {
		boundaryCoverage = 0.5
	}

	// 3. Uncertainty Reduction: higher task affinity and confidence reduce uncertainty
	uncertaintyReduction := 0.6*cand.TaskAffinity + 0.4*cand.Confidence

	// 4. Redundancy penalty
	redundancy := overlapWithSelected

	ig := w.AlphaNovelty*novelty +
		w.BetaBoundary*boundaryCoverage +
		w.GammaUncertainty*uncertaintyReduction -
		w.DeltaRedundancy*redundancy

	return math.Max(0.01, ig)
}

// InfoGainSelector implements the v2 experimental allocator (PR-11).
type InfoGainSelector struct {
	Weights InfoGainWeights
}

// NewInfoGainSelector creates an Information-Gain selector.
func NewInfoGainSelector() *InfoGainSelector {
	return &InfoGainSelector{Weights: DefaultInfoGainWeights()}
}

// Select greedily chooses candidates maximizing marginal Information Gain per token.
func (s *InfoGainSelector) Select(ctx context.Context, candidates []model.Candidate, budget int) ([]model.Candidate, Diagnostics) {
	diag := Diagnostics{
		Budget:    budget,
		Algorithm: "InfoGainSelector-v2",
	}

	remaining := make([]model.Candidate, len(candidates))
	copy(remaining, candidates)

	var selected []model.Candidate
	spent := 0
	totalIG := 0.0

	for spent < budget && len(remaining) > 0 {
		bestIdx := -1
		bestDensity := -1.0
		var bestCand model.Candidate

		for i, cand := range remaining {
			if cand.Tokens <= 0 || spent+cand.Tokens > budget {
				continue
			}
			ig := ComputeInformationGain(cand, selected, s.Weights)
			density := ig / float64(cand.Tokens)

			if density > bestDensity {
				bestDensity = density
				bestIdx = i
				bestCand = cand
			}
		}

		if bestIdx == -1 {
			break // No candidate fits
		}

		selected = append(selected, bestCand)
		spent += bestCand.Tokens
		totalIG += bestDensity * float64(bestCand.Tokens)

		// Remove chosen candidate
		remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
	}

	diag.SelectedCount = len(selected)
	diag.SelectedTokens = spent
	if budget > 0 {
		diag.Utilization = float64(spent) / float64(budget)
	}
	diag.MarginalUtility = totalIG
	return selected, diag
}

// OptimizeJointCachePrefix implements PR-12 Joint Cache-Aware Allocation:
// max_P Savings(P) - lambda * |P| - mu * Staleness(P)
func OptimizeJointCachePrefix(candidates []model.Candidate, reuseFactor float64, uncachedRate, cachedRate float64) ([]model.Candidate, []model.Candidate) {
	if len(candidates) == 0 {
		return nil, nil
	}

	lambda := 0.0001 // Token length penalty
	mu := 0.05       // Staleness penalty weight

	// Sort candidates by staleness risk ascending (least stale first for prefix)
	sorted := make([]model.Candidate, len(candidates))
	copy(sorted, candidates)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].StaleRisk < sorted[j].StaleRisk
	})

	bestScore := -1e9
	bestSplit := 0

	runningTokens := 0
	runningStaleness := 0.0

	for k := 1; k <= len(sorted); k++ {
		c := sorted[k-1]
		runningTokens += c.Tokens
		runningStaleness += c.StaleRisk

		// Savings(P) = N_reuse * |P| * (p_uncached - p_cached)
		savings := reuseFactor * float64(runningTokens) * (uncachedRate - cachedRate) / 1e6
		lengthPenalty := lambda * float64(runningTokens)
		stalePenalty := mu * runningStaleness

		netValue := savings - lengthPenalty - stalePenalty
		if netValue > bestScore {
			bestScore = netValue
			bestSplit = k
		}
	}

	if bestSplit <= 0 {
		return nil, candidates
	}
	return sorted[:bestSplit], sorted[bestSplit:]
}
