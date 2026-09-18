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

// BM25Score computes a BM25-style relevance score (Robertson & Sparck Jones, 1994) for a query
// against a document. It improves over plain Overlap by applying:
//   - TF saturation (k1=1.5): additional occurrences of a matched term yield diminishing returns.
//   - Length normalization (b=0.75): long verbose documents are penalized; short focused ones rewarded.
//
// avgDocLen is the average token count across the candidate corpus; pass 0 to use the document's own length.
// Result is normalized to [0,1] relative to a perfect match at average document length.
func BM25Score(query, doc string, avgDocLen float64) float64 {
	const k1, b = 1.5, 0.75
	qt := Tokens(query)
	dt := Tokens(doc)
	if len(qt) == 0 || len(dt) == 0 {
		return 0
	}
	dm := make(map[string]bool, len(dt))
	for _, t := range dt {
		dm[t] = true
	}
	dlen := float64(len(dt))
	if dlen < 1 {
		dlen = 1
	}
	if avgDocLen <= 0 {
		avgDocLen = dlen
	}
	// K is the length-normalized dampening factor for term frequency.
	K := k1 * (1 - b + b*dlen/avgDocLen)
	score := 0.0
	for _, t := range qt {
		if dm[t] {
			// tf=1 (binary after dedup); IDF approximated as 1 (no corpus available).
			score += (k1 + 1) / (1 + K)
		}
	}
	// Normalize so that a full match at avgDocLen gives 1.0.
	// When dlen==avgDocLen: K=k1, perTermScore=(k1+1)/(1+k1)=1.0.
	kAvg := k1 // K when dlen == avgDocLen
	perTermMax := (k1 + 1) / (1 + kAvg)
	maxScore := float64(len(qt)) * perTermMax
	if maxScore <= 0 {
		return 0
	}
	v := score / maxScore
	if v > 1 {
		return 1
	}
	return v
}
