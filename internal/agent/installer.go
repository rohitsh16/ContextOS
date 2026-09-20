package agent

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// SetupOptions specifies configuration for transactional agent setup.
type SetupOptions struct {
	RepoRoot   string
	RepoName   string
	HookBinary string
	MCPBinary  string
	HomeDir    string
	DryRun     bool
	AgentID    string // specific agent ID or empty for all detected
	Force      bool
}

// SetupResult contains the outcome of a transactional setup operation.
type SetupResult struct {
	Success          bool                        `json:"success"`
	DryRun           bool                        `json:"dry_run"`
	Plans            []InstallPlan               `json:"plans"`
	Installed        []string                    `json:"installed"`
	Validation       map[string]ValidationResult `json:"validation"`
	BackupDir        string                      `json:"backup_dir,omitempty"`
	RollbackOccurred bool                        `json:"rollback_occurred"`
	Error            string                      `json:"error,omitempty"`
}

// TransactionalInstaller manages atomic, reversible integration setup.
type TransactionalInstaller struct {
	adapters []AgentAdapter
}

// NewTransactionalInstaller returns an installer configured with registered adapters.
func NewTransactionalInstaller(adapters ...AgentAdapter) *TransactionalInstaller {
	if len(adapters) == 0 {
		adapters = List()
	}
	return &TransactionalInstaller{adapters: adapters}
}

// Execute performs the transactional plan -> snapshot -> apply -> validate -> commit/rollback lifecycle.
func (inst *TransactionalInstaller) Execute(opts SetupOptions) SetupResult {
	res := SetupResult{
		DryRun:     opts.DryRun,
		Validation: make(map[string]ValidationResult),
	}

	detectCtx := DetectContext{
		RepoRoot: opts.RepoRoot,
		HomeDir:  opts.HomeDir,
	}

	installCtx := InstallContext{
		RepoRoot:   opts.RepoRoot,
		RepoName:   opts.RepoName,
		HookBinary: opts.HookBinary,
		MCPBinary:  opts.MCPBinary,
		HomeDir:    opts.HomeDir,
		DryRun:     opts.DryRun,
	}

	// 1. Determine target adapters
	var targets []AgentAdapter
	for _, a := range inst.adapters {
		if opts.AgentID != "" {
			if a.ID() == opts.AgentID {
				targets = append(targets, a)
			}
			continue
		}
		// If no specific agent specified, install for detected agents or all if Force is true
		if opts.Force {
			targets = append(targets, a)
		} else {
			det := a.Detect(detectCtx)
			if det.Installed {
				targets = append(targets, a)
			}
		}
	}

	// Fallback if none detected and no specific agent requested: install all
	if len(targets) == 0 && opts.AgentID == "" {
		targets = inst.adapters
	}

	// 2. Plan phase
	affectedFilesMap := make(map[string]bool)
	for _, a := range targets {
		plan, err := a.Plan(installCtx)
		if err != nil {
			res.Error = fmt.Sprintf("plan failed for adapter %s: %v", a.ID(), err)
			return res
		}
		res.Plans = append(res.Plans, plan)
		for _, f := range plan.AffectedFiles {
			affectedFilesMap[f] = true
		}
	}

	if opts.DryRun {
		res.Success = true
		return res
	}

	// 3. Snapshot phase
	home := opts.HomeDir
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	backupDir := filepath.Join(home, ".contextos", "backups", fmt.Sprintf("setup-%d", time.Now().UnixNano()))
	res.BackupDir = backupDir

	snapshotEntries := make(map[string]string) // origPath -> backupPath
	for f := range affectedFilesMap {
		if _, err := os.Stat(f); err == nil {
			rel, _ := filepath.Rel(opts.RepoRoot, f)
			dst := filepath.Join(backupDir, rel)
			if err := copyFile(f, dst); err != nil {
				res.Error = fmt.Sprintf("snapshot backup failed for %s: %v", f, err)
				return res
			}
			snapshotEntries[f] = dst
		}
	}

	// 4. Apply phase
	for i, a := range targets {
		plan := res.Plans[i]
		if err := a.Apply(installCtx, plan); err != nil {
			res.Error = fmt.Sprintf("apply failed for adapter %s: %v", a.ID(), err)
			inst.rollback(snapshotEntries, affectedFilesMap)
			res.RollbackOccurred = true
			return res
		}
		res.Installed = append(res.Installed, a.ID())
	}

	// 5. Validate phase
	valCtx := ValidateContext{
		RepoRoot: opts.RepoRoot,
		HomeDir:  opts.HomeDir,
	}

	allValid := true
	for _, a := range targets {
		v := a.Validate(valCtx)
		res.Validation[a.ID()] = v
		if !v.Valid {
			allValid = false
			res.Error = fmt.Sprintf("validation failed for adapter %s: %v", a.ID(), v.Issues)
			break
		}
	}

	// 6. Rollback if validation failed
	if !allValid {
		inst.rollback(snapshotEntries, affectedFilesMap)
		res.RollbackOccurred = true
		res.Success = false
		return res
	}

	res.Success = true
	return res
}

func (inst *TransactionalInstaller) rollback(snapshotEntries map[string]string, affectedFiles map[string]bool) {
	for f := range affectedFiles {
		backupPath, existed := snapshotEntries[f]
		if existed {
			_ = copyFile(backupPath, f)
		} else {
			_ = os.Remove(f)
		}
	}
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
