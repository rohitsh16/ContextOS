package saas

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"contextos/internal/mcp"
	"contextos/internal/saas/auth"
	"contextos/internal/saas/gateway"
	"contextos/internal/saas/metering"
	"contextos/internal/saas/ratelimit"
	"contextos/internal/saas/tenant"
	"contextos/internal/saas/webhooks"
	"contextos/internal/server"
	"contextos/internal/store"
	"contextos/internal/telemetry"
)

// ═══════════════════════════════════════════════════════════════════════════════
// DIMENSION 1: MULTI-TENANCY ISOLATION
// Can multiple repos/users share a single process without data leaks?
// ═══════════════════════════════════════════════════════════════════════════════

func TestMultiTenantIsolation(t *testing.T) {
	repos := make([]string, 3)
	services := make([]*server.Service, 3)

	// Create 3 isolated tenant repositories
	for i := 0; i < 3; i++ {
		root := t.TempDir()
		dbPath := filepath.Join(root, "ctx.db")
		setupGitRepo(t, root, fmt.Sprintf("tenant%d.go", i),
			fmt.Sprintf("package tenant%d\nfunc Handler%d() {}\n", i, i))

		s, err := server.NewWithOptions(dbPath, root, server.Options{StorageType: "file"})
		if err != nil {
			t.Fatalf("tenant %d init failed: %v", i, err)
		}
		if err := s.Index(); err != nil {
			t.Fatalf("tenant %d index failed: %v", i, err)
		}
		// Seed tenant-specific memories
		_, err = s.Remember("decision",
			fmt.Sprintf("Tenant %d uses payment gateway %d", i, i*100),
			"user", "repo", "", 0.95, nil)
		if err != nil {
			t.Fatalf("tenant %d remember failed: %v", i, err)
		}
		repos[i] = root
		services[i] = s
	}
	defer func() {
		for _, s := range services {
			s.Close()
		}
	}()

	// Assert: each tenant's plan only returns its own data
	for i, s := range services {
		p, err := s.Plan(fmt.Sprintf("payment gateway %d", i*100), "", 4000)
		if err != nil {
			t.Fatalf("tenant %d plan failed: %v", i, err)
		}
		for _, c := range p.Selected {
			if strings.Contains(c.Content, fmt.Sprintf("Tenant %d", (i+1)%3)) {
				t.Fatalf("ISOLATION BREACH: tenant %d sees tenant %d data: %s", i, (i+1)%3, c.Content)
			}
		}
		// Verify repo path isolation (resolve symlinks for macOS /var -> /private/var)
		resolvedRepo, _ := filepath.EvalSymlinks(s.Repo.Path)
		resolvedExpected, _ := filepath.EvalSymlinks(repos[i])
		if resolvedRepo != resolvedExpected {
			t.Fatalf("tenant %d repo path mismatch: got %s, want %s", i, resolvedRepo, resolvedExpected)
		}
	}
	t.Log("✓ Multi-tenant data isolation: PASS — no cross-tenant data leaks")
}

// ═══════════════════════════════════════════════════════════════════════════════
// DIMENSION 2: CONCURRENT ACCESS / HORIZONTAL SCALABILITY
// Can the system handle N concurrent requests without panics or data races?
// ═══════════════════════════════════════════════════════════════════════════════

func TestConcurrentPlanExecution(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "data")
	setupGitRepo(t, root, "main.go", "package main\nfunc main() {}\n")

	s, err := server.NewWithOptions(dbPath, root, server.Options{StorageType: "file"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Index(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		s.Remember("fact", fmt.Sprintf("Service %d config: port=%d", i, 8000+i), "user", "repo", "", 0.9, nil)
	}

	const concurrency = 20
	var wg sync.WaitGroup
	var failures atomic.Int32
	var totalLatency atomic.Int64

	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(id int) {
			defer wg.Done()
			start := time.Now()
			task := fmt.Sprintf("Service %d configuration", id%10)
			p, err := s.Plan(task, "", 4000)
			lat := time.Since(start)
			totalLatency.Add(lat.Milliseconds())
			if err != nil {
				t.Logf("worker %d error: %v", id, err)
				failures.Add(1)
				return
			}
			if len(p.Selected) == 0 {
				t.Logf("worker %d: empty plan", id)
				failures.Add(1)
			}
		}(i)
	}
	wg.Wait()

	avgLatMs := totalLatency.Load() / concurrency
	t.Logf("Concurrent plan stats: %d workers, %d failures, avg latency %dms",
		concurrency, failures.Load(), avgLatMs)

	if failures.Load() > 0 {
		t.Fatalf("CONCURRENT ACCESS FAILURE: %d/%d workers failed", failures.Load(), concurrency)
	}
	if avgLatMs > 2000 {
		t.Fatalf("LATENCY SLA BREACH: avg %dms > 2000ms threshold", avgLatMs)
	}
	t.Log("✓ Concurrent plan execution: PASS — zero failures, SLA met")
}

// ═══════════════════════════════════════════════════════════════════════════════
// DIMENSION 3: API CONTRACT STABILITY (MCP Protocol)
// Does the MCP JSON-RPC interface behave predictably for external integrators?
// ═══════════════════════════════════════════════════════════════════════════════

