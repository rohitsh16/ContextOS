package verification

// VerificationPolicy determines the required verification rigor based on task risk and economic VOI.
type VerificationPolicy struct {
	MandatoryLevel VerificationLevel `json:"mandatory_level"`
	RiskThreshold  float64           `json:"risk_threshold"`
}

// EvaluateVerificationVOI calculates VOI_verify = P(error) * Impact(error) - Cost(verify).
func EvaluateVerificationVOI(probError, impactError, verifyCostUSD float64) float64 {
	expectedLossPrevented := probError * impactError
	return expectedLossPrevented - verifyCostUSD
}

// RecommendVerificationLevel selects the optimal verification tier balancing risk mitigation vs cost.
func RecommendVerificationLevel(risk float64, canBypass bool, costBudgetUSD float64) VerificationLevel {
	if canBypass {
		return Level0Deterministic
	}

	// Cost estimates per level
	costLevel3 := 0.005
	costLevel4 := 0.050

	if risk < 0.15 {
		return Level0Deterministic
	} else if risk < 0.40 {
		return Level2EvidenceConsistency
	} else if risk < 0.70 {
		if costBudgetUSD >= costLevel3 {
			return Level3CheapModel
		}
		return Level2EvidenceConsistency
	} else {
		if costBudgetUSD >= costLevel4 {
			return Level4StrongModel
		} else if costBudgetUSD >= costLevel3 {
			return Level3CheapModel
		}
		return Level2EvidenceConsistency
	}
}
