package allocator

import (
	"contextos/internal/model"
	"testing"
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
