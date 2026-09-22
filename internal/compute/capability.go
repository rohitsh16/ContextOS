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
	Samples int `json:"samples"`
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
	Provider        string
	Model           string
	Effort          EffortLevel
	TaskClass       TaskClass
	Difficulty      float64
	QualityScore    float64
	Success         bool
	ActualCostUSD   float64
	ActualLatencyMS float64
}

// EmpiricalCapabilityEstimator aggregates actual measured benchmark observations.
type EmpiricalCapabilityEstimator struct {
	mu           sync.RWMutex
	observations map[string][]EmpiricalObservation // Key: "provider:model:effort"
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

	key := fmt.Sprintf("%s:%s:%s", obs.Provider, obs.Model, obs.Effort)
	e.observations[key] = append(e.observations[key], obs)
}

func (e *EmpiricalCapabilityEstimator) EstimateEnvelope(
	task TaskProfile,
	provider, model string,
	effort EffortLevel,
) CapabilityEnvelope {
	e.mu.RLock()
	key := fmt.Sprintf("%s:%s:%s", provider, model, effort)
	records, ok := e.observations[key]
	e.mu.RUnlock()

	// If fewer than 3 empirical observations exist, fall back to conservative synthetic envelope
	if !ok || len(records) < 3 {
		synth := e.fallback.EstimateEnvelope(task, provider, model, effort)
		synth.Samples = len(records)
		// Penalize LCB when empirical data is scarce (Invariant B)
		synth.QualityLCB = math.Max(synth.QualityLCB-0.08, 0.01)
		synth.SuccessLCB = math.Max(synth.SuccessLCB-0.08, 0.01)
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

	return CapabilityEnvelope{
		Provider:           provider,
		Model:              model,
		Effort:             effort,
		MeanQuality:        meanQ,
		QualityLCB:         qualityLCB,
		SuccessProbability: successProb,
		SuccessLCB:         successLCB,
		ExpectedCostUSD:    meanCost,
		ExpectedLatencyMS:  meanLat,
		QualityStdErr:      stdErr,
		Samples:            len(records),
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
