package index

import (
	"container/heap"
	"sort"
)

// ScoredDoc represents an evaluated candidate document/node.
type ScoredDoc struct {
	DocID uint32
	Score float64
}

// PruningStats tracks execution metrics for dynamic pruning algorithms (PR.md Section 9).
type PruningStats struct {
	PostingsVisited    int
	BlocksSkipped      int
	SuperblocksSkipped int
	CandidatesScored   int
}

// TopKHeap maintains the running top-K candidates as a min-heap.
type TopKHeap []ScoredDoc

func (h TopKHeap) Len() int           { return len(h) }
func (h TopKHeap) Less(i, j int) bool { return h[i].Score < h[j].Score }
func (h TopKHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *TopKHeap) Push(x any) {
	*h = append(*h, x.(ScoredDoc))
}

func (h *TopKHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[0 : n-1]
	return item
}

// Threshold returns the minimum score required to enter the top-K heap.
func (h TopKHeap) Threshold(k int) float64 {
	if len(h) < k {
		return 0.0
	}
	return h[0].Score
}

// Insert adds a candidate to the top-K heap if it beats the current threshold.
func (h *TopKHeap) Insert(doc ScoredDoc, k int) {
	if k <= 0 {
		return
	}
	if len(*h) < k {
		heap.Push(h, doc)
	} else if doc.Score > (*h)[0].Score {
		(*h)[0] = doc
		heap.Fix(h, 0)
	}
}

// ToSortedSlice returns candidates sorted in descending order of score.
func (h TopKHeap) ToSortedSlice() []ScoredDoc {
	res := make([]ScoredDoc, len(h))
	copy(res, h)
	sort.Slice(res, func(i, j int) bool {
		return res[i].Score > res[j].Score
	})
	return res
}

// SuperBlockSize is the number of blocks grouped into a single superblock.
const SuperBlockSize = 8

// BlockMaxWAND implements Phase 5.3 and 5.4 Block-Max WAND dynamic top-K pruning (PR.md Section 9).
// It searches across posting iterators, skipping blocks whose maximum achievable score cannot beat theta.
func BlockMaxWAND(iterators []*PostingIterator, k int) ([]ScoredDoc, PruningStats) {
	var stats PruningStats
	if len(iterators) == 0 || k <= 0 {
		return nil, stats
	}

	// Filter out exhausted iterators
	active := make([]*PostingIterator, 0, len(iterators))
	for _, it := range iterators {
		if it.Valid() {
			active = append(active, it)
		}
	}

	h := &TopKHeap{}
	heap.Init(h)

	for len(active) > 0 {
		// Sort active iterators by their current DocID
		sort.Slice(active, func(i, j int) bool {
			return active[i].DocID() < active[j].DocID()
		})

		theta := h.Threshold(k)

		// Accumulate block upper-bounds to find the pivot
		var accum float64
		pivotIdx := -1

		for i, it := range active {
			accum += it.CurrentBlockMaxScore()
			if accum > theta {
				pivotIdx = i
				break
			}
		}

		// If total block-max accumulation across all iterators cannot beat threshold, terminate early!
		if pivotIdx == -1 {
			// All remaining blocks across all iterators are provably incapable of beating theta
			for _, it := range active {
				stats.BlocksSkipped += len(it.list.Blocks) - it.currBlock
			}
			break
		}

		pivotDocID := active[pivotIdx].DocID()

		// If first iterator matches pivotDocID, evaluate full score for this document
		if active[0].DocID() == pivotDocID {
			var score float64
			stats.CandidatesScored++

			// Sum scores across all iterators positioned at pivotDocID
			for _, it := range active {
				if it.DocID() == pivotDocID {
					score += it.Score()
					stats.PostingsVisited++
					it.Next()
				}
			}

			h.Insert(ScoredDoc{DocID: pivotDocID, Score: score}, k)
		} else {
			// Superblock & Block skipping: check if current block / superblock can be skipped
			firstIt := active[0]
			if firstIt.CurrentBlockMaxScore() <= theta && firstIt.list.Blocks[firstIt.currBlock].MaxDocID < pivotDocID {
				// We can skip the current block
				stats.BlocksSkipped++
				firstIt.SkipCurrentBlock()
				firstIt.Seek(pivotDocID)
			} else {
				// Regular Seek
				firstIt.Seek(pivotDocID)
			}
		}

		// Prune exhausted iterators
		n := 0
		for _, it := range active {
			if it.Valid() {
				active[n] = it
				n++
			}
		}
		active = active[:n]
	}

	return h.ToSortedSlice(), stats
}

// MaxScore implements Phase 5.1 MaxScore pruning algorithm (PR.md Section 9.1).
func MaxScore(iterators []*PostingIterator, k int) ([]ScoredDoc, PruningStats) {
	var stats PruningStats
	if len(iterators) == 0 || k <= 0 {
		return nil, stats
	}

	// Sort iterators by GlobalMaxScore ascending: iterators[0] has smallest max score
	sort.Slice(iterators, func(i, j int) bool {
		return iterators[i].GlobalMaxScore() < iterators[j].GlobalMaxScore()
	})

	// Precompute prefix sums of max scores: prefix[i] = sum(GlobalMaxScore for j <= i)
	prefixMax := make([]float64, len(iterators))
	var sum float64
	for i, it := range iterators {
		sum += it.GlobalMaxScore()
		prefixMax[i] = sum
	}

	h := &TopKHeap{}
	heap.Init(h)

	// Essential list boundary: iterators with index >= essentialIdx must be evaluated
	essentialIdx := 0

	for {
		theta := h.Threshold(k)

		// Advance essentialIdx: while prefixMax[essentialIdx] <= theta, lists 0..essentialIdx are non-essential
		for essentialIdx < len(iterators) && prefixMax[essentialIdx] <= theta {
			essentialIdx++
		}
		if essentialIdx >= len(iterators) {
			// Even the highest-scoring list cannot beat theta alone
			break
		}

		// Find minimum DocID among essential iterators
		var minDocID uint32 = ^uint32(0)
		for i := essentialIdx; i < len(iterators); i++ {
			if iterators[i].Valid() && iterators[i].DocID() < minDocID {
				minDocID = iterators[i].DocID()
			}
		}

		if minDocID == ^uint32(0) {
			// All essential iterators exhausted
			break
		}

		// Score minDocID
		var score float64
		stats.CandidatesScored++

		// Accumulate from essential iterators
		for i := essentialIdx; i < len(iterators); i++ {
			if iterators[i].Valid() && iterators[i].DocID() == minDocID {
				score += iterators[i].Score()
				stats.PostingsVisited++
				iterators[i].Next()
			}
		}

		// Check non-essential iterators only if partial score + remaining max could beat theta
		for i := essentialIdx - 1; i >= 0; i-- {
			if score+prefixMax[i] <= theta {
				// Pruned! Cannot beat theta
				stats.BlocksSkipped++
				break
			}
			iterators[i].Seek(minDocID)
			if iterators[i].Valid() && iterators[i].DocID() == minDocID {
				score += iterators[i].Score()
				stats.PostingsVisited++
			}
		}

		h.Insert(ScoredDoc{DocID: minDocID, Score: score}, k)
	}

	return h.ToSortedSlice(), stats
}
