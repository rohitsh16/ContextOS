package planning

import (
	"testing"

	"contextos/internal/compute"
	"contextos/internal/model"
	"contextos/internal/state"
	"contextos/internal/verification"
)

func TestJointCostOptimization(t *testing.T) {
	optimizer := NewJointCostOptimizer(0.05)

	// Test case: Claude pricing ($3/M input, $15/M reasoning), moderate difficulty 0.4
	opt := optimizer.OptimizeFrontier(0.4, 3.0, 15.0)

	if opt.ContextTokens <= 0 {
		t.Fatalf("expected positive context tokens, got %d", opt.ContextTokens)
	}
	if opt.TotalCostUSD <= 0 {
		t.Fatalf("expected positive total cost, got %f", opt.TotalCostUSD)
	}
	if opt.ResidualRisk > 0.05 {
		t.Fatalf("expected residual risk <= 0.05, got %f", opt.ResidualRisk)
	}
}

func TestComputePlannerDeterministicBypass(t *testing.T) {
	planner := NewComputePlanner()

	plan := planner.Generate("where is func HandlePlan defined in server.go", 0.05, "")
	if !plan.CanBypass {
		t.Fatalf("expected CanBypass to be true for definition lookup")
	}
	if plan.TaskClass != compute.T0Deterministic {
		t.Fatalf("expected T0Deterministic, got %v", plan.TaskClass)
	}
	if plan.Policy.Effort != compute.EffortMinimal {
		t.Fatalf("expected EffortMinimal for bypassed task, got %v", plan.Policy.Effort)
	}
	if plan.EstimatedCostUSD != 0.0 {
		t.Fatalf("expected $0.0 estimated cost for bypassed task, got %f", plan.EstimatedCostUSD)
	}
}

func TestComputePlannerComplexTask(t *testing.T) {
	planner := NewComputePlanner()

	plan := planner.Generate("refactor distributed consensus engine to prevent deadlock", 0.05, "")
	if plan.CanBypass {
		t.Fatalf("expected CanBypass to be false for complex refactor")
	}
	if plan.TaskClass < compute.T2Moderate {
		t.Fatalf("expected at least T2Moderate for distributed refactor, got %v", plan.TaskClass)
	}
	if plan.Policy.Effort < compute.EffortMedium {
		t.Fatalf("expected at least EffortMedium for difficult task, got %v", plan.Policy.Effort)
	}
	if plan.Budget.MaxCostUSD <= 0 {
		t.Fatalf("expected positive budget")
	}
}

func TestBuildExecutionPlan(t *testing.T) {
	planner := NewComputePlanner()
	cPlan := planner.Generate("investigate performance bottleneck in store", 0.05, "")

	ctxPlan := model.ContextPlan{
		Task:           "investigate performance bottleneck in store",
		Budget:         4000,
		SelectedTokens: 1200,
		EstimatedCost:  0.005,
	}

	ds := state.NewDecisionState("investigate performance bottleneck in store")
	ds.Risk = 0.50

	execPlan := BuildExecutionPlan("investigate performance bottleneck in store", ctxPlan, cPlan, ds)

	if execPlan.VerificationLevel < verification.Level2EvidenceConsistency {
		t.Fatalf("expected at least Level 2 verification for 0.5 risk, got %v", execPlan.VerificationLevel)
	}
	expectedCost := ctxPlan.EstimatedCost + cPlan.EstimatedCostUSD
	if execPlan.TotalEstimatedCostUSD != expectedCost {
		t.Fatalf("expected total cost %f, got %f", expectedCost, execPlan.TotalEstimatedCostUSD)
	}
}
