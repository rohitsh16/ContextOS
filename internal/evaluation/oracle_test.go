package evaluation

import "testing"

func TestRubricRequiresAllHardCriteria(t *testing.T) {
	r := Rubric{Weights: map[string]float64{"build": .5, "unit_tests": .5}, Required: []string{"build", "unit_tests"}}
	got := r.Score(map[string]float64{"build": 1, "unit_tests": .5})
	if got.Success || got.Quality != .75 {
		t.Fatalf("objective score must not let partial tests pass: %+v", got)
	}
}
