package bench

import (
	"context"
	"math"
	"sort"
	"strings"

	"contextos/internal/allocator"
	"contextos/internal/graph"
	"contextos/internal/model"
	"contextos/internal/semantic"
	"contextos/internal/textutil"
)

// BaselineID identifies each of the experimental baselines.
type BaselineID string

const (
	B0FullHistory     BaselineID = "B0 Full history"
	B1Recency         BaselineID = "B1 Recency"
	B2TopKLexical     BaselineID = "B2 Top-K lexical"
	B3CurrentContextOS BaselineID = "B3 Current ContextOS"
	B4BM25            BaselineID = "B4 BM25"
	B5BM25Dense       BaselineID = "B5 BM25 + dense"
	B6BM25Graph       BaselineID = "B6 BM25 + graph"
	B7Hybrid          BaselineID = "B7 Hybrid retrieval"
	B8HybridSubmod    BaselineID = "B8 Hybrid + submodular selection"
	B9FullContextOS   BaselineID = "B9 Full ContextOS research system"
)

// AllBaselines lists all 10 experimental baselines in order.
var AllBaselines = []BaselineID{
	B0FullHistory,
	B1Recency,
	B2TopKLexical,
	B3CurrentContextOS,
	B4BM25,
	B5BM25Dense,
	B6BM25Graph,
	B7Hybrid,
	B8HybridSubmod,
	B9FullContextOS,
}

// AblationConfig controls individual subsystem toggles for controlled ablation experiments.
type AblationConfig struct {
	PersistentMemory bool `json:"persistent_memory"`
	Graph            bool `json:"graph"`
	CacheAware       bool `json:"cache_aware"`
	TemporalValidity bool `json:"temporal_validity"`
}

// DefaultAblations returns the full configuration with all subsystems enabled.
func DefaultAblations() AblationConfig {
	return AblationConfig{
		PersistentMemory: true,
		Graph:            true,
		CacheAware:       true,
		TemporalValidity: true,
	}
}

// TaskContext represents the environment of a single benchmark task.
type TaskContext struct {
	ID           string
	Task         string
	Query        string
	Memories     []model.Memory
	RelevantIDs  map[string]bool // Ground truth relevant memory IDs
	StaleIDs     map[string]bool // Memories that are stale/contradictory
	Revision     string
	ChangedFiles map[string]string
	Graph        *graph.Graph
	CorpusStats  *textutil.CorpusStats
}

// BaselineExecution holds the output of executing a baseline on a task.
type BaselineExecution struct {
	BaselineID      BaselineID
	RankedIDs       []string
	Selected        []model.Candidate
	SelectedTokens  int
	DurationMs      float64
	CachedTokens    int
	UncachedTokens  int
	Regressions     int
	FactsCovered    int
	TotalTargetHits int
}

