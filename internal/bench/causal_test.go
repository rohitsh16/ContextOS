package bench

import (
	"testing"

	"contextos/internal/model"
)

func TestCausalAttribution(t *testing.T) {
	cands := []model.Candidate{
		{ID: "c1", Kind: "decision", Content: "Transactional outbox", Confidence: 0.95, Tokens: 20, Semantic: 0.8},
		{ID: "c2", Kind: "constraint", Content: "Idempotent event emitter", Confidence: 0.99, Tokens: 15, Semantic: 0.85},
		{ID: "c3", Kind: "code", Content: "func main()", Confidence: 0.70, Tokens: 30, Semantic: 0.6},
	}

	report := EvaluateCausalAttribution(cands, 1024, 1.0)
	if report.Status != "GREEN" {
		t.Fatalf("expected causal attribution status GREEN, got %s", report.Status)
	}

	if report.DoublyRobustTau <= 0 {
		t.Fatalf("expected positive Doubly Robust Tau, got %.4f", report.DoublyRobustTau)
	}

	if report.KindAttributions["decision"] <= 0 {
		t.Fatalf("expected positive causal attribution for decision memories")
	}
}
