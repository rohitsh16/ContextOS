package compute

import (
	"testing"
)

func TestOptimizerInvariantA_RejectCheapBelowFloor(t *testing.T) {
	// Task requiring high quality (T3 Difficult)
	task := TaskProfile{Class: T3Difficult, Difficulty: 0.75}
	floor := ResolveCapabilityFloor(task, 0.05)

	estimator := NewSyntheticCapabilityEstimator()
	opt := NewCapabilityPreservingOptimizer(estimator, DefaultModels())

	state := ControllerState{
		EstimatedConfidence: 0.60,
		Difficulty:          0.75,
		EvidenceCoverage:    0.70,
		ContextTokens:       4000,
	}

	chosen, ok := opt.SelectMinimumSufficient(state, task, floor)
	if !ok {
		t.Fatalf("expected a valid configuration to be selected")
	}

	// Invariant A: The chosen configuration must NOT be a cheap weak model with minimal effort
	if chosen.Model.Model == "gpt-4o-mini" && chosen.Effort == EffortMinimal {
		t.Errorf("Invariant A violated: cheap-but-insufficient configuration was selected")
	}

	// The chosen envelope's quality LCB must meet or exceed the floor
	if chosen.PredictedQualityLCB < floor.RequiredQuality {
		t.Errorf("Invariant A violated: selected quality LCB %f < required %f",
			chosen.PredictedQualityLCB, floor.RequiredQuality)
	}
}

func TestOptimizerInvariantD_SelectCheapWhenSufficient(t *testing.T) {
	// Simple task (T1 Trivial) where small model with minimal effort easily meets floor
	task := TaskProfile{Class: T1Trivial, Difficulty: 0.05}
	floor := ResolveCapabilityFloor(task, 0.05)

	estimator := NewSyntheticCapabilityEstimator()
	opt := NewCapabilityPreservingOptimizer(estimator, DefaultModels())

	state := ControllerState{
		EstimatedConfidence: 0.90,
		Difficulty:          0.05,
		EvidenceCoverage:    0.95,
		ContextTokens:       1000,
	}

	chosen, ok := opt.SelectMinimumSufficient(state, task, floor)
	if !ok {
		t.Fatalf("expected valid configuration")
	}

	// Invariant D: For a trivial task, avoid expensive max reasoning models like o1 max
	if chosen.Effort == EffortMaximum && chosen.Model.Model == "o1" {
		t.Errorf("Invariant D violated: unnecessarily chose most expensive model/effort for trivial task")
	}
	if chosen.ExpectedE2ECostUSD > 0.10 {
		t.Errorf("expected cost < $0.10 for trivial task, got %f", chosen.ExpectedE2ECostUSD)
	}
}

func TestOptimizerInvariantC_HardTaskAllowedMoreCompute(t *testing.T) {
	// Mission-critical complex task (T4 Critical)
	task := TaskProfile{Class: T4Critical, Difficulty: 0.95, Features: TaskFeatures{Risk: 0.90}}
	floor := ResolveCapabilityFloor(task, 0.01)

	estimator := NewSyntheticCapabilityEstimator()
	opt := NewCapabilityPreservingOptimizer(estimator, DefaultModels())

	state := ControllerState{
		EstimatedConfidence: 0.40,
		Difficulty:          0.95,
		EvidenceCoverage:    0.50,
		ContextTokens:       8000,
	}

	chosen, _ := opt.SelectMinimumSufficient(state, task, floor)

	// Invariant C: Critical task must receive strong model / high reasoning / verification
	if chosen.Effort < EffortHigh && chosen.Model.Model != "o1" && chosen.Model.Model != "claude-3-7-sonnet" {
		t.Errorf("Invariant C violated: difficult task should receive high capability configuration, got %s:%s",
			chosen.Model.Model, chosen.Effort)
	}
	if !chosen.VerifyEnabled {
		t.Errorf("Invariant C violated: verification should be enabled for T4 task")
	}
}

func TestAdaptiveController_StepPreserving(t *testing.T) {
	ac := NewAdaptiveController()

	task := TaskProfile{
		Class:      T4Critical,
		Difficulty: 0.90,
		Features: TaskFeatures{
			Risk: 0.85,
		},
	}
	floor := ResolveCapabilityFloor(task, 0.01)

	state := ControllerState{
		Difficulty:          0.90,
		Risk:                0.85,
		EstimatedConfidence: 0.80,
		EvidenceCoverage:    0.90,
		ContextTokens:       4000,
		TurnCount:           1,
		Verified:            false, // Not yet verified
	}

	result := ac.StepPreserving(state, task, floor, EffortLow)

	if result.Configuration == nil {
		t.Fatalf("expected Configuration to be populated in StepPreserving result")
	}

	// For T4Critical with unverified state, controller must mandate verification before stopping
	if result.Action != ActionVerify {
		t.Errorf("expected ActionVerify for T4 task with Verified=false, got %s", result.Action)
	}

	// Mark verified and re-step
	state.Verified = true
	resultVerified := ac.StepPreserving(state, task, floor, result.Effort)
	if resultVerified.Action == ActionVerify {
		t.Errorf("expected action to proceed past verification once Verified=true")
	}
}
