package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"contextos/internal/server"
)

func TestMCPServerProtocols(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "mcp_test.db")

	runGit(root, "init", "-q")
	runGit(root, "config", "user.email", "test@example.com")
	runGit(root, "config", "user.name", "Test")
	os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc Run() {}\n"), 0600)
	runGit(root, "add", ".")
	runGit(root, "commit", "-qm", "init")

	srv, err := server.New(dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	m := New(srv)

	// 1. Test initialize
	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n"
	// 2. Test tools/list
	listToolsReq := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}` + "\n"
	// 3. Test context_work_start tool
	startWorkReq := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"context_work_start","arguments":{"title":"MCP Test WorkItem"}}}` + "\n"
	// 4. Test resources/read
	readResReq := `{"jsonrpc":"2.0","id":4,"method":"resources/read","params":{"uri":"context://repo/current"}}` + "\n"

	input := strings.NewReader(initReq + listToolsReq + startWorkReq + readResReq)
	var output bytes.Buffer

	if err := m.Run(input, &output); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 JSON-RPC responses, got %d. Output: %s", len(lines), output.String())
	}

	// Verify initialize response
	var r1 Response
	if err := json.Unmarshal([]byte(lines[0]), &r1); err != nil || r1.Error != nil {
		t.Fatalf("initialize failed: %v", lines[0])
	}

	// Verify tools list response
	var r2 Response
	if err := json.Unmarshal([]byte(lines[1]), &r2); err != nil || r2.Error != nil {
		t.Fatalf("tools/list failed: %v", lines[1])
	}

	// Verify work start
	var r3 Response
	if err := json.Unmarshal([]byte(lines[2]), &r3); err != nil || r3.Error != nil {
		t.Fatalf("context_work_start failed: %v", lines[2])
	}

	// Verify resources/read
	var r4 Response
	if err := json.Unmarshal([]byte(lines[3]), &r4); err != nil || r4.Error != nil {
		t.Fatalf("resources/read failed: %v", lines[3])
	}
}

func runGit(dir string, args ...string) error {
	c := exec.Command("git", args...)
	c.Dir = dir
	return c.Run()
}
