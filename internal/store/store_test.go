package store

import (
	"path/filepath"
	"testing"
	"time"

	"contextos/internal/gitidx"
	"contextos/internal/model"
)

func testStoreContract(t *testing.T, s Store) {
	defer s.Close()

	// 1. GetOrCreateRepo & UpdateRepo
	repoID, err := s.GetOrCreateRepo("/path/to/repo", "testrepo", "rev1", "main", "wt1")
	if err != nil {
		t.Fatalf("GetOrCreateRepo failed: %v", err)
	}
	if repoID == "" {
		t.Fatal("expected non-empty repoID")
	}

	if err := s.UpdateRepo(repoID, "rev2", "main", "wt2"); err != nil {
		t.Fatalf("UpdateRepo failed: %v", err)
	}
	if err := s.AddRevision(repoID, "rev2", "main"); err != nil {
		t.Fatalf("AddRevision failed: %v", err)
	}

	// 2. SaveNodesAndEdges & ListNodes
	files := []gitidx.SourceFile{
		{Path: "main.go", Hash: "hash1", Lines: 40},
	}
	syms := []gitidx.Symbol{
		{Path: "main.go", Kind: "func", Name: "main", Start: 1, End: 10, Signature: "func main()", Hash: "shash1"},
	}
	edges := []EdgeRecord{
		{SrcID: "s1", DstID: "s2", Kind: "references"},
	}
	if err := s.SaveNodesAndEdges(repoID, files, syms, edges); err != nil {
		t.Fatalf("SaveNodesAndEdges failed: %v", err)
	}
	nodes, err := s.ListNodes(repoID)
	if err != nil {
		t.Fatalf("ListNodes failed: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}

	// 3. Remember, Search, List, Invalidate, Reuse
	m1, err := s.Remember(repoID, model.Memory{
		Kind:              "decision",
		Content:           "Use Redis for session cache",
		ValidFromRevision: "rev2",
		Authority:         "user",
		Confidence:        0.95,
	}, []string{"spec|RFC-101"})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}
	if m1.ID == "" {
		t.Fatal("expected memory ID")
	}

	mems, err := s.ListMemories(repoID, 10)
	if err != nil {
		t.Fatalf("ListMemories failed: %v", err)
	}
	if len(mems) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(mems))
	}

	searchRes, err := s.SearchMemories(repoID, "Redis session", 10)
	if err != nil {
		t.Fatalf("SearchMemories failed: %v", err)
	}
	if len(searchRes) != 1 {
		t.Fatalf("expected 1 search result, got %d", len(searchRes))
	}

	if err := s.IncrementMemoryReuse(repoID, m1.ID); err != nil {
		t.Fatalf("IncrementMemoryReuse failed: %v", err)
	}
	if err := s.Invalidate(repoID, m1.ID, "rev3"); err != nil {
		t.Fatalf("Invalidate failed: %v", err)
	}

	// 4. WorkItems & Sessions
	wi, err := s.SetWorkItem(repoID, "Fix Redis race condition", "main")
	if err != nil {
		t.Fatalf("SetWorkItem failed: %v", err)
	}
	curWI, err := s.CurrentWorkItem(repoID)
	if err != nil || curWI == nil || curWI.ID != wi.ID {
		t.Fatalf("CurrentWorkItem mismatch: %+v %v", curWI, err)
	}

	if err := s.StartSession(repoID, "sess1", wi.ID, "codex"); err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}
	sess, err := s.LatestSession(repoID)
	if err != nil || sess == nil || sess.ID != "sess1" {
		t.Fatalf("LatestSession failed: %+v %v", sess, err)
	}
	if err := s.EndSession(repoID, "sess1"); err != nil {
		t.Fatalf("EndSession failed: %v", err)
	}

	// 5. Events & Traces
	if err := s.AddEvent(repoID, "sess1", "tool_call", `{"tool":"bash"}`); err != nil {
		t.Fatalf("AddEvent failed: %v", err)
	}
	events, err := s.ListEvents("sess1", 10)
	if err != nil || len(events) != 1 {
		t.Fatalf("ListEvents failed: %+v %v", events, err)
	}

	trace := ContextTraceRecord{
		RepoID:         repoID,
		WorkItemID:     wi.ID,
		Task:           "Redis session race",
		Model:          "gpt-5.3-codex",
		Budget:         4000,
		SelectedTokens: 150,
		EstimatedCost:  0.002,
		CacheHit:       false,
		DecisionJSON:   `{"selected":1}`,
	}
	if err := s.AddTrace(trace); err != nil {
		t.Fatalf("AddTrace failed: %v", err)
	}
	trMap, err := s.LatestTrace(repoID)
	if err != nil || trMap["task"] != "Redis session race" {
		t.Fatalf("LatestTrace failed: %+v %v", trMap, err)
	}
	tokens, hits, count, err := s.TraceStats(repoID)
	if err != nil || count != 1 || tokens != 150 || hits != 0 {
		t.Fatalf("TraceStats failed: tokens=%d hits=%d count=%d err=%v", tokens, hits, count, err)
	}

	trace2 := trace
	trace2.Task = "Second trace"
	if err := s.AddTrace(trace2); err != nil {
		t.Fatalf("AddTrace 2 failed: %v", err)
	}

	// 6. Context Cache
	cacheKey := "hash_key_123"
	if err := s.PutCache(cacheKey, repoID, "rev2", "wt2", "Redis session race", "gpt-5.3-codex", 4000, `{"plan":"ok"}`); err != nil {
		t.Fatalf("PutCache failed: %v", err)
	}
	cached, err := s.GetCache(cacheKey, repoID, "rev2", "wt2")
	if err != nil || cached != `{"plan":"ok"}` {
		t.Fatalf("GetCache failed: %q %v", cached, err)
	}
	if err := s.IncrementCacheHit(cacheKey); err != nil {
		t.Fatalf("IncrementCacheHit failed: %v", err)
	}

	// 7. Prune (dry run and real run)
	pruneOpts := PruneOptions{
		RepoID:          repoID,
		CurrentRevision: "rev3",
		CacheTTL:        1 * time.Nanosecond, // Force cache expiration
		MaxTraces:       1,                   // Keep 1, prune the other (we added 2)
		EventTTL:        1 * time.Nanosecond, // Force event expiration
		DryRun:          true,
	}
	rep, err := s.Prune(pruneOpts)
	if err != nil {
		t.Fatalf("Prune (DryRun) failed: %v", err)
	}
	if rep.CacheEntriesPruned != 1 || rep.EventsPruned != 1 || rep.TracesPruned != 1 {
		t.Fatalf("DryRun expected 1 of each, got %+v", rep)
	}

	pruneOpts.DryRun = false
	repReal, err := s.Prune(pruneOpts)
	if err != nil {
		t.Fatalf("Prune (Real) failed: %v", err)
	}
	if repReal.CacheEntriesPruned != 1 || repReal.EventsPruned != 1 || repReal.TracesPruned != 1 {
		t.Fatalf("Real Prune expected 1 of each, got %+v", repReal)
	}
}

