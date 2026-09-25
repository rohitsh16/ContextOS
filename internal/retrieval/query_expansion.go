package retrieval

import (
	"strings"
	"unicode"
)

// ExpandedTerms contains the bounded set of generated lexical, casing, and morphological variants (R18.1 §9).
type ExpandedTerms struct {
	OriginalQuery      string              `json:"original_query"`
	PrimaryTerms       []string            `json:"primary_terms"`
	CasingVariants     []string            `json:"casing_variants"`
	MorphologicalStems []string            `json:"morphological_stems"`
	IdentifierVariants []string            `json:"identifier_variants"`
	AllSearchTokens    []string            `json:"all_search_tokens"`
}

// Bounded expansion constraints
const (
	MaxPrimaryTerms        = 25
	MaxCasingVariants      = 10
	MaxMorphologicalVariants = 10
	MaxAllTokens           = 35
)

// ExpandQueryTerms generates repository-aware casing and morphological variations from a QueryRepresentation.
func ExpandQueryTerms(rep *QueryRepresentation) *ExpandedTerms {
	ext := &ExpandedTerms{
		OriginalQuery:      rep.Raw,
		PrimaryTerms:       make([]string, 0),
		CasingVariants:     make([]string, 0),
		MorphologicalStems: make([]string, 0),
		IdentifierVariants: make([]string, 0),
		AllSearchTokens:    make([]string, 0),
	}

	seen := make(map[string]bool)
	addToken := func(t string, dest *[]string) {
		t = strings.TrimSpace(t)
		if len(t) < 2 {
			return
		}
		low := strings.ToLower(t)
		// Filter basic stop words
		if isCommonStopWord(low) {
			return
		}
		if !seen[low] {
			seen[low] = true
			*dest = append(*dest, t)
			if len(ext.AllSearchTokens) < MaxAllTokens {
				ext.AllSearchTokens = append(ext.AllSearchTokens, t)
			}
		}
	}

	// 1. Add primary identifiers and entities with high priority
	for _, id := range rep.Identifiers {
		if len(ext.PrimaryTerms) < MaxPrimaryTerms {
			addToken(id, &ext.PrimaryTerms)
		}
	}
	for _, ent := range rep.Entities {
		if len(ext.PrimaryTerms) < MaxPrimaryTerms {
			addToken(ent.Name, &ext.PrimaryTerms)
		}
	}

	// 2. Generate casing variants (CamelCase <-> snake_case <-> kebab-case <-> spaced)
	for _, id := range rep.Identifiers {
		if len(ext.CasingVariants) >= MaxCasingVariants {
			break
		}
		// snake_case to CamelCase & space
		if strings.Contains(id, "_") {
			parts := strings.Split(id, "_")
			camel := ""
			for _, p := range parts {
				if len(p) > 0 {
					camel += strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
				}
			}
			addToken(camel, &ext.CasingVariants)
			addToken(strings.Join(parts, " "), &ext.CasingVariants)
			addToken(strings.Join(parts, "-"), &ext.CasingVariants)
		}
		// kebab-case to snake_case & space
		if strings.Contains(id, "-") {
			parts := strings.Split(id, "-")
			addToken(strings.Join(parts, "_"), &ext.CasingVariants)
			addToken(strings.Join(parts, " "), &ext.CasingVariants)
		}
		// CamelCase to snake_case
		if isCamelCase(id) {
			snake := camelToSnake(id)
			addToken(snake, &ext.CasingVariants)
			addToken(strings.ReplaceAll(snake, "_", " "), &ext.CasingVariants)
			addToken(strings.ReplaceAll(snake, "_", "-"), &ext.CasingVariants)
		}
	}

	// 3. Morphological variants for common engineering verbs and nouns
	morphologyMap := map[string][]string{
		"attach":     {"attachment", "attached", "attaching"},
		"attachment": {"attach", "attached"},
		"exclude":    {"exclusion", "excluded", "excludes"},
		"exclusion":  {"exclude", "excluded"},
		"hydrate":    {"hydration", "hydrates"},
		"hydration":  {"hydrate", "hydrates"},
		"policy":     {"policies"},
		"policies":   {"policy"},
		"route":      {"routing", "router"},
		"routing":    {"route", "router"},
		"bunker":     {"bunkers"},
		"service":    {"services"},
		"workflow":   {"workflows"},
	}

	for _, token := range rep.Identifiers {
		low := strings.ToLower(token)
		if variants, ok := morphologyMap[low]; ok {
			for _, v := range variants {
				if len(ext.MorphologicalStems) < MaxMorphologicalVariants {
					addToken(v, &ext.MorphologicalStems)
				}
			}
		}
	}

	// 4. Always add significant non-stop words from raw query
	for _, w := range strings.Fields(rep.Raw) {
		w = strings.TrimFunc(w, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-'
		})
		addToken(w, &ext.PrimaryTerms)
	}

	return ext
}

func isCommonStopWord(w string) bool {
	switch w {
	case "a", "an", "the", "in", "on", "at", "by", "for", "to", "of", "from",
		"where", "why", "how", "what", "which", "who", "when", "does", "do", "did",
		"is", "are", "was", "were", "be", "been", "being", "have", "has", "had",
		"those", "these", "that", "this", "it", "its", "their", "them", "they",
		"get", "gets", "got", "can", "could", "would", "should":
		return true
	default:
		return false
	}
}

func isCamelCase(s string) bool {
	hasUpper := false
	hasLower := false
	for _, r := range s {
		if unicode.IsUpper(r) {
			hasUpper = true
		}
		if unicode.IsLower(r) {
			hasLower = true
		}
	}
	return hasUpper && hasLower && !strings.ContainsAny(s, "_- ")
}

func camelToSnake(s string) string {
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
