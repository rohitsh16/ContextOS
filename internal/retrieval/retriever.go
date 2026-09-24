package retrieval

import (
	"context"
	"fmt"
	"sort"
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

// IndexedRetriever generates candidates strictly through targeted exact, path,
// and package indexes — avoiding O(N) repository-wide enumeration.
//
// R17 Phase 3+4+5: the retriever now classifies the query and dispatches to
// exact deterministic indexes before falling back to approximate search.
// All candidates are canonically deduplicated before return (Theorem 1).
type IndexedRetriever struct {
	Store store.Store
	// RepoID is used for canonical deduplication. Set by the caller.
	RepoID string
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

	// NOTE: The O(N) ListNodes call has been removed (R17 Phase 3).
	// TotalNodes is no longer eagerly computed to avoid full table scans.
	// It can be retrieved separately if needed for accounting.

	// Phase 4: Classify the query to determine the correct retrieval path.
	qc := ClassifyQuery(q.Task)
	trace.QueryClass = qc.Primary.String()

	var exactCands, lexicalCands []Candidate

	// Phase 3: Exact deterministic retrieval based on query class.
	tExact := time.Now()
	switch qc.Primary {
	case QueryClassPathExact:
		for _, ident := range qc.Identifiers {
			nodes, err := r.Store.LookupExactPath(q.RepoID, ident)
			if err == nil {
				exactCands = append(exactCands, nodesToCandidates(nodes, "exact_path", []string{"path_exact_match"})...)
			}
		}

	case QueryClassFileBasename:
		for _, ident := range qc.Identifiers {
			// Try exact path first, then basename
			nodes, err := r.Store.LookupExactPath(q.RepoID, ident)
			if err == nil && len(nodes) > 0 {
				exactCands = append(exactCands, nodesToCandidates(nodes, "exact_path", []string{"path_exact_match"})...)
			} else {
				nodes, err = r.Store.LookupBasename(q.RepoID, ident)
				if err == nil {
					exactCands = append(exactCands, nodesToCandidates(nodes, "exact_basename", []string{"basename_exact_match"})...)
				}
			}
		}

	case QueryClassQualifiedSymbol:
		for _, ident := range qc.Identifiers {
			nodes, err := r.Store.LookupQualifiedSymbol(q.RepoID, ident)
			if err == nil {
				exactCands = append(exactCands, nodesToCandidates(nodes, "exact_qualified_symbol", []string{"qualified_symbol_match"})...)
			}
		}

	case QueryClassSymbol:
		for _, ident := range qc.Identifiers {
			nodes, err := r.Store.LookupSymbol(q.RepoID, ident)
			if err == nil {
				exactCands = append(exactCands, nodesToCandidates(nodes, "exact_symbol", []string{"symbol_exact_match"})...)
			}
		}

	case QueryClassIdentifier:
		for _, ident := range qc.Identifiers {
			// Try basename first (identifiers often map to file names)
			nodes, err := r.Store.LookupBasename(q.RepoID, ident+".go")
			if err == nil && len(nodes) > 0 {
				exactCands = append(exactCands, nodesToCandidates(nodes, "identifier_basename", []string{"identifier_file_match"})...)
			}
			// Also try as a symbol name
			symNodes, err2 := r.Store.LookupSymbol(q.RepoID, ident)
			if err2 == nil {
				exactCands = append(exactCands, nodesToCandidates(symNodes, "exact_symbol", []string{"identifier_symbol_match"})...)
			}
		}
	}
	trace.LatencyScope = time.Since(tExact)
	trace.LexicalCandidates = len(exactCands)

	// Phase 3: Lexical enrichment — retrieve complementary candidates if
	// exact candidates do not saturate limit, or query has lexical/conceptual components.
	hasLexicalSecondary := false
	for _, sc := range qc.Secondary {
		if sc == QueryClassLexical {
			hasLexicalSecondary = true
			break
		}
	}
	if len(exactCands) < limit || hasLexicalSecondary ||
		qc.Primary == QueryClassLexical || qc.Primary == QueryClassConceptual ||
		qc.Primary == QueryClassRelation {
		tLex := time.Now()
		nodes, err := r.Store.SearchCodeCandidates(q.RepoID, q.Task, q.Scope, limit*3)
		if err == nil {
			lexicalCands = nodesToCandidates(nodes, "indexed_lookup", []string{"fts_match"})
		}
		trace.LatencyLexical = time.Since(tLex)
	}

	// Phase 5: Score and deduplicate all candidates.
	tScore := time.Now()
	allRaw := append(exactCands, lexicalCands...)
	for i := range allRaw {
		c := &allRaw[i]
		content := fmt.Sprintf("%s %s %s %s:%d", c.Kind, c.Name, c.Signature, c.Path, c.StartLine)
		c.Content = content
		lexSc := 0.6*textutil.HashSemantic(q.Task, content) + 0.4*textutil.Overlap(q.Task, content)
		c.LexicalScore = lexSc

		// Exact-tier candidates get a priority bonus so they always outrank
		// approximate results (R17 §24 Candidate Fusion Rule, Tier 0).
		if c.Stage == "exact_path" || c.Stage == "exact_basename" ||
			c.Stage == "exact_qualified_symbol" || c.Stage == "exact_symbol" {
			c.Score = 1.0 + lexSc // tier-0 bonus: score > 1.0 dominates lexical
		} else {
			c.Score = lexSc
		}
	}
	trace.LatencyPruning = time.Since(tScore)

	// Deduplicate by canonical identity (Theorem 1).
	dedup := DeduplicateCandidates(allRaw, q.RepoID)
	canonical := dedup.Canonical
	trace.PrunedCandidates = dedup.DuplicateCount

	// Sort by score descending.
	sort.Slice(canonical, func(i, j int) bool {
		return canonical[i].Score > canonical[j].Score
	})

	if len(canonical) > limit {
		canonical = canonical[:limit]
	}

	trace.ScopedNodes = len(allRaw)
	trace.FinalCandidates = len(canonical)
	totalNodes, err := r.Store.CountNodes(q.RepoID)
	if err == nil && totalNodes > 0 {
		trace.TotalNodes = totalNodes
	} else {
		trace.TotalNodes = len(allRaw)
	}
	trace.LatencyTotal = time.Since(start)
	trace.ComputeMetrics()

	return canonical, trace, nil
}

// nodesToCandidates converts a slice of NodeRecords to Candidates with the
// given stage and provenance labels.
func nodesToCandidates(nodes []store.NodeRecord, stage string, provenance []string) []Candidate {
	out := make([]Candidate, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, Candidate{
			ID:        "cand:" + n.ID,
			NodeID:    n.ID,
			Kind:      n.Kind,
			Name:      n.Name,
			Path:      n.Path,
			StartLine: n.StartLine,
			EndLine:   n.EndLine,
			Signature: n.Signature,
			Stage:     stage,
			Provenance: provenance,
		})
	}
	return out
}

