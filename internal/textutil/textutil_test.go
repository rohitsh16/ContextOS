package textutil

import (
	"math"
	"testing"
)

// ─── BM25Score ───────────────────────────────────────────────────────────────

func TestBM25FullMatchAtAverageLength(t *testing.T) {
	// When doc length == avgDocLen and all query terms present, score should be ~1.0.
	score := BM25Score("kafka transaction", "kafka transaction", 0)
	if math.Abs(score-1.0) > 0.01 {
		t.Errorf("full match at average length: want ~1.0, got %.4f", score)
	}
}

func TestBM25ZeroOverlap(t *testing.T) {
	score := BM25Score("kafka transaction", "redis caching session layer", 0)
	if score != 0 {
		t.Errorf("zero overlap should return 0, got %.4f", score)
	}
}

func TestBM25PartialMatch(t *testing.T) {
	// 1 of 2 query terms present.
	score := BM25Score("kafka transaction", "kafka latency dashboard", 0)
	if score <= 0 || score >= 1 {
		t.Errorf("partial match should be in (0,1), got %.4f", score)
	}
	full := BM25Score("kafka transaction", "kafka transaction outbox", 0)
	if score >= full {
		t.Errorf("partial match (%.4f) should be less than full match (%.4f)", score, full)
	}
}

func TestBM25LengthNormalizationPenalizesLongDocs(t *testing.T) {
	// Short doc with full match should score >= long doc with full match,
	// when both are compared against a small avgDocLen.
	shortScore := BM25Score("kafka transaction", "kafka transaction", 2.0)
	longScore := BM25Score("kafka transaction",
		"kafka transaction is a distributed system mechanism that supports atomicity across brokers and databases in microservices",
		2.0)
	if shortScore < longScore {
		t.Errorf("short precise doc should score ≥ long verbose doc: short=%.4f long=%.4f", shortScore, longScore)
	}
}

func TestBM25EmptyQuery(t *testing.T) {
	if BM25Score("", "kafka transaction", 0) != 0 {
		t.Error("empty query should return 0")
	}
}

func TestBM25EmptyDoc(t *testing.T) {
	if BM25Score("kafka transaction", "", 0) != 0 {
		t.Error("empty doc should return 0")
	}
}

func TestBM25BothEmpty(t *testing.T) {
	if BM25Score("", "", 0) != 0 {
		t.Error("both empty should return 0")
	}
}

func TestBM25NeverExceedsOne(t *testing.T) {
	// With a very short avgDocLen, per-term score can exceed 1.0 before capping.
	score := BM25Score("kafka transaction outbox retry pattern", "kafka transaction outbox retry pattern", 0.5)
	if score > 1.0+1e-9 {
		t.Errorf("BM25Score must not exceed 1.0, got %.6f", score)
	}
}

func TestBM25ScoreIsNonNegative(t *testing.T) {
	cases := [][2]string{
		{"kafka", "redis"},
		{"", "kafka"},
		{"kafka", ""},
		{"kafka transaction", "kafka transaction outbox pattern retry exactly once"},
	}
	for _, c := range cases {
		s := BM25Score(c[0], c[1], 0)
		if s < 0 {
			t.Errorf("BM25Score(%q, %q) returned negative: %.4f", c[0], c[1], s)
		}
	}
}

func TestBM25AvgDocLenZeroUsesDocLength(t *testing.T) {
	// When avgDocLen=0, defaults to doc's own length → score=1 for full match.
	s := BM25Score("kafka", "kafka", 0)
	if math.Abs(s-1.0) > 0.01 {
		t.Errorf("avgDocLen=0 full match should be ~1.0, got %.4f", s)
	}
}

// ─── HashSemantic ────────────────────────────────────────────────────────────

func TestHashSemanticSelfSimilarity(t *testing.T) {
	s := "kafka transaction outbox pattern for exactly once delivery"
	score := HashSemantic(s, s)
	if score < 0.99 {
		t.Errorf("self-similarity should be ~1.0, got %.4f", score)
	}
}

func TestHashSemanticRelatedBeatsUnrelated(t *testing.T) {
	task := "kafka transaction retry"
	related := "kafka retry mechanism for transaction delivery"
	unrelated := "css flexbox grid layout responsive design"
	rel := HashSemantic(task, related)
	unrel := HashSemantic(task, unrelated)
	if rel <= unrel {
		t.Errorf("related (%.4f) should score higher than unrelated (%.4f)", rel, unrel)
	}
}

func TestHashSemanticIsSymmetric(t *testing.T) {
	a, b := "kafka transaction", "outbox pattern delivery"
	forward := HashSemantic(a, b)
	backward := HashSemantic(b, a)
	if math.Abs(forward-backward) > 1e-9 {
		t.Errorf("HashSemantic should be symmetric: forward=%.6f backward=%.6f", forward, backward)
	}
}

func TestHashSemanticEmptyString(t *testing.T) {
	if HashSemantic("", "kafka transaction") != 0 {
		t.Error("empty query should return 0")
	}
	if HashSemantic("kafka transaction", "") != 0 {
		t.Error("empty doc should return 0")
	}
}

