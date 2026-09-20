package doctor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDiagnostics(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "doctor-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "context.db")

	rep := RunDiagnostics(tmpDir, dbPath)
	if rep.RepoRoot != tmpDir {
		t.Errorf("expected RepoRoot %s, got %s", tmpDir, rep.RepoRoot)
	}
	if len(rep.Categories) != 4 {
		t.Errorf("expected 4 categories, got %d", len(rep.Categories))
	}

	formatted := Format(rep)
	if !strings.Contains(formatted, "ContextOS Doctor") {
		t.Errorf("formatted report missing title: %s", formatted)
	}
	if !strings.Contains(formatted, "Overall Status") {
		t.Errorf("formatted report missing overall status: %s", formatted)
	}

	jsonStr := JSON(rep)
	var decoded DoctorReport
	if err := json.Unmarshal([]byte(jsonStr), &decoded); err != nil {
		t.Fatalf("failed to decode JSON output: %v", err)
	}
	if decoded.RepoRoot != tmpDir {
		t.Errorf("decoded JSON RepoRoot mismatch: %s vs %s", decoded.RepoRoot, tmpDir)
	}
}
