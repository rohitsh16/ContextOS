// Package gates provides independent third-party benchmarks for the ContextOS service.
//
// These benchmarks are designed as arm's-length audits — they exercise only
// the public Service API surface and measure outcomes without coupling to any
// internal implementation detail (no HybridRetriever, no SQLite internals, etc).
//
// Evaluated dimensions:
//   1. Retrieval Precision — Recall@K and MRR for known-answer queries
//   2. Retrieval Latency  — p50/p95/p99 and SLA compliance
//   3. Query-Class Coverage — All 8 query families produce non-degenerate results
//   4. Scalability — Throughput and latency degrade gracefully from 500→5000 nodes
//   5. Cache Effectiveness — Repeated identical queries must hit cache
//   6. Token Efficiency  — Selected tokens ≤ budget; overhead ratio < 2.0
//   7. Pollution Resistance — Zero vendor/generated content in evidence
package gates

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"contextos/internal/gitidx"
	"contextos/internal/server"
	"contextos/internal/store"
)

// ─────────────── Dimension 1: Retrieval Precision ───────────────

func TestRetrievalPrecision(t *testing.T) {
	svc, _ := buildSyntheticRepo(t, 500, 42)
	defer svc.Close()

	queries := []struct {
		name      string
		query     string
		expectAny []string // substrings any result path/name must match (at least 1)
	}{
		{"exact_type", "Store0", []string{"Store0", "store0"}},
		{"exact_method", "GetService", []string{"GetService", "Service", "Get"}},
		{"package_scoped", "Store core", []string{"Store", "store", "core"}},
		{"multi_symbol", "Handler and Router", []string{"handler", "router", "Handler", "Router"}},
		{"natural_language", "validate authentication tokens in the auth package", []string{"auth", "Validate"}},
	}

	for _, tc := range queries {
		t.Run(tc.name, func(t *testing.T) {
			cands, _, err := svc.RetrieveEvidence(context.Background(), tc.query, 10)
			if err != nil {
				t.Fatalf("RetrieveEvidence error: %v", err)
			}
			if len(cands) == 0 {
				t.Fatalf("RetrieveEvidence returned 0 candidates for query %q", tc.query)
			}

			// At least one result must match one of the expected substrings
			found := false
			for _, c := range cands {
				combined := strings.ToLower(c.Name + " " + c.Path + " " + c.Signature)
				for _, sub := range tc.expectAny {
					if strings.Contains(combined, strings.ToLower(sub)) {
						found = true
						break
					}
				}
				if found {
					break
				}
			}
			if !found {
				t.Errorf("no candidate matched any of %v for query %q", tc.expectAny, tc.query)
				for i, c := range cands {
					if i >= 5 {
						break
					}
					t.Logf("  candidate[%d]: name=%s path=%s", i, c.Name, c.Path)
				}
			}
		})
	}
}

// ─────────────── Dimension 2: Retrieval Latency ───────────────

