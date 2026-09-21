package state

import (
	"fmt"
	"testing"

	"contextos/internal/providers"
)

func TestEvidenceStateCoverageAndConflict(t *testing.T) {
	reqs := []EvidenceRequirement{
		{ID: "req-1", Description: "Auth token validated", Weight: 2.0, Satisfied: false},
		{ID: "req-2", Description: "Unit tests pass", Weight: 1.0, Satisfied: false},
		{ID: "req-3", Description: "Database migration succeeds", Weight: 3.0, Satisfied: false},
	}

	es := NewEvidenceState(reqs)
	if es.Coverage != 0.0 {
		t.Fatalf("expected 0.0 coverage initially, got %f", es.Coverage)
	}
	if len(es.Missing) != 3 {
		t.Fatalf("expected 3 missing requirements, got %d", len(es.Missing))
	}

	// Satisfy req-3 (weight 3.0 out of 6.0 total = 50% coverage)
	es.Required[2].Satisfied = true
	es.Recompute()
	if es.Coverage < 0.49 || es.Coverage > 0.51 {
		t.Fatalf("expected 0.50 coverage, got %f", es.Coverage)
	}

	// Add conflicting evidence
	es.AddEvidence(Evidence{
		ID:       "ev-conflict",
		Content:  "Test failed with timeout",
		Weight:   1.0,
		Conflict: true,
	})
	if es.Conflict <= 0 {
		t.Fatalf("expected positive conflict score, got %f", es.Conflict)
	}
}

func TestDecisionStateOperations(t *testing.T) {
	ds := NewDecisionState("Implement cross-provider reasoning")

	ds.AddFact("OpenAI o3-mini supports reasoning_effort", "internal/providers/openai/openai.go")
	if len(ds.Facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(ds.Facts))
	}

	ds.RecordDecision("Provider-neutral core", "Decouple compute policy from provider native APIs", "architect")
	if len(ds.Decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(ds.Decisions))
	}

	hyp := Hypothesis{ID: "h1", Claim: "Always use maximum reasoning effort"}
	ds.Hypotheses = append(ds.Hypotheses, hyp)
	ds.RejectHypothesis(hyp, "Prohibitively expensive on simple tasks")

	if len(ds.Hypotheses) != 0 {
		t.Fatalf("expected 0 active hypotheses after rejection")
	}
	if len(ds.RejectedHypotheses) != 1 {
		t.Fatalf("expected 1 rejected hypothesis")
	}
}

func TestConversationCompilerCompaction(t *testing.T) {
	compiler := NewConversationCompiler(3)

	// Simulate 30 turns of verbose chat
	var messages []providers.Message
	for i := 1; i <= 30; i++ {
		role := providers.RoleUser
		if i%2 == 0 {
			role = providers.RoleAssistant
		}
		messages = append(messages, providers.Message{
			Role: role,
			Content: fmt.Sprintf("Turn %d: In this turn we discussed various aspects of the architecture and database schema in detail. "+
				"We analyzed query performance, indexing strategies, and multi-tenant partitioning.\n"+
				"Fact: Redis cache is configured with 200MB limit.\n"+
				"Decision: Use sqlite for embedded store.", i),
		})
	}

	bundle := compiler.Compile("Build persistent memory store", messages, nil)

	if bundle.OriginalTokens <= bundle.CompiledTokens {
		t.Fatalf("expected compiled tokens (%d) < original tokens (%d)", bundle.CompiledTokens, bundle.OriginalTokens)
	}
	if bundle.CompressionRatio < 0.50 {
		t.Fatalf("expected at least 50%% compression ratio, got %f", bundle.CompressionRatio)
	}
	if len(bundle.RecentMessages) != 3 {
		t.Fatalf("expected exactly 3 recent messages, got %d", len(bundle.RecentMessages))
	}
}
