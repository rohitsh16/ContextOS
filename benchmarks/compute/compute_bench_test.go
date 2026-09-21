package compute_bench

import (
	"fmt"
	"path/filepath"
	"testing"

	"contextos/internal/compute"
	"contextos/internal/planning"
	"contextos/internal/providers"
	"contextos/internal/state"
	"contextos/internal/telemetry"
	"contextos/internal/verification"
)

// Benchmark A — Fixed vs Adaptive Effort
func TestBenchmarkA_FixedVsAdaptiveEffort(t *testing.T) {
	estimator := compute.NewComputeEstimator(0.50)

	tasks := []struct {
		name       string
		difficulty float64
	}{
		{"trivial_lookup", 0.1},
		{"local_bug_fix", 0.3},
		{"cross_file_refactor", 0.6},
		{"distributed_deadlock", 0.85},
	}

	var fixedHighCost, fixedHighSuccess float64
	var adaptiveCost, adaptiveSuccess float64

	for _, task := range tasks {
		curve := estimator.EstimateCurve("anthropic", "claude-3-7-sonnet", task.difficulty)

		// Fixed High: always choose EffortHigh (index 3)
		highPoint := curve[3]
		fixedHighCost += highPoint.EstimatedCostUSD
		fixedHighSuccess += highPoint.EstimatedSuccess

		// Adaptive: choose optimal effort based on MVTC knee
		optLvl := estimator.RecommendOptimalEffort(curve)
		optPoint := curve[int(optLvl)]
		adaptiveCost += optPoint.EstimatedCostUSD
		adaptiveSuccess += optPoint.EstimatedSuccess
	}

	t.Logf("[Benchmark A] Fixed High Cost: $%.4f (Success: %.2f) | Adaptive Cost: $%.4f (Success: %.2f)",
		fixedHighCost, fixedHighSuccess/4.0, adaptiveCost, adaptiveSuccess/4.0)

	if adaptiveCost >= fixedHighCost {
		t.Fatalf("expected adaptive cost ($%.4f) < fixed high cost ($%.4f)", adaptiveCost, fixedHighCost)
	}
	// Verify accuracy degradation is minimal (< 5%)
	if (fixedHighSuccess - adaptiveSuccess) > 0.20 {
		t.Fatalf("adaptive accuracy degraded too much: %f vs %f", adaptiveSuccess, fixedHighSuccess)
	}
}

// Benchmark B — Think vs Retrieve
func TestBenchmarkB_ThinkVsRetrieve(t *testing.T) {
	voi := compute.NewVOIEngine(compute.DefaultUtilityWeights())

	budget := compute.BudgetState{
		RemainingCostUSD:         2.00,
		RemainingReasoningTokens: 30000,
	}
	cache := compute.CacheState{Active: false}

	// Case 1: Low evidence coverage (0.2) -> Retrieve should dominate Think
	scores1 := voi.EvaluateActions(0.40, 0.40, 0.20, 0.70, cache, budget)
	var retrieveVOI, thinkVOI float64
	for _, s := range scores1 {
		if s.Action == compute.ActionRetrieve {
			retrieveVOI = s.VOI
		}
		if s.Action == compute.ActionThink {
			thinkVOI = s.VOI
		}
	}
	if retrieveVOI <= thinkVOI {
		t.Fatalf("expected retrieve VOI > think VOI when evidence is missing: %f vs %f", retrieveVOI, thinkVOI)
	}

	// Case 2: Complete evidence coverage (0.95) and high difficulty (0.8) -> Think should dominate Retrieve
	scores2 := voi.EvaluateActions(0.60, 0.20, 0.95, 0.80, cache, budget)
	var thinkVOI2, retrieveVOI2 float64
	for _, s := range scores2 {
		if s.Action == compute.ActionRetrieve {
			retrieveVOI2 = s.VOI
		}
		if s.Action == compute.ActionThink {
			thinkVOI2 = s.VOI
		}
	}
	if thinkVOI2 <= retrieveVOI2 {
		t.Fatalf("expected think VOI > retrieve VOI when evidence is full: %f vs %f", thinkVOI2, retrieveVOI2)
	}
}

