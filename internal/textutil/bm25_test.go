package textutil

import (
	"math"
	"sort"
	"testing"
)

func TestCorpusStatsRareTermDominance(t *testing.T) {
	// Known corpus from PR.md:
	// D1: kafka transactional outbox
	// D2: kafka retries
	// D3: postgres transaction
	// D4: unrelated
	docs := []string{
		"kafka transactional outbox",
		"kafka retries",
		"postgres transaction",
		"unrelated system logging",
	}

	cs := NewCorpusStats(docs)
	if cs.DocCount != 4 {
		t.Fatalf("expected DocCount=4, got %d", cs.DocCount)
	}

	// "kafka" appears in D1 and D2 (df=2)
	// "outbox" appears only in D1 (df=1)
	idfKafka := cs.IDF("kafka")
	idfOutbox := cs.IDF("outbox")

	if idfOutbox <= idfKafka {
		t.Errorf("rare term 'outbox' (df=1, IDF=%.4f) must have higher IDF than frequent term 'kafka' (df=2, IDF=%.4f)",
			idfOutbox, idfKafka)
	}

	// For query "kafka outbox":
	// D1 contains both "kafka" and "outbox"
	// D2 contains only "kafka"
	scoreD1 := cs.ScoreBM25("kafka outbox", docs[0], 1.5, 0.75)
	scoreD2 := cs.ScoreBM25("kafka outbox", docs[1], 1.5, 0.75)
	scoreD3 := cs.ScoreBM25("kafka outbox", docs[2], 1.5, 0.75)
	scoreD4 := cs.ScoreBM25("kafka outbox", docs[3], 1.5, 0.75)

	if scoreD1 <= scoreD2 {
		t.Errorf("D1 (both terms) must outrank D2 (one term): D1=%.4f, D2=%.4f", scoreD1, scoreD2)
	}
	if scoreD2 <= scoreD3 {
		t.Errorf("D2 ('kafka' match) must outrank D3 (no match): D2=%.4f, D3=%.4f", scoreD2, scoreD3)
	}
	if scoreD4 != 0.0 {
		t.Errorf("D4 (unrelated) must have score 0, got %.4f", scoreD4)
	}
}

func TestBM25TermFrequencySaturation(t *testing.T) {
	docs := []string{
		"kafka retry error handler",
		"kafka processing pipeline",
		"database storage engine",
	}
	cs := NewCorpusStats(docs)

	doc1 := "kafka message event"
	doc5 := "kafka kafka kafka kafka kafka message event"

	s1 := cs.ScoreBM25("kafka", doc1, 1.5, 0.75)
	s5 := cs.ScoreBM25("kafka", doc5, 1.5, 0.75)

	if s5 <= s1 {
		t.Errorf("5 occurrences (%.4f) should score higher than 1 occurrence (%.4f)", s5, s1)
	}
	// Diminishing returns: score with 5 occurrences must be strictly less than 5x score with 1 occurrence
	if s5 >= 5.0*s1 {
		t.Errorf("TF saturation failed: s5 (%.4f) must exhibit diminishing returns compared to 5 * s1 (%.4f)", s5, 5.0*s1)
	}
}

func TestBM25LengthNormalization(t *testing.T) {
	docs := []string{
		"kafka transaction",
		"kafka transaction outbox pattern message broker architecture documentation and implementation guidelines",
	}
	cs := NewCorpusStats(docs)

	shortDoc := docs[0]
	longDoc := docs[1]

	shortScore := cs.ScoreBM25("kafka transaction", shortDoc, 1.5, 0.75)
	longScore := cs.ScoreBM25("kafka transaction", longDoc, 1.5, 0.75)

	if shortScore <= longScore {
		t.Errorf("concise document (score=%.4f) should score higher than verbose document (score=%.4f) under length normalization",
			shortScore, longScore)
	}
}

