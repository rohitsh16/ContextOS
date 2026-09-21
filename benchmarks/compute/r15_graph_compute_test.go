package compute_bench

import (
	"testing"
)

func TestExtendedR15Benchmark(t *testing.T) {
	outDir := "../results/r15"
	summary, err := RunExtendedR15Benchmark(outDir)
	if err != nil {
		t.Fatalf("RunExtendedR15Benchmark failed: %v", err)
	}

	if summary.TotalTasks != 120 {
		t.Errorf("expected 120 tasks, got %d", summary.TotalTasks)
	}

	if summary.EmpiricalErrorRate > summary.ConstraintEpsilon {
		t.Errorf("empirical error rate %.3f exceeded constraint %.3f", summary.EmpiricalErrorRate, summary.ConstraintEpsilon)
	}

	if len(summary.EngineBenchmarks) != 3 {
		t.Errorf("expected 3 engine benchmarks, got %d", len(summary.EngineBenchmarks))
	}

	t.Logf("Extended R15 Mean Cost: $%.5f (vs Standard R15: $%.5f, Cut: %.2f%%)",
		summary.MeanTotalCost, summary.StandardR15MeanCost, summary.GraphOptimizedCutPct)
	t.Logf("Empirical Error Rate: %.2f%% (Constraint: <= %.2f%%)",
		summary.EmpiricalErrorRate*100, summary.ConstraintEpsilon*100)
}
