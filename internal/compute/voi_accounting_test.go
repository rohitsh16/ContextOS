package compute

import (
	"math"
	"testing"
)

func TestVOIAccountingNoDoubleCharging(t *testing.T) {
	weights := UtilityWeights{
		LambdaCost: 1.0,
		MuLatency:  0.0,
		RhoRisk:    0.0,
	}
	engine := NewVOIEngine(weights)

	currentConf := 0.70
	currentRisk := 0.20
	evidenceCoverage := 0.50 // missing 0.50
	difficulty := 0.50

	budget := BudgetState{
		MaxCostUSD:               1.0,
		SpentCostUSD:             0.10,
		RemainingCostUSD:         0.90,
		RemainingReasoningTokens: 8192,
		VerificationReserveUSD:   0.05,
		EscalationReserveUSD:     0.10,
	}
	cache := CacheState{Active: false}

	actions := engine.EvaluateActions(currentConf, currentRisk, evidenceCoverage, difficulty, cache, budget)

	// Current utility with weights: U_0 = Quality - Lambda * Cost
	u0 := currentConf - (1.0 * budget.SpentCostUSD)

	for _, a := range actions {
		if a.Action == ActionStop {
			if a.VOI != 0 {
				t.Errorf("expected STOP VOI = 0, got %f", a.VOI)
			}
			continue
		}

		// Projected cost is spent + action cost
		projectedSpent := budget.SpentCostUSD + a.ExpectedCost
		// Net projected quality is current + gain (capped at 0.99)
		projQuality := math.Min(currentConf+a.ExpectedGain, 0.99)
		uAfter := projQuality - (1.0 * projectedSpent)

		expectedVOI := ActionDelta(u0, uAfter)
		if math.Abs(a.VOI-expectedVOI) > 1e-7 {
			t.Errorf("Action %s VOI discrepancy: expected %f, got %f", a.Action, expectedVOI, a.VOI)
		}
	}
}
