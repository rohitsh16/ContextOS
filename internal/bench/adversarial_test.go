package bench

import (
	"testing"
)

func TestAdversarialEvaluationSuite(t *testing.T) {
	targetRev := "rev-100"
	report := RunAdversarialSuite(targetRev)

	if report.Status != "GREEN" {
		t.Fatalf("expected adversarial suite status GREEN, got %s (robustness=%.2f%%)",
			report.Status, report.RobustnessRate*100)
	}

	if report.BlockedAttacks < 4 {
		t.Fatalf("expected at least 4 attacks blocked, got %d", report.BlockedAttacks)
	}

	// Verify each vector was tested
	expectedVectors := []AdversarialAttackVector{
		AttackSemanticDistractor,
		AttackDeprecatedCodeTrap,
		AttackBudgetStarvation,
		AttackMemoryPoisoning,
		AttackFutureLeakageTrap,
	}

	for _, vec := range expectedVectors {
		if _, ok := report.VectorResilience[vec]; !ok {
			t.Errorf("missing resilience result for attack vector %s", vec)
		}
	}
}
