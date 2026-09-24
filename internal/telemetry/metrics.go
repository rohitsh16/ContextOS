package telemetry

import "time"

// TaskRunTelemetry records the end-to-end execution, cost, and verification outcome of a single task.
type TaskRunTelemetry struct {
	RunID         string    `json:"run_id"`
	TraceID       string    `json:"trace_id"`
	TaskID        string    `json:"task_id"`
	Provider      string    `json:"provider"`
	Model         string    `json:"model"`
	ModelVersion  string    `json:"model_version"`
	Timestamp     time.Time `json:"timestamp"`

	Usage UsageMetrics `json:"usage"`

	ContextTokens   int64 `json:"context_tokens"`
	RetrievalTokens int64 `json:"retrieval_tokens"`

	TaskFamily         string             `json:"task_family,omitempty"`
	TaskClass          string             `json:"task_class,omitempty"`
	DifficultyBucket   string             `json:"difficulty_bucket,omitempty"`
	Difficulty         float64            `json:"difficulty,omitempty"`
	QualityScore       float64            `json:"quality_score,omitempty"`
	QualityComponents  map[string]float64 `json:"quality_components,omitempty"`
	RequestedEffort    string             `json:"requested_effort,omitempty"`
	EstimatorSource    string             `json:"estimator_source,omitempty"`
	EstimatorSamples   int                `json:"estimator_samples,omitempty"`
	FallbackLevel      int                `json:"fallback_level,omitempty"`

	Success            bool   `json:"success"`
	TestsPassed        bool   `json:"tests_passed"`
	DecisionPreserved  bool   `json:"decision_preserved"`
	VerificationResult string `json:"verification_result,omitempty"`
}

// SummaryKPIs aggregates cost, efficiency, and quality across multiple task runs.
type SummaryKPIs struct {
	TotalRuns         int     `json:"total_runs"`
	SuccessfulTasks   int     `json:"successful_tasks"`
	SuccessRate       float64 `json:"success_rate"`

	TotalCostUSD      float64 `json:"total_cost_usd"`
	CostPerSuccessUSD float64 `json:"cost_per_successful_task_usd"` // CPS = TotalCost / SuccessfulTasks

	TotalReasoningTokens int64   `json:"total_reasoning_tokens"`
	ReasoningEfficiency  float64 `json:"reasoning_efficiency"` // RE = SuccessfulTasks / ReasoningTokens

	TotalTurns     int     `json:"total_turns"`
	TurnEfficiency float64 `json:"turn_efficiency"` // TE = SuccessfulTasks / Turns

	BaselineReasoningCostUSD float64 `json:"baseline_reasoning_cost_usd,omitempty"`
	ComputeCompression       float64 `json:"compute_compression"` // CC = 1 - (ContextOSCost / BaselineCost)
}

// ComputeSummaryKPIs computes KPIs across a slice of TaskRunTelemetry records.
func ComputeSummaryKPIs(runs []TaskRunTelemetry, baselineReasoningCostUSD float64) SummaryKPIs {
	kpi := SummaryKPIs{
		TotalRuns:                len(runs),
		BaselineReasoningCostUSD: baselineReasoningCostUSD,
	}
	if len(runs) == 0 {
		return kpi
	}

	var totalReasoningCost float64

	for _, r := range runs {
		if r.Success {
			kpi.SuccessfulTasks++
		}
		kpi.TotalCostUSD += r.Usage.EstimatedCostUSD
		kpi.TotalReasoningTokens += r.Usage.ReasoningTokens
		kpi.TotalTurns += r.Usage.Turns
		if pricing, ok := LookupPricing(r.Provider, r.Model); ok {
			totalReasoningCost += (float64(r.Usage.ReasoningTokens) / 1e6) * pricing.ReasoningPerMillion
		}
	}

	kpi.SuccessRate = float64(kpi.SuccessfulTasks) / float64(kpi.TotalRuns)

	if kpi.SuccessfulTasks > 0 {
		kpi.CostPerSuccessUSD = kpi.TotalCostUSD / float64(kpi.SuccessfulTasks)
		kpi.ReasoningEfficiency = float64(kpi.SuccessfulTasks) / float64(kpi.TotalReasoningTokens)
		if kpi.TotalTurns > 0 {
			kpi.TurnEfficiency = float64(kpi.SuccessfulTasks) / float64(kpi.TotalTurns)
		}
	}

	if baselineReasoningCostUSD > 0 {
		kpi.ComputeCompression = 1.0 - (totalReasoningCost / baselineReasoningCostUSD)
	}

	return kpi
}
