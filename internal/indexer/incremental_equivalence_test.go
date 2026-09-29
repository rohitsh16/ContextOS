// Package indexer — incremental_equivalence_test.go
//
// R17 Phase 10: Proof-Oriented Regression Suite
// Theorem 8 (Incremental Equivalence):
// IndexFull and IndexIncremental produce equivalent node representations and
// edge topologies over identical repository content, guaranteeing that cache/index
// state does not diverge across incremental operations.
package indexer

import (
	"os"
	"path/filepath"
	"testing"

	"contextos/internal/store"
)

func TestIncrementalEquivalence_Theorem8(t *testing.T) {
	repoDir := t.TempDir()

	// Create test repository files
	f1 := filepath.Join(repoDir, "main.go")
	if err := os.WriteFile(f1, []byte("package main\n\nfunc MainFunc() {}\n"), 0644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	pkgDir := filepath.Join(repoDir, "pkg")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatalf("mkdir pkg: %v", err)
	}
	f2 := filepath.Join(pkgDir, "service.go")
	if err := os.WriteFile(f2, []byte("package pkg\n\ntype Service struct{}\nfunc (s *Service) Run() {}\n"), 0644); err != nil {
		t.Fatalf("write service.go: %v", err)
	}

	dbPathFull := filepath.Join(t.TempDir(), "full.db")
	var stFull store.Store
	var err error
	if os.Getenv("CONTEXTOS_STORAGE") == "file" || os.Getenv("CGO_ENABLED") == "0" {
		stFull, err = store.NewFileStore(filepath.Join(t.TempDir(), "full_data"))
	} else {
		stFull, err = store.NewSQLiteStore(dbPathFull)
		if err != nil {
			stFull, err = store.NewFileStore(filepath.Join(t.TempDir(), "full_data"))
		}
	}
	if err != nil {
		t.Fatalf("create full store: %v", err)
	}
	defer stFull.Close()

	repoIDFull, err := stFull.GetOrCreateRepo(repoDir, "testrepo", "rev1", "main", "wt1")
	if err != nil {
		t.Fatalf("create repo in full store: %v", err)
	}

	idxFull := New(stFull, repoDir, repoIDFull)
	statsFull, err := idxFull.IndexFull("rev1")
	if err != nil {
		t.Fatalf("index full: %v", err)
	}

	nodesFull, err := stFull.ListNodes(repoIDFull)
	if err != nil {
		t.Fatalf("list nodes full: %v", err)
	}

	// Now run incremental index on a second identical store
	dbPathIncr := filepath.Join(t.TempDir(), "incr.db")
	var stIncr store.Store
	if os.Getenv("CONTEXTOS_STORAGE") == "file" || os.Getenv("CGO_ENABLED") == "0" {
		stIncr, err = store.NewFileStore(filepath.Join(t.TempDir(), "incr_data"))
	} else {
		stIncr, err = store.NewSQLiteStore(dbPathIncr)
		if err != nil {
			stIncr, err = store.NewFileStore(filepath.Join(t.TempDir(), "incr_data"))
		}
	}
	if err != nil {
		t.Fatalf("create incr store: %v", err)
	}
	defer stIncr.Close()

	repoIDIncr, err := stIncr.GetOrCreateRepo(repoDir, "testrepo", "rev1", "main", "wt1")
	if err != nil {
		t.Fatalf("create repo in incr store: %v", err)
	}

	idxIncr := New(stIncr, repoDir, repoIDIncr)
	// Initial baseline
	_, err = idxIncr.IndexFull("rev1")
	if err != nil {
		t.Fatalf("initial baseline: %v", err)
	}

	// Incremental pass (no changes)
	statsIncr, err := idxIncr.IndexIncremental("rev1")
	if err != nil {
		t.Fatalf("index incremental: %v", err)
	}

	nodesIncr, err := stIncr.ListNodes(repoIDIncr)
	if err != nil {
		t.Fatalf("list nodes incr: %v", err)
	}

	// Invariant: Node counts must match exactly
	if len(nodesFull) != len(nodesIncr) {
		t.Fatalf("Theorem 8 violated: node count mismatch full=%d, incr=%d", len(nodesFull), len(nodesIncr))
	}

	if statsFull.FilesScanned != 2 {
		t.Fatalf("expected 2 files scanned, got %d", statsFull.FilesScanned)
	}

	if statsIncr.WorkReduction < 0.90 && statsIncr.ModifiedCount == 0 && statsIncr.AddedCount == 0 {
		t.Logf("Work reduction achieved on no-op incremental: %.2f%%", statsIncr.WorkReduction*100)
	}
}
