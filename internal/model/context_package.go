package model

import (
	"fmt"
	"strings"
)

// RepositoryIdentity contains canonical identification for a source repository.
type RepositoryIdentity struct {
	Path         string `json:"path"`
	Name         string `json:"name"`
	Revision     string `json:"revision"`
	Branch       string `json:"branch"`
	WorktreeHash string `json:"worktree_hash"`
}

// ContextPackage represents the canonical, provider-neutral output of context planning.
type ContextPackage struct {
	SchemaVersion  string             `json:"schema_version"`
	Repository     RepositoryIdentity `json:"repository"`
	Revision       string             `json:"revision"`
	Task           string             `json:"task"`
	Model          string             `json:"model"`
	Budget         int                `json:"budget"`
	SelectedTokens int                `json:"selected_tokens"`
	EstimatedCost  float64            `json:"estimated_cost"`
	CacheHit       bool               `json:"cache_hit"`
	Stable         []Candidate        `json:"stable"`
	Dynamic        []Candidate        `json:"dynamic"`
	Decisions      []Candidate        `json:"decisions"`
	Failures       []Candidate        `json:"failures"`
	Trace          []string           `json:"trace"`
}

// NewContextPackage builds a canonical ContextPackage from a ContextPlan.
func NewContextPackage(repo RepositoryIdentity, plan ContextPlan) ContextPackage {
	pkg := ContextPackage{
		SchemaVersion:  "1.0.0",
		Repository:     repo,
		Revision:       repo.Revision,
		Task:           plan.Task,
		Model:          plan.Model,
		Budget:         plan.Budget,
		SelectedTokens: plan.SelectedTokens,
		EstimatedCost:  plan.EstimatedCost,
		CacheHit:       plan.CacheHit,
		Stable:         plan.StablePrefix,
		Dynamic:        plan.VariableContext,
	}

	for _, c := range plan.Selected {
		if c.Kind == "decision" || c.Kind == "constraint" {
			pkg.Decisions = append(pkg.Decisions, c)
		} else if c.Kind == "failure" {
			pkg.Failures = append(pkg.Failures, c)
		}
	}

	return pkg
}

// RenderText produces a universal markdown representation of the context package.
func (cp *ContextPackage) RenderText() string {
	var sb strings.Builder
	sb.WriteString("# ContextOS Context Package\n")
	sb.WriteString(fmt.Sprintf("Task: %s\n", cp.Task))
	sb.WriteString(fmt.Sprintf("Budget: %d tokens | Selected: %d tokens | Cache: %t\n\n", cp.Budget, cp.SelectedTokens, cp.CacheHit))

	if len(cp.Decisions) > 0 {
		sb.WriteString("## Architectural Decisions & Constraints\n")
		for _, d := range cp.Decisions {
			sb.WriteString(fmt.Sprintf("- [%s] %s\n", d.Kind, d.Content))
		}
		sb.WriteString("\n")
	}

	if len(cp.Failures) > 0 {
		sb.WriteString("## Known Failures & Dead Ends\n")
		for _, f := range cp.Failures {
			sb.WriteString(fmt.Sprintf("- %s\n", f.Content))
		}
		sb.WriteString("\n")
	}

	if len(cp.Stable) > 0 {
		sb.WriteString("## Stable Context (KV-Cache Optimized)\n")
		for _, s := range cp.Stable {
			loc := s.Location
			if loc == "" {
				loc = s.Source
			}
			sb.WriteString(fmt.Sprintf("### %s (%s)\n```\n%s\n```\n", s.ID, loc, s.Content))
		}
	}

	if len(cp.Dynamic) > 0 {
		sb.WriteString("## Task-Specific Context\n")
		for _, d := range cp.Dynamic {
			loc := d.Location
			if loc == "" {
				loc = d.Source
			}
			sb.WriteString(fmt.Sprintf("### %s (%s)\n```\n%s\n```\n", d.ID, loc, d.Content))
		}
	}

	return sb.String()
}
