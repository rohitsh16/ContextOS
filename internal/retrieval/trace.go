package retrieval

import (
	"fmt"
	"time"
)

// RetrievalTrace captures granular instrumentation for each retrieval stage,
// as mandated by PR.md Section 4 (Phase 0: Instrument the exhaustive implementation).
type RetrievalTrace struct {
	QueryID  string `json:"query_id"`
	RepoID   string `json:"repo_id"`
	Revision string `json:"revision"`

	// QueryClass is the R17 classification of the query (e.g. "FILE_BASENAME").
	QueryClass string `json:"query_class,omitempty"`

	TotalNodes         int   `json:"total_nodes"`
	ScopedNodes        int   `json:"scoped_nodes"`
	LexicalCandidates  int   `json:"lexical_candidates"`
	PrunedCandidates   int   `json:"pruned_candidates"`
	GraphExpanded      int   `json:"graph_expanded"`
	SemanticEvaluated  int   `json:"semantic_evaluated"`
	FinalCandidates    int   `json:"final_candidates"`
	PostingsVisited    int   `json:"postings_visited"`
	BlocksSkipped      int   `json:"blocks_skipped"`
	SourceBytesLoaded  int64 `json:"source_bytes_loaded"`

	LatencyTotal      time.Duration `json:"latency_total"`
	LatencyScope      time.Duration `json:"latency_scope"`
	LatencyLexical    time.Duration `json:"latency_lexical"`
	LatencyPruning    time.Duration `json:"latency_pruning"`
	LatencyGraph      time.Duration `json:"latency_graph"`
	LatencySemantic   time.Duration `json:"latency_semantic"`
	LatencyAllocation time.Duration `json:"latency_allocation"`

	// TouchRatio = entities touched / entities indexed. Target < 1% for normal, < 0.1% localized.
	TouchRatio float64 `json:"touch_ratio"`
}


// ComputeMetrics finalizes touch ratio and derived invariants on the trace.
func (t *RetrievalTrace) ComputeMetrics() {
	if t.TotalNodes > 0 {
		touched := t.ScopedNodes
		if touched <= 0 {
			touched = t.LexicalCandidates + t.GraphExpanded
		}
		if touched > t.TotalNodes {
			touched = t.TotalNodes
		}
		t.TouchRatio = float64(touched) / float64(t.TotalNodes)
	}
}

// String returns a human-readable summary of the retrieval trace.
func (t *RetrievalTrace) String() string {
	return fmt.Sprintf("Trace[query=%s total=%d scoped=%d final=%d touch=%.2f%% lat=%s]",
		t.QueryID, t.TotalNodes, t.ScopedNodes, t.FinalCandidates, t.TouchRatio*100, t.LatencyTotal)
}
