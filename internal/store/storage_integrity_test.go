// Package store — storage_integrity_test.go
//
// R17 Phase 10: Proof-Oriented Regression Suite
// Gate R17.5 & Phase 2 (Storage Integrity & Zero Orphan Edges):
// Validates that all edges in storage reference valid node endpoints (E \subseteq V \times V)
// with zero orphan edges (#orphanEdges = 0).
package store

import (
	"path/filepath"
	"strings"
	"testing"

	"contextos/internal/gitidx"
)

func TestStorageIntegrity_ZeroOrphanEdges(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "integrity_test.db")
	var st Store
	var err error
	if strings.ToLower(filepath.Ext(dbPath)) == ".db" {
		st, err = NewSQLiteStore(dbPath)
		if err != nil && strings.Contains(err.Error(), "requires cgo") {
			st, err = NewFileStore(filepath.Join(dir, "filestore"))
		}
	} else {
		st, err = NewFileStore(filepath.Join(dir, "filestore"))
	}
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer st.Close()

	repoID, err := st.GetOrCreateRepo("/repo", "testrepo", "rev1", "main", "wt1")
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	files := []gitidx.SourceFile{
		{Path: "pkg/a/a.go", Hash: "ha", Lines: 50},
		{Path: "pkg/b/b.go", Hash: "hb", Lines: 60},
	}
	syms := []gitidx.Symbol{
		{Path: "pkg/a/a.go", Name: "FuncA", Kind: "function", Start: 10, End: 20, Signature: "func FuncA()"},
		{Path: "pkg/b/b.go", Name: "FuncB", Kind: "function", Start: 15, End: 30, Signature: "func FuncB()"},
	}

	// First save valid nodes
	if err := st.SaveNodesAndEdges(repoID, files, syms, nil); err != nil {
		t.Fatalf("save nodes: %v", err)
	}

	nodes, err := st.ListNodes(repoID)
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	if len(nodes) < 2 {
		t.Fatalf("expected at least 2 nodes")
	}

	nodeMap := make(map[string]bool)
	for _, n := range nodes {
		nodeMap[n.ID] = true
	}

	// Add an edge connecting the two existing nodes
	validEdges := []EdgeRecord{
		{SrcID: nodes[0].ID, DstID: nodes[1].ID, Kind: "call"},
	}
	if err := st.SaveNodesAndEdges(repoID, nil, nil, validEdges); err != nil {
		t.Fatalf("save valid edge: %v", err)
	}

	// Retrieve all edges and verify #orphanEdges == 0
	edges, err := st.ListEdges(repoID)
	if err != nil {
		t.Fatalf("list edges: %v", err)
	}

	orphanCount := 0
	for _, e := range edges {
		if !nodeMap[e.SrcID] || !nodeMap[e.DstID] {
			orphanCount++
		}
	}

	if orphanCount != 0 {
		t.Fatalf("Storage integrity violation: Gate R17.5 failed: #orphanEdges = %d, expected 0", orphanCount)
	}
}
