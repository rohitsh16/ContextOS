package temporal

import (
	"path/filepath"
	"strings"

	"contextos/internal/graph"
	"contextos/internal/model"
)

// ScopeType enumerates valid scopes for a memory.
type ScopeType string

const (
	ScopeRepository ScopeType = "repository"
	ScopeDirectory  ScopeType = "directory"
	ScopeFile       ScopeType = "file"
	ScopeSymbol     ScopeType = "symbol"
	ScopeTest       ScopeType = "test"
	ScopeWorkItem   ScopeType = "work-item"
	ScopeBranch     ScopeType = "branch"
)

// ScopedStalenessEvaluator computes fine-grained staleness risk using version-aware
// validity intervals and path-scoped diff intersections (PR-08).
type ScopedStalenessEvaluator struct {
	ChangedFiles    map[string]string // path -> status ("M", "A", "D")
	CurrentRevision string
	DepGraph        *graph.Graph // Optional AST dependency graph for transitive checks
}

func NewScopedStalenessEvaluator(changedFiles map[string]string, currentRev string, depGraph *graph.Graph) *ScopedStalenessEvaluator {
	return &ScopedStalenessEvaluator{
		ChangedFiles:    changedFiles,
		CurrentRevision: currentRev,
		DepGraph:        depGraph,
	}
}

// EvaluateStaleness calculates freshRisk [0.0, 1.0] and hardStale (true if invalid/deleted).
// It guarantees that changes to unrelated files do not penalize a scoped memory.
func (e *ScopedStalenessEvaluator) EvaluateStaleness(m model.Memory) (float64, bool) {
	// 1. Explicit invalidation
	if m.InvalidatedAtRevision != "" {
		return 1.0, true
	}
	if m.SupersededBy != "" {
		return 1.0, true
	}

	// 2. Future information leak prevention (PR.md R0-H4):
	// Evidence created in future revisions is invalid / hard-stale for historical replay at e.CurrentRevision
	if e.CurrentRevision != "" && m.ValidFromRevision != "" {
		if m.ValidFromRevision > e.CurrentRevision {
			return 1.0, true
		}
	}

	// 3. If revision matches current revision, it is definitely fresh
	if m.ValidFromRevision != "" && m.ValidFromRevision == e.CurrentRevision {
		return 0.0, false
	}

	// 3. Collect scoped paths for this memory
	scopedPaths := make(map[string]bool)
	if m.Location != "" {
		scopedPaths[filepath.ToSlash(filepath.Clean(m.Location))] = true
	}
	for _, loc := range m.Locations {
		if loc != "" {
			scopedPaths[filepath.ToSlash(filepath.Clean(loc))] = true
		}
	}
	for _, ev := range m.Evidence {
		if ev.Path != "" {
			scopedPaths[filepath.ToSlash(filepath.Clean(ev.Path))] = true
		}
	}

	// 4. If memory has no file scope or is purely repository-level,
	// apply slight temporal decay if revisions differ
	if len(scopedPaths) == 0 || m.Scope == string(ScopeRepository) {
		if m.ValidFromRevision != "" && e.CurrentRevision != "" && m.ValidFromRevision != e.CurrentRevision {
			// Freshness decay for unscoped repo memories when revisions differ
			return 0.25, false
		}
		return 0.0, false
	}

	// 5. Scoped diff intersection:
	// StaleRisk(m) = 1 - prod_{f in Scope(m)} (1 - p_f)
	intersection := false
	anyDeleted := false
	prodUnchanged := 1.0

	for path := range scopedPaths {
		for chPath, status := range e.ChangedFiles {
			cleanCh := filepath.ToSlash(filepath.Clean(chPath))
			if cleanCh == path || strings.HasPrefix(cleanCh, path+"/") || strings.HasPrefix(path, cleanCh+"/") {
				intersection = true
				pf := 0.80
				if status == "D" {
					anyDeleted = true
					pf = 1.0
				}
				prodUnchanged *= (1.0 - pf)
			}
		}
	}

	// Transitive dependency checking if dependency graph is available
	if !intersection && e.DepGraph != nil {
		for path := range scopedPaths {
			// Check neighbors in dependency graph
			for chPath := range e.ChangedFiles {
				cleanCh := filepath.ToSlash(filepath.Clean(chPath))
				if e.DepGraph.HasEdge(path, cleanCh) || e.DepGraph.HasEdge(cleanCh, path) {
					intersection = true
					pf := 0.35 // Transitive change impact
					prodUnchanged *= (1.0 - pf)
					break
				}
			}
		}
	}

	if !intersection {
		// Crucial PR-08 property:
		// Memory about README.md does not become invalid because payment_service.go changed!
		return 0.0, false
	}

	if anyDeleted {
		return 1.0, true
	}

	staleRisk := 1.0 - prodUnchanged
	if staleRisk > 0.90 {
		staleRisk = 0.90
	}
	return staleRisk, false
}
