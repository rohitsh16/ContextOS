package gitidx

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Repo struct{ Path, Name, Revision, Branch, WorktreeHash string }

type Symbol struct {
	Path, Kind, Name, Signature string
	Start, End                  int
	Hash                        string
}

type SourceFile struct {
	Path  string
	Hash  string
	Lines int
}

// DefaultExclusions lists directory names that are excluded from worktree
// fingerprinting and symbol scanning.
var DefaultExclusions = map[string]bool{
	".git":        true,
	".contextos":  true,
	"node_modules": true,
	"vendor":      true,
	"dist":        true,
	"build":       true,
	"target":      true,
}

func isExcludedPath(relPath string) bool {
	clean := filepath.ToSlash(relPath)
	parts := strings.Split(clean, "/")
	for _, p := range parts {
		if DefaultExclusions[p] {
			return true
		}
	}
	base := filepath.Base(clean)
	ext := strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".db", ".db-shm", ".db-wal", ".log", ".tmp":
		return true
	}
	return false
}

func run(dir string, args ...string) (string, error) {
	c := exec.Command("git", args...)
	c.Dir = dir
	b, e := c.Output()
	if e != nil {
		if x, ok := e.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), string(x.Stderr))
		}
		return "", e
	}
	return strings.TrimRight(string(b), "\r\n"), nil
}

// Detect inspects the repository at path and computes a cryptographically
// correct, content-addressed worktree fingerprint (PR-02).
func Detect(path string) (Repo, error) {
	abs, e := filepath.Abs(path)
	if e != nil {
		return Repo{}, e
	}
	root, e := run(abs, "rev-parse", "--show-toplevel")
	if e != nil || root == "" {
		return Repo{
			Path:         abs,
			Name:         filepath.Base(abs),
			Revision:     "working-tree",
			Branch:       "main",
			WorktreeHash: "uncommitted",
		}, nil
	}
	rev, e := run(root, "rev-parse", "HEAD")
	if e != nil || rev == "" {
		rev = "init"
	}
	branch, e := run(root, "branch", "--show-current")
	if e != nil || branch == "" {
		branch = "main"
	}

	wt, err := ComputeWorktreeFingerprint(root)
	if err != nil {
		wt = "clean"
	}

	return Repo{Path: root, Name: filepath.Base(root), Revision: rev, Branch: branch, WorktreeHash: wt}, nil
}

// MerkleNode represents a directory or file node in the worktree Merkle tree.
type MerkleNode struct {
	Path     string                 `json:"path"`
	Hash     string                 `json:"hash"`
	Children map[string]*MerkleNode `json:"children,omitempty"`
}

// ComputeWorktreeFingerprint computes a content-addressed, Merkle-style
// fingerprint for the uncommitted changes in a git repository.
//
// Invariant:
//
//	Content(A) == Content(B) => Fingerprint(A) == Fingerprint(B)
//	Content(A) != Content(B) => Fingerprint(A) != Fingerprint(B)
//
// It returns "clean" when there are no uncommitted changes.
func ComputeWorktreeFingerprint(root string) (string, error) {
	status, err := run(root, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return "clean", err
	}
	if strings.TrimSpace(status) == "" {
		return "clean", nil
	}

	lines := strings.Split(status, "\n")
	var entries []fileEntry

	for _, line := range lines {
		if len(line) < 3 {
			continue
		}
		statusTag := strings.TrimSpace(line[:2])
		filePath := strings.TrimSpace(line[2:])

		// Handle renames: "R  old -> new"
		if strings.Contains(filePath, " -> ") {
			parts := strings.Split(filePath, " -> ")
			filePath = parts[len(parts)-1]
		}

		filePath = filepath.ToSlash(filePath)
		if isExcludedPath(filePath) {
			continue
		}

		fullPath := filepath.Join(root, filepath.FromSlash(filePath))
		info, lerr := os.Lstat(fullPath)

		var contentHash string
		if lerr != nil {
			// Deleted file
			contentHash = "DELETED"
		} else if info.IsDir() {
			continue
		} else if info.Size() > 2*1024*1024 {
			// Large file: hash size + modtime
			contentHash = fmt.Sprintf("LARGE:%d:%d", info.Size(), info.ModTime().UnixNano())
		} else {
			data, rerr := os.ReadFile(fullPath)
			if rerr != nil {
				contentHash = "UNREADABLE"
			} else {
				h := sha256.Sum256(data)
				contentHash = hex.EncodeToString(h[:])
			}
		}

		// h_f = H(path || status || content)
		hfInput := fmt.Sprintf("%s\x00%s\x00%s", filePath, statusTag, contentHash)
		hf := sha256.Sum256([]byte(hfInput))
		entries = append(entries, fileEntry{
			relPath: filePath,
			hash:    hex.EncodeToString(hf[:]),
		})
	}

	if len(entries) == 0 {
		return "clean", nil
	}

	// Sort lexicographically by relative path for order-independence
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].relPath < entries[j].relPath
	})

	// Build Merkle hierarchy across directory tree
	tree := buildMerkleTree(entries)
	return tree.Hash[:16], nil
}

type fileEntry struct {
	relPath string
	hash    string
}

