// Package evaluator is intentionally controller-agnostic.
package evaluator

import (
	"fmt"
	"sort"
)

type Manifest struct {
	BenchmarkID          string  `json:"benchmark_id"`
	Seed                 int64   `json:"seed"`
	QualityFloor         float64 `json:"quality_floor"`
	SuccessFloor         float64 `json:"success_floor"`
	NonInferiorityMargin float64 `json:"non_inferiority_margin"`
}
type Task struct {
	ID       string `json:"id"`
	Family   string `json:"family"`
	Class    string `json:"class"`
	Critical bool   `json:"critical"`
}
type GroundTruth struct {
	TaskID   string             `json:"task_id"`
	Required map[string]float64 `json:"required"`
}
type Execution struct {
	RunID        string   `json:"run_id"`
	TaskID       string   `json:"task_id"`
	Policy       string   `json:"policy"`
	Model        string   `json:"model"`
	Effort       string   `json:"requested_effort"`
	Cost         *float64 `json:"recomputed_cost"`
	ProviderCost *float64 `json:"provider_reported_cost"`
	Quality      float64  `json:"quality"`
	Success      bool     `json:"success"`
	TestsPassed  bool     `json:"tests_passed"`
}
type Score struct {
	Execution       Execution `json:"execution"`
	FloorPass       bool      `json:"floor_pass"`
	CriticalFailure bool      `json:"critical_failure"`
}

func Validate(tasks []Task, truth []GroundTruth, runs []Execution) error {
	seen := map[string]bool{}
	for _, t := range tasks {
		if t.ID == "" || seen[t.ID] {
			return fmt.Errorf("duplicate or empty task id %q", t.ID)
		}
		seen[t.ID] = true
	}
	runIDs := map[string]bool{}
	for _, r := range runs {
		if r.RunID == "" || runIDs[r.RunID] {
			return fmt.Errorf("duplicate or empty run id %q", r.RunID)
		}
		runIDs[r.RunID] = true
		if !seen[r.TaskID] {
			return fmt.Errorf("unknown task %q", r.TaskID)
		}
		if r.Cost == nil {
			return fmt.Errorf("missing cost for run %s", r.RunID)
		}
	}
	return nil
}

// ScoreAll never branches on policy: policy labels cannot influence evaluation.
func ScoreAll(m Manifest, tasks []Task, runs []Execution) ([]Score, error) {
	if err := Validate(tasks, nil, runs); err != nil {
		return nil, err
	}
	byID := map[string]Task{}
	for _, t := range tasks {
		byID[t.ID] = t
	}
	out := make([]Score, 0, len(runs))
	for _, r := range runs {
		pass := r.Quality >= m.QualityFloor && r.Success
		out = append(out, Score{Execution: r, FloorPass: pass, CriticalFailure: byID[r.TaskID].Critical && !pass})
	}
	return out, nil
}
func SortStable(runs []Execution) {
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].RunID < runs[j].RunID })
}
