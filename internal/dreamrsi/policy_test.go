package dreamrsi

import (
	"math"
	"testing"
)

func TestDefaultBaselinePolicyEquivalence(t *testing.T) {
	p := DefaultBaselinePolicy()

	// Verify standard baseline matches ASC-1 parameters
	if p.WeightSemantic != 1.15 {
		t.Fatalf("expected WeightSemantic=1.15, got %f", p.WeightSemantic)
	}
	if p.WeightLexical != 1.00 {
		t.Fatalf("expected WeightLexical=1.00, got %f", p.WeightLexical)
	}
	if p.WeightTaskAffinity != 0.90 {
		t.Fatalf("expected WeightTaskAffinity=0.90, got %f", p.WeightTaskAffinity)
	}
	if p.WeightAuthority != 0.75 {
		t.Fatalf("expected WeightAuthority=0.75, got %f", p.WeightAuthority)
	}
	if p.WeightFreshnessRisk != 1.50 {
		t.Fatalf("expected WeightFreshnessRisk=1.50, got %f", p.WeightFreshnessRisk)
	}
	if p.RRFConstant != 60.0 {
		t.Fatalf("expected RRFConstant=60.0, got %f", p.RRFConstant)
	}
	if p.MinAuthorityFloor != 0.60 {
		t.Fatalf("expected MinAuthorityFloor=0.60, got %f", p.MinAuthorityFloor)
	}

	// Test candidate score equivalence
	// score := 1.15*sem + 1.0*lex + 0.9*affinity + 0.75*auth + 0.6*evidence + 0.45*reuse + 0.35*graph - 1.5*freshRisk - 0.15*min(1, tok/4000)
	sem, lex, affinity, auth, evidence, reuse, graph, freshRisk := 0.8, 0.7, 0.9, 0.95, 1.0, 0.5, 0.6, 0.1
	tok := 100

	score, density := p.ScoreCandidate(sem, lex, affinity, auth, evidence, reuse, graph, freshRisk, tok)
	expectedScore := 1.15*0.8 + 1.0*0.7 + 0.9*0.9 + 0.75*0.95 + 0.6*1.0 + 0.45*0.5 + 0.35*0.6 - 1.5*0.1 - 0.15*math.Min(1, float64(tok)/4000.0)
	expectedDensity := expectedScore / float64(tok)

	if math.Abs(score-expectedScore) > 1e-6 {
		t.Fatalf("score mismatch: got %f, expected %f", score, expectedScore)
	}
	if math.Abs(density-expectedDensity) > 1e-6 {
		t.Fatalf("density mismatch: got %f, expected %f", density, expectedDensity)
	}
}

func TestAuthorityFloorRejection(t *testing.T) {
	p := DefaultBaselinePolicy()
	// Authority < 0.60 should yield negative infinity density
	_, density := p.ScoreCandidate(1.0, 1.0, 1.0, 0.45, 1.0, 1.0, 1.0, 0.0, 50)
	if !math.IsInf(density, -1) {
		t.Fatalf("expected density -Inf for low authority (0.45 < 0.60), got %f", density)
	}
}

func TestPolicyMutationDeterministic(t *testing.T) {
	p := DefaultBaselinePolicy()
	m1 := p.Mutate(42, 1.0)
	m2 := p.Mutate(42, 1.0)
	m3 := p.Mutate(99, 1.0)

	if m1.WeightSemantic != m2.WeightSemantic {
		t.Fatalf("expected deterministic mutations for identical seed 42")
	}
	if m1.WeightSemantic == m3.WeightSemantic {
		t.Fatalf("expected different mutations for different seeds")
	}
	if m1.Version != p.Version+1 {
		t.Fatalf("expected mutated version to increment: got %d, expected %d", m1.Version, p.Version+1)
	}
}

func TestRRFScoring(t *testing.T) {
	p := DefaultBaselinePolicy()
	// RRF(1, 1, 1) = 3 / (60 + 1) = 3 / 61
	rrf := p.RRFScore(1, 1, 1)
	expected := 3.0 / 61.0
	if math.Abs(rrf-expected) > 1e-6 {
		t.Fatalf("expected RRF %f, got %f", expected, rrf)
	}
}
