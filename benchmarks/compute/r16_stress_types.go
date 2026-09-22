package compute_bench

import (
	"time"

	"contextos/internal/compute"
	"contextos/internal/providers"
	"contextos/internal/telemetry"
)

// FailureTaxonomyType represents the 16 failure classifications from R16 Section 13.
type FailureTaxonomyType string

const (
	FailureUnderCompute       FailureTaxonomyType = "UNDER_COMPUTE"
	FailureOverCompute        FailureTaxonomyType = "OVER_COMPUTE"
	FailureUnderRetrieval     FailureTaxonomyType = "UNDER_RETRIEVAL"
	FailureOverRetrieval      FailureTaxonomyType = "OVER_RETRIEVAL"
	FailureBadModelRoute      FailureTaxonomyType = "BAD_MODEL_ROUTE"
	FailureBadEffort          FailureTaxonomyType = "BAD_EFFORT"
	FailureBadStopping        FailureTaxonomyType = "BAD_STOPPING"
	FailureBadVerification    FailureTaxonomyType = "BAD_VERIFICATION"
	FailureBadEscalation      FailureTaxonomyType = "BAD_ESCALATION"
	FailureBadCalibration     FailureTaxonomyType = "BAD_CALIBRATION"
	FailureCostAccounting     FailureTaxonomyType = "COST_ACCOUNTING"
	FailureCacheSideEffect    FailureTaxonomyType = "CACHE_SIDE_EFFECT"
	FailureToolFailure        FailureTaxonomyType = "TOOL_FAILURE"
	FailureProviderFailure    FailureTaxonomyType = "PROVIDER_FAILURE"
	FailureControllerOverhead FailureTaxonomyType = "CONTROLLER_OVERHEAD"
	FailureOracleMismatch     FailureTaxonomyType = "ORACLE_MISMATCH"
)

// StressBaselineType represents the 10 required baselines from R16 Section 12.
type StressBaselineType string

const (
	B0FixedStrongDefault StressBaselineType = "B0_fixed_strong_default"
	B1FixedHighReasoning StressBaselineType = "B1_fixed_high_reasoning"
	B2FixedMaximum       StressBaselineType = "B2_fixed_maximum"
	B3ContextOnly        StressBaselineType = "B3_context_only"
	B4ComputeOnly        StressBaselineType = "B4_compute_only"
	B5ModelRoutingOnly   StressBaselineType = "B5_model_routing_only"
	B6ContextPlusCompute StressBaselineType = "B6_context_plus_compute"
	B7ContextPlusRouting StressBaselineType = "B7_context_plus_routing"
	B8CapabilityFloor    StressBaselineType = "B8_full_capability_floor"
	B8NoFloor            StressBaselineType = "B8_no_floor_ablated"
	B9OfflineOracle      StressBaselineType = "B9_offline_oracle"
)

// GateVerdict represents the R16 Section 15 Green/Yellow/Red verdict.
type GateVerdict string

const (
	VerdictGreen  GateVerdict = "GREEN"
	VerdictYellow GateVerdict = "YELLOW"
	VerdictRed    GateVerdict = "RED"
)

// R16StressJSONLTrace matches the exact JSONL format specified in R16 Section 16.
type R16StressJSONLTrace struct {
	RunID                 string              `json:"run_id"`
	TaskID                string              `json:"task_id"`
	Policy                string              `json:"policy"`
	Provider              string              `json:"provider"`
	Model                 string              `json:"model"`
	RequestedEffort       compute.EffortLevel `json:"requested_effort"`
	ActualReasoningTokens int64               `json:"actual_reasoning_tokens"`
	InputTokens           int64               `json:"input_tokens"`
	CachedInputTokens     int64               `json:"cached_input_tokens"`
	CacheWriteTokens      int64               `json:"cache_write_tokens"`
	VisibleOutputTokens   int64               `json:"visible_output_tokens"`
	ToolCalls             int                 `json:"tool_calls"`
	VerificationCalls     int                 `json:"verification_calls"`
	Escalations           int                 `json:"escalations"`
	TotalCostUSD          float64             `json:"total_cost_usd"`
	Quality               float64             `json:"quality"`
	QualityLCB            float64             `json:"quality_lcb"`
	CapabilityFloor       float64             `json:"capability_floor"`
	CapabilityFloorPass   bool                `json:"capability_floor_pass"`
	Success               bool                `json:"success"`
	TestsPassed           bool                `json:"tests_passed"`
	LatencyMS             float64             `json:"latency_ms"`
	OracleCostUSD         float64             `json:"oracle_cost_usd"`
}

// Ring0TrapResult records the outcome of Ring 0 safety and adversarial traps.
type Ring0TrapResult struct {
	TrapName             string  `json:"trap_name"`
	Passed               bool    `json:"passed"`
	ViolationCount       int     `json:"violation_count"`
	TargetFloor          float64 `json:"target_floor"`
	ObservedLCB          float64 `json:"observed_lcb"`
	SelectedModel        string  `json:"selected_model"`
	SelectedEffort       string  `json:"selected_effort"`
	UnderComputeDetected bool    `json:"under_compute_detected"`
	OverdraftDetected    bool    `json:"overdraft_detected"`
	Details              string  `json:"details"`
}