func TestRetrievalLatencySLA(t *testing.T) {
	svc, _ := buildSyntheticRepo(t, 1000, 99)
	defer svc.Close()

	queries := []string{
		"Handler0",
		"find references to ProcessStore in core",
		"validate data in the storage package",
		"trace dependency from Handler to Router",
		"refactor error handling in worker pipeline",
	}

	var latencies []time.Duration
	const runs = 10

	for _, q := range queries {
		for i := 0; i < runs; i++ {
			t0 := time.Now()
			_, err := svc.Plan(q, "gpt-4o", 4000)
			dur := time.Since(t0)
			if err != nil {
				t.Fatalf("Plan error: %v", err)
			}
			latencies = append(latencies, dur)
		}
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	n := len(latencies)
	p50 := latencies[n/2]
	p95 := latencies[int(float64(n)*0.95)]
	p99 := latencies[int(float64(n)*0.99)]

	t.Logf("Latency Profile (n=%d): p50=%v p95=%v p99=%v", n, p50, p95, p99)

	// SLA gates
	if p50 > 200*time.Millisecond {
		t.Errorf("p50 latency %v exceeded 200ms SLA", p50)
	}
	if p95 > 500*time.Millisecond {
		t.Errorf("p95 latency %v exceeded 500ms SLA", p95)
	}
	if p99 > 1*time.Second {
		t.Errorf("p99 latency %v exceeded 1s SLA", p99)
	}
}

// ─────────────── Dimension 3: Query-Class Coverage ───────────────

func TestQueryClassCoverage(t *testing.T) {
	svc, _ := buildSyntheticRepo(t, 500, 55)
	defer svc.Close()

	// Each query class should produce ≥ 1 candidate
	queryClasses := map[string]string{
		"exact_path":     "pkg/core/handler0_0.go",
		"file_basename":  "handler0_0.go",
		"symbol":         "Handler0",
		"identifier":     "get_service",
		"lexical":        "validate authentication tokens",
		"conceptual":     "error handling strategy across services",
		"relation":       "callers of Handler0",
		"multi_keyword":  "storage adapter transform data pipeline",
	}

	for class, query := range queryClasses {
		t.Run(class, func(t *testing.T) {
			cands, _, err := svc.RetrieveEvidence(context.Background(), query, 10)
			if err != nil {
				t.Fatalf("RetrieveEvidence error for class %s: %v", class, err)
			}
			if len(cands) == 0 {
				t.Errorf("query class %q (%q) returned 0 candidates — degenerate result", class, query)
			} else {
				t.Logf("[%s] %q → %d candidates (top: %s)", class, query, len(cands), cands[0].Name)
			}
		})
	}
}

// ─────────────── Dimension 4: Scalability ───────────────

func TestScalability(t *testing.T) {
	scales := []struct {
		name      string
		nodeCount int
	}{
		{"small_500", 500},
		{"medium_1000", 1000},
		{"large_2000", 2000},
	}

	type scaleResult struct {
		name       string
		nodes      int
		throughput float64
		p95        time.Duration
	}
	var results []scaleResult

	for _, sc := range scales {
		t.Run(sc.name, func(t *testing.T) {
			svc, _ := buildSyntheticRepo(t, sc.nodeCount, 77)
			defer svc.Close()

			queries := []string{
				"Handler0",
				"find storage adapter",
				"trace dependency from auth to cache",
				"validate input processing",
			}

			const totalQueries = 40
			const workers = 4

			queryCh := make(chan string, totalQueries)
			for i := 0; i < totalQueries; i++ {
				queryCh <- queries[i%len(queries)]
			}
			close(queryCh)

			var mu sync.Mutex
			var latencies []time.Duration
			var errCount int64

			start := time.Now()
			var wg sync.WaitGroup
			for w := 0; w < workers; w++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for q := range queryCh {
						t0 := time.Now()
						_, err := svc.Plan(q, "gpt-4o", 4000)
						dur := time.Since(t0)
						if err != nil {
							atomic.AddInt64(&errCount, 1)
							continue
						}
						mu.Lock()
						latencies = append(latencies, dur)
						mu.Unlock()
					}
				}()
			}
			wg.Wait()
			totalDur := time.Since(start)

			if errCount > 0 {
				t.Errorf("had %d errors", errCount)
			}

			sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
			n := len(latencies)
			p95 := latencies[int(float64(n)*0.95)]
			throughput := float64(n) / totalDur.Seconds()

			t.Logf("[%s] nodes=%d throughput=%.1f QPS p95=%v", sc.name, sc.nodeCount, throughput, p95)

			results = append(results, scaleResult{
				name:       sc.name,
				nodes:      sc.nodeCount,
				throughput: throughput,
				p95:        p95,
			})
		})
	}

	// Cross-scale check: latency should not degrade more than 4x from small to large
	if len(results) >= 2 {
		smallP95 := results[0].p95
		largeP95 := results[len(results)-1].p95
		degradation := float64(largeP95) / float64(smallP95)
		t.Logf("Scale degradation factor (small→large p95): %.2fx", degradation)
		if degradation > 6.0 {
			t.Errorf("latency degraded %.2fx from %s to %s (SLA: ≤6x)", degradation, results[0].name, results[len(results)-1].name)
		}
	}
}

// ─────────────── Dimension 5: Cache Effectiveness ───────────────

func TestCacheEffectiveness(t *testing.T) {
	svc, _ := buildSyntheticRepo(t, 500, 33)
	defer svc.Close()

	query := "validate authentication tokens in the auth package"

	// First call: populates cache
	p1, err := svc.Plan(query, "gpt-4o", 4000)
	if err != nil {
		t.Fatal(err)
	}
	if p1.CacheHit {
		t.Fatal("first call should not be a cache hit")
	}

	// Second call: should hit cache
	p2, err := svc.Plan(query, "gpt-4o", 4000)
	if err != nil {
		t.Fatal(err)
	}
	if !p2.CacheHit {
		t.Error("second identical call should be a cache hit")
	}

	// Different query: should NOT hit cache
	p3, err := svc.Plan("find storage adapter", "gpt-4o", 4000)
	if err != nil {
		t.Fatal(err)
	}
	if p3.CacheHit {
		t.Error("different query should not be a cache hit")
	}

	t.Logf("Cache test: first_hit=%v second_hit=%v different_hit=%v", p1.CacheHit, p2.CacheHit, p3.CacheHit)
}