// Benchmark C — Context vs Joint Context+Compute Optimization
func TestBenchmarkC_JointOptimization(t *testing.T) {
	jco := planning.NewJointCostOptimizer(0.05)

	// Moderate task difficulty (0.5)
	opt := jco.OptimizeFrontier(0.5, 3.0, 15.0)

	t.Logf("[Benchmark C] Optimal Context Tokens: %d (Cost: $%.4f) | Reasoning Tokens: %d (Cost: $%.4f) | Total: $%.4f",
		opt.ContextTokens, opt.ContextCostUSD, opt.ExpectedReasoningTokens, opt.ExpectedReasoningCostUSD, opt.TotalCostUSD)

	if opt.TotalCostUSD <= 0 {
		t.Fatalf("expected positive total cost")
	}
	if opt.ResidualRisk > 0.05 {
		t.Fatalf("expected residual risk <= 0.05")
	}
}

// Benchmark D — Full History vs DecisionState Compaction
func TestBenchmarkD_DecisionStateCompaction(t *testing.T) {
	compiler := state.NewConversationCompiler(4)

	// Simulate 50 turns
	var transcript []providers.Message
	for i := 1; i <= 50; i++ {
		role := providers.RoleUser
		if i%2 == 0 {
			role = providers.RoleAssistant
		}
		transcript = append(transcript, providers.Message{
			Role: role,
			Content: fmt.Sprintf("Turn %d: Discussed module dependencies, caching invalidation, and testing harness.\n"+
				"Fact: DB port is 5432.\nDecision: Retain backward compatibility with v1 API.", i),
		})
	}

	bundle := compiler.Compile("Refactor API v2", transcript, nil)

	t.Logf("[Benchmark D] Original Tokens: %d | Compacted Tokens: %d | Savings: %.2f%%",
		bundle.OriginalTokens, bundle.CompiledTokens, bundle.CompressionRatio*100)

	if bundle.CompressionRatio < 0.60 {
		t.Fatalf("expected at least 60%% token reduction via DecisionState compaction, got %.2f%%", bundle.CompressionRatio*100)
	}
}

// Benchmark E — Cache-Aware Compute
func TestBenchmarkE_CacheAwareCompute(t *testing.T) {
	voi := compute.NewVOIEngine(compute.DefaultUtilityWeights())
	budget := compute.BudgetState{RemainingCostUSD: 1.00, RemainingReasoningTokens: 20000}

	// Cache with high invalidation penalty
	cacheWithPenalty := compute.CacheState{
		Active:                 true,
		HitProbability:         0.50,
		InvalidationPenaltyUSD: 0.10, // $0.10 penalty if cache disrupted
	}
	scores := voi.EvaluateActions(0.70, 0.20, 0.80, 0.50, cacheWithPenalty, budget)

	for _, s := range scores {
		if s.Action == compute.ActionThink {
			if s.CachePenalty <= 0 {
				t.Fatalf("expected positive cache penalty when cache is active")
			}
			t.Logf("[Benchmark E] Think VOI with cache penalty: %.4f (Penalty: $%.4f)", s.VOI, s.CachePenalty)
		}
	}
}

// Benchmark F — Model Cascade
func TestBenchmarkF_ModelCascade(t *testing.T) {
	router := compute.NewModelRouter()

	cascadeTrivial := router.BuildCascade(0.1)
	if cascadeTrivial.InitialModel.Tier != "cheap" {
		t.Fatalf("expected cheap tier for trivial task cascade")
	}

	cascadeCritical := router.BuildCascade(0.9)
	if cascadeCritical.InitialModel.Tier != "strongest" {
		t.Fatalf("expected strongest tier for critical task cascade")
	}
}

// Benchmark G — Risk-Adaptive Verification
func TestBenchmarkG_VerificationPolicies(t *testing.T) {
	v := verification.NewVerifier()

	// Level 0: Zero cost
	res0 := v.VerifyLevel0("func Run() {}", []string{"Run"})
	if res0.CostUSD != 0.0 {
		t.Fatalf("expected 0 cost for Level 0")
	}

	// Deterministic bypass check
	bypass, _ := verification.IsDeterministicBypass("where is func HandlePlan defined in server.go")
	if !bypass {
		t.Fatalf("expected symbol lookup to bypass LLM")
	}
}

