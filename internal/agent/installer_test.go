package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTransactionalInstallerDryRun(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "installer-dryrun-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	installer := NewTransactionalInstaller()
	opts := SetupOptions{
		RepoRoot:   tmpDir,
		RepoName:   "dryrun-repo",
		HookBinary: "/bin/ctx-hook",
		MCPBinary:  "/bin/contextd",
		HomeDir:    tmpDir,
		DryRun:     true,
	}

	res := installer.Execute(opts)
	if !res.Success {
		t.Fatalf("expected dry run success, got error: %s", res.Error)
	}
	if !res.DryRun {
		t.Errorf("expected DryRun true")
	}
	if len(res.Plans) == 0 {
		t.Errorf("expected generated plans in dry run")
	}

	// Verify no files were actually written to tmpDir
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no files created during dry-run, found %d entries", len(entries))
	}
}

func TestTransactionalInstallerIdempotency(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "installer-idempotent-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	installer := NewTransactionalInstaller()
	opts := SetupOptions{
		RepoRoot:   tmpDir,
		RepoName:   "idempotent-repo",
		HookBinary: "/bin/ctx-hook",
		MCPBinary:  "/bin/contextd",
		HomeDir:    tmpDir,
		DryRun:     false,
		AgentID:    "cursor",
	}

	// First execution
	res1 := installer.Execute(opts)
	if !res1.Success {
		t.Fatalf("first execution failed: %s", res1.Error)
	}

	// Read content of .cursor/mcp.json
	mcpPath := filepath.Join(tmpDir, ".cursor", "mcp.json")
	content1, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", mcpPath, err)
	}

	// Second execution (idempotency check)
	res2 := installer.Execute(opts)
	if !res2.Success {
		t.Fatalf("second execution failed: %s", res2.Error)
	}

	content2, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("failed to read %s on second run: %v", mcpPath, err)
	}

	if string(content1) != string(content2) {
		t.Errorf("expected identical configuration on repeated setup, got diff:\nRun 1: %s\nRun 2: %s", string(content1), string(content2))
	}
}
