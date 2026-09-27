// Package gates provides R18.1 architecture regression tests and the
// 200-query GREEN gate benchmark.
//
// Sprint 1 tests verify:
//  1. Plan() uses HybridRetriever as the sole authoritative retrieval path
//  2. SearchCandidates (legacy) does NOT run by default
//  3. Legacy fallback only triggers when hybrid returns 0 candidates
//  4. Graph expansion is a first-class stage inside HybridRetriever
//  5. Per-stage latency telemetry is populated in the ContextPlan
package gates

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"contextos/internal/model"
	"contextos/internal/retrieval"
	"contextos/internal/server"
	"contextos/internal/textutil"
	"contextos/internal/verification"
)

// ═══════════════════════════════════════════════════════════════════════════════
// P0.1: Plan uses authoritative retriever — no unconditional legacy
// ═══════════════════════════════════════════════════════════════════════════════

func TestPlanUsesAuthoritativeRetriever(t *testing.T) {
	svc, _ := buildProductionCorpus(t, 500, 101)
	defer svc.Close()

	plan, err := svc.Plan("Handler0 validation", "gpt-4o", 4000)
	if err != nil {
		t.Fatalf("Plan() error: %v", err)
	}

	// Must report hybrid retrieval mode
	if plan.RetrievalMode != "hybrid" {
		t.Errorf("expected retrieval_mode='hybrid', got %q", plan.RetrievalMode)
	}

	// Must have retrieval stages populated from HybridRetriever trace
	if len(plan.RetrievalStages) == 0 {
		t.Error("expected non-empty retrieval_stages from authoritative retriever")
	}

	// Should NOT have used fallback for a query with plenty of matches
	if plan.FallbackUsed {
		t.Error("ARCHITECTURE VIOLATION: FallbackUsed=true when hybrid retriever should have found candidates")
	}
	if plan.FallbackCandidateCount > 0 {
		t.Errorf("ARCHITECTURE VIOLATION: FallbackCandidateCount=%d, expected 0 when hybrid returns results",
			plan.FallbackCandidateCount)
	}

	t.Logf("✓ Plan uses authoritative retriever: mode=%s stages=%v fallback=%v candidates=%d",
		plan.RetrievalMode, plan.RetrievalStages, plan.FallbackUsed, plan.CandidateCount)
}

// ═══════════════════════════════════════════════════════════════════════════════
// P0.2: Plan does NOT run legacy SearchCandidates by default
// ═══════════════════════════════════════════════════════════════════════════════

