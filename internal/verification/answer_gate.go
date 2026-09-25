package verification

import (
	"contextos/internal/retrieval"
)

// AnswerStatus defines the authoritative gating outcome for an answer (R18.3 §46).
type AnswerStatus string

const (
	AnswerSupported          AnswerStatus = "supported"
	AnswerPartiallySupported AnswerStatus = "partially_supported"
	AnswerContradicted       AnswerStatus = "contradicted"
	AnswerInsufficient       AnswerStatus = "insufficient_evidence"
	AnswerStale              AnswerStatus = "stale_evidence"
	AnswerUnverified         AnswerStatus = "unverified"
	AnswerAbstain            AnswerStatus = "abstain"
)

// GateAction dictates the operational next step in the agent loop (R18.3 §45).
type GateAction string

const (
	ActionAnswer              GateAction = "ANSWER"
	ActionRetrieveMore        GateAction = "RETRIEVE_MORE"
	ActionInvestigateConflict GateAction = "INVESTIGATE_CONFLICT"
	ActionAbstain             GateAction = "ABSTAIN"
)

// GateDecision encapsulates the decision made by the answer gate.
type GateDecision struct {
	Status      AnswerStatus `json:"status"`
	Action      GateAction   `json:"action"`
	Reason      string       `json:"reason"`
	FailureCode string       `json:"failure_code,omitempty"` // R0 to R9 failure taxonomy
}

// EvaluateAnswerGate executes the state machine in R18.3 §45.
// Enforces: Never trade correctness for context compression (R18.5 §95).
func EvaluateAnswerGate(
	sufficiency retrieval.SufficiencyResult,
	claims ClaimVerificationResult,
	contradictions ContradictionResult,
) GateDecision {
	// 1. Contradiction gate: If opposing evidence or negative assertions detected
	if contradictions.Contradicted {
		return GateDecision{
			Status:      AnswerContradicted,
			Action:      ActionInvestigateConflict,
			Reason:      "conflicting evidence or semantic contradiction detected",
			FailureCode: "R5", // Context integrity failure
		}
	}

	// 2. Sufficiency gate: If required evidence is missing or coverage is below threshold
	if !sufficiency.Sufficient {
		action := ActionRetrieveMore
		if len(sufficiency.MissingEvidence) > 2 || sufficiency.Coverage == 0 {
			action = ActionAbstain
		}
		return GateDecision{
			Status:      AnswerInsufficient,
			Action:      action,
			Reason:      sufficiency.Reason,
			FailureCode: "R4", // Context insufficiency
		}
	}

	// 3. Claim verification gate
	if claims.ContradictionRate > 0.0 {
		return GateDecision{
			Status:      AnswerContradicted,
			Action:      ActionInvestigateConflict,
			Reason:      "one or more claims contradicted by admitted evidence",
			FailureCode: "R5",
		}
	}

	if claims.UnsupportedClaimRate > 0.4 {
		return GateDecision{
			Status:      AnswerPartiallySupported,
			Action:      ActionAbstain,
			Reason:      "too many unsupported claims in answer",
			FailureCode: "R7", // Claim support failure
		}
	}

	if claims.ClaimPrecision < 0.6 && claims.TotalClaims > 0 {
		return GateDecision{
			Status:      AnswerPartiallySupported,
			Action:      ActionRetrieveMore,
			Reason:      "claim precision below required threshold",
			FailureCode: "R7",
		}
	}

	// 4. All checks passed: Answer is verified and fully supported
	return GateDecision{
		Status: AnswerSupported,
		Action: ActionAnswer,
		Reason: "evidence is sufficient, unconflicted, and all claims are grounded",
	}
}
