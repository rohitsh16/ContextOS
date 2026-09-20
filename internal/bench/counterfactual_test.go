package bench

import (
	"math"
	"testing"

	"contextos/internal/model"
)

func TestEvaluateCounterfactualAblations(t *testing.T) {
	cands := []model.Candidate{
		{ID: "d1", Kind: "decision", Content: "Decision: use sqlite", Tokens: 100},
		{ID: "f1", Kind: "failure", Content: "Failure: lock contention", Tokens: 80},
		{ID: "s1", Kind: "symbol", Content: "func Connect()", Tokens: 150},
		{ID: "g1", Kind: "graph", Content: "Edge: Connect -> Pool", Tokens: 50},
		{ID: "p1", Kind: "symbol", Content: "Stable prefix header", Tokens: 100, StaleRisk: 0.01},
	}

	// Mock quality evaluator: decisions and failures contribute significantly
	evalFn := func(subset []model.Candidate) float64 {
		score := 0.0
		for _, c := range subset {
			switch c.Kind {
			case "decision":
				score += 0.4
			case "failure":
				score += 0.3
			case "symbol":
				score += 0.15
			case "graph":
				score += 0.15
			}
		}
		return score
	}

	results := EvaluateCounterfactualAblations(cands, evalFn)
	if len(results) != 5 {
		t.Fatalf("expected 5 ablation dimensions evaluated, got %d", len(results))
	}

	foundDecision := false
	for _, r := range results {
		if r.Dimension == AblationDecisions {
			foundDecision = true
			if r.DeltaQuality <= 0 {
				t.Errorf("expected positive delta quality for decisions, got %f", r.DeltaQuality)
			}
			if !r.Essential {
				t.Errorf("expected decisions to be marked as essential")
			}
		}
	}
	if !foundDecision {
		t.Errorf("missing decision ablation result")
	}
}

func TestEstimateDoublyRobustReward(t *testing.T) {
	records := []TaskExecutionRecord{
		{Model: "gpt-5.3-codex", Success: true, PropensityScore: 0.5},
		{Model: "gpt-5.3-codex", Success: true, PropensityScore: 0.5},
		{Model: "local", Success: false, PropensityScore: 0.5},
		{Model: "local", Success: true, PropensityScore: 0.5},
	}

	targetPolicy := func(rec TaskExecutionRecord) string {
		return "gpt-5.3-codex"
	}

	rewardModel := func(rec TaskExecutionRecord, action string) float64 {
		if action == "gpt-5.3-codex" {
			return 0.9
		}
		return 0.5
	}

	dr := EstimateDoublyRobustReward(records, targetPolicy, rewardModel)

	if dr.DoublyRobustScore <= 0 {
		t.Errorf("expected positive doubly robust score, got %f", dr.DoublyRobustScore)
	}
	if math.IsNaN(dr.DoublyRobustScore) || math.IsInf(dr.DoublyRobustScore, 0) {
		t.Fatalf("invalid doubly robust score: %f", dr.DoublyRobustScore)
	}
}

func TestComputeQuantiles(t *testing.T) {
	values := []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	med, p95, p99 := ComputeQuantiles(values)

	if med != 60 && med != 50 { // depending on integer division 10/2=5 -> values[5] is 60
		t.Errorf("unexpected median: %f", med)
	}
	if p95 < 90 {
		t.Errorf("expected p95 >= 90, got %f", p95)
	}
	if p99 < 90 {
		t.Errorf("expected p99 >= 90, got %f", p99)
	}
}