// LongHorizonTurnRecord tracks cumulative drift over multi-turn execution (Section 6).
type LongHorizonTurnRecord struct {
	TurnNumber        int     `json:"turn_number"`
	TurnAction        string  `json:"turn_action"`
	TurnCostUSD       float64 `json:"turn_cost_usd"`
	CumulativeCostUSD float64 `json:"cumulative_cost_usd"`
	Quality           float64 `json:"quality"`
	ReasoningTokens   int64   `json:"reasoning_tokens"`
	ContextTokens     int64   `json:"context_tokens"`
	CorrectAfterTurn  bool    `json:"correct_after_turn"`
}

// LongHorizonHorizonSummary captures cumulative economics for a horizon length (e.g. 10, 25, 50, 100 turns).
type LongHorizonHorizonSummary struct {
	HorizonTurns      int     `json:"horizon_turns"`
	TotalCostUSD      float64 `json:"total_cost_usd"`
	FinalQuality      float64 `json:"final_quality"`
	ReasoningDrift    float64 `json:"reasoning_drift"`
	ContextGrowthRate float64 `json:"context_growth_rate"`
	TaskRemainedSafe  bool    `json:"task_remained_safe"`
}

// SameTaskReasoningPoint records empirical performance for the Section 19 test-time compute curve.
type SameTaskReasoningPoint struct {
	EffortLevel     compute.EffortLevel `json:"effort_level"`
	ReasoningTokens int64               `json:"reasoning_tokens"`
	MeanQuality     float64             `json:"mean_quality"`
	QualityLCB      float64             `json:"quality_lcb"`
	CostUSD         float64             `json:"cost_usd"`
	LatencyMS       float64             `json:"latency_ms"`
	IsSufficient    bool                `json:"is_sufficient"`
}

// SameTaskComputeCurve captures the Section 19 falsification grid for a task.
type SameTaskComputeCurve struct {
	TaskID              string                   `json:"task_id"`
	Class               compute.TaskClass        `json:"class"`
	Difficulty          float64                  `json:"difficulty"`
	FloorQuality        float64                  `json:"floor_quality"`
	Points              []SameTaskReasoningPoint `json:"points"`
	ContextOSChoice     compute.EffortLevel      `json:"contextos_choice"`
	ContextOSChoiceCost float64                  `json:"contextos_choice_cost"`
	OracleChoice        compute.EffortLevel      `json:"oracle_choice"`
	OracleChoiceCost    float64                  `json:"oracle_choice_cost"`
	FloorSatisfied      bool                     `json:"floor_satisfied"`
	IsNearOptimal       bool                     `json:"is_near_optimal"` // within 20% of oracle cost
}

// FailureTaxonomySummary reports count and frequency for each failure classification.
type FailureTaxonomySummary struct {
	Category    FailureTaxonomyType `json:"category"`
	Count       int                 `json:"count"`
	Percentage  float64             `json:"percentage"`
	Description string              `json:"description"`
}

// BaselineComparisonSummary summarizes an evaluation baseline.
type BaselineComparisonSummary struct {
	Baseline               StressBaselineType      `json:"baseline"`
	Description            string                  `json:"description"`
	SuccessRate            float64                 `json:"success_rate"`
	TotalCostUSD           float64                 `json:"total_cost_usd"`
	AverageCostUSD         float64                 `json:"average_cost_usd"`
	CostPerSuccess         float64                 `json:"cost_per_success"`
	AverageReasoningTokens float64                 `json:"average_reasoning_tokens"`
	FloorViolations        int                     `json:"floor_violations"`
	CostBreakdown          telemetry.CostBreakdown `json:"cost_breakdown"`
}

// R16StressProtocolReport compiles the entire R16 Stress & Benchmark output.
type R16StressProtocolReport struct {
	RunID               string                                  `json:"run_id"`
	Timestamp           time.Time                               `json:"timestamp"`
	Metadata            providers.ExecutionMetadata             `json:"metadata"`
	Verdict             GateVerdict                             `json:"verdict"`
	VerdictReasons      []string                                `json:"verdict_reasons"`
	Ring0Traps          []Ring0TrapResult                       `json:"ring0_traps"`
	Ring0Passed         bool                                    `json:"ring0_passed"`
	Ring1TotalTasks     int                                     `json:"ring1_total_tasks"`
	Ring1ViolationRate  float64                                 `json:"ring1_violation_rate"`
	Ring1CostRegret     float64                                 `json:"ring1_cost_regret"`
	SameTaskCurves      []SameTaskComputeCurve                  `json:"same_task_compute_curves"`
	Baselines           map[StressBaselineType]BaselineComparisonSummary `json:"baselines"`
	FailureTaxonomy     []FailureTaxonomySummary                `json:"failure_taxonomy"`
	LongHorizon         []LongHorizonHorizonSummary             `json:"long_horizon"`
	ControllerOverhead  float64                                 `json:"controller_overhead_ratio"` // rho = C_controller / C_total
	AvoidableCostACR    float64                                 `json:"avoidable_cost_reduction"`
	FloorAblationDeltaQ float64                                 `json:"floor_ablation_delta_q"` // Q(B8) - Q(B8_NoFloor)
	TraceJSONLPath      string                                  `json:"trace_jsonl_path"`
}
