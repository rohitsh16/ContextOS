package compute_bench

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"contextos/internal/compute"
	"contextos/internal/server"
	"contextos/internal/store"
)

func TestMonorepoStress(t *testing.T) {
	dbDir, err := os.MkdirTemp("", "ctx_monorepo_stress_*")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	defer os.RemoveAll(dbDir)

	dbPath := filepath.Join(dbDir, "monorepo_stress.db")
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	defer st.Close()

	// Generate a 5,000-node scale-free monorepo with 25,000 power-law edges
	nodeCount := 5000
	edgeCount := 25000
	t0 := time.Now()
	repoID, queries, err := GenerateScaleFreeMonorepo(st, dbDir, nodeCount, edgeCount, 42)
	if err != nil {
		t.Fatalf("generate monorepo: %v", err)
	}
	t.Logf("Generated monorepo (%d nodes, %d edges) in %v", nodeCount, edgeCount, time.Since(t0))

	svc, err := server.New(dbPath, dbDir)
	if err != nil {
		t.Fatalf("new server service: %v", err)
	}
	defer svc.Close()
	svc.RepoID = repoID

	cfg := MonorepoStressConfig{
		NodeCount:    nodeCount,
		EdgeCount:    edgeCount,
		Workers:      8,
		TotalQueries: 60,
		TimeoutSLA:   5 * time.Second, // Claude Code 5s SLA
		Seed:         42,
	}

	result, err := RunMonorepoStress(svc, queries, cfg)
	if err != nil {
		t.Fatalf("run monorepo stress: %v", err)
	}

	t.Logf("=== Monorepo Stress Test Results ===")
	t.Logf("Total Queries: %d across %d workers", result.TotalQueries, result.Workers)
	t.Logf("Throughput: %.2f QPS", result.ThroughputQPS)
	t.Logf("p50 Latency: %v", result.P50Latency)
	t.Logf("p90 Latency: %v", result.P90Latency)
	t.Logf("p95 Latency: %v", result.P95Latency)
	t.Logf("p99 Latency: %v", result.P99Latency)
	t.Logf("Max Latency: %v", result.MaxLatency)
	t.Logf("Timeouts (> 5s SLA): %d", result.TimeoutCount)
	t.Logf("Warnings (> 100ms): %d", result.WarningCount)
	t.Logf("Errors: %d", result.ErrorCount)

	for class, p95 := range result.ClassLatencies {
		t.Logf("  [%s] p95: %v", class, p95)
	}

	snap := compute.GlobalProfiler().Snapshot()
	t.Logf("Profiler Overall: mean=%v p50=%v p95=%v p99=%v", snap.Overall.Mean, snap.Overall.P50, snap.Overall.P95, snap.Overall.P99)
	for stage, p := range snap.Stages {
		t.Logf("  Stage %-22s: p50=%-12v p95=%-12v p99=%-12v", stage, p.P50, p.P95, p.P99)
	}

	if result.TimeoutCount > 0 {
		t.Errorf("expected 0 timeouts, got %d", result.TimeoutCount)
	}
	if result.ErrorCount > 0 {
		t.Errorf("expected 0 errors, got %d", result.ErrorCount)
	}
	if result.P95Latency > 150*time.Millisecond {
		t.Errorf("p95 latency %v exceeded 150ms SLA target", result.P95Latency)
	}
}
