package textutil

import (
	"hash/fnv"
	"regexp"
	"strings"
)

var nonWord = regexp.MustCompile(`[^a-zA-Z0-9_]+`)

func Tokens(s string) []string {
	s = strings.ToLower(s)
	s = nonWord.ReplaceAllString(s, " ")
	raw := strings.Fields(s)
	out := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, t := range raw {
		if len(t) < 2 {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

func Overlap(a, b string) float64 {
	at, bt := Tokens(a), Tokens(b)
	if len(at) == 0 || len(bt) == 0 {
		return 0
	}
	bm := map[string]struct{}{}
	for _, t := range bt {
		bm[t] = struct{}{}
	}
	hit := 0
	for _, t := range at {
		if _, ok := bm[t]; ok {
			hit++
		}
	}
	return float64(hit) / float64(len(at))
}

// HashSemantic provides a dependency-free semantic-ish similarity signal.
// It is deliberately not called an embedding: it hashes token and character
// features into a fixed-dimensional bag and measures cosine similarity.
func HashSemantic(a, b string) float64 {
	const dims = 128
	va := make([]float64, dims)
	vb := make([]float64, dims)
	add := func(v []float64, s string) {
		toks := Tokens(s)
		for _, t := range toks {
			h := fnv.New32a()
			_, _ = h.Write([]byte(t))
			idx := int(h.Sum32() % dims)
			v[idx] += 1
			if len(t) >= 3 {
				for i := 0; i+3 <= len(t); i++ {
					h2 := fnv.New32a()
					_, _ = h2.Write([]byte(t[i : i+3]))
					idx2 := int(h2.Sum32() % dims)
					v[idx2] += 0.35
				}
			}
		}
	}
	add(va, a)
	add(vb, b)
	var dot, na, nb float64
	for i := 0; i < dims; i++ {
		dot += va[i] * vb[i]
		na += va[i] * va[i]
		nb += vb[i] * vb[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (sqrt(na) * sqrt(nb))
}

func sqrt(v float64) float64 {
	// Newton iteration; avoids importing math in this tiny package.
	if v <= 0 {
		return 0
	}
	x := v
	for i := 0; i < 10; i++ {
		x = 0.5 * (x + v/x)
	}
	return x
}

func EstimateTokens(s string) int {
	n := len(strings.Fields(s))
	if n == 0 {
		return 1
	}
	// Conservative 1.3 words/token heuristic for mixed source text.
	return int(float64(n)*1.3 + 0.999)
}
