package compute

import (
	"fmt"
	"math"
	"sort"
	"sync"

	"contextos/internal/telemetry"
)

// CapabilityEnvelope represents the normalized capability, quality confidence bounds,
// and expected economics of an inference configuration (R16 Section 3).
type CapabilityEnvelope struct {
	Provider string      `json:"provider"`
	Model    string      `json:"model"`
	Effort   EffortLevel `json:"effort"`

	// Predicted / empirical quality metric in [0.0, 1.0].
	MeanQuality float64 `json:"mean_quality"`

	// Lower confidence bound (LCB) used for safety and feasibility gating.
	QualityLCB float64 `json:"quality_lcb"`

	// Probability of meeting task success criteria.
	SuccessProbability float64 `json:"success_probability"`
	SuccessLCB         float64 `json:"success_lcb"`

	// Expected total financial cost in USD.
	ExpectedCostUSD float64 `json:"expected_cost_usd"`

	// Expected total wall-clock latency in milliseconds.
	ExpectedLatencyMS float64 `json:"expected_latency_ms"`

	// Standard error of quality estimation.
	QualityStdErr float64 `json:"quality_std_err"`

	// Number of empirical observations supporting this envelope.
	Samples              int     `json:"samples"`
	EstimatorSource      string  `json:"estimator_source,omitempty"`
	FallbackLevel        int     `json:"fallback_level,omitempty"`
	EstimatorUncertainty float64 `json:"estimator_uncertainty,omitempty"`
	ModelVersion         string  `json:"model_version,omitempty"`
}

// CapabilityEstimator abstracts estimation of capability envelopes across models and effort tiers.
type CapabilityEstimator interface {
	EstimateEnvelope(task TaskProfile, provider, model string, effort EffortLevel) CapabilityEnvelope
	EstimateSurface(task TaskProfile, models []ModelCandidate) []CapabilityEnvelope
}

// SyntheticCapabilityEstimator computes envelopes using analytical curves for simulation/tests.
type SyntheticCapabilityEstimator struct {
	confidenceLevel float64 // e.g. 0.95 (z = 1.96)
}

// NewSyntheticCapabilityEstimator creates a synthetic estimator with 95% confidence bounds.
func NewSyntheticCapabilityEstimator() *SyntheticCapabilityEstimator {
	return &SyntheticCapabilityEstimator{confidenceLevel: 1.96}
}

func (s *SyntheticCapabilityEstimator) EstimateEnvelope(
	task TaskProfile,
	provider, model string,
	effort EffortLevel,
) CapabilityEnvelope {
	pricing, err := telemetry.DefaultPricingRegistry().LookupLatest(provider, model)
	if err != nil {
		pricing = telemetry.PricingEntry{
			Provider:        provider,
			Model:           model,
			InputPerMillion: 3.0, OutputPerMillion: 15.0, ReasoningPerMillion: 15.0,
		}
	}

	var rTokens int64
	var baseLatency float64
	switch effort {
	case EffortMinimal:
		rTokens = 0
		baseLatency = 250.0
	case EffortLow:
		rTokens = 2048
		baseLatency = 600.0
	case EffortMedium:
		rTokens = 8192
		baseLatency = 1800.0
	case EffortHigh:
		rTokens = 16384
		baseLatency = 3500.0
	case EffortMaximum:
		rTokens = 32768
		baseLatency = 6500.0
	}

	// Model strength tier multiplier
	strength := 1.0
	switch {
	case model == "o1" || model == "claude-3-7-sonnet":
		strength = 1.30
	case model == "o3-mini" || model == "claude-3-5-sonnet" || model == "gemini-2.0-flash-thinking" || model == "gemini-2.5-pro":
		strength = 1.10
	case model == "gpt-4o-mini" || model == "claude-3-5-haiku" || model == "gemini-2.0-flash" || model == "gemini-2.5-flash":
		strength = 0.85
	}

	// Base probability without reasoning: linear decay with task difficulty
	strengthRatio := strength / 1.30
	baseProb := (0.92 - (task.Difficulty * 0.55)) * (0.70 + 0.30*strengthRatio)

	// Test-time compute (reasoning) scaling: logarithmic returns with reasoning tokens
	var reasoningBonus float64
	if rTokens > 0 {
		tokensFactor := math.Log2(1.0+float64(rTokens)/256.0) / 7.0
		if tokensFactor > 1.0 {
			tokensFactor = 1.0
		}
		reasoningBonus = 0.42 * strengthRatio * tokensFactor
	}

	successProb := baseProb + reasoningBonus
	if successProb > 0.99 {
		successProb = 0.99
	}
	if successProb < 0.05 {
		successProb = 0.05
	}

	// Quality correlates with success with slight variance
	meanQuality := math.Min(successProb+0.02, 1.0)

	// Uncertainty: increases with difficulty, decreases with effort/samples
	syntheticSamples := 30
	stdErr := (0.06 * task.Difficulty) / math.Sqrt(float64(syntheticSamples))

	qualityLCB := math.Max(meanQuality-(s.confidenceLevel*stdErr), 0.01)
	successLCB := math.Max(successProb-(s.confidenceLevel*stdErr), 0.01)

	usage := telemetry.UsageMetrics{
		InputTokens:         1500,
		OutputTokens:        rTokens + 400,
		ReasoningTokens:     rTokens,
		VisibleOutputTokens: 400,
	}
	costUSD := telemetry.ComputeCostBreakdown(pricing, usage, 0, 0, 0, 0, 0).TotalUSD

	return CapabilityEnvelope{
		Provider:           provider,
		Model:              model,
		Effort:             effort,
		MeanQuality:        meanQuality,
		QualityLCB:         qualityLCB,
		SuccessProbability: successProb,
		SuccessLCB:         successLCB,
		ExpectedCostUSD:    costUSD,
		ExpectedLatencyMS:  baseLatency,
		QualityStdErr:      stdErr,
		Samples:            syntheticSamples,
	}
}