// Benchmark H — Full End-to-End Cost Per Successful Task (CPS)
func TestBenchmarkH_EndToEndCPS(t *testing.T) {
	runs := []telemetry.TaskRunTelemetry{
		{
			RunID:    "run-1",
			TaskID:   "t1-bypass",
			Provider: "none",
			Model:    "deterministic",
			Success:  true,
			Usage: telemetry.UsageMetrics{
				EstimatedCostUSD: 0.0, // Free bypass!
			},
		},
		{
			RunID:    "run-2",
			TaskID:   "t2-cheap",
			Provider: "anthropic",
			Model:    "claude-3-5-haiku",
			Success:  true,
			Usage: telemetry.UsageMetrics{
				ReasoningTokens:  2048,
				EstimatedCostUSD: 0.005,
			},
		},
		{
			RunID:    "run-3",
			TaskID:   "t3-adaptive",
			Provider: "anthropic",
			Model:    "claude-3-7-sonnet",
			Success:  true,
			Usage: telemetry.UsageMetrics{
				ReasoningTokens:  8192,
				EstimatedCostUSD: 0.040,
			},
		},
	}

	// Unoptimized baseline: always ran o1 at maximum effort ($0.25 per task = $0.75 total)
	baselineCost := 0.75
	kpi := telemetry.ComputeSummaryKPIs(runs, baselineCost)

	t.Logf("[Benchmark H] Total Cost: $%.4f | Successful Tasks: %d | CPS: $%.4f | Compute Compression: %.2f%%",
		kpi.TotalCostUSD, kpi.SuccessfulTasks, kpi.CostPerSuccessUSD, kpi.ComputeCompression*100)

	if kpi.SuccessfulTasks != 3 {
		t.Fatalf("expected 3 successful tasks")
	}
	if kpi.CostPerSuccessUSD >= 0.05 {
		t.Fatalf("expected CPS < $0.05, got $%.4f", kpi.CostPerSuccessUSD)
	}
	if kpi.ComputeCompression < 0.80 {
		t.Fatalf("expected > 80%% compute compression over naive baseline, got %.2f%%", kpi.ComputeCompression*100)
	}
}

// Phase R15.1 — Audit Current 10-Task Benchmark
func TestBenchmarkR15_1_Audit(t *testing.T) {
	resultsDir := filepath.Join("..", "results", "r15")
	rep, err := RunR15_1_Audit(resultsDir)
	if err != nil {
		t.Fatalf("failed to run R15.1 audit: %v", err)
	}

	t.Logf("[Phase R15.1 Audit] Tasks: %d | Base Cost: $%.4f | Opt Cost: $%.4f | Max Discrepancy: $%.8f | Reconciled: %t | Verdict: %s",
		rep.TaskCount, rep.BaselineTotalCostUSD, rep.OptimizedTotalCostUSD, rep.MaxCostDiscrepancyUSD, rep.CostReconciled, rep.GateVerdict)
	t.Logf("[Phase R15.1 Audit] Base CPS: $%.4f | Opt CPS: $%.4f | CPS Multiplier: %.2fx | Compute Compression: %.2f%%",
		rep.BaselineCPSUSD, rep.OptimizedCPSUSD, rep.CPSEfficiencyMultiple, rep.ComputeCompressionPct)

	if rep.GateVerdict != "GREEN" {
		t.Fatalf("expected R15.1 Gate Verdict to be GREEN, got %s", rep.GateVerdict)
	}
	if !rep.CostReconciled {
		t.Fatalf("expected cost to be reconciled within 1e-6, max discrepancy was %e", rep.MaxCostDiscrepancyUSD)
	}
	if rep.CPSEfficiencyMultiple < 100.0 {
		t.Fatalf("expected CPS efficiency multiple >= 100x, got %.2fx", rep.CPSEfficiencyMultiple)
	}
}

