// Package retrieval — duplicate_amplification_test.go
//
// R17 Phase 10: Proof-Oriented Regression Suite
// Theorem 1 & Theorem 5 (Duplicate Amplification Invariance):
// When multiple duplicate copies of a candidate exist across worktrees or
// re-indexing runs, their presence must not artificially increase the candidate's
// ranking position over higher-relevance distinct candidates (DAR invariant).
package retrieval

import (
	"fmt"
	"sort"
	"testing"
)

func TestDuplicateAmplification_DARInvariant(t *testing.T) {
	repoID := "repo_dar_test"

	// High-relevance distinct candidate
	highCand := Candidate{
		ID:        "cand:high",
		NodeID:    "node:high",
		Kind:      "function",
		Name:      "AuthoritativeMethod",
		Path:      "pkg/core/engine.go",
		StartLine: 100,
		EndLine:   120,
		Score:     0.90,
	}

	// Medium-relevance candidate that gets duplicated 20 times (e.g. across 20 agent worktrees)
	var duplicatedCands []Candidate
	for i := 0; i < 20; i++ {
		duplicatedCands = append(duplicatedCands, Candidate{
			ID:        fmt.Sprintf("cand:med:%d", i),
			NodeID:    fmt.Sprintf("node:med:%d", i),
			Kind:      "function",
			Name:      "RepeatedHelper",
			Path:      "pkg/core/helper.go",
			StartLine: 10,
			EndLine:   25,
			Score:     0.70, // lower individual score than AuthoritativeMethod
		})
	}

	// Combine them into a raw result list
	var raw []Candidate
	raw = append(raw, highCand)
	raw = append(raw, duplicatedCands...)

	// Without canonicalization, naive frequency-based or un-deduplicated ranking
	// would flood 20 items of RepeatedHelper and might dominate the budget.
	// With R17 canonicalization:
	dedup := DeduplicateCandidates(raw, repoID)

	// Sort deduplicated results descending
	sort.Slice(dedup.Canonical, func(i, j int) bool {
		return dedup.Canonical[i].Score > dedup.Canonical[j].Score
	})

	// Invariant 1: Top candidate MUST be AuthoritativeMethod (score 0.90)
	if dedup.Canonical[0].Name != "AuthoritativeMethod" {
		t.Fatalf("DAR violation: repeated helper displaced authoritative method; top=%s", dedup.Canonical[0].Name)
	}

	// Invariant 2: Exactly 2 distinct candidates exist
	if len(dedup.Canonical) != 2 {
		t.Fatalf("expected exactly 2 canonical candidates, got %d", len(dedup.Canonical))
	}

	// Invariant 3: Score of RepeatedHelper remains exactly 0.70 (max, not sum 20*0.70 = 14.0)
	if dedup.Canonical[1].Score != 0.70 {
		t.Fatalf("expected RepeatedHelper score 0.70, got %.4f (amplification occurred)", dedup.Canonical[1].Score)
	}

	// Invariant 4: DAR is 19/21
	expectedDAR := 19.0 / 21.0
	if dedup.DAR < expectedDAR-1e-4 || dedup.DAR > expectedDAR+1e-4 {
		t.Fatalf("expected DAR %.4f, got %.4f", expectedDAR, dedup.DAR)
	}
}