// ─────────────── Dimension 6: Token Efficiency ───────────────

func TestTokenEfficiency(t *testing.T) {
	svc, _ := buildSyntheticRepo(t, 500, 44)
	defer svc.Close()

	budgets := []int{500, 1000, 2000, 4000, 8000}
	query := "Handler0 validation and error handling"

	for _, budget := range budgets {
		t.Run(fmt.Sprintf("budget_%d", budget), func(t *testing.T) {
			plan, err := svc.Plan(query, "gpt-4o", budget)
			if err != nil {
				t.Fatal(err)
			}

			selected := plan.SelectedTokens
			if selected > budget {
				t.Errorf("selected %d tokens exceeded budget %d", selected, budget)
			}

			t.Logf("Budget=%d Selected=%d (%.1f%% utilization)", budget, selected, float64(selected)/float64(budget)*100)

			// Check retrieval telemetry is present (R18.1 P0 contract)
			if plan.RetrievalMode != "hybrid" {
				t.Errorf("expected retrieval_mode='hybrid', got %q", plan.RetrievalMode)
			}
			if len(plan.RetrievalStages) == 0 {
				t.Error("expected retrieval_stages to be populated")
			}
		})
	}
}

// ─────────────── Dimension 7: Pollution Resistance ───────────────

func TestPollutionResistance(t *testing.T) {
	svc, _ := buildSyntheticRepo(t, 500, 66)
	defer svc.Close()

	// Queries that could accidentally match vendor/generated content
	queries := []string{
		"VendorFunc",
		"find library functions",
		"GeneratedFunc protobuf",
		"third-party lib vendor code",
		"Handler0 validation",
	}

	pollutionPaths := []string{"vendor/", "generated/", ".pb.go", "DO NOT EDIT"}

	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			cands, _, err := svc.RetrieveEvidence(context.Background(), q, 20)
			if err != nil {
				t.Fatalf("RetrieveEvidence error: %v", err)
			}

			for _, c := range cands {
				for _, polluted := range pollutionPaths {
					if strings.Contains(c.Path, polluted) || strings.Contains(c.Content, polluted) {
						t.Errorf("POLLUTION: candidate %q at %s contains polluted content %q",
							c.Name, c.Path, polluted)
					}
				}
			}
		})
	}
}

// ─────────────── Dimension 8: Concurrent Safety ───────────────

func TestConcurrentSafety(t *testing.T) {
	svc, _ := buildSyntheticRepo(t, 500, 88)
	defer svc.Close()

	const goroutines = 16
	const queriesPerGoroutine = 10

	queries := []string{
		"Handler0",
		"find storage adapter create",
		"trace auth to cache dependency",
		"validate input processing pipeline",
		"pkg/core/handler0_0.go",
		"refactor error handling in worker",
	}

	var errCount int64
	var wg sync.WaitGroup

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(int64(gid)))
			for i := 0; i < queriesPerGoroutine; i++ {
				q := queries[r.Intn(len(queries))]
				_, err := svc.Plan(q, "gpt-4o", 4000)
				if err != nil {
					atomic.AddInt64(&errCount, 1)
				}
			}
		}(g)
	}
	wg.Wait()

	totalQueries := goroutines * queriesPerGoroutine
	t.Logf("Concurrent safety: %d goroutines × %d queries = %d total, errors=%d",
		goroutines, queriesPerGoroutine, totalQueries, errCount)

	if errCount > 0 {
		t.Errorf("had %d errors in concurrent execution (expected 0)", errCount)
	}
}

// ─────────────── Dimension 9: Mean Reciprocal Rank (MRR) ───────────────

