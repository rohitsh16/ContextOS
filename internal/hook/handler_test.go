package hook

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestHandleClaudeInjectsDurableContext(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runGit(root, "init", "-q"); err != nil {
		t.Fatal(err)
	}
	if err := runGit(root, "config", "user.email", "test@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := runGit(root, "config", "user.name", "Test"); err != nil {
		t.Fatal(err)
	}
	if err := runGit(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := runGit(root, "commit", "-qm", "init"); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(root, "ctx.db")
	os.Setenv("CONTEXTOS_DB", db)
	defer os.Unsetenv("CONTEXTOS_DB")
	// Seed context using the service path through the normal hook itself on one event.
	raw := []byte(`{"cwd":"` + root + `","session_id":"s1","prompt":"implement kafka retry"}`)
	out, err := Handle("claude", "UserPromptSubmit", raw)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["hookSpecificOutput"]; !ok {
		t.Fatalf("expected Claude hookSpecificOutput, got %s", out)
	}
}

func runGit(dir string, args ...string) error {
	c := exec.Command("git", args...)
	c.Dir = dir
	return c.Run()
}
