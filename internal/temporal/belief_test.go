package temporal

import (
	"testing"

	"contextos/internal/model"
)

func TestBeliefStateUpdatingAndCalibration(t *testing.T) {
	// 1. Bayesian updating test
	prior := 0.80
	postPass := UpdateBelief(prior, "test_pass")
	if postPass <= prior {
		t.Fatalf("expected test_pass to increase belief, got %.4f <= %.4f", postPass, prior)
	}

	postFail := UpdateBelief(prior, "test_fail")
	if postFail >= prior {
		t.Fatalf("expected test_fail to decrease belief, got %.4f >= %.4f", postFail, prior)
	}

	// 2. Calibration evaluation
	memories := []model.Memory{
		{ID: "m1", Kind: "decision", Confidence: 0.85},
		{ID: "m2", Kind: "constraint", Confidence: 0.90},
		{ID: "m3", Kind: "code", Confidence: 0.70, InvalidatedAtRevision: "rev-001"},
		{ID: "m4", Kind: "code", Confidence: 0.65, InvalidatedAtRevision: "rev-002"},
	}

	events := map[string][]string{
		"m1": {"test_pass", "user_decision"},
		"m2": {"git_diff_clean", "test_pass"},
		"m3": {"git_diff_modified", "test_fail"},
		"m4": {"git_diff_modified", "test_fail"},
	}

	analysis := EvaluateBeliefState(memories, events)
	if analysis.Status != "GREEN" {
		t.Fatalf("expected belief state status GREEN, got %s (brier=%.4f, ece=%.4f)",
			analysis.Status, analysis.BrierScore, analysis.ECE)
	}

	if analysis.BrierScore > 0.20 {
		t.Fatalf("expected well-calibrated Brier score <= 0.20, got %.4f", analysis.BrierScore)
	}

	if analysis.ECE > 0.15 {
		t.Fatalf("expected ECE <= 0.15, got %.4f", analysis.ECE)
	}

	if analysis.ConformalCutoff <= 0.0 || analysis.ConformalCutoff >= 1.0 {
		t.Fatalf("expected valid conformal cutoff in (0, 1), got %.4f", analysis.ConformalCutoff)
	}
}
