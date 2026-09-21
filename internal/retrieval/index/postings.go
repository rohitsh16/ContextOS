package index

import (
	"encoding/binary"
	"sort"
)

// DefaultBlockSize is the number of postings per block for Block-Max pruning.
const DefaultBlockSize = 64

// Posting represents an occurrence of a term in a document/node.
type Posting struct {
	DocID     uint32
	TermFreq  uint32
	Positions []uint32
	Score     float64
}

// PostingBlock represents a fixed-size partition of a posting list with an upper bound score.
type PostingBlock struct {
	MinDocID uint32
	MaxDocID uint32
	MaxScore float64
	Count    int
	Postings []Posting
}

// PostingList represents a sorted sequence of postings for a specific term or trigram.
type PostingList struct {
	Term     string
	Postings []Posting
	Blocks   []PostingBlock
	MaxScore float64
}

// NewPostingList creates a new PostingList from postings, sorting them by DocID and partitioning into blocks.
func NewPostingList(term string, postings []Posting) *PostingList {
	if len(postings) == 0 {
		return &PostingList{Term: term}
	}

	// Sort postings by DocID
	sorted := make([]Posting, len(postings))
	copy(sorted, postings)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].DocID < sorted[j].DocID
	})

	var globalMaxScore float64
	for _, p := range sorted {
		if p.Score > globalMaxScore {
			globalMaxScore = p.Score
		}
	}

	// Partition into blocks
	numBlocks := (len(sorted) + DefaultBlockSize - 1) / DefaultBlockSize
	blocks := make([]PostingBlock, 0, numBlocks)

	for i := 0; i < len(sorted); i += DefaultBlockSize {
		end := i + DefaultBlockSize
		if end > len(sorted) {
			end = len(sorted)
		}
		chunk := sorted[i:end]

		var blockMax float64
		for _, p := range chunk {
			if p.Score > blockMax {
				blockMax = p.Score
			}
		}

		blocks = append(blocks, PostingBlock{
			MinDocID: chunk[0].DocID,
			MaxDocID: chunk[len(chunk)-1].DocID,
			MaxScore: blockMax,
			Count:    len(chunk),
			Postings: chunk,
		})
	}

	return &PostingList{
		Term:     term,
		Postings: sorted,
		Blocks:   blocks,
		MaxScore: globalMaxScore,
	}
}

// PostingIterator provides streaming cursor traversal over a PostingList with Block-Max skipping support.
type PostingIterator struct {
	list       *PostingList
	currIndex  int
	currBlock  int
}

// NewIterator creates a new PostingIterator for the given PostingList.
func (pl *PostingList) NewIterator() *PostingIterator {
	return &PostingIterator{
		list:      pl,
		currIndex: 0,
		currBlock: 0,
	}
}

// Valid returns true if the iterator currently points to a valid posting.
func (it *PostingIterator) Valid() bool {
	return it.currIndex < len(it.list.Postings)
}

// DocID returns the DocID of the current posting.
func (it *PostingIterator) DocID() uint32 {
	if !it.Valid() {
		return ^uint32(0) // Max uint32
	}
	return it.list.Postings[it.currIndex].DocID
}

// Score returns the score of the current posting.
func (it *PostingIterator) Score() float64 {
	if !it.Valid() {
		return 0.0
	}
	return it.list.Postings[it.currIndex].Score
}

// Posting returns the full Posting struct at the current cursor.
func (it *PostingIterator) Posting() Posting {
	if !it.Valid() {
		return Posting{}
	}
	return it.list.Postings[it.currIndex]
}

// Next advances the iterator to the next posting.
func (it *PostingIterator) Next() {
	if it.currIndex < len(it.list.Postings) {
		it.currIndex++
		it.syncBlock()
	}
}

// CurrentBlockMaxScore returns the maximum score achievable in the current block.
func (it *PostingIterator) CurrentBlockMaxScore() float64 {
	if it.currBlock < len(it.list.Blocks) {
		return it.list.Blocks[it.currBlock].MaxScore
	}
	return 0.0
}

// GlobalMaxScore returns the maximum score achievable in the entire posting list.
func (it *PostingIterator) GlobalMaxScore() float64 {
	return it.list.MaxScore
}

// Seek advances the iterator to the first posting with DocID >= targetDocID.
// It leverages block boundaries to skip blocks whose MaxDocID < targetDocID.
func (it *PostingIterator) Seek(targetDocID uint32) {
	if !it.Valid() || it.DocID() >= targetDocID {
		return
	}

	// Skip blocks whose MaxDocID < targetDocID
	for it.currBlock < len(it.list.Blocks) && it.list.Blocks[it.currBlock].MaxDocID < targetDocID {
		it.currIndex += it.list.Blocks[it.currBlock].Count - (it.currIndex % DefaultBlockSize)
		it.currBlock++
	}

	// Scan remaining postings within the current block or beyond
	for it.currIndex < len(it.list.Postings) && it.list.Postings[it.currIndex].DocID < targetDocID {
		it.currIndex++
	}
	it.syncBlock()
}

// SkipCurrentBlock advances the iterator past all postings in the current block.
func (it *PostingIterator) SkipCurrentBlock() {
	if it.currBlock >= len(it.list.Blocks) {
		it.currIndex = len(it.list.Postings)
		return
	}
	remainingInBlock := it.list.Blocks[it.currBlock].Count - (it.currIndex % DefaultBlockSize)
	if remainingInBlock <= 0 {
		remainingInBlock = DefaultBlockSize
	}
	it.currIndex += remainingInBlock
	it.currBlock++
	if it.currIndex > len(it.list.Postings) {
		it.currIndex = len(it.list.Postings)
	}
}

func (it *PostingIterator) syncBlock() {
	it.currBlock = it.currIndex / DefaultBlockSize
}

// EncodeGaps performs variable-length delta encoding on sorted docIDs.
func EncodeGaps(docIDs []uint32) []byte {
	if len(docIDs) == 0 {
		return nil
	}
	buf := make([]byte, 0, len(docIDs)*2)
	var prev uint32
	var tmp [binary.MaxVarintLen32]byte

	for _, id := range docIDs {
		gap := id - prev
		prev = id
		n := binary.PutUvarint(tmp[:], uint64(gap))
		buf = append(buf, tmp[:n]...)
	}
	return buf
}

// DecodeGaps decodes a variable-length delta-encoded byte slice back into sorted docIDs.
func DecodeGaps(buf []byte, count int) []uint32 {
	if len(buf) == 0 || count == 0 {
		return nil
	}
	res := make([]uint32, 0, count)
	var prev uint32
	offset := 0

	for offset < len(buf) && len(res) < count {
		val, n := binary.Uvarint(buf[offset:])
		if n <= 0 {
			break
		}
		offset += n
		curr := prev + uint32(val)
		res = append(res, curr)
		prev = curr
	}
	return res
}
