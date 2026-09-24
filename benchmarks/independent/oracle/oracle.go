package oracle

import "contextos/benchmarks/independent/evaluator"

// CheapestSufficient uses only observed outputs, never policy predictions.
func CheapestSufficient(m evaluator.Manifest, taskID string, scores []evaluator.Score) (evaluator.Execution, bool) {
	var best evaluator.Execution
	ok := false
	for _, s := range scores {
		if s.Execution.TaskID != taskID || !s.FloorPass {
			continue
		}
		if !ok || *s.Execution.Cost < *best.Cost {
			best, ok = s.Execution, true
		}
	}
	return best, ok
}
