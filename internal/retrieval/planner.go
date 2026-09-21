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
// Localize -> Hierarchical Retrieve -> Prune -> Bounded Graph Expand -> Fuse -> Adaptive Stop -> Compile ContextUnits -> Optimize Context -> Cache Prefix.
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

	// 3. Multi-source Candidate Retrieval
	retStart := time.Now()
	indexedRetriever := NewIndexedRetriever(p.Store)

	// Retrieve candidates within scope
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

	// Separate candidate sources for fusion
	var exactCands, lexicalCands, trigramCands, recentCands, sparseCands []Candidate
	for _, c := range rawCandidates {
		switch c.Stage {
		case "exact":
			exactCands = append(exactCands, c)
		case "trigram":
			trigramCands = append(trigramCands, c)
		default:
			lexicalCands = append(lexicalCands, c)
		}
	}

	// 4. Bounded Graph Expansion from top candidate seeds (PR-13, PR-15)
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

	// 5. Learned Sparse Retrieval Expansion (PR-22)
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

	// 6. Multi-stage Candidate Fusion (PR-19)
	fused := FuseCandidates(exactCands, lexicalCands, trigramCands, graphCands, recentCands, sparseCands, p.FusionWeights)

	// 7. Adaptive K & Adaptive Stopping (PR-20, PR-21)
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

	// 8. Compile candidates to ContextUnits (PR-25)
	var allUnits []ContextUnit
	for _, c := range stoppedCandidates {
		units := CompileCandidateToUnits(c)
		allUnits = append(allUnits, units...)
	}

	// 9. Decision-Aware Context Optimization (PR-26)
	optResult := OptimizeContext(allUnits, p.OptimizerCfg)

	// 10. Cache-Aware Ordering and Prefix Caching (PR-27, PR-28)
	orderedUnits := SortUnitsForCache(optResult.SelectedUnits)
	promptText := FormatContextUnits(orderedUnits)

	cacheEntry := p.Cache.Put(q.Task, orderedUnits)

	trace.LatencyTotal = time.Since(startTotal)
	trace.ComputeMetrics()

	tiersSearched := []LocalityTierName{TierHOT}
	if len(hierarchy.WARM.Paths) > 0 {
		tiersSearched = append(tiersSearched, TierWARM)
	}

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
