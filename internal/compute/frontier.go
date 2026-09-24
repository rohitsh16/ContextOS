package compute

import (
	"math"
	"sort"
)

// Frontier is an observed, task-local model/effort surface.  It is deliberately
// built from recorded outcomes; callers must not populate it from holdout labels.
type Frontier struct {
	TaskID         string          `json:"task_id"`
	Configurations []Configuration `json:"configurations"`
}

// FindMinimumSufficient selects the least-cost empirically feasible point.
func (f Frontier) FindMinimumSufficient(floor CapabilityFloor) (Configuration, bool) {
	feasible := make([]Configuration, 0, len(f.Configurations))
	for _, c := range f.Configurations {
		if ok, _ := floor.IsAdmissible(c.Envelope); ok {
			feasible = append(feasible, c)
		}
	}
	if len(feasible) == 0 {
		return Configuration{}, false
	}
	sort.SliceStable(feasible, func(i, j int) bool { return feasible[i].ExpectedE2ECostUSD < feasible[j].ExpectedE2ECostUSD })
	return feasible[0], true
}

// FindEmpiricalOracle has the same mathematical rule as production selection,
// but accepts only a pre-built observed frontier and therefore belongs in an
// offline evaluator, never the online estimator.
func (f Frontier) FindEmpiricalOracle(floor CapabilityFloor) (Configuration, bool) {
	return f.FindMinimumSufficient(floor)
}

// BuildAggregatedFrontier aggregates observations for a given taskID into an empirical Frontier.
// Repeated trials for the same (provider, model, effort) are combined:
// mean quality, mean cost, mean latency, and 95% LCB (using sample standard error) are computed.
func BuildAggregatedFrontier(taskID string, observations []EmpiricalObservation) Frontier {
	type key struct {
		provider string
		model    string
		effort   EffortLevel
	}
	grouped := make(map[key][]EmpiricalObservation)
	var orderedKeys []key

	for _, obs := range observations {
		if taskID != "" && obs.TaskID != taskID {
			continue
		}
		eff := obs.Effort
		if eff == 0 && obs.RequestedEffort != 0 {
			eff = obs.RequestedEffort
		}
		k := key{provider: obs.Provider, model: obs.Model, effort: eff}
		if _, exists := grouped[k]; !exists {
			orderedKeys = append(orderedKeys, k)
		}
		grouped[k] = append(grouped[k], obs)
	}

	frontier := Frontier{
		TaskID:         taskID,
		Configurations: make([]Configuration, 0, len(orderedKeys)),
	}

	for _, k := range orderedKeys {
		records := grouped[k]
		n := float64(len(records))
		var sumQ, sumCost, sumLat float64
		var successCount int
		var modelVer string

		for _, r := range records {
			sumQ += r.QualityScore
			sumCost += r.ActualCostUSD
			sumLat += r.ActualLatencyMS
			if r.Success {
				successCount++
			}
			if modelVer == "" && r.ModelVersion != "" {
				modelVer = r.ModelVersion
			}
		}

		meanQ := sumQ / n
		meanCost := sumCost / n
		meanLat := sumLat / n
		successProb := float64(successCount) / n

		var qualityLCB, successLCB, stdErrQ float64
		if n > 1 {
			var varSumQ, varSumS float64
			for _, r := range records {
				dq := r.QualityScore - meanQ
				varSumQ += dq * dq
				sVal := 0.0
				if r.Success {
					sVal = 1.0
				}
				ds := sVal - successProb
				varSumS += ds * ds
			}
			stdErrQ = math.Sqrt(varSumQ/(n-1)) / math.Sqrt(n)
			stdErrS := math.Sqrt(varSumS/(n-1)) / math.Sqrt(n)
			qualityLCB = math.Max(meanQ-1.96*stdErrQ, 0.01)
			successLCB = math.Max(successProb-1.96*stdErrS, 0.01)
		} else {
			// Single observation fallback: no sample variance
			qualityLCB = meanQ
			successLCB = successProb
		}

		env := CapabilityEnvelope{
			Provider:             k.provider,
			Model:                k.model,
			ModelVersion:         modelVer,
			Effort:               k.effort,
			MeanQuality:          meanQ,
			QualityLCB:           qualityLCB,
			SuccessProbability:   successProb,
			SuccessLCB:           successLCB,
			ExpectedCostUSD:      meanCost,
			ExpectedLatencyMS:    meanLat,
			QualityStdErr:        stdErrQ,
			EstimatorUncertainty: stdErrQ,
			Samples:              len(records),
			EstimatorSource:      "empirical_aggregate",
			FallbackLevel:        0,
		}

		frontier.Configurations = append(frontier.Configurations, Configuration{
			Model:               ModelCandidate{Provider: k.provider, Model: k.model},
			Effort:              k.effort,
			Envelope:            env,
			PredictedQualityLCB: qualityLCB,
			PredictedSuccessLCB: successLCB,
			ExpectedE2ECostUSD:  meanCost,
		})
	}

	sort.SliceStable(frontier.Configurations, func(i, j int) bool {
		if frontier.Configurations[i].ExpectedE2ECostUSD != frontier.Configurations[j].ExpectedE2ECostUSD {
			return frontier.Configurations[i].ExpectedE2ECostUSD < frontier.Configurations[j].ExpectedE2ECostUSD
		}
		if frontier.Configurations[i].Model.Provider != frontier.Configurations[j].Model.Provider {
			return frontier.Configurations[i].Model.Provider < frontier.Configurations[j].Model.Provider
		}
		if frontier.Configurations[i].Model.Model != frontier.Configurations[j].Model.Model {
			return frontier.Configurations[i].Model.Model < frontier.Configurations[j].Model.Model
		}
		return frontier.Configurations[i].Effort < frontier.Configurations[j].Effort
	})

	return frontier
}