func (s *SyntheticCapabilityEstimator) EstimateSurface(task TaskProfile, models []ModelCandidate) []CapabilityEnvelope {
	efforts := []EffortLevel{EffortMinimal, EffortLow, EffortMedium, EffortHigh, EffortMaximum}
	var out []CapabilityEnvelope
	for _, m := range models {
		for _, e := range efforts {
			out = append(out, s.EstimateEnvelope(task, m.Provider, m.Model, e))
		}
	}
	return out
}

// EmpiricalObservation records a single observed task run outcome.
type EmpiricalObservation struct {
	TaskID                  string             `json:"task_id"`
	TaskFamily              string             `json:"task_family"`
	TaskClass               TaskClass          `json:"task_class"`
	DifficultyBucket        string             `json:"difficulty_bucket"`
	Difficulty              float64            `json:"difficulty"`
	Query                   string             `json:"query,omitempty"`
	RepoID                  string             `json:"repo_id,omitempty"`
	RepoRevision            string             `json:"repo_revision,omitempty"`
	TaskSeed                int64              `json:"task_seed,omitempty"`

	Provider                string             `json:"provider"`
	Model                   string             `json:"model"`
	ModelVersion            string             `json:"model_version"`
	RequestedEffort         EffortLevel        `json:"requested_effort"`
	RequestedReasoningBudget int64             `json:"requested_reasoning_budget,omitempty"`
	ExecutionMode           string             `json:"execution_mode,omitempty"`

	InputTokens             int64              `json:"input_tokens,omitempty"`
	CachedInputTokens       int64              `json:"cached_input_tokens,omitempty"`
	CacheWriteTokens        int64              `json:"cache_write_tokens,omitempty"`
	ReasoningTokens         int64              `json:"reasoning_tokens,omitempty"`
	VisibleOutputTokens     int64              `json:"visible_output_tokens,omitempty"`
	TotalTokens             int64              `json:"total_tokens,omitempty"`
	ToolCalls               int                `json:"tool_calls,omitempty"`
	Turns                   int                `json:"turns,omitempty"`
	ActualLatencyMS         float64            `json:"actual_latency_ms"`
	ActualCostUSD           float64            `json:"actual_cost_usd"`
	ProviderReportedCostUSD float64            `json:"provider_reported_cost_usd,omitempty"`

	Success                 bool               `json:"success"`
	TestsPassed             bool               `json:"tests_passed,omitempty"`
	QualityScore            float64            `json:"quality_score"`
	QualityComponents       map[string]float64 `json:"quality_components,omitempty"`
	VerificationResult      string             `json:"verification_result,omitempty"`

	Effort                  EffortLevel        `json:"effort"` // backward-compat alias
}

// EmpiricalCapabilityEstimator aggregates actual measured benchmark observations.
type EmpiricalCapabilityEstimator struct {
	mu           sync.RWMutex
	observations map[string][]EmpiricalObservation // task stratum + provider/model/version/effort
	fallback     *SyntheticCapabilityEstimator
}

// NewEmpiricalCapabilityEstimator creates an empirical estimator backed by observed data.
func NewEmpiricalCapabilityEstimator() *EmpiricalCapabilityEstimator {
	return &EmpiricalCapabilityEstimator{
		observations: make(map[string][]EmpiricalObservation),
		fallback:     NewSyntheticCapabilityEstimator(),
	}
}

// RecordObservation logs an actual benchmark execution result into the empirical store.
func (e *EmpiricalCapabilityEstimator) RecordObservation(obs EmpiricalObservation) {
	e.mu.Lock()
	defer e.mu.Unlock()

	key := empiricalKey(obs.TaskFamily, obs.DifficultyBucket, obs.TaskClass, obs.Provider, obs.Model, obs.ModelVersion, obs.Effort)
	e.observations[key] = append(e.observations[key], obs)
}

