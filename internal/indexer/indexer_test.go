package indexer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"contextos/internal/store"
)

func setupTestRepo(t *testing.T, fileCount int) (string, store.Store) {
	t.Helper()
	root := t.TempDir()

	for i := 0; i < fileCount; i++ {
		name := fmt.Sprintf("file_%02d.go", i)
		content := fmt.Sprintf("package main\n\nfunc Func%02d() int { return %d }\n", i, i)
		if i > 0 {
			// Add reference to previous file
			content += fmt.Sprintf("func CallPrev%02d() { _ = file_%02d.go }\n", i, i-1)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	st, err := store.NewFileStore(filepath.Join(root, ".contextos", "data"))
	if err != nil {
		t.Fatal(err)
	}

	return root, st
}

func TestFullIndexCreatesManifestAndStore(t *testing.T) {
	root, st := setupTestRepo(t, 5)
	defer st.Close()

	idx := New(st, root, "repo1")
	stats, err := idx.IndexFull("rev1")
	if err != nil {
		t.Fatalf("IndexFull failed: %v", err)
	}

	if stats.FilesParsed != 5 {
		t.Fatalf("expected 5 files parsed, got %d", stats.FilesParsed)
	}
	if stats.SymbolsExtracted == 0 {
		t.Fatalf("expected symbols extracted, got 0")
	}

	// Verify manifest was written
	m, err := LoadManifest(idx.ManifestPath)
	if err != nil {
		t.Fatalf("LoadManifest failed: %v", err)
	}
	if len(m.Files) != 5 {
		t.Fatalf("expected 5 manifest files, got %d", len(m.Files))
	}
	if m.Revision != "rev1" {
		t.Fatalf("expected rev1, got %s", m.Revision)
	}

	// Verify nodes in store
	nodes, err := st.ListNodes("repo1")
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) == 0 {
		t.Fatal("expected nodes in store")
	}
}

func TestIncrementalNoChangesEarlyExit(t *testing.T) {
	root, st := setupTestRepo(t, 5)
	defer st.Close()

	idx := New(st, root, "repo1")
	_, err := idx.IndexFull("rev1")
	if err != nil {
		t.Fatal(err)
	}

	// Run incremental without modifications
	incStats, err := idx.IndexIncremental("rev1")
	if err != nil {
		t.Fatalf("IndexIncremental failed: %v", err)
	}

	if incStats.FilesParsed != 0 {
		t.Fatalf("expected 0 files parsed for unchanged repo, got %d", incStats.FilesParsed)
	}
	if incStats.WorkReduction != 1.0 {
		t.Fatalf("expected 100%% work reduction (1.0), got %f", incStats.WorkReduction)
	}
}

func TestIncrementalAddModifyDelete(t *testing.T) {
	root, st := setupTestRepo(t, 5)
	defer st.Close()

	idx := New(st, root, "repo1")
	_, err := idx.IndexFull("rev1")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Add file
	newFile := filepath.Join(root, "added.go")
	if err := os.WriteFile(newFile, []byte("package main\nfunc Added() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Modify file
	modFile := filepath.Join(root, "file_00.go")
	if err := os.WriteFile(modFile, []byte("package main\nfunc Func00Modified() int { return 999 }\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. Delete file
	delFile := filepath.Join(root, "file_04.go")
	if err := os.Remove(delFile); err != nil {
		t.Fatal(err)
	}

	incStats, err := idx.IndexIncremental("rev2")
	if err != nil {
		t.Fatalf("IndexIncremental failed: %v", err)
	}

	if incStats.AddedCount != 1 {
		t.Fatalf("expected 1 added file, got %d", incStats.AddedCount)
	}
	if incStats.ModifiedCount != 1 {
		t.Fatalf("expected 1 modified file, got %d", incStats.ModifiedCount)
	}
	if incStats.DeletedCount != 1 {
		t.Fatalf("expected 1 deleted file, got %d", incStats.DeletedCount)
	}
	if incStats.FilesParsed != 2 {
		t.Fatalf("expected 2 files parsed (added + modified), got %d", incStats.FilesParsed)
	}

	// Verify deleted node is gone
	nodes, _ := st.ListNodes("repo1")
	for _, n := range nodes {
		if n.Path == "file_04.go" {
			t.Fatalf("expected file_04.go to be deleted from store, but found node: %+v", n)
		}
	}

	// Verify added node exists
	var foundAdded bool
	for _, n := range nodes {
		if n.Path == "added.go" {
			foundAdded = true
		}
	}
	if !foundAdded {
		t.Fatal("added.go node not found in store")
	}
}

func TestEquivalenceIncrementalEqualsFull(t *testing.T) {
	// Consistency property: IndexIncremental(R, Delta) == IndexFull(R + Delta)
	root, stInc := setupTestRepo(t, 10)
	defer stInc.Close()

	idxInc := New(stInc, root, "repo1")
	_, err := idxInc.IndexFull("rev1")
	if err != nil {
		t.Fatal(err)
	}

	// Apply delta: add, modify, delete
	if err := os.WriteFile(filepath.Join(root, "new_service.go"), []byte("package main\nfunc NewService() string { return \"svc\" }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file_03.go"), []byte("package main\nfunc Updated03() { println(\"updated\") }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "file_08.go")); err != nil {
		t.Fatal(err)
	}

	// 1. Run incremental index
	_, err = idxInc.IndexIncremental("rev2")
	if err != nil {
		t.Fatalf("incremental index failed: %v", err)
	}

	// 2. Run full index on a separate fresh store
	freshDir := t.TempDir()
	stFull, err := store.NewFileStore(freshDir)
	if err != nil {
		t.Fatal(err)
	}
	defer stFull.Close()

	idxFull := New(stFull, root, "repo1")
	idxFull.ManifestPath = filepath.Join(freshDir, "manifest.json")
	_, err = idxFull.IndexFull("rev2")
	if err != nil {
		t.Fatalf("full index failed: %v", err)
	}

	// 3. Compare nodes between Incremental and Full
	incNodes, _ := stInc.ListNodes("repo1")
	fullNodes, _ := stFull.ListNodes("repo1")

	if len(incNodes) != len(fullNodes) {
		t.Fatalf("node count mismatch: incremental has %d, full has %d", len(incNodes), len(fullNodes))
	}

	sortNodes := func(nodes []store.NodeRecord) {
		sort.Slice(nodes, func(i, j int) bool {
			if nodes[i].Path != nodes[j].Path {
				return nodes[i].Path < nodes[j].Path
			}
			return nodes[i].Name < nodes[j].Name
		})
	}
	sortNodes(incNodes)
	sortNodes(fullNodes)

	for i := range incNodes {
		if incNodes[i].Path != fullNodes[i].Path || incNodes[i].Name != fullNodes[i].Name || incNodes[i].Kind != fullNodes[i].Kind {
			t.Fatalf("node mismatch at index %d:\n  inc:  %+v\n  full: %+v", i, incNodes[i], fullNodes[i])
		}
	}

	// 4. Compare edges between Incremental and Full
	incEdges, _ := stInc.ListEdges("repo1")
	fullEdges, _ := stFull.ListEdges("repo1")

	if len(incEdges) != len(fullEdges) {
		t.Fatalf("edge count mismatch: incremental has %d, full has %d", len(incEdges), len(fullEdges))
	}
}
