package indexer

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	"contextos/internal/gitidx"
	"contextos/internal/store"
)

type IndexStats struct {
	Full             bool    `json:"full"`
	FilesScanned     int     `json:"files_scanned"`
	FilesParsed      int     `json:"files_parsed"`
	SymbolsExtracted int     `json:"symbols_extracted"`
	EdgesUpdated     int     `json:"edges_updated"`
	AddedCount       int     `json:"added_count"`
	ModifiedCount    int     `json:"modified_count"`
	DeletedCount     int     `json:"deleted_count"`
	DurationMs       float64 `json:"duration_ms"`
	WorkReduction    float64 `json:"work_reduction"`
}

type Indexer struct {
	Store        store.Store
	RepoRoot     string
	RepoID       string
	ManifestPath string
}

func New(st store.Store, repoRoot, repoID string) *Indexer {
	return &Indexer{
		Store:        st,
		RepoRoot:     repoRoot,
		RepoID:       repoID,
		ManifestPath: filepath.Join(repoRoot, ".contextos", "manifest.json"),
	}
}

// Index performs full or incremental indexing depending on the full flag and manifest availability.
func (idx *Indexer) Index(full bool, revision string) (*IndexStats, error) {
	if full {
		return idx.IndexFull(revision)
	}
	return idx.IndexIncremental(revision)
}

// IndexFull rebuilds the entire repository index and writes a new manifest.
func (idx *Indexer) IndexFull(revision string) (*IndexStats, error) {
	start := time.Now()

	files, err := gitidx.ListSourceFiles(idx.RepoRoot)
	if err != nil {
		return nil, err
	}
	syms, err := gitidx.WalkSymbols(idx.RepoRoot)
	if err != nil {
		return nil, err
	}

	// Group symbols by file path
	symsByPath := make(map[string][]gitidx.Symbol)
	for _, s := range syms {
		symsByPath[s.Path] = append(symsByPath[s.Path], s)
	}

	manifest := NewManifest(revision)
	for _, f := range files {
		fullPath := filepath.Join(idx.RepoRoot, filepath.FromSlash(f.Path))
		info, _ := os.Stat(fullPath)
		var size, mtime int64
		if info != nil {
			size = info.Size()
			mtime = info.ModTime().UnixNano()
		}
		fileSyms := symsByPath[f.Path]
		manifest.Files[f.Path] = FileManifestEntry{
			Path:            f.Path,
			ContentHash:     f.Hash,
			Size:            size,
			MTime:           mtime,
			Language:        detectLanguage(f.Path),
			SymbolHash:      ComputeSymbolHash(fileSyms),
			IndexedRevision: revision,
		}
	}

	// Build edges from all files
	fileByBase := make(map[string]string)
	for _, f := range files {
		fileNodeID := storeHashID("file|" + f.Path)
		fileByBase[filepath.Base(f.Path)] = fileNodeID
		fileByBase[f.Path] = fileNodeID
	}

	var edges []store.EdgeRecord
	for _, f := range files {
		p := filepath.Join(idx.RepoRoot, filepath.FromSlash(f.Path))
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			continue
		}
		txt := string(b)
		srcID := storeHashID("file|" + f.Path)
		var deps []string
		for base, dst := range fileByBase {
			if base == filepath.Base(f.Path) || base == f.Path {
				continue
			}
			if strings.Contains(txt, base) {
				edgeKind := "references"
				if strings.HasSuffix(f.Path, "_test.go") || strings.Contains(f.Path, "test") {
					edgeKind = "test-reference"
				}
				edges = append(edges, store.EdgeRecord{SrcID: srcID, DstID: dst, Kind: edgeKind})
				deps = append(deps, base)
			}
		}
		entry := manifest.Files[f.Path]
		entry.DependencyHash = ComputeDependencyHash(deps)
		manifest.Files[f.Path] = entry
	}

	if err := idx.Store.SaveNodesAndEdges(idx.RepoID, files, syms, edges); err != nil {
		return nil, err
	}
	_ = SaveManifest(idx.ManifestPath, manifest)

	dur := float64(time.Since(start).Microseconds()) / 1000.0
	return &IndexStats{
		Full:             true,
		FilesScanned:     len(files),
		FilesParsed:      len(files),
		SymbolsExtracted: len(syms),
		EdgesUpdated:     len(edges),
		AddedCount:       len(files),
		DurationMs:       dur,
		WorkReduction:    0.0,
	}, nil
}

