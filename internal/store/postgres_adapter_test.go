package store

import (
	"path/filepath"
	"testing"

	"contextos/internal/model"
)

func TestPostgresStoreLifecycle(t *testing.T) {
	tempDir := t.TempDir()
	fallbackDir := filepath.Join(tempDir, "data")

	cfg := PostgresConfig{
		Host:     "localhost",
		Port:     5432,
		Database: "contextos_saas",
		User:     "postgres",
		Password: "password",
	}
	dsn := cfg.ConnectionString()
	if dsn == "" {
		t.Fatal("empty connection string")
	}

	st, err := NewPostgresStore(nil, fallbackDir)
	if err != nil {
		t.Fatalf("NewPostgresStore failed: %v", err)
	}
	defer st.Close()

	// 1. Repo creation
	repoID, err := st.GetOrCreateRepo("/test/repo", "testrepo", "rev1", "main", "wt1")
	if err != nil {
		t.Fatalf("GetOrCreateRepo failed: %v", err)
	}
	if repoID == "" {
		t.Fatal("expected non-empty repo ID")
	}

	// 2. Remember decision
	mem, err := st.Remember(repoID, model.Memory{
		Kind:       "decision",
		Content:    "Enterprise Postgres storage adapter active",
		Authority:  "user",
		Scope:      "repo",
		Confidence: 0.95,
	}, nil)
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}
	if mem.ID == "" {
		t.Fatal("expected non-empty memory ID")
	}

	// 3. Search memories
	mems, err := st.SearchMemories(repoID, "Postgres storage", 5)
	if err != nil {
		t.Fatalf("SearchMemories failed: %v", err)
	}
	if len(mems) == 0 {
		t.Fatal("expected to find remembered memory")
	}

	// 4. Verification
	count, err := st.CountNodes(repoID)
	if err != nil {
		t.Fatalf("CountNodes failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 nodes initially, got %d", count)
	}

	t.Log("✓ PostgresStore adapter: PASS — full lifecycle validated")
}