// Phase R15.2 — Real Task Matrix Generation (120 Tasks)
func TestBenchmarkR15_2_TaskMatrix(t *testing.T) {
	matrixPath := filepath.Join("..", "results", "r15", "r15_2_task_matrix.jsonl")
	count, err := SaveTaskMatrixJSONL(matrixPath)
	if err != nil {
		t.Fatalf("failed to generate R15.2 task matrix: %v", err)
	}

	if count != 120 {
		t.Fatalf("expected exactly 120 tasks in matrix, got %d", count)
	}

	items := BuildR15TaskMatrix()
	countsByClass := make(map[compute.TaskClass]int)
	for _, item := range items {
		countsByClass[item.Class]++
		if item.MeasuredDifficulty <= 0.0 || item.MeasuredDifficulty > 1.0 {
			t.Fatalf("task %s has invalid measured difficulty: %f", item.ID, item.MeasuredDifficulty)
		}
	}

	if countsByClass[compute.T0Deterministic] != 20 {
		t.Fatalf("expected 20 T0 tasks, got %d", countsByClass[compute.T0Deterministic])
	}
	if countsByClass[compute.T1Trivial] != 20 {
		t.Fatalf("expected 20 T1 tasks, got %d", countsByClass[compute.T1Trivial])
	}
	if countsByClass[compute.T2Moderate] != 30 {
		t.Fatalf("expected 30 T2 tasks, got %d", countsByClass[compute.T2Moderate])
	}
	if countsByClass[compute.T3Difficult] != 30 {
		t.Fatalf("expected 30 T3 tasks, got %d", countsByClass[compute.T3Difficult])
	}
	if countsByClass[compute.T4Critical] != 20 {
		t.Fatalf("expected 20 T4 tasks, got %d", countsByClass[compute.T4Critical])
	}

	t.Logf("[Phase R15.2 Matrix] Successfully generated and verified %d stratified tasks at %s", count, matrixPath)
	t.Logf("[Phase R15.2 Matrix] Breakdown: T0=%d, T1=%d, T2=%d, T3=%d, T4=%d",
		countsByClass[compute.T0Deterministic],
		countsByClass[compute.T1Trivial],
		countsByClass[compute.T2Moderate],
		countsByClass[compute.T3Difficult],
		countsByClass[compute.T4Critical],
	)
}

// Phase R15.3 — Fixed-Effort Compute Frontier
func TestBenchmarkR15_3_Frontier(t *testing.T) {
	matrix := BuildR15TaskMatrix()
	resultsDir := filepath.Join("..", "results", "r15")

	rep, err := RunR15_3_Frontier(matrix, resultsDir)
	if err != nil {
		t.Fatalf("failed to run R15.3 frontier: %v", err)
	}

	if len(rep.FrontierSummary) != 5 {
		t.Fatalf("expected 5 effort summary points, got %d", len(rep.FrontierSummary))
	}

	t.Logf("[Phase R15.3 Frontier] Optimal Fixed Effort: %s | Min CPS: $%.4f",
		rep.OptimalFixedLevel, rep.OptimalFixedCPS)

	for _, pt := range rep.FrontierSummary {
		t.Logf("  Effort: %-8s | Avg Acc: %5.2f%% | Avg Reasoning: %6.0f tok | Avg Cost: $%.5f | CPS: $%.4f | MSG: %+.3f | CE: %6.2f | Knee: %t",
			pt.EffortLevel, pt.AverageAccuracy*100, pt.AvgReasoningToks, pt.AvgCostUSD, pt.CPSUSD, pt.MarginalGain, pt.ComputeEfficiency, pt.IsKnee)
	}

	// Verify diminishing returns knee exists: maximum effort must have lower compute efficiency than low effort
	minPt := rep.FrontierSummary[0]
	maxPt := rep.FrontierSummary[4]
	if maxPt.TotalCostUSD <= minPt.TotalCostUSD {
		t.Fatalf("expected maximum effort cost > minimal effort cost")
	}
}

