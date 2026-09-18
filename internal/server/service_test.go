package server

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestServicePersistenceAndCache(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "ctx.db")
	if err := runGit(root, "init", "-q"); err != nil {
		t.Fatal(err)
	}
	if err := runGit(root, "config", "user.email", "test@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := runGit(root, "config", "user.name", "Test"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runGit(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := runGit(root, "commit", "-qm", "init"); err != nil {
		t.Fatal(err)
	}

	s, e := New(dbPath, root)
	if e != nil {
		t.Fatal(e)
	}
	if e := s.Index(); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Remember("decision", "Use outbox for payment events", "user", "repo", "", 0.9, nil); e != nil {
		t.Fatal(e)
	}
	if p, e := s.Plan("payment events", "gpt-5.3-codex", 10); e != nil || len(p.Selected) != 1 {
		t.Fatalf("plan failed: %+v %v", p, e)
	}
	s.Close()

	s, e = New(dbPath, root)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if p, e := s.Plan("payment events", "gpt-5.3-codex", 10); e != nil || len(p.Selected) != 1 {
		t.Fatalf("cached plan failed: %+v %v", p, e)
	}
	tr, e := s.LatestTrace()
	if e != nil {
		t.Fatal(e)
	}
	if tr["cache_hit"] != "1" {
		t.Fatalf("expected cache hit, got %#v", tr)
	}
}
func TestWorktreeChangeBreaksContextCache(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "ctx.db")
	if err := runGit(root, "init", "-q"); err != nil {
		t.Fatal(err)
	}
	if err := runGit(root, "config", "user.email", "test@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := runGit(root, "config", "user.name", "Test"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runGit(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := runGit(root, "commit", "-qm", "init"); err != nil {
		t.Fatal(err)
	}
	s, err := New(dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Remember("decision", "Use outbox for payment events", "user", "repo", "", 0.9, nil); err != nil {
		t.Fatal(err)
	}
	p1, err := s.Plan("payment events", "gpt-5.3-codex", 100)
	if err != nil {
		t.Fatal(err)
	}
	if p1.CacheHit {
		t.Fatal("first plan unexpectedly cached")
	}
	_, err = s.Plan("payment events", "gpt-5.3-codex", 100)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n// changed\nfunc main() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	p3, err := s.Plan("payment events", "gpt-5.3-codex", 100)
	if err != nil {
		t.Fatal(err)
	}
	if p3.CacheHit {
		t.Fatal("worktree mutation incorrectly reused cache")
	}
}

func runGit(dir string, args ...string) error {
	c := exec.Command("git", args...)
	c.Dir = dir
	return c.Run()
}
