package compute_bench

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"contextos/internal/compute"
)

// TaskFeatures captures measured complexity and evidence attributes.
type TaskFeatures struct {
	FilesCount         int     `json:"files_count"`
	SymbolsCount       int     `json:"symbols_count"`
	DependenciesCount  int     `json:"dependencies_count"`
	GraphDistance      int     `json:"graph_distance"`
	AmbiguityScore     float64 `json:"ambiguity_score"`     // 0.0 = unambiguous, 1.0 = highly underspecified
	EvidenceCoverage   float64 `json:"evidence_coverage"`   // 0.0 = no evidence, 1.0 = complete evidence
	ExpectedPatchLines int     `json:"expected_patch_lines"`
	TestComplexity     float64 `json:"test_complexity"` // 0.0 = simple assertion, 1.0 = concurrent stress
}

// TaskMatrixItem defines a concrete task in the 120-task R15 matrix.
type TaskMatrixItem struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	Class              compute.TaskClass `json:"class"`
	Category           string            `json:"category"`
	Query              string            `json:"query"`
	Features           TaskFeatures      `json:"features"`
	MeasuredDifficulty float64           `json:"measured_difficulty"`
	CanBypass          bool              `json:"can_bypass"`
	SuccessOracle      string            `json:"success_oracle"`
	GroundTruthFiles   []string          `json:"ground_truth_files"`
}

// ComputeMeasuredDifficulty calculates D in [0.0, 1.0] from feature vector.
func ComputeMeasuredDifficulty(f TaskFeatures) float64 {
	normFiles := mathMin(float64(f.FilesCount)/10.0, 1.0)
	normSymbols := mathMin(float64(f.SymbolsCount)/20.0, 1.0)
	normGraphDist := mathMin(float64(f.GraphDistance)/5.0, 1.0)
	normPatch := mathMin(float64(f.ExpectedPatchLines)/250.0, 1.0)

	diff := 0.15*normFiles +
		0.15*normSymbols +
		0.15*normGraphDist +
		0.20*f.AmbiguityScore +
		0.15*(1.0-f.EvidenceCoverage) +
		0.10*normPatch +
		0.10*f.TestComplexity

	if diff < 0.02 {
		diff = 0.02
	}
	if diff > 0.98 {
		diff = 0.98
	}
	return mathRound(diff*100) / 100.0
}