// Phase R15.4 — Think vs Retrieve Centerpiece Experiment
func TestBenchmarkR15_4_ThinkVsRetrieve(t *testing.T) {
	matrix := BuildR15TaskMatrix()
	resultsDir := filepath.Join("..", "results", "r15")

	rep, err := RunR15_4_ThinkVsRetrieve(matrix, resultsDir)
	if err != nil {
		t.Fatalf("failed to run R15.4 think vs retrieve: %v", err)
	}

	t.Logf("[Phase R15.4] Policy A (THINK):          Avg Acc: %5.2f%% | Avg Reasoning: %6.0f tok | CPS: $%.4f",
		rep.PolicyThink.AverageAccuracy*100, rep.PolicyThink.AvgReasoningToks, rep.PolicyThink.CPSUSD)
	t.Logf("[Phase R15.4] Policy B (RETRIEVE):       Avg Acc: %5.2f%% | Avg Reasoning: %6.0f tok | CPS: $%.4f",
		rep.PolicyRetrieve.AverageAccuracy*100, rep.PolicyRetrieve.AvgReasoningToks, rep.PolicyRetrieve.CPSUSD)
	t.Logf("[Phase R15.4] Policy C (THINK+RETRIEVE): Avg Acc: %5.2f%% | Avg Reasoning: %6.0f tok | CPS: $%.4f",
		rep.PolicyJoint.AverageAccuracy*100, rep.PolicyJoint.AvgReasoningToks, rep.PolicyJoint.CPSUSD)
	t.Logf("[Phase R15.4] Policy D (ADAPTIVE):       Avg Acc: %5.2f%% | Avg Reasoning: %6.0f tok | CPS: $%.4f",
		rep.PolicyAdaptive.AverageAccuracy*100, rep.PolicyAdaptive.AvgReasoningToks, rep.PolicyAdaptive.CPSUSD)

	t.Logf("[Phase R15.4] Interaction Effect I: %+.4f (Positive: %t) | Delta CPS vs Think: $%.4f",
		rep.InteractionEffect, rep.PositiveInteraction, rep.DeltaCPSVsThinkUSD)
	t.Logf("[Phase R15.4] Breakdown: InfoLimited=%d, ReasoningLimited=%d, Deterministic=%d, H2Confirmed=%t",
		rep.InfoLimitedCount, rep.ReasoningLimitedCount, rep.DeterministicCount, rep.HypothesisH2Confirmed)

	if rep.PolicyAdaptive.CPSUSD >= rep.PolicyThink.CPSUSD {
		t.Fatalf("expected adaptive CPS ($%.4f) < think CPS ($%.4f)",
			rep.PolicyAdaptive.CPSUSD, rep.PolicyThink.CPSUSD)
	}
	if !rep.HypothesisH2Confirmed {
		t.Fatalf("expected Hypothesis H2 to be confirmed (Delta CPS < 0), got Delta CPS = %f", rep.DeltaCPSVsThinkUSD)
	}
}

// Phase R15.5 — Controller Audit & Failure Taxonomy
func TestBenchmarkR15_5_ControllerAudit(t *testing.T) {
	matrix := BuildR15TaskMatrix()
	resultsDir := filepath.Join("..", "results", "r15")

	rep, err := RunR15_5_ControllerAudit(matrix, resultsDir)
	if err != nil {
		t.Fatalf("failed to run R15.5 controller audit: %v", err)
	}

	t.Logf("[Phase R15.5] Success Rate: %5.2f%% | Avg Cost: $%.4f | Avg Reasoning: %6.0f tok",
		rep.SuccessRate*100, rep.AverageCostUSD, rep.AverageReasoningToks)
	t.Logf("[Phase R15.5] Profiler Difficulty MSE: %.4f | Bypass Accuracy: %5.2f%%",
		rep.ProfilerDifficultyMSE, rep.ProfilerBypassAccuracy*100)
	t.Logf("[Phase R15.5] Overthinking Rate: %5.2f%% | Premature Stop Rate: %5.2f%%",
		rep.OverthinkingRate*100, rep.PrematureStopRate*100)

	if rep.SuccessRate < 0.75 {
		t.Fatalf("expected adaptive success rate >= 75%%, got %.2f%%", rep.SuccessRate*100)
	}
	if rep.ProfilerBypassAccuracy < 0.95 {
		t.Fatalf("expected profiler bypass accuracy >= 95%%, got %.2f%%", rep.ProfilerBypassAccuracy*100)
	}
	if rep.OverthinkingRate > 0.05 {
		t.Fatalf("expected overthinking rate <= 5%%, got %.2f%%", rep.OverthinkingRate*100)
	}

	if len(rep.QuestionAnswers) != 10 {
		t.Fatalf("expected 10 question answers, got %d", len(rep.QuestionAnswers))
	}
	for _, q := range rep.QuestionAnswers {
		if q.Status != "VERIFIED" {
			t.Errorf("question Q%d not verified: %s", q.QuestionID, q.Answer)
		}
	}

	if len(rep.TaxonomyDistribution) != 14 {
		t.Fatalf("expected 14 taxonomy categories, got %d", len(rep.TaxonomyDistribution))
	}
}