func empiricalKey(family, bucket string, class TaskClass, provider, model, version string, effort EffortLevel) string {
	return fmt.Sprintf("%s:%s:%d:%s:%s:%s:%s", family, bucket, class, provider, model, version, effort)
}

func empiricalBucket(d float64) string {
	if d < .35 {
		return "0.00-0.35"
	}
	if d < .60 {
		return "0.35-0.60"
	}
	if d < .75 {
		return "0.60-0.75"
	}
	return "0.75-1.00"
}

func (e *EmpiricalCapabilityEstimator) EstimateEnvelope(
	task TaskProfile,
	provider, model string,
	effort EffortLevel,
) CapabilityEnvelope {
	strata := []struct {
		key, source string
		level       int
	}{
		{empiricalKey(task.Family, empiricalBucket(task.Difficulty), task.Class, provider, model, "", effort), "exact", 0},
		{empiricalKey(task.Family, "", task.Class, provider, model, "", effort), "family", 1},
		{empiricalKey("", "", task.Class, provider, model, "", effort), "class", 2},
		{empiricalKey("", "", 0, provider, model, "", effort), "global", 3},
	}
	var records []EmpiricalObservation
	source, level := "prior", 4
	e.mu.RLock()
	for _, stratum := range strata {
		if rs := e.observations[stratum.key]; len(rs) > 0 {
			records, source, level = rs, stratum.source, stratum.level
			break
		}
	}
	e.mu.RUnlock()

	// If fewer than 3 empirical observations exist, fall back to conservative synthetic envelope
	if len(records) < 3 {
		synth := e.fallback.EstimateEnvelope(task, provider, model, effort)
		synth.Samples = len(records)
		// Penalize LCB when empirical data is scarce (Invariant B)
		synth.QualityLCB = math.Max(synth.QualityLCB-0.08, 0.01)
		synth.SuccessLCB = math.Max(synth.SuccessLCB-0.08, 0.01)
		synth.EstimatorSource, synth.FallbackLevel = source, level
		synth.EstimatorUncertainty = synth.QualityStdErr + 0.08
		return synth
	}

	var sumQ, sumCost, sumLat float64
	var successCount int

	for _, r := range records {
		sumQ += r.QualityScore
		sumCost += r.ActualCostUSD
		sumLat += r.ActualLatencyMS
		if r.Success {
			successCount++
		}
	}

	n := float64(len(records))
	meanQ := sumQ / n
	successProb := float64(successCount) / n
	meanCost := sumCost / n
	meanLat := sumLat / n

	// Calculate sample standard deviation
	var varSum float64
	for _, r := range records {
		diff := r.QualityScore - meanQ
		varSum += diff * diff
	}
	stdDev := math.Sqrt(varSum / math.Max(n-1, 1.0))
	stdErr := stdDev / math.Sqrt(n)

	// 95% Confidence LCB (t_crit ~ 1.96)
	qualityLCB := math.Max(meanQ-(1.96*stdErr), 0.01)
	successLCB := math.Max(successProb-(1.96*stdErr), 0.01)

	var modelVer string
	if len(records) > 0 {
		modelVer = records[0].ModelVersion
	}

	return CapabilityEnvelope{
		Provider:             provider,
		Model:                model,
		ModelVersion:         modelVer,
		Effort:               effort,
		MeanQuality:          meanQ,
		QualityLCB:           qualityLCB,
		SuccessProbability:   successProb,
		SuccessLCB:           successLCB,
		ExpectedCostUSD:      meanCost,
		ExpectedLatencyMS:    meanLat,
		QualityStdErr:        stdErr,
		EstimatorUncertainty: stdErr,
		Samples:              len(records),
		EstimatorSource:      source,
		FallbackLevel:        level,
	}
}

func (e *EmpiricalCapabilityEstimator) EstimateSurface(task TaskProfile, models []ModelCandidate) []CapabilityEnvelope {
	efforts := []EffortLevel{EffortMinimal, EffortLow, EffortMedium, EffortHigh, EffortMaximum}
	var out []CapabilityEnvelope
	for _, m := range models {
		for _, eff := range efforts {
			out = append(out, e.EstimateEnvelope(task, m.Provider, m.Model, eff))
		}
	}
	return out
}

// MinimumSufficientCompute calculates R*(s, m): the least effort level where QualityLCB >= floor (R16 Section 7).
func MinimumSufficientCompute(envelopes []CapabilityEnvelope, minQuality float64) (CapabilityEnvelope, bool) {
	// Filter admissible envelopes
	var admissible []CapabilityEnvelope
	for _, env := range envelopes {
		if env.QualityLCB >= minQuality {
			admissible = append(admissible, env)
		}
	}

	if len(admissible) == 0 {
		return CapabilityEnvelope{}, false
	}

	// Order by expected cost ascending
	sort.Slice(admissible, func(i, j int) bool {
		return admissible[i].ExpectedCostUSD < admissible[j].ExpectedCostUSD
	})

	return admissible[0], true
}
