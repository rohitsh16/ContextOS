package retrieval

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"contextos/internal/store"
	"contextos/internal/textutil"
)

// Retriever defines the core contract for contextual code retrieval,
// evolving ContextOS from repository-wide scanning to hierarchical, adaptive retrieval (PR.md Section 5).
type Retriever interface {
	Retrieve(ctx context.Context, q Query) ([]Candidate, RetrievalTrace, error)
}

// ExhaustiveRetriever is the baseline oracle that scans every node in the repository (PR.md Section 4).
// It serves as the scientific ground-truth baseline against which optimized indexed retrievers are compared.
type ExhaustiveRetriever struct {
	Store store.Store
}

func NewExhaustiveRetriever(st store.Store) *ExhaustiveRetriever {
	return &ExhaustiveRetriever{Store: st}
}

func (r *ExhaustiveRetriever) Retrieve(ctx context.Context, q Query) ([]Candidate, RetrievalTrace, error) {
	start := time.Now()
	trace := RetrievalTrace{
		QueryID:  fmt.Sprintf("q_exh_%d", time.Now().UnixNano()),
		RepoID:   q.RepoID,
		Revision: q.Revision,
	}

	nodes, err := r.Store.ListNodes(q.RepoID)
	if err != nil {
		return nil, trace, err
	}
	trace.TotalNodes = len(nodes)
	trace.ScopedNodes = len(nodes) // touches 100% of nodes

	limit := q.MaxResults
	if limit <= 0 {
		limit = 100
	}

	tLex := time.Now()
	type scored struct {
		c  Candidate
		sc float64
	}
	tmp := make([]scored, 0, len(nodes))

	for _, n := range nodes {
		select {
		case <-ctx.Done():
			return nil, trace, ctx.Err()
		default:
		}

		content := fmt.Sprintf("%s %s %s %s:%d", n.Kind, n.Name, n.Signature, n.Path, n.StartLine)
		lexSc := 0.6*textutil.HashSemantic(q.Task, content) + 0.4*textutil.Overlap(q.Task, content)

		// Record touch
		trace.LexicalCandidates++
		if lexSc < 0.05 && q.Task != "" {
			trace.PrunedCandidates++
			continue
		}

		c := Candidate{
			ID:            "cand:" + n.ID,
			NodeID:        n.ID,
			Kind:          n.Kind,
			Name:          n.Name,
			Path:          n.Path,
			StartLine:     n.StartLine,
			EndLine:       n.EndLine,
			Signature:     n.Signature,
			Content:       content,
			LexicalScore:  lexSc,
			Score:         lexSc,
			Stage:         "exhaustive_scan",
			Provenance:    []string{"oracle_full_scan"},
		}
		tmp = append(tmp, scored{c: c, sc: lexSc})
	}
	trace.LatencyLexical = time.Since(tLex)

	tSort := time.Now()
	sort.Slice(tmp, func(i, j int) bool {
		return tmp[i].sc > tmp[j].sc
	})
	trace.LatencyPruning = time.Since(tSort)

	if len(tmp) > limit {
		tmp = tmp[:limit]
	}

	res := make([]Candidate, len(tmp))
	for i, s := range tmp {
		res[i] = s.c
	}
	trace.FinalCandidates = len(res)
	trace.LatencyTotal = time.Since(start)
	trace.ComputeMetrics()

	return res, trace, nil
}

// IndexedRetriever generates candidates strictly through targeted exact, path, and package indexes
// avoiding O(N) repository-wide enumeration (PR.md Section 7).
type IndexedRetriever struct {
	Store store.Store
}

func NewIndexedRetriever(st store.Store) *IndexedRetriever {
	return &IndexedRetriever{Store: st}
}

func (r *IndexedRetriever) Retrieve(ctx context.Context, q Query) ([]Candidate, RetrievalTrace, error) {
	start := time.Now()
	trace := RetrievalTrace{
		QueryID:  fmt.Sprintf("q_idx_%d", time.Now().UnixNano()),
		RepoID:   q.RepoID,
		Revision: q.Revision,
	}

	limit := q.MaxResults
	if limit <= 0 {
		limit = 100
	}

	// 1. Determine total entities (for touch ratio accounting)
	allNodes, _ := r.Store.ListNodes(q.RepoID)
	trace.TotalNodes = len(allNodes)

	// 2. Localized candidate generation using exact/prefix/keyword indices
	tScope := time.Now()
	nodes, err := r.Store.SearchCodeCandidates(q.RepoID, q.Task, q.Scope, limit*5)
	trace.LatencyScope = time.Since(tScope)

	if err != nil {
		return nil, trace, err
	}

	trace.ScopedNodes = len(nodes)
	trace.LexicalCandidates = len(nodes)

	// 3. Score only the localized candidates
	tLex := time.Now()
	type scored struct {
		c  Candidate
		sc float64
	}
	tmp := make([]scored, 0, len(nodes))

	for _, n := range nodes {
		select {
		case <-ctx.Done():
			return nil, trace, ctx.Err()
		default:
		}

		content := fmt.Sprintf("%s %s %s %s:%d", n.Kind, n.Name, n.Signature, n.Path, n.StartLine)
		lexSc := 0.6*textutil.HashSemantic(q.Task, content) + 0.4*textutil.Overlap(q.Task, content)

		stage := "indexed_lookup"
		prov := []string{"index_match"}
		if strings.Contains(strings.ToLower(q.Task), strings.ToLower(n.Name)) {
			stage = "exact_symbol"
			prov = append(prov, "exact_symbol_match")
		}


		c := Candidate{
			ID:            "cand:" + n.ID,
			NodeID:        n.ID,
			Kind:          n.Kind,
			Name:          n.Name,
			Path:          n.Path,
			StartLine:     n.StartLine,
			EndLine:       n.EndLine,
			Signature:     n.Signature,
			Content:       content,
			LexicalScore:  lexSc,
			Score:         lexSc,
			Stage:         stage,
			Provenance:    prov,
		}
		tmp = append(tmp, scored{c: c, sc: lexSc})
	}
	trace.LatencyLexical = time.Since(tLex)

	tSort := time.Now()
	sort.Slice(tmp, func(i, j int) bool {
		return tmp[i].sc > tmp[j].sc
	})
	trace.LatencyPruning = time.Since(tSort)

	if len(tmp) > limit {
		tmp = tmp[:limit]
	}

	res := make([]Candidate, len(tmp))
	for i, s := range tmp {
		res[i] = s.c
	}
	trace.FinalCandidates = len(res)
	trace.LatencyTotal = time.Since(start)
	trace.ComputeMetrics()

	return res, trace, nil
}
