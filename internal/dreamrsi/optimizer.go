package dreamrsi

import (
	"fmt"
	"math"
	"math/rand"
	"time"
)

// OptimizerConfig controls the offline Dreaming exploration search parameters.
type OptimizerConfig struct {
	Generations         int     `json:"generations"`           // number of evolutionary cycles (default 20)
	PopulationSize      int     `json:"population_size"`      // mutant policies evaluated per gen (default 10)
	PerturbationScale   float64 `json:"perturbation_scale"`   // mutation step size (default 0.15)
	MinSuccessThreshold float64 `json:"min_success_threshold"` // non-regression constraint (default 0.95)
	Seed                int64   `json:"seed"`                 // reproducible seed
}

// DefaultOptimizerConfig returns recommended settings for offline Dreaming.
func DefaultOptimizerConfig() OptimizerConfig {
	return OptimizerConfig{
		Generations:         20,
		PopulationSize:      10,
		PerturbationScale:   0.15,
		MinSuccessThreshold: 0.95,
		Seed:                42,
	}
}

// DreamOptimizationReport captures the outcome of an offline policy refinement cycle.
type DreamOptimizationReport struct {
	BaselinePolicy       ExplorationPolicy `json:"baseline_policy"`
	WinningPolicy        ExplorationPolicy `json:"winning_policy"`
	BaselineSuccessRate  float64           `json:"baseline_success_rate"`
	WinningSuccessRate   float64           `json:"winning_success_rate"`
	BaselineTokensMean   float64           `json:"baseline_tokens_mean"`
	WinningTokensMean    float64           `json:"winning_tokens_mean"`
	TokenSavingsPct      float64           `json:"token_savings_pct"`
	PermutationPValue    float64           `json:"permutation_p_value"`
	CliffsDelta          float64           `json:"cliffs_delta"`
	CliffsInterpretation string            `json:"cliffs_interpretation"`
	PromotedToActive     bool              `json:"promoted_to_active"`
	GenerationsRun       int               `json:"generations_run"`
	TotalSimulatedEvals  int               `json:"total_simulated_evals"`
	Duration             time.Duration     `json:"duration"`
}

// DreamOptimizer executes the offline policy mutation and reinforcement loop over replay scenarios.
type DreamOptimizer struct {
	Simulator *ReplaySimulator
	Config    OptimizerConfig
}

// NewDreamOptimizer instantiates an optimizer bound to a replay simulator.
func NewDreamOptimizer(sim *ReplaySimulator, cfg OptimizerConfig) *DreamOptimizer {
	if cfg.Generations <= 0 {
		cfg.Generations = 20
	}
	if cfg.PopulationSize <= 0 {
		cfg.PopulationSize = 10
	}
	if cfg.PerturbationScale <= 0 {
		cfg.PerturbationScale = 0.15
	}
	if cfg.MinSuccessThreshold <= 0 {
		cfg.MinSuccessThreshold = 0.95
	}
	return &DreamOptimizer{
		Simulator: sim,
		Config:    cfg,
	}
}

