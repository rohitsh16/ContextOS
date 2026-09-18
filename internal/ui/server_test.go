package ui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"contextos/internal/server"
)

func setupTestRepo(t *testing.T) (string, *server.Service) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_ = runGit(root, "init", "-q")
	_ = runGit(root, "config", "user.email", "test@example.com")
	_ = runGit(root, "config", "user.name", "Test")
	_ = runGit(root, "add", ".")
	_ = runGit(root, "commit", "-qm", "init")

	dbPath := filepath.Join(root, "ctx.db")
	s, err := server.New(dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Index()
	return root, s
}

func runGit(dir string, args ...string) error {
	c := exec.Command("git", args...)
	c.Dir = dir
	return c.Run()
}

func TestServerStatusAndMemoriesEndpoints(t *testing.T) {
	root, s := setupTestRepo(t)
	defer s.Close()

	srv := NewServer(s, root, 8765)
	handler := srv.Handler()

	// 1. GET /api/status
	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// 2. POST /api/memories
	body := []byte(`{"kind":"decision","content":"Use Raft consensus for state replication","authority":"user","confidence":0.95}`)
	req = httptest.NewRequest("POST", "/api/memories", bytes.NewReader(body))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	// 3. GET /api/memories
	req = httptest.NewRequest("GET", "/api/memories", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	var mems []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &mems); err != nil {
		t.Fatal(err)
	}
	if len(mems) == 0 {
		t.Fatal("expected at least one memory")
	}

	// 4. POST /api/plan
	planBody := []byte(`{"task":"Raft consensus failover","budget":2000}`)
	req = httptest.NewRequest("POST", "/api/plan", bytes.NewReader(planBody))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	var planRes map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &planRes); err != nil {
		t.Fatal(err)
	}
	if _, ok := planRes["plan"]; !ok {
		t.Fatal("missing plan in response")
	}
}

func TestServerStaticIndex(t *testing.T) {
	root, s := setupTestRepo(t)
	defer s.Close()

	srv := NewServer(s, root, 8765)
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected index 200, got %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("ContextOS")) {
		t.Fatalf("expected index.html to contain ContextOS")
	}
}

func TestServerReportEndpoint(t *testing.T) {
	root, s := setupTestRepo(t)
	defer s.Close()

	srv := NewServer(s, root, 8765)
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/api/report", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected report 200, got %d: %s", w.Code, w.Body.String())
	}
	var res map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if _, ok := res["markdown"]; !ok {
		t.Fatal("expected markdown field in report response")
	}
}

