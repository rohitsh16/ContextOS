package compute

import (
	"testing"
	"time"
)

func TestQueryProfilerRecordAndSnapshot(t *testing.T) {
	qp := NewQueryProfiler(10)
	qp.Reset()

	// Record sample queries
	for i := 1; i <= 5; i++ {
		qp.Record(QueryMetrics{
			TotalDuration: time.Duration(i*10) * time.Millisecond,
			StageDurations: map[QueryStage]time.Duration{
				StageCandidateGen:       time.Duration(i*2) * time.Millisecond,
				StageComputeGraphScores: time.Duration(i*5) * time.Millisecond,
				StageGraphTraversal:     time.Duration(i*1) * time.Millisecond,
			},
			NodesScanned:     i * 10,
			EdgesScanned:     i * 5,
			GraphExpansions:  i * 2,
			SQLRowsReturned:  i * 4,
			CandidatesScored: i * 8,
		})
	}

	snap := qp.Snapshot()
	if snap.TotalQueries != 5 {
		t.Fatalf("expected 5 queries, got %d", snap.TotalQueries)
	}

	if snap.Overall.P50 != 30*time.Millisecond {
		t.Errorf("expected p50 of 30ms, got %v", snap.Overall.P50)
	}

	if snap.Counters["nodes_scanned"] != 150 {
		t.Errorf("expected 150 nodes_scanned, got %d", snap.Counters["nodes_scanned"])
	}

	if snap.Counters["graph_expansions"] != 30 {
		t.Errorf("expected 30 graph_expansions, got %d", snap.Counters["graph_expansions"])
	}

	if snap.AverageCounters["candidates_scored"] != 24.0 {
		t.Errorf("expected avg candidates_scored 24.0, got %f", snap.AverageCounters["candidates_scored"])
	}
}
