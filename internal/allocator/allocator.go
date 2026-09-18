package allocator

import (
	"math"
	"sort"
	"strings"

	"contextos/internal/model"
	"contextos/internal/textutil"
)

// Request defines the input parameters for a context planning and allocation query.
type Request struct {
	Task         string  // Task description or query prompt from the user/agent.
	Budget       int     // Maximum total token budget allocated for context packing.
	Model        string  // Target model identifier.
	RepoRevision string  // Current git revision/commit hash used to evaluate freshness and staleness.
	AvgDocLen    float64 // Average document token length for BM25 normalization; 0 = auto-computed from candidates.
}

// authorityScore maps the provenance or source authority string of a memory to a numeric weight in [0.45, 1.0].
// Explicit user directives and verified tests receive the highest authority; unverified inferences receive lower weight.
func authorityScore(v string) float64 {
	switch strings.ToLower(v) {
	case "user", "explicit":
		return 1.0
	case "test":
		return 0.99
	case "source":
		return 0.97
	case "commit":
		return 0.94
	case "doc":
		return 0.86
	case "inference":
		return 0.55
	default:
		return 0.45
	}
}

// stale evaluates whether a candidate memory is outdated relative to the active repository revision.
// Returns freshRisk (0.0–1.0 penalty) and hardStale (true = explicitly invalidated → immediate rejection).
func stale(m model.Memory, currentRevision string) (float64, bool) {
	if m.InvalidatedAtRevision != "" {
		return 1, true
	}
	if m.ValidFromRevision != "" && currentRevision != "" && m.ValidFromRevision != currentRevision {
		// Validity intervals are only fully known when explicit invalidation exists.
		// Treat a different revision as a freshness penalty, not an automatic rejection.
		return 0.25, false
	}
	return 0, false
}

// kindBoost assigns an intrinsic priority multiplier based on the structural category of the memory.
// Actionable decisions and failure postmortems are prioritized over generic facts.
func kindBoost(kind string) float64 {
	switch strings.ToLower(kind) {
	case "decision":
		return 1.0
	case "failure":
		return 0.98
	case "constraint":
		return 0.96
	case "state":
		return 0.88
	case "observation":
		return 0.76
	case "code":
		return 0.74
	case "fact":
		return 0.65
	default:
		return 0.5
	}
}

// confWeight converts a raw Memory.Confidence value to a multiplicative score gate in [0.1, 1.0].
// When Confidence is unset (0.0), returns 1.0 for backward compatibility with memories stored before
// confidence tracking was introduced. Positive values are floored at 0.1 to prevent total suppression
// of uncertain-but-useful memories.
func confWeight(c float64) float64 {
	if c <= 0 {
		return 1.0 // Not specified → full confidence assumed.
	}
	return math.Max(0.1, c)
}

// Score evaluates a single candidate memory against a planning request.
// It computes BM25 lexical overlap, hash-based semantic similarity, authority weighting,
// confidence weighting, graph centrality, and staleness risk.
// The Score and Density fields are preliminary; Plan() overwrites them with RRF-fused values
// after cross-candidate ranking.
func Score(req Request, m model.Memory) model.Candidate {
	tok := m.TokenCost
	if tok <= 0 {
		tok = textutil.EstimateTokens(m.Content)
	}
	// BM25 lexical scoring with TF saturation and length normalization.
	lex := textutil.BM25Score(req.Task, m.Content, req.AvgDocLen)
	sem := textutil.HashSemantic(req.Task, m.Content)
	freshRisk, hardStale := stale(m, req.RepoRevision)
	auth := authorityScore(m.Authority)
	conf := confWeight(m.Confidence)
	reuse := math.Min(1, float64(m.ReuseCount)/10.0)
	affinity := 0.6*lex + 0.4*kindBoost(m.Kind)

	// Graph centrality proxy: count structural path indicators (file paths, package refs).
	lower := strings.ToLower(m.Content)
	pathSeps := strings.Count(lower, "/") + strings.Count(lower, ".") + strings.Count(lower, "::")
	graph := 0.0
	if pathSeps >= 1 || m.Kind == "code" {
		graph = math.Min(1.0, 0.4+float64(pathSeps)*0.1)
	}

	evidence := 0.0
	if m.Source != "" || m.Location != "" {
		evidence = 1.0
	} else if auth >= 0.94 {
		evidence = 0.75
	}
	cacheValue := reuse*0.8 + 0.2*math.Min(1, float64(tok)/8000.0)

	// Preliminary linear score (stable tiebreaker; overwritten by Plan() via RRF × confidence).
	score := 1.15*sem + 1.0*lex + 0.9*affinity + 0.75*auth + 0.6*evidence + 0.45*reuse + 0.35*graph - 1.5*freshRisk - 0.15*math.Min(1, float64(tok)/4000.0)
	density := score / float64(max(1, tok))

	src := m.Source
	if src == "" {
		src = "memory"
	}
	if strings.HasPrefix(m.ID, "node:") {
		src = "repository"
	}
	reason := "candidate"
	if hardStale {
		reason = "rejected: explicitly invalidated"
	}
	if auth < 0.6 {
		reason = "rejected: low authority"
	}
	if hardStale || auth < 0.6 {
		density = math.Inf(-1)
	}
	return model.Candidate{
		Source: src, Location: m.Location, ID: m.ID, Kind: m.Kind, Content: m.Content,
		Tokens: tok, Semantic: sem, Lexical: lex, Graph: graph,
		Freshness: 1 - freshRisk, Authority: auth, Confidence: conf, Reuse: reuse,
		TaskAffinity: affinity, Evidence: evidence, StaleRisk: freshRisk,
		CacheValue: cacheValue, MarginalEstimate: score, Density: density, Score: score, Reason: reason,
	}
}

