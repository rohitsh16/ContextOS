package allocator

import (
	"testing"

	"contextos/internal/model"
)

func TestSubmodularityAndSynergy(t *testing.T) {
	c1 := model.Candidate{ID: "c1", Kind: "code", Content: "type Storage interface", Tokens: 10, Semantic: 0.9}
	c2 := model.Candidate{ID: "c2", Kind: "code", Content: "func (s *SQLStorage) Save()", Tokens: 20, Semantic: 0.85}
	c3 := model.Candidate{ID: "c3", Kind: "decision", Content: "Use SQL storage", Tokens: 15, Semantic: 0.8}

	candidates := []model.Candidate{c1, c2, c3}

	utilFn := func(subset []model.Candidate) float64 {
		if len(subset) == 0 {
			return 0.0
		}
		u := 0.0
		hasIface := false
		hasImpl := false
		for _, c := range subset {
			u += c.Semantic
			if c.ID == "c1" {
				hasIface = true
			}
			if c.ID == "c2" {
				hasImpl = true
			}
		}
		// Submodular diminishing returns on size
		dim := float64(len(subset)) * 0.1
		// Supermodular synergy
		syn := 0.0
		if hasIface && hasImpl {
			syn = 0.5
		}
		return u + dim + syn
	}

	analysis := EvaluateSubmodularityAndSynergy(candidates, utilFn)
	if analysis.Status != "GREEN" {
		t.Fatalf("expected submodularity analysis status GREEN, got %s", analysis.Status)
	}

	if analysis.Curvature < 0.0 || analysis.Curvature > 1.0 {
		t.Fatalf("expected curvature in [0, 1], got %.2f", analysis.Curvature)
	}

	// Test Hybrid Context Selection
	selected := HybridContextSelect(candidates, 30, utilFn)
	if len(selected) == 0 {
		t.Fatalf("expected hybrid context select to pick items")
	}
}
