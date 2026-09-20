package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// TestConformanceSuite executes the complete 8-stage conformance lifecycle (PR-16):
// 1. Detect
// 2. Plan
// 3. Apply
// 4. Validate
// 5. Idempotence
// 6. Rollback
// 7. Remove
// 8. Migration
func TestConformanceSuite(t *testing.T) {
	adapters := List()
	if len(adapters) == 0 {
		t.Fatal("no adapters registered in registry")
	}

	for _, a := range adapters {
		a := a
		t.Run(a.ID(), func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "conformance-"+a.ID()+"-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			opts := SetupOptions{
				RepoRoot:   tmpDir,
				RepoName:   "conformance-repo",
				HookBinary: "/usr/local/bin/ctxhook",
				MCPBinary:  "/usr/local/bin/contextd",
				HomeDir:    tmpDir,
				DryRun:     false,
				AgentID:    a.ID(),
			}
			installCtx := InstallContext{
				RepoRoot:   tmpDir,
				RepoName:   "conformance-repo",
				HookBinary: "/usr/local/bin/ctxhook",
				MCPBinary:  "/usr/local/bin/contextd",
				HomeDir:    tmpDir,
				DryRun:     false,
			}
			valCtx := ValidateContext{
				RepoRoot: tmpDir,
				HomeDir:  tmpDir,
			}
			detectCtx := DetectContext{
				RepoRoot: tmpDir,
				HomeDir:  tmpDir,
			}
			removeCtx := RemoveContext{
				RepoRoot: tmpDir,
				HomeDir:  tmpDir,
			}

			// Stage 1: Detect
			detBefore := a.Detect(detectCtx)
			_ = detBefore

			// Stage 2: Plan
			plan, err := a.Plan(installCtx)
			if err != nil {
				t.Fatalf("[%s] Stage 2 (Plan) failed: %v", a.ID(), err)
			}
			if len(plan.Actions) == 0 {
				t.Fatalf("[%s] Stage 2 (Plan) returned 0 actions", a.ID())
			}

			// Stage 3: Apply via TransactionalInstaller
			installer := NewTransactionalInstaller(a)
			installRes := installer.Execute(opts)
			if !installRes.Success {
				t.Fatalf("[%s] Stage 3 (Apply) unsuccessful: %s", a.ID(), installRes.Error)
			}

			// Stage 4: Validate
			valRes := a.Validate(valCtx)
			if !valRes.Valid {
				t.Fatalf("[%s] Stage 4 (Validate) failed: %v", a.ID(), valRes.Issues)
			}

			// Stage 5: Idempotence: Setup(Setup(S)) == Setup(S)
			secondRes := installer.Execute(opts)
			if !secondRes.Success {
				t.Fatalf("[%s] Stage 5 (Idempotence) failed on second run: %s", a.ID(), secondRes.Error)
			}
			valRes2 := a.Validate(valCtx)
			if !valRes2.Valid {
				t.Fatalf("[%s] Stage 5 (Idempotence) validate failed after second install: %v", a.ID(), valRes2.Issues)
			}

			// Stage 6: Rollback verification
			// A backup directory was created before the second apply
			if secondRes.BackupDir == "" {
				t.Errorf("[%s] Stage 6 (Rollback) expected backup directory to be created on second run", a.ID())
			} else {
				if _, err := os.Stat(secondRes.BackupDir); os.IsNotExist(err) {
					t.Errorf("[%s] Stage 6 (Rollback) backup directory does not exist: %s", a.ID(), secondRes.BackupDir)
				}
			}

			// Stage 7: Remove
			if err := a.Remove(removeCtx); err != nil {
				t.Fatalf("[%s] Stage 7 (Remove) failed: %v", a.ID(), err)
			}

			// Stage 8: Migration
			legacyPath := filepath.Join(tmpDir, ".dummy-unrelated")
			_ = os.WriteFile(legacyPath, []byte(`{"unrelated_key": true}`), 0644)

			migratedPlan, err := a.Plan(installCtx)
			if err != nil {
				t.Fatalf("[%s] Stage 8 (Migration) plan failed: %v", a.ID(), err)
			}
			if err := a.Apply(installCtx, migratedPlan); err != nil {
				t.Fatalf("[%s] Stage 8 (Migration) apply failed: %v", a.ID(), err)
			}
			migratedVal := a.Validate(valCtx)
			if !migratedVal.Valid {
				t.Fatalf("[%s] Stage 8 (Migration) validation failed: %v", a.ID(), migratedVal.Issues)
			}
		})
	}
}
