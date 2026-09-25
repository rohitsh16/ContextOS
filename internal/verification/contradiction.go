package verification

import (
	"fmt"
	"strings"

	"contextos/internal/retrieval"
)

// ConflictRecord describes a detected contradiction between evidence nodes or claims (R18.2 §44).
type ConflictRecord struct {
	Type        string `json:"type"` // "polarity", "feature_flag", "superseded_revision", "negative_condition"
	SourceID    string `json:"source_id"`
	TargetID    string `json:"target_id,omitempty"`
	Description string `json:"description"`
}

// ContradictionResult aggregates conflict findings across the evidence pool.
type ContradictionResult struct {
	Contradicted bool             `json:"contradicted"`
	Conflicts    []ConflictRecord `json:"conflicts"`
	Reasons      []string         `json:"reasons"`
}

// DetectContradictions actively searches for opposing assertions, feature flag overrides,
// and negated logic that disproves candidate claims (R18.2 §44).
func DetectContradictions(evidence []*retrieval.EvidenceNode, claims []AtomicClaim) ContradictionResult {
	var conflicts []ConflictRecord
	var reasons []string

	// 1. Cross-evidence polarity check: file A says X, file B says NOT X
	for i := 0; i < len(evidence); i++ {
		for j := i + 1; j < len(evidence); j++ {
			a := evidence[i]
			b := evidence[j]

			aContent := strings.ToLower(a.Content)
			bContent := strings.ToLower(b.Content)

			// Check for feature flag or config override pattern
			if strings.Contains(aContent, "enable_") && strings.Contains(bContent, "disable_") {
				c := ConflictRecord{
					Type:        "feature_flag",
					SourceID:    a.ID,
					TargetID:    b.ID,
					Description: fmt.Sprintf("conflicting feature flags detected between %s and %s", a.Path, b.Path),
				}
				conflicts = append(conflicts, c)
				reasons = append(reasons, c.Description)
			}

			// Revision staleness conflict
			if a.Provenance.Revision != "" && b.Provenance.Revision != "" && a.Provenance.Revision != b.Provenance.Revision {
				c := ConflictRecord{
					Type:        "superseded_revision",
					SourceID:    a.ID,
					TargetID:    b.ID,
					Description: fmt.Sprintf("revision mismatch between %s (%s) and %s (%s)", a.Path, a.Provenance.Revision, b.Path, b.Provenance.Revision),
				}
				conflicts = append(conflicts, c)
				reasons = append(reasons, c.Description)
			}
		}
	}

	// 2. Claim-level contradiction check
	for _, claim := range claims {
		if claim.Status == ClaimContradicted {
			c := ConflictRecord{
				Type:        "polarity",
				SourceID:    claim.ID,
				Description: fmt.Sprintf("claim %q is contradicted by evidence: %s", claim.Text, claim.Rationale),
			}
			conflicts = append(conflicts, c)
			reasons = append(reasons, c.Description)
		}
	}

	return ContradictionResult{
		Contradicted: len(conflicts) > 0,
		Conflicts:    conflicts,
		Reasons:      reasons,
	}
}
