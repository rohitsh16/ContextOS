package evaluation

import "testing"

func TestCodingT1Pass(t *testing.T) {
	res := EvaluateCoding("T1", map[string]float64{
		"build":       1.0,
		"unit_tests":  1.0,
		"patch_valid": 0.8,
	})
	if !res.Success {
		t.Fatalf("T1 with build+tests=1 should pass: %+v", res)
	}
	if res.Quality < 0.9 {
		t.Fatalf("expected quality >= 0.9, got %f", res.Quality)
	}
}

func TestCodingT1FailMissingTests(t *testing.T) {
	res := EvaluateCoding("T1", map[string]float64{
		"build":       1.0,
		"unit_tests":  0.5,
		"patch_valid": 1.0,
	})
	if res.Success {
		t.Fatalf("T1 with unit_tests < 1 must not pass the hard gate: %+v", res)
	}
}

func TestCodingT3RaceDetectorRequired(t *testing.T) {
	res := EvaluateCoding("T3", map[string]float64{
		"build":             1.0,
		"unit_tests":        1.0,
		"integration_tests": 1.0,
		"regression_tests":  1.0,
		"race_detector":     0.0, // fails required gate
		"patch_valid":       1.0,
	})
	if res.Success {
		t.Fatalf("T3 must require race_detector=1: %+v", res)
	}
}

func TestCodingT4AllRequired(t *testing.T) {
	// All required criteria satisfied
	res := EvaluateCoding("T4", map[string]float64{
		"build":             1.0,
		"unit_tests":        1.0,
		"integration_tests": 1.0,
		"regression_tests":  1.0,
		"race_detector":     1.0,
		"static_analysis":   0.9,
		"patch_valid":       0.95,
	})
	if !res.Success {
		t.Fatalf("T4 with all required=1 should pass: %+v", res)
	}
	if res.Quality < 0.95 {
		t.Fatalf("expected quality >= 0.95, got %f", res.Quality)
	}
}

func TestCodingT4FailIntegration(t *testing.T) {
	res := EvaluateCoding("T4", map[string]float64{
		"build":             1.0,
		"unit_tests":        1.0,
		"integration_tests": 0.5, // required, fails hard gate
		"regression_tests":  1.0,
		"race_detector":     1.0,
		"static_analysis":   1.0,
		"patch_valid":       1.0,
	})
	if res.Success {
		t.Fatalf("T4 must require integration_tests=1: %+v", res)
	}
}

func TestReasoningT3Pass(t *testing.T) {
	res := EvaluateReasoning("T3", map[string]float64{
		"constraints_satisfied": 1.0,
		"invariants_satisfied":  1.0,
		"components_present":    0.8,
		"counterexamples":       0.7,
		"reference_cases":       0.6,
	})
	if !res.Success {
		t.Fatalf("T3 reasoning with required=1 should pass: %+v", res)
	}
}

func TestReasoningT4FailMissingComponent(t *testing.T) {
	res := EvaluateReasoning("T4", map[string]float64{
		"constraints_satisfied": 1.0,
		"invariants_satisfied":  1.0,
		"components_present":    0.5, // required, fails hard gate
		"counterexamples":       1.0,
		"reference_cases":       1.0,
		"formal_correctness":    1.0,
	})
	if res.Success {
		t.Fatalf("T4 reasoning must require components_present=1: %+v", res)
	}
}

func TestReasoningDefaultPass(t *testing.T) {
	res := EvaluateReasoning("unknown", map[string]float64{
		"constraints_satisfied": 1.0,
		"invariants_satisfied":  1.0,
		"components_present":    1.0,
		"counterexamples":       1.0,
	})
	if !res.Success {
		t.Fatalf("default reasoning rubric should pass with all 1.0: %+v", res)
	}
	if res.Quality != 1.0 {
		t.Fatalf("expected quality 1.0, got %f", res.Quality)
	}
}

func TestCodingRubricDeterministic(t *testing.T) {
	// Invariant: same inputs => same score (no model-as-judge randomness)
	components := map[string]float64{
		"build":       1.0,
		"unit_tests":  0.8,
		"patch_valid": 0.9,
	}
	r1 := EvaluateCoding("T1", components)
	r2 := EvaluateCoding("T1", components)
	if r1.Quality != r2.Quality || r1.Success != r2.Success {
		t.Fatalf("rubric must be deterministic: %+v vs %+v", r1, r2)
	}
}
