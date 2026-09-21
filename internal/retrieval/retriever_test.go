package retrieval

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"contextos/internal/gitidx"
	"contextos/internal/store"
)

func setupTestStore(t *testing.T, count int) (store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "retrieval_test.db")
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create sqlite store: %v", err)
	}

	repoID, err := st.GetOrCreateRepo("/test/repo", "testrepo", "rev1", "main", "wt1")
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	var syms []gitidx.Symbol
	var files []gitidx.SourceFile

	packages := []string{"auth", "api", "db", "server", "worker"}
	for i := 0; i < count; i++ {
		pkg := packages[i%len(packages)]
		path := fmt.Sprintf("pkg/%s/service_%d.go", pkg, i)
		files = append(files, gitidx.SourceFile{
			Path:  path,
			Hash:  fmt.Sprintf("hash_%d", i),
			Lines: 50,
		})
		syms = append(syms, gitidx.Symbol{
			Path:      path,
			Name:      fmt.Sprintf("HandleRequest%d", i),
			Kind:      "function",
			Start:     10,
			End:       30,
			Signature: fmt.Sprintf("func HandleRequest%d(ctx context.Context, req Request) error", i),
		})
		syms = append(syms, gitidx.Symbol{
			Path:      path,
			Name:      fmt.Sprintf("Config%d", i),
			Kind:      "type",
			Start:     35,
			End:       45,
			Signature: fmt.Sprintf("type Config%d struct", i),
		})

	}

	if err := st.SaveNodesAndEdges(repoID, files, syms, nil); err != nil {
		t.Fatalf("failed to save test nodes: %v", err)
	}

	return st, repoID
}

func TestRetrievalTraceAccounting(t *testing.T) {
	trace := RetrievalTrace{
		QueryID:           "test_q1",
		RepoID:            "repo_1",
		Revision:          "rev1",
		TotalNodes:        1000,
		ScopedNodes:       15,
		FinalCandidates:   10,
		LatencyTotal:      5 * time.Millisecond,
		LatencyScope:      1 * time.Millisecond,
		LatencyLexical:    3 * time.Millisecond,
		LatencyAllocation: 1 * time.Millisecond,
	}
	trace.ComputeMetrics()

	if trace.TouchRatio > 0.02 {
		t.Fatalf("expected TouchRatio < 0.02, got %.4f", trace.TouchRatio)
	}
	if trace.TouchRatio != 0.015 {
		t.Fatalf("expected TouchRatio == 0.015, got %.4f", trace.TouchRatio)
	}
}

func TestExhaustiveRetrieverOracleBaseline(t *testing.T) {
	st, repoID := setupTestStore(t, 50)
	defer st.Close()

	oracle := NewExhaustiveRetriever(st)
	ctx := context.Background()

	q := Query{
		Task:       "HandleRequest12 in auth package",
		RepoID:     repoID,
		Revision:   "rev1",
		MaxResults: 10,
		Mode:       ModeExhaustive,
	}

	cands, trace, err := oracle.Retrieve(ctx, q)
	if err != nil {
		t.Fatalf("oracle retrieval failed: %v", err)
	}

	if len(cands) == 0 {
		t.Fatalf("expected candidates, got 0")
	}

	// In exhaustive mode, touch ratio MUST be 1.0 (100% of nodes touched)
	if trace.TouchRatio < 0.99 {
		t.Fatalf("expected exhaustive TouchRatio == 1.0, got %.4f", trace.TouchRatio)
	}
	if trace.ScopedNodes != trace.TotalNodes {
		t.Fatalf("expected scoped (%d) == total (%d)", trace.ScopedNodes, trace.TotalNodes)
	}

	// Verify top match is HandleRequest12
	found := false
	for _, c := range cands {
		if c.Name == "HandleRequest12" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("oracle failed to rank HandleRequest12 in top 10")
	}
}

func TestIndexedRetrieverPruningAndTouchRatio(t *testing.T) {
	st, repoID := setupTestStore(t, 200) // 200 files * 2 symbols + files = 600 nodes
	defer st.Close()

	indexed := NewIndexedRetriever(st)
	oracle := NewExhaustiveRetriever(st)
	ctx := context.Background()

	q := Query{
		Task:       "HandleRequest42",
		RepoID:     repoID,
		Revision:   "rev1",
		MaxResults: 10,
		Mode:       ModeFast,
	}

	// 1. Run indexed
	candsIdx, traceIdx, err := indexed.Retrieve(ctx, q)
	if err != nil {
		t.Fatalf("indexed retrieval failed: %v", err)
	}

	// 2. Run oracle
	candsOracle, traceOracle, err := oracle.Retrieve(ctx, q)
	if err != nil {
		t.Fatalf("oracle retrieval failed: %v", err)
	}

	// Invariant: Touch ratio must be significantly lower than oracle
	if traceIdx.TouchRatio >= traceOracle.TouchRatio {
		t.Fatalf("expected indexed touch ratio (%.4f) < oracle touch ratio (%.4f)", traceIdx.TouchRatio, traceOracle.TouchRatio)
	}
	if traceIdx.TouchRatio > 0.15 {
		t.Fatalf("expected indexed touch ratio < 15%% on exact query, got %.2f%%", traceIdx.TouchRatio*100)
	}

	// Invariant: Top candidate must be identical
	if len(candsIdx) == 0 || len(candsOracle) == 0 {
		t.Fatalf("empty candidates returned")
	}
	if candsIdx[0].Name != candsOracle[0].Name {
		t.Fatalf("top candidate mismatch: indexed=%s, oracle=%s", candsIdx[0].Name, candsOracle[0].Name)
	}
}

func TestExactSymbolAndPackageLookups(t *testing.T) {
	st, repoID := setupTestStore(t, 20)
	defer st.Close()

	// Exact symbol lookup
	syms, err := st.LookupSymbol(repoID, "HandleRequest5")
	if err != nil || len(syms) == 0 {
		t.Fatalf("LookupSymbol failed: %v (len=%d)", err, len(syms))
	}
	if syms[0].Name != "HandleRequest5" {
		t.Fatalf("expected HandleRequest5, got %s", syms[0].Name)
	}

	// Package lookup
	pkgNodes, err := st.LookupPackage(repoID, "auth")
	if err != nil || len(pkgNodes) == 0 {
		t.Fatalf("LookupPackage failed: %v (len=%d)", err, len(pkgNodes))
	}

	// Qualified symbol lookup
	qual, err := st.LookupQualifiedSymbol(repoID, "auth.HandleRequest0")
	if err != nil || len(qual) == 0 {
		t.Fatalf("LookupQualifiedSymbol failed: %v", err)
	}
	if qual[0].Name != "HandleRequest0" {
		t.Fatalf("expected HandleRequest0, got %s", qual[0].Name)
	}
}
