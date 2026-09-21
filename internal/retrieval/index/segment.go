package index

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"
)

// Segment represents an index segment containing postings and metadata (PR.md Section 11).
type Segment struct {
	mu            sync.RWMutex
	SegmentID     string                  `json:"segment_id"`
	Generation    uint64                  `json:"generation"`
	RevisionRange string                  `json:"revision_range"`
	NodeCount     int                     `json:"node_count"`
	ByteSize      int64                   `json:"byte_size"`
	Checksum      uint64                  `json:"checksum"`
	Immutable     bool                    `json:"immutable"`
	Terms         map[string]*PostingList `json:"-"`
	Tombstones    map[uint32]bool         `json:"-"`
}

// NewSegment creates a new mutable delta segment.
func NewSegment(id string, generation uint64) *Segment {
	return &Segment{
		SegmentID:  id,
		Generation: generation,
		Immutable:  false,
		Terms:      make(map[string]*PostingList),
		Tombstones: make(map[uint32]bool),
	}
}

// AddPosting appends a posting to the segment for a given term.
func (s *Segment) AddPosting(term string, p Posting) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Immutable {
		return
	}

	pl, exists := s.Terms[term]
	if !exists {
		pl = NewPostingList(term, []Posting{p})
		s.Terms[term] = pl
	} else {
		// Append to postings and rebuild blocks
		newPostings := append(pl.Postings, p)
		s.Terms[term] = NewPostingList(term, newPostings)
	}
	s.NodeCount++
	s.ByteSize += int64(len(term) + 16)
}

// MarkTombstone marks a docID as deleted.
func (s *Segment) MarkTombstone(docID uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Tombstones[docID] = true
}

// IsTombstoned checks if a docID is deleted.
func (s *Segment) IsTombstoned(docID uint32) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Tombstones[docID]
}

// Freeze marks the segment as immutable and computes its checksum.
func (s *Segment) Freeze() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Immutable = true
	h := sha256.New()
	for term, pl := range s.Terms {
		h.Write([]byte(term))
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], uint64(len(pl.Postings)))
		h.Write(buf[:])
	}
	sum := h.Sum(nil)
	s.Checksum = binary.LittleEndian.Uint64(sum[:8])
}

// Manifest represents the atomic state of active segments (PR.md Section 11).
type Manifest struct {
	Generation uint64     `json:"generation"`
	BaseIDs    []string   `json:"base_ids"`
	DeltaID    string     `json:"delta_id"`
}

// SegmentManager coordinates immutable base segments and an active delta segment.
type SegmentManager struct {
	mu         sync.RWMutex
	base       []*Segment
	delta      *Segment
	generation uint64
	docCounter uint32
}

// NewSegmentManager initializes a SegmentManager.
func NewSegmentManager() *SegmentManager {
	return &SegmentManager{
		base:       nil,
		delta:      NewSegment("delta_0", 1),
		generation: 1,
	}
}

// NextDocID atomically generates a monotonic docID.
func (sm *SegmentManager) NextDocID() uint32 {
	return atomic.AddUint32(&sm.docCounter, 1)
}

// IndexTerm indexes a term posting into the current delta segment.
func (sm *SegmentManager) IndexTerm(term string, p Posting) {
	sm.mu.RLock()
	delta := sm.delta
	sm.mu.RUnlock()

	delta.AddPosting(term, p)
}

// DeleteDoc records a tombstone across active segments.
func (sm *SegmentManager) DeleteDoc(docID uint32) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sm.delta.MarkTombstone(docID)
	for _, b := range sm.base {
		b.MarkTombstone(docID)
	}
}

// SearchTerm retrieves iterators for a term across base and delta segments, filtering tombstones.
func (sm *SegmentManager) SearchTerm(term string) []*PostingIterator {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var iterators []*PostingIterator

	// Delta segment
	if sm.delta != nil {
		sm.delta.mu.RLock()
		if pl, ok := sm.delta.Terms[term]; ok {
			iterators = append(iterators, pl.NewIterator())
		}
		sm.delta.mu.RUnlock()
	}

	// Base segments
	for _, b := range sm.base {
		b.mu.RLock()
		if pl, ok := b.Terms[term]; ok {
			iterators = append(iterators, pl.NewIterator())
		}
		b.mu.RUnlock()
	}

	return iterators
}

// Compact merges the mutable delta segment into a new immutable base segment.
func (sm *SegmentManager) Compact() (*Segment, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.delta == nil || len(sm.delta.Terms) == 0 {
		return nil, fmt.Errorf("no delta to compact")
	}

	oldDelta := sm.delta
	oldDelta.Freeze()

	// Merge all terms from oldDelta and existing base segments
	mergedTerms := make(map[string][]Posting)

	// Collect from existing base
	for _, b := range sm.base {
		b.mu.RLock()
		for term, pl := range b.Terms {
			for _, p := range pl.Postings {
				if !b.IsTombstoned(p.DocID) && !oldDelta.IsTombstoned(p.DocID) {
					mergedTerms[term] = append(mergedTerms[term], p)
				}
			}
		}
		b.mu.RUnlock()
	}

	// Collect from delta
	oldDelta.mu.RLock()
	for term, pl := range oldDelta.Terms {
		for _, p := range pl.Postings {
			if !oldDelta.IsTombstoned(p.DocID) {
				mergedTerms[term] = append(mergedTerms[term], p)
			}
		}
	}
	oldDelta.mu.RUnlock()

	// Construct new base segment
	sm.generation++
	newBaseID := fmt.Sprintf("base_gen_%d", sm.generation)
	newBase := NewSegment(newBaseID, sm.generation)

	for term, postings := range mergedTerms {
		newBase.Terms[term] = NewPostingList(term, postings)
		newBase.NodeCount += len(postings)
		newBase.ByteSize += int64(len(term) + len(postings)*16)
	}
	newBase.Freeze()

	// Atomic update: replace base with newBase, create fresh delta
	sm.base = []*Segment{newBase}
	sm.delta = NewSegment(fmt.Sprintf("delta_%d", sm.generation), sm.generation)

	return newBase, nil
}
