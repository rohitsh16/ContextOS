package compute

import (
	"testing"
)

func TestResolveCapabilityFloor(t *testing.T) {
	// T0 Deterministic: Must enforce 1.0 success and 0 margin
	t0Task := TaskProfile{Class: T0Deterministic, Difficulty: 0.05}
	f0 := ResolveCapabilityFloor(t0Task, 0.05)
	if f0.RequiredSuccessProbability != 1.0 || f0.NonInferiorityMargin != 0.0 {
		t.Errorf("T0 floor mismatch: %+v", f0)
	}

	// T1 Trivial: Non-inferiority margin 0.05
	t1Task := TaskProfile{Class: T1Trivial, Difficulty: 0.10}
	f1 := ResolveCapabilityFloor(t1Task, 0.05)
	if f1.NonInferiorityMargin != 0.05 {
		t.Errorf("T1 margin expected 0.05, got %f", f1.NonInferiorityMargin)
	}

	// T4 Critical: Strictest margin 0.01 + mandatory verification
	t4Task := TaskProfile{Class: T4Critical, Difficulty: 0.90}
	f4 := ResolveCapabilityFloor(t4Task, 0.05)
	if f4.NonInferiorityMargin != 0.01 || !f4.RequireVerification {
		t.Errorf("T4 floor must require verification and strict 0.01 margin: %+v", f4)
	}
}

func TestCapabilityFloorAdmissibility(t *testing.T) {
	floor := CapabilityFloor{
		RequiredQuality:            0.80,
		RequiredSuccessProbability: 0.85,
	}

	// Case 1: Inadmissible due to low LCB
	badEnv := CapabilityEnvelope{
		MeanQuality:        0.85,
		QualityLCB:         0.78, // Below 0.80
		SuccessProbability: 0.90,
		SuccessLCB:         0.86,
	}
	admissible, reason := floor.IsAdmissible(badEnv)
	if admissible || reason != "quality_lcb_below_floor" {
		t.Errorf("expected rejection due to quality LCB, got %v (%s)", admissible, reason)
	}

	// Case 2: Admissible
	goodEnv := CapabilityEnvelope{
		MeanQuality:        0.90,
		QualityLCB:         0.82,
		SuccessProbability: 0.92,
		SuccessLCB:         0.88,
	}
	admissible, _ = floor.IsAdmissible(goodEnv)
	if !admissible {
		t.Errorf("expected goodEnv to be admissible")
	}
}
