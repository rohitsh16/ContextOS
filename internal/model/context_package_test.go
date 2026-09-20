package model

import (
	"strings"
	"testing"
)

func TestNewContextPackage(t *testing.T) {
	repo := RepositoryIdentity{
		Path:         "/workspace/project",
		Name:         "project",
		Revision:     "abcdef1",
		Branch:       "main",
		WorktreeHash: "wt-12345",
	}

	plan := ContextPlan{
		Task:           "Refactor memory allocation",
		Model:          "claude-3-5-sonnet",
		Budget:         4000,
		SelectedTokens: 1800,
		EstimatedCost:  0.0054,
		CacheHit:       true,
		StablePrefix: []Candidate{
			{ID: "Schema", Location: "internal/db/schema.go", Content: "type Schema struct{}", Tokens: 50, Kind: "code"},
		},
		VariableContext: []Candidate{
			{ID: "Allocator", Location: "internal/allocator/allocator.go", Content: "func Plan(){}", Tokens: 100, Kind: "code"},
		},
		Selected: []Candidate{
			{Kind: "decision", Content: "Use submodular density greedy knapsack", Tokens: 20},
			{Kind: "failure", Content: "Avoid recursive tree traversal", Tokens: 15},
		},
	}

	pkg := NewContextPackage(repo, plan)
	if pkg.SchemaVersion != "1.0.0" {
		t.Errorf("expected SchemaVersion 1.0.0, got %s", pkg.SchemaVersion)
	}
	if pkg.Task != "Refactor memory allocation" {
		t.Errorf("Task mismatch: %s", pkg.Task)
	}
	if len(pkg.Decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(pkg.Decisions))
	}
	if pkg.Decisions[0].Content != "Use submodular density greedy knapsack" {
		t.Errorf("decision content mismatch: %s", pkg.Decisions[0].Content)
	}
	if len(pkg.Failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(pkg.Failures))
	}

	rendered := pkg.RenderText()
	if !strings.Contains(rendered, "ContextOS Context Package") {
		t.Errorf("rendered missing header: %s", rendered)
	}
	if !strings.Contains(rendered, "Architectural Decisions & Constraints") {
		t.Errorf("rendered missing decisions section: %s", rendered)
	}
	if !strings.Contains(rendered, "Known Failures & Dead Ends") {
		t.Errorf("rendered missing failures section: %s", rendered)
	}
	if !strings.Contains(rendered, "Stable Context") {
		t.Errorf("rendered missing stable context: %s", rendered)
	}
}
