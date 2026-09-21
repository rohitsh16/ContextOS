package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

	// 1. Test modern server/discover (Step 1)
	discoverReq := `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{}}` + "\n"
	// 2. Test legacy initialize (Step 3)
	initReq := `{"jsonrpc":"2.0","id":2,"method":"initialize","params":{}}` + "\n"
	// 3. Test deterministic tools/list (Step 4)
	listToolsReq1 := `{"jsonrpc":"2.0","id":3,"method":"tools/list","params":{}}` + "\n"
	listToolsReq2 := `{"jsonrpc":"2.0","id":4,"method":"tools/list","params":{}}` + "\n"
	// 4. Test tools/list pagination (Step 5)
	paginatedToolsReq := `{"jsonrpc":"2.0","id":5,"method":"tools/list","params":{"limit":3}}` + "\n"
	// 5. Test resources/list (Step 4)
	listResReq := `{"jsonrpc":"2.0","id":6,"method":"resources/list","params":{}}` + "\n"
	// 6. Test context_work_start tool
	startWorkReq := `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"context_work_start","arguments":{"title":"MCP Test WorkItem"}}}` + "\n"
	// 7. Test resources/read
	readResReq := `{"jsonrpc":"2.0","id":8,"method":"resources/read","params":{"uri":"context://repo/current"}}` + "\n"
	// 8. Test notification (must not elicit response)
	notifyReq := `{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}` + "\n"
	// 9. Test unknown method
	unknownReq := `{"jsonrpc":"2.0","id":9,"method":"unknown/method","params":{}}` + "\n"

	input := strings.NewReader(discoverReq + initReq + listToolsReq1 + listToolsReq2 + paginatedToolsReq + listResReq + startWorkReq + readResReq + notifyReq + unknownReq)
	var output bytes.Buffer

	if err := m.Run(input, &output); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	// Expected 9 responses (notification must be ignored)
	if len(lines) != 9 {
		t.Fatalf("expected 9 JSON-RPC responses, got %d. Output: %s", len(lines), output.String())
	}

	// 1. Verify server/discover
	var r1 Response
	if err := json.Unmarshal([]byte(lines[0]), &r1); err != nil || r1.Error != nil {
		t.Fatalf("server/discover failed: %v", lines[0])
	}
	resMap, ok := r1.Result.(map[string]any)
	if !ok || resMap["serverInfo"] == nil {
		t.Errorf("server/discover result missing serverInfo")
	}

	// 2. Verify legacy initialize
	var r2 Response
	if err := json.Unmarshal([]byte(lines[1]), &r2); err != nil || r2.Error != nil {
		t.Fatalf("initialize failed: %v", lines[1])
	}

	// 3. Verify deterministic tools/list: listTools_1 == listTools_2
	var r3, r4 Response
	_ = json.Unmarshal([]byte(lines[2]), &r3)
	_ = json.Unmarshal([]byte(lines[3]), &r4)
	if r3.Error != nil || r4.Error != nil {
		t.Fatalf("tools/list returned error: %v", lines[2])
	}
	tools1Bytes, _ := json.Marshal(r3.Result)
	tools2Bytes, _ := json.Marshal(r4.Result)
	if string(tools1Bytes) != string(tools2Bytes) {
		t.Errorf("tools/list must be deterministic: %s != %s", string(tools1Bytes), string(tools2Bytes))
	}

	// 4. Verify pagination
	var r5 Response
	_ = json.Unmarshal([]byte(lines[4]), &r5)
	r5Map := r5.Result.(map[string]any)
	paginatedList := r5Map["tools"].([]any)
	if len(paginatedList) != 3 {
		t.Errorf("expected 3 paginated tools, got %d", len(paginatedList))
	}
	if r5Map["nextCursor"] != "3" {
		t.Errorf("expected nextCursor='3', got %v", r5Map["nextCursor"])
	}

	// 5. Verify resources/list
	var r6 Response
	_ = json.Unmarshal([]byte(lines[5]), &r6)
	if r6.Error != nil {
		t.Fatalf("resources/list failed: %v", lines[5])
	}

	// 6. Verify work start
	var r7 Response
	_ = json.Unmarshal([]byte(lines[6]), &r7)
	if r7.Error != nil {
		t.Fatalf("context_work_start failed: %v", lines[6])
	}

	// 7. Verify resources/read
	var r8 Response
	_ = json.Unmarshal([]byte(lines[7]), &r8)
	if r8.Error != nil {
		t.Fatalf("resources/read failed: %v", lines[7])
	}

	// 9. Verify unknown method error
	var r9 Response
	_ = json.Unmarshal([]byte(lines[8]), &r9)
	if r9.Error == nil || r9.Error.Code != -32601 {
		t.Errorf("expected -32601 Method not found error, got %v", r9.Error)
	}
}

func TestMCPServerTimeoutAndAdaptiveContext(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "timeout_test.db")

	runGit(root, "init", "-q")
	runGit(root, "config", "user.email", "test@example.com")
	runGit(root, "config", "user.name", "Test")
	os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc Run() {}\nfunc Process() {}\n"), 0600)
	runGit(root, "add", ".")
	runGit(root, "commit", "-qm", "init")

	srv, err := server.NewWithOptions(dbPath, root, server.Options{
		Timeout:         500 * time.Millisecond,
		DefaultBudget:   2000,
		MinBudget:       500,
		AdaptiveTimeout: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	m := New(srv)

	// Call context_plan with custom timeout_ms and adaptive_budget
	planReq := `{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"context_plan","arguments":{"task":"Run Process","budget":1500,"timeout_ms":300,"adaptive_budget":true}}}` + "\n"
	searchReq := `{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"context_search","arguments":{"task":"Run","limit":5,"timeout_ms":200}}}` + "\n"

	input := strings.NewReader(planReq + searchReq)
	var output bytes.Buffer

	if err := m.Run(input, &output); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(lines))
	}

	var r10 Response
	if err := json.Unmarshal([]byte(lines[0]), &r10); err != nil {
		t.Fatalf("failed to unmarshal plan response: %v", err)
	}
	if r10.Error != nil {
		t.Fatalf("context_plan failed: %v", r10.Error)
	}

	var r11 Response
	if err := json.Unmarshal([]byte(lines[1]), &r11); err != nil {
		t.Fatalf("failed to unmarshal search response: %v", err)
	}
	if r11.Error != nil {
		t.Fatalf("context_search failed: %v", r11.Error)
	}
}

func runGit(dir string, args ...string) error {
	c := exec.Command("git", args...)
	c.Dir = dir
	return c.Run()
}
