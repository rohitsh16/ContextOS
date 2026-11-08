package evaluation

// ReasoningRubric returns the rubric for architectural and design tasks
// where outcomes are evaluated with structured criteria rather than unit tests.
func ReasoningRubric(taskClass string) Rubric {
	switch taskClass {
	case "T3", "complex_reasoning":
		return Rubric{
			Weights: map[string]float64{
				"constraints_satisfied": 0.30,
				"invariants_satisfied":  0.25,
				"components_present":    0.20,
				"counterexamples":       0.15,
				"reference_cases":       0.10,
			},
			Required: []string{"constraints_satisfied", "invariants_satisfied"},
		}
	case "T4", "architecture_design":
		return Rubric{
			Weights: map[string]float64{
				"constraints_satisfied": 0.25,
				"invariants_satisfied":  0.25,
				"components_present":    0.15,
				"counterexamples":       0.15,
				"reference_cases":       0.10,
				"formal_correctness":    0.10,
			},
			Required: []string{"constraints_satisfied", "invariants_satisfied", "components_present"},
		}
	default:
		return Rubric{
			Weights: map[string]float64{
				"constraints_satisfied": 0.35,
				"invariants_satisfied":  0.30,
				"components_present":    0.20,
				"counterexamples":       0.15,
			},
			Required: []string{"constraints_satisfied"},
		}
	}
}

// EvaluateReasoning scores a reasoning/design task against its preregistered
// rubric. Components must be normalized to [0,1].
func EvaluateReasoning(taskClass string, components map[string]float64) Result {
	rubric := ReasoningRubric(taskClass)
	return rubric.Score(components)
}
