package allocator

import (
	"testing"

	"contextos/internal/model"
)

func TestPlanRejectsInvalidAndLowAuthority(t *testing.T) {
	req := Request{Task: "kafka transaction", Budget: 100}
	ms := []model.Memory{
		{ID: "good", Kind: "decision", Content: "Use outbox for kafka transaction", Authority: "user", TokenCost: 8},
		{ID: "stale", Kind: "decision", Content: "Old kafka transaction design", Authority: "user", InvalidatedAtRevision: "deadbeef", TokenCost: 8},
		{ID: "weak", Kind: "fact", Content: "Kafka transaction detail", Authority: "inference", TokenCost: 8},
	}
	p := Plan(req, ms)
	if len(p.Selected) != 1 || p.Selected[0].ID != "good" {
		t.Fatalf("unexpected selection: %+v", p)
	}
}

func TestPlanRespectsBudget(t *testing.T) {
	req := Request{Task: "payment kafka", Budget: 10}
	ms := []model.Memory{
		{ID: "a", Kind: "decision", Content: "payment kafka decision", Authority: "user", TokenCost: 6},
		{ID: "b", Kind: "decision", Content: "payment kafka failure", Authority: "user", TokenCost: 6},
	}
	p := Plan(req, ms)
	if p.SelectedTokens > 10 {
		t.Fatalf("budget exceeded: %d", p.SelectedTokens)
	}
}

// TestBM25LexicalBoostHighRelevance verifies that BM25 lexical scoring ranks a candidate
// with rich multi-term task overlap above one with minimal overlap, even at equal authority.
func TestBM25LexicalBoostHighRelevance(t *testing.T) {
	req := Request{Task: "kafka transaction outbox pattern", Budget: 500}
	ms := []model.Memory{
		{ID: "hi", Kind: "decision", Content: "Use kafka transaction outbox pattern for exactly-once delivery", Authority: "user", TokenCost: 40},
		{ID: "lo", Kind: "decision", Content: "Use redis caching for user session data", Authority: "user", TokenCost: 40},
	}
	p := Plan(req, ms)
	if len(p.Selected) < 2 {
		t.Fatalf("expected both selected, got %+v", p.Selected)
	}
	// "hi" has 4/4 query terms matched; "lo" has 0/4 — BM25+RRF must rank "hi" first.
	if p.Selected[0].ID != "hi" {
		t.Errorf("expected high-overlap item ranked first; got %s (scores: hi=%.5f lo=%.5f)",
			p.Selected[0].ID,
			func() float64 {
				for _, c := range p.Selected {
					if c.ID == "hi" {
						return c.Score
					}
				}
				return 0
			}(),
			func() float64 {
				for _, c := range p.Selected {
					if c.ID == "lo" {
						return c.Score
					}
				}
				return 0
			}(),
		)
	}
}

// TestSingletonRescueRecoversBudget verifies the (1-1/e) singleton rescue pass (Sviridenko 2004).
// When a single high-value item fits the full budget and its score exceeds the sum of the
// greedily-packed smaller items, it should replace the greedy selection.
func TestSingletonRescueRecoversBudget(t *testing.T) {
	// Budget=100. s1(50 tok) + s2(60 tok) = 110 > 100, so greedy packs s1 only.
	// "big" perfectly matches the task + has user authority → its single-item score
	// should exceed s1's score, triggering singleton rescue.
	req := Request{Task: "kafka transaction critical outbox", Budget: 100}
	ms := []model.Memory{
		{ID: "big", Kind: "decision", Content: "kafka transaction critical outbox exactly once delivery guarantee", Authority: "user", TokenCost: 95},
		{ID: "s1", Kind: "observation", Content: "network packet loss detected", Authority: "commit", TokenCost: 50},
		{ID: "s2", Kind: "observation", Content: "cpu idle percentage high", Authority: "commit", TokenCost: 60},
	}
	p := Plan(req, ms)
	if p.SelectedTokens > req.Budget {
		t.Errorf("budget exceeded: %d > %d", p.SelectedTokens, req.Budget)
	}
	// "big" has 4 of 4 task terms matching + user authority + decision kind:
	// its score should dominate s1 alone (0 task terms, commit authority).
	if len(p.Selected) == 1 && p.Selected[0].ID != "big" {
		t.Errorf("singleton rescue should have selected 'big'; got %s (score=%.5f)",
			p.Selected[0].ID, p.Selected[0].Score)
	}
	t.Logf("selected=%v tokens=%d", func() []string {
		ids := make([]string, len(p.Selected))
		for i, c := range p.Selected {
			ids[i] = c.ID
		}
		return ids
	}(), p.SelectedTokens)
}
