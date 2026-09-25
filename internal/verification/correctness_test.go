package verification

import (
	"testing"

	"contextos/internal/gitidx"
	"contextos/internal/retrieval"
)

func TestExtractAtomicClaims(t *testing.T) {
	text := "GCP shared-VPC bunker projects are excluded by policy. The policy is invoked during workflow hydration. The service does not use Apache Kafka."
	claims := ExtractAtomicClaims(text)

	if len(claims) != 3 {
		t.Fatalf("Expected 3 atomic claims, got %d", len(claims))
	}
	if claims[0].Text != "GCP shared-VPC bunker projects are excluded by policy" {
		t.Fatalf("Claim 1 mismatch: %q", claims[0].Text)
	}
}

func TestSemanticVerifyClaims_SupportedAndContradicted(t *testing.T) {
	claims := []AtomicClaim{
		{ID: "c1", Text: "bunker projects are excluded by policy"},
		{ID: "c2", Text: "the service uses Apache Kafka for streaming"},
	}

	evidence := []*retrieval.EvidenceNode{
		{
			ID:      "e1",
			Path:    "plan/policy.go",
			Content: "func check() { // bunker projects are excluded by policy }",
			Provenance: gitidx.EvidenceProvenance{
				Eligible:  true,
				Authority: 1.0,
			},
		},
		{
			ID:      "e2",
			Path:    "config/queue.go",
			Content: "func setup() { // the service does not use Apache Kafka for streaming }",
			Provenance: gitidx.EvidenceProvenance{
				Eligible:  true,
				Authority: 1.0,
			},
		},
	}

	res := SemanticVerifyClaims(claims, evidence)
	if res.SupportedClaims != 1 {
		t.Fatalf("Expected 1 supported claim, got %d", res.SupportedClaims)
	}
	if res.ContradictedClaims != 1 {
		t.Fatalf("Expected 1 contradicted claim (Kafka negation), got %d", res.ContradictedClaims)
	}
	if res.ContradictionRate <= 0.0 {
		t.Fatalf("Expected positive contradiction rate, got %f", res.ContradictionRate)
	}
}

func TestDetectContradictions(t *testing.T) {
	ev1 := &retrieval.EvidenceNode{
		ID:      "ev1",
		Path:    "pkg/feature.go",
		Content: "const enable_bunker_routing = true",
		Provenance: gitidx.EvidenceProvenance{
			Revision: "rev-100",
		},
	}
	ev2 := &retrieval.EvidenceNode{
		ID:      "ev2",
		Path:    "pkg/override.go",
		Content: "const disable_bunker_routing = true",
		Provenance: gitidx.EvidenceProvenance{
			Revision: "rev-200",
		},
	}

	res := DetectContradictions([]*retrieval.EvidenceNode{ev1, ev2}, nil)
	if !res.Contradicted {
		t.Fatalf("Expected contradiction between conflicting feature flags and revisions")
	}
	if len(res.Conflicts) < 2 {
		t.Fatalf("Expected at least 2 detected conflicts, got %d", len(res.Conflicts))
	}
}

func TestAnswerGate(t *testing.T) {
	// 1. Contradiction -> INVESTIGATE_CONFLICT
	contraRes := ContradictionResult{
		Contradicted: true,
		Reasons:      []string{"feature flag conflict"},
	}
	dec1 := EvaluateAnswerGate(retrieval.SufficiencyResult{Sufficient: true}, ClaimVerificationResult{ClaimPrecision: 1.0}, contraRes)
	if dec1.Action != ActionInvestigateConflict || dec1.Status != AnswerContradicted {
		t.Fatalf("Expected ActionInvestigateConflict, got %s (status: %s)", dec1.Action, dec1.Status)
	}
	if dec1.FailureCode != "R5" {
		t.Fatalf("Expected failure code R5, got %s", dec1.FailureCode)
	}

	// 2. Insufficient evidence -> ABSTAIN or RETRIEVE_MORE
	suffFail := retrieval.SufficiencyResult{
		Sufficient:      false,
		Coverage:        0.2,
		MissingEvidence: []string{"a.go", "b.go", "c.go"},
		Reason:          "missing critical nodes",
	}
	dec2 := EvaluateAnswerGate(suffFail, ClaimVerificationResult{}, ContradictionResult{})
	if dec2.Action != ActionAbstain || dec2.Status != AnswerInsufficient {
		t.Fatalf("Expected ActionAbstain, got %s (status: %s)", dec2.Action, dec2.Status)
	}
	if dec2.FailureCode != "R4" {
		t.Fatalf("Expected failure code R4, got %s", dec2.FailureCode)
	}

	// 3. Fully supported & verified -> ANSWER
	suffPass := retrieval.SufficiencyResult{
		Sufficient: true,
		Coverage:   1.0,
	}
	claimsPass := ClaimVerificationResult{
		TotalClaims:     3,
		SupportedClaims: 3,
		ClaimPrecision:  1.0,
	}
	dec3 := EvaluateAnswerGate(suffPass, claimsPass, ContradictionResult{})
	if dec3.Action != ActionAnswer || dec3.Status != AnswerSupported {
		t.Fatalf("Expected ActionAnswer, got %s (status: %s)", dec3.Action, dec3.Status)
	}
}

func TestCalibrationMetrics(t *testing.T) {
	obs := []PredictionObservation{
		{Confidence: 0.9, Correct: true},
		{Confidence: 0.8, Correct: true},
		{Confidence: 0.7, Correct: false},
		{Confidence: 0.2, Correct: false},
	}

	report := ComputeCalibration(obs)
	if report.Observations != 4 {
		t.Fatalf("Expected 4 observations, got %d", report.Observations)
	}
	if report.BrierScore <= 0.0 || report.BrierScore > 0.5 {
		t.Fatalf("Unexpected Brier score: %f", report.BrierScore)
	}

	sel := EvaluateSelectiveAnswering(obs, 0.75)
	if sel.AnsweredCount != 2 {
		t.Fatalf("Expected 2 answered at threshold 0.75, got %d", sel.AnsweredCount)
	}
	if sel.SelectiveAccuracy != 1.0 {
		t.Fatalf("Expected selective accuracy 1.0, got %f", sel.SelectiveAccuracy)
	}
}
