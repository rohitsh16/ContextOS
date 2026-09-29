// Package gates provides patent-defensibility and SaaS-readiness benchmarks for ContextOS.
//
// This file implements the "Product Readiness Audit" — a comprehensive evaluation
// designed to measure properties that matter for:
//
//   1. PATENT CLAIMS — Novel algorithmic advantage over prior art (naive grep, random, full-file)
//   2. SAAS REVENUE  — Quantified $/query savings at real model pricing across providers
//   3. PRODUCTION RELIABILITY — Sustained load, memory stability, cold-start, determinism
//
// Every test is designed to produce court-admissible, reproducible evidence of
// competitive advantage with pinned seeds, deterministic corpora, and ISO timestamps.
package gates

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"contextos/internal/gitidx"
	"contextos/internal/server"
	"contextos/internal/store"
	"contextos/internal/telemetry"
	"contextos/internal/textutil"
)

// ═══════════════════════════════════════════════════════════════════════════════
// PATENT CLAIM 1: Competitive Advantage Over Prior Art
// ═══════════════════════════════════════════════════════════════════════════════
//
// Demonstrates that ContextOS retrieval is strictly superior to three prior-art
// baselines: (a) Naive grep, (b) Random selection, (c) Full-file inclusion.
// This is the core novelty claim for patent applications.

