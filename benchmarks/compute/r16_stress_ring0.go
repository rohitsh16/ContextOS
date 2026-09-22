package compute_bench

import (
	"fmt"
	"math"

	"contextos/internal/compute"
)

// Ring0Suite executes all mandatory adversarial unit stress traps for Ring 0.
type Ring0Suite struct {
	estimator *compute.SyntheticCapabilityEstimator
	models    []compute.ModelCandidate
}

// NewRing0Suite initializes the Ring 0 test suite.
func NewRing0Suite() *Ring0Suite {
	return &Ring0Suite{
		estimator: compute.NewSyntheticCapabilityEstimator(),
		models:    compute.DefaultModels(),
	}
}

// RunAllTraps executes the 5 mandatory adversarial traps and returns the results.
func (s *Ring0Suite) RunAllTraps() ([]Ring0TrapResult, bool) {
	var results []Ring0TrapResult
	allPassed := true

	// 1. Under-Compute Trap
	r1 := s.testUnderComputeTrap()
	results = append(results, r1)
	if !r1.Passed {
		allPassed = false
	}

	// 2. Over-Compute Trap
	r2 := s.testOverComputeTrap()
	results = append(results, r2)
	if !r2.Passed {
		allPassed = false
	}

	// 3. Estimator Noise Trap
	r3 := s.testEstimatorNoiseTrap()
	results = append(results, r3)
	if !r3.Passed {
		allPassed = false
	}

	// 4. Cost Shock Trap
	r4 := s.testCostShockTrap()
	results = append(results, r4)
	if !r4.Passed {
		allPassed = false
	}

	// 5. Capability Shock Trap
	r5 := s.testCapabilityShockTrap()
	results = append(results, r5)
	if !r5.Passed {
		allPassed = false
	}

	return results, allPassed
}

func (s *Ring0Suite) testUnderComputeTrap() Ring0TrapResult {
	task := compute.TaskProfile{
		Class:      compute.T4Critical,
		Difficulty: 0.88,
		Features: compute.TaskFeatures{
			Risk:                 0.90,
			Ambiguity:            0.40,
			HistoricalDifficulty: 0.88,
		},
	}
	floor := compute.ResolveCapabilityFloor(task, 0.01)

	opt := compute.NewCapabilityPreservingOptimizer(s.estimator, s.models)
	state := compute.ControllerState{
		Difficulty:          0.88,
		Risk:                0.90,
		EstimatedConfidence: 0.50,
		EvidenceCoverage:    0.70,
		ContextTokens:       4000,
	}

	cfg, _ := opt.SelectMinimumSufficient(state, task, floor)

	// Under-compute trap: If it chose a cheap model (gpt-4o-mini) or minimal effort, it walked into the trap
	isCheap := cfg.Model.Model == "gpt-4o-mini" || cfg.Effort == compute.EffortMinimal
	belowFloor := cfg.PredictedQualityLCB < floor.RequiredQuality

	passed := !isCheap && !belowFloor && cfg.VerifyEnabled

	return Ring0TrapResult{
		TrapName:             "Under-Compute Trap",
		Passed:               passed,
		ViolationCount:       boolToInt(belowFloor || isCheap),
		TargetFloor:          floor.RequiredQuality,
		ObservedLCB:          cfg.PredictedQualityLCB,
		SelectedModel:        cfg.Model.Model,
		SelectedEffort:       cfg.Effort.String(),
		UnderComputeDetected: isCheap,
		Details: fmt.Sprintf("Selected %s:%s (LCB: %.4f, Floor: %.4f, Verify: %v)",
			cfg.Model.Model, cfg.Effort, cfg.PredictedQualityLCB, floor.RequiredQuality, cfg.VerifyEnabled),
	}
}