func TestSQLiteStore(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "test.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	testStoreContract(t, s)
}

func TestFileStore(t *testing.T) {
	root := t.TempDir()
	s, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore failed: %v", err)
	}
	testStoreContract(t, s)
}

func TestBidirectionalMigration(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "test.db")
	fileDir := filepath.Join(root, "file_data")
	repo := gitidx.Repo{Path: "/test/repo", Name: "testrepo", Revision: "rev1", Branch: "main", WorktreeHash: "wt1"}

	// 1. Populate SQLiteStore
	sqlStore, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlStore.Close()

	repoID, err := sqlStore.GetOrCreateRepo(repo.Path, repo.Name, repo.Revision, repo.Branch, repo.WorktreeHash)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sqlStore.Remember(repoID, model.Memory{
		Kind:      "decision",
		Content:   "Migrate database cleanly",
		Authority: "user",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	wi, err := sqlStore.SetWorkItem(repoID, "Migration task", "main")
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlStore.StartSession(repoID, "sess_mig", wi.ID, "claude"); err != nil {
		t.Fatal(err)
	}
	if err := sqlStore.AddEvent(repoID, "sess_mig", "hook", `{"action":"start"}`); err != nil {
		t.Fatal(err)
	}

	// 2. Migrate SQLite -> FileStore
	fileStore, err := NewFileStore(fileDir)
	if err != nil {
		t.Fatal(err)
	}
	defer fileStore.Close()

	rep1, err := Migrate(sqlStore, fileStore, repo)
	if err != nil {
		t.Fatalf("Migrate SQLite->File failed: %v", err)
	}
	if rep1.MemoriesMigrated != 1 || rep1.WorkItemsMigrated != 1 || rep1.SessionsMigrated != 1 || rep1.EventsMigrated != 1 {
		t.Fatalf("Migrate SQLite->File unexpected report: %+v", rep1)
	}

	// Verify data exists in FileStore
	mems, err := fileStore.ListMemories(rep1.RepoID, 10)
	if err != nil || len(mems) != 1 {
		t.Fatalf("FileStore expected 1 memory, got %d: %v", len(mems), err)
	}

	// 3. Migrate FileStore -> new SQLiteStore
	dbPath2 := filepath.Join(root, "test2.db")
	sqlStore2, err := NewSQLiteStore(dbPath2)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlStore2.Close()

	rep2, err := Migrate(fileStore, sqlStore2, repo)
	if err != nil {
		t.Fatalf("Migrate File->SQLite failed: %v", err)
	}
	if rep2.MemoriesMigrated != 1 || rep2.WorkItemsMigrated != 1 || rep2.SessionsMigrated != 1 || rep2.EventsMigrated != 1 {
		t.Fatalf("Migrate File->SQLite unexpected report: %+v", rep2)
	}

	mems2, err := sqlStore2.ListMemories(rep2.RepoID, 10)
	if err != nil || len(mems2) != 1 {
		t.Fatalf("SQLiteStore2 expected 1 memory, got %d: %v", len(mems2), err)
	}
}

