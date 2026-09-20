package indexer

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"

	"contextos/internal/gitidx"
)

// ChangeSet summarizes changes between filesystem state and the stored index manifest.
type ChangeSet struct {
	Added              []string `json:"added"`
	Deleted            []string `json:"deleted"`
	Modified           []string `json:"modified"`
	Unchanged          []string `json:"unchanged"`
	DependencyAffected []string `json:"dependency_affected"`
}

func (cs ChangeSet) HasChanges() bool {
	return len(cs.Added) > 0 || len(cs.Deleted) > 0 || len(cs.Modified) > 0
}

// DetectChanges scans the repo and compares against manifest using a multi-tier approach:
// Tier 1: Stat-based size/mtime check.
// Tier 2: Content hashing only when stat changes.
// Tier 3: Transitive closure on reverse dependency graph for affected files.
func DetectChanges(root string, m *Manifest, reverseDeps map[string][]string) (ChangeSet, error) {
	var cs ChangeSet
	currentFiles := make(map[string]os.FileInfo)

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if gitidx.DefaultExclusions[name] {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Size() > 2*1024*1024 || !gitidx.Supported(path) {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		currentFiles[relSlash] = info
		return nil
	})
	if err != nil {
		return cs, err
	}

	if m == nil || m.Files == nil {
		// No previous manifest: all current files are Added
		for path := range currentFiles {
			cs.Added = append(cs.Added, path)
		}
		return cs, nil
	}

	// 1. Detect Deleted files
	for path := range m.Files {
		if _, exists := currentFiles[path]; !exists {
			cs.Deleted = append(cs.Deleted, path)
		}
	}

	// 2. Detect Added, Modified, and Unchanged files
	for path, info := range currentFiles {
		entry, exists := m.Files[path]
		if !exists {
			cs.Added = append(cs.Added, path)
			continue
		}

		// Tier 1: Quick stat check (size and mtime)
		if info.Size() == entry.Size && info.ModTime().UnixNano() == entry.MTime {
			cs.Unchanged = append(cs.Unchanged, path)
			continue
		}

		// Tier 2: Content hash comparison
		fullPath := filepath.Join(root, filepath.FromSlash(path))
		data, rerr := os.ReadFile(fullPath)
		if rerr != nil {
			cs.Modified = append(cs.Modified, path)
			continue
		}
		h := sha256.Sum256(data)
		ch := hex.EncodeToString(h[:])

		if ch == entry.ContentHash {
			// Content is identical; touch update only
			entry.MTime = info.ModTime().UnixNano()
			entry.Size = info.Size()
			m.Files[path] = entry
			cs.Unchanged = append(cs.Unchanged, path)
		} else {
			cs.Modified = append(cs.Modified, path)
		}
	}

	// Tier 3: Compute reverse dependency closure
	if reverseDeps != nil && (len(cs.Modified) > 0 || len(cs.Deleted) > 0) {
		visited := make(map[string]bool)
		for _, p := range cs.Modified {
			visited[p] = true
		}
		for _, p := range cs.Added {
			visited[p] = true
		}
		for _, p := range cs.Deleted {
			visited[p] = true
		}

		var queue []string
		queue = append(queue, cs.Modified...)
		queue = append(queue, cs.Deleted...)

		for len(queue) > 0 {
			curr := queue[0]
			queue = queue[1:]
			for _, dependent := range reverseDeps[curr] {
				if !visited[dependent] {
					visited[dependent] = true
					cs.DependencyAffected = append(cs.DependencyAffected, dependent)
					queue = append(queue, dependent)
				}
			}
		}
	}

	return cs, nil
}
