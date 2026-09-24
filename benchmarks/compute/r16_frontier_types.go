package compute_bench

import (
	"contextos/internal/compute"
	"contextos/internal/evaluation"
	"contextos/internal/providers"
	"contextos/internal/telemetry"
)

// R16S2Task is immutable benchmark input. The runner never derives an oracle
// from a task's holdout outcome before the configured split is locked.
type R16S2Task struct {
	ID       string              `json:"id"`
	Family   string              `json:"family"`
	Split    string              `json:"split,omitempty"` // "calibration", "validation", "holdout"
	Profile  compute.TaskProfile `json:"profile"`
	Rubric   evaluation.Rubric   `json:"rubric"`
}

type R16S2Run struct {
	TaskID           string                 `json:"task_id"`
	Policy           string                 `json:"policy"`
	Provider         string                 `json:"provider"`
	Model            string                 `json:"model"`
	ModelVersion     string                 `json:"model_version"`
	Effort           providers.EffortLevel  `json:"requested_effort"`
	Usage            telemetry.UsageMetrics `json:"usage"`
	Evaluation       evaluation.Result      `json:"evaluation"`
	EstimatorSource  string                 `json:"estimator_source"`
	EstimatorSamples int                    `json:"estimator_samples"`
	FallbackLevel    int                    `json:"fallback_level"`
}

// R16S2Config configures the R16-S2 empirical frontier canary benchmark.
type R16S2Config struct {
	Mode           string                   `json:"mode"` // "mock" or "real"
	ResultsDir     string                   `json:"results_dir"`
	Seed           int64                    `json:"seed"`
	Repeats        int                      `json:"repeats"`
	Tasks          []R16S2Task              `json:"tasks"`
	Models         []compute.ModelCandidate `json:"models"`
	CalibrationPct float64                  `json:"calibration_pct"`
	ValidationPct  float64                  `json:"validation_pct"`
	HoldoutPct     float64                  `json:"holdout_pct"`
}

// R16S2Manifest defines the reproducibility manifest per Section 34.
type R16S2Manifest struct {
	GitCommitSHA        string                 `json:"git_commit_sha"`
	ManifestID          string                 `json:"manifest_id"`
	BenchmarkCodeVersion string                `json:"benchmark_code_version"`
	ProviderMode        string                 `json:"provider_mode"`
	ProviderNames       []string               `json:"provider_names"`
	ModelVersions       map[string]string      `json:"model_versions"`
	PricingVersion      string                 `json:"pricing_version"`
	PricingSourceURLs   []string               `json:"pricing_source_urls"`
	TaskMatrixVersion   string                 `json:"task_matrix_version"`
	RandomSeed          int64                  `json:"random_seed"`
	ExecutionDate       string                 `json:"execution_date"`
	Platform            string                 `json:"platform"`
	GoVersion           string                 `json:"go_version"`
	ConfigurationFlags  map[string]interface{} `json:"configuration_flags"`
}

// R16S2ObservationRecord defines the immutable JSONL record per Section 33.
type R16S2ObservationRecord struct {
	RunID                   string             `json:"run_id"`
	TaskID                  string             `json:"task_id"`
	TaskFamily              string             `json:"task_family"`
	TaskClass               string             `json:"task_class"`
	RepoRevision            string             `json:"repo_revision"`
	Policy                  string             `json:"policy"`
	Provider                string             `json:"provider"`
	Model                   string             `json:"model"`
	ModelVersion            string             `json:"model_version"`
	RequestedEffort         string             `json:"requested_effort"`
	RequestedReasoningBudget int64             `json:"requested_reasoning_budget"`
	ActualReasoningTokens   int64              `json:"actual_reasoning_tokens"`
	InputTokens             int64              `json:"input_tokens"`
	CachedInputTokens       int64              `json:"cached_input_tokens"`
	CacheWriteTokens        int64              `json:"cache_write_tokens"`
	VisibleOutputTokens     int64              `json:"visible_output_tokens"`
	ToolCalls               int                `json:"tool_calls"`
	VerificationCalls       int                `json:"verification_calls"`
	Escalations             int                `json:"escalations"`
	Retries                 int                `json:"retries"`
	TotalCostUSD            float64            `json:"total_cost_usd"`
	ProviderReportedCostUSD float64            `json:"provider_reported_cost_usd"`
	Quality                 float64            `json:"quality"`
	Success                 bool               `json:"success"`
	TestsPassed             bool               `json:"tests_passed"`
	QualityLCB              float64            `json:"quality_lcb"`
	SuccessLCB              float64            `json:"success_lcb"`
	CapabilityFloor         float64            `json:"capability_floor"`
	CapabilityFloorPass     bool               `json:"capability_floor_pass"`
	OracleCostUSD           float64            `json:"oracle_cost_usd"`
	OracleRegretUSD         float64            `json:"oracle_regret_usd"`
	LatencyMS               float64            `json:"latency_ms"`
	ControllerOverheadMS    float64            `json:"controller_overhead_ms"`
	EstimatorSource         string             `json:"estimator_source"`
	EstimatorSamples        int                `json:"estimator_samples"`
	FallbackLevel           int                `json:"fallback_level"`
	QualityComponents       map[string]float64 `json:"quality_components,omitempty"`
}

// R16S2PolicyMetrics summarizes policy performance across the benchmark.
type R16S2PolicyMetrics struct {
	Policy               string  `json:"policy"`
	Description          string  `json:"description"`
	TotalRuns            int     `json:"total_runs"`
	SuccessCount         int     `json:"success_count"`
	SuccessRate          float64 `json:"success_rate"`
	MeanQuality          float64 `json:"mean_quality"`
	QualityLCB           float64 `json:"quality_lcb"`
	CostPerSuccess       float64 `json:"cost_per_success"` // CPS = TotalCost / SuccessCount
	MeanCostUSD          float64 `json:"mean_cost_usd"`
	P95CostUSD           float64 `json:"p95_cost_usd"`
	TotalCostUSD         float64 `json:"total_cost_usd"`
	MeanReasoningTokens  float64 `json:"mean_reasoning_tokens"`
	MeanLatencyMS        float64 `json:"mean_latency_ms"`
	FloorViolationRate   float64 `json:"floor_violation_rate"`
	OracleRelativeRegret float64 `json:"oracle_relative_regret"`
}

// R16S2ClassBreakdown summarizes class-level metrics.
type R16S2ClassBreakdown struct {
	Class               string  `json:"class"`
	TotalRuns           int     `json:"total_runs"`
	SuccessRate         float64 `json:"success_rate"`
	MeanQuality         float64 `json:"mean_quality"`
	MeanCostUSD         float64 `json:"mean_cost_usd"`
	MeanReasoningTokens float64 `json:"mean_reasoning_tokens"`
}

// R16S2Summary aggregates the canary outcomes and integrity gate checks.
type R16S2Summary struct {
	Manifest                   R16S2Manifest                  `json:"manifest"`
	TotalExecutions            int                            `json:"total_executions"`
	CalibrationRuns            int                            `json:"calibration_runs"`
	ValidationRuns             int                            `json:"validation_runs"`
	HoldoutRuns                int                            `json:"holdout_runs"`
	Policies                   map[string]R16S2PolicyMetrics  `json:"policies"`
	ClassBreakdown             map[string]R16S2ClassBreakdown `json:"class_breakdown"`
	Frontiers                  []compute.Frontier             `json:"frontiers"`
	BillingReconciliationDiff  float64                        `json:"billing_reconciliation_diff"`
	CapabilityFloorViolationRate float64                      `json:"capability_floor_violation_rate"`
	GatesPassed                bool                           `json:"gates_passed"`
}

