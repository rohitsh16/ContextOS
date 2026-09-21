package graph

import (
	"sort"
	"testing"
)

func TestPersistentGraphNeighborsAndSeedExpansion(t *testing.T) {
	pg := NewPersistentGraph()

	// Register nodes: fileA defines symA1, symA2; fileB defines symB1; fileC defines symC1
	pg.AddNode("symA1", "pkgA/a.go", "pkgA", true)
	pg.AddNode("symA2", "pkgA/a.go", "pkgA", true)
	pg.AddNode("symB1", "pkgB/b.go", "pkgB", true)
	pg.AddNode("symC1", "pkgC/c.go", "pkgC", true)

	// Add edges: symA1 -> symB1 -> symC1
	pg.AddEdge(Edge{From: "symA1", To: "symB1", Type: "calls", Weight: 1.0})
	pg.AddEdge(Edge{From: "symB1", To: "symC1", Type: "calls", Weight: 1.0})
	pg.AddEdge(Edge{From: "symA2", To: "symC1", Type: "references", Weight: 0.5})

	// 1. Test 1-hop neighbors
	n1 := pg.Neighbors("symA1", 1, 10)
	if len(n1) != 1 || n1[0] != "symB1" {
		t.Fatalf("expected [symB1], got %v", n1)
	}

	// 2. Test 2-hop neighbors
	n2 := pg.Neighbors("symA1", 2, 10)
	if len(n2) != 2 {
		t.Fatalf("expected 2 neighbors (symB1, symC1), got %v", n2)
	}

	// 3. Test reverse neighbors
	rev := pg.ReverseNeighbors("symC1", 1, 10)
	sort.Strings(rev)
	if len(rev) != 2 || rev[0] != "symA2" || rev[1] != "symB1" {
		t.Fatalf("expected [symA2, symB1], got %v", rev)
	}

	// 4. Test ExpandSeeds
	expanded := pg.ExpandSeeds([]string{"symA1"}, 2, 5)
	if len(expanded) != 2 || expanded[0] != "symB1" || expanded[1] != "symC1" {
		t.Fatalf("expected [symB1, symC1], got %v", expanded)
	}
}

func TestIncrementalGraphEqualsFullRebuild(t *testing.T) {
	// Full graph from scratch
	fullGraph := NewPersistentGraph()
	fullGraph.AddNode("symA1", "a.go", "main", true)
	fullGraph.AddNode("symA2", "a.go", "main", true)
	fullGraph.AddNode("symB1", "b.go", "main", true)
	fullGraph.AddEdge(Edge{From: "symA1", To: "symB1", Type: "calls"})
	fullGraph.AddEdge(Edge{From: "symA2", To: "symB1", Type: "calls"})

	// Incremental graph: starts with symA1 -> symB1 and symOld -> symB1
	incGraph := NewPersistentGraph()
	incGraph.AddNode("symA1", "a.go", "main", true)
	incGraph.AddNode("symOld", "a.go", "main", true)
	incGraph.AddNode("symB1", "b.go", "main", true)
	incGraph.AddEdge(Edge{From: "symA1", To: "symB1", Type: "calls"})
	incGraph.AddEdge(Edge{From: "symOld", To: "symB1", Type: "calls"})

	// File a.go was edited: symOld removed, symA2 added
	oldSymbols := []string{"symA1", "symOld"}
	newSymbols := []string{"symA1", "symA2"}
	oldEdges := []Edge{
		{From: "symA1", To: "symB1", Type: "calls"},
		{From: "symOld", To: "symB1", Type: "calls"},
	}
	newEdges := []Edge{
		{From: "symA1", To: "symB1", Type: "calls"},
		{From: "symA2", To: "symB1", Type: "calls"},
	}

	delta := ComputeFileDelta("a.go", oldSymbols, newSymbols, oldEdges, newEdges)
	incGraph.AddNode("symA2", "a.go", "main", true)
	incGraph.ApplyDelta(delta)

	// Validate incremental graph neighbors match fullGraph
	nFull := fullGraph.Neighbors("symA2", 1, 10)
	nInc := incGraph.Neighbors("symA2", 1, 10)
	if len(nFull) != len(nInc) || nFull[0] != nInc[0] {
		t.Fatalf("neighbors of symA2 mismatch: full=%v, inc=%v", nFull, nInc)
	}

	revFull := fullGraph.ReverseNeighbors("symB1", 1, 10)
	revInc := incGraph.ReverseNeighbors("symB1", 1, 10)
	sort.Strings(revFull)
	sort.Strings(revInc)
	if len(revFull) != len(revInc) {
		t.Fatalf("reverse neighbors mismatch: full=%v, inc=%v", revFull, revInc)
	}
	for i := range revFull {
		if revFull[i] != revInc[i] {
			t.Errorf("at index %d: full=%s, inc=%s", i, revFull[i], revInc[i])
		}
	}
}