func mathMin(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func mathRound(v float64) float64 {
	return float64(int64(v+0.5))
}

// BuildR15TaskMatrix returns the standardized 120-task matrix.
func BuildR15TaskMatrix() []TaskMatrixItem {
	var items []TaskMatrixItem

	// 1. T0 Deterministic: 20 tasks (Symbols, Callers, Imports, Exact Lookups)
	t0Defs := []struct {
		name     string
		cat      string
		query    string
		oracle   string
		files    []string
		syms     int
		graphD   int
		ambig    float64
		cov      float64
		patch    int
		testComp float64
	}{
		{"find_symbol_handleplan", "symbol_lookup", "where is func HandlePlan defined in server.go", "exact_symbol_location", []string{"internal/ui/server.go"}, 1, 0, 0.01, 1.0, 0, 0.05},
		{"find_callers_computecost", "caller_discovery", "find callers of ComputeCost in router.go", "exact_caller_graph", []string{"internal/router/router.go"}, 2, 1, 0.02, 1.0, 0, 0.05},
		{"list_gitidx_imports", "import_dependency", "list imports of gitidx package", "exact_import_list", []string{"internal/gitidx/git.go"}, 1, 0, 0.01, 1.0, 0, 0.05},
		{"locate_memory_lifecycle", "symbol_lookup", "locate struct MemoryLifecycle in bench/layers.go", "exact_symbol_location", []string{"internal/bench/layers.go"}, 1, 0, 0.01, 1.0, 0, 0.05},
		{"find_store_interface", "symbol_lookup", "find implementations of Store interface in store package", "exact_implementations", []string{"internal/store/store.go"}, 2, 1, 0.02, 1.0, 0, 0.05},
		{"method_calculateusagecost", "symbol_lookup", "get method signature of CalculateUsageCost in telemetry", "exact_symbol_signature", []string{"internal/telemetry/costs.go"}, 1, 0, 0.01, 1.0, 0, 0.05},
		{"find_pricing_catalog", "symbol_lookup", "find where defaultPricingCatalog is initialized in costs.go", "exact_symbol_location", []string{"internal/telemetry/costs.go"}, 1, 0, 0.01, 1.0, 0, 0.05},
		{"find_effort_level_enum", "symbol_lookup", "find definition of EffortLevel enum in compute/effort.go", "exact_symbol_location", []string{"internal/compute/effort.go"}, 1, 0, 0.01, 1.0, 0, 0.05},
		{"callers_estimatecurve", "caller_discovery", "find callers of EstimateCurve in estimator.go", "exact_caller_graph", []string{"internal/compute/estimator.go"}, 2, 1, 0.02, 1.0, 0, 0.05},
		{"imports_internal_mcp", "import_dependency", "list imports in internal/mcp/server.go", "exact_import_list", []string{"internal/mcp/server.go"}, 1, 0, 0.01, 1.0, 0, 0.05},
		{"find_ppr_algorithm", "symbol_lookup", "locate PersonalizedPageRank function in graph package", "exact_symbol_location", []string{"internal/graph/graph.go"}, 1, 0, 0.01, 1.0, 0, 0.05},
		{"find_sqlite_wal_pragma", "symbol_lookup", "where is PRAGMA journal_mode=WAL executed in sqlite store", "exact_ast_match", []string{"internal/store/sqlite_store.go"}, 1, 0, 0.01, 1.0, 0, 0.05},
		{"find_wand_block_size", "symbol_lookup", "find block size constant in Block-Max WAND implementation", "exact_const_value", []string{"internal/textutil/wand.go"}, 1, 0, 0.01, 1.0, 0, 0.05},
		{"callers_remember_memory", "caller_discovery", "find callers of service.Remember in server package", "exact_caller_graph", []string{"internal/server/service.go"}, 2, 1, 0.02, 1.0, 0, 0.05},
		{"find_taskprofiler_type", "symbol_lookup", "find definition of TaskProfiler struct in compute package", "exact_symbol_location", []string{"internal/compute/profiler.go"}, 1, 0, 0.01, 1.0, 0, 0.05},
		{"find_evidence_coverage", "symbol_lookup", "where is EvidenceState.CoverageRatio defined in state package", "exact_symbol_location", []string{"internal/state/evidence.go"}, 1, 0, 0.01, 1.0, 0, 0.05},
		{"list_router_dependencies", "import_dependency", "list dependencies of internal/router package", "exact_import_list", []string{"internal/router/router.go"}, 1, 0, 0.01, 1.0, 0, 0.05},
		{"find_decisionstate_compile", "symbol_lookup", "locate CompileConversation in state/compiler.go", "exact_symbol_location", []string{"internal/state/compiler.go"}, 1, 0, 0.01, 1.0, 0, 0.05},
		{"find_hierarchical_verifier", "symbol_lookup", "where is HierarchicalVerifier instantiated in verification package", "exact_symbol_location", []string{"internal/verification/verifier.go"}, 1, 0, 0.01, 1.0, 0, 0.05},
		{"find_mcp_protocol_version", "symbol_lookup", "find JSON-RPC protocol version constant in mcp package", "exact_const_value", []string{"internal/mcp/server.go"}, 1, 0, 0.01, 1.0, 0, 0.05},
	}

	for i, d := range t0Defs {
		f := TaskFeatures{
			FilesCount:         len(d.files),
			SymbolsCount:       d.syms,
			DependenciesCount:  0,
			GraphDistance:      d.graphD,
			AmbiguityScore:     d.ambig,
			EvidenceCoverage:   d.cov,
			ExpectedPatchLines: d.patch,
			TestComplexity:     d.testComp,
		}
		items = append(items, TaskMatrixItem{
			ID:                 fmt.Sprintf("T0-%02d", i+1),
			Name:               d.name,
			Class:              compute.T0Deterministic,
			Category:           d.cat,
			Query:              d.query,
			Features:           f,
			MeasuredDifficulty: ComputeMeasuredDifficulty(f),
			CanBypass:          true,
			SuccessOracle:      d.oracle,
			GroundTruthFiles:   d.files,
		})
	}

	// 2. T1 Trivial: 20 tasks (Typos, docstrings, formatting, minor constants)
	t1Defs := []struct {
		name   string
		query  string
		oracle string
		files  []string
		diff   float64
	}{
		{"fix_typo_readme", "fix typo in README license link", "exact_patch_match", []string{"README.md"}, 0.12},
		{"add_docstring_exportgraph", "add docstring comment for ExportGraph in graph.go", "linter_and_doc_presence", []string{"internal/graph/graph.go"}, 0.14},
		{"update_cli_usage_flag", "update usage text for -adaptive-budget flag in ctxbench", "exact_patch_match", []string{"cmd/ctxbench/main.go"}, 0.13},
		{"add_json_tag_metadata", "ensure Json tags are present on TaskFeatures struct", "linter_clean", []string{"internal/compute/profiler.go"}, 0.15},
		{"bump_version_minor", "bump minor version in internal/version/version.go", "exact_patch_match", []string{"internal/version/version.go"}, 0.11},
		{"clarify_err_timeout", "clarify error message when context planning times out", "exact_patch_match", []string{"internal/ui/server.go"}, 0.16},
		{"export_const_default_budget", "export DefaultContextBudget constant in model package", "symbol_export_check", []string{"internal/model/types.go"}, 0.14},
		{"fix_comment_bmw_invariant", "fix comment typo describing Block-Max WAND upper bound invariant", "exact_patch_match", []string{"internal/textutil/wand.go"}, 0.12},
		{"format_go_imports", "clean up unused imports in router_test.go", "go_fmt_clean", []string{"internal/router/router_test.go"}, 0.11},
		{"add_cost_kpi_docstring", "add docstring comment explaining Cost Per Success calculation", "linter_and_doc_presence", []string{"internal/telemetry/metrics.go"}, 0.15},
		{"add_nil_check_doc", "add docstring for SafeNilCheck in textutil", "linter_and_doc_presence", []string{"internal/textutil/textutil.go"}, 0.13},
		{"update_license_year", "update copyright year in LICENSE file to 2026", "exact_patch_match", []string{"LICENSE"}, 0.10},
		{"add_log_prefix_debug", "add standard [DEBUG] prefix to planner diagnostic log", "exact_patch_match", []string{"internal/planning/cost_model.go"}, 0.14},
		{"export_error_sentinel", "define ErrTaskNotFound sentinel error in store package", "symbol_export_check", []string{"internal/store/store.go"}, 0.15},
		{"fix_typo_godoc_pruning", "correct misspelling of submodularity in allocator godoc", "exact_patch_match", []string{"internal/allocator/selector.go"}, 0.12},
		{"add_version_endpoint_comment", "add HTTP route comment to handleStatus in ui server", "exact_patch_match", []string{"internal/ui/server.go"}, 0.13},
		{"rename_internal_helper", "rename unexported tokenCounter to countTokens in textutil", "compiles_and_passes_tests", []string{"internal/textutil/textutil.go"}, 0.17},
		{"add_const_max_cache_age", "declare DefaultMaxCacheAgeSeconds = 3600 in store", "symbol_export_check", []string{"internal/store/sqlite_store.go"}, 0.14},
		{"fix_markdown_table_header", "align markdown column pipes in PR.md telemetry table", "exact_patch_match", []string{"PR.md"}, 0.11},
		{"add_test_helper_comment", "document test fixture helper in server_test.go", "linter_and_doc_presence", []string{"internal/ui/server_test.go"}, 0.13},
	}

	for i, d := range t1Defs {
		f := TaskFeatures{
			FilesCount:         len(d.files),
			SymbolsCount:       2,
			DependenciesCount:  1,
			GraphDistance:      1,
			AmbiguityScore:     0.10,
			EvidenceCoverage:   0.95,
			ExpectedPatchLines: 15,
			TestComplexity:     0.10,
		}
		diff := d.diff
		if diff == 0 {
			diff = ComputeMeasuredDifficulty(f)
		}
		items = append(items, TaskMatrixItem{
			ID:                 fmt.Sprintf("T1-%02d", i+1),
			Name:               d.name,
			Class:              compute.T1Trivial,
			Category:           "typo_doc_edit",
			Query:              d.query,
			Features:           f,
			MeasuredDifficulty: diff,
			CanBypass:          false,
			SuccessOracle:      d.oracle,
			GroundTruthFiles:   d.files,
		})
	}

	// 3. T2 Moderate: 30 tasks (Local bug fixes, nil checks, unit tests, error propagation)
	t2Defs := []struct {
		name   string
		cat    string
		query  string
		oracle string
		files  []string
		diff   float64
	}{
		{"nil_pointer_handling", "local_bug_fix", "handle nil pointer in json error response", "compiler_and_unit_test", []string{"internal/ui/server.go"}, 0.35},
		{"session_unit_test", "unit_test_addition", "add unit test for sqlite store session expiration", "unit_test_pass", []string{"internal/store/sqlite_store_test.go"}, 0.40},
		{"bounds_check_trigrams", "local_bug_fix", "prevent slice out-of-bounds in trigram sliding window", "unit_test_pass", []string{"internal/textutil/textutil.go"}, 0.32},
		{"validate_timeout_bounds", "local_bug_fix", "validate timeout parameter bounds in handlePlan", "compiler_and_unit_test", []string{"internal/ui/server.go"}, 0.30},
		{"unit_test_exp_backoff", "unit_test_addition", "add unit test for exponential backoff in provider retry", "unit_test_pass", []string{"internal/providers/provider_test.go"}, 0.38},
		{"sanitize_query_input", "local_bug_fix", "strip null bytes and control chars from query input in router", "unit_test_pass", []string{"internal/router/router.go"}, 0.33},
		{"unit_test_token_accounting", "unit_test_addition", "add unit test verifying effective input tokens calculation", "unit_test_pass", []string{"internal/telemetry/telemetry_test.go"}, 0.36},
		{"fix_sqlite_session_query", "local_bug_fix", "fix SQL parameter binding in ListSessions order clause", "unit_test_pass", []string{"internal/store/sqlite_store.go"}, 0.42},
		{"unit_test_bmw_wand", "unit_test_addition", "add test verifying Block-Max WAND skips non-competitive postings", "unit_test_pass", []string{"internal/textutil/wand_test.go"}, 0.44},
		{"handle_empty_corpus", "local_bug_fix", "return early without error when indexing empty repository directory", "unit_test_pass", []string{"internal/indexer/indexer.go"}, 0.34},
		{"propagate_context_deadline", "local_bug_fix", "propagate request context deadline into store query methods", "unit_test_pass", []string{"internal/store/sqlite_store.go"}, 0.41},
		{"unit_test_profiler_strat", "unit_test_addition", "add unit test for task difficulty stratification across T0-T4", "unit_test_pass", []string{"internal/compute/profiler_test.go"}, 0.37},
		{"fix_json_unmarshal_field", "local_bug_fix", "support both snake_case and camelCase in MCP tool options JSON", "unit_test_pass", []string{"internal/mcp/server.go"}, 0.35},
		{"unit_test_evidence_sat", "unit_test_addition", "add test for evidence satisfaction weighted summation", "unit_test_pass", []string{"internal/state/evidence_test.go"}, 0.36},
		{"enforce_max_budget_ceiling", "local_bug_fix", "clamp context budget to maximum 64k tokens in allocator", "unit_test_pass", []string{"internal/allocator/selector.go"}, 0.33},
		{"unit_test_hierarchical_budget", "unit_test_addition", "add unit test for hierarchical budget reservation overdraft", "unit_test_pass", []string{"internal/compute/compute_test.go"}, 0.39},
		{"fix_cache_ttl_check", "local_bug_fix", "correct monotonic clock comparison in plan cache freshness check", "unit_test_pass", []string{"internal/cache/cache.go"}, 0.38},
		{"unit_test_decision_state_hash", "unit_test_addition", "add test asserting deterministic hash ID of DecisionState", "unit_test_pass", []string{"internal/state/state_test.go"}, 0.35},
		{"handle_closed_db_graceful", "local_bug_fix", "return ErrDatabaseClosed instead of panicking on closed db", "unit_test_pass", []string{"internal/store/sqlite_store.go"}, 0.36},
		{"unit_test_pricing_lookup", "unit_test_addition", "test model pricing catalog fallback for unknown model names", "unit_test_pass", []string{"internal/telemetry/telemetry_test.go"}, 0.32},
		{"prevent_division_by_zero_cps", "local_bug_fix", "guard against zero successful tasks in ComputeSummaryKPIs", "unit_test_pass", []string{"internal/telemetry/metrics.go"}, 0.31},
		{"unit_test_calibrated_stopping", "unit_test_addition", "test stopping criteria when calibrated risk drops below epsilon", "unit_test_pass", []string{"internal/compute/stopping_test.go"}, 0.43},
		{"fix_mcp_error_code_mapping", "local_bug_fix", "map context cancellation to JSON-RPC -32000 error code", "unit_test_pass", []string{"internal/mcp/server.go"}, 0.34},
		{"unit_test_git_head_parsing", "unit_test_addition", "add test parsing detached HEAD in gitidx package", "unit_test_pass", []string{"internal/gitidx/git_test.go"}, 0.37},
		{"fix_memory_confidence_clamp", "local_bug_fix", "clamp extracted confidence score to [0.0, 1.0]", "unit_test_pass", []string{"internal/extractor/extractor.go"}, 0.30},
		{"unit_test_file_store_prune", "unit_test_addition", "add unit test verifying file store auto-prune deletes old traces", "unit_test_pass", []string{"internal/store/file_store_test.go"}, 0.40},
		{"fix_regex_pattern_escape", "local_bug_fix", "properly escape special characters in trigram regex matcher", "unit_test_pass", []string{"internal/textutil/textutil.go"}, 0.36},
		{"unit_test_uncertainty_action", "unit_test_addition", "test uncertainty classification mapping to remediating action", "unit_test_pass", []string{"internal/state/state_test.go"}, 0.39},
		{"prevent_goroutine_leak_timer", "local_bug_fix", "ensure time.Timer is stopped in adaptive timeout select", "unit_test_pass", []string{"internal/ui/server.go"}, 0.42},
		{"unit_test_doctor_diagnostics", "unit_test_addition", "add unit test asserting doctor reports missing database cleanly", "unit_test_pass", []string{"internal/doctor/doctor_test.go"}, 0.33},
	}

	for i, d := range t2Defs {
		f := TaskFeatures{
			FilesCount:         len(d.files),
			SymbolsCount:       4,
			DependenciesCount:  2,
			GraphDistance:      2,
			AmbiguityScore:     0.28,
			EvidenceCoverage:   0.82,
			ExpectedPatchLines: 35,
			TestComplexity:     0.35,
		}
		items = append(items, TaskMatrixItem{
			ID:                 fmt.Sprintf("T2-%02d", i+1),
			Name:               d.name,
			Class:              compute.T2Moderate,
			Category:           d.cat,
			Query:              d.query,
			Features:           f,
			MeasuredDifficulty: d.diff,
			CanBypass:          false,
			SuccessOracle:      d.oracle,
			GroundTruthFiles:   d.files,
		})
	}

	// 4. T3 Difficult: 30 tasks (Cross-module refactors, multi-file invalidation, deadlocks)
	t3Defs := []struct {
		name   string
		cat    string
		query  string
		oracle string
		files  []string
		diff   float64
	}{
		{"cross_module_refactor", "multifile_refactor", "refactor cross-module cache invalidation across store and graph", "cross_module_integration_tests", []string{"internal/store/store.go", "internal/graph/graph.go", "internal/server/service.go"}, 0.65},
		{"concurrency_deadlock", "race_deadlock", "investigate concurrent deadlock in session scheduler under load", "race_detector_and_deadlock_test", []string{"internal/server/service.go", "internal/store/sqlite_store.go"}, 0.70},
		{"two_phase_commit_file_store", "multifile_refactor", "implement atomic two-phase commit in file store WAL", "integration_test_pass", []string{"internal/store/file_store.go", "internal/store/wal.go"}, 0.68},
		{"optimize_ppr_pool", "multifile_refactor", "optimize PPR graph walk memory allocations using sync.Pool", "benchmark_allocs_pass", []string{"internal/graph/graph.go", "internal/retrieval/graph/ppr.go"}, 0.62},
		{"atomic_index_swap", "multifile_refactor", "implement zero-lock atomic pointer swap for active search index", "concurrency_test_pass", []string{"internal/indexer/indexer.go", "internal/retrieval/index/index.go"}, 0.67},
		{"joint_pareto_optimizer", "api_change", "implement Pareto frontier solver balancing context and reasoning tokens", "pareto_solver_test", []string{"internal/planning/cost_model.go", "internal/planning/compute_plan.go"}, 0.72},
		{"conversation_compaction_89", "multifile_refactor", "compact 100-turn history into DecisionState preserving commitments", "compaction_ratio_and_facts_pass", []string{"internal/state/compiler.go", "internal/state/decision_state.go"}, 0.69},
		{"model_cascade_backoff", "api_change", "implement automated multi-tier cascade fallback on provider rate limits", "cascade_simulation_test", []string{"internal/compute/routing.go", "internal/providers/provider.go"}, 0.64},
		{"dynamic_budget_partitioning", "multifile_refactor", "implement hierarchical budget manager enforcing verification reserve", "budget_partition_test", []string{"internal/compute/budget.go", "internal/compute/controller.go"}, 0.66},
		{"cache_aware_voi_controller", "api_change", "incorporate cache thrashing penalty into Value-of-Information evaluator", "voi_cache_penalty_test", []string{"internal/compute/voi.go", "internal/compute/controller.go"}, 0.71},
		{"cross_repo_symlink_traversal", "local_bug_fix", "prevent infinite cyclic symlink traversal during incremental git indexing", "cycle_detection_test", []string{"internal/gitidx/git.go", "internal/indexer/indexer.go"}, 0.58},
		{"bidirectional_edge_reconciliation", "multifile_refactor", "reconcile caller-callee bidirectional graph edges on partial file edit", "graph_consistency_test", []string{"internal/graph/graph.go", "internal/store/sqlite_store.go"}, 0.63},
		{"dynamic_timeout_degradation", "api_change", "adaptively downscale context budget when query reaches 80% SLA deadline", "sla_degradation_test", []string{"internal/server/service.go", "internal/allocator/selector.go"}, 0.61},
		{"sqlite_wal_checkpoint_tuning", "multifile_refactor", "implement passive background WAL checkpointing to prevent disk bloat", "wal_checkpoint_test", []string{"internal/store/sqlite_store.go"}, 0.59},
		{"submodular_candidate_fusion", "multifile_refactor", "implement Reciprocal Rank Fusion blending graph PPR and BM25 postings", "rrf_fusion_test", []string{"internal/allocator/fusion.go", "internal/allocator/selector.go"}, 0.67},
		{"temporal_scope_freshness_penalty", "api_change", "penalize stale memories when modified file commit diverges from revision", "freshness_penalty_test", []string{"internal/temporal/temporal.go", "internal/server/service.go"}, 0.64},
		{"cross_agent_handoff_persistence", "multifile_refactor", "serialize and persist cross-agent handoff state across Antigravity and Cursor", "handoff_roundtrip_test", []string{"internal/integrations/integrations.go", "internal/state/compiler.go"}, 0.68},
		{"concurrent_eviction_ring_buffer", "race_deadlock", "prevent race condition in memory eviction ring buffer under parallel inserts", "race_detector_pass", []string{"internal/cache/cache.go"}, 0.65},
		{"multi_stage_verification_escalation", "api_change", "escalate failed Level 1 compiler check to Level 3 cheap model validation", "verification_escalation_test", []string{"internal/verification/verifier.go", "internal/verification/policies.go"}, 0.73},
		{"causal_attribution_blame_graph", "multifile_refactor", "attribute task failures to missing context entities via causal DAG walk", "causal_attribution_test", []string{"internal/graph/causal.go", "internal/server/service.go"}, 0.74},
		{"adaptive_reasoning_curve_fitting", "api_change", "fit parametric sigmoid curve to empirical reasoning-accuracy checkpoints", "curve_fitting_test", []string{"internal/compute/estimator.go"}, 0.63},
		{"compressed_postings_delta_pfor", "multifile_refactor", "implement SIMD-friendly FastPFor/Elias-Fano compression for postings lists", "compression_ratio_test", []string{"internal/textutil/compressed.go"}, 0.72},
		{"provider_neutral_token_normalizer", "api_change", "normalize Anthropic thinking tokens, OpenAI reasoning tokens, and Gemini thoughts", "token_normalization_test", []string{"internal/telemetry/usage.go", "internal/providers/provider.go"}, 0.60},
		{"graph_cycle_pruning_heisenbug", "race_deadlock", "debug non-deterministic graph pruning heisenbug during parallel git diffs", "race_stress_100_runs", []string{"internal/graph/graph.go"}, 0.75},
		{"multi_tenant_repo_isolation", "multifile_refactor", "enforce strict cryptographic tenant isolation across shared SQLite store", "tenant_isolation_test", []string{"internal/store/sqlite_store.go"}, 0.67},
		{"adaptive_hysteresis_policy", "api_change", "implement policy transition hysteresis preventing rapid model thrashing", "hysteresis_damping_test", []string{"internal/compute/controller.go"}, 0.65},
		{"semantic_drift_detector", "multifile_refactor", "detect embedding semantic drift between code entities across git branches", "drift_detection_test", []string{"internal/semantic/semantic.go"}, 0.69},
		{"hierarchical_verifier_circuit_breaker", "api_change", "trip circuit breaker and abort escalation when verification cost > task budget", "circuit_breaker_test", []string{"internal/verification/policies.go"}, 0.64},
		{"content_addressed_worktree_hash", "multifile_refactor", "generate deterministic Merkle root hash of dirty unstaged git worktree", "merkle_root_test", []string{"internal/gitidx/git.go"}, 0.62},
		{"out_of_core_graph_spilling", "multifile_refactor", "spill cold graph nodes to disk when active node cache exceeds 100k nodes", "spilling_integration_test", []string{"internal/graph/graph.go", "internal/store/file_store.go"}, 0.71},
	}

	for i, d := range t3Defs {
		f := TaskFeatures{
			FilesCount:         len(d.files),
			SymbolsCount:       9,
			DependenciesCount:  4,
			GraphDistance:      3,
			AmbiguityScore:     0.55,
			EvidenceCoverage:   0.62,
			ExpectedPatchLines: 120,
			TestComplexity:     0.68,
		}
		items = append(items, TaskMatrixItem{
			ID:                 fmt.Sprintf("T3-%02d", i+1),
			Name:               d.name,
			Class:              compute.T3Difficult,
			Category:           d.cat,
			Query:              d.query,
			Features:           f,
			MeasuredDifficulty: d.diff,
			CanBypass:          false,
			SuccessOracle:      d.oracle,
			GroundTruthFiles:   d.files,
		})
	}

	// 5. T4 Critical / Frontier: 20 tasks (Distributed consensus, architectural changes, invariants)
	t4Defs := []struct {
		name   string
		cat    string
		query  string
		oracle string
		files  []string
		diff   float64
	}{
		{"distributed_consensus", "distributed_system", "design distributed multi-region replication protocol with Raft consensus", "invariants_simulation_and_safety_checks", []string{"internal/store/raft.go", "internal/store/cluster.go"}, 0.88},
		{"byzantine_fault_tolerance", "distributed_system", "design BFT consensus engine resilient to 1/3 malicious peer nodes", "byzantine_simulation_test", []string{"internal/store/bft.go"}, 0.92},
		{"zero_downtime_online_migration", "architectural_change", "architect zero-downtime online schema migration for multi-terabyte index", "online_migration_safety_test", []string{"internal/store/migration.go"}, 0.89},
		{"formal_invariant_verification", "architectural_change", "formalize mathematical invariant proofs for state transition graph", "tla_plus_invariant_check", []string{"internal/state/invariants.go"}, 0.95},
		{"multi_region_active_active", "distributed_system", "implement multi-region active-active masterless context sync engine", "conflict_free_crdt_test", []string{"internal/store/crdt.go"}, 0.91},
		{"cryptographic_provenance_chain", "architectural_change", "implement tamper-evident Merkle tree proving decision state history", "cryptographic_audit_pass", []string{"internal/state/provenance.go"}, 0.86},
		{"autonomous_self_healing_index", "architectural_change", "design autonomous self-healing index recovery after ungraceful crash", "crash_consistency_fuzz_test", []string{"internal/indexer/repair.go"}, 0.87},
		{"differential_privacy_telemetry", "architectural_change", "implement local differential privacy budget for cross-repo cost telemetry", "differential_privacy_epsilon_test", []string{"internal/telemetry/privacy.go"}, 0.85},
		{"hardware_enclave_mcp_auth", "architectural_change", "design secure enclave attestation for remote MCP server executions", "enclave_attestation_test", []string{"internal/mcp/enclave.go"}, 0.93},
		{"lock_free_skip_list_index", "architectural_change", "implement cache-oblivious lock-free skip list for concurrent memory lookups", "lock_free_stress_test", []string{"internal/store/skiplist.go"}, 0.90},
		{"asynchronous_event_sourcing", "architectural_change", "transition entire ContextOS core to CQRS event sourcing event store", "event_sourcing_replay_test", []string{"internal/state/event_store.go"}, 0.88},
		{"global_p2p_gossip_protocol", "distributed_system", "design hybrid push-pull epidemic gossip protocol for cluster membership", "gossip_convergence_test", []string{"internal/store/gossip.go"}, 0.89},
		{"provably_safe_model_sandbox", "architectural_change", "implement WebAssembly runtime sandbox for executing untrusted tool hooks", "wasm_isolation_test", []string{"internal/hook/wasm.go"}, 0.94},
		{"vector_quantization_product_code", "architectural_change", "implement Product Quantization (PQ) vector index scaling to 10M embeddings", "pq_recall_and_compression_test", []string{"internal/semantic/pq.go"}, 0.87},
		{"linearizable_shared_registers", "distributed_system", "implement multi-writer linearizable register using Paxos consensus", "jepsen_linearizability_test", []string{"internal/store/paxos.go"}, 0.93},
		{"zero_copy_ipc_kernel_bypass", "architectural_change", "design shared memory ring buffer IPC between host agent and context daemon", "shm_latency_benchmark", []string{"internal/server/ipc.go"}, 0.86},
		{"adaptive_gradient_budget_controller", "architectural_change", "implement online reinforcement learning controller for continuous budget allocation", "rl_convergence_simulation", []string{"internal/compute/rl.go"}, 0.96},
		{"chaos_fault_injection_framework", "distributed_system", "build Jepsen-style network partition fault injector for ContextOS clusters", "partition_tolerance_test", []string{"internal/doctor/chaos.go"}, 0.91},
		{"multi_modal_ast_embedding_fusion", "architectural_change", "fuse AST control-flow graphs and transformer embeddings into unified manifold", "cross_modal_alignment_test", []string{"internal/semantic/multimodal.go"}, 0.92},
		{"provable_termination_verifier", "architectural_change", "implement static bounded model checker proving agent loops always terminate", "model_checker_soundness_test", []string{"internal/verification/bounded.go"}, 0.97},
	}

	for i, d := range t4Defs {
		f := TaskFeatures{
			FilesCount:         len(d.files),
			SymbolsCount:       18,
			DependenciesCount:  8,
			GraphDistance:      5,
			AmbiguityScore:     0.78,
			EvidenceCoverage:   0.40,
			ExpectedPatchLines: 350,
			TestComplexity:     0.90,
		}
		items = append(items, TaskMatrixItem{
			ID:                 fmt.Sprintf("T4-%02d", i+1),
			Name:               d.name,
			Class:              compute.T4Critical,
			Category:           d.cat,
			Query:              d.query,
			Features:           f,
			MeasuredDifficulty: d.diff,
			CanBypass:          false,
			SuccessOracle:      d.oracle,
			GroundTruthFiles:   d.files,
		})
	}

	return items
}

// SaveTaskMatrixJSONL writes the 120-task matrix to the designated path.
func SaveTaskMatrixJSONL(path string) (int, error) {
	items := BuildR15TaskMatrix()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return 0, err
	}

	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, item := range items {
		line, err := json.Marshal(item)
		if err != nil {
			return 0, err
		}
		if _, err := w.Write(append(line, '\n')); err != nil {
			return 0, err
		}
	}
	if err := w.Flush(); err != nil {
		return 0, err
	}

	return len(items), nil
}