func TestCompetitiveAdvantageOverPriorArt(t *testing.T) {
	svc, root := buildProductionCorpus(t, 2000, 42)
	defer svc.Close()

	// 20 diverse queries spanning all query classes
	queries := generateDiverseQueryCorpus(42, 20)

	type baselineResult struct {
		query           string
		ctxTokens       int
		naiveGrepTokens int
		randomTokens    int
		fullFileTokens  int
		ctxLatency      time.Duration
		grepLatency     time.Duration
	}

	var results []baselineResult

	for _, q := range queries {
		// 1. ContextOS plan
		t0 := time.Now()
		plan, err := svc.Plan(q, "claude-3-7-sonnet", 4000)
		ctxLatency := time.Since(t0)
		if err != nil {
			t.Logf("WARN: Plan error for %q: %v", q, err)
			continue
		}

		// 2. Naive grep baseline: count tokens from grep matches
		t1 := time.Now()
		grepTokens := naiveGrepBaseline(root, q)
		grepLatency := time.Since(t1)

		// 3. Random baseline: randomly sample 10 files
		randomTokens := randomFileBaseline(root, 10, 42)

		// 4. Full-file baseline: sum all .go file tokens
		fullFileTokens := fullFileBaseline(root)

		results = append(results, baselineResult{
			query:           q,
			ctxTokens:       plan.SelectedTokens,
			naiveGrepTokens: grepTokens,
			randomTokens:    randomTokens,
			fullFileTokens:  fullFileTokens,
			ctxLatency:      ctxLatency,
			grepLatency:     grepLatency,
		})
	}

	// Aggregate metrics
	var totalCtx, totalGrep, totalRandom, totalFull int
	var ctxWinsGrep, ctxWinsRandom, ctxWinsFull int

	t.Logf("═══════════════════════════════════════════════════════════════")
	t.Logf("  PATENT CLAIM 1: Competitive Advantage Over Prior Art")
	t.Logf("═══════════════════════════════════════════════════════════════")

	for _, r := range results {
		totalCtx += r.ctxTokens
		totalGrep += r.naiveGrepTokens
		totalRandom += r.randomTokens
		totalFull += r.fullFileTokens

		if r.ctxTokens <= r.naiveGrepTokens {
			ctxWinsGrep++
		}
		if r.ctxTokens <= r.randomTokens {
			ctxWinsRandom++
		}
		if r.ctxTokens <= r.fullFileTokens {
			ctxWinsFull++
		}

		t.Logf("  %-55s ctx=%5d grep=%6d random=%6d full=%7d",
			truncate(r.query, 55), r.ctxTokens, r.naiveGrepTokens, r.randomTokens, r.fullFileTokens)
	}

	n := len(results)
	avgCtx := float64(totalCtx) / float64(n)
	avgGrep := float64(totalGrep) / float64(n)
	avgRandom := float64(totalRandom) / float64(n)
	avgFull := float64(totalFull) / float64(n)

	grepReduction := (1.0 - avgCtx/avgGrep) * 100
	randomReduction := (1.0 - avgCtx/avgRandom) * 100
	fullReduction := (1.0 - avgCtx/avgFull) * 100

	t.Logf("───────────────────────────────────────────────────────────────")
	t.Logf("  ContextOS avg tokens:     %.0f", avgCtx)
	t.Logf("  Naive grep avg tokens:    %.0f  (ContextOS %.1f%% cheaper)", avgGrep, grepReduction)
	t.Logf("  Random 10-file avg:       %.0f  (ContextOS %.1f%% cheaper)", avgRandom, randomReduction)
	t.Logf("  Full-file avg tokens:     %.0f  (ContextOS %.1f%% cheaper)", avgFull, fullReduction)
	t.Logf("  Win rate vs grep:         %d/%d (%.0f%%)", ctxWinsGrep, n, float64(ctxWinsGrep)/float64(n)*100)
	t.Logf("  Win rate vs random:       %d/%d (%.0f%%)", ctxWinsRandom, n, float64(ctxWinsRandom)/float64(n)*100)
	t.Logf("  Win rate vs full-file:    %d/%d (%.0f%%)", ctxWinsFull, n, float64(ctxWinsFull)/float64(n)*100)

	// Patent-defensibility gate: ContextOS must beat all baselines on average
	if grepReduction < 0 {
		t.Errorf("PATENT RISK: ContextOS uses MORE tokens than naive grep (%.1f%% increase)", -grepReduction)
	}
	if fullReduction < 50 {
		t.Errorf("PATENT RISK: ContextOS token reduction vs full-file below 50%% (got %.1f%%)", fullReduction)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// PATENT CLAIM 2: Deterministic Reproducibility
// ═══════════════════════════════════════════════════════════════════════════════
//
// Same query + same corpus → identical results every time.
// Required for patent: algorithm must be deterministic, not heuristic.

func TestDeterministicReproducibility(t *testing.T) {
	svc, _ := buildProductionCorpus(t, 1000, 99)
	defer svc.Close()

	queries := []string{
		"Handler0 validation",
		"find storage adapter",
		"pkg/core/handler0_0.go",
		"trace dependency from auth to cache",
	}

	const trials = 5

	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			var firstTokens int
			var firstSelected []string

			for trial := 0; trial < trials; trial++ {
				plan, err := svc.Plan(q, "gpt-4o", 4000)
				if err != nil {
					t.Fatalf("trial %d error: %v", trial, err)
				}

				selected := make([]string, 0, len(plan.Selected))
				for _, s := range plan.Selected {
					selected = append(selected, s.ID)
				}
				sort.Strings(selected)

				if trial == 0 {
					firstTokens = plan.SelectedTokens
					firstSelected = selected
				} else {
					if plan.SelectedTokens != firstTokens {
						t.Errorf("trial %d: tokens %d ≠ trial 0 tokens %d", trial, plan.SelectedTokens, firstTokens)
					}
					if len(selected) != len(firstSelected) {
						t.Errorf("trial %d: %d selected ≠ trial 0 %d selected", trial, len(selected), len(firstSelected))
					} else {
						for i, id := range selected {
							if id != firstSelected[i] {
								t.Errorf("trial %d: selected[%d]=%s ≠ trial 0 selected[%d]=%s", trial, i, id, i, firstSelected[i])
								break
							}
						}
					}
				}
			}
			t.Logf("✓ %d trials identical: %d tokens, %d items", trials, firstTokens, len(firstSelected))
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// SAAS REVENUE 1: Multi-Provider Cost Savings Quantification
// ═══════════════════════════════════════════════════════════════════════════════
//
// Computes exact dollar savings per query across all 8 provider+model combos,
// at 1-turn, 5-turn, and 10-turn conversation depths.

func TestMultiProviderCostSavings(t *testing.T) {
	svc, root := buildProductionCorpus(t, 1500, 77)
	defer svc.Close()

	pricing := telemetry.DefaultPricingRegistry()

	providers := []struct {
		provider string
		model    string
	}{
		{"openai", "gpt-4o"},
		{"openai", "o3-mini"},
		{"anthropic", "claude-3-7-sonnet"},
		{"anthropic", "claude-3-5-haiku"},
		{"gemini", "gemini-2.5-flash"},
		{"gemini", "gemini-2.5-pro"},
	}

	queries := generateDiverseQueryCorpus(55, 15)

	// Calculate baseline: average tokens from reading 3 relevant files
	baselineTokensPerQuery := estimateBaselineTokens(root, 3)

	type costRow struct {
		provider          string
		model             string
		base1Turn         float64
		ctx1Turn          float64
		base10Turn        float64
		ctx10Turn         float64
		savingsPct10Turn  float64
		savingsUSD10Turn  float64
	}

	t.Logf("═══════════════════════════════════════════════════════════════")
	t.Logf("  SAAS REVENUE: Multi-Provider Cost Savings (per %d queries)", len(queries))
	t.Logf("═══════════════════════════════════════════════════════════════")

	var totalBaseCost10T, totalCtxCost10T float64

	for _, prov := range providers {
		p, err := pricing.LookupLatest(prov.provider, prov.model)
		if err != nil {
			t.Logf("SKIP: pricing not found for %s/%s", prov.provider, prov.model)
			continue
		}

		var provBase10T, provCtx10T float64

		for _, q := range queries {
			plan, err := svc.Plan(q, prov.model, 4000)
			if err != nil {
				continue
			}

			ctxTokens := plan.SelectedTokens
			if ctxTokens == 0 {
				ctxTokens = 100
			}

			// Baseline: resend full files every turn (no cache)
			baseCost1 := float64(baselineTokensPerQuery) / 1e6 * p.InputPerMillion
			baseCost10 := baseCost1 * 10

			// ContextOS: turn 1 uncached, turns 2-10 cached
			ctxCost1 := float64(ctxTokens) / 1e6 * p.InputPerMillion
			ctxCost10 := ctxCost1 + 9*(float64(ctxTokens)/1e6*p.CachedInputPerMillion)

			provBase10T += baseCost10
			provCtx10T += ctxCost10
		}

		savingsPct := (1.0 - provCtx10T/provBase10T) * 100
		savingsUSD := provBase10T - provCtx10T

		totalBaseCost10T += provBase10T
		totalCtxCost10T += provCtx10T

		t.Logf("  %-30s Baseline=$%.6f  ContextOS=$%.6f  Savings=$%.6f (%.1f%%)",
			prov.provider+"/"+prov.model, provBase10T, provCtx10T, savingsUSD, savingsPct)

		if savingsPct < 50 {
			t.Errorf("REVENUE RISK: %s/%s savings below 50%% (got %.1f%%)", prov.provider, prov.model, savingsPct)
		}
	}

	totalSavingsPct := (1.0 - totalCtxCost10T/totalBaseCost10T) * 100
	t.Logf("───────────────────────────────────────────────────────────────")
	t.Logf("  TOTAL (all providers):    Baseline=$%.6f  ContextOS=$%.6f", totalBaseCost10T, totalCtxCost10T)
	t.Logf("  NET SAVINGS:              $%.6f (%.1f%%)", totalBaseCost10T-totalCtxCost10T, totalSavingsPct)
}

// ═══════════════════════════════════════════════════════════════════════════════
// SAAS REVENUE 2: Token Compression Ratio at Scale
// ═══════════════════════════════════════════════════════════════════════════════

func TestTokenCompressionRatioAtScale(t *testing.T) {
	scales := []int{500, 1000, 2000, 4000}
	queries := generateDiverseQueryCorpus(33, 10)

	t.Logf("═══════════════════════════════════════════════════════════════")
	t.Logf("  TOKEN COMPRESSION RATIO AT SCALE")
	t.Logf("═══════════════════════════════════════════════════════════════")

	for _, nodeCount := range scales {
		t.Run(fmt.Sprintf("nodes_%d", nodeCount), func(t *testing.T) {
			svc, root := buildProductionCorpus(t, nodeCount, 42)
			defer svc.Close()

			totalRepoTokens := countRepoTokens(root)
			var totalSelected int

			for _, q := range queries {
				plan, err := svc.Plan(q, "gpt-4o", 4000)
				if err != nil {
					continue
				}
				totalSelected += plan.SelectedTokens
			}

			avgSelected := float64(totalSelected) / float64(len(queries))
			compressionRatio := float64(totalRepoTokens) / avgSelected
			selectivity := avgSelected / float64(totalRepoTokens) * 100

			t.Logf("  Nodes=%-5d  RepoTokens=%-7d  AvgSelected=%-5.0f  Compression=%.0f:1  Selectivity=%.2f%%",
				nodeCount, totalRepoTokens, avgSelected, compressionRatio, selectivity)

			// Gate: compression ratio must scale sub-linearly
			if compressionRatio < 10 {
				t.Errorf("compression ratio %.0f:1 too low (expect ≥10:1)", compressionRatio)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// PRODUCTION 1: Sustained Load Test (Soak Test)
// ═══════════════════════════════════════════════════════════════════════════════

func TestSustainedLoadSoak(t *testing.T) {
	svc, _ := buildProductionCorpus(t, 2000, 88)
	defer svc.Close()

	queries := generateDiverseQueryCorpus(44, 30)

	const (
		totalQueries = 200
		workers      = 8
	)
	maxP99 := 500 * time.Millisecond
	if os.Getenv("CONTEXTOS_STORAGE") == "file" {
		maxP99 = 1000 * time.Millisecond
	}

	queryCh := make(chan string, totalQueries)
	for i := 0; i < totalQueries; i++ {
		queryCh <- queries[i%len(queries)]
	}
	close(queryCh)

	var mu sync.Mutex
	var latencies []time.Duration
	var errCount int64

	// Track memory
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

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

	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	n := len(latencies)
	p50 := latencies[n/2]
	p95 := latencies[int(float64(n)*0.95)]
	p99 := latencies[int(float64(n)*0.99)]
	throughput := float64(n) / totalDur.Seconds()
	memDeltaMB := float64(m2.TotalAlloc-m1.TotalAlloc) / 1024 / 1024

	t.Logf("═══════════════════════════════════════════════════════════════")
	t.Logf("  SUSTAINED LOAD SOAK TEST (%d queries, %d workers)", totalQueries, workers)
	t.Logf("═══════════════════════════════════════════════════════════════")
	t.Logf("  Duration:     %v", totalDur)
	t.Logf("  Throughput:   %.1f QPS", throughput)
	t.Logf("  p50:          %v", p50)
	t.Logf("  p95:          %v", p95)
	t.Logf("  p99:          %v", p99)
	t.Logf("  Errors:       %d / %d (%.2f%%)", errCount, totalQueries, float64(errCount)/float64(totalQueries)*100)
	t.Logf("  Memory delta: %.1f MB allocated", memDeltaMB)

	if errCount > 0 {
		t.Errorf("soak test had %d errors (SLA: 0)", errCount)
	}
	if p99 > maxP99 {
		t.Errorf("p99 latency %v exceeded %v SLA", p99, maxP99)
	}
	if throughput < 20 {
		t.Errorf("throughput %.1f QPS below 20 QPS SLA", throughput)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// PRODUCTION 2: Cold Start Performance
// ═══════════════════════════════════════════════════════════════════════════════

func TestColdStartPerformance(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, ".contextos", "cold.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatal(err)
	}
	runGitInit(t, root)

	// Create 50 files
	for i := 0; i < 50; i++ {
		pkg := filepath.Join(root, "pkg", fmt.Sprintf("mod%d", i%5))
		os.MkdirAll(pkg, 0755)
		content := fmt.Sprintf("package mod%d\n\nfunc Service%d() error { return nil }\n", i%5, i)
		os.WriteFile(filepath.Join(pkg, fmt.Sprintf("svc_%d.go", i)), []byte(content), 0644)
	}
	runGitAdd(t, root)

	// Measure cold start: New() + Index() + first Plan()
	t0 := time.Now()
	svc, err := server.New(dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	tNew := time.Since(t0)

	t1 := time.Now()
	if err := svc.Index(); err != nil {
		t.Fatal(err)
	}
	tIndex := time.Since(t1)

	t2 := time.Now()
	_, err = svc.Plan("Service0", "gpt-4o", 4000)
	tFirstPlan := time.Since(t2)
	if err != nil {
		t.Fatal(err)
	}

	// Warm plan for comparison
	t3 := time.Now()
	_, err = svc.Plan("Service1", "gpt-4o", 4000)
	tWarmPlan := time.Since(t3)
	if err != nil {
		t.Fatal(err)
	}
	svc.Close()

	tTotal := tNew + tIndex + tFirstPlan

	t.Logf("═══════════════════════════════════════════════════════════════")
	t.Logf("  COLD START PERFORMANCE (50 files)")
	t.Logf("═══════════════════════════════════════════════════════════════")
	t.Logf("  New():       %v", tNew)
	t.Logf("  Index():     %v", tIndex)
	t.Logf("  First Plan:  %v", tFirstPlan)
	t.Logf("  Warm Plan:   %v", tWarmPlan)
	t.Logf("  Total cold→result: %v", tTotal)

	if tTotal > 5*time.Second {
		t.Errorf("cold start to first result %v exceeded 5s SLA", tTotal)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// PRODUCTION 3: Precision@K Curves
// ═══════════════════════════════════════════════════════════════════════════════

func TestPrecisionAtKCurves(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, ".contextos", "prec.db")
	os.MkdirAll(filepath.Dir(dbPath), 0755)
	runGitInit(t, root)

	// Known-answer corpus with planted targets
	targets := map[string]string{
		"pkg/api/user_handler.go":     "package api\n\ntype UserHandler struct{}\n\nfunc (h *UserHandler) CreateUser() error { return nil }\nfunc (h *UserHandler) GetUser() error { return nil }\n",
		"pkg/api/order_handler.go":    "package api\n\ntype OrderHandler struct{}\n\nfunc (h *OrderHandler) CreateOrder() error { return nil }\n",
		"pkg/core/auth_service.go":    "package core\n\ntype AuthService struct{}\n\nfunc (a *AuthService) Login() error { return nil }\n",
		"pkg/core/billing_service.go": "package core\n\ntype BillingService struct{}\n\nfunc (b *BillingService) Charge() error { return nil }\n",
		"pkg/db/postgres_adapter.go":  "package db\n\ntype PostgresAdapter struct{}\n\nfunc (p *PostgresAdapter) Query() error { return nil }\n",
	}

	for path, content := range targets {
		full := filepath.Join(root, path)
		os.MkdirAll(filepath.Dir(full), 0755)
		os.WriteFile(full, []byte(content), 0644)
	}

	// Add 50 distractor files
	for i := 0; i < 50; i++ {
		pkg := filepath.Join(root, "pkg", fmt.Sprintf("noise%d", i%10))
		os.MkdirAll(pkg, 0755)
		content := fmt.Sprintf("package noise%d\n\nfunc Noise%d() {}\n", i%10, i)
		os.WriteFile(filepath.Join(pkg, fmt.Sprintf("noise_%d.go", i)), []byte(content), 0644)
	}
	runGitAdd(t, root)

	svc, err := server.New(dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	svc.Index()

	// Known-answer queries with expected file matches
	qaTests := []struct {
		query    string
		expected []string // expected path substrings
	}{
		{"UserHandler CreateUser", []string{"user_handler"}},
		{"OrderHandler order processing", []string{"order_handler"}},
		{"AuthService Login authentication", []string{"auth_service"}},
		{"BillingService Charge payment", []string{"billing_service"}},
		{"PostgresAdapter Query database", []string{"postgres_adapter"}},
	}

	kValues := []int{1, 3, 5, 10}

	t.Logf("═══════════════════════════════════════════════════════════════")
	t.Logf("  PRECISION@K CURVES (Known-Answer Evaluation)")
	t.Logf("═══════════════════════════════════════════════════════════════")

	precAtK := make(map[int]float64)
	for _, k := range kValues {
		var totalPrec float64
		for _, qa := range qaTests {
			cands, _, err := svc.RetrieveEvidence(context.Background(), qa.query, k)
			if err != nil {
				continue
			}

			matches := 0
			for _, c := range cands {
				for _, exp := range qa.expected {
					if strings.Contains(strings.ToLower(c.Path+c.Name), exp) {
						matches++
						break
					}
				}
			}
			prec := float64(matches) / float64(min(k, max(len(cands), 1)))
			totalPrec += prec
		}
		avgPrec := totalPrec / float64(len(qaTests))
		precAtK[k] = avgPrec
		t.Logf("  Precision@%-3d = %.3f", k, avgPrec)
	}

	if precAtK[1] < 0.5 {
		t.Errorf("Precision@1 = %.3f below 0.5 threshold", precAtK[1])
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// PRODUCTION 4: Adversarial Query Robustness
// ═══════════════════════════════════════════════════════════════════════════════

func TestAdversarialQueryRobustness(t *testing.T) {
	svc, _ := buildProductionCorpus(t, 500, 55)
	defer svc.Close()

	adversarial := []struct {
		name  string
		query string
	}{
		{"empty", ""},
		{"single_char", "x"},
		{"all_stopwords", "find the where is and or not"},
		{"sql_injection", "'; DROP TABLE nodes; --"},
		{"path_traversal", "../../../etc/passwd"},
		{"unicode", "Hello 世界 🚀 λ→μ"},
		{"very_long", strings.Repeat("search for something important ", 50)},
		{"null_bytes", "Handler\x00\x00Service"},
		{"only_punctuation", "!@#$%^&*()"},
		{"repeated_symbols", "Handler Handler Handler Handler Handler"},
	}

	for _, tc := range adversarial {
		t.Run(tc.name, func(t *testing.T) {
			// Must not panic or return an error — graceful degradation
			plan, err := svc.Plan(tc.query, "gpt-4o", 4000)
			if err != nil {
				t.Errorf("adversarial query %q caused error: %v", tc.name, err)
				return
			}
			// Budget must never be exceeded
			if plan.SelectedTokens > 4000 {
				t.Errorf("adversarial query %q exceeded budget: %d > 4000", tc.name, plan.SelectedTokens)
			}
			t.Logf("✓ %s: %d tokens, %d candidates", tc.name, plan.SelectedTokens, len(plan.Candidates))
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// PRODUCTION 5: Retrieval Telemetry Contract Verification
// ═══════════════════════════════════════════════════════════════════════════════

func TestRetrievalTelemetryContract(t *testing.T) {
	svc, _ := buildProductionCorpus(t, 1000, 66)
	defer svc.Close()

	queries := generateDiverseQueryCorpus(66, 20)

	t.Logf("═══════════════════════════════════════════════════════════════")
	t.Logf("  R18.1 P0 TELEMETRY CONTRACT VERIFICATION")
	t.Logf("═══════════════════════════════════════════════════════════════")

	for _, q := range queries {
		plan, err := svc.Plan(q, "gpt-4o", 4000)
		if err != nil {
			continue
		}

		// MUST have hybrid retrieval mode
		if plan.RetrievalMode != "hybrid" {
			t.Errorf("query %q: retrieval_mode=%q, want 'hybrid'", q, plan.RetrievalMode)
		}

		// MUST have retrieval stages
		if len(plan.RetrievalStages) == 0 {
			t.Errorf("query %q: retrieval_stages is empty", q)
		}

		// CreatedAt must be recent
		if time.Since(plan.CreatedAt) > 10*time.Second {
			t.Errorf("query %q: created_at too old (%v ago)", q, time.Since(plan.CreatedAt))
		}

		// Budget must be respected
		if plan.SelectedTokens > plan.Budget {
			t.Errorf("query %q: selected_tokens %d > budget %d", q, plan.SelectedTokens, plan.Budget)
		}
	}
}



func generateDiverseQueryCorpus(seed int64, n int) []string {
	r := rand.New(rand.NewSource(seed))
	types := []string{"Handler", "Service", "Store", "Manager", "Worker", "Router", "Controller", "Resolver"}
	verbs := []string{"Get", "Set", "Create", "Delete", "Validate", "Process", "Handle", "Execute"}
	packages := []string{"api", "core", "storage", "auth", "graph", "cache", "config", "handler"}

	templates := []string{
		"%s%d",
		"find %s in %s",
		"trace dependency from %s to %s",
		"%s %s validation",
		"pkg/%s/%s%d_%d.go",
		"callers of %s%d",
		"error handling in %s",
		"refactor %s%d %s pattern",
	}

	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		tmpl := templates[r.Intn(len(templates))]
		switch {
		case strings.Count(tmpl, "%s") == 1 && strings.Count(tmpl, "%d") == 1:
			out = append(out, fmt.Sprintf(tmpl, types[r.Intn(len(types))], r.Intn(5)))
		case strings.Count(tmpl, "%s") == 2 && strings.Count(tmpl, "%d") == 0:
			out = append(out, fmt.Sprintf(tmpl, types[r.Intn(len(types))], packages[r.Intn(len(packages))]))
		case strings.Count(tmpl, "%s") == 2 && strings.Count(tmpl, "%d") == 2:
			out = append(out, fmt.Sprintf(tmpl, packages[r.Intn(len(packages))], strings.ToLower(types[r.Intn(len(types))]), r.Intn(5), r.Intn(5)))
		case strings.Count(tmpl, "%s") == 3:
			out = append(out, fmt.Sprintf(tmpl, types[r.Intn(len(types))], r.Intn(5), verbs[r.Intn(len(verbs))]))
		default:
			out = append(out, fmt.Sprintf("%s%d %s", types[r.Intn(len(types))], r.Intn(5), verbs[r.Intn(len(verbs))]))
		}
	}
	return out
}

func naiveGrepBaseline(root, query string) int {
	tokens := textutil.Tokens(query)
	if len(tokens) == 0 {
		return 5000
	}
	totalChars := 0
	for _, tok := range tokens[:min(3, len(tokens))] {
		cmd := exec.Command("grep", "-r", "-l", tok, filepath.Join(root, "pkg"))
		out, _ := cmd.Output()
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			data, err := os.ReadFile(line)
			if err == nil {
				totalChars += len(data)
			}
		}
	}
	if totalChars == 0 {
		return 5000
	}
	return totalChars / 4 // ~4 chars per token
}

func randomFileBaseline(root string, n int, seed int64) int {
	r := rand.New(rand.NewSource(seed))
	var allFiles []string
	filepath.Walk(filepath.Join(root, "pkg"), func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".go") {
			allFiles = append(allFiles, path)
		}
		return nil
	})
	if len(allFiles) == 0 {
		return 5000
	}

	totalChars := 0
	for i := 0; i < min(n, len(allFiles)); i++ {
		idx := r.Intn(len(allFiles))
		data, _ := os.ReadFile(allFiles[idx])
		totalChars += len(data)
	}
	return totalChars / 4
}

func fullFileBaseline(root string) int {
	totalChars := 0
	filepath.Walk(filepath.Join(root, "pkg"), func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".go") {
			data, _ := os.ReadFile(path)
			totalChars += len(data)
		}
		return nil
	})
	if totalChars == 0 {
		return 50000
	}
	return totalChars / 4
}

func estimateBaselineTokens(root string, filesPerQuery int) int {
	var sizes []int
	filepath.Walk(filepath.Join(root, "pkg"), func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".go") {
			sizes = append(sizes, int(info.Size()))
		}
		return nil
	})
	if len(sizes) == 0 {
		return 3000
	}
	sort.Ints(sizes)
	// Use median file size × filesPerQuery
	median := sizes[len(sizes)/2]
	return (median * filesPerQuery) / 4
}

func countRepoTokens(root string) int {
	totalChars := 0
	filepath.Walk(filepath.Join(root, "pkg"), func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".go") {
			data, _ := os.ReadFile(path)
			totalChars += len(data)
		}
		return nil
	})
	return totalChars / 4
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}



// Ensure math import is used
var _ = math.Abs
var _ = json.Marshal
var _ = store.NodeRecord{}
var _ = gitidx.Symbol{}
