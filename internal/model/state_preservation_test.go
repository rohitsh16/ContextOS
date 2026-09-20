package model

import (
	"testing"
)

func TestStatePreservationAndContinuity(t *testing.T) {
	work := WorkItem{
		ID:    "work-42",
		Title: "Refactor auth middleware to use redis token store",
	}

	memories := []Memory{
		{ID: "m1", Kind: "decision", Content: "Use redis cluster for session tokens"},
		{ID: "m2", Kind: "constraint", Content: "JWT expiration must remain 15m"},
		{ID: "m3", Kind: "failure", Content: "Redis connection timeout during startup"},
	}

	files := map[string]string{
		"middleware/auth.go": "M",
		"store/redis.go":     "A",
	}

	// 1. Export state package
	raw, err := ExportStatePreservationPackage("claude-3.5-sonnet", "cursor-gpt-5", work, memories, files)
	if err != nil {
		t.Fatalf("failed to export state package: %v", err)
	}

	if len(raw) == 0 {
		t.Fatalf("expected non-empty state preservation package")
	}

	// 2. Evaluate State Continuity
	audit, err := EvaluateStateContinuity(raw, 1, 1)
	if err != nil {
		t.Fatalf("failed to evaluate state continuity: %v", err)
	}

	if audit.Status != "GREEN" {
		t.Fatalf("expected state continuity status GREEN, got %s", audit.Status)
	}

	if audit.StateContinuity < 0.95 {
		t.Fatalf("expected state continuity >= 0.95, got %.4f", audit.StateContinuity)
	}

	if !audit.RediscoveryAvoided {
		t.Fatalf("expected rediscovery to be avoided upon successful state transfer")
	}

	if audit.DecisionsPreserved != 1 {
		t.Fatalf("expected 1 decision preserved, got %d", audit.DecisionsPreserved)
	}
}
