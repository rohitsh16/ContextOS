package cache

import (
	"testing"

	"contextos/internal/model"
)

func TestCacheEconomicsAndPrefixOptimization(t *testing.T) {
	cands := []model.Candidate{
		{ID: "rule-1", Kind: "rule", Content: "Always write tests", Tokens: 20},
		{ID: "diff-1", Kind: "code", Location: "diff/patch.go", Content: "diff chunk", Tokens: 15},
		{ID: "dec-1", Kind: "decision", Content: "Use Postgres", Tokens: 25},
		{ID: "ast-1", Kind: "code", Location: "service.go", Content: "type S struct{}", Tokens: 30},
	}

	ordered := OptimizePrefixOrdering(cands)
	if len(ordered) != 4 {
		t.Fatalf("expected 4 candidates ordered")
	}

	// First item must be rule (TierGlobalRules)
	if ordered[0].Kind != "rule" {
		t.Fatalf("expected first item to be rule, got %s", ordered[0].Kind)
	}

	// Last item must be diff (TierWorkingDiff)
	if ordered[3].ID != "diff-1" {
		t.Fatalf("expected last item to be diff-1, got %s", ordered[3].ID)
	}

	val := EvaluateCacheEconomics(cands, "", 0.05)
	if val.StablePrefixTokens <= 0 {
		t.Fatalf("expected positive stable prefix tokens")
	}
	if val.CROI <= 0 {
		t.Fatalf("expected positive CROI, got %.2f", val.CROI)
	}

	report := RunCacheEconomicsSuite(5, [][]model.Candidate{cands, cands, cands, cands, cands})
	if report.Status != "GREEN" {
		t.Fatalf("expected cache economics report status GREEN, got %s", report.Status)
	}
}
