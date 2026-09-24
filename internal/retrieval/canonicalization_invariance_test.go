// Package retrieval — canonicalization_invariance_test.go
//
// R17 Phase 10: Proof-Oriented Regression Suite
// Theorem 1 (Duplicate-Invariant Canonicalization):
// For any candidate set containing k physical duplicates of a logical source
// node, canonicalization collapses them to exactly one logical candidate whose
// score equals max_{i} score(c_i), ensuring duplicate physical copies cannot
// artificially amplify ranking mass or displace legitimate candidates.
package retrieval

import (
	"fmt"
	"testing"
)

func TestCanonicalizationInvariance_Theorem1(t *testing.T) {
	repoID := "repo_test_thm1"

	// Create 1 unique primary candidate
	baseCand := Candidate{
		ID:        "cand:orig:1",
		NodeID:    "node:1",
		Kind:      "function",
		Name:      "HandleRequest",
		Path:      "pkg/api/handler.go",
		StartLine: 10,
		EndLine:   30,
		Score:     0.85,
		Provenance: []string{"exact_symbol_match"},
	}

	// Create 5 physical duplicates representing stale worktrees or re-indexes
	var candidates []Candidate
	candidates = append(candidates, baseCand)

	for i := 2; i <= 6; i++ {
		candidates = append(candidates, Candidate{
			ID:        fmt.Sprintf("cand:dup:%d", i),
			NodeID:    fmt.Sprintf("node:%d", i),
			Kind:      "function",
			Name:      "HandleRequest",
			Path:      "pkg/api/handler.go", // Same logical path and symbol
			StartLine: 10,
			EndLine:   30,
			Score:     0.70 + float64(i)*0.02, // Varying individual scores
			Provenance: []string{fmt.Sprintf("stale_observation_%d", i)},
		})
	}

	// Also add an unrelated candidate with lower score
	candidates = append(candidates, Candidate{
		ID:        "cand:other:100",
		NodeID:    "node:100",
		Kind:      "function",
		Name:      "OtherFunc",
		Path:      "pkg/api/other.go",
		StartLine: 50,
		EndLine:   60,
		Score:     0.60,
		Provenance: []string{"lexical_match"},
	})

	// Run canonicalization
	dedup := DeduplicateCandidates(candidates, repoID)

	// Invariant 1: Exactly 2 canonical candidates remain (HandleRequest and OtherFunc)
	if len(dedup.Canonical) != 2 {
		t.Fatalf("expected 2 canonical candidates, got %d", len(dedup.Canonical))
	}

	// Invariant 2: DuplicateCount equals 5 (the 5 duplicates of HandleRequest)
	if dedup.DuplicateCount != 5 {
		t.Fatalf("expected 5 duplicates merged, got %d", dedup.DuplicateCount)
	}

	// Invariant 3: Score of HandleRequest is max(0.85, 0.74, 0.76, 0.78, 0.80, 0.82) = 0.85
	handleCand := dedup.Canonical[0]
	if handleCand.Name != "HandleRequest" {
		t.Fatalf("expected first candidate to be HandleRequest, got %s", handleCand.Name)
	}
	expectedMaxScore := 0.85
	if handleCand.Score < expectedMaxScore-1e-6 || handleCand.Score > expectedMaxScore+1e-6 {
		t.Fatalf("expected max score %.2f, got %.4f (score summation or attenuation violated)", expectedMaxScore, handleCand.Score)
	}

	// Invariant 4: Provenance from all 6 observations is preserved
	if len(handleCand.Provenance) != 6 {
		t.Fatalf("expected 6 merged provenance items, got %d: %v", len(handleCand.Provenance), handleCand.Provenance)
	}

	// Invariant 5: DAR (Duplicate Amplification Ratio) = 5 / 7
	expectedDAR := 5.0 / 7.0
	if dedup.DAR < expectedDAR-1e-4 || dedup.DAR > expectedDAR+1e-4 {
		t.Fatalf("expected DAR %.4f, got %.4f", expectedDAR, dedup.DAR)
	}
}
