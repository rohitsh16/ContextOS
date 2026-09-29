// Package retrieval — planner_soundness_test.go
//
// R17 Phase 10: Proof-Oriented Regression Suite
// Theorem 3 (Planner Soundness Contract):
// If the retrieval pipeline discovers NO_EVIDENCE for a specific query and no
// declared fallback applies, the planner must emit an EvidenceNone status and
// must not select unrelated candidates into the final context bundle.
package retrieval

import (
	"context"
	"testing"

	"contextos/internal/gitidx"
	"contextos/internal/retrieval/graph"
	"contextos/internal/retrieval/index"
)

func TestPlannerSoundness_Theorem3(t *testing.T) {
	dir := t.TempDir()
	st := newTestStore(t, dir)
	defer st.Close()

	repoID, err := st.GetOrCreateRepo("/repo", "testrepo", "rev1", "main", "wt1")
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	// Index a few unrelated nodes
	files := []gitidx.SourceFile{
		{Path: "pkg/auth/auth.go", Hash: "h1", Lines: 100},
		{Path: "pkg/db/db.go", Hash: "h2", Lines: 150},
	}
	syms := []gitidx.Symbol{
		{Path: "pkg/auth/auth.go", Name: "Authenticate", Kind: "function", Start: 10, End: 30, Signature: "func Authenticate() bool"},
	}
	if err := st.SaveNodesAndEdges(repoID, files, syms, nil); err != nil {
		t.Fatalf("save nodes: %v", err)
	}

	g := graph.NewPersistentGraph()
	sr := index.NewShardRouter(2)
	cache := NewContextCache(10)
	planner := NewSubsystemPlanner(st, g, sr, cache)
	ctx := context.Background()

	// Query for a non-existent exact file
	nonExistentQuery := Query{
		Task:       "nonexistent_subsystem_service_component.go",
		RepoID:     repoID,
		MaxResults: 10,
	}
	lctx := LocalizerContext{
		CurrentPackage: "pkg",
		RepoRoot:       ".",
	}

	plan, trace, err := planner.ExecutePlan(ctx, nonExistentQuery, lctx)
	if err != nil {
		t.Fatalf("execute plan: %v", err)
	}

	// Theorem 3 Invariants:
	// 1. Planner must produce 0 selected context units when retrieval finds NO_EVIDENCE.
	if len(plan.SelectedUnits) != 0 {
		t.Fatalf("Theorem 3 violated: non-existent file query selected %d units, expected 0", len(plan.SelectedUnits))
	}

	// 2. Total tokens must be 0
	if plan.TotalTokens != 0 {
		t.Fatalf("Theorem 3 violated: total tokens is %d, expected 0", plan.TotalTokens)
	}

	// 3. Final candidates in trace must be 0
	if trace.FinalCandidates != 0 {
		t.Fatalf("Theorem 3 violated: trace final candidates is %d, expected 0", trace.FinalCandidates)
	}
}
