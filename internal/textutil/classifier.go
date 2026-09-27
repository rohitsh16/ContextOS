package textutil

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// TokenClass categorizes a query token for query-aware search budgeting (R18.1 §7).
type TokenClass int

const (
	ClassPathFilename   TokenClass = iota // Exact file paths or filenames with extension
	ClassSymbol                           // Symbol identifiers: camelCase, PascalCase, dotted symbols
	ClassIdentifier                       // snake_case, kebab-case identifiers
	ClassEntity                           // Domain acronyms and named entities (e.g. GCP, DRMC, VPC)
	ClassTechnicalTerm                    // Domain technical nouns
	ClassAction                           // Operational action verbs
	ClassGeneric                          // Generic natural language query boilerplate
	ClassStopword                         // Syntactic English stopwords
)

func (c TokenClass) String() string {
	switch c {
	case ClassPathFilename:
		return "PATH/FILENAME"
	case ClassSymbol:
		return "SYMBOL"
	case ClassIdentifier:
		return "IDENTIFIER"
	case ClassEntity:
		return "ENTITY"
	case ClassTechnicalTerm:
		return "TECHNICAL_TERM"
	case ClassAction:
		return "ACTION"
	case ClassGeneric:
		return "GENERIC"
	case ClassStopword:
		return "STOPWORD"
	default:
		return "UNKNOWN"
	}
}

// ClassifiedToken represents an analyzed query token with priority and bounded variants.
type ClassifiedToken struct {
	Text     string     `json:"text"`
	Class    TokenClass `json:"class"`
	Priority int        `json:"priority"` // Lower number = higher search priority
	Variants []string   `json:"variants,omitempty"`
}

