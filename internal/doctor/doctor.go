package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"contextos/internal/agent"
	"contextos/internal/integrations"
	"contextos/internal/server"
	"contextos/internal/version"
)

// CheckStatus represents the health of a single system check.
type CheckStatus string

const (
	StatusPass CheckStatus = "PASS"
	StatusWarn CheckStatus = "WARN"
	StatusFail CheckStatus = "FAIL"
)

// Check records the result of an individual diagnostic check.
type Check struct {
	Name        string      `json:"name"`
	Status      CheckStatus `json:"status"`
	Details     string      `json:"details,omitempty"`
	Remediation string      `json:"remediation,omitempty"`
}

// Category groups related diagnostic checks.
type Category struct {
	Title  string  `json:"title"`
	Checks []Check `json:"checks"`
}

// DoctorReport contains the full diagnostic output.
type DoctorReport struct {
	Healthy    bool       `json:"healthy"`
	Version    string     `json:"version"`
	Timestamp  string     `json:"timestamp"`
	RepoRoot   string     `json:"repo_root"`
	Categories []Category `json:"categories"`
}

// RunDiagnostics executes system-wide health checks across binaries, databases, and agent adapters.
func RunDiagnostics(repoRoot, dbPath string) DoctorReport {
	if repoRoot == "" {
		repoRoot, _ = os.Getwd()
	}
	repoRoot, _ = filepath.Abs(repoRoot)

	rep := DoctorReport{
		Healthy:   true,
		Version:   version.Info(),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		RepoRoot:  repoRoot,
	}

	// 1. Core Binaries
	var coreChecks []Check
	for _, binName := range []string{"ctx", "contextd", "ctx-hook"} {
		p, err := integrations.ResolveBinary(binName, repoRoot)
		if err != nil {
			// Fallback: check current executable dir
			if ex, e := os.Executable(); e == nil {
				cand := filepath.Join(filepath.Dir(ex), binName)
				if _, statErr := os.Stat(cand); statErr == nil {
					p = cand
					err = nil
				}
			}
		}

		if err != nil {
			coreChecks = append(coreChecks, Check{
				Name:        "binary: " + binName,
				Status:      StatusWarn,
				Details:     fmt.Sprintf("not found in PATH or local ./bin/ (%v)", err),
				Remediation: fmt.Sprintf("run 'make build' or 'go build -o bin/%s ./cmd/%s'", binName, binName),
			})
		} else {
			coreChecks = append(coreChecks, Check{
				Name:    "binary: " + binName,
				Status:  StatusPass,
				Details: p,
			})
		}
	}
	rep.Categories = append(rep.Categories, Category{
		Title:  "Core Executables",
		Checks: coreChecks,
	})

	// 2. Storage & Database
	var dbChecks []Check
	if dbPath == "" {
		dbPath = server.DefaultDBPath()
	}
	dbChecks = append(dbChecks, Check{
		Name:    "storage path",
		Status:  StatusPass,
		Details: dbPath,
	})

	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		dbChecks = append(dbChecks, Check{
			Name:        "storage directory",
			Status:      StatusFail,
			Details:     fmt.Sprintf("cannot create/access directory: %v", err),
			Remediation: "check filesystem permissions for " + dbDir,
		})
		rep.Healthy = false
	} else {
		dbChecks = append(dbChecks, Check{
			Name:    "storage directory",
			Status:  StatusPass,
			Details: dbDir,
		})
	}

	rep.Categories = append(rep.Categories, Category{
		Title:  "Storage & Database",
		Checks: dbChecks,
	})

	// 3. Git Repository Health
	var gitChecks []Check
	gitDir := filepath.Join(repoRoot, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		gitChecks = append(gitChecks, Check{
			Name:        "git repository",
			Status:      StatusWarn,
			Details:     "not a git repository root",
			Remediation: "initialize with 'git init' or run ctx in repository root",
		})
	} else {
		gitChecks = append(gitChecks, Check{
			Name:    "git repository",
			Status:  StatusPass,
			Details: repoRoot,
		})
		// Check branch
		cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
		cmd.Dir = repoRoot
		if out, err := cmd.Output(); err == nil {
			branch := strings.TrimSpace(string(out))
			gitChecks = append(gitChecks, Check{
				Name:    "git branch",
				Status:  StatusPass,
				Details: branch,
			})
		}
	}
	rep.Categories = append(rep.Categories, Category{
		Title:  "Repository State",
		Checks: gitChecks,
	})

	// 4. Agent Adapters
	valCtx := agent.ValidateContext{
		RepoRoot: repoRoot,
	}
	detCtx := agent.DetectContext{
		RepoRoot: repoRoot,
	}

	var adapterChecks []Check
	for _, a := range agent.List() {
		det := a.Detect(detCtx)
		val := a.Validate(valCtx)

		var status CheckStatus
		var details string
		var remed string

		if !det.Installed && !val.Valid {
			status = StatusPass
			details = "not configured (optional)"
		} else if val.Valid {
			status = StatusPass
			details = fmt.Sprintf("configured and valid (%d files verified)", len(val.CheckedFiles))
		} else {
			status = StatusWarn
			details = strings.Join(val.Issues, "; ")
			remed = fmt.Sprintf("run 'ctx setup --agent %s' to repair configuration", a.ID())
		}

		adapterChecks = append(adapterChecks, Check{
			Name:        a.Name(),
			Status:      status,
			Details:     details,
			Remediation: remed,
		})
	}
	rep.Categories = append(rep.Categories, Category{
		Title:  "Agent Integrations",
		Checks: adapterChecks,
	})

	return rep
}

// Format returns an ANSI-styled human-readable diagnostic report.
func Format(rep DoctorReport) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== ContextOS Doctor (v%s) ===\n", version.ApplicationVersion))
	sb.WriteString(fmt.Sprintf("Repository: %s\n", rep.RepoRoot))
	sb.WriteString(fmt.Sprintf("Timestamp:  %s\n\n", rep.Timestamp))

	for _, cat := range rep.Categories {
		sb.WriteString(fmt.Sprintf("%s\n", cat.Title))
		for _, c := range cat.Checks {
			icon := "✓"
			if c.Status == StatusWarn {
				icon = "!"
			} else if c.Status == StatusFail {
				icon = "✗"
			}
			sb.WriteString(fmt.Sprintf("  [%s] %-20s : %s\n", icon, c.Name, c.Details))
			if c.Remediation != "" {
				sb.WriteString(fmt.Sprintf("      -> Remediation: %s\n", c.Remediation))
			}
		}
		sb.WriteString("\n")
	}

	overall := "HEALTHY"
	if !rep.Healthy {
		overall = "UNHEALTHY (issues require remediation)"
	}
	sb.WriteString(fmt.Sprintf("Overall Status: %s\n", overall))
	return sb.String()
}

// JSON returns a formatted JSON string of the report.
func JSON(rep DoctorReport) string {
	b, _ := json.MarshalIndent(rep, "", "  ")
	return string(b)
}