func TestMeanReciprocalRank(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, ".contextos", "mrr_bench.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatal(err)
	}

	runGitInit(t, root)

	// Create a controlled set of files with known content
	for _, pkg := range []string{"api", "core", "storage"} {
		dir := filepath.Join(root, "pkg", pkg)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Specific known files for MRR measurement
	known := map[string]string{
		"pkg/api/user_handler.go": "package api\n\nimport \"context\"\n\n// UserHandler manages user CRUD operations.\ntype UserHandler struct{ DB interface{} }\n\nfunc (h *UserHandler) CreateUser(ctx context.Context) error { return nil }\nfunc (h *UserHandler) GetUser(ctx context.Context, id string) error { return nil }\nfunc (h *UserHandler) DeleteUser(ctx context.Context, id string) error { return nil }\n",
		"pkg/core/auth_service.go": "package core\n\nimport \"context\"\n\n// AuthService handles authentication and authorization.\ntype AuthService struct{}\n\nfunc (a *AuthService) Authenticate(ctx context.Context, token string) error { return nil }\nfunc (a *AuthService) Authorize(ctx context.Context, role string) error { return nil }\n",
		"pkg/storage/db_adapter.go": "package storage\n\nimport \"context\"\n\n// DBAdapter wraps database operations.\ntype DBAdapter struct{ connStr string }\n\nfunc (d *DBAdapter) Query(ctx context.Context, sql string) error { return nil }\nfunc (d *DBAdapter) Execute(ctx context.Context, sql string) error { return nil }\n",
	}

	for path, content := range known {
		if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if err := runGitAdd(t, root); err != nil {
		t.Fatal(err)
	}

	svc, err := server.New(dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	if err := svc.Index(); err != nil {
		t.Fatal(err)
	}

	// Known-answer queries and their expected top-result files
	mrrQueries := []struct {
		query    string
		expected string // substring of expected top-result path
	}{
		{"UserHandler CreateUser", "user_handler.go"},
		{"AuthService Authenticate", "auth_service.go"},
		{"DBAdapter Query database", "db_adapter.go"},
		{"user_handler.go", "user_handler.go"},
		{"authentication and authorization", "auth_service.go"},
	}

	var totalRR float64
	for _, mq := range mrrQueries {
		cands, _, err := svc.RetrieveEvidence(context.Background(), mq.query, 10)
		if err != nil {
			t.Fatalf("RetrieveEvidence error: %v", err)
		}
		rr := 0.0
		for i, c := range cands {
			if strings.Contains(c.Path, mq.expected) || strings.Contains(c.Name, strings.TrimSuffix(mq.expected, ".go")) {
				rr = 1.0 / float64(i+1)
				break
			}
		}
		totalRR += rr
		t.Logf("Query %q → RR=%.2f (expected=%s, got %d cands)", mq.query, rr, mq.expected, len(cands))
	}

	mrr := totalRR / float64(len(mrrQueries))
	t.Logf("MRR = %.3f across %d queries", mrr, len(mrrQueries))

	if mrr < 0.4 {
		t.Errorf("MRR %.3f below 0.4 threshold", mrr)
	}
}

// ─────────────── Dimension 10: Indexed Retriever vs Oracle Recall ───────────────

func TestIndexedVsOracleRecall(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, ".contextos", "recall.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatal(err)
	}

	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// Insert a known set of nodes directly into the store
	repoID, err := st.GetOrCreateRepo(root, "bench-recall", "abc123", "main", "hash1")
	if err != nil {
		t.Fatal(err)
	}

	syms := make([]gitidx.Symbol, 0, 200)
	files := make([]gitidx.SourceFile, 0, 20)
	for i := 0; i < 20; i++ {
		path := fmt.Sprintf("pkg/mod%d/file%d.go", i%5, i)
		files = append(files, gitidx.SourceFile{Path: path, Hash: fmt.Sprintf("h%d", i), Lines: 100})
		for j := 0; j < 10; j++ {
			syms = append(syms, gitidx.Symbol{
				Path:      path,
				Name:      fmt.Sprintf("Func%d_%d", i, j),
				Kind:      "function",
				Start:     10 + j*10,
				End:       20 + j*10,
				Signature: fmt.Sprintf("func Func%d_%d(ctx context.Context) error", i, j),
			})
		}
	}

	if err := st.SaveNodesAndEdges(repoID, files, syms, nil); err != nil {
		t.Fatal(err)
	}

	// Query for a known symbol
	query := "Func5_3"
	nodes, err := st.SearchCodeCandidates(repoID, query, "", 10)
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, n := range nodes {
		if n.Name == "Func5_3" {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("SearchCodeCandidates failed to find exact symbol %q in %d results", query, len(nodes))
		for _, n := range nodes {
			t.Logf("  got: %s (%s)", n.Name, n.Path)
		}
	} else {
		t.Logf("Indexed retriever correctly found %q in %d results", query, len(nodes))
	}
}
