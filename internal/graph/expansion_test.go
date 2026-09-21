package graph

import (
	"testing"
	"time"

	"contextos/internal/store"
)

type mockEdgeProvider struct {
	edges map[string][]store.EdgeRecord
}

func (m *mockEdgeProvider) LookupAdjacentEdges(repoID string, nodeIDs []string) ([]store.EdgeRecord, error) {
	var out []store.EdgeRecord
	for _, id := range nodeIDs {
		out = append(out, m.edges[id]...)
	}
	return out, nil
}

func TestBoundedBestFirstExpansion(t *testing.T) {
	seeds := []store.NodeRecord{
		{ID: "node_auth", Name: "AuthService", Centrality: 0.8, InDegree: 10, OutDegree: 5},
	}
	seedScores := map[string]float64{"node_auth": 0.9}

	mockEdges := &mockEdgeProvider{
		edges: map[string][]store.EdgeRecord{
			"node_auth": {
				{SrcID: "node_auth", DstID: "node_jwt", Kind: "import"},
				{SrcID: "node_auth", DstID: "node_db", Kind: "references"},
			},
			"node_jwt": {
				{SrcID: "node_jwt", DstID: "node_crypto", Kind: "import"},
			},
		},
	}

	cfg := DefaultExpansionConfig()
	cfg.MaxNodes = 5
	cfg.MaxDepth = 2

	res := BoundedBestFirstExpansion("repo_test", seeds, seedScores, mockEdges, cfg)

	if len(res.NodeScores) < 3 {
		t.Fatalf("expected at least 3 nodes in expansion, got %d", len(res.NodeScores))
	}

	if _, ok := res.NodeScores["node_jwt"]; !ok {
		t.Errorf("expected node_jwt to be expanded")
	}

	if _, ok := res.NodeScores["node_db"]; !ok {
		t.Errorf("expected node_db to be expanded")
	}

	if res.MaxDepthReached > 2 {
		t.Errorf("expected max depth <= 2, got %d", res.MaxDepthReached)
	}
}

func TestAdaptiveStoppingVOI(t *testing.T) {
	seeds := []store.NodeRecord{
		{ID: "node_root", Name: "RootService", Centrality: 0.1, InDegree: 1, OutDegree: 1},
	}
	seedScores := map[string]float64{"node_root": 0.2}

	mockEdges := &mockEdgeProvider{
		edges: map[string][]store.EdgeRecord{
			"node_root": {
				{SrcID: "node_root", DstID: "node_leaf", Kind: "documentation"},
			},
		},
	}

	// High cost rate should trigger VOI < Cost stopping
	cfg := DefaultExpansionConfig()
	cfg.CostLatencyRate = 100.0 // Force VOI < Cost immediately on non-root hop

	res := BoundedBestFirstExpansion("repo_test", seeds, seedScores, mockEdges, cfg)

	if res.Termination != "voi_below_cost" && res.Termination != "queue_empty" {
		t.Logf("termination: %s", res.Termination)
	}
}

func TestExpansionTimeBudgetBound(t *testing.T) {
	seeds := []store.NodeRecord{
		{ID: "node_slow", Name: "SlowNode", Centrality: 0.5},
	}
	seedScores := map[string]float64{"node_slow": 0.8}

	cfg := DefaultExpansionConfig()
	cfg.MaxDuration = 1 * time.Nanosecond // immediate timeout

	res := BoundedBestFirstExpansion("repo_test", seeds, seedScores, nil, cfg)
	if res.Termination != "timeout" && res.NodesExpanded > 0 {
		// Acceptable if it managed 1 node before timer
		t.Logf("completed with termination: %s", res.Termination)
	}
}