func TestBM25EmptyInputs(t *testing.T) {
	cs := NewCorpusStats([]string{"kafka transaction"})

	if s := cs.ScoreBM25("", "kafka transaction", 1.5, 0.75); s != 0 {
		t.Errorf("empty query should score 0, got %.4f", s)
	}
	if s := cs.ScoreBM25("kafka", "", 1.5, 0.75); s != 0 {
		t.Errorf("empty doc should score 0, got %.4f", s)
	}

	emptyCS := NewCorpusStats([]string{})
	if s := emptyCS.ScoreBM25("kafka", "kafka", 1.5, 0.75); s < 0 {
		t.Errorf("empty corpus should handle safely, got %.4f", s)
	}
}

func TestBM25PlusAndBM25L(t *testing.T) {
	docs := []string{
		"kafka transaction outbox",
		"kafka event streaming platform with enterprise partition management and message serialization protocols",
		"redis session storage",
	}
	cs := NewCorpusStats(docs)

	// In BM25+, documents with matches receive a lower-bound delta boost.
	bm25Score := cs.ScoreBM25("kafka", docs[1], 1.5, 0.75)
	bm25PlusScore := cs.ScoreBM25Plus("kafka", docs[1], 1.5, 0.75, 1.0)

	if bm25PlusScore <= bm25Score {
		t.Errorf("BM25+ (%.4f) should exceed standard BM25 (%.4f) due to delta boost", bm25PlusScore, bm25Score)
	}

	// BM25L avoids over-penalizing long documents
	bm25LScore := cs.ScoreBM25L("kafka", docs[1], 1.5, 0.75, 0.5)
	if bm25LScore <= 0 {
		t.Errorf("BM25L score should be positive, got %.4f", bm25LScore)
	}
}

func TestBM25EvaluationMetrics(t *testing.T) {
	// Benchmark matrix comparing lexical retrieval models:
	// Calculate MRR@10, Recall@10, nDCG@10, P@1
	corpus := []string{
		"kafka transactional outbox publisher service",      // D0: Highly relevant to "kafka outbox"
		"kafka retry queue consumer",                        // D1: Partially relevant
		"postgres relational database transactional schema", // D2: Relevant to "transaction"
		"redis in-memory cache layer",                       // D3: Irrelevant
		"kubernetes cluster deployment configuration",       // D4: Irrelevant
	}
	cs := NewCorpusStats(corpus)

	query := "kafka outbox"
	// Ground truth relevance judgments (graded 0 to 2)
	relevance := map[int]float64{
		0: 2.0, // D0
		1: 1.0, // D1
		2: 0.0,
		3: 0.0,
		4: 0.0,
	}

	type ScoredDoc struct {
		Index int
		Score float64
	}

	var ranked []ScoredDoc
	for i, doc := range corpus {
		score := cs.ScoreBM25(query, doc, 1.5, 0.75)
		ranked = append(ranked, ScoredDoc{Index: i, Score: score})
	}

	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].Score > ranked[j].Score
	})

	// Top document must be D0
	if ranked[0].Index != 0 {
		t.Fatalf("expected top document to be D0 (relevance 2), got D%d", ranked[0].Index)
	}

	// P@1
	pAt1 := 0.0
	if relevance[ranked[0].Index] > 0 {
		pAt1 = 1.0
	}
	if pAt1 != 1.0 {
		t.Errorf("P@1 should be 1.0, got %.2f", pAt1)
	}

	// MRR@10
	mrr := 0.0
	for rank, item := range ranked {
		if relevance[item.Index] > 0 {
			mrr = 1.0 / float64(rank+1)
			break
		}
	}
	if mrr != 1.0 {
		t.Errorf("MRR should be 1.0, got %.2f", mrr)
	}

	// nDCG@10
	dcg := 0.0
	for rank, item := range ranked {
		rel := relevance[item.Index]
		dcg += (math.Pow(2, rel) - 1.0) / math.Log2(float64(rank+2))
	}
	// Ideal DCG: sorted by relevance descending (2.0, 1.0, 0, 0, 0)
	idcg := (math.Pow(2, 2) - 1.0)/math.Log2(2) + (math.Pow(2, 1) - 1.0)/math.Log2(3)
	ndcg := dcg / idcg
	if ndcg < 0.95 {
		t.Errorf("nDCG@10 should be close to 1.0, got %.4f", ndcg)
	}
}
