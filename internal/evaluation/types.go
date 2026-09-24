// Package evaluation contains preregistered, policy-independent task oracles.
package evaluation

type Result struct {
	Success            bool               `json:"success"`
	Quality            float64            `json:"quality"`
	TestsPassed        bool               `json:"tests_passed"`
	VerificationResult string             `json:"verification_result"`
	Components         map[string]float64 `json:"quality_components"`
}

type Rubric struct {
	Weights  map[string]float64 `json:"weights"`
	Required []string           `json:"required"`
}

func (r Rubric) Score(components map[string]float64) Result {
	var q, total float64
	ok := true
	for _, name := range r.Required {
		if components[name] < 1 {
			ok = false
		}
	}
	for name, weight := range r.Weights {
		q += components[name] * weight
		total += weight
	}
	if total > 0 {
		q /= total
	}
	return Result{Success: ok, Quality: q, TestsPassed: components["unit_tests"] == 1, VerificationResult: map[bool]string{true: "PASS", false: "FAIL"}[ok], Components: components}
}
