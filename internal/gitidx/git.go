package gitidx

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	return strings.TrimSpace(string(b)), nil
}
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
	status, _ := run(root, "status", "--porcelain=v1", "--untracked-files=all")
	wt := "clean"
	if strings.TrimSpace(status) != "" {
		sum := sha256.Sum256([]byte(status))
		wt = hex.EncodeToString(sum[:])[:12]
	}
	return Repo{Path: root, Name: filepath.Base(root), Revision: rev, Branch: branch, WorktreeHash: wt}, nil
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
			switch info.Name() {
			case ".git", "node_modules", "vendor", ".contextos", "dist", "build", "target":
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
			switch info.Name() {
			case ".git", "node_modules", "vendor", ".contextos", "dist", "build", "target":
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