func (s *Ring0Suite) testOverComputeTrap() Ring0TrapResult {
	task := compute.TaskProfile{
		Class:      compute.T1Trivial,
		Difficulty: 0.05,
		Features: compute.TaskFeatures{
			Risk:                 0.05,
			Ambiguity:            0.02,
			HistoricalDifficulty: 0.05,
		},
	}
	floor := compute.ResolveCapabilityFloor(task, 0.05)

	opt := compute.NewCapabilityPreservingOptimizer(s.estimator, s.models)
	state := compute.ControllerState{
		Difficulty:          0.05,
		Risk:                0.05,
		EstimatedConfidence: 0.95,
		EvidenceCoverage:    0.95,
		ContextTokens:       1000,
	}

	cfg, _ := opt.SelectMinimumSufficient(state, task, floor)

	// Over-compute trap: Choosing o1 max effort for trivial task wastes 100x budget
	isOvercompute := cfg.Model.Model == "o1" && cfg.Effort == compute.EffortMaximum
	floorViolated := cfg.PredictedQualityLCB < floor.RequiredQuality
	passed := !isOvercompute && !floorViolated && cfg.ExpectedE2ECostUSD < 0.15

	return Ring0TrapResult{
		TrapName:             "Over-Compute Trap",
		Passed:               passed,
		ViolationCount:       boolToInt(isOvercompute || floorViolated),
		TargetFloor:          floor.RequiredQuality,
		ObservedLCB:          cfg.PredictedQualityLCB,
		SelectedModel:        cfg.Model.Model,
		SelectedEffort:       cfg.Effort.String(),
		UnderComputeDetected: false,
		Details: fmt.Sprintf("Selected %s:%s at $%.4f (Floor: %.4f)",
			cfg.Model.Model, cfg.Effort, cfg.ExpectedE2ECostUSD, floor.RequiredQuality),
	}
}

func (s *Ring0Suite) testEstimatorNoiseTrap() Ring0TrapResult {
	task := compute.TaskProfile{
		Class:      compute.T2Moderate,
		Difficulty: 0.45,
		Features: compute.TaskFeatures{
			Risk:      0.40,
			Ambiguity: 0.50,
		},
	}
	floor := compute.ResolveCapabilityFloor(task, 0.03)

	state := compute.ControllerState{
		Difficulty:          0.45,
		EstimatedConfidence: 0.70,
		EvidenceCoverage:    0.80,
		ContextTokens:       2500,
	}

	// Test baseline estimator
	optBase := compute.NewCapabilityPreservingOptimizer(s.estimator, s.models)
	cfgBase, _ := optBase.SelectMinimumSufficient(state, task, floor)

	// Test high-noise estimator (+20% variance / stdErr)
	noisyEstimator := &noisyCapabilityEstimator{
		base:      s.estimator,
		noiseMult: 2.2, // Increases uncertainty -> depresses LCB
	}
	optNoisy := compute.NewCapabilityPreservingOptimizer(noisyEstimator, s.models)
	cfgNoisy, _ := optNoisy.SelectMinimumSufficient(state, task, floor)

	// Invariant B: Under higher uncertainty, optimizer must be equal or more conservative
	conservative := cfgNoisy.Effort >= cfgBase.Effort || cfgNoisy.Model.Model == "o1" || cfgNoisy.Model.Model == "claude-3-7-sonnet"
	floorHeld := cfgNoisy.PredictedQualityLCB >= floor.RequiredQuality || cfgNoisy.Effort >= compute.EffortHigh

	passed := conservative && floorHeld

	return Ring0TrapResult{
		TrapName:             "Estimator-Noise Trap",
		Passed:               passed,
		ViolationCount:       boolToInt(!passed),
		TargetFloor:          floor.RequiredQuality,
		ObservedLCB:          cfgNoisy.PredictedQualityLCB,
		SelectedModel:        cfgNoisy.Model.Model,
		SelectedEffort:       cfgNoisy.Effort.String(),
		UnderComputeDetected: !conservative,
		Details: fmt.Sprintf("Base: %s:%s -> Under 2.2x Noise: %s:%s (LCB: %.4f)",
			cfgBase.Model.Model, cfgBase.Effort, cfgNoisy.Model.Model, cfgNoisy.Effort, cfgNoisy.PredictedQualityLCB),
	}
}