func TestHashSemanticInRange(t *testing.T) {
	cases := [][2]string{
		{"kafka", "kafka transaction outbox"},
		{"payment processing", "fraud detection ml model"},
		{"go channel goroutine", "go channel goroutine"},
	}
	for _, c := range cases {
		s := HashSemantic(c[0], c[1])
		if s < 0 || s > 1+1e-9 {
			t.Errorf("HashSemantic(%q,%q)=%.6f out of [0,1]", c[0], c[1], s)
		}
	}
}

// ─── Overlap ────────────────────────────────────────────────────────────────

func TestOverlapFullMatch(t *testing.T) {
	// All query terms present in doc.
	score := Overlap("kafka transaction", "kafka transaction outbox")
	if math.Abs(score-1.0) > 0.01 {
		t.Errorf("full match should be 1.0, got %.4f", score)
	}
}

func TestOverlapPartialMatch(t *testing.T) {
	// 2 of 3 query terms present.
	score := Overlap("kafka transaction retry", "kafka transaction")
	want := 2.0 / 3.0
	if math.Abs(score-want) > 0.01 {
		t.Errorf("2/3 overlap: want %.4f, got %.4f", want, score)
	}
}

func TestOverlapZeroMatch(t *testing.T) {
	if Overlap("kafka transaction", "redis caching") != 0 {
		t.Error("no common terms should give 0")
	}
}

func TestOverlapEmptyQuery(t *testing.T) {
	if Overlap("", "kafka transaction") != 0 {
		t.Error("empty query should return 0")
	}
}

func TestOverlapEmptyDoc(t *testing.T) {
	if Overlap("kafka transaction", "") != 0 {
		t.Error("empty doc should return 0")
	}
}

func TestOverlapCaseInsensitive(t *testing.T) {
	lower := Overlap("kafka", "kafka transaction")
	upper := Overlap("KAFKA", "KAFKA TRANSACTION")
	if math.Abs(lower-upper) > 1e-9 {
		t.Errorf("overlap should be case-insensitive: lower=%.4f upper=%.4f", lower, upper)
	}
}

func TestOverlapDeduplicatesTokens(t *testing.T) {
	// Repeated tokens in query should not artificially inflate score.
	deduped := Overlap("kafka kafka kafka", "kafka")
	single := Overlap("kafka", "kafka")
	if math.Abs(deduped-single) > 1e-9 {
		t.Errorf("repeated query tokens should be deduped: deduped=%.4f single=%.4f", deduped, single)
	}
}

// ─── EstimateTokens ──────────────────────────────────────────────────────────

func TestEstimateTokensEmpty(t *testing.T) {
	if EstimateTokens("") != 1 {
		t.Errorf("empty string should return 1 (minimum), got %d", EstimateTokens(""))
	}
}

func TestEstimateTokensSingleWord(t *testing.T) {
	got := EstimateTokens("kafka")
	// 1 word → int(1*1.3+0.999) = int(2.299) = 2
	if got != 2 {
		t.Errorf("single word: want 2, got %d", got)
	}
}

func TestEstimateTokensFourWords(t *testing.T) {
	got := EstimateTokens("kafka transaction outbox pattern")
	// 4 words → int(4*1.3+0.999) = int(6.199) = 6
	if got != 6 {
		t.Errorf("four words: want 6, got %d", got)
	}
}

func TestEstimateTokensProportional(t *testing.T) {
	short := EstimateTokens("kafka")
	long := EstimateTokens("kafka transaction outbox pattern retry exactly once delivery guarantee")
	if long <= short {
		t.Errorf("longer text should produce higher token estimate: short=%d long=%d", short, long)
	}
}

func TestEstimateTokensNeverNegative(t *testing.T) {
	cases := []string{"", " ", "\t", "kafka", "a b c d e f g h i j"}
	for _, s := range cases {
		n := EstimateTokens(s)
		if n <= 0 {
			t.Errorf("EstimateTokens(%q) = %d, want > 0", s, n)
		}
	}
}

// ─── Tokens ─────────────────────────────────────────────────────────────────

func TestTokensFiltersShortWords(t *testing.T) {
	toks := Tokens("a kafka b transaction")
	for _, t2 := range toks {
		if len(t2) < 2 {
			t.Errorf("token %q shorter than 2 chars should be filtered", t2)
		}
	}
}

func TestTokensDeduplicates(t *testing.T) {
	toks := Tokens("kafka kafka kafka transaction transaction")
	seen := map[string]int{}
	for _, t2 := range toks {
		seen[t2]++
		if seen[t2] > 1 {
			t.Errorf("token %q appears more than once", t2)
		}
	}
}

func TestTokensLowercases(t *testing.T) {
	toks := Tokens("KAFKA Transaction OUTBOX")
	for _, t2 := range toks {
		for _, ch := range t2 {
			if ch >= 'A' && ch <= 'Z' {
				t.Errorf("token %q contains uppercase", t2)
			}
		}
	}
}
