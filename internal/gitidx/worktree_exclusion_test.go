// Package gitidx — worktree_exclusion_test.go
//
// R17 Phase 10: Proof-Oriented Regression Suite
// Theorem 7 & Gate R17.4 (Worktree Exclusion & WCR = 0):
// Asserts that agent-created worktree paths (.claude/worktrees, .codex/worktrees,
// .cursor/worktrees, etc.) are strictly excluded at ingestion time, guaranteeing
// Worktree Contamination Rate WCR = 0.
package gitidx

import (
	"testing"
)

func TestWorktreeExclusion_GateR17_4(t *testing.T) {
	policy := DefaultExclusionPolicy()

	testCases := []struct {
		path     string
		excluded bool
		reason   string
	}{
		// Canonical repository files — must NOT be excluded
		{"internal/store/sqlite_store.go", false, "core implementation"},
		{"cmd/contextd/main.go", false, "daemon entry point"},
		{"pkg/api/handler.go", false, "api package"},

		// Agent worktrees — MUST be excluded
		{".claude/worktrees/agent-123/main.go", true, "claude worktree"},
		{".cursor/worktrees/task-456/internal/store/sqlite_store.go", true, "cursor worktree"},
		{".codex/worktrees/branch-a/foo.go", true, "codex worktree"},
		{".agents/worktrees/session/bar.go", true, "generic agent worktree"},
		{".aider/worktrees/feat/baz.go", true, "aider worktree"},

		// Standard build/vendor exclusions
		{"vendor/github.com/foo/bar.go", true, "vendor tree"},
		{"node_modules/package/index.js", true, "node_modules"},
		{".git/config", true, "git dir"},
		{".contextos/manifest.json", true, "contextos metadata"},
	}

	contaminatedCount := 0
	totalAgentWorktreeCases := 0

	for _, tc := range testCases {
		isEx := policy.IsExcluded(tc.path)
		if isEx != tc.excluded {
			t.Errorf("path %q (%s): got excluded=%v, expected %v", tc.path, tc.reason, isEx, tc.excluded)
		}
		if tc.reason == "claude worktree" || tc.reason == "cursor worktree" ||
			tc.reason == "codex worktree" || tc.reason == "generic agent worktree" ||
			tc.reason == "aider worktree" {
			totalAgentWorktreeCases++
			if !isEx {
				contaminatedCount++
			}
		}
	}

	wcr := float64(contaminatedCount) / float64(totalAgentWorktreeCases)
	if wcr > 0.0 {
		t.Fatalf("Gate R17.4 Violated: Worktree Contamination Rate WCR = %.4f > 0.0", wcr)
	}
}
