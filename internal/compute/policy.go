package compute

import (
	"time"

	"contextos/internal/providers"
)

// TaskClass represents discrete difficulty tiers for engineering tasks.
type TaskClass int

const (
	T0Deterministic TaskClass = iota // Direct symbol lookup, imports, syntax checks (bypass LLM)
	T1Trivial                         // Simple typo, documentation query, 1-line edit
	T2Moderate                        // Local function fix, test failure, small refactor
	T3Difficult                       // Multi-file refactor, race conditions, architecture changes
	T4Critical                        // Distributed debugging, security audit, repository migration
)

func (t TaskClass) String() string {
	switch t {
	case T0Deterministic:
		return "T0-deterministic"
	case T1Trivial:
		return "T1-trivial"
	case T2Moderate:
		return "T2-moderate"
	case T3Difficult:
		return "T3-difficult"
	case T4Critical:
		return "T4-critical"
	default:
		return "T2-moderate"
	}
}

// ComputePolicy is re-exported from providers package.
type ComputePolicy = providers.ComputePolicy

// DeterministicPolicyBaseline maps a task class into the standard baseline effort level.
func DeterministicPolicyBaseline(class TaskClass) EffortLevel {
	switch class {
	case T0Deterministic:
		return EffortMinimal
	case T1Trivial:
		return EffortLow
	case T2Moderate:
		return EffortMedium
	case T3Difficult:
		return EffortHigh
	case T4Critical:
		return EffortMaximum
	default:
		return EffortMedium
	}
}

// DefaultPolicyForTask produces an initial ComputePolicy given a task class and risk target.
func DefaultPolicyForTask(class TaskClass, riskTarget float64) ComputePolicy {
	effort := DeterministicPolicyBaseline(class)
	var maxReasoningTokens *int64
	switch effort {
	case EffortMinimal:
		zero := int64(0)
		maxReasoningTokens = &zero
	case EffortLow:
		t := int64(2048)
		maxReasoningTokens = &t
	case EffortMedium:
		t := int64(8192)
		maxReasoningTokens = &t
	case EffortHigh:
		t := int64(16384)
		maxReasoningTokens = &t
	case EffortMaximum:
		t := int64(32768)
		maxReasoningTokens = &t
	}

	if riskTarget <= 0 {
		riskTarget = 0.05 // default 5% error risk tolerance (95% reliability)
	}

	return ComputePolicy{
		Effort:             effort,
		Adaptive:           true,
		MaxReasoningTokens: maxReasoningTokens,
		RiskTarget:         riskTarget,
		Timeout:            30 * time.Second,
	}
}
