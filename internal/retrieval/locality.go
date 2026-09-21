package retrieval

import (
	"strings"
)

// LocalityTierName identifies the locality tier (PR.md Section 14).
type LocalityTierName string

const (
	TierHOT  LocalityTierName = "HOT"
	TierWARM LocalityTierName = "WARM"
	TierCOLD LocalityTierName = "COLD"
)

// LocalityTier represents a single tier in the locality hierarchy.
type LocalityTier struct {
	Name     LocalityTierName
	Priority int
	Paths    map[string]bool
	Packages map[string]bool
}

// Matches returns true if the candidate's path or package belongs to this locality tier.
func (t *LocalityTier) Matches(filePath, pkg string) bool {
	if t.Name == TierCOLD {
		return true // COLD covers the entire repository
	}
	if t.Paths[filePath] {
		return true
	}
	if t.Packages[pkg] {
		return true
	}
	for p := range t.Paths {
		if strings.HasPrefix(filePath, p) {
			return true
		}
	}
	return false
}

// LocalityHierarchy manages the HOT / WARM / COLD logical tiers.
type LocalityHierarchy struct {
	HOT  LocalityTier
	WARM LocalityTier
	COLD LocalityTier
}

// BuildLocalityHierarchy constructs the hierarchy from inferred scope and context.
func BuildLocalityHierarchy(scope Scope, ctx LocalizerContext) *LocalityHierarchy {
	hotPaths := make(map[string]bool)
	hotPkgs := make(map[string]bool)

	// HOT: modified files, current task files, current package
	for _, p := range scope.Primary {
		hotPaths[p] = true
	}
	for _, cf := range ctx.ChangedFiles {
		hotPaths[cf] = true
	}
	if ctx.CurrentPackage != "" {
		hotPkgs[ctx.CurrentPackage] = true
	}

	warmPaths := make(map[string]bool)
	warmPkgs := make(map[string]bool)

	// WARM: secondary scope, recent files, 1-2 hop dependencies
	for _, s := range scope.Secondary {
		if !hotPaths[s] {
			warmPaths[s] = true
		}
	}
	for _, rf := range ctx.RecentFiles {
		if !hotPaths[rf] {
			warmPaths[rf] = true
		}
	}

	return &LocalityHierarchy{
		HOT: LocalityTier{
			Name:     TierHOT,
			Priority: 1,
			Paths:    hotPaths,
			Packages: hotPkgs,
		},
		WARM: LocalityTier{
			Name:     TierWARM,
			Priority: 2,
			Paths:    warmPaths,
			Packages: warmPkgs,
		},
		COLD: LocalityTier{
			Name:     TierCOLD,
			Priority: 3,
			Paths:    nil,
			Packages: nil,
		},
	}
}

// CascadeSearch executes a cascading search: HOT -> WARM -> COLD (PR.md Section 14).
// Stops early when candidate count >= targetK and highest score >= minConfidence.
func (h *LocalityHierarchy) CascadeSearch(
	targetK int,
	minConfidence float64,
	searchTierFunc func(tier LocalityTierName) (candidates []Candidate, err error),
) (results []Candidate, tiersSearched []LocalityTierName, err error) {
	tiers := []LocalityTierName{TierHOT, TierWARM, TierCOLD}
	seen := make(map[string]bool)

	for _, tier := range tiers {
		tiersSearched = append(tiersSearched, tier)

		tierCandidates, searchErr := searchTierFunc(tier)
		if searchErr != nil {
			return results, tiersSearched, searchErr
		}

		for _, c := range tierCandidates {
			if !seen[c.NodeID] {
				seen[c.NodeID] = true
				results = append(results, c)
			}
		}

		// Check early stopping criteria:
		// Sufficient candidates found AND at least one high-confidence candidate
		if len(results) >= targetK {
			var maxScore float64
			for _, c := range results {
				if c.Score > maxScore {
					maxScore = c.Score
				}
			}
			if maxScore >= minConfidence {
				// Confident match in current tier; early stop
				break
			}
		}
	}

	return results, tiersSearched, nil
}
