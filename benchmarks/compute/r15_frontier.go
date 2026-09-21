package compute_bench

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"contextos/internal/compute"
	"contextos/internal/telemetry"
)

// EffortPointMetrics holds aggregated evaluation metrics for a single fixed effort level.
type EffortPointMetrics struct {
	EffortLevel        string  `json:"effort_level"`
	EffortIndex        int     `json:"effort_index"`
	AverageAccuracy    float64 `json:"average_accuracy"`
	TotalReasoningToks int64   `json:"total_reasoning_tokens"`
	AvgReasoningToks   float64 `json:"avg_reasoning_tokens_per_task"`
	TotalCostUSD       float64 `json:"total_cost_usd"`
	AvgCostUSD         float64 `json:"avg_cost_per_task_usd"`
	TotalSuccessTasks  float64 `json:"total_successful_tasks"`
	CPSUSD             float64 `json:"cps_usd"`
	AvgLatencyMS       float64 `json:"avg_latency_ms"`
	MarginalGain       float64 `json:"marginal_success_gain"`  // MSG(e) = A(e) - A(e-1)
	MarginalCostUSD    float64 `json:"marginal_cost_usd"`      // MC(e) = C(e) - C(e-1)
	ComputeEfficiency  float64 `json:"compute_efficiency_pts"` // CE(e) = MSG(e) / MC(e)
	IsKnee             bool    `json:"is_diminishing_returns_knee"`
}

// TaskFrontierPoint stores per-task evaluation across all 5 effort levels.
type TaskFrontierPoint struct {
	TaskID             string             `json:"task_id"`
	Class              string             `json:"class"`
	Difficulty         float64            `json:"difficulty"`
	CanBypass          bool               `json:"can_bypass"`
	PointsByEffort     map[string]TaskRun `json:"points_by_effort"`
	RecommendedKneeLvl string             `json:"recommended_knee_level"`
}

// TaskRun captures a single task at a specific effort level.
type TaskRun struct {
	Effort          string  `json:"effort"`
	ReasoningTokens int64   `json:"reasoning_tokens"`
	CostUSD         float64 `json:"cost_usd"`
	Accuracy        float64 `json:"accuracy"`
	LatencyMS       float64 `json:"latency_ms"`
}

// R15FrontierReport contains the complete Phase R15.3 analysis.
type R15FrontierReport struct {
	ManifestID        string               `json:"manifest_id"`
	BenchmarkVersion  string               `json:"benchmark_version"`
	Timestamp         time.Time            `json:"timestamp"`
	Gate              string               `json:"gate"`
	TotalTasks        int                  `json:"total_tasks"`
	EffortLevels      []string             `json:"effort_levels"`
	FrontierSummary   []EffortPointMetrics `json:"frontier_summary"`
	OptimalFixedLevel string               `json:"optimal_fixed_effort_level"`
	OptimalFixedCPS   float64              `json:"optimal_fixed_cps_usd"`
	TaskFrontiers     []TaskFrontierPoint  `json:"task_frontiers"`
}

