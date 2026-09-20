package textutil

import (
	"hash/fnv"
	"math"
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

// TokenCounts returns the term frequencies and total token count for a document.
func TokenCounts(s string) (map[string]int, int) {
	s = strings.ToLower(s)
	s = nonWord.ReplaceAllString(s, " ")
	raw := strings.Fields(s)
	counts := make(map[string]int, len(raw))
	total := 0
	for _, t := range raw {
		if len(t) < 2 {
			continue
		}
		counts[t]++
		total++
	}
	return counts, total
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

// CorpusStats maintains collection-level statistics for corpus-aware BM25 scoring.
type CorpusStats struct {
	DocCount   int            `json:"doc_count"`
	DocLengths int            `json:"doc_lengths"`
	AvgDocLen  float64        `json:"avg_doc_len"`
	DocFreqs   map[string]int `json:"doc_freqs"`
}

// NewCorpusStats builds CorpusStats from a slice of document contents.
func NewCorpusStats(docs []string) *CorpusStats {
	cs := &CorpusStats{
		DocCount: len(docs),
		DocFreqs: make(map[string]int),
	}
	totalLen := 0
	for _, doc := range docs {
		counts, docLen := TokenCounts(doc)
		totalLen += docLen
		for term := range counts {
			cs.DocFreqs[term]++
		}
	}
	cs.DocLengths = totalLen
	if cs.DocCount > 0 {
		cs.AvgDocLen = float64(totalLen) / float64(cs.DocCount)
	}
	return cs
}

// AddDocument dynamically updates the corpus statistics with an additional document.
func (cs *CorpusStats) AddDocument(doc string) {
	if cs.DocFreqs == nil {
		cs.DocFreqs = make(map[string]int)
	}
	counts, docLen := TokenCounts(doc)
	cs.DocCount++
	cs.DocLengths += docLen
	cs.AvgDocLen = float64(cs.DocLengths) / float64(cs.DocCount)
	for term := range counts {
		cs.DocFreqs[term]++
	}
}

// IDF computes the Robertson-Spärck Jones probabilistic inverse document frequency with +1 smoothing:
// IDF(t) = ln(1 + (N - df(t) + 0.5) / (df(t) + 0.5))
// This formulation ensures non-negative scores for any document frequency df(t) <= N.
func (cs *CorpusStats) IDF(term string) float64 {
	if cs == nil || cs.DocCount == 0 {
		return 0.0
	}
	term = strings.ToLower(term)
	df := 0
	if cs.DocFreqs != nil {
		df = cs.DocFreqs[term]
	}
	numerator := float64(cs.DocCount-df) + 0.5
	denominator := float64(df) + 0.5
	return math.Log(1.0 + numerator/denominator)
}

// ScoreBM25 computes corpus-aware BM25(D, Q) for query against doc.
// k1 defaults to 1.5 if <= 0; b defaults to 0.75 if <= 0.
func (cs *CorpusStats) ScoreBM25(query, doc string, k1, b float64) float64 {
	if k1 <= 0 {
		k1 = 1.5
	}
	if b <= 0 {
		b = 0.75
	}
	qt := Tokens(query)
	if len(qt) == 0 {
		return 0.0
	}
	counts, dlen := TokenCounts(doc)
	if dlen == 0 {
		return 0.0
	}
	avgdl := float64(dlen)
	if cs != nil && cs.AvgDocLen > 0 {
		avgdl = cs.AvgDocLen
	}
	score := 0.0
	K := k1 * (1.0 - b + b*(float64(dlen)/avgdl))
	for _, t := range qt {
		tf := float64(counts[t])
		if tf <= 0 {
			continue
		}
		idf := 1.0
		if cs != nil {
			idf = cs.IDF(t)
		}
		score += idf * (tf * (k1 + 1.0)) / (tf + K)
	}
	return score
}

// ScoreBM25Plus computes the BM25+ extension (Lv & Zhai, 2011), adding a lower-bound delta to prevent
// over-penalization of long documents with low term frequencies.
// delta defaults to 1.0 if <= 0.
func (cs *CorpusStats) ScoreBM25Plus(query, doc string, k1, b, delta float64) float64 {
	if k1 <= 0 {
		k1 = 1.5
	}
	if b <= 0 {
		b = 0.75
	}
	if delta <= 0 {
		delta = 1.0
	}
	qt := Tokens(query)
	if len(qt) == 0 {
		return 0.0
	}
	counts, dlen := TokenCounts(doc)
	if dlen == 0 {
		return 0.0
	}
	avgdl := float64(dlen)
	if cs != nil && cs.AvgDocLen > 0 {
		avgdl = cs.AvgDocLen
	}
	score := 0.0
	K := k1 * (1.0 - b + b*(float64(dlen)/avgdl))
	for _, t := range qt {
		tf := float64(counts[t])
		if tf <= 0 {
			continue
		}
		idf := 1.0
		if cs != nil {
			idf = cs.IDF(t)
		}
		score += idf * ((tf*(k1+1.0))/(tf+K) + delta)
	}
	return score
}

// ScoreBM25L computes the BM25L extension (Lv & Zhai, 2011), adjusting term frequency for long documents.
// delta defaults to 0.5 if <= 0.
func (cs *CorpusStats) ScoreBM25L(query, doc string, k1, b, delta float64) float64 {
	if k1 <= 0 {
		k1 = 1.5
	}
	if b <= 0 {
		b = 0.75
	}
	if delta <= 0 {
		delta = 0.5
	}
	qt := Tokens(query)
	if len(qt) == 0 {
		return 0.0
	}
	counts, dlen := TokenCounts(doc)
	if dlen == 0 {
		return 0.0
	}
	avgdl := float64(dlen)
	if cs != nil && cs.AvgDocLen > 0 {
		avgdl = cs.AvgDocLen
	}
	norm := 1.0 - b + b*(float64(dlen)/avgdl)
	score := 0.0
	for _, t := range qt {
		tf := float64(counts[t])
		if tf <= 0 {
			continue
		}
		idf := 1.0
		if cs != nil {
			idf = cs.IDF(t)
		}
		cPrime := tf / norm
		if float64(dlen) > avgdl {
			cPrime += delta
		}
		score += idf * (cPrime * (k1 + 1.0)) / (cPrime + k1)
	}
	return score
}

// MaxQueryScore computes the theoretical maximum score attainable by this query under TF saturation.
func (cs *CorpusStats) MaxQueryScore(query string, k1 float64) float64 {
	if k1 <= 0 {
		k1 = 1.5
	}
	qt := Tokens(query)
	maxScore := 0.0
	for _, t := range qt {
		idf := 1.0
		if cs != nil {
			idf = cs.IDF(t)
		}
		maxScore += idf * (k1 + 1.0)
	}
	return maxScore
}
