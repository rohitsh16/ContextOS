package retrieval

import (
	"context"
	"testing"

	"contextos/internal/gitidx"
)

func TestEvidenceGraphAndChains(t *testing.T) {
	eg := NewEvidenceGraph()

	n1 := &EvidenceNode{ID: "n1", Path: "plan/policy/provider/gcp/gcp_attach_service_project_policy.go", Symbol: "AttachPolicy", Tokens: 100, Authority: 1.0}
	n2 := &EvidenceNode{ID: "n2", Path: "workflow/hydration/workflow.go", Symbol: "Hydrate", Tokens: 150, Authority: 1.0}
	n3 := &EvidenceNode{ID: "n3", Path: "plan/plan.go", Symbol: "BuildPlan", Tokens: 120, Authority: 1.0}
	n4 := &EvidenceNode{ID: "n4", Path: "workflow/shared/fd_plan.go", Symbol: "SharedFDPlan", Tokens: 80, Authority: 1.0}
	n5 := &EvidenceNode{ID: "n5", Path: "plan/policyadd/policyadd.go", Symbol: "AddPolicy", Tokens: 90, Authority: 1.0}

	for _, n := range []*EvidenceNode{n1, n2, n3, n4, n5} {
		n.Provenance = gitidx.EvidenceProvenance{Eligible: true, Authority: 1.0}
		eg.AddNode(n)
	}

	eg.AddEdge(EvidenceEdge{From: "n1", To: "n2", Relation: Calls, Weight: 1.0})
	eg.AddEdge(EvidenceEdge{From: "n2", To: "n3", Relation: Calls, Weight: 1.0})
	eg.AddEdge(EvidenceEdge{From: "n3", To: "n4", Relation: Calls, Weight: 1.0})
	eg.AddEdge(EvidenceEdge{From: "n4", To: "n5", Relation: Calls, Weight: 1.0})

	// Find chain from n1 to n5
	chains := eg.FindChains("n1", "n5", 6)
	if len(chains) == 0 {
		t.Fatalf("Expected at least 1 chain from n1 to n5, got none")
	}
	expectedChain := []string{"n1", "n2", "n3", "n4", "n5"}
	if len(chains[0]) != len(expectedChain) {
		t.Fatalf("Chain length mismatch: got %v, want %v", chains[0], expectedChain)
	}

	// Test DependencyClosure
	closure := eg.DependencyClosure([]string{"n1"}, 5)
	if len(closure) != 5 {
		t.Fatalf("Expected closure size 5, got %d", len(closure))
	}
}

func TestSufficiencyEvaluation(t *testing.T) {
	contract := QueryContract{
		Query:    "Trace GCP shared-VPC bunker exclusion",
		TaskType: "trace",
		RequiredEvidence: []string{
			"plan/policy/provider/gcp/gcp_attach_service_project_policy.go",
			"workflow/hydration/workflow.go",
			"plan/plan.go",
		},
		RequiredClaims: []Claim{
			{ID: "c1", Text: "bunker projects are excluded", Required: true, Weight: 1.0},
			{ID: "c2", Text: "gcp attach service project policy handles exclusion", Required: true, Weight: 1.0},
		},
	}

	e1 := &EvidenceNode{
		ID:         "e1",
		Path:       "plan/policy/provider/gcp/gcp_attach_service_project_policy.go",
		Content:    "func checkBunker() { // bunker projects are excluded by policy }",
		Authority:  1.0,
		Provenance: gitidx.EvidenceProvenance{Eligible: true, Authority: 1.0},
	}
	e2 := &EvidenceNode{
		ID:         "e2",
		Path:       "workflow/hydration/workflow.go",
		Content:    "func hydrateWorkflow() { // handles workflow }",
		Authority:  1.0,
		Provenance: gitidx.EvidenceProvenance{Eligible: true, Authority: 1.0},
	}

	// Partial set (missing plan.go) should NOT be sufficient
	resPartial := EvaluateSufficiency([]*EvidenceNode{e1, e2}, contract, 0.8)
	if resPartial.Sufficient {
		t.Fatalf("Expected partial evidence set to be insufficient")
	}
	if len(resPartial.MissingEvidence) != 1 || resPartial.MissingEvidence[0] != "plan/plan.go" {
		t.Fatalf("Expected missing plan.go, got: %v", resPartial.MissingEvidence)
	}

	// Complete set
	e3 := &EvidenceNode{
		ID:         "e3",
		Path:       "plan/plan.go",
		Content:    "func buildPlan() { gcp attach service project policy handles exclusion }",
		Authority:  1.0,
		Provenance: gitidx.EvidenceProvenance{Eligible: true, Authority: 1.0},
	}
	resComplete := EvaluateSufficiency([]*EvidenceNode{e1, e2, e3}, contract, 0.8)
	if !resComplete.Sufficient {
		t.Fatalf("Expected complete evidence set to be sufficient: %s", resComplete.Reason)
	}
	if resComplete.Coverage < 0.8 {
		t.Fatalf("Expected coverage >= 0.8, got %f", resComplete.Coverage)
	}
}

