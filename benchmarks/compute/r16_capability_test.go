package compute_bench

import (
	"os"
	"path/filepath"
	"testing"

	"contextos/internal/providers"
)

func TestR16CapabilityBenchmark(t *testing.T) {
	matrix := BuildR15TaskMatrix()
	if len(matrix) == 0 {
		t.Fatalf("expected non-empty task matrix")
	}

	resultsDir := filepath.Join("..", "results", "r16")
	_ = os.MkdirAll(resultsDir, 0755)

	report, err := RunR16CapabilityBenchmark(nil, providers.RunTypeSynthetic, resultsDir)
	if err != nil {
		t.Fatalf("RunR16CapabilityBenchmark failed: %v", err)
	}

	if report.TotalTasksEvaluated == 0 {
		t.Fatalf("expected evaluated tasks > 0")
	}

	// 1. Invariant A Verification: Zero below-floor selections
	if !report.Invariants.InvariantAHeld {
		t.Errorf("Invariant A violated: %d tasks had quality below floor", report.Invariants.InvariantAViolations)
	}

	// 2. Invariant C Verification: Critical tasks must receive high compute / verification
	if !report.Invariants.InvariantCHeld {
		t.Errorf("Invariant C violated: critical tasks did not receive high compute and verification")
	}

	// 3. Invariant D Verification: Cost savings from avoidable spend
	if !report.Invariants.InvariantDHeld {
		t.Errorf("Invariant D violated: ContextOS cost should be lower than fixed baselines")
	}

	// 4. Significant Cost Savings vs Fixed Max (> 50%)
	if report.CostSavingsVsFixedMax < 50.0 {
		t.Errorf("expected cost savings vs Fixed Max > 50%%, got %.2f%%", report.CostSavingsVsFixedMax)
	}

	// 5. Cost Per Success (CPS) lower than Baseline Fixed Max
	ctxSummary := report.Policies[PolicyContextOSOptimizer]
	fixedSummary := report.Policies[PolicyBaselineFixedMax]
	if ctxSummary.CostPerSuccess >= fixedSummary.CostPerSuccess {
		t.Errorf("CPS for ContextOS ($%.4f) should be lower than Fixed Max ($%.4f)",
			ctxSummary.CostPerSuccess, fixedSummary.CostPerSuccess)
	}

	// 6. Verify non-inferiority reports
	for _, ni := range report.NonInferiority {
		if !ni.NonInferior {
			t.Errorf("non-inferiority condition failed for %s: delta = %.2f%%, CI = [%.2f%%, %.2f%%]",
				ni.BaselinePolicy, ni.DeltaSuccessRate*100, ni.CI95Lower*100, ni.CI95Upper*100)
		}
	}

	// 7. Verify report files generated
	jsonPath := filepath.Join(resultsDir, "r16_summary.json")
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		t.Errorf("expected r16_summary.json to exist at %s", jsonPath)
	}

	mdPath := filepath.Join(resultsDir, "r16_report.md")
	if _, err := os.Stat(mdPath); os.IsNotExist(err) {
		t.Errorf("expected r16_report.md to exist at %s", mdPath)
	}
}