var (
	reClassifierPath       = regexp.MustCompile(`[a-zA-Z0-9_.-]+(?:/[a-zA-Z0-9_.-]+)+`)
	reClassifierFileExt    = regexp.MustCompile(`(?i)\b([a-zA-Z0-9_.-]+\.(?:go|py|ts|tsx|js|json|md|yaml|yml|proto|sh|sql|rs|c|cpp|h))\b`)
	reClassifierDotted     = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)+)\b`)
	reClassifierPascal     = regexp.MustCompile(`\b([A-Z][a-z0-9]+(?:[A-Z][a-z0-9]+)+)\b`)
	reClassifierCamel      = regexp.MustCompile(`\b([a-z][a-z0-9]*(?:[A-Z][a-z0-9]*)+)\b`)
	reClassifierAcronym    = regexp.MustCompile(`\b([A-Z]{2,6})\b`)
	reClassifierSnake           = regexp.MustCompile(`\b([a-z0-9]+(?:_[a-z0-9]+)+)\b`)
	reClassifierKebab           = regexp.MustCompile(`\b([a-z0-9]+(?:-[a-z0-9]+)+)\b`)
	reClassifierSymbolWithDigit = regexp.MustCompile(`\b([A-Za-z_][a-zA-Z0-9_]*[0-9]+[a-zA-Z0-9_]*)\b`)
)

var technicalTerms = map[string]bool{
	"attachment": true, "attached": true, "attaching": true, "bunker": true, "bunkers": true,
	"policy": true, "policies": true, "routing": true, "router": true, "admission": true,
	"allocation": true, "provenance": true, "frontier": true, "cache": true, "retrieval": true,
	"mutation": true, "invariant": true, "telemetry": true, "cluster": true, "tenant": true,
	"service": true, "services": true, "provider": true, "providers": true, "handler": true,
	"workflow": true, "workflows": true, "memory": true, "memories": true, "spec": true,
	"decision": true, "decisions": true, "session": true, "sessions": true,
}

var actionVerbs = map[string]bool{
	"attach": true, "attaches": true, "exclude": true, "excludes": true, "excluding": true,
	"hydrate": true, "hydrates": true, "filter": true, "filters": true, "trace": true,
	"tracing": true, "debug": true, "verify": true, "verifies": true, "check": true,
	"inspect": true, "build": true, "compile": true, "route": true, "block": true,
	"blocks": true, "deny": true, "denies": true, "prevent": true, "prevents": true,
}

var genericBoilerplate = map[string]bool{
	"find": true, "where": true, "show": true, "tell": true, "what": true, "does": true,
	"how": true, "logic": true, "code": true, "file": true, "files": true, "project": true,
	"projects": true, "thing": true, "things": true, "why": true, "which": true, "when": true,
	"who": true, "look": true, "explain": true, "about": true, "implement": true,
	"implementation": true, "details": true, "example": true, "give": true, "want": true,
	"need": true, "work": true, "system": true, "operations": true, "pattern": true,
	"related": true, "handled": true, "handles": true,
}

var commonStopwords = map[string]bool{
	"a": true, "an": true, "the": true, "in": true, "on": true, "at": true, "by": true,
	"for": true, "to": true, "of": true, "from": true, "with": true, "is": true, "are": true,
	"was": true, "were": true, "be": true, "been": true, "being": true, "that": true,
	"this": true, "those": true, "these": true, "it": true, "its": true, "their": true,
	"and": true, "or": true, "as": true, "into": true, "if": true, "out": true, "up": true,
}

// ClassifyToken assigns a TokenClass and priority to an individual token string.
func ClassifyToken(token string) (TokenClass, int) {
	low := strings.ToLower(token)
	if commonStopwords[low] {
		return ClassStopword, 8
	}

	// 1. Path or filename
	if strings.Contains(token, "/") || reClassifierFileExt.MatchString(token) {
		return ClassPathFilename, 1
	}

	// 2. Go symbol: camelCase, PascalCase, dotted, or symbol with digit
	if reClassifierDotted.MatchString(token) || reClassifierCamel.MatchString(token) || reClassifierPascal.MatchString(token) || reClassifierSymbolWithDigit.MatchString(token) {
		return ClassSymbol, 2
	}

	// 3. Capitalized symbol/identifier (not stopword, boilerplate, or action)
	if len(token) >= 2 && unicode.IsUpper(rune(token[0])) && !genericBoilerplate[low] && !actionVerbs[low] {
		return ClassSymbol, 2
	}

	// 4. Entity acronym
	if reClassifierAcronym.MatchString(token) && token != "AND" && token != "THE" && token != "FOR" && token != "NOT" {
		return ClassEntity, 3
	}

	// 5. Identifier: snake_case, kebab-case
	if strings.Contains(token, "_") || strings.Contains(token, "-") {
		return ClassIdentifier, 4
	}

	// 6. Technical domain term
	if technicalTerms[low] {
		return ClassTechnicalTerm, 5
	}

	// 7. Action verb
	if actionVerbs[low] {
		return ClassAction, 6
	}

	// 8. Generic natural language boilerplate
	if genericBoilerplate[low] {
		return ClassGeneric, 7
	}

	return ClassTechnicalTerm, 5
}

// GenerateBoundedVariants produces deterministic casing and format variants (R18.1 §9).
func GenerateBoundedVariants(token string, class TokenClass) []string {
	var variants []string
	seen := map[string]bool{token: true}
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] && len(variants) < 6 {
			seen[v] = true
			variants = append(variants, v)
		}
	}

	switch class {
	case ClassPathFilename:
		base := filepath.Base(token)
		add(base)
		ext := filepath.Ext(base)
		if ext != "" {
			noExt := strings.TrimSuffix(base, ext)
			add(noExt)
			if strings.Contains(noExt, "_") {
				add(strings.ReplaceAll(noExt, "_", " "))
				add(strings.ReplaceAll(noExt, "_", "-"))
			}
		}
	case ClassSymbol:
		// Strip trailing digits if any (e.g. Handler0 -> Handler, handler)
		trimmed := strings.TrimRight(token, "0123456789")
		if trimmed != "" && trimmed != token {
			add(trimmed)
			add(strings.ToLower(trimmed))
		}
		// e.g. isBackupTeam -> is_backup_team, is-backup-team, is backup team
		snake := CamelToSnake(token)
		add(snake)
		add(strings.ReplaceAll(snake, "_", "-"))
		add(strings.ReplaceAll(snake, "_", " "))
		add(strings.ToLower(token))
	case ClassIdentifier:
		// snake_case
		if strings.Contains(token, "_") {
			parts := strings.Split(token, "_")
			add(strings.Join(parts, "-"))
			add(strings.Join(parts, " "))
			// PascalCase
			var camel strings.Builder
			for _, p := range parts {
				if len(p) > 0 {
					camel.WriteString(strings.ToUpper(p[:1]) + strings.ToLower(p[1:]))
				}
			}
			add(camel.String())
		}
		// kebab-case
		if strings.Contains(token, "-") {
			parts := strings.Split(token, "-")
			add(strings.Join(parts, "_"))
			add(strings.Join(parts, " "))
		}
	}

	return variants
}

// ClassifyQueryTokens parses raw query text into classified tokens ordered by priority (R18.1 §7).
func ClassifyQueryTokens(query string) []ClassifiedToken {
	seen := make(map[string]bool)
	var tokens []ClassifiedToken

	addTok := func(text string, class TokenClass, priority int) {
		text = strings.TrimSpace(text)
		if len(text) < 2 {
			return
		}
		key := strings.ToLower(text)
		if seen[key] {
			return
		}
		seen[key] = true

		variants := GenerateBoundedVariants(text, class)
		tokens = append(tokens, ClassifiedToken{
			Text:     text,
			Class:    class,
			Priority: priority,
			Variants: variants,
		})
	}

	// 1. High-priority regex extractions first
	for _, m := range reClassifierPath.FindAllString(query, -1) {
		addTok(m, ClassPathFilename, 1)
	}
	for _, m := range reClassifierFileExt.FindAllString(query, -1) {
		addTok(m, ClassPathFilename, 1)
	}
	for _, m := range reClassifierDotted.FindAllString(query, -1) {
		addTok(m, ClassSymbol, 2)
	}
	for _, m := range reClassifierCamel.FindAllString(query, -1) {
		addTok(m, ClassSymbol, 2)
	}
	for _, m := range reClassifierPascal.FindAllString(query, -1) {
		addTok(m, ClassSymbol, 2)
	}
	for _, m := range reClassifierSymbolWithDigit.FindAllString(query, -1) {
		addTok(m, ClassSymbol, 2)
	}
	for _, m := range reClassifierAcronym.FindAllString(query, -1) {
		if m != "AND" && m != "THE" && m != "FOR" && m != "NOT" {
			addTok(m, ClassEntity, 3)
		}
	}
	for _, m := range reClassifierSnake.FindAllString(query, -1) {
		addTok(m, ClassIdentifier, 4)
	}
	for _, m := range reClassifierKebab.FindAllString(query, -1) {
		addTok(m, ClassIdentifier, 4)
	}

	// 2. Individual word tokens from query
	words := strings.FieldsFunc(query, func(r rune) bool {
		return unicode.IsSpace(r) || r == ',' || r == ';' || r == ':' || r == '(' || r == ')' || r == '[' || r == ']' || r == '"' || r == '\'' || r == '?' || r == '!'
	})
	for _, w := range words {
		w = strings.TrimSpace(w)
		if len(w) < 2 {
			continue
		}
		class, prio := ClassifyToken(w)
		if class == ClassStopword {
			continue
		}
		addTok(w, class, prio)
	}

	// 3. Sort by priority ascending (Priority 1 = Path/Filename first)
	sort.SliceStable(tokens, func(i, j int) bool {
		return tokens[i].Priority < tokens[j].Priority
	})

	return tokens
}

// ExtractPrioritizedTokens returns search query tokens ordered strictly by discriminative value (R18.1 §8).
func ExtractPrioritizedTokens(query string) []string {
	classified := ClassifyQueryTokens(query)
	var out []string
	seen := make(map[string]bool)

	add := func(s string) {
		s = strings.TrimSpace(s)
		if len(s) >= 2 && !seen[strings.ToLower(s)] {
			seen[strings.ToLower(s)] = true
			out = append(out, s)
		}
	}

	// Primary tokens in priority order
	for _, ct := range classified {
		if ct.Class == ClassStopword {
			continue
		}
		add(ct.Text)
		for _, v := range ct.Variants {
			add(v)
		}
	}

	return out
}

// CamelToSnake converts camelCase or PascalCase to snake_case.
func CamelToSnake(s string) string {
	var sb strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				sb.WriteByte('_')
			}
			sb.WriteRune(unicode.ToLower(r))
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
