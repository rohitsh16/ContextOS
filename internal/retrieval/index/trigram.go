package index

import (
	"strings"
	"unicode"
)

// Trigram represents a 3-character n-gram key.
type Trigram [3]byte

// PositionalTrigram records the occurrence of a trigram at a specific character position.
type PositionalTrigram struct {
	Tri Trigram
	Pos int
}

// NormalizeIdentifier splits identifiers by camelCase, snake_case, and non-alphanumerics into normalized tokens.
func NormalizeIdentifier(s string) []string {
	var words []string
	var current []rune

	for i, r := range s {
		if r == '_' || r == '/' || r == '.' || r == '-' || r == ':' {
			if len(current) > 0 {
				words = append(words, string(current))
				current = current[:0]
			}
			continue
		}
		if unicode.IsUpper(r) {
			if len(current) > 0 {
				// If previous is lowercase or next is lowercase, treat as boundary
				runes := []rune(s)
				if i > 0 && unicode.IsLower(runes[i-1]) {
					words = append(words, string(current))
					current = current[:0]
				} else if i+1 < len(runes) && unicode.IsLower(runes[i+1]) && len(current) > 1 {
					last := current[len(current)-1]
					words = append(words, string(current[:len(current)-1]))
					current = []rune{last}
				}
			}
		}
		current = append(current, unicode.ToLower(r))
	}
	if len(current) > 0 {
		words = append(words, string(current))
	}
	return words
}

// ExtractTrigrams extracts unique positional trigrams from a string (PR.md Section 8).
func ExtractTrigrams(s string) []PositionalTrigram {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) < 3 {
		return nil
	}
	bytes := []byte(s)
	n := len(bytes) - 2
	out := make([]PositionalTrigram, 0, n)
	seen := make(map[Trigram]bool)

	for i := 0; i < n; i++ {
		tri := Trigram{bytes[i], bytes[i+1], bytes[i+2]}
		if !seen[tri] {
			seen[tri] = true
			out = append(out, PositionalTrigram{
				Tri: tri,
				Pos: i,
			})
		}
	}
	return out
}

// ScoreTrigramMatch computes candidate positional scoring as defined in PR.md Section 8:
// S_tri = w1*coverage + w2*proximity + w3*exactness
func ScoreTrigramMatch(queryTris []PositionalTrigram, candidateText string) (score float64, matchedCount int) {
	if len(queryTris) == 0 || len(candidateText) < 3 {
		return 0, 0
	}
	candTris := ExtractTrigrams(candidateText)
	if len(candTris) == 0 {
		return 0, 0
	}

	candMap := make(map[Trigram]int, len(candTris))
	for _, pt := range candTris {
		candMap[pt.Tri] = pt.Pos
	}

	var matched int
	var proximityScore float64
	lastCandPos := -1

	for _, qt := range queryTris {
		if cpos, found := candMap[qt.Tri]; found {
			matched++
			if lastCandPos >= 0 {
				gap := cpos - lastCandPos
				if gap >= 1 && gap <= 5 {
					proximityScore += 1.0 / float64(gap)
				}
			}
			lastCandPos = cpos
		}
	}

	if matched == 0 {
		return 0, 0
	}

	coverage := float64(matched) / float64(len(queryTris))
	proximity := 0.0
	if matched > 1 {
		proximity = proximityScore / float64(matched-1)
	}

	exactness := 0.0
	if strings.Contains(strings.ToLower(candidateText), strings.ToLower(candidateText)) {
		exactness = 1.0
	}

	// PR.md weights: w1=0.6 coverage, w2=0.25 proximity, w3=0.15 exactness
	score = 0.60*coverage + 0.25*proximity + 0.15*exactness
	return score, matched
}
