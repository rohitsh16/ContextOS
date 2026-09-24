package retrieval

import (
	"context"
	"fmt"
	"time"

	"contextos/internal/retrieval/graph"
	"contextos/internal/retrieval/index"
	"contextos/internal/store"
)

// PlannedContext represents the compiled, decision-sufficient context output ready for the LLM.
type PlannedContext struct {
	Scope                Scope              `json:"scope"`
	TiersSearched        []LocalityTierName `json:"tiers_searched"`
	Candidates           []Candidate        `json:"candidates"`
	SelectedUnits        []ContextUnit      `json:"selected_units"`
	PromptText           string             `json:"prompt_text"`
	TotalTokens          int                `json:"total_tokens"`
	EstimatedCorrectness float64            `json:"estimated_correctness"`
	ObjectiveScore       float64            `json:"objective_score"`
	CacheHit             bool               `json:"cache_hit"`
	PrefixHash           string             `json:"prefix_hash"`
}

// SubsystemPlanner executes the complete hierarchical, adaptive retrieval and context compilation pipeline (PR.md PR-01 to PR-30).
type SubsystemPlanner struct {
	Store         store.Store
	Graph         *graph.PersistentGraph
	ShardRouter   *index.ShardRouter
	Cache         *ContextCache
	Sparse        *SparseExpander
	FusionWeights FusionWeights
	OptimizerCfg  OptimizerConfig
}

// NewSubsystemPlanner initializes a SubsystemPlanner.
func NewSubsystemPlanner(
	s store.Store,
	g *graph.PersistentGraph,
	sr *index.ShardRouter,
	cache *ContextCache,
) *SubsystemPlanner {
	if g == nil {
		g = graph.NewPersistentGraph()
	}
	if sr == nil {
		sr = index.NewShardRouter(4)
	}
	if cache == nil {
		cache = NewContextCache(256)
	}
	return &SubsystemPlanner{
		Store:         s,
		Graph:         g,
		ShardRouter:   sr,
		Cache:         cache,
		Sparse:        NewSparseExpander(),
		FusionWeights: DefaultFusionWeights,
		OptimizerCfg:  DefaultOptimizerConfig,
	}
}

