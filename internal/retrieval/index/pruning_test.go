package index

import (
	"fmt"
	"math/rand"
	"testing"
)

func TestEncodeDecodeGaps(t *testing.T) {
	docIDs := []uint32{5, 12, 120, 300, 301, 1000, 50000}
	encoded := EncodeGaps(docIDs)
	decoded := DecodeGaps(encoded, len(docIDs))

	if len(decoded) != len(docIDs) {
		t.Fatalf("expected length %d, got %d", len(docIDs), len(decoded))
	}
	for i := range docIDs {
		if decoded[i] != docIDs[i] {
			t.Errorf("at index %d: expected %d, got %d", i, docIDs[i], decoded[i])
		}
	}
}

func TestBlockMaxWANDCorrectness(t *testing.T) {
	r := rand.New(rand.NewSource(42))

	// Create 5 terms with postings across 1000 docs
	numTerms := 5
	lists := make([]*PostingList, numTerms)

	for termIdx := 0; termIdx < numTerms; termIdx++ {
		var postings []Posting
		for docID := uint32(1); docID <= 1000; docID++ {
			if r.Float64() < 0.3 { // 30% density
				score := r.Float64() * 10.0
				postings = append(postings, Posting{
					DocID: docID,
					Score: score,
				})
			}
		}
		lists[termIdx] = NewPostingList(fmt.Sprintf("term_%d", termIdx), postings)
	}

	// Exhaustive Ground Truth
	docScores := make(map[uint32]float64)
	for _, l := range lists {
		for _, p := range l.Postings {
			docScores[p.DocID] += p.Score
		}
	}

	k := 10

	// Run BlockMaxWAND
	var iters []*PostingIterator
	for _, l := range lists {
		iters = append(iters, l.NewIterator())
	}
	bmwResults, stats := BlockMaxWAND(iters, k)

	if len(bmwResults) != k {
		t.Fatalf("expected %d results, got %d", k, len(bmwResults))
	}

	// Verify top-1 score matches ground truth maximum
	var maxTruth float64
	var maxDocID uint32
	for id, s := range docScores {
		if s > maxTruth {
			maxTruth = s
			maxDocID = id
		}
	}

	if bmwResults[0].DocID != maxDocID {
		t.Errorf("top-1 doc mismatch: expected %d (score %f), got %d (score %f)",
			maxDocID, maxTruth, bmwResults[0].DocID, bmwResults[0].Score)
	}

	if stats.PostingsVisited == 0 {
		t.Errorf("expected postings visited > 0")
	}

	// Verify MaxScore
	var msIters []*PostingIterator
	for _, l := range lists {
		msIters = append(msIters, l.NewIterator())
	}
	msResults, msStats := MaxScore(msIters, k)

	if len(msResults) != k {
		t.Fatalf("MaxScore: expected %d results, got %d", k, len(msResults))
	}
	if msResults[0].DocID != maxDocID {
		t.Errorf("MaxScore top-1 doc mismatch: expected %d, got %d", maxDocID, msResults[0].DocID)
	}
	if msStats.PostingsVisited == 0 {
		t.Errorf("expected MaxScore postings visited > 0")
	}
}
