package evaluation

import "fmt"

// RequireExecutable rejects a canary task that has no preregistered observable oracle.
func RequireExecutable(r Rubric) error {
	if len(r.Required) == 0 || len(r.Weights) == 0 {
		return fmt.Errorf("objective evaluator requires preregistered required criteria and weights")
	}
	return nil
}
