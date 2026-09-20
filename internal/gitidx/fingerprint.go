package gitidx

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SymbolFingerprint tracks symbol identity and contents.
type SymbolFingerprint struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Signature string `json:"signature"`
	Start     int    `json:"start"`
	End       int    `json:"end"`
	Hash      string `json:"hash"`
}

// FileFingerprint tracks file content and child symbols.
type FileFingerprint struct {
	Path    string                       `json:"path"`
	Hash    string                       `json:"hash"`
	Symbols map[string]SymbolFingerprint `json:"symbols,omitempty"`
}

// DirFingerprint tracks directory hash aggregated from files and child dirs.
type DirFingerprint struct {
	Path        string `json:"path"`
	Hash        string `json:"hash"`
	FilesHash   string `json:"files_hash"`
	SubdirsHash string `json:"subdirs_hash"`
}

// FingerprintTree represents the entire hierarchical fingerprint of a repository.
type FingerprintTree struct {
	RepoHash  string                    `json:"repo_hash"`
	Dirs      map[string]DirFingerprint `json:"dirs"`
	Files     map[string]FileFingerprint `json:"files"`
	Timestamp int64                     `json:"timestamp"`
}

// InvalidationDelta reports exactly what changed between two fingerprint trees.
type InvalidationDelta struct {
	RepoChanged    bool     `json:"repo_changed"`
	ChangedDirs    []string `json:"changed_dirs"`
	ChangedFiles   []string `json:"changed_files"`
	DeletedFiles   []string `json:"deleted_files"`
	ChangedSymbols []string `json:"changed_symbols"`
}

// BuildHierarchicalFingerprints scans repo and constructs the full hierarchical fingerprint tree.
func BuildHierarchicalFingerprints(repoRoot string) (*FingerprintTree, error) {
	cleanRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, err
	}

	tree := &FingerprintTree{
		Dirs:  make(map[string]DirFingerprint),
		Files: make(map[string]FileFingerprint),
	}

	dirFiles := make(map[string][]string) // dir -> file hashes
	dirSubs := make(map[string][]string)  // dir -> subdir names

	err = filepath.WalkDir(cleanRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		rel, err := filepath.Rel(cleanRoot, path)
		if err != nil || rel == "." {
			return nil
		}
		if isExcludedPath(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			parent := filepath.Dir(rel)
			if parent != "." {
				dirSubs[parent] = append(dirSubs[parent], filepath.Base(rel))
			}
			return nil
		}

		// File
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		h := sha256.Sum256(b)
		fileHash := hex.EncodeToString(h[:])

		// Parse symbols
		symMap := make(map[string]SymbolFingerprint)
		symbols, _ := ParseSymbolsForContent(rel, string(b))
		for _, s := range symbols {
			symMap[s.Name] = SymbolFingerprint{
				Name:      s.Name,
				Kind:      s.Kind,
				Signature: s.Signature,
				Start:     s.Start,
				End:       s.End,
				Hash:      s.Hash,
			}
		}

		tree.Files[rel] = FileFingerprint{
			Path:    rel,
			Hash:    fileHash,
			Symbols: symMap,
		}

		dir := filepath.Dir(rel)
		dirFiles[dir] = append(dirFiles[dir], rel+":"+fileHash)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Compute directory fingerprints bottom-up
	var allDirs []string
	for d := range dirFiles {
		allDirs = append(allDirs, d)
	}
	for d := range dirSubs {
		allDirs = append(allDirs, d)
	}
	sort.Slice(allDirs, func(i, j int) bool {
		return len(strings.Split(allDirs[i], "/")) > len(strings.Split(allDirs[j], "/"))
	})

	for _, dir := range allDirs {
		fHashes := dirFiles[dir]
		sort.Strings(fHashes)
		hf := sha256.Sum256([]byte(strings.Join(fHashes, "|")))

		sSubs := dirSubs[dir]
		sort.Strings(sSubs)
		hs := sha256.Sum256([]byte(strings.Join(sSubs, "|")))

		comb := fmt.Sprintf("%x:%x", hf[:], hs[:])
		hd := sha256.Sum256([]byte(comb))

		tree.Dirs[dir] = DirFingerprint{
			Path:        dir,
			Hash:        hex.EncodeToString(hd[:]),
			FilesHash:   hex.EncodeToString(hf[:]),
			SubdirsHash: hex.EncodeToString(hs[:]),
		}
	}

	// Repo Hash
	var dirHashes []string
	for _, d := range tree.Dirs {
		dirHashes = append(dirHashes, d.Path+":"+d.Hash)
	}
	sort.Strings(dirHashes)
	hr := sha256.Sum256([]byte(strings.Join(dirHashes, "\n")))
	tree.RepoHash = hex.EncodeToString(hr[:])

	return tree, nil
}

// DiffFingerprints computes hierarchical invalidation delta between old and new trees.
func DiffFingerprints(oldTree, newTree *FingerprintTree) InvalidationDelta {
	delta := InvalidationDelta{}
	if oldTree == nil || newTree == nil {
		delta.RepoChanged = true
		return delta
	}
	if oldTree.RepoHash == newTree.RepoHash {
		return delta // zero change
	}
	delta.RepoChanged = true

	// Check changed files
	for path, newFile := range newTree.Files {
		oldFile, exists := oldTree.Files[path]
		if !exists || oldFile.Hash != newFile.Hash {
			delta.ChangedFiles = append(delta.ChangedFiles, path)
			// Check symbols
			for sName, newSym := range newFile.Symbols {
				oldSym, symExists := oldFile.Symbols[sName]
				if !symExists || oldSym.Hash != newSym.Hash {
					delta.ChangedSymbols = append(delta.ChangedSymbols, path+":"+sName)
				}
			}
		}
	}

	// Check deleted files
	for path := range oldTree.Files {
		if _, exists := newTree.Files[path]; !exists {
			delta.DeletedFiles = append(delta.DeletedFiles, path)
		}
	}

	// Check changed dirs
	for path, newDir := range newTree.Dirs {
		oldDir, exists := oldTree.Dirs[path]
		if !exists || oldDir.Hash != newDir.Hash {
			delta.ChangedDirs = append(delta.ChangedDirs, path)
		}
	}

	sort.Strings(delta.ChangedFiles)
	sort.Strings(delta.DeletedFiles)
	sort.Strings(delta.ChangedDirs)
	sort.Strings(delta.ChangedSymbols)

	return delta
}