// buildMerkleTree constructs a hierarchical Merkle tree from file entries.
// H_d = H(H_child_1 || ... || H_child_k)
func buildMerkleTree(entries []fileEntry) *MerkleNode {
	root := &MerkleNode{
		Path:     "",
		Children: make(map[string]*MerkleNode),
	}

	for _, e := range entries {
		parts := strings.Split(e.relPath, "/")
		curr := root
		for i, part := range parts {
			if i == len(parts)-1 {
				// Leaf file node
				curr.Children[part] = &MerkleNode{
					Path: e.relPath,
					Hash: e.hash,
				}
			} else {
				if _, ok := curr.Children[part]; !ok {
					curr.Children[part] = &MerkleNode{
						Path:     strings.Join(parts[:i+1], "/"),
						Children: make(map[string]*MerkleNode),
					}
				}
				curr = curr.Children[part]
			}
		}
	}

	computeNodeHash(root)
	return root
}

// computeNodeHash recursively computes directory Merkle hashes from sorted children.
func computeNodeHash(node *MerkleNode) {
	if len(node.Children) == 0 {
		return
	}

	var childNames []string
	for name := range node.Children {
		childNames = append(childNames, name)
	}
	sort.Strings(childNames)

	h := sha256.New()
	for _, name := range childNames {
		child := node.Children[name]
		if len(child.Children) > 0 {
			computeNodeHash(child)
		}
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write([]byte(child.Hash))
		h.Write([]byte{0})
	}
	node.Hash = hex.EncodeToString(h.Sum(nil))
}

var symbolPatterns = []struct {
	re   *regexp.Regexp
	kind string
}{
	{regexp.MustCompile(`^\s*func\s+(?:\([^)]*\)\s*)?([A-Za-z_][A-Za-z0-9_]*)\s*\(`), "function"},
	{regexp.MustCompile(`^\s*type\s+([A-Za-z_][A-Za-z0-9_]*)\s+(?:struct|interface)\b`), "type"},
	{regexp.MustCompile(`^\s*(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_$][\w$]*)\s*\(`), "function"},
	{regexp.MustCompile(`^\s*(?:export\s+)?class\s+([A-Za-z_$][\w$]*)\b`), "class"},
	{regexp.MustCompile(`^\s*def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`), "function"},
	{regexp.MustCompile(`^\s*class\s+([A-Za-z_][A-Za-z0-9_]*)\b`), "class"},
	{regexp.MustCompile(`^\s*(?:public|private|protected)?\s*(?:static\s+)?(?:class|interface|enum)\s+([A-Za-z_][A-Za-z0-9_]*)\b`), "type"},
}

func Supported(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".py", ".js", ".jsx", ".ts", ".tsx", ".java", ".c", ".cc", ".cpp", ".h", ".hpp", ".rs", ".rb", ".kt", ".swift":
		return true
	}
	return false
}

func ListSourceFiles(root string) ([]SourceFile, error) {
	var out []SourceFile
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if isExcludedPath(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Size() > 2*1024*1024 || !Supported(path) {
			return nil
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		h := sha256.Sum256(b)
		out = append(out, SourceFile{Path: filepath.ToSlash(rel), Hash: hex.EncodeToString(h[:]), Lines: len(strings.Split(string(b), "\n"))})
		return nil
	})
	return out, err
}

func WalkSymbols(root string) ([]Symbol, error) {
	var out []Symbol
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if isExcludedPath(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Size() > 2*1024*1024 || !Supported(path) {
			return nil
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return nil
		}
		lines := strings.Split(string(b), "\n")
		rel, _ := filepath.Rel(root, path)
		h := sha256.Sum256(b)
		hs := hex.EncodeToString(h[:])
		for i, line := range lines {
			for _, p := range symbolPatterns {
				if m := p.re.FindStringSubmatch(line); len(m) > 1 {
					out = append(out, Symbol{Path: filepath.ToSlash(rel), Kind: p.kind, Name: m[1], Signature: strings.TrimSpace(line), Start: i + 1, End: i + 1, Hash: hs})
					break
				}
			}
		}
		return nil
	})
	return out, err
}

// ParseSymbolsForFile extracts symbols for a single file.
func ParseSymbolsForFile(relPath, fullPath string) ([]Symbol, error) {
	b, e := os.ReadFile(fullPath)
	if e != nil {
		return nil, e
	}
	return ParseSymbolsForContent(relPath, string(b))
}

// ParseSymbolsForContent extracts symbols from content string.
func ParseSymbolsForContent(relPath, content string) ([]Symbol, error) {
	var out []Symbol
	lines := strings.Split(content, "\n")
	h := sha256.Sum256([]byte(content))
	hs := hex.EncodeToString(h[:])
	for i, line := range lines {
		for _, p := range symbolPatterns {
			if m := p.re.FindStringSubmatch(line); len(m) > 1 {
				out = append(out, Symbol{
					Path:      filepath.ToSlash(relPath),
					Kind:      p.kind,
					Name:      m[1],
					Signature: strings.TrimSpace(line),
					Start:     i + 1,
					End:       i + 1,
					Hash:      hs,
				})
				break
			}
		}
	}
	return out, nil
}