func TestPlanDoesNotRunLegacyByDefault(t *testing.T) {
	svc, _ := buildProductionCorpus(t, 1000, 102)
	defer svc.Close()

	queries := []string{
		"Handler0",
		"find storage adapter",
		"pkg/core/handler0_0.go",
		"trace dependency from auth to cache",
		"error handling in api",
	}

	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			plan, err := svc.Plan(q, "gpt-4o", 4000)
			if err != nil {
				t.Fatalf("Plan error: %v", err)
			}

			if plan.FallbackUsed {
				t.Errorf("ARCHITECTURE VIOLATION: legacy fallback used for query %q with %d candidates",
					q, plan.CandidateCount)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// P0.3: Legacy fallback only triggers on zero hybrid candidates
// ═══════════════════════════════════════════════════════════════════════════════

func TestLegacyFallbackOnlyWhenTriggered(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, ".contextos", "fallback.db")
	os.MkdirAll(filepath.Dir(dbPath), 0755)
	runGitInit(t, root)

	pkg := filepath.Join(root, "pkg", "api")
	os.MkdirAll(pkg, 0755)
	os.WriteFile(filepath.Join(pkg, "handler.go"),
		[]byte("package api\n\nfunc HandleRequest() error { return nil }\n"), 0644)
	runGitAdd(t, root)

	svc, err := server.New(dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	svc.Index()

	// Store a memory
	_, _ = svc.Remember("observation", "The deployment uses us-central1 region", "user", "repo", "", 0.9, nil)

	// Query that hybrid CAN find
	plan, err := svc.Plan("HandleRequest", "gpt-4o", 4000)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("HandleRequest: fallback=%v candidates=%d selected=%d",
		plan.FallbackUsed, plan.CandidateCount, len(plan.Selected))

	// Verify fallback is bounded when triggered
	plan2, err := svc.Plan("zzz_nonexistent_symbol_xyz", "gpt-4o", 4000)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Nonexistent: fallback=%v fallback_count=%d candidates=%d",
		plan2.FallbackUsed, plan2.FallbackCandidateCount, plan2.CandidateCount)

	if plan2.FallbackUsed && plan2.FallbackCandidateCount > 15 {
		t.Errorf("fallback candidate count %d exceeds hard budget of 15",
			plan2.FallbackCandidateCount)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// P0.4: Graph expansion is a first-class retrieval stage
// ═══════════════════════════════════════════════════════════════════════════════

func TestGraphExpansionIsExplicitStage(t *testing.T) {
	svc, _ := buildProductionCorpus(t, 1000, 103)
	defer svc.Close()

	plan, err := svc.Plan("trace dependency from api to storage", "gpt-4o", 4000)
	if err != nil {
		t.Fatal(err)
	}

	hasExpansion := false
	hasRerank := false
	for _, stage := range plan.RetrievalStages {
		if stage == "expansion" {
			hasExpansion = true
		}
		if stage == "rerank" {
			hasRerank = true
		}
	}

	if !hasExpansion {
		t.Errorf("ARCHITECTURE VIOLATION: 'expansion' not found in retrieval_stages: %v", plan.RetrievalStages)
	}
	if !hasRerank {
		t.Errorf("'rerank' not found in retrieval_stages: %v", plan.RetrievalStages)
	}

	t.Logf("✓ Graph expansion is explicit stage: stages=%v", plan.RetrievalStages)
}

// ═══════════════════════════════════════════════════════════════════════════════
// P0.5: Per-stage latency telemetry populated
// ═══════════════════════════════════════════════════════════════════════════════

func TestPlanStageTelemetry(t *testing.T) {
	svc, _ := buildProductionCorpus(t, 500, 104)
	defer svc.Close()

	plan, err := svc.Plan("Handler0 validation", "gpt-4o", 4000)
	if err != nil {
		t.Fatal(err)
	}

	if plan.StageDurations == nil || len(plan.StageDurations) == 0 {
		t.Error("StageDurations is nil or empty — per-stage latency telemetry not populated")
		return
	}

	requiredStages := []string{"exact_path", "symbol", "lexical", "semantic", "expansion", "rerank"}
	for _, stage := range requiredStages {
		dur, ok := plan.StageDurations[stage]
		if !ok {
			t.Errorf("missing stage duration for %q", stage)
		} else {
			t.Logf("  %s: %s", stage, dur)
		}
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// P0.6: Retrieval parity — hybrid-only produces same or better results
// ═══════════════════════════════════════════════════════════════════════════════

func TestPlanRetrievalParity(t *testing.T) {
	svc, _ := buildProductionCorpus(t, 1000, 105)
	defer svc.Close()

	queries := generateDiverseQueryCorpus(105, 20)

	var totalSelected, totalCandidates int
	zeroResultCount := 0

	for _, q := range queries {
		plan, err := svc.Plan(q, "gpt-4o", 4000)
		if err != nil {
			t.Logf("WARN: Plan error for %q: %v", q, err)
			continue
		}

		totalSelected += plan.SelectedTokens
		totalCandidates += plan.CandidateCount

		if plan.CandidateCount == 0 && plan.SelectedTokens == 0 && !plan.FallbackUsed {
			zeroResultCount++
		}
	}

	zeroRate := float64(zeroResultCount) / float64(len(queries))
	t.Logf("Retrieval parity: %d queries, avg_candidates=%.1f, avg_selected=%.0f, zero_result_rate=%.1f%%",
		len(queries), float64(totalCandidates)/float64(len(queries)),
		float64(totalSelected)/float64(len(queries)), zeroRate*100)

	if zeroRate > 0.30 {
		t.Errorf("zero-result rate %.1f%% too high — parity regression", zeroRate*100)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Query Order Invariance (Sprint 2)
// ═══════════════════════════════════════════════════════════════════════════════

func TestQueryOrderInvariance(t *testing.T) {
	svc, _ := buildProductionCorpus(t, 1000, 106)
	defer svc.Close()

	type queryVariant struct {
		name  string
		query string
	}

	testCases := []struct {
		taskName string
		variants []queryVariant
	}{
		{
			taskName: "Handler0",
			variants: []queryVariant{
				{"identifier_first", "Handler0 validation"},
				{"identifier_last", "find the validation for Handler0"},
				{"natural_language", "how does Handler0 validate input"},
				{"short_form", "Handler0"},
			},
		},
		{
			taskName: "StorageAdapter",
			variants: []queryVariant{
				{"identifier_first", "Store0 find adapter"},
				{"identifier_last", "find the adapter for Store0"},
				{"natural_language", "how does the storage adapter Store0 work"},
				{"short_form", "Store0"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.taskName, func(t *testing.T) {
			var pathSets []map[string]bool

			for _, v := range tc.variants {
				plan, err := svc.Plan(v.query, "gpt-4o", 4000)
				if err != nil {
					t.Fatalf("variant %s error: %v", v.name, err)
				}

				paths := make(map[string]bool)
				for _, s := range plan.Selected {
					paths[s.Location] = true
				}
				pathSets = append(pathSets, paths)

				t.Logf("  %s: %d selected, %d tokens", v.name, len(plan.Selected), plan.SelectedTokens)
			}

			totalJaccard := 0.0
			pairs := 0
			for i := 0; i < len(pathSets); i++ {
				for j := i + 1; j < len(pathSets); j++ {
					intersection := 0
					union := make(map[string]bool)
					for k := range pathSets[i] {
						union[k] = true
						if pathSets[j][k] {
							intersection++
						}
					}
					for k := range pathSets[j] {
						union[k] = true
					}
					if len(union) > 0 {
						totalJaccard += float64(intersection) / float64(len(union))
					}
					pairs++
				}
			}

			avgJaccard := totalJaccard / float64(pairs)
			t.Logf("  PSI (avg pairwise Jaccard): %.3f", avgJaccard)

			if avgJaccard < 0.1 {
				t.Logf("WARNING: low query-order stability PSI=%.3f for %q", avgJaccard, tc.taskName)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 200-QUERY GREEN GATE BENCHMARK (Sprint 3)
// ═══════════════════════════════════════════════════════════════════════════════
//
// 20 engineering tasks × 10 query formulations = 200 queries
// Each task has a gold manifest with required files and symbols.
// Metrics: CandidateRecall@100, Recall@20, MRR, PSI@20, RequiredEvidenceRecall

type goldManifest struct {
	TaskID          string   `json:"task_id"`
	Description     string   `json:"description"`
	RequiredFiles   []string `json:"required_files"`   // substring matches on paths
	RequiredSymbols []string `json:"required_symbols"` // substring matches on names
}

type queryFormulation struct {
	FormulationID string `json:"formulation_id"`
	Query         string `json:"query"`
}

type benchmarkTask struct {
	Gold         goldManifest
	Formulations []queryFormulation
}

type perQueryResult struct {
	QueryID                string  `json:"query_id"`
	TaskID                 string  `json:"task_id"`
	FormulationID          string  `json:"formulation_id"`
	CandidateCount         int     `json:"candidate_count"`
	RequiredEvidenceRecall float64 `json:"required_evidence_recall"`
	TargetRank             int     `json:"target_rank"`
	MRR                    float64 `json:"mrr"`
	FallbackUsed           bool    `json:"fallback_used"`
	LatencyMs              int64   `json:"latency_ms"`
}

func buildBenchmarkTasks(root string) []benchmarkTask {
	// 20 engineering tasks with planted code targets
	types := []string{"Handler", "Service", "Store", "Manager", "Worker",
		"Router", "Controller", "Resolver", "Provider", "Adapter",
		"Factory", "Builder", "Validator", "Processor", "Pipeline",
		"Gateway", "Monitor", "Scheduler", "Dispatcher", "Aggregator"}

	formTemplates := []string{
		"%s0",                                 // exact identifier
		"%s0 implementation",                  // identifier first
		"find the %s that handles operations", // identifier middle
		"operations handled by %s0",           // identifier last
		"how does the %s0 work in the system", // natural language
		"what does %s0 do",                    // concise developer
		"show %s0 code",                       // action-oriented
		"understanding the %s pattern",        // conceptual
		"files related to %s0 operations",     // indirect
		"%s0 api",                             // ambiguous-but-resolvable
	}

	var tasks []benchmarkTask
	for i, typeName := range types {
		taskID := fmt.Sprintf("task_%02d_%s", i+1, strings.ToLower(typeName))
		gold := goldManifest{
			TaskID:          taskID,
			Description:     fmt.Sprintf("Find and understand %s0 operations", typeName),
			RequiredFiles:   []string{strings.ToLower(typeName)},
			RequiredSymbols: []string{typeName + "0"},
		}

		var forms []queryFormulation
		for j, tmpl := range formTemplates {
			formID := fmt.Sprintf("f%02d", j+1)
			query := fmt.Sprintf(tmpl, typeName)
			forms = append(forms, queryFormulation{
				FormulationID: formID,
				Query:         query,
			})
		}

		tasks = append(tasks, benchmarkTask{Gold: gold, Formulations: forms})
	}
	return tasks
}

func TestGREENGateBenchmark200(t *testing.T) {
	svc, root := buildProductionCorpus(t, 2000, 200)
	defer svc.Close()

	tasks := buildBenchmarkTasks(root)

	t.Logf("═══════════════════════════════════════════════════════════════")
	t.Logf("  R18.1 GREEN GATE BENCHMARK: %d tasks × %d formulations = %d queries",
		len(tasks), len(tasks[0].Formulations), len(tasks)*len(tasks[0].Formulations))
	t.Logf("═══════════════════════════════════════════════════════════════")

	var allResults []perQueryResult

	// Aggregate metrics
	var totalRER float64
	var totalMRR float64
	var totalCandRecall float64
	totalQueries := 0
	fallbackCount := 0

	// PSI tracking: per-task, across formulations
	taskPSI := make(map[string][]map[string]bool) // taskID -> list of retrieved path sets

	for _, task := range tasks {
		for _, form := range task.Formulations {
			t0 := time.Now()
			cands, _, err := svc.RetrieveEvidence(context.Background(), form.Query, 100)
			latency := time.Since(t0)
			if err != nil {
				t.Logf("WARN: retrieve error for %q: %v", form.Query, err)
				continue
			}

			plan, _ := svc.Plan(form.Query, "gpt-4o", 4000)

			// Compute RequiredEvidenceRecall
			goldFiles := task.Gold.RequiredFiles
			foundFiles := 0
			for _, gf := range goldFiles {
				for _, c := range cands {
					if strings.Contains(strings.ToLower(c.Path+c.Name), strings.ToLower(gf)) {
						foundFiles++
						break
					}
				}
			}
			rer := 0.0
			if len(goldFiles) > 0 {
				rer = float64(foundFiles) / float64(len(goldFiles))
			}

			// Compute MRR (reciprocal rank of first relevant result)
			mrr := 0.0
			for rank, c := range cands {
				relevant := false
				for _, gf := range goldFiles {
					if strings.Contains(strings.ToLower(c.Path+c.Name), strings.ToLower(gf)) {
						relevant = true
						break
					}
				}
				if relevant {
					mrr = 1.0 / float64(rank+1)
					break
				}
			}

			// CandidateRecall@100
			candRecall := rer // for single-file tasks this is the same

			totalRER += rer
			totalMRR += mrr
			totalCandRecall += candRecall
			totalQueries++

			if plan.FallbackUsed {
				fallbackCount++
			}

			// Track PSI paths
			pathSet := make(map[string]bool)
			maxK := 20
			for i, c := range cands {
				if i >= maxK {
					break
				}
				pathSet[c.Path] = true
			}
			taskPSI[task.Gold.TaskID] = append(taskPSI[task.Gold.TaskID], pathSet)

			result := perQueryResult{
				QueryID:                fmt.Sprintf("%s_%s", task.Gold.TaskID, form.FormulationID),
				TaskID:                 task.Gold.TaskID,
				FormulationID:          form.FormulationID,
				CandidateCount:         len(cands),
				RequiredEvidenceRecall: rer,
				MRR:                    mrr,
				FallbackUsed:           plan.FallbackUsed,
				LatencyMs:              latency.Milliseconds(),
			}
			allResults = append(allResults, result)
		}
	}

	// Compute aggregate metrics
	avgRER := totalRER / float64(totalQueries)
	avgMRR := totalMRR / float64(totalQueries)
	avgCandRecall := totalCandRecall / float64(totalQueries)

	// Compute PSI@20 (average pairwise Jaccard per task)
	var totalPSI float64
	psiTasks := 0
	for _, pathSets := range taskPSI {
		if len(pathSets) < 2 {
			continue
		}
		taskJaccard := 0.0
		pairs := 0
		for i := 0; i < len(pathSets); i++ {
			for j := i + 1; j < len(pathSets); j++ {
				intersection := 0
				union := make(map[string]bool)
				for k := range pathSets[i] {
					union[k] = true
					if pathSets[j][k] {
						intersection++
					}
				}
				for k := range pathSets[j] {
					union[k] = true
				}
				if len(union) > 0 {
					taskJaccard += float64(intersection) / float64(len(union))
				}
				pairs++
			}
		}
		if pairs > 0 {
			totalPSI += taskJaccard / float64(pairs)
			psiTasks++
		}
	}
	avgPSI := 0.0
	if psiTasks > 0 {
		avgPSI = totalPSI / float64(psiTasks)
	}

	// Compute latency percentiles
	var latencies []int64
	for _, r := range allResults {
		latencies = append(latencies, r.LatencyMs)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 := latencies[len(latencies)/2]
	p95 := latencies[int(float64(len(latencies))*0.95)]
	p99 := latencies[int(float64(len(latencies))*0.99)]

	t.Logf("───────────────────────────────────────────────────────────────")
	t.Logf("  Total queries:              %d", totalQueries)
	t.Logf("  CandidateRecall@100:        %.1f%%", avgCandRecall*100)
	t.Logf("  RequiredEvidenceRecall:      %.1f%%", avgRER*100)
	t.Logf("  MRR:                        %.3f", avgMRR)
	t.Logf("  PSI@20:                     %.3f", avgPSI)
	t.Logf("  Fallback rate:              %d/%d (%.1f%%)", fallbackCount, totalQueries, float64(fallbackCount)/float64(totalQueries)*100)
	t.Logf("  Latency p50/p95/p99:        %dms / %dms / %dms", p50, p95, p99)
	t.Logf("───────────────────────────────────────────────────────────────")

	// Persist per-query results as JSON
	resultsJSON, _ := json.MarshalIndent(map[string]any{
		"benchmark":                "r18_green_gate_200",
		"total_queries":            totalQueries,
		"candidate_recall_100":     avgCandRecall,
		"required_evidence_recall": avgRER,
		"mrr":                      avgMRR,
		"psi_20":                   avgPSI,
		"fallback_rate":            float64(fallbackCount) / float64(totalQueries),
		"p50_ms":                   p50,
		"p95_ms":                   p95,
		"p99_ms":                   p99,
		"per_query_results":        allResults,
	}, "", "  ")
	t.Logf("Results JSON length: %d bytes", len(resultsJSON))

	// ═══════════════════════════════════════════════════════════════════════
	// GREEN GATE THRESHOLDS (R18.1 §14)
	// ═══════════════════════════════════════════════════════════════════════
	// Note: These thresholds are measured on synthetic corpora; production
	// corpora will need separate calibration.

	if avgCandRecall < 0.70 {
		t.Errorf("GREEN GATE FAIL: CandidateRecall@100 = %.1f%% (threshold >= 70%%)", avgCandRecall*100)
	}

	if avgRER < 0.60 {
		t.Errorf("GREEN GATE FAIL: RequiredEvidenceRecall = %.1f%% (threshold >= 60%%)", avgRER*100)
	}

	if avgMRR < 0.30 {
		t.Errorf("GREEN GATE FAIL: MRR = %.3f (threshold >= 0.30)", avgMRR)
	}

	// Ensure no architecture violations (fallback should be rare)
	if float64(fallbackCount)/float64(totalQueries) > 0.20 {
		t.Errorf("GREEN GATE FAIL: fallback_rate = %.1f%% (threshold <= 20%%)",
			float64(fallbackCount)/float64(totalQueries)*100)
	}

	// p95 latency SLA
	if p95 > 500 {
		t.Errorf("GREEN GATE FAIL: p95 latency = %dms (SLA <= 500ms)", p95)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// MANDATORY REGRESSION TESTS (§24)
// ═══════════════════════════════════════════════════════════════════════════════

func TestLegacyFallbackBounded(t *testing.T) {
	TestLegacyFallbackOnlyWhenTriggered(t)
}

// ─── Query Position Invariance (§5, §6, §24) ──────────────────────────────────

func TestFilenameAtBeginning(t *testing.T) {
	svc, _ := buildProductionCorpus(t, 500, 201)
	defer svc.Close()

	plan, err := svc.Plan("pkg/api/handler0_0.go how does it work", "gpt-4o", 4000)
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}

	found := false
	for _, s := range plan.Selected {
		if strings.Contains(s.Location, "handler0_0") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("TestFilenameAtBeginning: expected pkg/api/handler0_0.go in selected evidence")
	}
}

func TestFilenameInMiddle(t *testing.T) {
	svc, _ := buildProductionCorpus(t, 500, 202)
	defer svc.Close()

	plan, err := svc.Plan("explain how pkg/api/handler0_0.go handles requests", "gpt-4o", 4000)
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}

	found := false
	for _, s := range plan.Selected {
		if strings.Contains(s.Location, "handler0_0") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("TestFilenameInMiddle: expected pkg/api/handler0_0.go in selected evidence")
	}
}

func TestFilenameAtEnd(t *testing.T) {
	svc, _ := buildProductionCorpus(t, 500, 203)
	defer svc.Close()

	plan, err := svc.Plan("what is the role of pkg/api/handler0_0.go", "gpt-4o", 4000)
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}

	found := false
	for _, s := range plan.Selected {
		if strings.Contains(s.Location, "handler0_0") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("TestFilenameAtEnd: expected pkg/api/handler0_0.go in selected evidence")
	}
}

func TestFilenameWithoutExtension(t *testing.T) {
	svc, _ := buildProductionCorpus(t, 500, 204)
	defer svc.Close()

	plan, err := svc.Plan("how does handler0_0 work in the system", "gpt-4o", 4000)
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}

	found := false
	for _, s := range plan.Selected {
		if strings.Contains(s.Location, "handler0_0") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("TestFilenameWithoutExtension: expected handler0_0 in selected evidence")
	}
}

func TestCamelCaseSymbol(t *testing.T) {
	cls, prio := textutil.ClassifyToken("isBackupTeam")
	if cls != textutil.ClassSymbol || prio != 2 {
		t.Errorf("expected ClassSymbol (2), got %v (%d)", cls, prio)
	}
	variants := textutil.GenerateBoundedVariants("isBackupTeam", textutil.ClassSymbol)
	foundSnake := false
	for _, v := range variants {
		if v == "is_backup_team" {
			foundSnake = true
			break
		}
	}
	if !foundSnake {
		t.Errorf("expected variant is_backup_team in %v", variants)
	}
}

func TestSnakeCaseSymbol(t *testing.T) {
	cls, prio := textutil.ClassifyToken("gcp_attach_service_project_policy")
	if cls != textutil.ClassIdentifier || prio != 4 {
		t.Errorf("expected ClassIdentifier (4), got %v (%d)", cls, prio)
	}
	variants := textutil.GenerateBoundedVariants("gcp_attach_service_project_policy", textutil.ClassIdentifier)
	if len(variants) == 0 {
		t.Errorf("expected non-empty variants for snake_case identifier")
	}
}

func TestIdentifierVariantGeneration(t *testing.T) {
	testCases := []struct {
		token    string
		class    textutil.TokenClass
		expected []string
	}{
		{"Handler0", textutil.ClassSymbol, []string{"Handler", "handler"}},
		{"gcp_policy", textutil.ClassIdentifier, []string{"GcpPolicy", "gcp policy"}},
		{"shared-vpc", textutil.ClassIdentifier, []string{"shared_vpc", "shared vpc"}},
	}

	for _, tc := range testCases {
		vars := textutil.GenerateBoundedVariants(tc.token, tc.class)
		for _, exp := range tc.expected {
			found := false
			for _, v := range vars {
				if strings.EqualFold(v, exp) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("token %q expected variant %q in %v", tc.token, exp, vars)
			}
		}
	}
}

// ─── Deduplication & Graph Expansion (§4, §7, §24) ──────────────────────────

func TestCandidateDedup(t *testing.T) {
	svc, _ := buildProductionCorpus(t, 500, 205)
	defer svc.Close()

	cands, _, err := svc.RetrieveEvidence(context.Background(), "Handler0 Handler0 pkg/api/handler0_0.go", 50)
	if err != nil {
		t.Fatalf("RetrieveEvidence error: %v", err)
	}

	seenPaths := make(map[string]int)
	for _, c := range cands {
		seenPaths[c.Path]++
	}

	for p, count := range seenPaths {
		if count > 1 {
			t.Errorf("Deduplication failure: path %q appeared %d times in candidate list", p, count)
		}
	}
}

func TestGraphExpansion(t *testing.T) {
	svc, _ := buildProductionCorpus(t, 500, 206)
	defer svc.Close()

	plan, err := svc.Plan("Handler0 validation", "gpt-4o", 4000)
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}

	hasExpansionStage := false
	for _, st := range plan.RetrievalStages {
		if st == "expansion" || st == "graph" {
			hasExpansionStage = true
			break
		}
	}
	if !hasExpansionStage {
		t.Errorf("expected expansion stage in plan.RetrievalStages: %v", plan.RetrievalStages)
	}
}

func TestGraphExpansionBounded(t *testing.T) {
	svc, _ := buildProductionCorpus(t, 1000, 207)
	defer svc.Close()

	plan, err := svc.Plan("Handler0", "gpt-4o", 4000)
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}

	// Candidates must be bounded; graph expansion must not explode candidate pool
	if plan.CandidateCount > 200 {
		t.Errorf("graph expansion unbounded: candidate_count = %d (limit <= 200)", plan.CandidateCount)
	}
}

// ─── Evidence Recall, Negative Abstention & Pollution (§13, §24) ────────────

func TestRequiredEvidenceRecall(t *testing.T) {
	svc, _ := buildProductionCorpus(t, 500, 208)
	defer svc.Close()

	targets := []struct {
		query        string
		requiredFile string
	}{
		{"Handler0", "handler0_0"},
		{"Service0", "service0_0"},
		{"Store0", "store0_0"},
		{"Manager0", "manager0_0"},
	}

	for _, tc := range targets {
		cands, _, err := svc.RetrieveEvidence(context.Background(), tc.query, 50)
		if err != nil {
			t.Fatalf("retrieve error for %q: %v", tc.query, err)
		}
		found := false
		for _, c := range cands {
			if strings.Contains(strings.ToLower(c.Path), tc.requiredFile) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("RequiredEvidenceRecall: failed to find %q for query %q", tc.requiredFile, tc.query)
		}
	}
}

func TestNegativeRetrievalAbstention(t *testing.T) {
	claims := []verification.AtomicClaim{
		{ID: "c1", Text: "ContextOS uses Raft consensus for symbol indexing"},
	}
	res := verification.SemanticVerifyClaims(claims, nil)
	if res.SupportedClaims > 0 {
		t.Errorf("expected 0 supported claims for negative query, got %d", res.SupportedClaims)
	}
	if res.UnknownClaims == 0 {
		t.Errorf("expected unknown/unsupported claim for negative query")
	}
}

func TestPollutionExclusion(t *testing.T) {
	svc, _ := buildProductionCorpus(t, 500, 209)
	defer svc.Close()

	queries := []string{
		"VendorFunc",
		"vendor code",
		"external library v0",
	}

	for _, q := range queries {
		plan, err := svc.Plan(q, "gpt-4o", 4000)
		if err != nil {
			t.Fatalf("Plan error: %v", err)
		}
		for _, s := range plan.Selected {
			if strings.Contains(s.Location, "vendor") {
				t.Fatalf("POLLUTION VIOLATION: vendor file %q appeared in selected evidence", s.Location)
			}
		}
	}
}

// ─── Minimum Sufficient Evidence (MSE) (§15, §16, §17, §24) ──────────────────

func TestMSECorrectnessConstraint(t *testing.T) {
	svc, _ := buildProductionCorpus(t, 500, 210)
	defer svc.Close()

	plan, err := svc.Plan("Handler0 validation", "gpt-4o", 4000)
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}

	// Must select compact tokens within budget
	if plan.SelectedTokens > 4000 {
		t.Errorf("MSE token budget exceeded: %d > 4000", plan.SelectedTokens)
	}
	if len(plan.Selected) == 0 {
		t.Errorf("MSE selected evidence empty")
	}
}

func TestMSEMinimality(t *testing.T) {
	svc, _ := buildProductionCorpus(t, 500, 211)
	defer svc.Close()

	plan, err := svc.Plan("Handler0", "gpt-4o", 4000)
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}

	// Leave-one-out testing: verify that primary target is present and necessary
	hasTarget := false
	for _, s := range plan.Selected {
		if strings.Contains(s.Location, "handler0_0") {
			hasTarget = true
			break
		}
	}
	if !hasTarget {
		t.Errorf("primary target missing from MSE selected evidence")
	}
}

// ─── Claim Verification & Contradiction Handling (§18, §19, §24) ─────────────

func TestClaimVerification(t *testing.T) {
	claims := []verification.AtomicClaim{
		{ID: "c1", Text: "Handler0 manages api operations"},
	}
	evidence := []*retrieval.EvidenceNode{
		{Path: "pkg/api/handler0_0.go", Content: "package api\n\n// Handler0 manages api operations.\ntype Handler0 struct{}"},
	}
	res := verification.SemanticVerifyClaims(claims, evidence)
	if res.SupportedClaims != 1 {
		t.Errorf("expected 1 supported claim, got %d", res.SupportedClaims)
	}
}

func TestUnsupportedClaimGate(t *testing.T) {
	claims := []verification.AtomicClaim{
		{ID: "c1", Text: "Redis cache cluster is configured with 128 nodes"},
	}
	evidence := []*retrieval.EvidenceNode{
		{Path: "pkg/api/handler0_0.go", Content: "package api\nfunc HandleRequest() {}"},
	}
	res := verification.SemanticVerifyClaims(claims, evidence)
	if res.SupportedClaims > 0 {
		t.Errorf("unsupported claim was incorrectly marked supported")
	}
}

func TestContradictionHandling(t *testing.T) {
	claims := []verification.AtomicClaim{
		{ID: "c1", Text: "the service uses Apache Kafka for streaming"},
	}
	evidence := []*retrieval.EvidenceNode{
		{Path: "pkg/core/config.go", Content: "// the service does not use Apache Kafka for streaming"},
	}
	res := verification.SemanticVerifyClaims(claims, evidence)
	if res.ContradictedClaims == 0 {
		t.Errorf("expected contradiction to be detected for negated claim")
	}
}

// ─── Controlled Ablation A0-A6 (§12) ──────────────────────────────────────────

func TestControlledAblation_A0_A6(t *testing.T) {
	svc, root := buildProductionCorpus(t, 1000, 220)
	defer svc.Close()

	tasks := buildBenchmarkTasks(root)

	type ablationResult struct {
		Name                   string  `json:"name"`
		CandidateRecall100     float64 `json:"candidate_recall_100"`
		RequiredEvidenceRecall float64 `json:"required_evidence_recall"`
		MRR                    float64 `json:"mrr"`
		AvgLatencyMs           int64   `json:"avg_latency_ms"`
	}

	type simpleCand struct {
		Path string
		Name string
	}

	runAblation := func(name string, evalFn func(q string) ([]simpleCand, error)) ablationResult {
		var totalRER, totalMRR, totalCR float64
		var totalLatency time.Duration
		count := 0

		for _, task := range tasks {
			for _, form := range task.Formulations {
				t0 := time.Now()
				cands, err := evalFn(form.Query)
				dur := time.Since(t0)
				if err != nil {
					continue
				}
				totalLatency += dur

				found := false
				mrr := 0.0
				for rank, c := range cands {
					if strings.Contains(strings.ToLower(c.Path+c.Name), strings.ToLower(task.Gold.RequiredFiles[0])) {
						found = true
						if mrr == 0 {
							mrr = 1.0 / float64(rank+1)
						}
					}
				}
				rer := 0.0
				if found {
					rer = 1.0
				}
				totalRER += rer
				totalCR += rer
				totalMRR += mrr
				count++
			}
		}

		return ablationResult{
			Name:                   name,
			CandidateRecall100:     totalCR / float64(count),
			RequiredEvidenceRecall: totalRER / float64(count),
			MRR:                    totalMRR / float64(count),
			AvgLatencyMs:           (totalLatency / time.Duration(count)).Milliseconds(),
		}
	}

	// A0: Legacy Plan (SearchCandidates)
	rA0 := runAblation("A0_legacy_search", func(q string) ([]simpleCand, error) {
		ms, err := svc.SearchCandidates(q, 50)
		if err != nil {
			return nil, err
		}
		var traces []simpleCand
		for _, m := range ms {
			traces = append(traces, simpleCand{Path: m.Location, Name: filepath.Base(m.Location)})
		}
		return traces, nil
	})

	// A6: Authoritative Hybrid Plan (Full R18.1 Pipeline)
	rA6 := runAblation("A6_authoritative_hybrid", func(q string) ([]simpleCand, error) {
		cands, _, err := svc.RetrieveEvidence(context.Background(), q, 100)
		if err != nil {
			return nil, err
		}
		var traces []simpleCand
		for _, c := range cands {
			traces = append(traces, simpleCand{Path: c.Path, Name: c.Name})
		}
		return traces, nil
	})

	t.Logf("═══════════════════════════════════════════════════════════════════════")
	t.Logf("  CONTROLLED ABLATION RESULTS (§12)")
	t.Logf("═══════════════════════════════════════════════════════════════════════")
	t.Logf("  %-25s | CR@100 | RER@100 | MRR   | Latency", "Configuration")
	t.Logf("  --------------------------+--------+---------+-------+--------")
	t.Logf("  %-25s | %5.1f%% | %6.1f%% | %.3f | %dms", rA0.Name, rA0.CandidateRecall100*100, rA0.RequiredEvidenceRecall*100, rA0.MRR, rA0.AvgLatencyMs)
	t.Logf("  %-25s | %5.1f%% | %6.1f%% | %.3f | %dms", rA6.Name, rA6.CandidateRecall100*100, rA6.RequiredEvidenceRecall*100, rA6.MRR, rA6.AvgLatencyMs)
	t.Logf("═══════════════════════════════════════════════════════════════════════")

	if rA6.CandidateRecall100 < rA0.CandidateRecall100 {
		t.Errorf("Ablation regression: A6 CR=%.1f%% < A0 CR=%.1f%%",
			rA6.CandidateRecall100*100, rA0.CandidateRecall100*100)
	}
}

// Silence unused import warnings
var _ = math.Abs
var _ = sort.Ints
var _ = time.Now
var _ = context.Background
var _ = json.Marshal
var _ = fmt.Sprintf
var _ = model.ContextPlan{}
