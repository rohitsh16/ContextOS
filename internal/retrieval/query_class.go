// Package retrieval provides the layered, evidence-preserving retrieval
// pipeline (R17).
//
// query_class.go — Phase 4: Deterministic Query Classifier
//
// Classifies a user query into one or more typed QueryClass values so that
// the retriever can dispatch to exact, deterministic indexes before falling
// back to approximate lexical/semantic search.
//
// Theorem 2 (R17): if the query is FILE_BASENAME and the file exists, the
// exact path index guarantees Recall@1 = 1.0.
package retrieval

import (
	"path/filepath"
	"regexp"
	"strings"
)

// QueryClass categorizes the type of retrieval required.
type QueryClass uint8

const (
	// QueryClassUnknown is the zero value before classification.
	QueryClassUnknown QueryClass = iota

	// QueryClassPathExact matches a fully qualified repository-relative path.
	// e.g. "internal/store/sqlite_store.go"
	QueryClassPathExact

	// QueryClassFileBasename matches a bare filename without directory.
	// e.g. "sqlite_store.go"
	QueryClassFileBasename

	// QueryClassQualifiedSymbol matches a dotted qualified identifier.
	// e.g. "store.SQLiteStore.LookupPath"
	QueryClassQualifiedSymbol

	// QueryClassSymbol matches an unqualified symbol name (function, type, etc.).
	// e.g. "LookupPath"
	QueryClassSymbol

	// QueryClassIdentifier matches a general identifier token (snake_case, camelCase).
	// e.g. "gcp_attach_service_project_policy"
	QueryClassIdentifier

	// QueryClassLexical is a multi-word concept that needs FTS/lexical retrieval.
	QueryClassLexical

	// QueryClassRelation indicates a graph-structural question.
	// e.g. "what calls HandlePlan?"
	QueryClassRelation

	// QueryClassConceptual is an open-ended question without strong identifiers.
	QueryClassConceptual
)

func (qc QueryClass) String() string {
	switch qc {
	case QueryClassPathExact:
		return "PATH_EXACT"
	case QueryClassFileBasename:
		return "FILE_BASENAME"
	case QueryClassQualifiedSymbol:
		return "QUALIFIED_SYMBOL"
	case QueryClassSymbol:
		return "SYMBOL"
	case QueryClassIdentifier:
		return "IDENTIFIER"
	case QueryClassLexical:
		return "LEXICAL"
	case QueryClassRelation:
		return "RELATION"
	case QueryClassConceptual:
		return "CONCEPTUAL"
	default:
		return "UNKNOWN"
	}
}

// QueryClassification is the result of classifying a query.
type QueryClassification struct {
	// Primary is the dominant class; drives which index is queried first.
	Primary QueryClass

	// Secondary holds additional classes discovered (e.g. a query may be
	// both FILE_BASENAME and LEXICAL when it contains additional keywords).
	Secondary []QueryClass

	// Identifiers are the extracted strong-identifier tokens from the query.
	// These drive exact lookup before approximate retrieval.
	Identifiers []string

	// IsExact is true when Primary is PATH_EXACT, FILE_BASENAME,
	// QUALIFIED_SYMBOL, or SYMBOL — guaranteeing deterministic retrieval.
	IsExact bool
}

