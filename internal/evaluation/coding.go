package evaluation

// CodingRubric returns a preregistered rubric for coding tasks with composite
// quality scoring: Q = w_b·Build + w_u·UnitTests + w_i·IntegrationTests +
// w_r·RegressionTests + w_s·StaticAnalysis.
//
// Weights are fixed before analyzing holdout outcomes (Invariant E).
// For tasks where binary correctness is available, binary correctness
// dominates the gate via the Required field.
func CodingRubric(taskClass string) Rubric {
	switch taskClass {
	case "T1", "local_correctness":
		// Simple local fix: build + unit tests dominate.
		return Rubric{
			Weights: map[string]float64{
				"build":       0.30,
				"unit_tests":  0.50,
				"patch_valid": 0.20,
			},
			Required: []string{"build", "unit_tests"},
		}
	case "T2", "multi_file":
		// Multi-file engineering: integration tests add weight.
		return Rubric{
			Weights: map[string]float64{
				"build":             0.20,
				"unit_tests":        0.30,
				"integration_tests": 0.25,
				"patch_valid":       0.15,
				"static_analysis":   0.10,
			},
			Required: []string{"build", "unit_tests"},
		}
	case "T3", "concurrency":
		// Concurrency-sensitive: race detector and regression tests matter.
		return Rubric{
			Weights: map[string]float64{
				"build":            0.15,
				"unit_tests":       0.25,
				"integration_tests": 0.20,
				"regression_tests": 0.20,
				"race_detector":    0.10,
				"patch_valid":      0.10,
			},
			Required: []string{"build", "unit_tests", "race_detector"},
		}
	case "T4", "architecture":
		// High-complexity: all criteria must hold, strict gate.
		return Rubric{
			Weights: map[string]float64{
				"build":             0.10,
				"unit_tests":        0.20,
				"integration_tests": 0.20,
				"regression_tests":  0.15,
				"race_detector":     0.10,
				"static_analysis":   0.10,
				"patch_valid":       0.15,
			},
			Required: []string{"build", "unit_tests", "integration_tests", "regression_tests"},
		}
	default:
		// Conservative default: require build + tests.
		return Rubric{
			Weights: map[string]float64{
				"build":       0.30,
				"unit_tests":  0.40,
				"patch_valid": 0.30,
			},
			Required: []string{"build", "unit_tests"},
		}
	}
}

// EvaluateCoding scores a coding task outcome against its preregistered rubric.
// Components must be normalized to [0,1]. The caller supplies observed component
// scores; the rubric determines weights and hard-gate criteria.
func EvaluateCoding(taskClass string, components map[string]float64) Result {
	rubric := CodingRubric(taskClass)
	return rubric.Score(components)
}
