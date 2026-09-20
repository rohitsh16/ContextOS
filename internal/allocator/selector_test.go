package allocator

import (
	"context"
	"testing"

	"contextos/internal/model"
)

func TestSelectors(t *testing.T) {
	ctx := context.Background()

	candidates := []model.Candidate{
		{ID: "c1", Content: "kafka transaction outbox pattern", Tokens: 10, Score: 8.0, Density: 0.8},
		{ID: "c2", Content: "kafka retry mechanism consumer", Tokens: 10, Score: 7.0, Density: 0.7},
		{ID: "c3", Content: "database postgres transactional schema", Tokens: 10, Score: 6.0, Density: 0.6},
		// Singleton item with high score but large token size
		{ID: "c_single", Content: "monolithic architectural documentation for microservices", Tokens: 25, Score: 20.0, Density: 0.8},
	}

	budget := 25

	// 1. Test GreedySelector
	greedy := &GreedySelector{}
	sel1, diag1 := greedy.Select(ctx, candidates, budget)
	if len(sel1) == 0 {
		t.Fatalf("GreedySelector selected 0 items")
	}
	if diag1.Algorithm != "GreedySelector" {
		t.Errorf("expected GreedySelector, got %s", diag1.Algorithm)
	}

	// 2. Test GreedySingletonSelector (Singleton Rescue)
	// c_single has Score=20 > c1+c2 (8+7=15). It should be rescued!
	singleton := &GreedySingletonSelector{}
	sel2, diag2 := singleton.Select(ctx, candidates, budget)
	if len(sel2) != 1 || sel2[0].ID != "c_single" {
		t.Errorf("expected singleton rescue of c_single (Score 20 > 15), got %v", sel2)
	}
	if diag2.MarginalUtility < 20.0 {
		t.Errorf("expected MarginalUtility >= 20, got %.2f", diag2.MarginalUtility)
	}

	// 3. Test SubmodularSelector
	submodular := &SubmodularSelector{RedundancyPenalty: 0.5}
	sel3, diag3 := submodular.Select(ctx, candidates, budget)
	if len(sel3) == 0 {
		t.Fatalf("SubmodularSelector selected 0 items")
	}
	if diag3.SelectedTokens > budget {
		t.Errorf("budget exceeded: %d > %d", diag3.SelectedTokens, budget)
	}

	// 4. Test LazyGreedySelector
	lazy := &LazyGreedySelector{RedundancyPenalty: 0.5}
	sel4, diag4 := lazy.Select(ctx, candidates, budget)
	if len(sel4) == 0 {
		t.Fatalf("LazyGreedySelector selected 0 items")
	}
	if diag4.SelectedTokens > budget {
		t.Errorf("budget exceeded: %d > %d", diag4.SelectedTokens, budget)
	}

	// 5. Test LearnedSelector
	learned := DefaultLearnedSelector()
	sel5, diag5 := learned.Select(ctx, candidates, budget)
	if len(sel5) == 0 {
		t.Fatalf("LearnedSelector selected 0 items")
	}
	if diag5.SelectedTokens > budget {
		t.Errorf("budget exceeded: %d > %d", diag5.SelectedTokens, budget)
	}
}

func TestSubmodularRedundancyAvoidance(t *testing.T) {
	ctx := context.Background()

	candidates := []model.Candidate{
		{ID: "c1", Content: "kafka transaction outbox pattern", Tokens: 10, Score: 8.0, Density: 0.8},
		// c2 is almost identical to c1 (redundant)
		{ID: "c2", Content: "kafka transaction outbox pattern duplicate copy", Tokens: 10, Score: 7.9, Density: 0.79},
		// c3 is diverse
		{ID: "c3", Content: "kubernetes cluster ingress controller routing", Tokens: 10, Score: 7.0, Density: 0.7},
	}

	budget := 20
	submodular := &SubmodularSelector{RedundancyPenalty: 5.0} // High penalty for redundancy
	selected, _ := submodular.Select(ctx, candidates, budget)

	if len(selected) != 2 {
		t.Fatalf("expected 2 selected items, got %d", len(selected))
	}
	// With high redundancy penalty, c1 and c3 should be chosen over c1 and c2
	hasC1 := false
	hasC3 := false
	for _, c := range selected {
		if c.ID == "c1" {
			hasC1 = true
		}
		if c.ID == "c3" {
			hasC3 = true
		}
	}
	if !hasC1 || !hasC3 {
		t.Errorf("submodular selection should prefer diverse c3 over redundant c2: got %v", selected)
	}
}
