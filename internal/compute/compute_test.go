package compute

import (
	"testing"
)

func TestTaskProfilerClassification(t *testing.T) {
	profiler := NewTaskProfiler()

	// Case 1: Deterministic bypass
	p0 := profiler.Profile("where is func HandlePlan defined in server.go", 0)
	if !p0.CanBypass {
		t.Fatalf("expected CanBypass to be true for definition lookup")
	}
	if p0.Class != T0Deterministic {
		t.Fatalf("expected T0Deterministic, got %v", p0.Class)
	}

	// Case 2: Trivial task
	p1 := profiler.Profile("fix typo in readme title", 0)
	if p1.Difficulty >= 0.35 {
		t.Fatalf("expected low difficulty for trivial task, got %f", p1.Difficulty)
	}

	// Case 3: Difficult task
	p3 := profiler.Profile("investigate race condition and deadlock in concurrent session scheduler across auth.go and store.go", 0.5)
	if p3.Difficulty < 0.40 {
		t.Fatalf("expected high difficulty for concurrency bug, got %f", p3.Difficulty)
	}
	if p3.Class < T2Moderate {
		t.Fatalf("expected at least T2Moderate, got %v", p3.Class)
	}
}

func TestHierarchicalBudgetManagerReserves(t *testing.T) {
	policy := BudgetPolicy{
		MaxCostUSD:          1.00,
		MaxReasoningTokens:  10000,
		VerificationReserve: 0.20, // $0.20 reserved
		EscalationReserve:   0.20, // $0.20 reserved
	}
	bm := NewBudgetManager(policy)

	// Available for normal actions = 1.00 - 0.20 - 0.20 = $0.60
	// Normal action spending $0.50 should succeed
	if !bm.CanSpend(BudgetCategoryReasoning, 0.50, 2000) {
		t.Fatalf("expected normal action $0.50 to be allowed")
	}
	_ = bm.Spend(BudgetCategoryReasoning, 0.50, 2000, 1)

	// Another normal action of $0.20 should FAIL because it would invade reserves (0.50 + 0.20 = 0.70 > 0.60)
	if bm.CanSpend(BudgetCategoryReasoning, 0.20, 1000) {
		t.Fatalf("expected normal action $0.20 to be rejected to protect reserves")
	}

	// But verification spending $0.15 should succeed because verification reserve is unlocked for it
	if !bm.CanSpend(BudgetCategoryVerification, 0.15, 0) {
		t.Fatalf("expected verification action to access verification reserve")
	}
}

func TestEstimatorAndMarginalThinking(t *testing.T) {
	estimator := NewComputeEstimator(0.50)
	curve := estimator.EstimateCurve("anthropic", "claude-3-7-sonnet", 0.6)

	if len(curve) != 5 {
		t.Fatalf("expected 5 curve points, got %d", len(curve))
	}
	// Success rate should monotonically increase with effort
	for i := 0; i < len(curve)-1; i++ {
		if curve[i].EstimatedSuccess > curve[i+1].EstimatedSuccess {
			t.Fatalf("expected non-decreasing success rate, got %f > %f", curve[i].EstimatedSuccess, curve[i+1].EstimatedSuccess)
		}
	}

	recommended := estimator.RecommendOptimalEffort(curve)
	if recommended < EffortLow {
		t.Fatalf("expected at least EffortLow for 0.6 difficulty, got %v", recommended)
	}
}

func TestVOIEngineActions(t *testing.T) {
	voi := NewVOIEngine(DefaultUtilityWeights())

	budget := BudgetState{
		RemainingCostUSD:         1.00,
		RemainingReasoningTokens: 20000,
	}
	cache := CacheState{Active: false}

	// Scenario: Very low evidence coverage (0.2) -> RETRIEVE should beat THINK
	scores := voi.EvaluateActions(0.5, 0.3, 0.2, 0.6, cache, budget)
	var retrieveVOI, thinkVOI float64
	for _, s := range scores {
		if s.Action == ActionRetrieve {
			retrieveVOI = s.VOI
		}
		if s.Action == ActionThink {
			thinkVOI = s.VOI
		}
	}

	if retrieveVOI <= thinkVOI {
		t.Fatalf("expected RETRIEVE VOI (%f) > THINK VOI (%f) when evidence coverage is low", retrieveVOI, thinkVOI)
	}
}

func TestAdaptiveStoppingCriteria(t *testing.T) {
	stopping := NewAdaptiveStopping(0.05, 10)

	state := ControllerState{
		CalibratedRisk:   0.03, // Below target 0.05
		EvidenceCoverage: 0.95, // Above 0.85
		TurnCount:        3,
		RemainingBudget: BudgetState{
			RemainingCostUSD: 1.00,
		},
	}

	// When marginal VOI is non-positive, should stop
	stop, reason := stopping.ShouldStop(state, 0.0)
	if !stop {
		t.Fatalf("expected stopping when risk target met and VOI <= 0")
	}
	if reason == "" {
		t.Fatalf("expected non-empty stop reason")
	}
}

func TestModelRouterAndCascade(t *testing.T) {
	router := NewModelRouter()

	// Trivial task should pick cheap tier
	trivialModel := router.SelectOptimalModel(0.1, "")
	if trivialModel.Tier != "cheap" {
		t.Fatalf("expected cheap tier for trivial task, got %s", trivialModel.Tier)
	}

	// Critical task should avoid cheap tier
	criticalModel := router.SelectOptimalModel(0.85, "")
	if criticalModel.Tier == "cheap" {
		t.Fatalf("expected non-cheap tier for critical task")
	}

	// Cascade test
	cascade := router.BuildCascade(0.2)
	if cascade.InitialModel.Tier != "cheap" {
		t.Fatalf("expected cheap initial model in cascade, got %s", cascade.InitialModel.Tier)
	}
	if cascade.EscalateModel == nil {
		t.Fatalf("expected escalate model in cascade")
	}
}

func TestAdaptiveControllerStep(t *testing.T) {
	controller := NewAdaptiveController()

	state := ControllerState{
		Difficulty:          0.6,
		Risk:                0.3,
		EvidenceCoverage:    0.3, // Needs retrieval
		EstimatedConfidence: 0.5,
		TurnCount:           1,
		RemainingBudget: BudgetState{
			RemainingCostUSD:         1.50,
			RemainingReasoningTokens: 25000,
		},
	}

	res := controller.Step(state, DefaultPolicyForTask(T2Moderate, 0.05), EffortLow)
	if res.Action != ActionRetrieve {
		t.Fatalf("expected controller to choose ActionRetrieve when coverage is 0.3, got %v", res.Action)
	}
}
