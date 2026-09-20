package indexer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"contextos/internal/gitidx"
)

// FileManifestEntry tracks file and symbol hashes for incremental invalidation.
type FileManifestEntry struct {
	Path            string `json:"path"`
	ContentHash     string `json:"content_hash"`
	Size            int64  `json:"size"`
	MTime           int64  `json:"mtime"` // unix nanoseconds
	Language        string `json:"language"`
	SymbolHash      string `json:"symbol_hash"`
	DependencyHash  string `json:"dependency_hash"`
	IndexedRevision string `json:"indexed_revision"`
}

// Manifest represents the persistent repository index manifest.
type Manifest struct {
	Version  string                       `json:"version"`
	Revision string                       `json:"revision"`
	Files    map[string]FileManifestEntry `json:"files"`
}

func NewManifest(revision string) *Manifest {
	return &Manifest{
		Version:  "1.0",
		Revision: revision,
		Files:    make(map[string]FileManifestEntry),
	}
}

// ComputeSymbolHash calculates H_symbol = H(symbol_1 || ... || symbol_n)
// allowing the indexer to distinguish formatting/comment edits from API changes.
func ComputeSymbolHash(syms []gitidx.Symbol) string {
	if len(syms) == 0 {
		return "empty"
	}
	// Sort symbols for deterministic hashing
	sorted := make([]gitidx.Symbol, len(syms))
	copy(sorted, syms)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Path != sorted[j].Path {
			return sorted[i].Path < sorted[j].Path
		}
		if sorted[i].Start != sorted[j].Start {
			return sorted[i].Start < sorted[j].Start
		}
		return sorted[i].Name < sorted[j].Name
	})

	h := sha256.New()
	for _, s := range sorted {
		fmt.Fprintf(h, "%s\x00%s\x00%s\x00%d\x00%d\x00", s.Kind, s.Name, s.Signature, s.Start, s.End)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ComputeDependencyHash hashes the sorted list of target files this file depends on.
func ComputeDependencyHash(deps []string) string {
	if len(deps) == 0 {
		return "none"
	}
	sorted := make([]string, len(deps))
	copy(sorted, deps)
	sort.Strings(sorted)

	h := sha256.New()
	for _, d := range sorted {
		h.Write([]byte(d))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func LoadManifest(manifestPath string) (*Manifest, error) {
	b, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if m.Files == nil {
		m.Files = make(map[string]FileManifestEntry)
	}
	return &m, nil
}

func SaveManifest(manifestPath string, m *Manifest) error {
	if m == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(manifestPath, b, 0600)
}

func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js", ".jsx":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".hpp":
		return "cpp"
	default:
		if ext != "" {
			return ext[1:]
		}
		return "unknown"
	}
}
