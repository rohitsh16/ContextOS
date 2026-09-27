package retrieval

import (
	"path/filepath"
	"regexp"
	"strings"
)

// QueryIntent classifies the engineering objective of the user query (R18.1 §8 & §15).
type QueryIntent string

const (
	QueryIntentLookup       QueryIntent = "lookup"
	QueryIntentTrace        QueryIntent = "trace"
	QueryIntentPolicy       QueryIntent = "policy"
	QueryIntentDebug        QueryIntent = "debug"
	QueryIntentArchitecture QueryIntent = "architecture"
	QueryIntentTest         QueryIntent = "test"
	QueryIntentComparison   QueryIntent = "comparison"
)

// QueryEntity represents a domain entity, component name, or technical noun.
type QueryEntity struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"` // "acronym", "component", "provider", "noun"
	Salience float64 `json:"salience"`
}

// QueryAction represents an operation, verb, or behavior indicated in the query.
type QueryAction struct {
	Verb    string `json:"verb"`
	Negated bool   `json:"negated"`
}

// QueryConstraint represents a constraint or condition (e.g. "excluded from attachment").
type QueryConstraint struct {
	Description string `json:"description"`
}

// QueryArtifactType represents an expected code artifact category (policy, workflow, etc.).
type QueryArtifactType struct {
	Type string `json:"type"` // "policy", "workflow", "plan", "test", "config", "doc"
}

// QueryRepresentation provides a structured, multi-dimensional decomposition of a query (R18.1 §8 & §12).
type QueryRepresentation struct {
	Raw           string              `json:"raw"`
	Entities      []QueryEntity       `json:"entities"`
	Actions       []QueryAction       `json:"actions"`
	Constraints   []QueryConstraint   `json:"constraints"`
	ArtifactTypes []QueryArtifactType `json:"artifact_types"`
	Identifiers   []string            `json:"identifiers"`
	Paths         []string            `json:"paths"`
	Filenames     []string            `json:"filenames"`
	Symbols       []string            `json:"symbols"`
	Concepts      []string            `json:"concepts"`
	Negations     []string            `json:"negations"`
	Intent        QueryIntent         `json:"intent"`
}

