package integrations

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"contextos/internal/version"
)

const (
	// ManagedBy is embedded in generated configuration files to distinguish
	// ContextOS-managed entries from user-owned configuration.
	ManagedBy = "contextos"

	// ConfigVersion tracks the generation schema so the installer can
	// detect and upgrade stale managed configurations.
	ConfigVersion = version.ApplicationVersion
)

// ResolveBinary locates a ContextOS binary using a portable resolution
// strategy. The order is:
//
//  1. Explicit CONTEXTOS_BIN environment variable
//  2. PATH lookup
//  3. Repo-local ./bin/ directory
//  4. Well-defined installation path (~/.local/bin)
//
// This avoids embedding developer-specific absolute paths.
func ResolveBinary(name, repoRoot string) (string, error) {
	// 1. Explicit environment variable
	if envDir := os.Getenv("CONTEXTOS_BIN"); envDir != "" {
		candidate := filepath.Join(envDir, name)
		if isExecutable(candidate) {
			return candidate, nil
		}
	}

	// 2. PATH lookup
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}

	// 3. Repo-local ./bin/
	if repoRoot != "" {
		candidate := filepath.Join(repoRoot, "bin", name)
		if isExecutable(candidate) {
			abs, _ := filepath.Abs(candidate)
			return abs, nil
		}
	}

	// 4. Well-defined installation path
	home, err := os.UserHomeDir()
	if err == nil {
		var candidate string
		if runtime.GOOS == "windows" {
			candidate = filepath.Join(home, "AppData", "Local", "contextos", "bin", name)
		} else {
			candidate = filepath.Join(home, ".local", "bin", name)
		}
		if isExecutable(candidate) {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("binary %q not found in CONTEXTOS_BIN, PATH, %s/bin, or ~/.local/bin", name, repoRoot)
}

// isExecutable returns true if path exists and is executable (or exists on Windows).
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return !info.IsDir()
	}
	return !info.IsDir() && info.Mode()&0111 != 0
}

// managedMarker returns metadata fields that mark a JSON config as
// ContextOS-managed. This allows the installer to distinguish managed
// from user-owned configuration.
func managedMarker() map[string]any {
	return map[string]any{
		"_managedBy": ManagedBy,
		"_version":   ConfigVersion,
	}
}