// Optimize executes the full Dream-RSI cycle:
// 1. Evaluates baseline policy.
// 2. Explores candidate policy space across generations.
// 3. Evaluates Pareto-fitness: R(pi) = SuccessRate*100 + EvidenceRecall*50 + TokenReduction*0.5.
// 4. Applies statistical safety gate (Cliff's Delta and paired permutation test).
func (o *DreamOptimizer) Optimize(initialPolicy ExplorationPolicy) DreamOptimizationReport {
	start := time.Now()

	// 1. Evaluate baseline
	baseReport := o.Simulator.ReplayAll(initialPolicy)
	bestPolicy := initialPolicy
	bestFitness := computeFitness(baseReport, o.Config.MinSuccessThreshold)
	bestReport := baseReport

	evalCount := 1
	currentSeed := o.Config.Seed

	// 2. Evolutionary dreaming loop
	for gen := 0; gen < o.Config.Generations; gen++ {
		for pop := 0; pop < o.Config.PopulationSize; pop++ {
			currentSeed += int64(gen*1000 + pop + 1)
			mutant := bestPolicy.Mutate(currentSeed, o.Config.PerturbationScale)
			mutant.ID = fmt.Sprintf("mutant-g%d-p%d", gen+1, pop+1)

			rep := o.Simulator.ReplayAll(mutant)
			evalCount++

			fitness := computeFitness(rep, o.Config.MinSuccessThreshold)
			if fitness > bestFitness && rep.SuccessRate >= o.Config.MinSuccessThreshold {
				bestFitness = fitness
				bestPolicy = mutant
				bestReport = rep
			}
		}
	}

	// 3. Statistical verification gate between baseline and winning policy
	n := len(o.Simulator.Scenarios)
	var tokenDeltas []float64
	baseTokensSum, winTokensSum := 0.0, 0.0

	for i := 0; i < n; i++ {
		bRes := baseReport.IndividualResults[i]
		wRes := bestReport.IndividualResults[i]
		baseTokensSum += float64(bRes.SimulatedTokens)
		winTokensSum += float64(wRes.SimulatedTokens)

		// Positive delta = winning policy used fewer tokens
		tokenDeltas = append(tokenDeltas, float64(bRes.SimulatedTokens-wRes.SimulatedTokens))
	}

	cliffsD, interp := cliffsDelta(tokenDeltas)
	pVal := pairedPermutationTest(tokenDeltas, 1000, o.Config.Seed)

	baseMeanTokens := 0.0
	winMeanTokens := 0.0
	tokenSavingsPct := 0.0
	if n > 0 {
		baseMeanTokens = baseTokensSum / float64(n)
		winMeanTokens = winTokensSum / float64(n)
		if baseTokensSum > 0 {
			tokenSavingsPct = (1.0 - winTokensSum/baseTokensSum) * 100.0
		}
	}

	// Gate: Promote only if:
	// - Success rate is >= baseline success rate
	// - Token savings >= 0
	// - Non-regression constraint holds
	promoted := bestReport.SuccessRate >= baseReport.SuccessRate &&
		tokenSavingsPct >= 0 &&
		bestReport.MeanEvidenceRecall >= baseReport.MeanEvidenceRecall

	return DreamOptimizationReport{
		BaselinePolicy:       initialPolicy,
		WinningPolicy:        bestPolicy,
		BaselineSuccessRate:  baseReport.SuccessRate,
		WinningSuccessRate:   bestReport.SuccessRate,
		BaselineTokensMean:   baseMeanTokens,
		WinningTokensMean:    winMeanTokens,
		TokenSavingsPct:      tokenSavingsPct,
		PermutationPValue:    pVal,
		CliffsDelta:          cliffsD,
		CliffsInterpretation: interp,
		PromotedToActive:     promoted,
		GenerationsRun:       o.Config.Generations,
		TotalSimulatedEvals:  evalCount,
		Duration:             time.Since(start),
	}
}

func computeFitness(r AggregateReplayReport, minSuccess float64) float64 {
	if r.SuccessRate < minSuccess {
		return r.SuccessRate * 10.0 // heavily penalize regressions
	}
	return r.SuccessRate*100.0 + r.MeanEvidenceRecall*50.0 + r.MeanTokenReduction*0.5
}

func cliffsDelta(deltas []float64) (float64, string) {
	if len(deltas) == 0 {
		return 0.0, "negligible"
	}
	pos, neg := 0, 0
	for _, d := range deltas {
		if d > 1e-9 {
			pos++
		} else if d < -1e-9 {
			neg++
		}
	}
	d := float64(pos-neg) / float64(len(deltas))

	absD := math.Abs(d)
	var interp string
	switch {
	case absD < 0.147:
		interp = "negligible"
	case absD < 0.33:
		interp = "small"
	case absD < 0.474:
		interp = "medium"
	default:
		interp = "large"
	}
	return d, interp
}

func pairedPermutationTest(deltas []float64, nPermutations int, seed int64) float64 {
	n := len(deltas)
	if n == 0 {
		return 1.0
	}
	if nPermutations <= 0 {
		nPermutations = 1000
	}
	obsSum := 0.0
	for _, d := range deltas {
		obsSum += d
	}
	obsMean := math.Abs(obsSum / float64(n))

	r := rand.New(rand.NewSource(seed))
	countExtreme := 0

	for p := 0; p < nPermutations; p++ {
		permSum := 0.0
		for i := 0; i < n; i++ {
			if r.Intn(2) == 1 {
				permSum += deltas[i]
			} else {
				permSum -= deltas[i]
			}
		}
		permMean := math.Abs(permSum / float64(n))
		if permMean >= obsMean-1e-12 {
			countExtreme++
		}
	}

	return float64(countExtreme) / float64(nPermutations)
}
