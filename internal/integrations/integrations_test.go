package integrations

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallAllIsIdempotentAndCreatesMCP(t *testing.T) {
	root := t.TempDir()
	for _, a := range []string{"claude", "cursor", "codex", "gemini", "antigravity"} {
		if _, err := Install(a, root, "/tmp/contextos/ctx-hook", "/tmp/contextos/contextd"); err != nil {
			t.Fatal(a, err)
		}
		if _, err := Install(a, root, "/tmp/contextos/ctx-hook", "/tmp/contextos/contextd"); err != nil {
			t.Fatal(a, err)
		}
	}
	for _, p := range []string{
		filepath.Join(root, ".mcp.json"),
		filepath.Join(root, ".cursor", "mcp.json"),
		filepath.Join(root, ".cursor", "rules", "contextos.mdc"),
		filepath.Join(root, ".codex", "config.toml"),
		filepath.Join(root, ".gemini", "settings.json"),
		filepath.Join(root, ".agents", "mcp_config.json"),
		filepath.Join(root, ".agents", "hooks.json"),
		filepath.Join(root, ".agents", "rules", "contextos.md"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("missing %s: %v", p, err)
		}
	}
	var c map[string]any
	b, _ := os.ReadFile(filepath.Join(root, ".mcp.json"))
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	if _, ok := c["mcpServers"]; !ok {
		t.Fatal("missing mcpServers")
	}

	var agyMcp map[string]any
	ab, _ := os.ReadFile(filepath.Join(root, ".agents", "mcp_config.json"))
	if err := json.Unmarshal(ab, &agyMcp); err != nil {
		t.Fatal(err)
	}
	if _, ok := agyMcp["mcpServers"]; !ok {
		t.Fatal("missing Antigravity mcpServers")
	}

	toml, _ := os.ReadFile(filepath.Join(root, ".codex", "config.toml"))
	if strings.Count(string(toml), "[mcp_servers.contextos]") != 1 {
		t.Fatal("Codex MCP stanza not idempotent")
	}
}

func TestInstallPreservesUnrelatedConfig(t *testing.T) {
	root := t.TempDir()

	// Pre-create Cursor hooks with an unrelated user hook
	cursorHookDir := filepath.Join(root, ".cursor")
	if err := os.MkdirAll(cursorHookDir, 0700); err != nil {
		t.Fatal(err)
	}
	initialCursorHooks := map[string]any{
		"version": 1,
		"hooks": map[string]any{
			"sessionStart": []any{
				map[string]any{
					"command": "user-custom-script.sh",
				},
			},
		},
		"userCustomKey": "userCustomValue",
	}
	initBytes, _ := json.Marshal(initialCursorHooks)
	if err := os.WriteFile(filepath.Join(cursorHookDir, "hooks.json"), initBytes, 0600); err != nil {
		t.Fatal(err)
	}

	// Pre-create .mcp.json with unrelated MCP server
	initialMCP := map[string]any{
		"mcpServers": map[string]any{
			"unrelated-server": map[string]any{
				"command": "python",
				"args":    []any{"-m", "custom_mcp"},
			},
		},
	}
	mcpBytes, _ := json.Marshal(initialMCP)
	if err := os.WriteFile(filepath.Join(root, ".mcp.json"), mcpBytes, 0600); err != nil {
		t.Fatal(err)
	}

	// Install cursor and claude
	if _, err := Install("cursor", root, "/usr/local/bin/ctx-hook", "/usr/local/bin/contextd"); err != nil {
		t.Fatal(err)
	}
	if _, err := Install("claude", root, "/usr/local/bin/ctx-hook", "/usr/local/bin/contextd"); err != nil {
		t.Fatal(err)
	}

	// Verify Cursor hooks preserved userCustomKey and custom hook
	var cursorResult map[string]any
	cb, _ := os.ReadFile(filepath.Join(cursorHookDir, "hooks.json"))
	if err := json.Unmarshal(cb, &cursorResult); err != nil {
		t.Fatal(err)
	}
	if cursorResult["userCustomKey"] != "userCustomValue" {
		t.Fatalf("expected userCustomKey preserved, got %v", cursorResult["userCustomKey"])
	}
	hooksMap := cursorResult["hooks"].(map[string]any)
	sessionStartArr := hooksMap["sessionStart"].([]any)
	var foundUserHook bool
	for _, item := range sessionStartArr {
		if m, ok := item.(map[string]any); ok && m["command"] == "user-custom-script.sh" {
			foundUserHook = true
		}
	}
	if !foundUserHook {
		t.Fatalf("unrelated user hook was removed during install")
	}

	// Verify .mcp.json preserved unrelated-server and added contextos
	var mcpResult map[string]any
	mb, _ := os.ReadFile(filepath.Join(root, ".mcp.json"))
	if err := json.Unmarshal(mb, &mcpResult); err != nil {
		t.Fatal(err)
	}
	servers := mcpResult["mcpServers"].(map[string]any)
	if _, ok := servers["unrelated-server"]; !ok {
		t.Fatal("unrelated-server was removed during install")
	}
	if _, ok := servers["contextos"]; !ok {
		t.Fatal("contextos server was not added during install")
	}
}

func TestResolveBinaryStrategy(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0700); err != nil {
		t.Fatal(err)
	}

	// Create dummy executable in repo-local ./bin/
	dummyCtxHook := filepath.Join(binDir, "ctx-hook")
	if err := os.WriteFile(dummyCtxHook, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}

	// Test repo-local resolution
	resolved, err := ResolveBinary("ctx-hook", root)
	if err != nil {
		t.Fatalf("failed to resolve repo-local binary: %v", err)
	}
	if !strings.HasSuffix(resolved, filepath.Join("bin", "ctx-hook")) {
		t.Fatalf("expected repo-local binary, got %s", resolved)
	}

	// Test explicit CONTEXTOS_BIN override
	customDir := t.TempDir()
	customHook := filepath.Join(customDir, "ctx-hook")
	if err := os.WriteFile(customHook, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONTEXTOS_BIN", customDir)
	overrideResolved, err := ResolveBinary("ctx-hook", root)
	if err != nil {
		t.Fatalf("failed to resolve CONTEXTOS_BIN binary: %v", err)
	}
	if overrideResolved != customHook {
		t.Fatalf("expected %s, got %s", customHook, overrideResolved)
	}

	// Test missing binary returns descriptive error
	emptyDir := t.TempDir()
	t.Setenv("CONTEXTOS_BIN", "")
	_, missingErr := ResolveBinary("non-existent-binary-xyz", emptyDir)
	if missingErr == nil {
		t.Fatalf("expected error for missing binary, got nil")
	}
}

func TestManagedConfigMarkers(t *testing.T) {
	root := t.TempDir()
	if _, err := Install("cursor", root, "/bin/sh", "/bin/sh"); err != nil {
		t.Fatal(err)
	}

	// Check .cursor/mcp.json has _managedBy and _version
	var mcp map[string]any
	b, err := os.ReadFile(filepath.Join(root, ".cursor", "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &mcp); err != nil {
		t.Fatal(err)
	}
	if mcp["_managedBy"] != ManagedBy {
		t.Fatalf("expected _managedBy=%q, got %v", ManagedBy, mcp["_managedBy"])
	}
	if mcp["_version"] != ConfigVersion {
		t.Fatalf("expected _version=%q, got %v", ConfigVersion, mcp["_version"])
	}
}
