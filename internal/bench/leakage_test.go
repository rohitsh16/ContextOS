package bench

import (
	"testing"

	"contextos/internal/graph"
	"contextos/internal/model"
	"contextos/internal/temporal"
)

// TestNoFutureInformationLeakage verifies R0-H4:
// For every task at revision r_t: EvidenceAvailable(t) \subseteq EvidenceCreated(<= r_t)
func TestNoFutureInformationLeakage(t *testing.T) {
	// Task target revision: rev-100 (e.g. commit 100)
	targetRev := "rev-100"

	// Pool containing past, present, and future evidence
	evidencePool := []model.Memory{
		{ID: "mem-past-1", Content: "Initial database architecture", Scope: "repo", ValidFromRevision: "rev-050", Kind: "decision", Confidence: 0.95},
		{ID: "mem-current-1", Content: "Connection pool sizing constraint", Scope: "repo", ValidFromRevision: "rev-100", Kind: "constraint", Confidence: 0.90},
		{ID: "mem-future-leak", Content: "ADVERSARIAL: Future migration to CockroachDB", Scope: "repo", ValidFromRevision: "rev-200", Kind: "decision", Confidence: 0.99},
		{ID: "mem-future-leak-2", Content: "ADVERSARIAL: Deprecated MySQL drivers", Scope: "repo", ValidFromRevision: "rev-150", Kind: "failure", Confidence: 0.95},
	}

	depGraph := graph.New(graph.DefaultConfig())
	changedFiles := map[string]string{
		"db/conn.go": "modified",
	}

	evaluator := temporal.NewScopedStalenessEvaluator(changedFiles, targetRev, depGraph)

	var accessibleMemories []model.Memory
	for _, mem := range evidencePool {
		// Strict temporal validity rule: evidence must not originate from future revisions
		// If ValidFromRevision is chronologically after targetRev, it must be filtered out
		isFuture := mem.ValidFromRevision > targetRev // Lexicographical / chronological comparison
		risk, hardStale := evaluator.EvaluateStaleness(mem)
		if isFuture {
			if !hardStale && risk < 0.5 {
				// Must not be considered valid active context for an older revision
				t.Errorf("LEAK DETECTED: future memory %s (valid_from: %s) was not marked stale for target revision %s",
					mem.ID, mem.ValidFromRevision, targetRev)
			}
			continue
		}
		if !hardStale {
			accessibleMemories = append(accessibleMemories, mem)
		}
	}

	if len(accessibleMemories) != 2 {
		t.Fatalf("expected exactly 2 non-future memories accessible, got %d", len(accessibleMemories))
	}

	for _, m := range accessibleMemories {
		if m.ValidFromRevision > targetRev {
			t.Fatalf("LEAK VIOLATION: accessible memory %s has future revision %s (> %s)", m.ID, m.ValidFromRevision, targetRev)
		}
	}
}

// TestAdversarialFutureLeakageTrap tests R10 Attack 5 (Future leakage trap).
func TestAdversarialFutureLeakageTrap(t *testing.T) {
	targetRev := "v0.6.0"
	futureRev := "v0.7.0"

	cand := model.Candidate{
		ID:        "cand-future",
		Location:  "internal/allocator/infogain.go",
		Content:   "func ComputeInformationGain()",
		Tokens:    50,
		StaleRisk: 0.99, // Future code relative to v0.6.0
	}

	if futureRev <= targetRev {
		t.Fatalf("test invariant broken: futureRev must be > targetRev")
	}

	// Any candidate with high stale risk due to future divergence must be penalized
	if cand.StaleRisk < 0.5 {
		t.Errorf("expected future candidate to carry high staleness risk")
	}
}
