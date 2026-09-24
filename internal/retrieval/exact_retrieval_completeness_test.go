// Package retrieval — exact_retrieval_completeness_test.go
//
// R17 Phase 10: Proof-Oriented Regression Suite
// Theorem 2 (Exact Retrieval Completeness):
// If the user query specifies an exact canonical repository path or file
// basename that exists in the admissible index, the deterministic exact lookup
// guarantees Recall@1 = 1.0 unconditionally.
package retrieval

import (
	"context"
	"path/filepath"
	"testing"

	"contextos/internal/gitidx"
	"contextos/internal/store"
)

func TestExactRetrievalCompleteness_Theorem2(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "exact_test.db")
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer st.Close()

	repoID, err := st.GetOrCreateRepo("/repo", "testrepo", "rev1", "main", "wt1")
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	targetFile := "internal/store/sqlite_store.go"
	files := []gitidx.SourceFile{
		{Path: "cmd/main.go", Hash: "h1", Lines: 50},
		{Path: targetFile, Hash: "h2", Lines: 300},
		{Path: "internal/store/file_store.go", Hash: "h3", Lines: 200},
		{Path: "pkg/api/handler.go", Hash: "h4", Lines: 80},
	}
	syms := []gitidx.Symbol{
		{Path: targetFile, Name: "LookupExactPath", Kind: "function", Start: 270, End: 285, Signature: "func (s *SQLiteStore) LookupExactPath(repoID string, path string) ([]NodeRecord, error)"},
		{Path: targetFile, Name: "SQLiteStore", Kind: "type", Start: 50, End: 60, Signature: "type SQLiteStore struct"},
		{Path: "cmd/main.go", Name: "main", Kind: "function", Start: 10, End: 30, Signature: "func main()"},
	}

	if err := st.SaveNodesAndEdges(repoID, files, syms, nil); err != nil {
		t.Fatalf("save nodes: %v", err)
	}

	retriever := NewIndexedRetriever(st)
	ctx := context.Background()

	// Case 1: Exact full path query
	qPath := Query{
		Task:       targetFile,
		RepoID:     repoID,
		MaxResults: 10,
	}
	cands, trace, err := retriever.Retrieve(ctx, qPath)
	if err != nil {
		t.Fatalf("retrieve exact path: %v", err)
	}
	if len(cands) == 0 {
		t.Fatalf("expected non-empty results for exact path")
	}
	if cands[0].Path != targetFile {
		t.Fatalf("Theorem 2 violated: Recall@1 = 0, top candidate path is %q, expected %q", cands[0].Path, targetFile)
	}
	if trace.QueryClass != "PATH_EXACT" {
		t.Fatalf("expected QueryClass PATH_EXACT, got %s", trace.QueryClass)
	}

	// Case 2: Exact basename query
	qBasename := Query{
		Task:       "sqlite_store.go",
		RepoID:     repoID,
		MaxResults: 10,
	}
	candsBase, traceBase, err := retriever.Retrieve(ctx, qBasename)
	if err != nil {
		t.Fatalf("retrieve exact basename: %v", err)
	}
	if len(candsBase) == 0 {
		t.Fatalf("expected non-empty results for exact basename")
	}
	if filepath.Base(candsBase[0].Path) != "sqlite_store.go" {
		t.Fatalf("Theorem 2 violated for basename: top candidate path is %q, expected sqlite_store.go", candsBase[0].Path)
	}
	if traceBase.QueryClass != "FILE_BASENAME" {
		t.Fatalf("expected QueryClass FILE_BASENAME, got %s", traceBase.QueryClass)
	}

	// Case 3: Exact symbol query
	qSymbol := Query{
		Task:       "LookupExactPath",
		RepoID:     repoID,
		MaxResults: 10,
	}
	candsSym, _, err := retriever.Retrieve(ctx, qSymbol)
	if err != nil {
		t.Fatalf("retrieve exact symbol: %v", err)
	}
	if len(candsSym) == 0 || candsSym[0].Name != "LookupExactPath" {
		t.Fatalf("Theorem 2 violated for symbol: top candidate is not LookupExactPath")
	}
}
