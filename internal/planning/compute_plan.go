package planning

import (
	"time"

	"contextos/internal/compute"
	"contextos/internal/verification"
)

// ComputePlan encapsulates the inference compute, reasoning effort, and budget strategy for a task.
type ComputePlan struct {
	Task               string                 `json:"task"`
	TaskClass          compute.TaskClass      `json:"task_class"`
	Difficulty         float64                `json:"difficulty"`
	Policy             compute.ComputePolicy  `json:"policy"`
	Budget             compute.BudgetPolicy   `json:"budget"`
	SelectedModel      compute.ModelCandidate `json:"selected_model"`
	Cascade            compute.CascadePlan    `json:"cascade"`
	CanBypass          bool                   `json:"can_bypass"`
	BypassReason       string                 `json:"bypass_reason,omitempty"`
	EstimatedCostUSD   float64                `json:"estimated_cost_usd"`
	EstimatedLatencyMS int64                  `json:"estimated_latency_ms"`
	CreatedAt          time.Time              `json:"created_at"`
}

// ComputePlanner orchestrates compute policy synthesis.
type ComputePlanner struct {
	profiler *compute.TaskProfiler
	router   *compute.ModelRouter
}

// NewComputePlanner creates a compute planner.
func NewComputePlanner() *ComputePlanner {
	return &ComputePlanner{
		profiler: compute.NewTaskProfiler(),
		router:   compute.NewModelRouter(),
	}
}

// Generate builds an adaptive compute plan tailored to the task difficulty and risk constraints.
func (cp *ComputePlanner) Generate(task string, riskTarget float64, preferredProvider string) ComputePlan {
	if riskTarget <= 0 {
		riskTarget = 0.05
	}

	profile := cp.profiler.Profile(task, 0)
	bypass, bypassKind := verification.IsDeterministicBypass(task)
	if bypass {
		profile.CanBypass = true
		profile.Class = compute.T0Deterministic
	}

	model := cp.router.SelectOptimalModel(profile.Difficulty, preferredProvider)
	cascade := cp.router.BuildCascade(profile.Difficulty)
	policy := compute.DefaultPolicyForTask(profile.Class, riskTarget)

	var bypassReason string
	if profile.CanBypass {
		bypassReason = "deterministic bypass: task can be resolved via local symbol graph (" + bypassKind + ")"
		zeroTokens := int64(0)
		policy.MaxReasoningTokens = &zeroTokens
		policy.Effort = compute.EffortMinimal
	}

	budget := compute.BudgetPolicy{
		MaxCostUSD:          2.00,
		MaxContextTokens:    30000,
		MaxReasoningTokens:  32768,
		MaxToolCalls:        profile.Features.ExpectedToolCalls + 5,
		MaxTurns:            15,
		VerificationReserve: 0.20,
		EscalationReserve:   0.20,
	}

	cost := model.ExpectedCost()
	if profile.CanBypass {
		cost = 0.0
	}

	return ComputePlan{
		Task:               task,
		TaskClass:          profile.Class,
		Difficulty:         profile.Difficulty,
		Policy:             policy,
		Budget:             budget,
		SelectedModel:      model,
		Cascade:            cascade,
		CanBypass:          profile.CanBypass,
		BypassReason:       bypassReason,
		EstimatedCostUSD:   cost,
		EstimatedLatencyMS: int64(model.HistoricalLatencyMS),
		CreatedAt:          time.Now().UTC(),
	}
}
