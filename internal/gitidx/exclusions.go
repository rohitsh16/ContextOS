// Package gitidx provides git repository inspection, worktree fingerprinting,
// and source-file/symbol enumeration.
//
// R17 — Phase 1: Retrieval Boundary
//
// This file defines the authoritative exclusion policy and canonical-path
// normalization rules that ensure only files inside the canonical repository
// root are admitted to the index (Theorems 6 and 7 from R17).
package gitidx

import (
	"fmt"
	"path/filepath"
	"strings"
)

// AgentWorktreeRoots lists the known root directories that coding agents use to
// store nested worktrees, scratch checkouts, and ephemeral state.
// These paths must be excluded at index time (not query time) to prevent
// duplicate-evidence amplification (R17 §11, Theorem 7).
var AgentWorktreeRoots = []string{
	".claude/worktrees",
	".codex/worktrees",
	".cursor/worktrees",
	".agents/worktrees",
	".copilot/worktrees",
	".aider/worktrees",
	".cody/worktrees",
	".continue/worktrees",
}

// ExclusionPolicy is the configurable exclusion specification for a repository.
// It is applied during ingestion, before any indexing, to enforce the
// Authoritative Admissible Node Set A(S) from R17 §2.
type ExclusionPolicy struct {
	// DirectoryNames are exact directory-name segments that are excluded
	// wherever they appear in a path (e.g. "vendor", "node_modules").
	DirectoryNames map[string]bool

	// PathPrefixes are repository-relative path prefixes (slash-separated)
	// that are excluded (e.g. ".claude/worktrees").
	PathPrefixes []string

	// ExcludedExtensions are file extensions (including the dot) to skip.
	ExcludedExtensions map[string]bool
}

// DefaultExclusionPolicy constructs the baseline exclusion policy including
// standard generated/vendor directories and all known agent worktree roots.
func DefaultExclusionPolicy() ExclusionPolicy {
	return ExclusionPolicy{
		DirectoryNames: map[string]bool{
			".git":         true,
			".contextos":   true,
			"node_modules": true,
			"vendor":       true,
			"dist":         true,
			"build":        true,
			"target":       true,
			"__pycache__":  true,
			".mypy_cache":  true,
			".pytest_cache": true,
		},
		PathPrefixes: append([]string{}, AgentWorktreeRoots...),
		ExcludedExtensions: map[string]bool{
			".db":    true,
			".db-shm": true,
			".db-wal": true,
			".log":   true,
			".tmp":   true,
		},
	}
}

// IsExcluded reports whether a repository-relative path (slash-separated) is
// excluded by this policy. This is the single authoritative exclusion gate.
//
// Invariant: for any file f admitted by IsExcluded(p)==false, f ∈ A(S).
func (ep *ExclusionPolicy) IsExcluded(relPath string) bool {
	clean := filepath.ToSlash(relPath)

	// Check excluded path prefixes (worktree roots, etc.)
	for _, prefix := range ep.PathPrefixes {
		if strings.HasPrefix(clean, prefix+"/") || clean == prefix {
			return true
		}
	}

	// Check each directory-name segment
	parts := strings.Split(clean, "/")
	for _, part := range parts {
		if ep.DirectoryNames[part] {
			return true
		}
	}

	// Check file extension
	if len(parts) > 0 {
		base := parts[len(parts)-1]
		ext := strings.ToLower(filepath.Ext(base))
		if ep.ExcludedExtensions[ext] {
			return true
		}
	}

	return false
}

// ExclusionTelemetry counts paths considered vs. excluded for audit purposes.
type ExclusionTelemetry struct {
	Considered      int
	ExcludedPolicy  int // excluded by directory-name or extension rule
	ExcludedWorktree int // excluded specifically by agent-worktree prefix
}

// IsExcludedWithTelemetry is like IsExcluded but updates telemetry counters.
func (ep *ExclusionPolicy) IsExcludedWithTelemetry(relPath string, t *ExclusionTelemetry) bool {
	t.Considered++
	clean := filepath.ToSlash(relPath)

	for _, prefix := range ep.PathPrefixes {
		if strings.HasPrefix(clean, prefix+"/") || clean == prefix {
			t.ExcludedWorktree++
			return true
		}
	}

	parts := strings.Split(clean, "/")
	for _, part := range parts {
		if ep.DirectoryNames[part] {
			t.ExcludedPolicy++
			return true
		}
	}

	if len(parts) > 0 {
		base := parts[len(parts)-1]
		ext := strings.ToLower(filepath.Ext(base))
		if ep.ExcludedExtensions[ext] {
			t.ExcludedPolicy++
			return true
		}
	}

	return false
}

// CanonicalPath computes the repository-relative canonical path for a file
// given the repository root. It enforces:
//
//  1. Separator normalization (always forward slash)
//  2. Removal of . segments
//  3. Rejection of paths escaping root (path traversal)
//  4. Rejection of paths inside excluded subtrees
//
// Returns an error if the path is invalid or escapes the root.
func CanonicalPath(repoRoot, absPath string) (string, error) {
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", fmt.Errorf("canonicalize: invalid root %q: %w", repoRoot, err)
	}
	absFile, err := filepath.Abs(absPath)
	if err != nil {
		return "", fmt.Errorf("canonicalize: invalid path %q: %w", absPath, err)
	}

	// Normalize separators
	absRoot = filepath.ToSlash(absRoot)
	absFile = filepath.ToSlash(absFile)

	// Must be inside root
	rootWithSlash := strings.TrimSuffix(absRoot, "/") + "/"
	if !strings.HasPrefix(absFile, rootWithSlash) && absFile != absRoot {
		return "", fmt.Errorf("canonicalize: path %q escapes repository root %q", absPath, repoRoot)
	}

	rel := strings.TrimPrefix(absFile, rootWithSlash)
	if rel == "" {
		rel = "."
	}
	return rel, nil
}
