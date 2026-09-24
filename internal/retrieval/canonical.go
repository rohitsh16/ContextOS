// Package retrieval — canonical.go
//
// R17 Phase 5: Candidate Canonicalization
//
// Implements the quotient-space deduplication described in R17 §3 and Theorem 1.
// The key operation is:
//
//	canonicalize → deduplicate → rank
//
// NOT rank → penalize duplicates.
//
// For any set of physical candidates that represent the same logical source
// (same repo + path + kind + name + lines), this module collapses them to a
// single canonical candidate. Provenance from all physical observations is
// retained but not counted as independent evidence.
package retrieval

import (
	"fmt"
	"path/filepath"
	"strings"
)

// CanonicalNodeKey is the stable logical identity for a source node, as
// defined in R17 §3.1 (Equivalence Relation).
//
// Two nodes are equivalent (v ~ u) iff their CanonicalNodeKey is equal.
type CanonicalNodeKey struct {
	RepoID string
	Path   string // canonical (slash-separated, repo-relative)
	Kind   string
	Name   string
	StartLine int
	EndLine   int
}

// String returns a compact human-readable representation of the key.
func (k CanonicalNodeKey) String() string {
	return fmt.Sprintf("%s|%s|%s|%s|%d-%d", k.RepoID, k.Path, k.Kind, k.Name, k.StartLine, k.EndLine)
}

// CanonicalNodeKeyFor constructs the canonical identity key for a Candidate.
func CanonicalNodeKeyFor(c Candidate, repoID string) CanonicalNodeKey {
	return CanonicalNodeKey{
		RepoID:    repoID,
		Path:      CanonicalSlashPath(c.Path),
		Kind:      c.Kind,
		Name:      c.Name,
		StartLine: c.StartLine,
		EndLine:   c.EndLine,
	}
}

// CanonicalSlashPath normalizes a file path to the canonical slash-separated
// form, stripping leading "./" segments.
func CanonicalSlashPath(p string) string {
	s := filepath.ToSlash(p)
	s = strings.TrimPrefix(s, "./")
	return s
}

// DeduplicationResult is the output of candidate canonicalization.
type DeduplicationResult struct {
	// Canonical is the deduplicated candidate list. Each entry represents one
	// logical node; its Score is the max across all physical observations.
	Canonical []Candidate

	// DuplicateCount is the number of physical candidates that were merged
	// into existing canonical entries (i.e., total input − len(Canonical)).
	DuplicateCount int

	// DAR is the Duplicate Amplification Ratio:
	//   DAR = DuplicateCount / len(input)
	// For a perfectly clean corpus, DAR = 0.
	DAR float64
}

// DeduplicateCandidates applies the R17 canonicalization invariant to a
// candidate slice. For every set of candidates that share the same
// CanonicalNodeKey, only the one with the highest Score is retained, but
// the provenance of all observations is merged.
//
// Theorem 1 (R17): After canonicalization, adding or removing duplicate
// physical copies cannot change the score of the logical candidate.
func DeduplicateCandidates(candidates []Candidate, repoID string) DeduplicationResult {
	if len(candidates) == 0 {
		return DeduplicationResult{}
	}

	type entry struct {
		cand  Candidate
		count int
	}

	seen := make(map[string]*entry, len(candidates))
	order := make([]string, 0, len(candidates))

	for _, c := range candidates {
		key := CanonicalNodeKeyFor(c, repoID).String()
		if e, ok := seen[key]; ok {
			// Merge: keep the higher score (max, not sum — prevents amplification)
			if c.Score > e.cand.Score {
				e.cand.Score = c.Score
				e.cand.LexicalScore = max64(e.cand.LexicalScore, c.LexicalScore)
				e.cand.GraphScore = max64(e.cand.GraphScore, c.GraphScore)
				e.cand.SemanticScore = max64(e.cand.SemanticScore, c.SemanticScore)
			}
			// Retain all provenance observations without treating them as independent
			e.cand.Provenance = mergeProvenance(e.cand.Provenance, c.Provenance)
			e.count++
		} else {
			cp := c
			seen[key] = &entry{cand: cp, count: 1}
			order = append(order, key)
		}
	}

	result := make([]Candidate, 0, len(order))
	duplicates := 0
	for _, key := range order {
		e := seen[key]
		result = append(result, e.cand)
		duplicates += e.count - 1
	}

	dar := 0.0
	if len(candidates) > 0 {
		dar = float64(duplicates) / float64(len(candidates))
	}

	return DeduplicationResult{
		Canonical:      result,
		DuplicateCount: duplicates,
		DAR:            dar,
	}
}

// MergeCandidatesWithTier merges tiered candidate slices respecting the
// priority order defined in R17 §24:
//
//	Tier 0 (exact) > Tier 1 (lexical) > Tier 2 (graph) > Tier 3 (semantic)
//
// Exact-tier matches dominate: if a node appears in Tier 0, no lower-tier
// entry for the same canonical key can override it.
func MergeCandidatesWithTier(exact, lexical, graph, semantic []Candidate, repoID string) []Candidate {
	const (
		tierExact    = 0
		tierLexical  = 1
		tierGraph    = 2
		tierSemantic = 3
	)

	type tierEntry struct {
		cand Candidate
		tier int
	}

	seen := make(map[string]*tierEntry)
	order := []string{}

	addCandidates := func(cands []Candidate, tier int) {
		for _, c := range cands {
			key := CanonicalNodeKeyFor(c, repoID).String()
			if e, ok := seen[key]; ok {
				if tier < e.tier {
					// Higher-priority tier takes over
					e.cand = c
					e.tier = tier
				} else if tier == e.tier && c.Score > e.cand.Score {
					e.cand.Score = c.Score
				}
				e.cand.Provenance = mergeProvenance(e.cand.Provenance, c.Provenance)
			} else {
				cp := c
				seen[key] = &tierEntry{cand: cp, tier: tier}
				order = append(order, key)
			}
		}
	}

	addCandidates(exact, tierExact)
	addCandidates(lexical, tierLexical)
	addCandidates(graph, tierGraph)
	addCandidates(semantic, tierSemantic)

	result := make([]Candidate, 0, len(order))
	for _, key := range order {
		result = append(result, seen[key].cand)
	}
	return result
}

func max64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func mergeProvenance(a, b []string) []string {
	seen := make(map[string]bool, len(a))
	result := make([]string, len(a))
	copy(result, a)
	for _, s := range a {
		seen[s] = true
	}
	for _, s := range b {
		if !seen[s] {
			result = append(result, s)
			seen[s] = true
		}
	}
	return result
}
