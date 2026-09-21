package retrieval

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Scope represents the localized search scope determined from query and environment signals.
type Scope struct {
	Primary    []string           `json:"primary"`    // Primary packages or paths to search
	Secondary  []string           `json:"secondary"`  // 1-2 hop neighborhood or related packages
	Confidence float64            `json:"confidence"` // Confidence score in [0.0, 1.0]
	Signals    map[string]float64 `json:"signals"`    // Attribution of individual signal weights
}

// LocalizerContext holds external hints and environment state for scope inference.
type LocalizerContext struct {
	Cwd            string            `json:"cwd"`
	RepoRoot       string            `json:"repo_root"`
	ChangedFiles   []string          `json:"changed_files"`
	RecentFiles    []string          `json:"recent_files"`
	CurrentPackage string            `json:"current_package"`
	ActiveWorkItem string            `json:"active_work_item"`
	PackageDeps    map[string][]string // Package -> dependencies
}

var (
	filePathRegex = regexp.MustCompile(`[a-zA-Z0-9_\-\.\/]+\.(?:go|ts|js|py|rs|java|cpp|c|h|md|json|yaml|yml|sql)`)
	identifierRegex = regexp.MustCompile(`\b[A-Z][a-zA-Z0-9_]{2,}\b|\b[a-z]+(?:[A-Z][a-z0-9]+)+\b`)
)

// InferScope extracts signals and infers the localized Scope for a query (PR.md Section 13).
func InferScope(queryText string, ctx LocalizerContext) Scope {
	signals := make(map[string]float64)
	primarySet := make(map[string]bool)
	secondarySet := make(map[string]bool)

	var confidence float64

	// 1. Explicit paths mentioned in query (Strong signal)
	pathMatches := filePathRegex.FindAllString(queryText, -1)
	if len(pathMatches) > 0 {
		for _, p := range pathMatches {
			norm := filepath.Clean(p)
			primarySet[norm] = true
			dir := filepath.Dir(norm)
			if dir != "." && dir != "/" {
				primarySet[dir] = true
			}
		}
		signals["explicit_path"] = 0.95
		confidence += 0.40
	}

	// 2. Active changed files (Strong signal)
	if len(ctx.ChangedFiles) > 0 {
		for _, f := range ctx.ChangedFiles {
			primarySet[f] = true
			dir := filepath.Dir(f)
			if dir != "." && dir != "/" {
				primarySet[dir] = true
			}
		}
		signals["changed_files"] = 0.85
		confidence += 0.25
	}

	// 3. Current package / working directory relative to RepoRoot (Strong signal)
	if ctx.CurrentPackage != "" {
		primarySet[ctx.CurrentPackage] = true
		signals["current_package"] = 0.80
		confidence += 0.15
	} else if ctx.Cwd != "" && ctx.RepoRoot != "" {
		rel, err := filepath.Rel(ctx.RepoRoot, ctx.Cwd)
		if err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			primarySet[rel] = true
			signals["cwd_locality"] = 0.70
			confidence += 0.10
		}
	}

	// 4. Exact CamelCase / PascalCase symbols in query (Strong signal)
	symbols := identifierRegex.FindAllString(queryText, -1)
	if len(symbols) > 0 {
		signals["exact_symbol"] = 0.75
		confidence += 0.10
	}

	// 5. Recent files (Medium signal -> Secondary scope)
	if len(ctx.RecentFiles) > 0 {
		for _, rf := range ctx.RecentFiles {
			if !primarySet[rf] {
				secondarySet[rf] = true
			}
		}
		signals["recent_files"] = 0.60
		confidence += 0.05
	}

	// 6. Package dependency neighborhood (Medium signal -> Secondary scope)
	if ctx.PackageDeps != nil {
		for pkg := range primarySet {
			if deps, ok := ctx.PackageDeps[pkg]; ok {
				for _, dep := range deps {
					if !primarySet[dep] {
						secondarySet[dep] = true
					}
				}
			}
		}
		if len(secondarySet) > 0 {
			signals["dep_neighborhood"] = 0.50
			confidence += 0.05
		}
	}

	// Bound confidence to [0.1, 1.0]
	if confidence > 1.0 {
		confidence = 1.0
	}
	if confidence < 0.10 {
		confidence = 0.10 // baseline global uncertainty
	}

	var primary []string
	for k := range primarySet {
		primary = append(primary, k)
	}
	var secondary []string
	for k := range secondarySet {
		secondary = append(secondary, k)
	}

	return Scope{
		Primary:    primary,
		Secondary:  secondary,
		Confidence: confidence,
		Signals:    signals,
	}
}