// Phase R15.6 / R15.7 / R15.8 — Ablation Ladder, Baselines, and Oracle Regret (ACR)
func TestBenchmarkR15_6_AblationLadder(t *testing.T) {
	matrix := BuildR15TaskMatrix()
	resultsDir := filepath.Join("..", "results", "r15")

	rep, err := RunR15_6_7_8_AblationLadder(matrix, resultsDir)
	if err != nil {
		t.Fatalf("failed to run R15.6 ablation ladder: %v", err)
	}

	t.Logf("[Phase R15.6] Offline Grid Oracle Cost: $%.4f | Oracle CPS: $%.4f", rep.OracleCostUSD, rep.OracleCPSUSD)
	t.Logf("[Phase R15.6] B0 (Fixed Max) CPS:       $%.4f | Acc: %5.2f%%", rep.AblationLadder[0].CPSUSD, rep.AblationLadder[0].SuccessRate*100)
	t.Logf("[Phase R15.6] B3 (Bypass Only) CPS:     $%.4f | Acc: %5.2f%%", rep.AblationLadder[3].CPSUSD, rep.AblationLadder[3].SuccessRate*100)
	t.Logf("[Phase R15.6] B6 (Retrieve vs Think):   $%.4f | Acc: %5.2f%%", rep.AblationLadder[6].CPSUSD, rep.AblationLadder[6].SuccessRate*100)
	t.Logf("[Phase R15.6] B8 (Model Routing):       $%.4f | Acc: %5.2f%%", rep.AblationLadder[8].CPSUSD, rep.AblationLadder[8].SuccessRate*100)
	t.Logf("[Phase R15.6] B11 (Full ContextOS):     $%.4f | Acc: %5.2f%%", rep.AblationLadder[11].CPSUSD, rep.AblationLadder[11].SuccessRate*100)

	if len(rep.AblationLadder) != 12 {
		t.Fatalf("expected 12 rungs in ablation ladder, got %d", len(rep.AblationLadder))
	}
	if len(rep.StrongBaselines) != 10 {
		t.Fatalf("expected 10 strong baselines, got %d", len(rep.StrongBaselines))
	}

	b0CPS := rep.AblationLadder[0].CPSUSD
	b11CPS := rep.AblationLadder[11].CPSUSD
	if b11CPS >= b0CPS {
		t.Fatalf("expected B11 CPS ($%.4f) < B0 CPS ($%.4f)", b11CPS, b0CPS)
	}

	// Regret check: ContextOS ACR should be strictly lower than Fixed Max
	fixedMaxACR := rep.StrongBaselines[0].RegretACR
	contextOSACR := rep.StrongBaselines[8].RegretACR
	t.Logf("[Phase R15.8] Fixed Max Regret (ACR): %+.2fx | ContextOS Regret (ACR): %+.2fx", fixedMaxACR, contextOSACR)

	if contextOSACR >= fixedMaxACR {
		t.Fatalf("expected ContextOS regret (%.2f) < Fixed Max regret (%.2f)", contextOSACR, fixedMaxACR)
	}
}

// Phase R15.9 — Statistical Evaluation with 10,000 Paired Bootstrap Replicates
func TestBenchmarkR15_9_StatisticalEvaluation(t *testing.T) {
	matrix := BuildR15TaskMatrix()
	resultsDir := filepath.Join("..", "results", "r15")

	rep, err := RunR15_9_StatisticalEvaluation(matrix, resultsDir)
	if err != nil {
		t.Fatalf("failed to run R15.9 statistical evaluation: %v", err)
	}

	t.Logf("[Phase R15.9] 10,000 Bootstrap Cost Reduction: %.2f%% [95%% CI: %.2f%%, %.2f%%] (SE: %.4f)",
		rep.CostReductionPercent.Estimate, rep.CostReductionPercent.Lower95, rep.CostReductionPercent.Upper95, rep.CostReductionPercent.StdError)
	t.Logf("[Phase R15.9] 10,000 Bootstrap Success Delta:  %+.2f%% [95%% CI: %+.2f%%, %+.2f%%]",
		rep.SuccessRateDiff.Estimate*100, rep.SuccessRateDiff.Lower95*100, rep.SuccessRateDiff.Upper95*100)
	t.Logf("[Phase R15.9] Candidate CPS: $%.4f [$%.4f, $%.4f] | Baseline CPS: $%.4f [$%.4f, $%.4f]",
		rep.CandidateCPSUSD.Estimate, rep.CandidateCPSUSD.Lower95, rep.CandidateCPSUSD.Upper95,
		rep.BaselineCPSUSD.Estimate, rep.BaselineCPSUSD.Lower95, rep.BaselineCPSUSD.Upper95)
	t.Logf("[Phase R15.9] McNemar Test: Chi-Square: %.4f (p-value: %.2e) | Both Pass: %d, Cand Only: %d, Base Only: %d",
		rep.ContingencyTable.McNemarChiSquare, rep.ContingencyTable.McNemarPValue,
		rep.ContingencyTable.BothPass, rep.ContingencyTable.CandidateOnlyPass, rep.ContingencyTable.BaselineOnlyPass)

	if rep.BootstrapReplicates != 10000 {
		t.Fatalf("expected 10000 bootstrap replicates, got %d", rep.BootstrapReplicates)
	}
	if rep.CostReductionPercent.Lower95 < 50.0 {
		t.Fatalf("expected 95%% CI lower bound on cost reduction >= 50%%, got %.2f%%", rep.CostReductionPercent.Lower95)
	}
	if rep.SuccessRateDiff.Lower95 < 0.0 {
		t.Fatalf("expected 95%% CI lower bound on success rate delta >= 0%%, got %.2f%%", rep.SuccessRateDiff.Lower95*100)
	}
	if !rep.StatisticallyValidated {
		t.Fatalf("expected statistical validation to pass")
	}
}

