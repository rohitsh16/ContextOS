package bench

import (
	"os"
	"path/filepath"
	"testing"

	"contextos/internal/store"
)

func TestCorpusGeneratorAndRetrievalBenchmark(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "corpus_bench_test.db")
	var st store.Store
	var err error
	if os.Getenv("CONTEXTOS_STORAGE") == "file" || os.Getenv("CGO_ENABLED") == "0" {
		st, err = store.NewFileStore(filepath.Join(dir, "data"))
	} else {
		st, err = store.NewSQLiteStore(dbPath)
		if err != nil {
			st, err = store.NewFileStore(filepath.Join(dir, "data"))
		}
	}
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer st.Close()

	cfg := CorpusConfig{
		NodeCount: 1000,
		Seed:      12345,
	}

	corpus, err := GenerateSyntheticCodeCorpus(st, cfg)
	if err != nil {
		t.Fatalf("GenerateSyntheticCodeCorpus failed: %v", err)
	}


	if corpus.TotalNodes < 500 {
		t.Fatalf("expected >= 500 nodes generated, got %d", corpus.TotalNodes)
	}

	if len(corpus.Queries) != 10 {
		t.Fatalf("expected 10 query classes, got %d", len(corpus.Queries))
	}

	// Run comparative A/B benchmark (PR.md Section 42)
	rep, err := RunRetrievalBenchmark(st, corpus)
	if err != nil {
		t.Fatalf("RunRetrievalBenchmark failed: %v", err)
	}

	t.Logf("Benchmark Complete: Nodes=%d, AvgSpeedup=%.2fx, AvgRecall@10=%.2f, AvgIndexedTouch=%.2f%%",
		rep.CorpusNodes, rep.AvgSpeedup, rep.AvgRecallAt10, rep.AvgIndexedTouch*100)

	for _, res := range rep.Results {
		t.Logf("Query [%s] '%s': Recall@10=%.2f, Touch=%.2f%%, Speedup=%.2fx",
			res.QueryClass, res.QueryText, res.RecallAt10, res.IndexedTouch*100, res.Speedup)
	}

	// Verify PR.md invariants
	if rep.AvgIndexedTouch >= 0.35 {
		t.Fatalf("expected average indexed touch ratio < 35%% on synthetic corpus, got %.2f%%", rep.AvgIndexedTouch*100)
	}
	if rep.AvgRecallAt10 < 0.50 {
		t.Fatalf("expected average recall@10 >= 0.50, got %.2f", rep.AvgRecallAt10)
	}

	for _, res := range rep.Results {
		if res.IndexedTouch >= res.OracleTouch {
			t.Errorf("query [%s] indexed touch (%.2f%%) >= oracle touch (%.2f%%)",
				res.QueryClass, res.IndexedTouch*100, res.OracleTouch*100)
		}
	}
}