// ExecutePlan orchestrates the full pipeline:
// Classify → Exact Retrieval → Canonicalize → Evidence Validation →
// Bounded Graph Expand (from validated seeds only) → Fuse → Adaptive Stop →
// Compile ContextUnits → Optimize Context → Cache Prefix.
//
// R17 Phase 6: The planner now enforces the soundness contract (Theorem 3):
// if retrieval returns EvidenceNone, no candidates are selected.
func (p *SubsystemPlanner) ExecutePlan(ctx context.Context, q Query, lctx LocalizerContext) (*PlannedContext, RetrievalTrace, error) {
	startTotal := time.Now()
	trace := RetrievalTrace{
		QueryID:  fmt.Sprintf("q_plan_%d", time.Now().UnixNano()),
		RepoID:   q.RepoID,
		Revision: q.Revision,
	}

	// 1. Scope localization (PR-11)
	locStart := time.Now()
	scope := InferScope(q.Task, lctx)
	trace.LatencyScope = time.Since(locStart)

	// 2. Build Locality Hierarchy (PR-12)
	hierarchy := BuildLocalityHierarchy(scope, lctx)

	// 3. Multi-source Candidate Retrieval (R17 Phase 3+4+5)
	retStart := time.Now()
	indexedRetriever := NewIndexedRetriever(p.Store)

	// Retrieve candidates — now uses query classification + exact indexes
	rawCandidates, subTrace, err := indexedRetriever.Retrieve(ctx, q)
	if err != nil {
		return nil, trace, err
	}
	trace.LatencyLexical = time.Since(retStart)
	trace.TotalNodes = subTrace.TotalNodes
	trace.ScopedNodes = subTrace.ScopedNodes
	trace.LexicalCandidates = subTrace.LexicalCandidates
	trace.PrunedCandidates = subTrace.PrunedCandidates
	trace.PostingsVisited = subTrace.PostingsVisited
	trace.BlocksSkipped = subTrace.BlocksSkipped
	trace.QueryClass = subTrace.QueryClass

	// 4. Classify evidence (R17 Phase 6)
	qc := ClassifyQuery(q.Task)
	var exactCands, lexicalCands []Candidate
	for _, c := range rawCandidates {
		switch c.Stage {
		case "exact_path", "exact_basename", "exact_qualified_symbol", "exact_symbol", "identifier_basename":
			exactCands = append(exactCands, c)
		default:
			lexicalCands = append(lexicalCands, c)
		}
	}
	evidenceStatus := ClassifyEvidenceStatus(qc, exactCands, lexicalCands, nil, 0.10)
	evidencePkg := EvidencePackage{
		Status:                      evidenceStatus,
		QueryClass:                  qc.Primary,
		Candidates:                  rawCandidates,
		DuplicateAmplificationRatio: subTrace.TouchRatio,
	}

	// 5. Enforce planner soundness (Theorem 3): if no evidence, return empty.
	if evidenceStatus == EvidenceNone {
		evidencePkg.FallbackRequired = true
		evidencePkg.Confidence = 0.0
		trace.FinalCandidates = 0
		trace.LatencyTotal = time.Since(startTotal)
		return &PlannedContext{
			Scope:                scope,
			TiersSearched:        []LocalityTierName{TierHOT},
			Candidates:           nil,
			SelectedUnits:        nil,
			PromptText:           "",
			TotalTokens:          0,
			EstimatedCorrectness: 0.0,
			ObjectiveScore:       0.0,
			CacheHit:             false,
			PrefixHash:           "",
		}, trace, nil
	}

	// 6. Bounded Graph Expansion — seeded only from validated candidates (R17 Theorem 4).
	// Graph expansion from an empty seed returns empty (Theorem 4).
	graphStart := time.Now()
	var seedIDs []string
	for i := 0; i < len(rawCandidates) && i < 10; i++ {
		seedIDs = append(seedIDs, rawCandidates[i].NodeID)
	}

	expandedNodeIDs := p.Graph.ExpandSeeds(seedIDs, 2, 5)
	var graphCands []Candidate
	for _, nid := range expandedNodeIDs {
		graphCands = append(graphCands, Candidate{
			NodeID: nid,
			Score:  0.50,
			Stage:  "graph",
		})
	}
	trace.GraphExpanded = len(graphCands)
	trace.LatencyGraph = time.Since(graphStart)

	// 7. Separate candidate sources for fusion
	var sparseCands, recentCands, trigramCands []Candidate

	// 8. Learned Sparse Retrieval Expansion (PR-22)
	if p.Sparse.IsEnabled() {
		sparseTerms := p.Sparse.Expand(q.Task)
		for term, weight := range sparseTerms {
			if weight > 0.80 {
				sparseCands = append(sparseCands, Candidate{
					NodeID: "sparse:" + term,
					Score:  weight,
					Stage:  "sparse",
				})
			}
		}
	}

	// 9. Multi-stage Candidate Fusion (PR-19)
	fused := FuseCandidates(exactCands, lexicalCands, trigramCands, graphCands, recentCands, sparseCands, p.FusionWeights)

	// 10. Adaptive K & Adaptive Stopping (PR-20, PR-21)
	scores := make([]float64, len(fused))
	for i, c := range fused {
		scores[i] = c.Score
	}
	adaptiveK := DetermineAdaptiveK(scores, 5, 25, 1.0)
	if adaptiveK > len(fused) {
		adaptiveK = len(fused)
	}
	fusedTopK := fused[:adaptiveK]

	stopIdx := AdaptiveStopIndex(fusedTopK, 3, 0.70, 0.05)
	stoppedCandidates := fusedTopK[:stopIdx]
	trace.FinalCandidates = len(stoppedCandidates)

	// 11. Final soundness assertion (defensive check)
	if assertErr := AssertPlannerSoundness(evidencePkg, stoppedCandidates); assertErr != nil {
		// This should never happen given the EvidenceNone guard above,
		// but defense-in-depth requires we check here too.
		stoppedCandidates = nil
		trace.FinalCandidates = 0
	}

	// 12. Compile candidates to ContextUnits (PR-25)
	var allUnits []ContextUnit
	for _, c := range stoppedCandidates {
		units := CompileCandidateToUnits(c)
		allUnits = append(allUnits, units...)
	}

	// 13. Decision-Aware Context Optimization (PR-26)
	optResult := OptimizeContext(allUnits, p.OptimizerCfg)

	// 14. Cache-Aware Ordering and Prefix Caching (PR-27, PR-28)
	orderedUnits := SortUnitsForCache(optResult.SelectedUnits)
	promptText := FormatContextUnits(orderedUnits)

	cacheEntry := p.Cache.Put(q.Task, orderedUnits)

	trace.LatencyTotal = time.Since(startTotal)
	trace.ComputeMetrics()

	tiersSearched := []LocalityTierName{TierHOT}
	if len(hierarchy.WARM.Paths) > 0 {
		tiersSearched = append(tiersSearched, TierWARM)
	}

	_ = evidencePkg // available for future audit logging

	return &PlannedContext{
		Scope:                scope,
		TiersSearched:        tiersSearched,
		Candidates:           stoppedCandidates,
		SelectedUnits:        orderedUnits,
		PromptText:           promptText,
		TotalTokens:          optResult.TotalTokens,
		EstimatedCorrectness: optResult.EstimatedCorrectness,
		ObjectiveScore:       optResult.ObjectiveScore,
		CacheHit:             false,
		PrefixHash:           cacheEntry.PrefixHash,
	}, trace, nil
}

