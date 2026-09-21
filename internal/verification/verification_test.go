package verification

import (
	"context"
	"testing"
)

func TestVerifierLevel0(t *testing.T) {
	v := NewVerifier()

	code := `
package main
func HandleRequest(w http.ResponseWriter, r *http.Request) {
    // Process
}
`
	resPass := v.VerifyLevel0(code, []string{"HandleRequest", "http.ResponseWriter"})
	if !resPass.Passed {
		t.Fatalf("expected Level 0 check to pass")
	}
	if resPass.CostUSD != 0.0 {
		t.Fatalf("expected zero cost for Level 0 deterministic verification, got %f", resPass.CostUSD)
	}

	resFail := v.VerifyLevel0(code, []string{"HandleRequest", "MissingSymbol"})
	if resFail.Passed {
		t.Fatalf("expected Level 0 check to fail on missing symbol")
	}
	if len(resFail.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(resFail.Issues))
	}
}

func TestVerifierLevel2Consistency(t *testing.T) {
	v := NewVerifier()

	evidence := []string{
		"File internal/auth/jwt.go: token expiration is set to 24 hours",
		"File internal/db/user.go: user role defaults to guest",
	}

	claims := []string{
		"token expiration is set to 24 hours",
		"admin role defaults to superuser", // Unsupported claim
	}

	res := v.VerifyLevel2(claims, evidence)
	if res.Passed {
		t.Fatalf("expected failure due to unsupported claim")
	}
	if len(res.Issues) != 1 {
		t.Fatalf("expected 1 unsupported claim issue, got %d", len(res.Issues))
	}
	if res.Score != 0.5 {
		t.Fatalf("expected 0.5 score for 1 of 2 valid claims, got %f", res.Score)
	}
}

func TestVerifierLevel3CheapModel(t *testing.T) {
	v := NewVerifier()
	res := v.VerifyLevel3(context.Background(), "Hypothesis: caching reduces compute cost by 40%")
	if !res.Passed {
		t.Fatalf("expected Level 3 verification to pass")
	}
	if res.CostUSD <= 0 {
		t.Fatalf("expected positive cost for Level 3 model check")
	}
}

func TestVerificationVOIAndRecommendation(t *testing.T) {
	// VOI calculation: P(error) = 0.40, Impact = $1.00, Cost = $0.05
	// VOI = 0.40 * 1.00 - 0.05 = $0.35 > 0 (justified)
	voi := EvaluateVerificationVOI(0.40, 1.00, 0.05)
	if voi < 0.34 || voi > 0.36 {
		t.Fatalf("expected VOI $0.35, got %f", voi)
	}

	// Bypass should always recommend Level 0
	rec0 := RecommendVerificationLevel(0.8, true, 1.0)
	if rec0 != Level0Deterministic {
		t.Fatalf("expected Level0Deterministic for bypassed task, got %v", rec0)
	}

	// High risk should recommend Level 3 or 4
	recHigh := RecommendVerificationLevel(0.8, false, 1.0)
	if recHigh != Level4StrongModel {
		t.Fatalf("expected Level4StrongModel for high risk task, got %v", recHigh)
	}
}

func TestIsDeterministicBypass(t *testing.T) {
	bypass1, kind1 := IsDeterministicBypass("where is func HandlePlan defined")
	if !bypass1 || kind1 != "symbol_definition_lookup" {
		t.Fatalf("expected symbol definition lookup bypass, got (%v, %s)", bypass1, kind1)
	}

	bypass2, kind2 := IsDeterministicBypass("find callers of ComputeCost")
	if !bypass2 || kind2 != "symbol_caller_graph" {
		t.Fatalf("expected caller graph bypass, got (%v, %s)", bypass2, kind2)
	}

	bypassNo, _ := IsDeterministicBypass("redesign the authentication service to use OAuth2")
	if bypassNo {
		t.Fatalf("expected complex architecture redesign not to be bypassed")
	}
}
