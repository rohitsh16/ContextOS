package compute

import (
	"contextos/internal/providers"
)

// EffortLevel is re-exported from providers package for compute controller convenience.
type EffortLevel = providers.EffortLevel

const (
	EffortMinimal = providers.EffortMinimal
	EffortLow     = providers.EffortLow
	EffortMedium  = providers.EffortMedium
	EffortHigh    = providers.EffortHigh
	EffortMaximum = providers.EffortMaximum
)

// NormalizedEffort tracks requested effort level and provides resolution against model capabilities.
type NormalizedEffort struct {
	Level              EffortLevel
	TargetReasoningTokens int64
}

// ResolveNearestEffort selects the closest supported effort level for a given model capability set.
func ResolveNearestEffort(requested EffortLevel, supported []EffortLevel) EffortLevel {
	if len(supported) == 0 {
		return requested
	}

	best := supported[0]
	minDist := absDiff(int(requested), int(best))

	for _, s := range supported[1:] {
		dist := absDiff(int(requested), int(s))
		if dist < minDist {
			minDist = dist
			best = s
		}
	}

	return best
}

func absDiff(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}
