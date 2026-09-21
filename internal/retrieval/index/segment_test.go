package index

import (
	"testing"
)

func TestSegmentCompactionAndTombstones(t *testing.T) {
	sm := NewSegmentManager()

	doc1 := sm.NextDocID()
	doc2 := sm.NextDocID()
	doc3 := sm.NextDocID()

	// Add postings to delta
	sm.IndexTerm("router", Posting{DocID: doc1, Score: 5.0})
	sm.IndexTerm("router", Posting{DocID: doc2, Score: 3.0})
	sm.IndexTerm("allocator", Posting{DocID: doc3, Score: 8.0})

	// Search before compaction
	iters := sm.SearchTerm("router")
	if len(iters) != 1 {
		t.Fatalf("expected 1 iterator before compaction, got %d", len(iters))
	}
	if iters[0].Posting().DocID != doc1 {
		t.Fatalf("expected doc1 first, got %d", iters[0].Posting().DocID)
	}

	// Compact delta into immutable base
	baseSeg, err := sm.Compact()
	if err != nil {
		t.Fatalf("compaction failed: %v", err)
	}
	if !baseSeg.Immutable {
		t.Errorf("base segment should be immutable")
	}
	if baseSeg.Checksum == 0 {
		t.Errorf("expected non-zero checksum")
	}

	// Delete doc1 via tombstone
	sm.DeleteDoc(doc1)

	// Add new posting to new delta
	doc4 := sm.NextDocID()
	sm.IndexTerm("router", Posting{DocID: doc4, Score: 9.0})

	// Search after compaction: base has doc1 (tombstoned) and doc2; delta has doc4
	afterIters := sm.SearchTerm("router")
	if len(afterIters) != 2 { // 1 delta, 1 base
		t.Fatalf("expected 2 iterators, got %d", len(afterIters))
	}

	// Verify tombstone is recorded
	if !sm.base[0].IsTombstoned(doc1) {
		t.Errorf("expected doc1 to be tombstoned in base segment")
	}
}

func TestShardRouterBoundedSearch(t *testing.T) {
	sr := NewShardRouter(2)

	shardA := sr.GetOrCreateShard("pkgA")
	shardB := sr.GetOrCreateShard("pkgB")

	docA := shardA.RegisterNode("symA")
	docB := shardB.RegisterNode("symB")

	shardA.SegmentMgr.IndexTerm("handler", Posting{DocID: docA, Score: 10.0})
	shardB.SegmentMgr.IndexTerm("handler", Posting{DocID: docB, Score: 8.0})

	// Search across shards with scope targeting pkgA
	targets := sr.RouteShards([]string{"pkgA"}, nil)
	if len(targets) != 1 || targets[0].Package != "pkgA" {
		t.Fatalf("expected routing only to pkgA, got %v", targets)
	}

	results, stats := sr.SearchAcrossShards([]string{"handler"}, targets, 5)
	if len(results) != 1 || len(results[0].Results) != 1 {
		t.Fatalf("expected 1 result from pkgA, got %v", results)
	}
	if results[0].Results[0].DocID != docA {
		t.Errorf("expected docA, got %d", results[0].Results[0].DocID)
	}
	if stats.PostingsVisited == 0 {
		t.Errorf("expected postings visited > 0")
	}
}