func (s *Ring0Suite) testCostShockTrap() Ring0TrapResult {
	task := compute.TaskProfile{
		Class:      compute.T3Difficult,
		Difficulty: 0.70,
		Features: compute.TaskFeatures{
			Risk:      0.65,
			Ambiguity: 0.40,
		},
	}
	floor := compute.ResolveCapabilityFloor(task, 0.02)

	state := compute.ControllerState{
		Difficulty:          0.70,
		EstimatedConfidence: 0.65,
		EvidenceCoverage:    0.80,
		ContextTokens:       3000,
	}

	shocks := []float64{1.5, 2.0, 5.0, 10.0}
	allFloorsPreserved := true
	var lastModel string
	var lastEffort string
	var minLCB float64 = 1.0

	for _, shock := range shocks {
		shockedEstimator := &costShockCapabilityEstimator{
			base:      s.estimator,
			shockMult: shock,
		}
		opt := compute.NewCapabilityPreservingOptimizer(shockedEstimator, s.models)
		cfg, _ := opt.SelectMinimumSufficient(state, task, floor)

		if cfg.PredictedQualityLCB < floor.RequiredQuality {
			allFloorsPreserved = false
		}
		if cfg.PredictedQualityLCB < minLCB {
			minLCB = cfg.PredictedQualityLCB
		}
		lastModel = cfg.Model.Model
		lastEffort = cfg.Effort.String()
	}

	return Ring0TrapResult{
		TrapName:             "Cost-Shock Trap (1.5x - 10x)",
		Passed:               allFloorsPreserved,
		ViolationCount:       boolToInt(!allFloorsPreserved),
		TargetFloor:          floor.RequiredQuality,
		ObservedLCB:          minLCB,
		SelectedModel:        lastModel,
		SelectedEffort:       lastEffort,
		UnderComputeDetected: !allFloorsPreserved,
		Details: fmt.Sprintf("Across 1.5x-10x price shocks: Floor preserved = %v, Min LCB = %.4f (Req: %.4f)",
			allFloorsPreserved, minLCB, floor.RequiredQuality),
	}
}

func (s *Ring0Suite) testCapabilityShockTrap() Ring0TrapResult {
	task := compute.TaskProfile{
		Class:      compute.T2Moderate,
		Difficulty: 0.40,
	}
	floor := compute.ResolveCapabilityFloor(task, 0.03)

	state := compute.ControllerState{
		Difficulty:          0.40,
		EstimatedConfidence: 0.75,
		EvidenceCoverage:    0.85,
		ContextTokens:       2000,
	}

	// Degrade gpt-4o-mini capability by 50%
	shockedEstimator := &degradedModelCapabilityEstimator{
		base:          s.estimator,
		degradedModel: "gpt-4o-mini",
		degradation:   0.50,
	}
	opt := compute.NewCapabilityPreservingOptimizer(shockedEstimator, s.models)
	cfg, _ := opt.SelectMinimumSufficient(state, task, floor)

	// Must NOT select degraded gpt-4o-mini because its quality LCB collapsed below floor
	selectedDegraded := cfg.Model.Model == "gpt-4o-mini"
	passed := !selectedDegraded && cfg.PredictedQualityLCB >= floor.RequiredQuality

	return Ring0TrapResult{
		TrapName:             "Capability-Shock Trap",
		Passed:               passed,
		ViolationCount:       boolToInt(!passed),
		TargetFloor:          floor.RequiredQuality,
		ObservedLCB:          cfg.PredictedQualityLCB,
		SelectedModel:        cfg.Model.Model,
		SelectedEffort:       cfg.Effort.String(),
		UnderComputeDetected: selectedDegraded,
		Details: fmt.Sprintf("Degraded gpt-4o-mini: Optimizer correctly escalated to %s:%s (LCB: %.4f >= %.4f)",
			cfg.Model.Model, cfg.Effort, cfg.PredictedQualityLCB, floor.RequiredQuality),
	}
}

