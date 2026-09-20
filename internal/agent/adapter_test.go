package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryHasAllAdapters(t *testing.T) {
	expected := []string{"antigravity", "claude", "codex", "cursor", "gemini"}
	all := AllIDs()
	if len(all) != len(expected) {
		t.Fatalf("expected %d adapters, got %d: %v", len(expected), len(all), all)
	}
	for i, id := range expected {
		if all[i] != id {
			t.Errorf("expected adapter %s at index %d, got %s", id, i, all[i])
		}
		adapter, ok := Get(id)
		if !ok || adapter == nil {
			t.Errorf("adapter %s not found in registry", id)
		}
		if adapter.ID() != id {
			t.Errorf("adapter ID mismatch: %s vs %s", adapter.ID(), id)
		}
		if adapter.Name() == "" {
			t.Errorf("adapter %s has empty Name()", id)
		}
	}
}

func TestAdapterCapabilities(t *testing.T) {
	for _, a := range List() {
		caps := a.Capabilities()
		if !caps.MCP {
			t.Errorf("adapter %s expected to support MCP", a.ID())
		}
		if !caps.ProjectConfig {
			t.Errorf("adapter %s expected to support ProjectConfig", a.ID())
		}
	}
}

func TestAdapterPlanAndApply(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-adapter-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	installCtx := InstallContext{
		RepoRoot:   tmpDir,
		RepoName:   "test-repo",
		HookBinary: "/usr/local/bin/ctx-hook",
		MCPBinary:  "/usr/local/bin/contextd",
		HomeDir:    tmpDir,
		DryRun:     false,
	}

	for _, a := range List() {
		plan, err := a.Plan(installCtx)
		if err != nil {
			t.Fatalf("adapter %s Plan() failed: %v", a.ID(), err)
		}
		if plan.AdapterID != a.ID() {
			t.Errorf("plan AdapterID mismatch: got %s, want %s", plan.AdapterID, a.ID())
		}
		if len(plan.Actions) == 0 {
			t.Errorf("adapter %s generated empty plan actions", a.ID())
		}
		if len(plan.AffectedFiles) == 0 {
			t.Errorf("adapter %s generated empty affected files", a.ID())
		}

		// Apply the plan
		if err := a.Apply(installCtx, plan); err != nil {
			t.Fatalf("adapter %s Apply() failed: %v", a.ID(), err)
		}

		// Validate the installation
		valCtx := ValidateContext{
			RepoRoot: tmpDir,
			HomeDir:  tmpDir,
		}
		res := a.Validate(valCtx)
		if !res.Valid {
			t.Errorf("adapter %s Validate() failed: %v", a.ID(), res.Issues)
		}

		// Test removal
		remCtx := RemoveContext{
			RepoRoot: tmpDir,
			HomeDir:  tmpDir,
		}
		if err := a.Remove(remCtx); err != nil {
			t.Errorf("adapter %s Remove() failed: %v", a.ID(), err)
		}
	}
}

func TestAdapterDetection(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-detect-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a mock .cursor folder
	if err := os.MkdirAll(filepath.Join(tmpDir, ".cursor"), 0755); err != nil {
		t.Fatal(err)
	}

	detectCtx := DetectContext{
		RepoRoot: tmpDir,
		HomeDir:  tmpDir,
	}

	cursorAdapter, ok := Get("cursor")
	if !ok {
		t.Fatal("cursor adapter not found")
	}
	res := cursorAdapter.Detect(detectCtx)
	if !res.Installed {
		t.Errorf("expected cursor to be detected as installed")
	}

	claudeAdapter, ok := Get("claude")
	if !ok {
		t.Fatal("claude adapter not found")
	}
	resClaude := claudeAdapter.Detect(detectCtx)
	if resClaude.Installed {
		t.Errorf("expected claude not to be detected in empty repo")
	}
}