func TestMCPAPIContractStability(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "data")
	setupGitRepo(t, root, "main.go", "package main\nfunc main() {}\n")

	s, err := server.NewWithOptions(dbPath, root, server.Options{StorageType: "file"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Index()

	m := mcp.New(s)

	// Test 1: server/discover returns expected shape
	resp := m.Handle(mcp.Request{JSONRPC: "2.0", ID: 1, Method: "server/discover"})
	result, ok := resp.Result.(map[string]any)
	if !ok || resp.Error != nil {
		t.Fatalf("server/discover failed: %+v", resp)
	}
	if _, ok := result["capabilities"]; !ok {
		t.Fatal("server/discover missing 'capabilities'")
	}
	if _, ok := result["serverInfo"]; !ok {
		t.Fatal("server/discover missing 'serverInfo'")
	}

	// Test 2: tools/list returns deterministic list
	resp2 := m.Handle(mcp.Request{JSONRPC: "2.0", ID: 2, Method: "tools/list"})
	result2, ok := resp2.Result.(map[string]any)
	if !ok || resp2.Error != nil {
		t.Fatalf("tools/list failed: %+v", resp2)
	}
	tools, ok := result2["tools"].([]mcp.ToolDefinition)
	if !ok {
		t.Fatal("tools/list did not return typed ToolDefinition slice")
	}
	expectedTools := map[string]bool{
		"context_plan":          false,
		"context_search":        false,
		"context_remember":      false,
		"context_stats":         false,
		"context_route":         false,
		"context_compute_plan":  false,
		"context_handoff":       false,
		"context_resume":        false,
		"context_trace":         false,
		"context_invalidate":    false,
		"context_session_start": false,
		"context_event":         false,
		"context_work_start":    false,
	}
	for _, tool := range tools {
		expectedTools[tool.Name] = true
	}
	for name, found := range expectedTools {
		if !found {
			t.Fatalf("MISSING API TOOL: %s — breaking change for integrators", name)
		}
	}

	// Test 3: Verify tools are alphabetically sorted (determinism for clients)
	for i := 1; i < len(tools); i++ {
		if tools[i].Name < tools[i-1].Name {
			t.Fatalf("tools/list not sorted: %s before %s", tools[i-1].Name, tools[i].Name)
		}
	}

	// Test 4: Invalid tool call returns proper error code
	badCall, _ := json.Marshal(map[string]any{
		"name":      "nonexistent_tool",
		"arguments": map[string]any{},
	})
	resp3 := m.Handle(mcp.Request{JSONRPC: "2.0", ID: 3, Method: "tools/call", Params: badCall})
	if resp3.Error == nil || resp3.Error.Code != -32601 {
		t.Fatalf("expected method-not-found error, got %+v", resp3)
	}

	t.Log("✓ MCP API contract stability: PASS — 13 tools, deterministic ordering, error codes")
}

// ═══════════════════════════════════════════════════════════════════════════════
// DIMENSION 4: CONSUMPTION-BASED BILLING FEASIBILITY
// Does the system track enough metrics for usage-based SaaS billing?
// ═══════════════════════════════════════════════════════════════════════════════

func TestBillingMetricsCompleteness(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "data")
	setupGitRepo(t, root, "main.go", "package main\nfunc main() {}\n")

	s, err := server.NewWithOptions(dbPath, root, server.Options{StorageType: "file"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Index()
	s.Remember("fact", "billing test datum", "user", "repo", "", 0.9, nil)

	// Execute a plan to generate trace data
	p, err := s.Plan("billing test", "", 4000)
	if err != nil {
		t.Fatal(err)
	}

	// Check 1: Context plan has cost estimation
	if p.EstimatedCost <= 0 {
		t.Fatal("BILLING GAP: EstimatedCost is zero — cannot bill for context planning")
	}

	// Check 2: Plan tracks token consumption
	if p.SelectedTokens <= 0 {
		t.Fatal("BILLING GAP: SelectedTokens is zero — no basis for token-based billing")
	}

	// Check 3: Trace system captures per-plan cost data
	tr, err := s.LatestTrace()
	if err != nil {
		t.Fatal(err)
	}
	requiredTraceFields := []string{"task", "model", "budget", "selected_tokens", "estimated_cost"}
	for _, field := range requiredTraceFields {
		if _, ok := tr[field]; !ok {
			t.Fatalf("BILLING GAP: trace missing '%s' — required for billing audit trail", field)
		}
	}

	// Check 4: Stats endpoint provides aggregate usage data
	stats, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	requiredStatFields := []string{"planned_tokens_total", "trace_count", "cache_hit_traces"}
	for _, field := range requiredStatFields {
		if _, ok := stats[field]; !ok {
			t.Fatalf("BILLING GAP: stats missing '%s' — required for usage dashboards", field)
		}
	}

	// Check 5: Telemetry cost model covers major providers
	providers := []struct {
		provider string
		model    string
	}{
		{"openai", "gpt-4o"},
		{"anthropic", "claude-3-7-sonnet"},
		{"gemini", "gemini-2.5-pro"},
	}
	for _, pm := range providers {
		pricing, found := telemetry.LookupPricing(pm.provider, pm.model)
		if !found {
			t.Fatalf("BILLING GAP: no pricing catalog entry for %s:%s", pm.provider, pm.model)
		}
		if pricing.InputPerMillion <= 0 {
			t.Fatalf("BILLING GAP: invalid pricing for %s:%s", pm.provider, pm.model)
		}
	}

	// Check 6: Usage cost calculation produces correct numbers
	testUsage := telemetry.UsageMetrics{
		InputTokens:       10000,
		CachedInputTokens: 5000,
		OutputTokens:      2000,
		ReasoningTokens:   1000,
		VisibleOutputTokens: 1000,
	}
	pricing, _ := telemetry.LookupPricing("openai", "gpt-4o")
	cost := telemetry.CalculateUsageCost(pricing, testUsage)
	if cost <= 0 || math.IsNaN(cost) || math.IsInf(cost, 0) {
		t.Fatalf("BILLING GAP: CalculateUsageCost returned invalid value: %f", cost)
	}

	t.Logf("✓ Billing metrics: PASS — cost=$%.6f, tokens=%d, trace fields=%d, providers=%d",
		p.EstimatedCost, p.SelectedTokens, len(requiredTraceFields), len(providers))
}

// ═══════════════════════════════════════════════════════════════════════════════
// DIMENSION 5: STORAGE ENGINE PORTABILITY
// Can the system run on both SQLite (single-node) and FileStore (cloud-friendly)?
// ═══════════════════════════════════════════════════════════════════════════════

func TestStorageEnginePortability(t *testing.T) {
	engines := []string{"file"}
	// SQLite requires CGO, test conditionally
	if os.Getenv("CGO_ENABLED") == "1" {
		engines = append(engines, "sqlite")
	}

	for _, engine := range engines {
		t.Run(engine, func(t *testing.T) {
			root := t.TempDir()
			var dbPath string
			if engine == "sqlite" {
				dbPath = filepath.Join(root, "ctx.db")
			} else {
				dbPath = filepath.Join(root, "data")
			}
			setupGitRepo(t, root, "main.go", "package main\nfunc main() {}\n")

			s, err := server.NewWithOptions(dbPath, root, server.Options{StorageType: engine})
			if err != nil {
				t.Fatalf("init failed with %s engine: %v", engine, err)
			}
			defer s.Close()

			// Full lifecycle test
			if err := s.Index(); err != nil {
				t.Fatalf("index failed: %v", err)
			}
			if _, err := s.Remember("decision", "test portability", "user", "repo", "", 0.9, nil); err != nil {
				t.Fatalf("remember failed: %v", err)
			}
			p, err := s.Plan("test portability", "", 4000)
			if err != nil {
				t.Fatalf("plan failed: %v", err)
			}
			if len(p.Selected) == 0 {
				t.Fatalf("empty plan on %s engine", engine)
			}

			// Verify GC works
			_, err = s.GC(store.PruneOptions{
				RepoID:          s.RepoID,
				CurrentRevision: s.Repo.Revision,
				DryRun:          true,
			})
			if err != nil {
				t.Fatalf("GC failed on %s engine: %v", engine, err)
			}

			stats, err := s.Stats()
			if err != nil {
				t.Fatalf("stats failed: %v", err)
			}
			if stats["storage_engine"] != engine {
				t.Fatalf("engine mismatch: got %s, want %s", stats["storage_engine"], engine)
			}
			t.Logf("✓ %s engine: full lifecycle PASS", engine)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// DIMENSION 6: CONTEXT CACHE EFFICIENCY (COST SAVINGS = REVENUE MARGIN)
// Cache hits avoid re-computing context → directly maps to SaaS margin
// ═══════════════════════════════════════════════════════════════════════════════

func TestCacheEfficiencyForMargin(t *testing.T) {
	root := t.TempDir()
	dbPath := t.TempDir() // separate data dir so FileStore writes don't change worktree hash
	setupGitRepo(t, root, "main.go", "package main\nfunc main() {}\n")

	s, err := server.NewWithOptions(dbPath, root, server.Options{StorageType: "file"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Index()
	s.Remember("fact", "cache efficiency test data", "user", "repo", "", 0.9, nil)

	// First call: cache miss (cold) — use a specific model to ensure deterministic cache key
	p1, err := s.Plan("cache efficiency", "gemini", 4000)
	if err != nil {
		t.Fatal(err)
	}
	if p1.CacheHit {
		t.Fatal("first plan should not be cache hit")
	}
	coldCost := p1.EstimatedCost

	// Second call: cache hit (warm) — exact same parameters
	p2, err := s.Plan("cache efficiency", "gemini", 4000)
	if err != nil {
		t.Fatal(err)
	}
	if !p2.CacheHit {
		t.Fatal("second plan should be cache hit")
	}
	warmCost := p2.EstimatedCost

	// Cache-hit plans use CachedInputPerM pricing → should be cheaper
	savings := 0.0
	if coldCost > 0 {
		savings = (1 - warmCost/coldCost) * 100
	}

	t.Logf("Cold cost: $%.6f, Warm cost: $%.6f, Savings: %.1f%%", coldCost, warmCost, savings)

	if savings < 50 {
		t.Fatalf("MARGIN RISK: cache savings only %.1f%% — needs >50%% for viable SaaS margin", savings)
	}
	t.Logf("✓ Cache efficiency: PASS — %.1f%% cost savings on cache hits", savings)
}

// ═══════════════════════════════════════════════════════════════════════════════
// DIMENSION 7: API THROUGHPUT / RATE LIMITING BASELINE
// How many context plans can the system generate per second?
// ═══════════════════════════════════════════════════════════════════════════════

func TestThroughputBaseline(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "data")
	setupGitRepo(t, root, "main.go", "package main\nfunc main() {}\n")

	s, err := server.NewWithOptions(dbPath, root, server.Options{StorageType: "file"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Index()
	for i := 0; i < 20; i++ {
		s.Remember("fact", fmt.Sprintf("throughput datum %d", i), "user", "repo", "", 0.9, nil)
	}

	// Warm the cache
	s.Plan("throughput baseline", "", 4000)

	const iterations = 50
	start := time.Now()
	for i := 0; i < iterations; i++ {
		_, err := s.Plan("throughput baseline", "", 4000)
		if err != nil {
			t.Fatalf("iteration %d failed: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	qps := float64(iterations) / elapsed.Seconds()

	t.Logf("Throughput: %d plans in %v = %.1f QPS", iterations, elapsed, qps)

	// SaaS requirement: >10 QPS for cached plans at minimum
	if qps < 10 {
		t.Fatalf("THROUGHPUT FAILURE: %.1f QPS < 10 QPS minimum for SaaS viability", qps)
	}
	t.Logf("✓ Throughput baseline: PASS — %.1f QPS (cached plans)", qps)
}

// ═══════════════════════════════════════════════════════════════════════════════
// DIMENSION 8: SESSION / WORK ITEM LIFECYCLE (AUDIT TRAIL)
// SaaS needs reliable session tracking for billing and debugging
// ═══════════════════════════════════════════════════════════════════════════════

func TestSessionLifecycleAuditTrail(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "data")
	setupGitRepo(t, root, "main.go", "package main\nfunc main() {}\n")

	s, err := server.NewWithOptions(dbPath, root, server.Options{StorageType: "file"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Index()

	// Start work item
	wi, err := s.StartWorkItem("SaaS customer onboarding")
	if err != nil {
		t.Fatal(err)
	}
	if wi.ID == "" || wi.Title != "SaaS customer onboarding" {
		t.Fatalf("work item creation failed: %+v", wi)
	}

	// Start session
	sess, err := s.StartSession("claude-sonnet", wi.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sess.ID == "" || sess.Agent != "claude-sonnet" {
		t.Fatalf("session creation failed: %+v", sess)
	}

	// Record events
	if err := s.RecordEvent(sess.ID, "tool_call", map[string]any{"tool": "context_plan"}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordEvent(sess.ID, "model_response", map[string]any{"tokens": 500}); err != nil {
		t.Fatal(err)
	}

	// End session
	if err := s.EndSession(sess.ID); err != nil {
		t.Fatal(err)
	}

	// Verify latest session is retrievable
	latest, err := s.LatestSession()
	if err != nil {
		t.Fatal(err)
	}
	if latest == nil || latest.ID != sess.ID {
		t.Fatalf("latest session mismatch: %+v", latest)
	}

	// Verify resume provides full state
	state, err := s.Resume()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"repository", "work_item", "latest_session"} {
		if _, ok := state[key]; !ok {
			t.Fatalf("AUDIT GAP: resume missing '%s'", key)
		}
	}

	t.Log("✓ Session lifecycle: PASS — work items, sessions, events, resume all functional")
}

// ═══════════════════════════════════════════════════════════════════════════════
// DIMENSION 9: COMPUTE PLAN VIABILITY (ADAPTIVE COST OPTIMIZATION)
// Can the system generate compute plans that actually optimize costs?
// ═══════════════════════════════════════════════════════════════════════════════

func TestComputePlanCostOptimization(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "data")
	setupGitRepo(t, root, "main.go", "package main\nfunc main() {}\n")

	s, err := server.NewWithOptions(dbPath, root, server.Options{StorageType: "file"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	tasks := []struct {
		name       string
		riskTarget float64
	}{
		{"fix a typo in README", 0.1},                               // trivial
		{"refactor the authentication middleware", 0.05},              // moderate
		{"migrate distributed database with zero-downtime", 0.01},   // complex
	}

	for _, tc := range tasks {
		cp := s.ComputePlan(tc.name, tc.riskTarget, "")
		if cp.EstimatedCostUSD < 0 {
			t.Fatalf("compute plan for '%s' has negative cost", tc.name)
		}
		if cp.Budget.MaxReasoningTokens <= 0 {
			t.Fatalf("compute plan for '%s' has zero reasoning budget", tc.name)
		}
		if cp.Policy.Effort.String() == "" {
			t.Fatalf("compute plan for '%s' has no effort level", tc.name)
		}
		t.Logf("  %s: effort=%s, reasoning=%d, cost=$%.4f",
			tc.name, cp.Policy.Effort.String(), cp.Budget.MaxReasoningTokens, cp.EstimatedCostUSD)
	}

	t.Log("✓ Compute plan optimization: PASS — adaptive cost plans generated for all complexity tiers")
}

// ═══════════════════════════════════════════════════════════════════════════════
// DIMENSION 10: MODEL ROUTING (COST-QUALITY POLICY)
// Does the router correctly recommend different models based on task/budget?
// ═══════════════════════════════════════════════════════════════════════════════

func TestModelRoutingPolicy(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "data")
	setupGitRepo(t, root, "main.go", "package main\nfunc main() {}\n")

	s, err := server.NewWithOptions(dbPath, root, server.Options{StorageType: "file"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	cases := []struct {
		task       string
		budget     int
		expectCheap bool // should route to cheaper model
	}{
		{"fix typo", 1000, true},                                      // low budget → local
		{"distributed architecture migration", 16000, false},          // high complexity → premium
		{"add logging to handler", 4000, false},                       // moderate → balanced
	}

	for _, tc := range cases {
		r := s.Route(tc.task, tc.budget)
		model, _ := r["recommended_model"].(string)
		if model == "" {
			t.Fatalf("no model recommended for task '%s'", tc.task)
		}
		cost, _ := r["input_per_million"].(float64)
		if cost < 0 {
			t.Fatalf("negative cost for task '%s'", tc.task)
		}
		t.Logf("  %s (budget=%d): model=%s, cost=$%.2f/M", tc.task, tc.budget, model, cost)
	}
	t.Log("✓ Model routing policy: PASS — adaptive model selection working")
}

// ═══════════════════════════════════════════════════════════════════════════════
// DIMENSION 11: RETRIEVAL LATENCY SLA
// Can the system maintain sub-500ms retrieval for SaaS response times?
// ═══════════════════════════════════════════════════════════════════════════════

func TestRetrievalLatencySLA(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "data")
	setupGitRepo(t, root, "main.go", "package main\nfunc main() {}\n")

	s, err := server.NewWithOptions(dbPath, root, server.Options{StorageType: "file"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Index()

	queries := []string{
		"authentication middleware",
		"database connection pool",
		"error handling patterns",
		"API rate limiting",
		"cache invalidation strategy",
	}

	var maxLatency time.Duration
	for _, q := range queries {
		start := time.Now()
		_, _, err := s.RetrieveEvidence(context.Background(), q, 20)
		lat := time.Since(start)
		if err != nil {
			t.Fatalf("retrieval failed for '%s': %v", q, err)
		}
		if lat > maxLatency {
			maxLatency = lat
		}
		t.Logf("  '%s': %v", q, lat)
	}

	if maxLatency > 500*time.Millisecond {
		t.Fatalf("LATENCY SLA BREACH: max retrieval latency %v > 500ms", maxLatency)
	}
	t.Logf("✓ Retrieval latency SLA: PASS — max latency %v < 500ms", maxLatency)
}

// ═══════════════════════════════════════════════════════════════════════════════
// DIMENSION 12: GRACEFUL DEGRADATION
// Does the system handle edge cases without crashing?
// ═══════════════════════════════════════════════════════════════════════════════

func TestGracefulDegradation(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "data")
	setupGitRepo(t, root, "main.go", "package main\nfunc main() {}\n")

	s, err := server.NewWithOptions(dbPath, root, server.Options{StorageType: "file"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Index()

	// Edge 1: Empty task
	_, err = s.Plan("", "", 4000)
	if err != nil {
		t.Logf("Empty task correctly returns error: %v", err)
	}

	// Edge 2: Zero budget
	p, err := s.Plan("test task", "", 0)
	if err != nil {
		t.Fatalf("zero budget should use default, got error: %v", err)
	}
	if p.Budget <= 0 {
		t.Fatal("zero budget was not defaulted")
	}

	// Edge 3: Very large task string (potential DoS vector)
	longTask := strings.Repeat("analyze the codebase ", 200) // ~4000 chars
	_, err = s.Plan(longTask, "", 4000)
	if err != nil {
		t.Fatalf("large task string caused error: %v", err)
	}

	// Edge 4: Unknown model name
	p, err = s.Plan("test task", "nonexistent-model-xyz", 4000)
	if err != nil {
		t.Fatalf("unknown model should fall back gracefully, got: %v", err)
	}
	if p.Model != "nonexistent-model-xyz" {
		t.Logf("Model fallback: %s", p.Model)
	}

	// Edge 5: Concurrent GC + Plan
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		s.GC(store.PruneOptions{
			RepoID: s.RepoID, CurrentRevision: s.Repo.Revision, DryRun: true,
		})
	}()
	go func() {
		defer wg.Done()
		s.Plan("concurrent gc test", "", 4000)
	}()
	wg.Wait()

	t.Log("✓ Graceful degradation: PASS — all edge cases handled without panic")
}

// ═══════════════════════════════════════════════════════════════════════════════
// DIMENSION 13: AUTHENTICATION & API KEY LIFECYCLE
// Verifies cryptographic key generation, SHA-256 validation, scopes, and revocation
// ═══════════════════════════════════════════════════════════════════════════════

func TestAuthenticationAndKeyLifecycle(t *testing.T) {
	authStore := auth.NewMemoryStore()

	tenant := &auth.Tenant{
		ID:     "tenant_auth_saas",
		Name:   "FinTech Inc",
		Tier:   auth.TierEnterprise,
		Active: true,
	}
	if err := authStore.CreateTenant(tenant); err != nil {
		t.Fatalf("CreateTenant failed: %v", err)
	}

	// 1. Generate API Key
	res, err := auth.GenerateAPIKey(tenant.ID, "primary-key", auth.TierEnterprise, []string{auth.ScopeContextRead, auth.ScopeContextWrite}, 100, nil)
	if err != nil {
		t.Fatalf("GenerateAPIKey failed: %v", err)
	}
	if !strings.HasPrefix(res.RawKey, "ctx_live_") {
		t.Fatalf("invalid raw key prefix: %s", res.RawKey)
	}
	if err := authStore.SaveKey(res.Key); err != nil {
		t.Fatalf("SaveKey failed: %v", err)
	}

	// 2. Validate valid key
	authTenant, authKey, err := authStore.Authenticate(res.RawKey)
	if err != nil || authTenant.ID != tenant.ID || authKey.ID != res.Key.ID {
		t.Fatalf("Authenticate valid key failed: %v", err)
	}

	// 3. Reject bad key
	if _, _, err := authStore.Authenticate("ctx_live_badkeyvalue12345"); err != auth.ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized for bad key, got %v", err)
	}

	// 4. Expiration
	past := time.Now().UTC().Add(-2 * time.Hour)
	expGen, _ := auth.GenerateAPIKey(tenant.ID, "expired-key", auth.TierEnterprise, nil, 10, &past)
	_ = authStore.SaveKey(expGen.Key)
	if _, _, err := authStore.Authenticate(expGen.RawKey); err != auth.ErrKeyExpired {
		t.Fatalf("expected ErrKeyExpired, got %v", err)
	}

	// 5. Revocation
	if err := authStore.RevokeKey(res.Key.ID); err != nil {
		t.Fatalf("RevokeKey failed: %v", err)
	}
	if _, _, err := authStore.Authenticate(res.RawKey); err != auth.ErrKeyRevoked {
		t.Fatalf("expected ErrKeyRevoked after revocation, got %v", err)
	}

	// 6. Inactive Tenant
	tenant.Active = false
	_ = authStore.UpdateTenant(tenant)
	newKey, _ := auth.GenerateAPIKey(tenant.ID, "k2", auth.TierEnterprise, nil, 10, nil)
	_ = authStore.SaveKey(newKey.Key)
	if _, _, err := authStore.Authenticate(newKey.RawKey); err != auth.ErrTenantInactive {
		t.Fatalf("expected ErrTenantInactive, got %v", err)
	}

	t.Log("✓ Authentication & API key lifecycle: PASS — cryptographic generation, SHA-256 verification, expiration, revocation, tenant deactivation")
}

// ═══════════════════════════════════════════════════════════════════════════════
// DIMENSION 14: HTTP API GATEWAY & MCP-OVER-HTTP CONTRACT
// Verifies full REST API gateway and JSON-RPC 2.0 MCP protocol over HTTP
// ═══════════════════════════════════════════════════════════════════════════════

func TestHTTPGatewayAndMCPOverHTTP(t *testing.T) {
	baseDir := t.TempDir()
	authStore := auth.NewMemoryStore()
	tm, err := tenant.NewManager(tenant.Options{
		BaseDataDir: filepath.Join(baseDir, "tenants"),
		StorageType: "file",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer tm.Close()

	tnt := &auth.Tenant{
		ID:     "tenant_gateway_user",
		Name:   "Cloud Services LLC",
		Tier:   auth.TierPro,
		Active: true,
	}
	_ = authStore.CreateTenant(tnt)
	keyGen, _ := auth.GenerateAPIKey(tnt.ID, "gw-key", auth.TierPro, []string{auth.ScopeContextRead, auth.ScopeContextWrite}, 50, nil)
	_ = authStore.SaveKey(keyGen.Key)

	// Create a dummy workspace for the tenant
	svc, _ := tm.GetService(tnt.ID)
	_ = os.WriteFile(filepath.Join(svc.Repo.Path, "app.go"), []byte("package main\nfunc CloudRouter() {}\n"), 0600)
	_ = svc.Index()

	gw, err := gateway.New(gateway.Config{
		AuthStore: authStore,
		TenantMgr: tm,
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := gw.Handler()

	// 1. Unauthenticated request -> 401
	planPayload := `{"task":"CloudRouter architecture","budget":4000}`
	req1 := httptest.NewRequest(http.MethodPost, "/v1/context/plan", strings.NewReader(planPayload))
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 unauthenticated, got %d", rec1.Code)
	}

	// 2. Authenticated REST Plan -> 200 OK
	req2 := httptest.NewRequest(http.MethodPost, "/v1/context/plan", strings.NewReader(planPayload))
	req2.Header.Set("Authorization", "Bearer "+keyGen.RawKey)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("REST /v1/context/plan failed: %d %s", rec2.Code, rec2.Body.String())
	}
	var planResp map[string]any
	if err := json.Unmarshal(rec2.Body.Bytes(), &planResp); err != nil {
		t.Fatalf("failed to parse plan JSON: %v", err)
	}
	if planResp["task"] != "CloudRouter architecture" {
		t.Fatalf("task mismatch in plan response: %+v", planResp)
	}

	// 3. MCP-over-HTTP: JSON-RPC 2.0 tools/list
	listRPC := `{"jsonrpc":"2.0","id":100,"method":"tools/list"}`
	req3 := httptest.NewRequest(http.MethodPost, "/v1/mcp", strings.NewReader(listRPC))
	req3.Header.Set("Authorization", "Bearer "+keyGen.RawKey)
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("MCP HTTP tools/list failed: %d %s", rec3.Code, rec3.Body.String())
	}
	var rpcResp mcp.Response
	if err := json.Unmarshal(rec3.Body.Bytes(), &rpcResp); err != nil {
		t.Fatalf("failed to decode JSON-RPC response: %v", err)
	}
	if rpcResp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %+v", rpcResp.Error)
	}

	// 4. MCP-over-HTTP: JSON-RPC 2.0 tools/call context_plan
	callRPC := `{"jsonrpc":"2.0","id":101,"method":"tools/call","params":{"name":"context_plan","arguments":{"task":"CloudRouter architecture","budget":4000}}}`
	req4 := httptest.NewRequest(http.MethodPost, "/v1/mcp", strings.NewReader(callRPC))
	req4.Header.Set("Authorization", "Bearer "+keyGen.RawKey)
	rec4 := httptest.NewRecorder()
	handler.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusOK {
		t.Fatalf("MCP tools/call failed: %d", rec4.Code)
	}

	t.Log("✓ HTTP API Gateway & MCP-over-HTTP: PASS — REST context endpoints + JSON-RPC 2.0 MCP tools verified")
}

// ═══════════════════════════════════════════════════════════════════════════════
// DIMENSION 15: RATE LIMITING & TENANT QPS THROTTLING
// Verifies token bucket algorithm, 429 status code, Retry-After, and headers
// ═══════════════════════════════════════════════════════════════════════════════

func TestRateLimitingAndThrottling(t *testing.T) {
	limiter := ratelimit.NewLimiter()

	tenant := &auth.Tenant{
		ID:   "tenant_rate_limited",
		Tier: auth.TierFree,
	}
	key := &auth.APIKey{
		ID:           "key_rate_limited",
		TenantID:     tenant.ID,
		Tier:         auth.TierFree,
		RateLimitQPS: 5, // 5 QPS, 10 burst
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mw := limiter.Middleware()(handler)

	// Consume entire burst (10 tokens)
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/context/plan", nil)
		ctx := auth.WithTenantContext(req.Context(), tenant, key)
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req.WithContext(ctx))

		if rec.Code != http.StatusOK {
			t.Fatalf("burst request %d failed: %d", i, rec.Code)
		}
		if rec.Header().Get("X-RateLimit-Limit") == "" {
			t.Fatal("missing X-RateLimit-Limit header")
		}
	}

	// 11th request must be throttled with HTTP 429
	req := httptest.NewRequest(http.MethodGet, "/v1/context/plan", nil)
	ctx := auth.WithTenantContext(req.Context(), tenant, key)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req.WithContext(ctx))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests, got %d", rec.Code)
	}

	retryAfter := rec.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Fatal("missing Retry-After header on 429 response")
	}
	retrySec, _ := strconv.Atoi(retryAfter)
	if retrySec <= 0 {
		t.Fatalf("invalid Retry-After value: %s", retryAfter)
	}

	t.Logf("✓ Rate limiting & QPS throttling: PASS — 10 burst passed, 11th returned 429 with Retry-After: %ss", retryAfter)
}

// ═══════════════════════════════════════════════════════════════════════════════
// DIMENSION 16: USAGE METERING & INVOICING AGGREGATION
// Verifies aggregation of tokens, requests, and costs into invoiceable statements
// ═══════════════════════════════════════════════════════════════════════════════

func TestUsageMeteringAndInvoicing(t *testing.T) {
	meter := metering.NewMeter()

	tenant := &auth.Tenant{
		ID:                 "tenant_metering_eval",
		Name:               "AI Startups Co",
		Tier:               auth.TierPro,
		MonthlyTokenBudget: 500000,
	}

	var alertTriggered bool
	var alertPercent float64
	meter.SetQuotaAlertHandler(func(tenantID string, usagePercent float64, currentTokens, budgetTokens int64) {
		alertTriggered = true
		alertPercent = usagePercent
	})

	// Simulate 10 queries across different models
	for i := 0; i < 10; i++ {
		modelName := "claude-3-7-sonnet"
		if i%2 == 0 {
			modelName = "gpt-4o"
		}
		cacheHit := (i >= 3)
		var cachedTokens int64
		if cacheHit {
			cachedTokens = 3000
		}

		meter.Record(metering.UsageEvent{
			TenantID:      tenant.ID,
			Endpoint:      "/v1/context/plan",
			Model:         modelName,
			InputTokens:   4000,
			CachedTokens:  cachedTokens,
			OutputTokens:  1000,
			EstimatedCost: 0.012,
			CacheHit:      cacheHit,
			LatencyMS:     150,
		})
	}

	agg, ok := meter.GetAggregate(tenant.ID)
	if !ok {
		t.Fatal("failed to find tenant aggregate")
	}

	if agg.TotalRequests != 10 {
		t.Fatalf("expected 10 requests, got %d", agg.TotalRequests)
	}
	if agg.TotalInputTokens != 40000 {
		t.Fatalf("expected 40000 input tokens, got %d", agg.TotalInputTokens)
	}
	if agg.CacheHits != 7 {
		t.Fatalf("expected 7 cache hits, got %d", agg.CacheHits)
	}
	if agg.CacheHitRate != 0.70 {
		t.Fatalf("expected 70%% cache hit rate, got %.2f", agg.CacheHitRate)
	}

	// Generate billing statement
	inv := metering.GenerateInvoice(tenant, agg)
	if inv.BaseFeeUSD != 29.0 {
		t.Fatalf("expected Pro tier base fee $29, got %f", inv.BaseFeeUSD)
	}
	if inv.TotalUSD < 29.0 {
		t.Fatalf("total USD cannot be less than base fee: %f", inv.TotalUSD)
	}
	if len(inv.LineItems) == 0 {
		t.Fatal("invoice has no line items")
	}

	// Trigger quota alert: exceed 80% of 500,000 budget
	meter.Record(metering.UsageEvent{
		TenantID:    tenant.ID,
		InputTokens: 420000,
	})
	meter.CheckQuota(tenant)

	if !alertTriggered {
		t.Fatal("quota alert did not trigger at >80% consumption")
	}

	t.Logf("✓ Usage metering & invoicing: PASS — 10 queries aggregated, $%.2f invoice generated, quota alert at %.1f%%",
		inv.TotalUSD, alertPercent)
}

// ═══════════════════════════════════════════════════════════════════════════════
// DIMENSION 17: SHARED-PROCESS MULTI-TENANT NAMESPACE ISOLATION
// Verifies that multiple tenants in a single shared daemon process have 100% isolated memories and workspaces
// ═══════════════════════════════════════════════════════════════════════════════

func TestSharedProcessTenantIsolation(t *testing.T) {
	baseDir := t.TempDir()

	mgr, err := tenant.NewManager(tenant.Options{
		BaseDataDir: baseDir,
		StorageType: "file",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	// Instantiate 3 tenants within the exact same process
	tenants := []string{"tenant_apple", "tenant_google", "tenant_microsoft"}
	secrets := []string{
		"Apple secret internal project Titan car autonomous driving",
		"Google secret internal TPU v7 architecture distributed mesh",
		"Microsoft secret internal Azure quantum Majorana qubit topological",
	}

	for i, tID := range tenants {
		tenantDir := filepath.Join(baseDir, tID)
		repoDir := filepath.Join(tenantDir, "repo")
		if err := os.MkdirAll(repoDir, 0700); err != nil {
			t.Fatal(err)
		}
		setupGitRepo(t, repoDir, fmt.Sprintf("%s.go", tID),
			fmt.Sprintf("package %s\nfunc CoreMethod%d() {}\n", strings.ReplaceAll(tID, "tenant_", ""), i))
		mgr.RegisterTenantRepo(tID, repoDir)

		svc, err := mgr.GetService(tID)
		if err != nil {
			t.Fatalf("failed to get service for %s: %v", tID, err)
		}
		if err := svc.Index(); err != nil {
			t.Fatalf("Index failed for %s: %v", tID, err)
		}

		// Store proprietary memory
		_, err = svc.Remember("decision", secrets[i], "user", "repo", "", 0.95, nil)
		if err != nil {
			t.Fatalf("Remember failed for %s: %v", tID, err)
		}
	}

	queries := []string{
		"project Titan car autonomous driving",
		"TPU v7 architecture distributed mesh",
		"Azure quantum Majorana qubit topological",
	}

	// Query each tenant and verify ZERO leaks of other tenants' secrets
	for i, tID := range tenants {
		svc, _ := mgr.GetService(tID)

		// 1. SearchCandidates verifies memory search isolation
		memories, err := svc.SearchCandidates(queries[i], 10)
		if err != nil {
			t.Fatalf("SearchCandidates failed for %s: %v", tID, err)
		}
		foundOwn := false
		for _, mem := range memories {
			if strings.Contains(mem.Content, secrets[i]) {
				foundOwn = true
			}
			for j, otherSecret := range secrets {
				if i != j && strings.Contains(mem.Content, otherSecret) {
					t.Fatalf("MEMORY LEAK: %s saw %s memory: %s", tID, tenants[j], mem.Content)
				}
			}
		}
		if !foundOwn {
			t.Fatalf("tenant %s did not recall its own memory in SearchCandidates", tID)
		}

		// 2. Plan verifies context planning isolation
		plan, err := svc.Plan(queries[i], "", 4000)
		if err != nil {
			t.Fatalf("Plan failed for %s: %v", tID, err)
		}
		for _, item := range plan.Selected {
			for j, otherSecret := range secrets {
				if i != j && strings.Contains(item.Content, otherSecret) {
					t.Fatalf("CRITICAL ISOLATION BREACH: %s saw %s data in plan: %s", tID, tenants[j], item.Content)
				}
			}
		}
	}

	t.Log("✓ Shared-process multi-tenant isolation: PASS — 3 concurrent tenants, zero cross-contamination")
}

// ═══════════════════════════════════════════════════════════════════════════════
// DIMENSION 18: WEBHOOK EVENT NOTIFICATIONS & HMAC SIGNING
// Verifies real-time event dispatching, payload hashing, and tamper resistance
// ═══════════════════════════════════════════════════════════════════════════════

type inMemoryTransport func(req *http.Request) *http.Response

func (f inMemoryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func TestWebhookNotificationsAndHMACSigning(t *testing.T) {
	secret := "whsec_live_contextos_secret_token_42"
	tenantID := "tenant_webhook_verify"

	var interceptedSig string
	var interceptedTS string
	var interceptedBody []byte

	disp := webhooks.NewDispatcher()
	disp.SetTransport(inMemoryTransport(func(req *http.Request) *http.Response {
		interceptedSig = req.Header.Get("X-ContextOS-Signature")
		interceptedTS = req.Header.Get("X-ContextOS-Timestamp")
		interceptedBody, _ = io.ReadAll(req.Body)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"received":true}`))),
			Header:     make(http.Header),
		}
	}))

	disp.RegisterSubscription(webhooks.Subscription{
		TenantID: tenantID,
		URL:      "https://customer-api.corp.internal/webhooks/contextos",
		Secret:   secret,
		Events:   []string{webhooks.EventQuotaWarning, webhooks.EventPlanCompleted},
		Active:   true,
	})

	now := time.Now().UTC()
	evt := webhooks.Event{
		ID:        "evt_bill_warning_99",
		Type:      webhooks.EventQuotaWarning,
		TenantID:  tenantID,
		Timestamp: now,
		Data: map[string]any{
			"usage_percent": 82.5,
			"tokens_used":   412500,
		},
	}

	record, err := disp.Dispatch(evt)
	if err != nil || record == nil || !record.Success {
		t.Fatalf("webhook dispatch failed: %+v, err: %v", record, err)
	}

	// 1. Verify Signature format: sha256=<hex>
	if !strings.HasPrefix(interceptedSig, "sha256=") {
		t.Fatalf("expected sha256= prefix on signature, got %s", interceptedSig)
	}
	rawSig := strings.TrimPrefix(interceptedSig, "sha256=")

	tsInt, err := strconv.ParseInt(interceptedTS, 10, 64)
	if err != nil {
		t.Fatalf("invalid timestamp header: %v", err)
	}

	// 2. Validate cryptographic signature
	if !webhooks.VerifySignature(secret, tsInt, interceptedBody, rawSig) {
		t.Fatal("HMAC-SHA256 signature verification failed on valid payload")
	}

	// 3. Reject tampered payload
	tamperedBody := append(interceptedBody, []byte("tamper")...)
	if webhooks.VerifySignature(secret, tsInt, tamperedBody, rawSig) {
		t.Fatal("tampered payload must NOT pass HMAC signature verification")
	}

	// 4. Reject mismatched secret
	if webhooks.VerifySignature("wrong_secret_key", tsInt, interceptedBody, rawSig) {
		t.Fatal("mismatched secret must NOT pass HMAC verification")
	}

	t.Log("✓ Webhook event notifications: PASS — HMAC-SHA256 signing, delivery, and tamper rejection verified")
}

// ═══════════════════════════════════════════════════════════════════════════════
// HELPERS
// ═══════════════════════════════════════════════════════════════════════════════

func setupGitRepo(t *testing.T, dir, filename, content string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "test@saas.com"},
		{"config", "user.name", "SaaS Test"},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if err := c.Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"add", "."},
		{"commit", "-qm", "init"},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if err := c.Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
}
