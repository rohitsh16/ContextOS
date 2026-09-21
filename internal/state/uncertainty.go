package state

// UncertaintyType categorizes the root cause of confidence deficit.
type UncertaintyType string

const (
	// MissingInformation: We do not know the codebase fact or definition.
	// Primary remediation: RETRIEVE.
	UncertaintyMissingInformation UncertaintyType = "missing_information"

	// InsufficientReasoning: We have all facts but cannot deduce the logical consequence or interaction.
	// Primary remediation: THINK.
	UncertaintyInsufficientReasoning UncertaintyType = "insufficient_reasoning"

	// VerificationNeed: A proposed fix or hypothesis has not been confirmed by compiler, test, or static check.
	// Primary remediation: VERIFY.
	UncertaintyVerificationNeed UncertaintyType = "verification_need"

	// CapabilityMismatch: The task exceeds the reasoning or context capability of the active model.
	// Primary remediation: ESCALATE.
	UncertaintyCapabilityMismatch UncertaintyType = "capability_mismatch"
)

// Uncertainty represents an explicit epistemic deficit in the decision state.
type Uncertainty struct {
	ID                string          `json:"id"`
	Type              UncertaintyType `json:"type"`
	Description       string          `json:"description"`
	RecommendedAction string          `json:"recommended_action"` // "RETRIEVE", "THINK", "VERIFY", "ESCALATE"
	Severity          float64         `json:"severity"`           // 0.0 to 1.0
	Resolved          bool            `json:"resolved"`
}

// ClassifyUncertainty identifies the appropriate action given an uncertainty type.
func ClassifyUncertainty(t UncertaintyType, description string, severity float64) Uncertainty {
	var action string
	switch t {
	case UncertaintyMissingInformation:
		action = "RETRIEVE"
	case UncertaintyInsufficientReasoning:
		action = "THINK"
	case UncertaintyVerificationNeed:
		action = "VERIFY"
	case UncertaintyCapabilityMismatch:
		action = "ESCALATE"
	default:
		action = "RETRIEVE"
	}

	return Uncertainty{
		ID:                string(t) + ":" + description,
		Type:              t,
		Description:       description,
		RecommendedAction: action,
		Severity:          severity,
		Resolved:          false,
	}
}
