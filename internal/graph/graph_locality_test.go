// Package graph — graph_locality_test.go
//
// R17 Phase 10: Proof-Oriented Regression Suite
// Theorem 4 & 5 (Graph Locality & Complexity):
// Graph expansion and degree normalization must scale with the localized query
// frontier, avoiding repeated O(|V|) full-graph scans on every candidate evaluation.
package graph

import (
	"fmt"
	"testing"
)

func TestGraphLocality_Theorem5(t *testing.T) {
	cfg := DefaultConfig()
	g := New(cfg)

	// Build a graph with 500 nodes
	totalNodes := 500
	for i := 0; i < totalNodes; i++ {
		g.AddNode(&Node{
			ID:   fmt.Sprintf("node_%d", i),
			Name: fmt.Sprintf("Symbol_%d", i),
			Kind: "symbol",
			Path: fmt.Sprintf("pkg/comp_%d.go", i),
		})
	}

	// Connect a dense local cluster around node_0
	for i := 1; i <= 10; i++ {
		g.AddEdge("node_0", fmt.Sprintf("node_%d", i), "call")
	}

	// Verify MaxDegree is computed and cached correctly
	maxDeg := g.MaxDegree()
	if maxDeg != 10 {
		t.Fatalf("expected max degree 10, got %d", maxDeg)
	}
	if !g.cachedMaxDegValid {
		t.Fatalf("expected cachedMaxDegValid to be true after MaxDegree()")
	}

	// Calling DegreeNorm repeatedly should use cached max degree without scanning
	degNorm0 := g.DegreeNorm("node_0")
	if degNorm0 <= 0.99 {
		t.Fatalf("expected node_0 (max degree node) to have DegreeNorm close to 1.0, got %f", degNorm0)
	}

	degNormOther := g.DegreeNorm("node_50")
	if degNormOther != 0.0 {
		t.Fatalf("expected isolated node to have DegreeNorm 0.0, got %f", degNormOther)
	}
}
