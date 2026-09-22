package compute

import (
	"sort"
)

// Configuration represents a candidate inference and context configuration (R16 Section 8).
type Configuration struct {
	Model  ModelCandidate `json:"model"`
	Effort EffortLevel    `json:"effort"`

	MaxReasoningTokens *int64 `json:"max_reasoning_tokens,omitempty"`
	MaxOutputTokens    *int64 `json:"max_output_tokens,omitempty"`

	RetrieveBudget int  `json:"retrieve_budget"`
	VerifyEnabled  bool `json:"verify_enabled"`

	Envelope            CapabilityEnvelope `json:"envelope"`
	PredictedQualityLCB float64            `json:"predicted_quality_lcb"`
	PredictedSuccessLCB float64            `json:"predicted_success_lcb"`
	ExpectedE2ECostUSD  float64            `json:"expected_e2e_cost_usd"`
}

// CapabilityPreservingOptimizer selects the minimum-sufficient configuration satisfying task capability constraints.
type CapabilityPreservingOptimizer struct {
	estimator CapabilityEstimator
	models    []ModelCandidate
}

// NewCapabilityPreservingOptimizer creates an optimizer with a specific capability estimator and model catalog.
func NewCapabilityPreservingOptimizer(
	estimator CapabilityEstimator,
	models []ModelCandidate,
) *CapabilityPreservingOptimizer {
	if len(models) == 0 {
		models = DefaultModels()
	}
	if estimator == nil {
		estimator = NewSyntheticCapabilityEstimator()
	}
	return &CapabilityPreservingOptimizer{
		estimator: estimator,
		models:    models,
	}
}

// FeasibleConfigurations generates all potential configurations and filters out those below the capability floor.
func (o *CapabilityPreservingOptimizer) FeasibleConfigurations(
	state ControllerState,
	task TaskProfile,
	floor CapabilityFloor,
) []Configuration {
	efforts := []EffortLevel{EffortMinimal, EffortLow, EffortMedium, EffortHigh, EffortMaximum}
	var feasible []Configuration

	for _, m := range o.models {
		for _, eff := range efforts {
			env := o.estimator.EstimateEnvelope(task, m.Provider, m.Model, eff)

			// Step 1 & 2: Invariant A — Reject if lower confidence bound is below the floor
			if admissible, _ := floor.IsAdmissible(env); !admissible {
				continue
			}

			// Calculate full E2E cost (model inference + retrieval + verification)
			e2eCost := env.ExpectedCostUSD
			verifyNeeded := floor.RequireVerification || (task.Difficulty > 0.60 && eff >= EffortMedium)
			if verifyNeeded {
				e2eCost += 0.015 // Additional verification cost
			}
			if state.EvidenceCoverage < 0.80 {
				e2eCost += 0.005 // Retrieval expansion
			}

			cfg := Configuration{
				Model:               m,
				Effort:              eff,
				RetrieveBudget:      int(state.ContextTokens),
				VerifyEnabled:       verifyNeeded,
				Envelope:            env,
				PredictedQualityLCB: env.QualityLCB,
				PredictedSuccessLCB: env.SuccessLCB,
				ExpectedE2ECostUSD:  e2eCost,
			}
			feasible = append(feasible, cfg)
		}
	}

	return feasible
}

// SelectMinimumSufficient solves: min E[C_E2E] subject to Q_LCB >= Q_min (R16 Section 6 & 8).
func (o *CapabilityPreservingOptimizer) SelectMinimumSufficient(
	state ControllerState,
	task TaskProfile,
	floor CapabilityFloor,
) (Configuration, bool) {
	// Invariant: T0 deterministic bypass (R16 Section 14)
	if task.CanBypass && task.Class == T0Deterministic {
		env := CapabilityEnvelope{
			Provider:           "local",
			Model:              "deterministic-engine",
			Effort:             EffortMinimal,
			MeanQuality:        1.0,
			QualityLCB:         1.0,
			SuccessProbability: 1.0,
			SuccessLCB:         1.0,
			ExpectedCostUSD:    0.00005,
			ExpectedLatencyMS:  4.5,
			Samples:            100,
		}
		return Configuration{
			Model: ModelCandidate{
				Provider: "local",
				Model:    "deterministic-engine",
			},
			Effort:              EffortMinimal,
			RetrieveBudget:      int(state.ContextTokens),
			VerifyEnabled:       false,
			Envelope:            env,
			PredictedQualityLCB: 1.0,
			PredictedSuccessLCB: 1.0,
			ExpectedE2ECostUSD:  0.00005,
		}, true
	}

	feasible := o.FeasibleConfigurations(state, task, floor)

	// If no configuration satisfies the capability floor, Invariant B dictates choosing
	// the highest-capability configuration available rather than failing or choosing cheap.
	if len(feasible) == 0 {
		bestEnv := o.findHighestCapability(task)
		return Configuration{
			Model: ModelCandidate{
				Provider: bestEnv.Provider,
				Model:    bestEnv.Model,
			},
			Effort:              bestEnv.Effort,
			RetrieveBudget:      int(state.ContextTokens),
			VerifyEnabled:       true,
			Envelope:            bestEnv,
			PredictedQualityLCB: bestEnv.QualityLCB,
			PredictedSuccessLCB: bestEnv.SuccessLCB,
			ExpectedE2ECostUSD:  bestEnv.ExpectedCostUSD + 0.020,
		}, false
	}

	// Order by E2E cost ascending
	sort.Slice(feasible, func(i, j int) bool {
		// Primary sort: Lowest cost
		if feasible[i].ExpectedE2ECostUSD != feasible[j].ExpectedE2ECostUSD {
			return feasible[i].ExpectedE2ECostUSD < feasible[j].ExpectedE2ECostUSD
		}
		// Tie breaker: Higher quality LCB
		return feasible[i].PredictedQualityLCB > feasible[j].PredictedQualityLCB
	})

	// Invariant B: Under high uncertainty (QualityStdErr > 0.08), err toward safer higher-capability option
	chosen := feasible[0]
	if chosen.Envelope.QualityStdErr > 0.08 && len(feasible) > 1 {
		// Prefer configuration with higher samples or higher LCB if cost delta is moderate (< $0.05)
		for _, alt := range feasible[1:] {
			if alt.PredictedQualityLCB > chosen.PredictedQualityLCB+0.05 &&
				alt.ExpectedE2ECostUSD-chosen.ExpectedE2ECostUSD < 0.05 {
				chosen = alt
				break
			}
		}
	}

	return chosen, true
}

func (o *CapabilityPreservingOptimizer) findHighestCapability(task TaskProfile) CapabilityEnvelope {
	efforts := []EffortLevel{EffortMaximum, EffortHigh, EffortMedium}
	var best CapabilityEnvelope
	for _, m := range o.models {
		for _, eff := range efforts {
			env := o.estimator.EstimateEnvelope(task, m.Provider, m.Model, eff)
			if env.MeanQuality > best.MeanQuality {
				best = env
			}
		}
	}
	return best
}
