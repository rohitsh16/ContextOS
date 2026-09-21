package retrieval

import (
	"fmt"
	"strings"
)

// ContextUnitType categorizes the granularity and nature of a context element (PR.md Section 24).
type ContextUnitType string

const (
	UnitDefinition ContextUnitType = "definition"
	UnitSignature  ContextUnitType = "signature"
	UnitSnippet    ContextUnitType = "snippet"
	UnitDependency ContextUnitType = "dependency"
	UnitDecision   ContextUnitType = "decision"
	UnitEvidence   ContextUnitType = "evidence"
	UnitFailure    ContextUnitType = "failure"
)

// ContextUnit represents a discrete, decision-preserving unit of context (PR.md Section 24).
type ContextUnit struct {
	ID            string          `json:"id"`
	Type          ContextUnitType `json:"type"`
	SourceNode    string          `json:"source_node"`
	Path          string          `json:"path"`
	Package       string          `json:"package,omitempty"`
	Tokens        int             `json:"tokens"`
	DecisionValue float64         `json:"decision_value"` // Expected utility to the agent's decision [0.0, 1.0]
	Uncertainty   float64         `json:"uncertainty"`    // Residual ambiguity if omitted [0.0, 1.0]
	Freshness     float64         `json:"freshness"`      // Recency / temporal validity [0.0, 1.0]
	ReuseValue    float64         `json:"reuse_value"`    // Expected reuse in subsequent turns [0.0, 1.0]
	CacheReuse    float64         `json:"cache_reuse"`    // Alignment with prefix cache [0.0, 1.0]
	Content       string          `json:"content"`
	Dependencies  []string        `json:"dependencies,omitempty"`
}

// CompileCandidateToUnits decomposes a Candidate into compact decision-preserving units (PR.md Section 24).
func CompileCandidateToUnits(c Candidate) []ContextUnit {
	var units []ContextUnit

	// 1. Signature unit (ultra-compact representation)
	sig := c.Signature
	if sig == "" && c.Name != "" {
		sig = fmt.Sprintf("%s %s", c.Kind, c.Name)
	}
	if sig != "" {
		sigTokens := (len(sig) + 3) / 4
		units = append(units, ContextUnit{
			ID:            fmt.Sprintf("unit:%s:sig", c.NodeID),
			Type:          UnitSignature,
			SourceNode:    c.NodeID,
			Path:          c.Path,
			Package:       c.Package,
			Tokens:        sigTokens,
			DecisionValue: 0.60 * c.Score,
			Uncertainty:   0.30,
			Freshness:     1.0,
			ReuseValue:    0.80,
			CacheReuse:    0.90,
			Content:       fmt.Sprintf("// File: %s\n%s", c.Path, sig),
			Dependencies:  []string{c.Path},
		})
	}

	// 2. Definition / Snippet unit (content body)
	if c.Content != "" {
		contentTokens := c.Tokens
		if contentTokens <= 0 {
			contentTokens = (len(c.Content) + 3) / 4
		}
		units = append(units, ContextUnit{
			ID:            fmt.Sprintf("unit:%s:def", c.NodeID),
			Type:          UnitDefinition,
			SourceNode:    c.NodeID,
			Path:          c.Path,
			Package:       c.Package,
			Tokens:        contentTokens,
			DecisionValue: c.Score,
			Uncertainty:   0.10,
			Freshness:     1.0,
			ReuseValue:    0.50,
			CacheReuse:    0.70,
			Content:       fmt.Sprintf("// %s in %s (lines %d-%d)\n%s", c.Name, c.Path, c.StartLine, c.EndLine, c.Content),
			Dependencies:  []string{c.Path},
		})
	}

	return units
}

// FormatContextUnits serializes selected context units into the final prompt text.
func FormatContextUnits(units []ContextUnit) string {
	var sb strings.Builder
	for i, u := range units {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(u.Content)
	}
	return sb.String()
}
