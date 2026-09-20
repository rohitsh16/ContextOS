package gitidx

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initTestRepo creates a temporary git repo initialized with a commit.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Tester",
			"GIT_AUTHOR_EMAIL=tester@example.com",
			"GIT_COMMITTER_NAME=Tester",
			"GIT_COMMITTER_EMAIL=tester@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\nOutput: %s", args, err, string(out))
		}
	}

	runGit("init")
	runGit("config", "user.name", "Tester")
	runGit("config", "user.email", "tester@example.com")

	// Create initial files
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "helper.go"), []byte("package main\n\nfunc Help() string { return \"help\" }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".contextos/\n*.ignored\n"), 0644); err != nil {
		t.Fatal(err)
	}

	runGit("add", ".")
	runGit("commit", "-m", "initial commit")

	return dir
}

func TestWorktreeFingerprintStateTransitions(t *testing.T) {
	repoDir := initTestRepo(t)

	// 1. Clean repo -> F1 ("clean")
	f1, err := ComputeWorktreeFingerprint(repoDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f1 != "clean" {
		t.Fatalf("expected 'clean' for clean repo, got %q", f1)
	}

	// 2. Edit tracked file -> F2
	origMain := "package main\n\nfunc main() {}\n"
	newMain := "package main\n\nfunc main() { println(\"hello\") }\n"
	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte(newMain), 0644); err != nil {
		t.Fatal(err)
	}
	f2, err := ComputeWorktreeFingerprint(repoDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f2 == f1 {
		t.Fatalf("expected F2 != F1 after editing tracked file, both are %q", f2)
	}

	// 3. Edit different tracked file -> F3
	if err := os.WriteFile(filepath.Join(repoDir, "helper.go"), []byte("package main\n\nfunc Help() string { return \"v2\" }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	f3, err := ComputeWorktreeFingerprint(repoDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f3 == f2 || f3 == f1 {
		t.Fatalf("expected F3 distinct, got F1=%q, F2=%q, F3=%q", f1, f2, f3)
	}

	// 4. Restore original content -> F1
	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte(origMain), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "helper.go"), []byte("package main\n\nfunc Help() string { return \"help\" }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	fRestore, err := ComputeWorktreeFingerprint(repoDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fRestore != f1 {
		t.Fatalf("expected restore to return F1 (%q), got %q", f1, fRestore)
	}

	// 5. Add untracked file -> F4
	untrackedPath := filepath.Join(repoDir, "untracked.go")
	if err := os.WriteFile(untrackedPath, []byte("package main\nvar x = 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	f4, err := ComputeWorktreeFingerprint(repoDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f4 == f1 {
		t.Fatalf("expected F4 != F1 after adding untracked file, got %q", f4)
	}

	// 6. Change untracked contents -> F5 (CRITICAL: In v0.6.0 status-hash was identical!)
	if err := os.WriteFile(untrackedPath, []byte("package main\nvar x = 999999\n"), 0644); err != nil {
		t.Fatal(err)
	}
	f5, err := ComputeWorktreeFingerprint(repoDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f5 == f4 {
		t.Fatalf("CRITICAL DEFECT: F5 == F4 (%q) when untracked file contents changed! False cache reuse detected!", f5)
	}

	// Remove untracked file to return to clean state
	_ = os.Remove(untrackedPath)

	// 7. Delete file -> F6
	if err := os.Remove(filepath.Join(repoDir, "helper.go")); err != nil {
		t.Fatal(err)
	}
	f6, err := ComputeWorktreeFingerprint(repoDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f6 == f1 {
		t.Fatalf("expected F6 != F1 after deleting file, got %q", f6)
	}

	// Restore deleted file
	if err := os.WriteFile(filepath.Join(repoDir, "helper.go"), []byte("package main\n\nfunc Help() string { return \"help\" }\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 8. Modify ignored file -> unchanged (F1)
	ignoredPath := filepath.Join(repoDir, "temp.ignored")
	if err := os.WriteFile(ignoredPath, []byte("should be ignored"), 0644); err != nil {
		t.Fatal(err)
	}
	fIgnored, err := ComputeWorktreeFingerprint(repoDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fIgnored != f1 {
		t.Fatalf("expected ignored file change to keep F1 (%q), got %q", f1, fIgnored)
	}

	// 9. Modify .contextos/ directory -> unchanged (F1)
	ctxOSDir := filepath.Join(repoDir, ".contextos")
	_ = os.MkdirAll(ctxOSDir, 0700)
	if err := os.WriteFile(filepath.Join(ctxOSDir, "internal.db"), []byte("sqlite internal data"), 0644); err != nil {
		t.Fatal(err)
	}
	fCtxOS, err := ComputeWorktreeFingerprint(repoDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fCtxOS != f1 {
		t.Fatalf("expected .contextos change to keep F1 (%q), got %q", f1, fCtxOS)
	}
}

func TestCollisionResistanceEmpirical(t *testing.T) {
	repoDir := initTestRepo(t)
	seen := make(map[string]int)

	const iterations = 1000
	for i := 0; i < iterations; i++ {
		// Mutate tracked file with random payload
		content := fmt.Sprintf("package main\n// mutation %d: %d\nfunc main() {}\n", i, rand.Int63())
		if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		fp, err := ComputeWorktreeFingerprint(repoDir)
		if err != nil {
			t.Fatalf("iteration %d: error %v", i, err)
		}

		if prev, ok := seen[fp]; ok {
			t.Fatalf("COLLISION OBSERVED! Mutation %d collided with mutation %d: fingerprint %q", i, prev, fp)
		}
		seen[fp] = i
	}
}

func TestDetectIntegration(t *testing.T) {
	repoDir := initTestRepo(t)
	repo, err := Detect(repoDir)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if repo.WorktreeHash != "clean" {
		t.Fatalf("expected clean worktree hash, got %q", repo.WorktreeHash)
	}

	// Mutate
	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("// dirty"), 0644); err != nil {
		t.Fatal(err)
	}
	repoDirty, err := Detect(repoDir)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if repoDirty.WorktreeHash == "clean" || len(repoDirty.WorktreeHash) != 16 {
		t.Fatalf("expected 16-char content-addressed hash, got %q", repoDirty.WorktreeHash)
	}
}
