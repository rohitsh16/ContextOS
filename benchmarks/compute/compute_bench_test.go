package compute_bench

import (
	"fmt"
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