func TestMSEOptimizer_PruningAndMinimality(t *testing.T) {
	contract := QueryContract{
		Query: "Check policy",
		RequiredEvidence: []string{
			"policy.go",
		},
		RequiredClaims: []Claim{
			{ID: "c1", Text: "validates token permissions", Required: true, Weight: 1.0},
		},
	}

	essential := &EvidenceNode{
		ID:         "essential",
		Path:       "policy.go",
		Tokens:     100,
		Authority:  1.0,
		Content:    "func check() { validates token permissions }",
		Provenance: gitidx.EvidenceProvenance{Eligible: true, Authority: 1.0},
	}
	redundant1 := &EvidenceNode{
		ID:         "redundant1",
		Path:       "misc.go",
		Tokens:     300,
		Authority:  0.8,
		Content:    "func other() {}",
		Provenance: gitidx.EvidenceProvenance{Eligible: true, Authority: 0.8},
	}
	redundant2 := &EvidenceNode{
		ID:         "redundant2",
		Path:       "other.go",
		Tokens:     250,
		Authority:  0.8,
		Content:    "func helper() {}",
		Provenance: gitidx.EvidenceProvenance{Eligible: true, Authority: 0.8},
	}

	pool := []*EvidenceNode{essential, redundant1, redundant2}
	selected, stats := OptimizeMSE(pool, contract, 1000, 0.8)

	if len(selected) != 1 || selected[0].ID != "essential" {
		t.Fatalf("MSE optimizer should have pruned redundant nodes, got: %d nodes (%v)", len(selected), selected)
	}
	if stats.MinimalityRate < 1.0 {
		t.Fatalf("Expected MinimalityRate=1.0 for strictly necessary set, got %f", stats.MinimalityRate)
	}
	if stats.CompressionRatio <= 1.0 {
		t.Fatalf("Expected CompressionRatio > 1.0, got %f", stats.CompressionRatio)
	}
}

func TestEvidenceFrontier_Expansion(t *testing.T) {
	contract := QueryContract{
		Query: "trace flow",
		RequiredEvidence: []string{
			"a.go",
			"b.go",
		},
	}

	ef := NewEvidenceFrontier(contract, "test-repo", "rev1")

	c1 := Candidate{ID: "a.go", Path: "a.go", Content: "calls b.go", Kind: "file", Tokens: 50}
	c2 := Candidate{ID: "b.go", Path: "b.go", Content: "implements flow", Kind: "file", Tokens: 50}

	res := ef.ExpandFrontier(context.Background(), []Candidate{c1, c2})
	if !res.Sufficiency.Sufficient {
		t.Fatalf("Expected frontier with both seeds to be sufficient: %s", res.Sufficiency.Reason)
	}
	if len(res.Evidence) != 2 {
		t.Fatalf("Expected 2 evidence nodes, got %d", len(res.Evidence))
	}
}
