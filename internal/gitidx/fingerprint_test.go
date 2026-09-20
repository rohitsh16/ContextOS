package gitidx

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHierarchicalFingerprintsAndDiff(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "fingerprint-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	subDir := filepath.Join(tmpDir, "pkg", "auth")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	file1 := filepath.Join(subDir, "login.go")
	if err := os.WriteFile(file1, []byte("package auth\nfunc Login() bool { return true }\n"), 0644); err != nil {
		t.Fatal(err)
	}

	file2 := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(file2, []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 1. Build baseline tree
	tree1, err := BuildHierarchicalFingerprints(tmpDir)
	if err != nil {
		t.Fatalf("BuildHierarchicalFingerprints failed: %v", err)
	}

	if tree1.RepoHash == "" {
		t.Errorf("expected non-empty RepoHash")
	}
	if len(tree1.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(tree1.Files))
	}

	// Diff with self should have no changes
	deltaSame := DiffFingerprints(tree1, tree1)
	if deltaSame.RepoChanged || len(deltaSame.ChangedFiles) > 0 {
		t.Errorf("expected no changes for identical tree, got %+v", deltaSame)
	}

	// 2. Modify one file
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(file1, []byte("package auth\nfunc Login() bool { return false }\nfunc Logout() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	tree2, err := BuildHierarchicalFingerprints(tmpDir)
	if err != nil {
		t.Fatalf("BuildHierarchicalFingerprints 2 failed: %v", err)
	}

	delta := DiffFingerprints(tree1, tree2)
	if !delta.RepoChanged {
		t.Errorf("expected RepoChanged true")
	}
	if len(delta.ChangedFiles) != 1 || delta.ChangedFiles[0] != filepath.Join("pkg", "auth", "login.go") {
		t.Errorf("expected only login.go changed, got %v", delta.ChangedFiles)
	}

	// main.go should NOT be marked as changed
	if tree1.Files["main.go"].Hash != tree2.Files["main.go"].Hash {
		t.Errorf("unaffected main.go hash should remain unchanged")
	}
}

func BenchmarkHierarchicalFingerprints(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "bench-fingerprint-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	for i := 0; i < 20; i++ {
		d := filepath.Join(tmpDir, "pkg", "mod")
		_ = os.MkdirAll(d, 0755)
		f := filepath.Join(d, "file.go")
		_ = os.WriteFile(f, []byte("package mod\nfunc Foo() {}\n"), 0644)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = BuildHierarchicalFingerprints(tmpDir)
	}
}
