package allocator

import (
	"testing"

	"contextos/internal/model"
)

func TestAdaptiveBudgeting(t *testing.T) {
	// 1. Task classification
	simple := ClassifyTaskRisk("Simple lookup of config", 0)
	if simple.TaskType != "simple_lookup" || simple.MinBudget != 256 {
		t.Fatalf("expected simple_lookup, got %+v", simple)
	}

	complexTask := ClassifyTaskRisk("Migrate billing architecture to event-driven outbox", 20)
	if complexTask.TaskType != "architectural_migration" || complexTask.MaxBudget != 8192 {
		t.Fatalf("expected architectural_migration, got %+v", complexTask)
	}

	// 2. Optimal budget calculation
	bSimple := CalculateOptimalBudget(simple, 0.1)
	bComplex := CalculateOptimalBudget(complexTask, 0.8)
	if bSimple >= bComplex {
		t.Fatalf("expected complex budget > simple budget, got simple=%d, complex=%d", bSimple, bComplex)
	}

	// 3. Adaptive packing with early stopping
	cands := []model.Candidate{
		{ID: "c1", Kind: "decision", Tokens: 100, Confidence: 0.95},
		{ID: "c2", Kind: "code", Tokens: 1000, Confidence: 0.10}, // Low marginal utility
	}
	selected, dec := AdaptiveBudgetPacker(cands, simple, 0.1)
	if len(selected) == 0 {
		t.Fatalf("expected at least 1 candidate packed")
	}
	if dec.OptimalBudget <= 0 {
		t.Fatalf("expected positive optimal budget")
	}

	// 4. Suite evaluation
	tasks := []string{"fix bug in handler", "refactor database connection pool"}
	analysis := EvaluateAdaptiveBudgetSuite(tasks, 4096)
	if analysis.Status != "GREEN" {
		t.Fatalf("expected adaptive budget status GREEN, got %s", analysis.Status)
	}
}