var (
	// reGoFile matches a Go filename like "foo_bar.go"
	reGoFile = regexp.MustCompile(`(?i)\b([a-z][a-z0-9_]*\.go)\b`)

	// reGenericFile matches common source file extensions
	reGenericFile = regexp.MustCompile(`(?i)\b([a-zA-Z][a-zA-Z0-9_.-]*\.(go|py|ts|tsx|js|jsx|java|rs|rb|kt|swift|c|cc|cpp|h|hpp))\b`)

	// reQualifiedSymbol matches dotted names like "pkg.Type.Method"
	reQualifiedSymbol = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*){2,})\b`)

	// reExportedSymbol matches PascalCase identifiers (likely exported Go types/funcs)
	reExportedSymbol = regexp.MustCompile(`\b([A-Z][A-Za-z0-9]{2,})\b`)

	// reSnakeIdentifier matches snake_case identifiers (common in file names and Python)
	reSnakeIdentifier = regexp.MustCompile(`\b([a-z][a-z0-9]{2,}(?:_[a-z0-9]+){1,})\b`)

	// reRelationKeywords triggers graph-relation classification
	reRelationKeywords = regexp.MustCompile(`(?i)\b(calls?|callers?|called by|imports?|imported by|depends? on|uses?|references?|references? by|parents?|children of)\b`)

	// rePathSep detects a path-like string with arbitrary slashes
	rePathSep = regexp.MustCompile(`[a-zA-Z0-9_.-]+(?:/[a-zA-Z0-9_.-]+)+`)
)

// ClassifyQuery analyzes the query string and returns a QueryClassification.
// Classification is deterministic and does not call any LLM or remote API.
//
// Design principle: strong identifiers (filenames, symbols) are detected first.
// Only queries with no strong identifier fall through to LEXICAL/CONCEPTUAL.
func ClassifyQuery(query string) QueryClassification {
	q := strings.TrimSpace(query)
	if q == "" {
		return QueryClassification{Primary: QueryClassConceptual}
	}

	var identifiers []string
	var secondary []QueryClass

	// --- Path-exact: contains a path separator ---
	if rePathSep.MatchString(q) {
		// Extract the path-like token
		tok := rePathSep.FindString(q)
		if strings.HasSuffix(strings.ToLower(tok), ".go") ||
			hasSourceExtension(tok) {
			identifiers = append(identifiers, tok)
			return QueryClassification{
				Primary:     QueryClassPathExact,
				Secondary:   secondary,
				Identifiers: identifiers,
				IsExact:     true,
			}
		}
	}

	// --- File basename: single token with source extension ---
	if fileMatches := reGenericFile.FindAllString(q, -1); len(fileMatches) > 0 {
		for _, m := range fileMatches {
			identifiers = append(identifiers, m)
		}
		primary := QueryClassFileBasename

		// May also be LEXICAL if the query has more words
		words := strings.Fields(q)
		if len(words) > len(fileMatches) {
			secondary = append(secondary, QueryClassLexical)
		}
		return QueryClassification{
			Primary:     primary,
			Secondary:   secondary,
			Identifiers: identifiers,
			IsExact:     true,
		}
	}

	// --- Qualified symbol: dotted name with 3+ parts ---
	if qsMatches := reQualifiedSymbol.FindAllString(q, -1); len(qsMatches) > 0 {
		for _, m := range qsMatches {
			identifiers = append(identifiers, m)
		}
		return QueryClassification{
			Primary:     QueryClassQualifiedSymbol,
			Secondary:   secondary,
			Identifiers: identifiers,
			IsExact:     true,
		}
	}

	// --- Graph relation query ---
	if reRelationKeywords.MatchString(q) {
		// Extract any symbol identifiers present
		for _, m := range reExportedSymbol.FindAllString(q, -1) {
			identifiers = append(identifiers, m)
		}
		secondary = append(secondary, QueryClassLexical)
		return QueryClassification{
			Primary:     QueryClassRelation,
			Secondary:   secondary,
			Identifiers: identifiers,
			IsExact:     false,
		}
	}

	// --- Symbol: exported (PascalCase) identifier ---
	exportedMatches := reExportedSymbol.FindAllString(q, -1)
	snakeMatches := reSnakeIdentifier.FindAllString(q, -1)

	if len(exportedMatches) > 0 {
		for _, m := range exportedMatches {
			identifiers = append(identifiers, m)
		}
		words := strings.Fields(q)
		if len(words) > len(exportedMatches) {
			secondary = append(secondary, QueryClassLexical)
		}
		return QueryClassification{
			Primary:     QueryClassSymbol,
			Secondary:   secondary,
			Identifiers: identifiers,
			IsExact:     len(words) == 1, // exact only when the query IS the symbol
		}
	}

	// --- Identifier: snake_case token ---
	if len(snakeMatches) > 0 {
		for _, m := range snakeMatches {
			identifiers = append(identifiers, m)
		}
		words := strings.Fields(q)
		if len(words) > len(snakeMatches) {
			secondary = append(secondary, QueryClassLexical)
		}
		return QueryClassification{
			Primary:     QueryClassIdentifier,
			Secondary:   secondary,
			Identifiers: identifiers,
			IsExact:     false,
		}
	}

	// --- Lexical fallback: multi-word conceptual query ---
	words := strings.Fields(q)
	if len(words) >= 3 {
		return QueryClassification{
			Primary:     QueryClassLexical,
			Secondary:   nil,
			Identifiers: nil,
			IsExact:     false,
		}
	}

	// --- Open-ended conceptual ---
	return QueryClassification{
		Primary:     QueryClassConceptual,
		Secondary:   nil,
		Identifiers: nil,
		IsExact:     false,
	}
}

// hasSourceExtension reports whether a path string ends with a known source
// file extension.
func hasSourceExtension(p string) bool {
	ext := strings.ToLower(filepath.Ext(p))
	switch ext {
	case ".go", ".py", ".ts", ".tsx", ".js", ".jsx", ".java", ".rs",
		".rb", ".kt", ".swift", ".c", ".cc", ".cpp", ".h", ".hpp":
		return true
	}
	return false
}
