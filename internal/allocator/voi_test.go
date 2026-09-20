package allocator

import (
	"testing"

	"contextos/internal/model"
)

func TestValueOfInformationAndSequentialStopping(t *testing.T) {
	estimator := NewDecisionEntropyEstimator()

	candDecision := model.Candidate{
		ID:         "dec-1",
		Kind:       "decision",
		Content:    "Use outbox pattern",
		Tokens:     20,
		Semantic:   0.75,
		Confidence: 0.95,
	}
	candDocDump := model.Candidate{
		ID:         "code-1",
		Kind:       "code",
		Content:    "Large boilerplate documentation",
		Tokens:     500,
		Semantic:   0.88, // High lexical/semantic match
		Confidence: 0.50,
	}

	estDec := estimator.Estimate(nil, candDecision, "")
	estDoc := estimator.Estimate(nil, candDocDump, "")

	if estDec.VOI <= estDoc.VOI {
		t.Fatalf("expected decision item to carry higher VOI than doc dump, got dec=%.3f, doc=%.3f",
			estDec.VOI, estDoc.VOI)
	}

	if estDec.VOIPerToken <= estDoc.VOIPerToken {
		t.Fatalf("expected decision item to have higher VOI/Token, got dec=%.5f, doc=%.5f",
			estDec.VOIPerToken, estDoc.VOIPerToken)
	}

	candidates := []model.Candidate{candDecision, candDocDump}
	selected, analysis := SequentialVOISelect(candidates, 1000, 0.001)

	if len(selected) == 0 {
		t.Fatalf("expected at least 1 candidate selected by sequential VOI")
	}

	if analysis.Status != "GREEN" {
		t.Fatalf("expected VOI analysis status GREEN, got %s", analysis.Status)
	}

	ranked := RankByVOI(candidates)
	if len(ranked) != 2 || ranked[0].ID != "dec-1" {
		t.Fatalf("expected dec-1 ranked first by VOI, got %s", ranked[0].ID)
	}
}
