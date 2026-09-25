package retrieval

import (
	"sort"
	"strings"
)

// MSEStats records empirical metrics from the Minimum Sufficient Evidence optimization (R18 §32, §35).
type MSEStats struct {
	InitialNodes        int     `json:"initial_nodes"`
	SelectedNodes       int     `json:"selected_nodes"`
	InitialTokens       int     `json:"initial_tokens"`
	SelectedTokens      int     `json:"selected_tokens"`
	CompressionRatio    float64 `json:"compression_ratio"`
	MinimalityRate      float64 `json:"minimality_rate"`
	SufficiencyCoverage float64 `json:"sufficiency_coverage"`
	SynergyScore        float64 `json:"synergy_score"`
}

// OptimizeMSE finds the minimal authoritative evidence subset E* that preserves sufficiency (R18 §22, §35).
// Solves: E* = argmin Cost(E) s.t. Sufficiency(E, q) >= tau_s, Validity(E) = 1.
func OptimizeMSE(
	pool []*EvidenceNode,
	contract QueryContract,
	tokenBudget int,
	minCoverage float64,
) ([]*EvidenceNode, MSEStats) {
	if minCoverage <= 0 {
		minCoverage = 0.8
	}
	if tokenBudget <= 0 {
		tokenBudget = 4000
	}

	initialTokens := 0
	for _, n := range pool {
		initialTokens += n.Tokens
	}

	// 1. Filter strictly for validity (Validity(E) = 1)
	validPool := make([]*EvidenceNode, 0, len(pool))
	for _, n := range pool {
		if n.Provenance.Eligible && n.Authority > 0.0 {
			validPool = append(validPool, n)
		}
	}

	// 2. Greedy density sorting
	// Marginal density = (standalone value + authority) / tokens
	type scoredNode struct {
		node    *EvidenceNode
		density float64
	}
	var scored []scoredNode
	for _, n := range validPool {
		cost := float64(n.Tokens)
		if cost <= 0 {
			cost = 1.0
		}
		// Value = Authority * Relevance bonus if path matches required evidence
		val := n.Authority
		for _, req := range contract.RequiredEvidence {
			if strings.EqualFold(n.Path, req) || strings.Contains(strings.ToLower(n.Path), strings.ToLower(req)) {
				val += 2.0
				break
			}
		}
		scored = append(scored, scoredNode{
			node:    n,
			density: val / cost,
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].density > scored[j].density
	})

	// 3. Accumulate candidates until sufficiency is satisfied under token budget
	var selected []*EvidenceNode
	usedTokens := 0

	for _, sn := range scored {
		if usedTokens+sn.node.Tokens > tokenBudget && len(selected) > 0 {
			continue
		}
		selected = append(selected, sn.node)
		usedTokens += sn.node.Tokens

		// Check if current selection meets sufficiency
		suff := EvaluateSufficiency(selected, contract, minCoverage)
		if suff.Sufficient {
			break
		}
	}

	// 4. Prune redundant nodes to enforce Minimality (R18 §32)
	// For each e in E*, test if Sufficiency(E* \ {e}) >= tau_s.
	// If so, e was redundant and can be dropped without losing sufficiency.
	var minimal []*EvidenceNode
	necessaryCount := 0

	for i := 0; i < len(selected); i++ {
		target := selected[i]
		// Create trial set without target
		trial := make([]*EvidenceNode, 0, len(selected)-1)
		for j := 0; j < len(selected); j++ {
			if i != j {
				trial = append(trial, selected[j])
			}
		}

		trialSuff := EvaluateSufficiency(trial, contract, minCoverage)
		if trialSuff.Sufficient {
			// target is redundant, skip it!
			continue
		}
		// target is necessary for sufficiency!
		minimal = append(minimal, target)
		necessaryCount++
	}

	// If pruning made it empty or insufficient, revert to selected
	if len(minimal) == 0 || !EvaluateSufficiency(minimal, contract, minCoverage).Sufficient {
		minimal = selected
	}

	finalTokens := 0
	for _, m := range minimal {
		finalTokens += m.Tokens
	}

	compression := 1.0
	if finalTokens > 0 {
		compression = float64(initialTokens) / float64(finalTokens)
	}

	minimalityRate := 1.0
	if len(minimal) > 0 {
		// Re-evaluate minimality on final set
		strictlyNecessary := 0
		for i := range minimal {
			trial := make([]*EvidenceNode, 0, len(minimal)-1)
			for j := range minimal {
				if i != j {
					trial = append(trial, minimal[j])
				}
			}
			if !EvaluateSufficiency(trial, contract, minCoverage).Sufficient {
				strictlyNecessary++
			}
		}
		minimalityRate = float64(strictlyNecessary) / float64(len(minimal))
	}

	// 5. Calculate synergy score (R18 §37)
	synergy := 0.0
	for i := 0; i < len(minimal); i++ {
		for j := i + 1; j < len(minimal); j++ {
			a := minimal[i]
			b := minimal[j]
			// Caller + callee or test + target synergy
			if (a.Symbol != "" && strings.Contains(b.Content, a.Symbol)) ||
				(b.Symbol != "" && strings.Contains(a.Content, b.Symbol)) {
				synergy += 1.0
			}
			if strings.HasSuffix(a.Path, "_test.go") && strings.TrimSuffix(a.Path, "_test.go") == strings.TrimSuffix(b.Path, ".go") {
				synergy += 1.5
			}
		}
	}

	finalSuff := EvaluateSufficiency(minimal, contract, minCoverage)

	stats := MSEStats{
		InitialNodes:        len(pool),
		SelectedNodes:       len(minimal),
		InitialTokens:       initialTokens,
		SelectedTokens:      finalTokens,
		CompressionRatio:    compression,
		MinimalityRate:      minimalityRate,
		SufficiencyCoverage: finalSuff.Coverage,
		SynergyScore:        synergy,
	}

	return minimal, stats
}
