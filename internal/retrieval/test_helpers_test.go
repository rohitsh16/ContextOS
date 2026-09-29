package retrieval

import (
	"os"
	"path/filepath"
	"testing"

	"contextos/internal/store"
)

func newTestStore(t *testing.T, dir string) store.Store {
	t.Helper()
	var (
		st  store.Store
		err error
	)
	if os.Getenv("CONTEXTOS_STORAGE") == "file" {
		st, err = store.NewFileStore(filepath.Join(dir, "data"))
	} else {
		dbPath := filepath.Join(dir, "test.db")
		st, err = store.NewSQLiteStore(dbPath)
		if err != nil {
			st, err = store.NewFileStore(filepath.Join(dir, "data"))
		}
	}
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	return st
}