// IndexIncremental updates only the added, modified, deleted, and dependency-affected files.
// Invariant: IndexIncremental(R, Delta) == IndexFull(R + Delta).
func (idx *Indexer) IndexIncremental(revision string) (*IndexStats, error) {
	manifest, err := LoadManifest(idx.ManifestPath)
	if err != nil || manifest == nil || len(manifest.Files) == 0 {
		// No previous manifest; fall back to full index
		return idx.IndexFull(revision)
	}

	start := time.Now()

	// Load existing edges to build reverse dependency map
	existingEdges, _ := idx.Store.ListEdges(idx.RepoID)
	existingNodes, _ := idx.Store.ListNodes(idx.RepoID)

	nodeIDToPath := make(map[string]string)
	for _, n := range existingNodes {
		if n.Kind == "file" {
			nodeIDToPath[n.ID] = n.Path
		}
	}

	reverseDeps := make(map[string][]string) // targetPath -> []sourcePath
	for _, e := range existingEdges {
		srcPath := nodeIDToPath[e.SrcID]
		dstPath := nodeIDToPath[e.DstID]
		if srcPath != "" && dstPath != "" {
			reverseDeps[dstPath] = append(reverseDeps[dstPath], srcPath)
		}
	}

	// Detect changes using Tier 1 and Tier 2 filters
	cs, err := DetectChanges(idx.RepoRoot, manifest, reverseDeps)
	if err != nil {
		return nil, err
	}

	totalFiles := len(cs.Added) + len(cs.Modified) + len(cs.Unchanged)
	if !cs.HasChanges() {
		// Nothing changed; early exit
		manifest.Revision = revision
		_ = SaveManifest(idx.ManifestPath, manifest)
		return &IndexStats{
			Full:          false,
			FilesScanned:  totalFiles,
			FilesParsed:   0,
			DurationMs:    float64(time.Since(start).Microseconds()) / 1000.0,
			WorkReduction: 1.0,
		}, nil
	}

	// Parse symbols only for Added and Modified files (Tier 2)
	var newFiles []gitidx.SourceFile
	var newSyms []gitidx.Symbol

	for _, relPath := range append(cs.Added, cs.Modified...) {
		fullPath := filepath.Join(idx.RepoRoot, filepath.FromSlash(relPath))
		b, rerr := os.ReadFile(fullPath)
		if rerr != nil {
			continue
		}
		info, _ := os.Stat(fullPath)
		var size, mtime int64
		if info != nil {
			size = info.Size()
			mtime = info.ModTime().UnixNano()
		}

		h := sha256.Sum256(b)
		fileHash := hex.EncodeToString(h[:])
		lines := len(strings.Split(string(b), "\n"))

		newFiles = append(newFiles, gitidx.SourceFile{
			Path:  relPath,
			Hash:  fileHash,
			Lines: lines,
		})

		fileSyms, _ := gitidx.ParseSymbolsForContent(relPath, string(b))
		newSyms = append(newSyms, fileSyms...)

		manifest.Files[relPath] = FileManifestEntry{
			Path:            relPath,
			ContentHash:     fileHash,
			Size:            size,
			MTime:           mtime,
			Language:        detectLanguage(relPath),
			SymbolHash:      ComputeSymbolHash(fileSyms),
			IndexedRevision: revision,
		}
	}

	// Remove Deleted files from manifest
	for _, p := range cs.Deleted {
		delete(manifest.Files, p)
	}

	// Recompute all edges deterministically across current active files
	// to ensure exact graph equivalence
	fileByBase := make(map[string]string)
	for p := range manifest.Files {
		fileNodeID := storeHashID("file|" + p)
		fileByBase[filepath.Base(p)] = fileNodeID
		fileByBase[p] = fileNodeID
	}

	var allEdges []store.EdgeRecord
	for p := range manifest.Files {
		fullPath := filepath.Join(idx.RepoRoot, filepath.FromSlash(p))
		b, rerr := os.ReadFile(fullPath)
		if rerr != nil {
			continue
		}
		txt := string(b)
		srcID := storeHashID("file|" + p)
		var deps []string
		for base, dst := range fileByBase {
			if base == filepath.Base(p) || base == p {
				continue
			}
			if strings.Contains(txt, base) {
				edgeKind := "references"
				if strings.HasSuffix(p, "_test.go") || strings.Contains(p, "test") {
					edgeKind = "test-reference"
				}
				allEdges = append(allEdges, store.EdgeRecord{SrcID: srcID, DstID: dst, Kind: edgeKind})
				deps = append(deps, base)
			}
		}
		entry := manifest.Files[p]
		entry.DependencyHash = ComputeDependencyHash(deps)
		manifest.Files[p] = entry
	}

	// Update store incrementally
	if err := idx.Store.UpdateNodesAndEdges(idx.RepoID, newFiles, newSyms, cs.Deleted, allEdges); err != nil {
		return nil, err
	}

	manifest.Revision = revision
	_ = SaveManifest(idx.ManifestPath, manifest)

	dur := float64(time.Since(start).Microseconds()) / 1000.0
	parsedCount := len(cs.Added) + len(cs.Modified)
	workReduction := 0.0
	if totalFiles > 0 {
		workReduction = 1.0 - float64(parsedCount)/float64(totalFiles)
	}

	return &IndexStats{
		Full:             false,
		FilesScanned:     totalFiles,
		FilesParsed:      parsedCount,
		SymbolsExtracted: len(newSyms),
		EdgesUpdated:     len(allEdges),
		AddedCount:       len(cs.Added),
		ModifiedCount:    len(cs.Modified),
		DeletedCount:     len(cs.Deleted),
		DurationMs:       dur,
		WorkReduction:    workReduction,
	}, nil
}

func storeHashID(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])[:20]
}
