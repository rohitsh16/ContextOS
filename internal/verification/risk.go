package verification

import (
	"strings"
)

// RiskEvaluator calculates impact and residual risk of errors.
type RiskEvaluator struct{}

// NewRiskEvaluator creates a risk evaluator.
func NewRiskEvaluator() *RiskEvaluator {
	return &RiskEvaluator{}
}

// ComputeRiskScore calculates composite risk score in [0.0, 1.0].
func (re *RiskEvaluator) ComputeRiskScore(probError, impact float64) float64 {
	score := probError * impact
	if score > 1.0 {
		return 1.0
	}
	if score < 0 {
		return 0
	}
	return score
}

// IsDeterministicBypass checks whether an engineering task can be answered with 100% precision
// using ContextOS's internal index, graph, or compiler without invoking any LLM reasoning.
func IsDeterministicBypass(query string) (bool, string) {
	clean := strings.ToLower(strings.TrimSpace(query))

	if strings.HasPrefix(clean, "where is") || strings.HasPrefix(clean, "find definition") || strings.HasPrefix(clean, "def of") {
		return true, "symbol_definition_lookup"
	}
	if strings.HasPrefix(clean, "find callers") || strings.HasPrefix(clean, "who calls") {
		return true, "symbol_caller_graph"
	}
	if strings.HasPrefix(clean, "list imports") || strings.HasPrefix(clean, "show imports") {
		return true, "import_dependency_index"
	}
	if strings.HasPrefix(clean, "find implement") || strings.HasPrefix(clean, "list implementations") {
		return true, "interface_implementation_lookup"
	}
	if strings.HasPrefix(clean, "show diff") || strings.HasPrefix(clean, "git diff") {
		return true, "git_revision_diff"
	}

	return false, ""
}