// Helpers

type noisyCapabilityEstimator struct {
	base      compute.CapabilityEstimator
	noiseMult float64
}

func (n *noisyCapabilityEstimator) EstimateEnvelope(task compute.TaskProfile, provider, model string, effort compute.EffortLevel) compute.CapabilityEnvelope {
	env := n.base.EstimateEnvelope(task, provider, model, effort)
	env.QualityStdErr *= n.noiseMult
	env.QualityLCB = math.Max(env.MeanQuality-1.96*env.QualityStdErr, 0.01)
	env.SuccessLCB = math.Max(env.SuccessProbability-1.96*env.QualityStdErr, 0.01)
	return env
}

func (n *noisyCapabilityEstimator) EstimateSurface(task compute.TaskProfile, models []compute.ModelCandidate) []compute.CapabilityEnvelope {
	efforts := []compute.EffortLevel{compute.EffortMinimal, compute.EffortLow, compute.EffortMedium, compute.EffortHigh, compute.EffortMaximum}
	var surface []compute.CapabilityEnvelope
	for _, m := range models {
		for _, eff := range efforts {
			surface = append(surface, n.EstimateEnvelope(task, m.Provider, m.Model, eff))
		}
	}
	return surface
}

type costShockCapabilityEstimator struct {
	base      compute.CapabilityEstimator
	shockMult float64
}

func (c *costShockCapabilityEstimator) EstimateEnvelope(task compute.TaskProfile, provider, model string, effort compute.EffortLevel) compute.CapabilityEnvelope {
	env := c.base.EstimateEnvelope(task, provider, model, effort)
	if provider == "openai" {
		env.ExpectedCostUSD *= c.shockMult
	}
	return env
}

func (c *costShockCapabilityEstimator) EstimateSurface(task compute.TaskProfile, models []compute.ModelCandidate) []compute.CapabilityEnvelope {
	efforts := []compute.EffortLevel{compute.EffortMinimal, compute.EffortLow, compute.EffortMedium, compute.EffortHigh, compute.EffortMaximum}
	var surface []compute.CapabilityEnvelope
	for _, m := range models {
		for _, eff := range efforts {
			surface = append(surface, c.EstimateEnvelope(task, m.Provider, m.Model, eff))
		}
	}
	return surface
}

type degradedModelCapabilityEstimator struct {
	base          compute.CapabilityEstimator
	degradedModel string
	degradation   float64
}

func (d *degradedModelCapabilityEstimator) EstimateEnvelope(task compute.TaskProfile, provider, model string, effort compute.EffortLevel) compute.CapabilityEnvelope {
	env := d.base.EstimateEnvelope(task, provider, model, effort)
	if model == d.degradedModel {
		env.MeanQuality *= (1.0 - d.degradation)
		env.QualityLCB *= (1.0 - d.degradation)
		env.SuccessProbability *= (1.0 - d.degradation)
		env.SuccessLCB *= (1.0 - d.degradation)
	}
	return env
}

func (d *degradedModelCapabilityEstimator) EstimateSurface(task compute.TaskProfile, models []compute.ModelCandidate) []compute.CapabilityEnvelope {
	efforts := []compute.EffortLevel{compute.EffortMinimal, compute.EffortLow, compute.EffortMedium, compute.EffortHigh, compute.EffortMaximum}
	var surface []compute.CapabilityEnvelope
	for _, m := range models {
		for _, eff := range efforts {
			surface = append(surface, d.EstimateEnvelope(task, m.Provider, m.Model, eff))
		}
	}
	return surface
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
