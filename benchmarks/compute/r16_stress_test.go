package compute_bench

import (
	"os"
	"path/filepath"
	"testing"
)

func TestR16StressProtocol_Ring0AdversarialTraps(t *testing.T) {
	suite := NewRing0Suite()
	traps, allPassed := suite.RunAllTraps()

	if len(traps) != 5 {
		t.Fatalf("expected 5 mandatory traps, got %d", len(traps))
	}

	for _, trap := range traps {
		if !trap.Passed {
			t.Errorf("Ring 0 trap failed: %s - %s", trap.TrapName, trap.Details)
		}
		if trap.ViolationCount > 0 {
			t.Errorf("Ring 0 trap %s reported %d violations (gate requires 0)", trap.TrapName, trap.ViolationCount)
		}
	}

	if !allPassed {
		t.Fatalf("Ring 0 suite failed one or more adversarial traps")
	}
}

func TestR16StressProtocol_FullMultiRing(t *testing.T) {
	resultsDir := filepath.Join("..", "results", "r16")
	_ = os.MkdirAll(resultsDir, 0755)

	report, err := RunR16StressBenchmark(500, 4, resultsDir)
	if err != nil {
		t.Fatalf("RunR16StressBenchmark failed: %v", err)
	}

	// 1. Gate: Verdict must be GREEN
	if report.Verdict != VerdictGreen {
		t.Errorf("expected Verdict GREEN, got %s (Reasons: %v)", report.Verdict, report.VerdictReasons)
	}

	// 2. Gate: Ring 0 Passed
	if !report.Ring0Passed {
		t.Errorf("expected Ring 0 passed")
	}

	// 3. Gate: Ring 1 Violation Rate must be 0.00%
	if report.Ring1ViolationRate > 0.0 {
		t.Errorf("expected Ring 1 violation rate 0.00%%, got %.4f%%", report.Ring1ViolationRate*100)
	}

	// 4. Gate: Avoidable Cost Reduction (ACR) should be > 60%
	if report.AvoidableCostACR < 0.60 {
		t.Errorf("expected ACR > 0.60, got %.4f", report.AvoidableCostACR)
	}

	// 5. Gate: Floor Ablation Delta Q must be positive (proves floor prevents quality degradation)
	if report.FloorAblationDeltaQ <= 0.0 {
		t.Errorf("expected Floor Ablation Delta Q > 0, got %.4f", report.FloorAblationDeltaQ)
	}

	// 6. Gate: Controller overhead ratio must be <= 5%
	if report.ControllerOverhead > 0.05 {
		t.Errorf("expected controller overhead <= 0.05, got %.4f", report.ControllerOverhead)
	}

	// 7. Verify generated artifacts
	tracePath := filepath.Join(resultsDir, "r16_stress_trace.jsonl")
	if info, err := os.Stat(tracePath); os.IsNotExist(err) || info.Size() == 0 {
		t.Errorf("expected non-empty r16_stress_trace.jsonl at %s", tracePath)
	}

	mdPath := filepath.Join(resultsDir, "r16_stress_report.md")
	if info, err := os.Stat(mdPath); os.IsNotExist(err) || info.Size() == 0 {
		t.Errorf("expected non-empty r16_stress_report.md at %s", mdPath)
	}

	sumPath := filepath.Join(resultsDir, "r16_stress_summary.json")
	if info, err := os.Stat(sumPath); os.IsNotExist(err) || info.Size() == 0 {
		t.Errorf("expected non-empty r16_stress_summary.json at %s", sumPath)
	}
}