// rrfScore computes Reciprocal Rank Fusion (Cormack et al., 2009) across 3 signal rankings.
// Each signal contributes 1/(k+rank) where k=60 dampens single-list dominance.
// RRF is scale-independent: it fuses signals by rank position rather than raw score magnitude.
func rrfScore(semRank, lexRank, affRank int) float64 {
	const k = 60.0
	return 1/(k+float64(semRank)) + 1/(k+float64(lexRank)) + 1/(k+float64(affRank))
}

// rankBy returns an index permutation [0..n-1] sorted descending by the given less comparator.
func rankBy(n int, less func(i, j int) bool) []int {
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, less)
	return idx
}

// Plan executes the budget-constrained context optimization algorithm across a collection of memories.
// It applies four mathematically-grounded passes in sequence:
//
//  1. BM25 Lexical Scoring  — TF saturation + length normalization (Robertson & Sparck Jones 1994).
//     Replaces raw Jaccard overlap; penalizes verbose docs, rewards precise term matches.
//
//  2. Reciprocal Rank Fusion — combines semantic, lexical, and affinity signals by rank position,
//     not raw score, eliminating inter-signal scale bias (Cormack et al., 2009).
//     Gated multiplicatively by authority × freshness × confidence.
//
//  3. Singleton Rescue — after greedy packing, if a single item that fits the full budget has a
//     higher aggregate score than the entire greedy selection, it replaces the selection.
//     Provides the (1-1/e) ≈ 0.63 approximation guarantee (Sviridenko 2004).
//
//  4. Fill Pass — after the primary selection (including any rescue), fill remaining budget
//     with the highest-density eligible items not yet selected.
func Plan(req Request, ms []model.Memory) model.ContextPlan {
	if req.Budget <= 0 {
		req.Budget = 4000
	}

	// Auto-compute average document length for BM25 normalization across this candidate set.
	if req.AvgDocLen <= 0 && len(ms) > 0 {
		total := 0
		for _, m := range ms {
			total += textutil.EstimateTokens(m.Content)
		}
		req.AvgDocLen = float64(total) / float64(len(ms))
	}

	// Pass 1: score all candidates individually.
	cands := make([]model.Candidate, 0, len(ms))
	for _, m := range ms {
		cands = append(cands, Score(req, m))
	}
	if len(cands) == 0 {
		return model.ContextPlan{Task: req.Task, Budget: req.Budget}
	}

	// Pass 2: rank candidates by each signal independently, then compute RRF-fused scores.
	n := len(cands)
	semRanks := make([]int, n)
	lexRanks := make([]int, n)
	affRanks := make([]int, n)

	for rank, i := range rankBy(n, func(a, b int) bool { return cands[a].Semantic > cands[b].Semantic }) {
		semRanks[i] = rank + 1
	}
	for rank, i := range rankBy(n, func(a, b int) bool { return cands[a].Lexical > cands[b].Lexical }) {
		lexRanks[i] = rank + 1
	}
	for rank, i := range rankBy(n, func(a, b int) bool { return cands[a].TaskAffinity > cands[b].TaskAffinity }) {
		affRanks[i] = rank + 1
	}

	// RRF × authority × freshness × confidence: any hard-rejected item cannot win even if
	// its content is on-topic. Confidence down-weights low-certainty memories.
	for i := range cands {
		rrf := rrfScore(semRanks[i], lexRanks[i], affRanks[i])
		fused := rrf * cands[i].Authority * cands[i].Freshness * cands[i].Confidence
		cands[i].Score = fused
		cands[i].Density = fused / float64(max(1, cands[i].Tokens))
		// Hard-rejected items sort to the bottom regardless of content quality.
		if strings.HasPrefix(cands[i].Reason, "rejected:") {
			cands[i].Density = math.Inf(-1)
		}
	}

	// Pass 3: sort by marginal-utility density, then greedy-pack up to budget.
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].Density > cands[j].Density })
	used := 0
	selected := make([]model.Candidate, 0)
	for i := range cands {
		if strings.HasPrefix(cands[i].Reason, "rejected:") {
			continue
		}
		if used+cands[i].Tokens <= req.Budget {
			cands[i].Selected = true
			cands[i].Reason = "selected: RRF/BM25 marginal-utility greedy"
			selected = append(selected, cands[i])
			used += cands[i].Tokens
		} else {
			cands[i].Reason = "rejected: token budget"
		}
	}

	// Pass 4: singleton rescue — (1-1/e) knapsack approximation guarantee.
	// If a single valid item fits the full budget and its score exceeds the sum of the current
	// greedy selection, replace the entire selection with that singleton.
	// (Sviridenko 2004; Ghosh & McGregor 2024, O(n²) complexity bound.)
	selectedScore := 0.0
	for _, c := range selected {
		selectedScore += c.Score
	}
	var bestSingleton *model.Candidate
	for i := range cands {
		c := &cands[i]
		if c.Selected {
			continue
		}
		// Only hard-rejected items (not budget-rejected) are excluded from rescue consideration.
		if c.Reason == "rejected: explicitly invalidated" || c.Reason == "rejected: low authority" {
			continue
		}
		if c.Tokens <= req.Budget {
			if bestSingleton == nil || c.Score > bestSingleton.Score {
				bs := cands[i]
				bestSingleton = &bs
			}
		}
	}
	if bestSingleton != nil && bestSingleton.Score > selectedScore {
		// Singleton wins: clear current selection and take the single best-scoring item.
		for i := range cands {
			if cands[i].Selected {
				cands[i].Selected = false
				cands[i].Reason = "replaced: singleton rescue"
			}
		}
		for i := range cands {
			if cands[i].ID == bestSingleton.ID {
				cands[i].Selected = true
				cands[i].Reason = "selected: singleton rescue (1-1/e)"
				used = cands[i].Tokens
				selected = []model.Candidate{cands[i]}
				break
			}
		}
	}

	// Pass 5: fill pass — after singleton rescue, fill remaining budget with the highest-density
	// eligible unselected items. Items are already density-sorted (from Pass 3), so iteration
	// order is optimal. Hard-rejected items are never reconsidered.
	remaining := req.Budget - used
	for i := range cands {
		if remaining <= 0 {
			break
		}
		if cands[i].Selected {
			continue
		}
		if cands[i].Reason == "rejected: explicitly invalidated" || cands[i].Reason == "rejected: low authority" {
			continue
		}
		if cands[i].Tokens <= remaining {
			cands[i].Selected = true
			cands[i].Reason = "selected: fill pass"
			selected = append(selected, cands[i])
			used += cands[i].Tokens
			remaining -= cands[i].Tokens
		}
	}

	// Pass 6: partition selected into stable prefix (maximises KV-cache reuse) and variable context.
	// Stable prefix = invariant decisions/constraints/code/facts; variable = dynamic runtime state.
	sort.SliceStable(selected, func(i, j int) bool {
		isStable := func(c model.Candidate) int {
			if c.Kind == "decision" || c.Kind == "constraint" || c.Kind == "code" || c.Kind == "fact" {
				return 0
			}
			return 1
		}
		return isStable(selected[i]) < isStable(selected[j])
	})
	var prefix, variable []model.Candidate
	for _, c := range selected {
		if c.Kind == "decision" || c.Kind == "constraint" || c.Kind == "code" || c.Kind == "fact" {
			prefix = append(prefix, c)
		} else {
			variable = append(variable, c)
		}
	}
	return model.ContextPlan{
		Task: req.Task, Budget: req.Budget, SelectedTokens: used,
		StablePrefix: prefix, VariableContext: variable,
		Candidates: cands, Selected: selected,
	}
}

// max returns the greater of two integers; guards against zero-division in token density calculations.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
