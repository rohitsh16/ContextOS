package compute

import (
	"math"
)

// CapabilityFloor defines the minimum acceptable capability and non-inferiority constraints (R16 Section 4).
type CapabilityFloor struct {
	// Minimum acceptable probability of task success.
	RequiredSuccessProbability float64 `json:"required_success_probability"`

	// Minimum acceptable continuous quality score in [0.0, 1.0].
	RequiredQuality float64 `json:"required_quality"`

	// Pre-registered non-inferiority margin delta_s vs baseline.
	NonInferiorityMargin float64 `json:"non_inferiority_margin"`

	// Conservatism multiplier for high-risk or mission-critical tasks.
	RiskMultiplier float64 `json:"risk_multiplier"`

	// Requires explicit static or execution verification before final delivery.
	RequireVerification bool `json:"require_verification"`
}

// ResolveCapabilityFloor maps a task's complexity profile and risk appetite to a concrete capability floor.
func ResolveCapabilityFloor(task TaskProfile, riskTarget float64) CapabilityFloor {
	if riskTarget <= 0 {
		riskTarget = 0.05 // Default 5% failure tolerance
	}

	riskMult := 1.0
	if riskTarget < 0.02 {
		riskMult = 1.20 // Extra conservatism for high-risk tasks (Invariant B)
	}

	switch task.Class {
	case T0Deterministic:
		// T0 deterministic: Exact result oracle invariant (zero tolerance for inaccuracy)
		return CapabilityFloor{
			RequiredSuccessProbability: 1.0,
			RequiredQuality:            1.0,
			NonInferiorityMargin:       0.0,
			RiskMultiplier:             1.0,
			RequireVerification:        false,
		}

	case T1Trivial:
		// T1 trivial: General non-inferiority margin
		return CapabilityFloor{
			RequiredSuccessProbability: math.Min(0.78*riskMult, 0.90),
			RequiredQuality:            math.Min(0.75*riskMult, 0.90),
			NonInferiorityMargin:       0.05,
			RiskMultiplier:             riskMult,
			RequireVerification:        false,
		}

	case T2Moderate:
		// T2 moderate: Standard bug fixes
		return CapabilityFloor{
			RequiredSuccessProbability: math.Min(0.82*riskMult, 0.92),
			RequiredQuality:            math.Min(0.80*riskMult, 0.92),
			NonInferiorityMargin:       0.03,
			RiskMultiplier:             riskMult,
			RequireVerification:        false,
		}

	case T3Difficult:
		// T3 difficult: Stricter non-inferiority
		return CapabilityFloor{
			RequiredSuccessProbability: math.Min(0.86*riskMult, 0.94),
			RequiredQuality:            math.Min(0.84*riskMult, 0.94),
			NonInferiorityMargin:       0.02,
			RiskMultiplier:             riskMult,
			RequireVerification:        task.Features.Risk > 0.50 || task.Difficulty > 0.70,
		}

	case T4Critical:
		// T4 critical: Strictest observed baseline + mandatory verification (Invariant C)
		return CapabilityFloor{
			RequiredSuccessProbability: math.Min(0.78*riskMult, 0.88),
			RequiredQuality:            math.Min(0.76*riskMult, 0.85),
			NonInferiorityMargin:       0.01,
			RiskMultiplier:             riskMult,
			RequireVerification:        true,
		}

	default:
		return CapabilityFloor{
			RequiredSuccessProbability: 0.85,
			RequiredQuality:            0.80,
			NonInferiorityMargin:       0.03,
			RiskMultiplier:             1.0,
			RequireVerification:        false,
		}
	}
}

// IsAdmissible verifies whether a capability envelope satisfies the floor (Invariant A & B).
func (f CapabilityFloor) IsAdmissible(env CapabilityEnvelope) (bool, string) {
	if env.QualityLCB < f.RequiredQuality {
		return false, "quality_lcb_below_floor"
	}
	if env.SuccessLCB < f.RequiredSuccessProbability {
		return false, "success_lcb_below_floor"
	}
	return true, "admissible"
}