func memAuthorityScore(auth string) float64 {
	switch strings.ToLower(auth) {
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

// ExecuteBaseline runs a specific baseline algorithm on the provided task context and budget.
func ExecuteBaseline(id BaselineID, tc TaskContext, budget int, ablations AblationConfig) BaselineExecution {
	memories := tc.Memories
	if !ablations.PersistentMemory {
		// Ablation: Persistent Memory OFF -> agent starts with empty memory pool
		memories = nil
	}

	if len(memories) == 0 {
		return BaselineExecution{BaselineID: id}
	}

	switch id {
	case B0FullHistory:
		// B0: Full history — unconditional FIFO accumulation up to budget
		var selected []model.Candidate
		tokens := 0
		var rankedIDs []string
		for _, m := range memories {
			rankedIDs = append(rankedIDs, m.ID)
			cost := m.TokenCost
			if cost <= 0 {
				cost = 50
			}
			if tokens+cost <= budget {
				selected = append(selected, model.Candidate{
					ID:        m.ID,
					Kind:      m.Kind,
					Content:   m.Content,
					Tokens:    cost,
					Score:     1.0,
					Authority: memAuthorityScore(m.Authority),
				})
				tokens += cost
			}
		}
		return evaluateExecution(id, rankedIDs, selected, tokens, tc, ablations)

	case B1Recency:
		// B1: Recency — sort descending by creation order / timestamp, pack until budget
		reversed := make([]model.Memory, len(memories))
		copy(reversed, memories)
		for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
			reversed[i], reversed[j] = reversed[j], reversed[i]
		}
		var selected []model.Candidate
		tokens := 0
		var rankedIDs []string
		for _, m := range reversed {
			rankedIDs = append(rankedIDs, m.ID)
			cost := m.TokenCost
			if cost <= 0 {
				cost = 50
			}
			if tokens+cost <= budget {
				selected = append(selected, model.Candidate{
					ID:        m.ID,
					Kind:      m.Kind,
					Content:   m.Content,
					Tokens:    cost,
					Score:     1.0,
					Authority: memAuthorityScore(m.Authority),
				})
				tokens += cost
			}
		}
		return evaluateExecution(id, rankedIDs, selected, tokens, tc, ablations)

	case B2TopKLexical:
		// B2: Top-K lexical — naive word overlap score
		type scoredMem struct {
			mem   model.Memory
			score float64
		}
		qWords := strings.Fields(strings.ToLower(tc.Query))
		var scored []scoredMem
		for _, m := range memories {
			mWords := strings.Fields(strings.ToLower(m.Content))
			overlap := 0
			for _, qw := range qWords {
				for _, mw := range mWords {
					if qw == mw {
						overlap++
						break
					}
				}
			}
			scored = append(scored, scoredMem{mem: m, score: float64(overlap)})
		}
		sort.SliceStable(scored, func(i, j int) bool {
			return scored[i].score > scored[j].score
		})
		var rankedIDs []string
		var selected []model.Candidate
		tokens := 0
		for _, sm := range scored {
			rankedIDs = append(rankedIDs, sm.mem.ID)
			cost := sm.mem.TokenCost
			if cost <= 0 {
				cost = 50
			}
			if tokens+cost <= budget {
				selected = append(selected, model.Candidate{
					ID:        sm.mem.ID,
					Kind:      sm.mem.Kind,
					Content:   sm.mem.Content,
					Tokens:    cost,
					Score:     sm.score,
					Authority: memAuthorityScore(sm.mem.Authority),
				})
				tokens += cost
			}
		}
		return evaluateExecution(id, rankedIDs, selected, tokens, tc, ablations)

	case B3CurrentContextOS:
		// B3: ContextOS v1 — linear utility-score density greedy (no RRF, no submodular selection)
		req := allocator.Request{
			Task:         tc.Task,
			Budget:       budget,
			RepoRevision: tc.Revision,
			ChangedFiles: tc.ChangedFiles,
		}
		plan := allocator.Plan(req, memories)
		var rankedIDs []string
		for _, c := range plan.Selected {
			rankedIDs = append(rankedIDs, c.ID)
		}
		for _, m := range memories {
			found := false
			for _, rid := range rankedIDs {
				if rid == m.ID {
					found = true
					break
				}
			}
			if !found {
				rankedIDs = append(rankedIDs, m.ID)
			}
		}
		return evaluateExecution(id, rankedIDs, plan.Selected, plan.SelectedTokens, tc, ablations)

	case B4BM25:
		// B4: BM25 corpus-aware ranking alone
		stats := tc.CorpusStats
		if stats == nil {
			docs := make([]string, len(memories))
			for i, m := range memories {
				docs[i] = m.Content
			}
			stats = textutil.NewCorpusStats(docs)
		}
		type scoredMem struct {
			mem   model.Memory
			score float64
		}
		var scored []scoredMem
		for _, m := range memories {
			bm25 := stats.ScoreBM25(tc.Query, m.Content, 1.2, 0.75)
			scored = append(scored, scoredMem{mem: m, score: bm25})
		}
		sort.SliceStable(scored, func(i, j int) bool {
			return scored[i].score > scored[j].score
		})
		var rankedIDs []string
		var selected []model.Candidate
		tokens := 0
		for _, sm := range scored {
			rankedIDs = append(rankedIDs, sm.mem.ID)
			cost := sm.mem.TokenCost
			if cost <= 0 {
				cost = 50
			}
			if tokens+cost <= budget {
				selected = append(selected, model.Candidate{
					ID:        sm.mem.ID,
					Kind:      sm.mem.Kind,
					Content:   sm.mem.Content,
					Tokens:    cost,
					Score:     sm.score,
					Authority: memAuthorityScore(sm.mem.Authority),
				})
				tokens += cost
			}
		}
		return evaluateExecution(id, rankedIDs, selected, tokens, tc, ablations)

	case B5BM25Dense:
		// B5: BM25 + dense cosine similarity merged via Reciprocal Rank Fusion (RRF)
		denseMap := computeDenseMap(tc.Query, memories)
		req := allocator.Request{
			Task:         tc.Task,
			Budget:       budget,
			CorpusStats:  tc.CorpusStats,
			DenseScores:  denseMap,
			RepoRevision: tc.Revision,
		}
		plan := allocator.Plan(req, memories)
		var rankedIDs []string
		for _, c := range plan.Selected {
			rankedIDs = append(rankedIDs, c.ID)
		}
		return evaluateExecution(id, rankedIDs, plan.Selected, plan.SelectedTokens, tc, ablations)

	case B6BM25Graph:
		// B6: BM25 + graph centrality (Personalized PageRank) merged via RRF
		var graphScores map[string]float64
		if ablations.Graph && tc.Graph != nil {
			graphScores = tc.Graph.ComputePPR(map[string]float64{"main": 1.0})
		}
		req := allocator.Request{
			Task:         tc.Task,
			Budget:       budget,
			CorpusStats:  tc.CorpusStats,
			GraphScores:  graphScores,
			RepoRevision: tc.Revision,
		}
		plan := allocator.Plan(req, memories)
		var rankedIDs []string
		for _, c := range plan.Selected {
			rankedIDs = append(rankedIDs, c.ID)
		}
		return evaluateExecution(id, rankedIDs, plan.Selected, plan.SelectedTokens, tc, ablations)

	case B7Hybrid:
		// B7: Hybrid retrieval (BM25 + Dense + Graph + Affinity RRF)
		var graphScores map[string]float64
		if ablations.Graph && tc.Graph != nil {
			graphScores = tc.Graph.ComputePPR(map[string]float64{"main": 1.0})
		}
		denseMap := computeDenseMap(tc.Query, memories)
		req := allocator.Request{
			Task:         tc.Task,
			Budget:       budget,
			CorpusStats:  tc.CorpusStats,
			GraphScores:  graphScores,
			DenseScores:  denseMap,
			RepoRevision: tc.Revision,
		}
		plan := allocator.Plan(req, memories)
		var rankedIDs []string
		for _, c := range plan.Selected {
			rankedIDs = append(rankedIDs, c.ID)
		}
		return evaluateExecution(id, rankedIDs, plan.Selected, plan.SelectedTokens, tc, ablations)

	case B8HybridSubmod:
		// B8: Hybrid retrieval + Submodular Selector (coverage, diversity, authority)
		var graphScores map[string]float64
		if ablations.Graph && tc.Graph != nil {
			graphScores = tc.Graph.ComputePPR(map[string]float64{"main": 1.0})
		}
		denseMap := computeDenseMap(tc.Query, memories)
		req := allocator.Request{
			Task:         tc.Task,
			Budget:       budget,
			CorpusStats:  tc.CorpusStats,
			GraphScores:  graphScores,
			DenseScores:  denseMap,
			RepoRevision: tc.Revision,
		}
		candidates := allocator.RankCandidates(req, memories)
		submod := &allocator.SubmodularSelector{RedundancyPenalty: 0.35}
		selected, _ := submod.Select(context.Background(), candidates, budget)
		var rankedIDs []string
		tokens := 0
		for _, c := range selected {
			rankedIDs = append(rankedIDs, c.ID)
			tokens += c.Tokens
		}
		return evaluateExecution(id, rankedIDs, selected, tokens, tc, ablations)

	case B9FullContextOS:
		// B9: Full ContextOS research system
		// (Hybrid + Submodular/Singleton rescue + Cache-Aware prefix ordering + Scoped temporal validity)
		var graphScores map[string]float64
		if ablations.Graph && tc.Graph != nil {
			graphScores = tc.Graph.ComputePPR(map[string]float64{"main": 1.0})
		}
		denseMap := computeDenseMap(tc.Query, memories)
		changed := tc.ChangedFiles
		if !ablations.TemporalValidity {
			changed = nil // disable temporal scoping
		}
		req := allocator.Request{
			Task:         tc.Task,
			Budget:       budget,
			CorpusStats:  tc.CorpusStats,
			GraphScores:  graphScores,
			DenseScores:  denseMap,
			RepoRevision: tc.Revision,
			ChangedFiles: changed,
		}
		plan := allocator.Plan(req, memories)

		selected := plan.Selected
		if !ablations.CacheAware {
			// Ablation: Cache-aware prefix order OFF -> shuffle order of selected items
			for i := range selected {
				j := (i * 7 + 3) % len(selected)
				selected[i], selected[j] = selected[j], selected[i]
			}
		}

		var rankedIDs []string
		for _, c := range selected {
			rankedIDs = append(rankedIDs, c.ID)
		}
		return evaluateExecution(id, rankedIDs, selected, plan.SelectedTokens, tc, ablations)

	default:
		return BaselineExecution{BaselineID: id}
	}
}

func computeDenseMap(query string, memories []model.Memory) map[string]float64 {
	provider := semantic.NewHashEmbeddingProvider()
	qEmb, _ := provider.Embed(context.Background(), []string{query})
	m := make(map[string]float64)
	if len(qEmb) == 0 {
		return m
	}
	for _, mem := range memories {
		dEmb, _ := provider.Embed(context.Background(), []string{mem.Content})
		if len(dEmb) > 0 {
			sim, _ := semantic.CosineSimilarity(qEmb[0], dEmb[0])
			m[mem.ID] = float64(sim)
		}
	}
	return m
}

func evaluateExecution(id BaselineID, rankedIDs []string, selected []model.Candidate, tokens int, tc TaskContext, ablations AblationConfig) BaselineExecution {
	regressions := 0
	factsCovered := 0
	for _, c := range selected {
		if tc.StaleIDs[c.ID] {
			regressions++
		}
		if tc.RelevantIDs[c.ID] {
			factsCovered++
		}
	}

	// Cache calculation: CacheAware enables exact prefix matching across calls
	cachedTokens := 0
	uncachedTokens := tokens
	if ablations.CacheAware && len(selected) > 0 {
		// Stable prefix caching: First stable items (system/directives) qualify for prompt caching
		for i, c := range selected {
			if i < 2 && c.Authority >= 0.99 {
				cachedTokens += c.Tokens
			}
		}
		uncachedTokens = tokens - cachedTokens
		if uncachedTokens < 0 {
			uncachedTokens = 0
		}
	}

	return BaselineExecution{
		BaselineID:      id,
		RankedIDs:       rankedIDs,
		Selected:        selected,
		SelectedTokens:  tokens,
		DurationMs:      math.Max(0.5, float64(tokens)/150.0),
		CachedTokens:    cachedTokens,
		UncachedTokens:  uncachedTokens,
		Regressions:     regressions,
		FactsCovered:    factsCovered,
		TotalTargetHits: len(tc.RelevantIDs),
	}
}
