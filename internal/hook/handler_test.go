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

func TestHandleAntigravityInjectsEphemeralSteps(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_ = runGit(root, "init", "-q")
	_ = runGit(root, "config", "user.email", "test@example.com")
	_ = runGit(root, "config", "user.name", "Test")
	_ = runGit(root, "add", ".")
	_ = runGit(root, "commit", "-qm", "init")

	db := filepath.Join(root, "ctx.db")
	os.Setenv("CONTEXTOS_DB", db)
	defer os.Unsetenv("CONTEXTOS_DB")

	raw := []byte(`{"workspacePaths":["` + root + `"],"conversationId":"c-123","prompt":"implement redlock failover"}`)
	out, err := Handle("antigravity", "PreInvocation", raw)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["injectSteps"]; !ok {
		t.Fatalf("expected Antigravity injectSteps, got %s", out)
	}

	stopRaw := []byte(`{"conversationId":"c-123"}`)
	stopOut, err := Handle("antigravity", "Stop", stopRaw)
	if err != nil {
		t.Fatal(err)
	}
	var stopMap map[string]any
	if err := json.Unmarshal(stopOut, &stopMap); err != nil {
		t.Fatal(err)
	}
	if stopMap["decision"] != "allow" {
		t.Fatalf("expected decision allow, got %s", stopOut)
	}
}

func TestHandleCursorReturnsAdditionalContext(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_ = runGit(root, "init", "-q")
	_ = runGit(root, "config", "user.email", "test@example.com")
	_ = runGit(root, "config", "user.name", "Test")
	_ = runGit(root, "add", ".")
	_ = runGit(root, "commit", "-qm", "init")

	db := filepath.Join(root, "ctx.db")
	os.Setenv("CONTEXTOS_DB", db)
	defer os.Unsetenv("CONTEXTOS_DB")

	s, err := openService(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Remember("decision", "Fix race condition using redis mutex", "user", "repo", "", 1.0, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	raw := []byte(`{"cwd":"` + root + `","session_id":"s-cursor","prompt":"Fix race condition using redis mutex"}`)
	out, err := Handle("cursor", "beforeSubmitPrompt", raw)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["continue"] != true {
		t.Fatalf("expected continue=true, got %s", out)
	}
	if _, ok := m["additional_context"]; !ok {
		t.Fatalf("expected additional_context in cursor response, got %s", out)
	}
}

func runGit(dir string, args ...string) error {
	c := exec.Command("git", args...)
	c.Dir = dir
	return c.Run()
}
