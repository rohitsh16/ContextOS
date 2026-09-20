package allocator

import (
	"testing"

	"contextos/internal/model"
)

func TestMSCFrontierAndDecisionPreservation(t *testing.T) {
	frontier := ComputeMSCFrontier(func(budget int) (float64, float64, float64) {
		normB := float64(budget)
		success := 0.70 + 0.30*(normB/2048.0)
		if success > 1.0 {
			success = 1.0
		}
		dpr := 0.65 + 0.35*(normB/2048.0)
		if dpr > 1.0 {
			dpr = 1.0
		}
		return success, dpr, normB * 0.5
	})

	if frontier.Status != "GREEN" {
		t.Fatalf("expected MSC frontier status GREEN, got %s", frontier.Status)
	}

	b95, ok := frontier.BStar["0.95"]
	if !ok || b95 <= 0 {
		t.Fatalf("expected valid B*(95%%), got %d", b95)
	}

	// Test Decision Preservation logic
	dFull := DecisionVector{
		Strategy:           "event-outbox",
		Constraints:        []string{"idempotent", "max-retry-3"},
		AffectedComponents: []string{"billing/service.go", "billing/store.go"},
		TestsRequired:      []string{"TestOutboxPersistence"},
	}
	dComp := DecisionVector{
		Strategy:           "event-outbox",
		Constraints:        []string{"idempotent", "max-retry-3"},
		AffectedComponents: []string{"billing/service.go", "billing/store.go"},
		TestsRequired:      []string{"TestOutboxPersistence"},
	}

	preserved, fidelity := EvaluateDecisionPreservation(dComp, dFull)
	if !preserved || fidelity < 1.0 {
		t.Fatalf("expected perfect decision preservation, got preserved=%v fidelity=%.2f", preserved, fidelity)
	}

	// Test ExtractDecisionFromPlan
	plan := model.ContextPlan{
		Selected: []model.Candidate{
			{Kind: "decision", Content: "event-outbox"},
			{Kind: "constraint", Content: "idempotent"},
			{Kind: "code", Location: "billing/service.go:1"},
		},
	}
	extracted := ExtractDecisionFromPlan(plan)
	if extracted.Strategy != "event-outbox" {
		t.Fatalf("expected strategy event-outbox, got %s", extracted.Strategy)
	}
	if len(extracted.Constraints) != 1 || extracted.Constraints[0] != "idempotent" {
		t.Fatalf("expected constraint idempotent, got %+v", extracted.Constraints)
	}
}
