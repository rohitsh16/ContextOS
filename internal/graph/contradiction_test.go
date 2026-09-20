package graph

import (
	"testing"

	"contextos/internal/model"
)

func TestComputeKappa(t *testing.T) {
	m1 := model.Memory{
		ID:        "mem-1",
		Content:   "Always use SQLite WAL mode",
		Authority: "user",
	}
	m2 := model.Memory{
		ID:        "mem-2",
		Content:   "Do not use SQLite WAL mode",
		Authority: "inference",
	}

	// Semantic conflict is high (opposite directives)
	kappa := ComputeKappa(m1, m2, 0.9)
	if kappa < 0.4 {
		t.Errorf("expected high kappa for opposite directives, got %f", kappa)
	}

	// If m1 was invalidated before m2 was created, temporal overlap is zero
	m1.InvalidatedAtRevision = "commit-10"
	m2.ValidFromRevision = "commit-20"
	kappaNonOverlapping := ComputeKappa(m1, m2, 0.9)
	if kappaNonOverlapping != 0.0 {
		t.Errorf("expected zero kappa for non-overlapping lifetimes, got %f", kappaNonOverlapping)
	}
}

func TestResolveContradictions(t *testing.T) {
	cg := NewContradictionGraph()

	memories := []model.Memory{
		{
			ID:        "mem-user",
			Content:   "Always run on port 8765",
			Authority: "user",
			Scope:     "*",
		},
		{
			ID:        "mem-agent",
			Content:   "Avoid port 8765, use port 9000",
			Authority: "inference",
			Scope:     "*",
		},
		{
			ID:        "mem-unrelated",
			Content:   "Database schema uses WAL mode",
			Authority: "commit",
			Scope:     "db",
		},
	}

	active, reports := cg.Resolve(memories, 0.45)
	if len(active) != 2 {
		t.Fatalf("expected 2 active memories, got %d", len(active))
	}

	hasUser := false
	hasAgent := false
	for _, m := range active {
		if m.ID == "mem-user" {
			hasUser = true
		}
		if m.ID == "mem-agent" {
			hasAgent = true
		}
	}

	if !hasUser {
		t.Errorf("user authority memory should be retained")
	}
	if hasAgent {
		t.Errorf("conflicting agent inference memory should have been suppressed")
	}
	if len(reports) == 0 {
		t.Errorf("expected contradiction report")
	}
}

func TestExplicitSupersedesResolution(t *testing.T) {
	cg := NewContradictionGraph()

	mOld := model.Memory{
		ID:        "m-old",
		Content:   "Use 6-pass context allocation",
		Authority: "commit",
	}
	mNew := model.Memory{
		ID:         "m-new",
		Content:    "Use 8-pass context allocation with Graph PPR",
		Authority:  "commit",
		Supersedes: "m-old",
	}

	active, reports := cg.Resolve([]model.Memory{mOld, mNew}, 0.5)
	if len(active) != 1 || active[0].ID != "m-new" {
		t.Fatalf("expected only m-new to remain, got %v", active)
	}
	if len(reports) != 1 || reports[0].Resolution != "superseded_a" {
		t.Errorf("unexpected report: %+v", reports)
	}
}