var (
	rePathToken        = regexp.MustCompile(`[a-zA-Z0-9_.-]+(?:/[a-zA-Z0-9_.-]+)+`)
	reFileExtension    = regexp.MustCompile(`(?i)\b([a-zA-Z0-9_.-]+\.(?:go|py|ts|tsx|js|json|md|yaml|yml|proto|sh))\b`)
	reDottedSymbol     = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)+)\b`)
	rePascalCase       = regexp.MustCompile(`\b([A-Z][a-z0-9]+(?:[A-Z][a-z0-9]+)+)\b`)
	reCamelCase        = regexp.MustCompile(`\b([a-z][a-z0-9]*(?:[A-Z][a-z0-9]*)+)\b`)
	reSnakeCase        = regexp.MustCompile(`\b([a-z0-9]+(?:_[a-z0-9]+)+)\b`)
	reKebabCase        = regexp.MustCompile(`\b([a-z0-9]+(?:-[a-z0-9]+)+)\b`)
	reAcronym          = regexp.MustCompile(`\b([A-Z]{2,6})\b`)
	reSymbolWithDigit  = regexp.MustCompile(`\b([A-Za-z_][a-zA-Z0-9_]*[0-9]+[a-zA-Z0-9_]*)\b`)
	reCapitalizedIdent = regexp.MustCompile(`\b([A-Z][a-zA-Z0-9_]*)\b`)
	reNegations        = regexp.MustCompile(`(?i)\b(not|never|no|stop|stops|prevent|prevents|exclude|excludes|excluding|block|blocks|blocking|without|disabled?|deny|denies)\b`)
	reArtifactKeywords = regexp.MustCompile(`(?i)\b(policy|policies|workflow|workflows|provider|providers|plan|plans|test|tests|testing|config|configs|configuration|spec|specs|doc|docs|documentation|script|scripts|handler|handlers|helper|helpers)\b`)
	reActionVerbs      = regexp.MustCompile(`(?i)\b(attach|attaches|attachment|attaching|exclude|excludes|exclusion|excluding|hydrate|hydrates|hydration|build|builds|building|create|delete|remove|filter|find|trace|tracing|debug|fix|route|routing|inspect|verify|call|calls)\b`)
)

// DecomposeQueryRepresentation deterministically extracts structured features from raw query text.
func DecomposeQueryRepresentation(query string) *QueryRepresentation {
	raw := strings.TrimSpace(query)
	rep := &QueryRepresentation{
		Raw:           raw,
		Entities:      make([]QueryEntity, 0),
		Actions:       make([]QueryAction, 0),
		Constraints:   make([]QueryConstraint, 0),
		ArtifactTypes: make([]QueryArtifactType, 0),
		Identifiers:   make([]string, 0),
		Paths:         make([]string, 0),
		Filenames:     make([]string, 0),
		Symbols:       make([]string, 0),
		Concepts:      make([]string, 0),
		Negations:     make([]string, 0),
		Intent:        QueryIntentLookup,
	}
	if raw == "" {
		return rep
	}

	seenIdent := make(map[string]bool)
	addIdent := func(id string) {
		id = strings.TrimSpace(id)
		if len(id) >= 2 && !seenIdent[strings.ToLower(id)] {
			seenIdent[strings.ToLower(id)] = true
			rep.Identifiers = append(rep.Identifiers, id)
		}
	}

	// 1. Extract file paths and basenames (R18.1 §6)
	for _, p := range rePathToken.FindAllString(raw, -1) {
		rep.Paths = append(rep.Paths, p)
		base := filepath.Base(p)
		rep.Filenames = append(rep.Filenames, base)
		addIdent(p)
		addIdent(base)
		ext := filepath.Ext(base)
		if ext != "" {
			addIdent(strings.TrimSuffix(base, ext))
		}
	}
	for _, f := range reFileExtension.FindAllString(raw, -1) {
		rep.Paths = append(rep.Paths, f)
		base := filepath.Base(f)
		rep.Filenames = append(rep.Filenames, base)
		addIdent(f)
		addIdent(base)
		ext := filepath.Ext(base)
		if ext != "" {
			addIdent(strings.TrimSuffix(base, ext))
		}
	}

	// 2. Extract qualified, PascalCase, symbols with digits, and camelCase symbols (R18.1 §6 & §8)
	for _, s := range reDottedSymbol.FindAllString(raw, -1) {
		rep.Symbols = append(rep.Symbols, s)
		addIdent(s)
	}
	for _, s := range rePascalCase.FindAllString(raw, -1) {
		rep.Symbols = append(rep.Symbols, s)
		addIdent(s)
	}
	for _, s := range reCamelCase.FindAllString(raw, -1) {
		rep.Symbols = append(rep.Symbols, s)
		addIdent(s)
	}
	for _, s := range reSymbolWithDigit.FindAllString(raw, -1) {
		rep.Symbols = append(rep.Symbols, s)
		addIdent(s)
		trimmed := strings.TrimRight(s, "0123456789")
		if len(trimmed) >= 2 {
			addIdent(trimmed)
		}
	}
	for _, s := range reCapitalizedIdent.FindAllString(raw, -1) {
		low := strings.ToLower(s)
		if len(s) >= 2 && !isCommonStopWord(low) && !isGenericBoilerplateWord(low) && !reActionVerbs.MatchString(low) && !reNegations.MatchString(low) {
			rep.Symbols = append(rep.Symbols, s)
			addIdent(s)
		}
	}

	// 3. Extract snake_case and kebab-case identifiers
	for _, s := range reSnakeCase.FindAllString(raw, -1) {
		addIdent(s)
	}
	for _, k := range reKebabCase.FindAllString(raw, -1) {
		addIdent(k)
	}

	// 4. Extract Acronyms and technical entities (e.g. DRMC, GCP, VPC)
	for _, ac := range reAcronym.FindAllString(raw, -1) {
		if ac != "AND" && ac != "THE" && ac != "FOR" {
			addIdent(ac)
			rep.Entities = append(rep.Entities, QueryEntity{
				Name:     ac,
				Type:     "acronym",
				Salience: 1.0,
			})
		}
	}

	// Common domain nouns as entities
	low := strings.ToLower(raw)
	commonNouns := []string{"bunker", "service project", "shared vpc", "routing", "admission", "provenance"}
	for _, cn := range commonNouns {
		if strings.Contains(low, cn) {
			rep.Entities = append(rep.Entities, QueryEntity{
				Name:     cn,
				Type:     "component",
				Salience: 0.8,
			})
			addIdent(cn)
		}
	}

	// 5. Extract negations and constraints
	for _, neg := range reNegations.FindAllString(raw, -1) {
		negLow := strings.ToLower(neg)
		rep.Negations = append(rep.Negations, negLow)
		rep.Constraints = append(rep.Constraints, QueryConstraint{
			Description: neg,
		})
		// Extract object immediately following negation word (e.g. "without SQLite" -> "sqlite")
		idx := strings.Index(low, negLow+" ")
		if idx >= 0 {
			after := strings.Fields(low[idx+len(negLow)+1:])
			if len(after) > 0 {
				negTerm := strings.Trim(after[0], "?.,;:!\"'")
				if negTerm != "" {
					rep.Negations = append(rep.Negations, negTerm)
				}
			}
		}
	}

	// 6. Extract actions
	for _, act := range reActionVerbs.FindAllString(raw, -1) {
		isNegated := false
		for _, neg := range rep.Negations {
			if strings.Contains(low, neg+" "+strings.ToLower(act)) {
				isNegated = true
				break
			}
		}
		rep.Actions = append(rep.Actions, QueryAction{
			Verb:    strings.ToLower(act),
			Negated: isNegated,
		})
	}

	// 7. Extract artifact types
	for _, art := range reArtifactKeywords.FindAllString(raw, -1) {
		baseArt := strings.ToLower(art)
		if strings.HasSuffix(baseArt, "ies") {
			baseArt = strings.TrimSuffix(baseArt, "ies") + "y"
		} else if strings.HasSuffix(baseArt, "s") {
			baseArt = strings.TrimSuffix(baseArt, "s")
		}
		rep.ArtifactTypes = append(rep.ArtifactTypes, QueryArtifactType{Type: baseArt})
	}

	// 8. Classify Intent
	switch {
	case strings.Contains(low, "trace") || strings.Contains(low, "call") || strings.Contains(low, "chain") || strings.Contains(low, "workflow") || strings.Contains(low, "flow"):
		rep.Intent = QueryIntentTrace
	case strings.Contains(low, "policy") || strings.Contains(low, "exclude") || strings.Contains(low, "prevent") || strings.Contains(low, "rule") || strings.Contains(low, "stop"):
		rep.Intent = QueryIntentPolicy
	case strings.Contains(low, "why") || strings.Contains(low, "bug") || strings.Contains(low, "fail") || strings.Contains(low, "error") || strings.Contains(low, "issue") || strings.Contains(low, "debug"):
		rep.Intent = QueryIntentDebug
	case strings.Contains(low, "test") || strings.Contains(low, "spec") || strings.Contains(low, "verify"):
		rep.Intent = QueryIntentTest
	case strings.Contains(low, "architecture") || strings.Contains(low, "design") || strings.Contains(low, "overview"):
		rep.Intent = QueryIntentArchitecture
	case strings.Contains(low, "compare") || strings.Contains(low, "difference") || strings.Contains(low, "vs"):
		rep.Intent = QueryIntentComparison
	default:
		rep.Intent = QueryIntentLookup
	}

	return rep
}

func isGenericBoilerplateWord(w string) bool {
	switch w {
	case "find", "show", "tell", "where", "what", "how", "why", "who", "which",
		"when", "does", "code", "file", "files", "project", "projects", "thing",
		"explain", "implementation", "details", "example", "about", "work", "system",
		"operations", "pattern", "related", "handled", "handles":
		return true
	default:
		return false
	}
}
