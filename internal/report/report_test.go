package report

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"contextos/internal/server"
)

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.Run()
}

func TestReportGeneration(t *testing.T) {
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

	s, err := server.New(dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.Index(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Remember("decision", "Use outbox for payment events", "user", "repo", "", 0.9, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Remember("failure", "Direct concurrent writes fail without lock", "user", "repo", "", 0.9, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Plan("payment events", "gpt-4o", 1000); err != nil {
		t.Fatal(err)
	}

	rep, err := Generate(s)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if rep.Summary.TotalTraces != 1 {
		t.Errorf("expected 1 trace, got %d", rep.Summary.TotalTraces)
	}
	if rep.Summary.TotalMemories != 2 {
		t.Errorf("expected 2 memories, got %d", rep.Summary.TotalMemories)
	}
	if rep.Summary.DecisionsCount != 1 {
		t.Errorf("expected 1 decision, got %d", rep.Summary.DecisionsCount)
	}
	if rep.Summary.FailuresCount != 1 {
		t.Errorf("expected 1 failure, got %d", rep.Summary.FailuresCount)
	}

	md := rep.ToMarkdown()
	if !strings.Contains(md, "# ContextOS Empirical Evaluation & Benchmark Report") {
		t.Errorf("markdown missing title: %s", md)
	}
	if !strings.Contains(md, "Executive Summary") {
		t.Errorf("markdown missing executive summary: %s", md)
	}

	jsonStr, err := rep.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}
	if !strings.Contains(jsonStr, `"total_traces": 1`) {
		t.Errorf("json missing expected content: %s", jsonStr)
	}
}
