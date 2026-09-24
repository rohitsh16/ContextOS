// Package retrieval — evidence.go
//
// R17 Phase 6: Evidence State Machine and Planner Soundness Contract
//
// Implements the typed evidence status described in R17 §15, §27, and §32 (Test Family E).
//
// Core soundness theorem (R17 Theorem 3):
//   If Retrieve(q) = ∅ and no fallback has produced authoritative evidence,
//   then Plan(q) = ∅.
//
// The planner must NEVER synthesize context from nothing. This module
// provides the types and helper functions that enforce this contract.
package retrieval

import "fmt"

// EvidenceStatus represents the typed state of the retrieval pipeline at the
// point the planner is called (R17 §15).
type EvidenceStatus uint8

const (
	// EvidenceUnknown is the zero value before retrieval completes.
	EvidenceUnknown EvidenceStatus = iota

	// EvidenceExactFound: authoritative exact path/symbol match located.
	// Recall@1 should be 1.0 when this status is reached.
	EvidenceExactFound

	// EvidenceLexicalFound: FTS/keyword retrieval produced at least one
	// candidate that meets the minimum evidence threshold.
	EvidenceLexicalFound

	// EvidenceGraphFound: bounded graph expansion from a validated seed
	// produced additional related candidates.
	EvidenceGraphFound

	// EvidenceSemanticFound: semantic/embedding retrieval produced candidates
	// above the relevance lower-confidence bound.
	EvidenceSemanticFound

	// EvidenceInsufficient: some candidates were found but confidence is
	// below the relevance threshold τ_R. May trigger a broader search.
	EvidenceInsufficient

	// EvidenceNone: no retrieval source returned any candidate for this query.
	// The planner MUST NOT produce non-empty selected context in this state
	// without an explicit declared fallback.
	EvidenceNone
)

func (es EvidenceStatus) String() string {
	switch es {
	case EvidenceExactFound:
		return "EXACT_FOUND"
	case EvidenceLexicalFound:
		return "LEXICAL_FOUND"
	case EvidenceGraphFound:
		return "GRAPH_FOUND"
	case EvidenceSemanticFound:
		return "SEMANTIC_FOUND"
	case EvidenceInsufficient:
		return "INSUFFICIENT"
	case EvidenceNone:
		return "NO_EVIDENCE"
	default:
		return "UNKNOWN"
	}
}

// HasEvidence reports whether the status indicates at least some usable
// candidates exist.
func (es EvidenceStatus) HasEvidence() bool {
	switch es {
	case EvidenceExactFound, EvidenceLexicalFound, EvidenceGraphFound, EvidenceSemanticFound:
		return true
	}
	return false
}

// IsAuthoritative reports whether the status represents an authoritative
// (deterministic, exact) match.
func (es EvidenceStatus) IsAuthoritative() bool {
	return es == EvidenceExactFound
}

// EvidencePackage is the typed output of the retrieval layer, consumed by the
// planner. The planner must inspect EvidencePackage.Status before acting.
//
// R17 §27: A zero-result search must produce EvidenceNone, not a confident
// selection over unrelated evidence.
type EvidencePackage struct {
	// Status is the typed evidence state (R17 §15).
	Status EvidenceStatus `json:"status"`

	// QueryClass is the classification used to drive retrieval.
	QueryClass QueryClass `json:"query_class"`

	// Candidates is the canonically-deduplicated, tier-ordered list.
	// Empty when Status == EvidenceNone.
	Candidates []Candidate `json:"candidates"`

	// DuplicateAmplificationRatio measures how many raw candidates were
	// collapsed during canonicalization (DAR from R17 §38).
	// DAR = 0 means the corpus had no duplicates.
	DuplicateAmplificationRatio float64 `json:"dar"`

	// WorktreeContaminationCount is the number of candidates filtered because
	// they came from agent-worktree subtrees (WCR numerator).
	// If the exclusion policy works correctly this should always be 0 at
	// retrieval time (they never enter the index).
	WorktreeContaminationCount int `json:"wcr_count"`

	// FallbackRequired is true when Status == EvidenceNone and a broader
	// search should be attempted before surfacing a failure to the user.
	FallbackRequired bool `json:"fallback_required"`

	// Confidence is a calibrated [0,1] estimate of P(relevant | evidence).
	// Should not be treated as a probability unless calibrated empirically
	// (R17 §16). Use LCB for safety-sensitive selection.
	Confidence float64 `json:"confidence"`
}

// NoEvidencePackage returns the canonical empty-evidence package for a query.
// Encodes R17 §27 and Theorem 3: empty retrieval is explicit, not silent.
func NoEvidencePackage(qc QueryClass) EvidencePackage {
	return EvidencePackage{
		Status:           EvidenceNone,
		QueryClass:       qc,
		Candidates:       nil,
		FallbackRequired: true,
		Confidence:       0.0,
	}
}

// PlannerSoundnessError is returned by AssertPlannerSoundness when the planner
// contract (Theorem 3) would be violated.
type PlannerSoundnessError struct {
	Status        EvidenceStatus
	AttemptedSize int
}

func (e *PlannerSoundnessError) Error() string {
	return fmt.Sprintf("planner soundness violation: attempted to produce %d selected candidates from status %s (NO_EVIDENCE contract requires 0)", e.AttemptedSize, e.Status)
}

// AssertPlannerSoundness validates that the planner contract is satisfied.
// Returns a PlannerSoundnessError if a NO_EVIDENCE state has been combined
// with a non-empty selection — which would violate Theorem 3.
//
// Usage: call this at the start of ExecutePlan after obtaining an EvidencePackage.
func AssertPlannerSoundness(pkg EvidencePackage, selectedCandidates []Candidate) error {
	if pkg.Status == EvidenceNone && len(selectedCandidates) > 0 {
		return &PlannerSoundnessError{
			Status:        pkg.Status,
			AttemptedSize: len(selectedCandidates),
		}
	}
	return nil
}

// ClassifyEvidenceStatus determines the EvidenceStatus from a candidate slice
// and a query classification. Used internally to construct EvidencePackages.
func ClassifyEvidenceStatus(qc QueryClassification, exact, lexical, graph []Candidate, minConfidence float64) EvidenceStatus {
	if len(exact) > 0 && qc.IsExact {
		return EvidenceExactFound
	}
	if len(exact) > 0 {
		return EvidenceLexicalFound
	}
	if len(lexical) > 0 {
		// Simple confidence proxy: top lexical score
		if len(lexical) > 0 && lexical[0].Score >= minConfidence {
			return EvidenceLexicalFound
		}
		return EvidenceInsufficient
	}
	if len(graph) > 0 {
		return EvidenceGraphFound
	}
	return EvidenceNone
}
