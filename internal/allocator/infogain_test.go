package allocator

import (
	"context"
	"testing"

	"contextos/internal/model"
)

func TestComputeInformationGain(t *testing.T) {
	weights := DefaultInfoGainWeights()

	cand1 := model.Candidate{
		ID:           "cand-1",
		Content:      "func calculateTax(amount float64) float64",
		Kind:         "symbol",
		Confidence:   0.9,
		TaskAffinity: 0.8,
		Tokens:       20,
	}

	// Unselected - should have high novelty and high IG
	ig1 := ComputeInformationGain(cand1, nil, weights)
	if ig1 <= 0 {
		t.Fatalf("expected positive IG, got %f", ig1)
	}

	// Now cand2 has identical content to cand1, so redundancy should penalize it
	cand2 := model.Candidate{
		ID:           "cand-2",
		Content:      "func calculateTax(amount float64) float64",
		Kind:         "symbol",
		Confidence:   0.9,
		TaskAffinity: 0.8,
		Tokens:       20,
	}
	ig2 := ComputeInformationGain(cand2, []model.Candidate{cand1}, weights)
	if ig2 >= ig1 {
		t.Fatalf("expected IG with redundancy (%f) to be strictly less than unredundant IG (%f)", ig2, ig1)
	}
}

func TestInfoGainSelector_Select(t *testing.T) {
	sel := NewInfoGainSelector()

	candidates := []model.Candidate{
		{
			ID:           "cand-decision",
			Content:      "decision: use Postgres for multi-tenant isolation",
			Kind:         "decision",
			Confidence:   0.95,
			TaskAffinity: 0.9,
			Tokens:       50,
		},
		{
			ID:           "cand-symbol",
			Content:      "type DBCluster struct { Nodes []Node }",
			Kind:         "symbol",
			Confidence:   0.85,
			TaskAffinity: 0.8,
			Tokens:       50,
		},
		{
			ID:           "cand-file",
			Content:      "// General comments and license header...",
			Kind:         "file",
			Confidence:   0.4,
			TaskAffinity: 0.2,
			Tokens:       100,
		},
	}

	// Budget of 120 tokens allows 2 out of 3 candidates
	selected, diag := sel.Select(context.Background(), candidates, 120)
	if len(selected) != 2 {
		t.Fatalf("expected 2 candidates selected, got %d", len(selected))
	}
	if diag.SelectedTokens > 120 {
		t.Fatalf("budget exceeded: tokens=%d, budget=120", diag.SelectedTokens)
	}
	if selected[0].ID != "cand-decision" {
		t.Errorf("expected cand-decision first due to higher boundary coverage and affinity, got %s", selected[0].ID)
	}
}

func TestOptimizeJointCachePrefix(t *testing.T) {
	candidates := []model.Candidate{
		{
			ID:        "stable-schema",
			Content:   "schema definition",
			Tokens:    100,
			StaleRisk: 0.01,
		},
		{
			ID:        "stable-types",
			Content:   "core types",
			Tokens:    150,
			StaleRisk: 0.02,
		},
		{
			ID:        "volatile-diff",
			Content:   "recent git diff uncommitted",
			Tokens:    300,
			StaleRisk: 0.95,
		},
	}

	prefix, dynamic := OptimizeJointCachePrefix(candidates, 10.0, 3.0, 0.5)
	if len(prefix) == 0 {
		t.Fatalf("expected non-empty cache prefix for highly reused, stable items")
	}

	// High stale risk item should not be in prefix
	for _, p := range prefix {
		if p.ID == "volatile-diff" {
			t.Errorf("volatile item should not be placed into prefix due to staleness penalty")
		}
	}
	if len(dynamic) == 0 {
		t.Fatalf("expected volatile item in dynamic context")
	}
}
