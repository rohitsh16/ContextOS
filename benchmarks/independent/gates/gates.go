package gates

import "contextos/benchmarks/independent/evaluator"

type Verdict string

const (
	Green  Verdict = "GREEN"
	Yellow Verdict = "YELLOW"
	Red    Verdict = "RED"
)

type Result struct {
	Verdict Verdict `json:"verdict"`
	Reason  string  `json:"reason"`
}

func Evaluate(scores []evaluator.Score) Result {
	if len(scores) == 0 {
		return Result{Red, "no scored executions"}
	}
	for _, s := range scores {
		if s.CriticalFailure {
			return Result{Red, "critical capability-floor violation"}
		}
	}
	for _, s := range scores {
		if !s.FloorPass {
			return Result{Yellow, "non-critical capability-floor violation"}
		}
	}
	return Result{Green, "all scored executions meet frozen floors"}
}