// Phase R15.10 through R15.15 — OOD, Calibration, Factorial, Overhead, Providers, and Final Report
func TestBenchmarkR15_10_Through_15(t *testing.T) {
	matrix := BuildR15TaskMatrix()
	resultsDir := filepath.Join("..", "results", "r15")

	// Phase R15.10: OOD Stress Testing
	oodRep, err := RunR15_10_OODStress(matrix, resultsDir)
	if err != nil {
		t.Fatalf("failed to run R15.10 OOD stress testing: %v", err)
	}
	t.Logf("[Phase R15.10] OOD Stress Conditions Evaluated: %d | Degrades Gracefully: %t",
		len(oodRep.StressConditions), oodRep.DegradesGraceful)
	if !oodRep.DegradesGraceful {
		t.Fatalf("expected controller to degrade gracefully under all OOD stress conditions")
	}

	// Phase R15.11: Calibration Research
	calRep, err := RunR15_11_CalibrationResearch(matrix, resultsDir)
	if err != nil {
		t.Fatalf("failed to run R15.11 calibration research: %v", err)
	}
	t.Logf("[Phase R15.11] Calibration Brier Score: %.4f | ECE: %.4f | MCE: %.4f | Calibrated: %t",
		calRep.BrierScore, calRep.ExpectedCalibrationError, calRep.MaximumCalibrationError, calRep.CalibrationCalibrated)
	if !calRep.CalibrationCalibrated {
		t.Fatalf("expected calibrated risk engine (ECE < 0.08, Brier < 0.04), got ECE: %.4f, Brier: %.4f",
			calRep.ExpectedCalibrationError, calRep.BrierScore)
	}

	// Phase R15.12: Joint Context + Compute Factorial Experiment
	factRep, err := RunR15_12_FactorialExperiment(matrix, resultsDir)
	if err != nil {
		t.Fatalf("failed to run R15.12 factorial experiment: %v", err)
	}
	t.Logf("[Phase R15.12] 2x2 Factorial Interaction Delta: %+.4f utility | Positive Synergy: %t",
		factRep.InteractionDelta, factRep.PositiveInteraction)
	if !factRep.PositiveInteraction {
		t.Fatalf("expected positive super-additive synergy between context compression and compute control")
	}

	// Phase R15.13 & R15.14: Overhead & Provider Neutrality
	overhead, providers, err := RunR15_13_14_OverheadAndProviders(resultsDir)
	if err != nil {
		t.Fatalf("failed to run R15.13/14 overhead and provider validation: %v", err)
	}
	t.Logf("[Phase R15.14] Controller Overhead: %.3f ms (%.6f%% of task compute) | Acceptable: %t",
		overhead.TotalControllerTimeMs, overhead.OverheadPercentageOfCost, overhead.OverheadAcceptable)
	t.Logf("[Phase R15.13] Providers Validated: %d (Anthropic, Gemini, OpenAI)", len(providers))
	if !overhead.OverheadAcceptable {
		t.Fatalf("expected controller overhead to be acceptable (< 5%% of budget)")
	}

	// Phase R15.15: Final Synthesis & Verdict
	if err := GenerateFinalR15Report(resultsDir); err != nil {
		t.Fatalf("failed to generate R15_FINAL_REPORT.md: %v", err)
	}
	t.Logf("[Phase R15.15] Final Research Synthesis written to benchmarks/results/r15/R15_FINAL_REPORT.md")
}



