package retrieval

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"contextos/internal/gitidx"
	"contextos/internal/retrieval/graph"
	"contextos/internal/retrieval/index"
	"contextos/internal/store"
)

func TestInferScopeAndLocality(t *testing.T) {
	ctx := LocalizerContext{
		Cwd:            "internal/retrieval",
		RepoRoot:       ".",
		ChangedFiles:   []string{"internal/retrieval/planner.go"},
		RecentFiles:    []string{"internal/retrieval/unit.go"},
		CurrentPackage: "retrieval",
	}

	scope := InferScope("Inspect SubsystemPlanner in internal/retrieval/planner.go", ctx)

	if scope.Confidence < 0.50 {
		t.Errorf("expected high confidence scope, got %f", scope.Confidence)
	}

	hierarchy := BuildLocalityHierarchy(scope, ctx)
	if !hierarchy.HOT.Matches("internal/retrieval/planner.go", "retrieval") {
		t.Errorf("expected planner.go to match HOT locality tier")
	}
	if !hierarchy.COLD.Matches("any/path/file.go", "other") {
		t.Errorf("expected COLD tier to match any file")
	}
}

func TestAdaptiveKAndStopping(t *testing.T) {
	// Sharp distribution: candidate 0 dominates
	sharpScores := []float64{10.0, 1.0, 0.5, 0.2, 0.1}
	kSharp := DetermineAdaptiveK(sharpScores, 5, 20, 1.0)
	if kSharp > 10 {
		t.Errorf("expected small K for sharp distribution, got %d", kSharp)
	}

	// Uniform/uncertain distribution
	flatScores := []float64{5.0, 4.9, 4.8, 4.7, 4.6}
	kFlat := DetermineAdaptiveK(flatScores, 5, 20, 1.0)
	if kFlat < kSharp {
		t.Errorf("expected kFlat (%d) >= kSharp (%d)", kFlat, kSharp)
	}

	// Adaptive stopping
	candidates := []Candidate{
		{NodeID: "c1", Score: 0.95},
		{NodeID: "c2", Score: 0.90},
		{NodeID: "c3", Score: 0.85},
		{NodeID: "c4", Score: 0.84}, // marginal gain 0.01 < epsilon 0.05
		{NodeID: "c5", Score: 0.30},
	}
	stopIdx := AdaptiveStopIndex(candidates, 2, 0.80, 0.05)
	if stopIdx != 3 {
		t.Errorf("expected stop index 3, got %d", stopIdx)
	}
}

func TestDecisionAwareOptimizer(t *testing.T) {
	units := []ContextUnit{
		{ID: "u1", Tokens: 100, DecisionValue: 0.90, Uncertainty: 0.10, CacheReuse: 0.80},
		{ID: "u2", Tokens: 200, DecisionValue: 0.80, Uncertainty: 0.20, CacheReuse: 0.60},
		{ID: "u3", Tokens: 1500, DecisionValue: 0.40, Uncertainty: 0.50, CacheReuse: 0.10},
		{ID: "u4", Tokens: 3000, DecisionValue: 0.20, Uncertainty: 0.70, CacheReuse: 0.00},
	}

	cfg := OptimizerConfig{
		BudgetTokens:   500,
		TargetAccuracy: 0.85,
		LambdaTokens:   0.0001,
		LambdaRisk:     0.20,
		LambdaUtility:  1.00,
	}

	res := OptimizeContext(units, cfg)

	if res.TotalTokens > cfg.BudgetTokens {
		t.Errorf("selected tokens %d exceeded budget %d", res.TotalTokens, cfg.BudgetTokens)
	}
	if len(res.SelectedUnits) == 0 {
		t.Fatalf("expected at least one unit selected")
	}
	if res.EstimatedCorrectness < 0.80 {
		t.Errorf("expected correctness >= 0.80, got %f", res.EstimatedCorrectness)
	}
	if res.PrunedTokens <= 0 {
		t.Errorf("expected positive pruned tokens, got %d", res.PrunedTokens)
	}
}

func TestDependencyAwareCacheInvalidation(t *testing.T) {
	cache := NewContextCache(10)

	unitsA := []ContextUnit{
		{ID: "uA1", Path: "pkg/auth/login.go", Tokens: 50},
		{ID: "uA2", Path: "pkg/auth/token.go", Tokens: 60},
	}
	unitsB := []ContextUnit{
		{ID: "uB1", Path: "pkg/db/store.go", Tokens: 80},
	}

	cache.Put("query_auth", unitsA)
	cache.Put("query_db", unitsB)

	// Invalidate modified file in auth
	invalidated := cache.InvalidateModifiedFiles([]string{"pkg/auth/login.go"})
	if invalidated != 1 {
		t.Errorf("expected 1 invalidated entry, got %d", invalidated)
	}

	// Auth entry should be gone
	if _, found := cache.Get("query_auth"); found {
		t.Errorf("query_auth should have been invalidated")
	}

	// DB entry should still be intact
	if _, found := cache.Get("query_db"); !found {
		t.Errorf("query_db should still exist in cache")
	}
}

func TestSubsystemPlannerEndToEnd(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "planner_test.db")
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create sqlite store: %v", err)
	}

	repoID, err := st.GetOrCreateRepo("/test/repo", "testrepo", "rev1", "main", "wt1")
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	var syms []gitidx.Symbol
	var files []gitidx.SourceFile

	packages := []string{"auth", "api", "db", "server"}
	for i := 0; i < 50; i++ {
		pkg := packages[i%len(packages)]
		path := fmt.Sprintf("pkg/%s/service_%d.go", pkg, i)
		files = append(files, gitidx.SourceFile{
			Path:  path,
			Hash:  fmt.Sprintf("hash_%d", i),
			Lines: 40,
		})
		syms = append(syms, gitidx.Symbol{
			Path:      path,
			Name:      fmt.Sprintf("HandleAuthToken%d", i),
			Kind:      "function",
			Start:     10,
			End:       30,
			Signature: fmt.Sprintf("func HandleAuthToken%d() error", i),
		})
	}

	if err := st.SaveNodesAndEdges(repoID, files, syms, nil); err != nil {
		t.Fatalf("failed to save test nodes: %v", err)
	}

	g := graph.NewPersistentGraph()
	sr := index.NewShardRouter(2)
	cache := NewContextCache(50)

	planner := NewSubsystemPlanner(st, g, sr, cache)

	q := Query{
		Task:       "HandleAuthToken0 in auth package",
		RepoID:     repoID,
		MaxResults: 20,
	}
	lctx := LocalizerContext{
		CurrentPackage: "auth",
		RepoRoot:       ".",
	}

	plan, trace, err := planner.ExecutePlan(context.Background(), q, lctx)
	if err != nil {
		t.Fatalf("ExecutePlan failed: %v", err)
	}

	if plan == nil {
		t.Fatalf("expected non-nil plan")
	}
	if len(plan.SelectedUnits) == 0 {
		t.Errorf("expected selected context units")
	}
	if plan.TotalTokens <= 0 {
		t.Errorf("expected total tokens > 0")
	}
	if trace.LatencyTotal <= 0 {
		t.Errorf("expected populated trace with latency")
	}
}
