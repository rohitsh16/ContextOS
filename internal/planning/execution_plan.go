package planning

import (
	"time"

	"contextos/internal/model"
	"contextos/internal/state"
	"contextos/internal/verification"
)

// ExecutionPlan is the comprehensive plan governing context, compute, model, and verification for a task.
type ExecutionPlan struct {
	Task                  string                         `json:"task"`
	ContextPlan           model.ContextPlan              `json:"context_plan"`
	ComputePlan           ComputePlan                    `json:"compute_plan"`
	VerificationLevel     verification.VerificationLevel `json:"verification_level"`
	DecisionState         state.DecisionState            `json:"decision_state"`
	TotalEstimatedCostUSD float64                        `json:"total_estimated_cost_usd"`
	CreatedAt             time.Time                      `json:"created_at"`
}

// BuildExecutionPlan synthesizes individual sub-plans into a master execution strategy.
func BuildExecutionPlan(
	task string,
	contextPlan model.ContextPlan,
	computePlan ComputePlan,
	decisionState state.DecisionState,
) ExecutionPlan {
	verifyLevel := verification.RecommendVerificationLevel(
		decisionState.Risk,
		computePlan.CanBypass,
		computePlan.Budget.MaxCostUSD,
	)

	totalCost := contextPlan.EstimatedCost + computePlan.EstimatedCostUSD

	return ExecutionPlan{
		Task:                  task,
		ContextPlan:           contextPlan,
		ComputePlan:           computePlan,
		VerificationLevel:     verifyLevel,
		DecisionState:         decisionState,
		TotalEstimatedCostUSD: totalCost,
		CreatedAt:             time.Now().UTC(),
	}
}
