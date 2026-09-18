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
	for _, a := range []string{"claude", "cursor", "codex", "gemini"} {
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
		filepath.Join(root, ".codex", "config.toml"),
		filepath.Join(root, ".gemini", "settings.json"),
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
	toml, _ := os.ReadFile(filepath.Join(root, ".codex", "config.toml"))
	if strings.Count(string(toml), "[mcp_servers.contextos]") != 1 {
		t.Fatal("Codex MCP stanza not idempotent")
	}
}
