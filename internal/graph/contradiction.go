package graph

import (
	"math"
	"strings"
	"sync"
	"time"

	"contextos/internal/model"
)

// Typed relationship kinds per PR-09 specification.
const (
	RelationSupports    = "supports"
	RelationContradicts = "contradicts"
	RelationSupersedes  = "supersedes"
	RelationInvalidates = "invalidates"
	RelationDerivedFrom = "derived-from"
)

// MemoryRelation models an architectural dependency or conflict between memories.
type MemoryRelation struct {
	SrcID     string    `json:"src_id"`
	DstID     string    `json:"dst_id"`
	Kind      string    `json:"kind"`
	Weight    float64   `json:"weight"`
	Conflict  float64   `json:"conflict"` // kappa score
	Timestamp time.Time `json:"timestamp"`
}

// ContradictionReport documents detected semantic conflicts between memories.
type ContradictionReport struct {
	MemoryA    string  `json:"memory_a"`
	MemoryB    string  `json:"memory_b"`
	Kappa      float64 `json:"kappa"`
	Resolution string  `json:"resolution"` // "superseded_a", "superseded_b", "suppressed_inference", "unresolved_conflict"
	WinnerID   string  `json:"winner_id,omitempty"`
}

// ContradictionGraph manages memory relationships and automated conflict resolution.
type ContradictionGraph struct {
	mu        sync.RWMutex
	relations map[string][]MemoryRelation // srcID -> []relations
}

// NewContradictionGraph creates a contradiction graph.
func NewContradictionGraph() *ContradictionGraph {
	return &ContradictionGraph{
		relations: make(map[string][]MemoryRelation),
	}
}

// AddRelation records a directed relation.
func (cg *ContradictionGraph) AddRelation(rel MemoryRelation) {
	cg.mu.Lock()
	defer cg.mu.Unlock()
	cg.relations[rel.SrcID] = append(cg.relations[rel.SrcID], rel)
}

// ComputeKappa computes the formal contradiction score:
// kappa_ij = semanticConflict * authorityDifference * temporalOverlap
func ComputeKappa(m1, m2 model.Memory, semanticConflict float64) float64 {
	if semanticConflict <= 0 {
		return 0.0
	}

	auth1 := authorityWeight(m1.Authority)
	auth2 := authorityWeight(m2.Authority)
	authDiff := math.Abs(auth1 - auth2)
	// Higher authority difference indicates clear dominance
	authFactor := math.Max(0.5, authDiff)

	// Temporal overlap: if both valid currently, overlap is 1.0
	temporalOverlap := 1.0
	if m1.InvalidatedAtRevision != "" && m2.ValidFromRevision != "" {
		if m1.InvalidatedAtRevision < m2.ValidFromRevision {
			temporalOverlap = 0.0 // No overlap: m1 was invalidated before m2 was created
		}
	} else if m2.InvalidatedAtRevision != "" && m1.ValidFromRevision != "" {
		if m2.InvalidatedAtRevision < m1.ValidFromRevision {
			temporalOverlap = 0.0
		}
	}

	kappa := semanticConflict * authFactor * temporalOverlap
	if kappa < 0 {
		return 0
	}
	if kappa > 1.0 {
		return 1.0
	}
	return kappa
}

// Resolve filters conflicting memories, suppressing superseded or low-authority contradictions.
func (cg *ContradictionGraph) Resolve(memories []model.Memory, conflictThreshold float64) ([]model.Memory, []ContradictionReport) {
	if conflictThreshold <= 0 {
		conflictThreshold = 0.45
	}

	suppressed := make(map[string]bool)
	var reports []ContradictionReport

	for i := 0; i < len(memories); i++ {
		for j := i + 1; j < len(memories); j++ {
			m1 := memories[i]
			m2 := memories[j]

			// Explicit supersedes link
			if m1.Supersedes == m2.ID || m2.SupersededBy == m1.ID {
				suppressed[m2.ID] = true
				reports = append(reports, ContradictionReport{
					MemoryA:    m1.ID,
					MemoryB:    m2.ID,
					Kappa:      1.0,
					Resolution: "superseded_b",
					WinnerID:   m1.ID,
				})
				continue
			}
			if m2.Supersedes == m1.ID || m1.SupersededBy == m2.ID {
				suppressed[m1.ID] = true
				reports = append(reports, ContradictionReport{
					MemoryA:    m1.ID,
					MemoryB:    m2.ID,
					Kappa:      1.0,
					Resolution: "superseded_a",
					WinnerID:   m2.ID,
				})
				continue
			}

			// Estimate semantic conflict if scopes overlap
			if m1.Scope == m2.Scope || m1.Scope == "*" || m2.Scope == "*" {
				conflict := estimateConflict(m1.Content, m2.Content)
				kappa := ComputeKappa(m1, m2, conflict)

				if kappa >= conflictThreshold {
					auth1 := authorityWeight(m1.Authority)
					auth2 := authorityWeight(m2.Authority)

					var report ContradictionReport
					report.MemoryA = m1.ID
					report.MemoryB = m2.ID
					report.Kappa = kappa

					if auth1 > auth2 {
						suppressed[m2.ID] = true
						report.Resolution = "suppressed_inference"
						report.WinnerID = m1.ID
					} else if auth2 > auth1 {
						suppressed[m1.ID] = true
						report.Resolution = "suppressed_inference"
						report.WinnerID = m2.ID
					} else {
						// Equal authority: keep newer revision
						if m1.ValidFromRevision >= m2.ValidFromRevision {
							suppressed[m2.ID] = true
							report.Resolution = "temporal_recency_winner"
							report.WinnerID = m1.ID
						} else {
							suppressed[m1.ID] = true
							report.Resolution = "temporal_recency_winner"
							report.WinnerID = m2.ID
						}
					}
					reports = append(reports, report)
				}
			}
		}
	}

	var active []model.Memory
	for _, m := range memories {
		if !suppressed[m.ID] {
			active = append(active, m)
		}
	}

	return active, reports
}

func authorityWeight(auth string) float64 {
	switch strings.ToLower(auth) {
	case "user", "developer":
		return 1.0
	case "test", "verification":
		return 0.9
	case "commit", "git":
		return 0.7
	case "inference", "agent":
		return 0.4
	default:
		return 0.5
	}
}

// estimateConflict computes heuristic semantic polarity conflict between two textual claims.
func estimateConflict(c1, c2 string) float64 {
	s1 := strings.ToLower(c1)
	s2 := strings.ToLower(c2)

	// Direct negation patterns
	negations := []string{"not", "never", "avoid", "deprecated", "removed", "disabled", "don't", "no longer"}
	hasNeg1 := false
	hasNeg2 := false
	for _, neg := range negations {
		if strings.Contains(s1, neg) {
			hasNeg1 = true
		}
		if strings.Contains(s2, neg) {
			hasNeg2 = true
		}
	}

	if hasNeg1 != hasNeg2 {
		// One claims positive, one claims negative
		return 0.85
	}
	return 0.0
}