// RunR15_3_Frontier evaluates the 120-task matrix on the complete fixed-effort grid.
func RunR15_3_Frontier(matrix []TaskMatrixItem, resultsDir string) (*R15FrontierReport, error) {
	effortLevels := []compute.EffortLevel{
		compute.EffortMinimal,
		compute.EffortLow,
		compute.EffortMedium,
		compute.EffortHigh,
		compute.EffortMaximum,
	}

	effortNames := []string{"minimal", "low", "medium", "high", "maximum"}
	estimator := compute.NewComputeEstimator(0.50)
	pricing, _ := telemetry.LookupPricing("anthropic", "claude-3-7-sonnet")

	// Collect per-task curves and aggregate stats
	taskFrontiers := make([]TaskFrontierPoint, len(matrix))
	aggPoints := make([]EffortPointMetrics, len(effortLevels))

	for eIdx := range effortLevels {
		aggPoints[eIdx] = EffortPointMetrics{
			EffortLevel: effortNames[eIdx],
			EffortIndex: eIdx,
		}
	}

	for tIdx, task := range matrix {
		curve := estimator.EstimateCurve("anthropic", "claude-3-7-sonnet", task.MeasuredDifficulty)
		pointsMap := make(map[string]TaskRun)
		bestCE := -1.0
		kneeLvl := "medium"

		for eIdx, eLvl := range effortLevels {
			pt := curve[int(eLvl)]
			reasoning := pt.ReasoningTokens
			inputTokens := int64(1500)
			outputTokens := reasoning + 300

			// Cost calculation
			usage := telemetry.UsageMetrics{
				InputTokens:     inputTokens,
				OutputTokens:    outputTokens,
				ReasoningTokens: reasoning,
			}
			cost := telemetry.CalculateUsageCost(pricing, usage)
			accuracy := pt.EstimatedSuccess
			latency := float64(800 + (eIdx * 900))

			pointsMap[effortNames[eIdx]] = TaskRun{
				Effort:          effortNames[eIdx],
				ReasoningTokens: reasoning,
				CostUSD:         cost,
				Accuracy:        accuracy,
				LatencyMS:       latency,
			}

			// Aggregate
			aggPoints[eIdx].AverageAccuracy += accuracy
			aggPoints[eIdx].TotalReasoningToks += reasoning
			aggPoints[eIdx].TotalCostUSD += cost
			aggPoints[eIdx].TotalSuccessTasks += accuracy
			aggPoints[eIdx].AvgLatencyMS += latency

			if eIdx > 0 {
				gain := pt.EstimatedSuccess - curve[eIdx-1].EstimatedSuccess
				if gain > bestCE && gain > 0.05 {
					bestCE = gain
					kneeLvl = effortNames[eIdx]
				}
			}
		}

		taskFrontiers[tIdx] = TaskFrontierPoint{
			TaskID:             task.ID,
			Class:              task.Class.String(),
			Difficulty:         task.MeasuredDifficulty,
			CanBypass:          task.CanBypass,
			PointsByEffort:     pointsMap,
			RecommendedKneeLvl: kneeLvl,
		}
	}

	numTasks := float64(len(matrix))
	optimalFixedLvl := "low"
	minCPS := 1e9

	for eIdx := range aggPoints {
		aggPoints[eIdx].AverageAccuracy /= numTasks
		aggPoints[eIdx].AvgReasoningToks = float64(aggPoints[eIdx].TotalReasoningToks) / numTasks
		aggPoints[eIdx].AvgCostUSD = aggPoints[eIdx].TotalCostUSD / numTasks
		aggPoints[eIdx].AvgLatencyMS /= numTasks

		if aggPoints[eIdx].TotalSuccessTasks > 0 {
			aggPoints[eIdx].CPSUSD = aggPoints[eIdx].TotalCostUSD / aggPoints[eIdx].TotalSuccessTasks
		}

		if eIdx > 0 {
			msg := aggPoints[eIdx].AverageAccuracy - aggPoints[eIdx-1].AverageAccuracy
			mc := aggPoints[eIdx].AvgCostUSD - aggPoints[eIdx-1].AvgCostUSD
			aggPoints[eIdx].MarginalGain = msg
			aggPoints[eIdx].MarginalCostUSD = mc
			if mc > 0 {
				aggPoints[eIdx].ComputeEfficiency = msg / mc
			}
			// Diminishing returns knee defined when CE drops sharply (CE < 10.0 pts/$)
			if aggPoints[eIdx].ComputeEfficiency < 10.0 && eIdx >= 2 {
				aggPoints[eIdx].IsKnee = true
			}
		}

		if aggPoints[eIdx].CPSUSD < minCPS && aggPoints[eIdx].CPSUSD > 0 {
			minCPS = aggPoints[eIdx].CPSUSD
			optimalFixedLvl = aggPoints[eIdx].EffortLevel
		}
	}

	report := &R15FrontierReport{
		ManifestID:        "manifest-r15-freeze-42",
		BenchmarkVersion:  "R15.0-alpha",
		Timestamp:         time.Now().UTC(),
		Gate:              "R15.3",
		TotalTasks:        len(matrix),
		EffortLevels:      effortNames,
		FrontierSummary:   aggPoints,
		OptimalFixedLevel: optimalFixedLvl,
		OptimalFixedCPS:   minCPS,
		TaskFrontiers:     taskFrontiers,
	}

	if resultsDir != "" {
		_ = os.MkdirAll(resultsDir, 0755)
		path := filepath.Join(resultsDir, "r15_3_effort_frontier.json")
		b, err := json.MarshalIndent(report, "", "  ")
		if err == nil {
			_ = os.WriteFile(path, b, 0644)
		}
	}

	return report, nil
}
