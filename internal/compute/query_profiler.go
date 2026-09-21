package compute

import (
	"math"
	"sort"
	"sync"
	"time"
)

// QueryStage represents a distinct phase of planning/retrieval execution.
type QueryStage string

const (
	StageListNodes          QueryStage = "list_nodes"
	StageComputeGraphScores QueryStage = "compute_graph_scores"
	StageCandidateGen       QueryStage = "candidate_generation"
	StageGraphTraversal     QueryStage = "graph_traversal"
	StageSorting            QueryStage = "sorting"
	StageSQLiteQueries      QueryStage = "sqlite_queries"
	StageSerialization      QueryStage = "serialization"
)

// QueryMetrics captures telemetry and timings for a single query execution.
type QueryMetrics struct {
	Timestamp time.Time `json:"timestamp"`
	TotalDuration time.Duration `json:"total_duration"`

	// Stage durations
	StageDurations map[QueryStage]time.Duration `json:"stage_durations"`

	// Counters
	NodesScanned      int `json:"nodes_scanned"`
	EdgesScanned      int `json:"edges_scanned"`
	GraphExpansions   int `json:"graph_expansions"`
	SQLRowsReturned   int `json:"sql_rows_returned"`
	CandidatesScored  int `json:"candidates_scored"`
}

// PercentileStats records p50, p95, and p99 statistics for a duration.
type PercentileStats struct {
	Count int           `json:"count"`
	Min   time.Duration `json:"min"`
	P50   time.Duration `json:"p50"`
	P95   time.Duration `json:"p95"`
	P99   time.Duration `json:"p99"`
	Max   time.Duration `json:"max"`
	Mean  time.Duration `json:"mean"`
}

// QueryProfileSnapshot captures aggregated p50/p95/p99 percentiles and counter totals.
type QueryProfileSnapshot struct {
	TotalQueries int                                `json:"total_queries"`
	Overall      PercentileStats                    `json:"overall"`
	Stages       map[QueryStage]PercentileStats     `json:"stages"`
	Counters     map[string]int64                   `json:"counters"`
	AverageCounters map[string]float64              `json:"average_counters"`
}

// QueryProfiler is a thread-safe ring-buffer telemetry collector.
type QueryProfiler struct {
	mu          sync.RWMutex
	capacity    int
	samples     []QueryMetrics
	counterTotals map[string]int64
}

// DefaultProfilerCapacity is the default size of the rolling window.
const DefaultProfilerCapacity = 500

var (
	globalProfiler     *QueryProfiler
	globalProfilerOnce sync.Once
)

// GlobalProfiler returns the shared global profiler instance.
func GlobalProfiler() *QueryProfiler {
	globalProfilerOnce.Do(func() {
		globalProfiler = NewQueryProfiler(DefaultProfilerCapacity)
	})
	return globalProfiler
}

// NewQueryProfiler creates a new profiler with the given ring-buffer capacity.
func NewQueryProfiler(capacity int) *QueryProfiler {
	if capacity <= 0 {
		capacity = DefaultProfilerCapacity
	}
	return &QueryProfiler{
		capacity:      capacity,
		samples:       make([]QueryMetrics, 0, capacity),
		counterTotals: make(map[string]int64),
	}
}

// Record records a completed query metric sample.
func (qp *QueryProfiler) Record(m QueryMetrics) {
	qp.mu.Lock()
	defer qp.mu.Unlock()

	if m.Timestamp.IsZero() {
		m.Timestamp = time.Now().UTC()
	}

	if len(qp.samples) >= qp.capacity {
		qp.samples = qp.samples[1:]
	}
	qp.samples = append(qp.samples, m)

	qp.counterTotals["nodes_scanned"] += int64(m.NodesScanned)
	qp.counterTotals["edges_scanned"] += int64(m.EdgesScanned)
	qp.counterTotals["graph_expansions"] += int64(m.GraphExpansions)
	qp.counterTotals["sql_rows_returned"] += int64(m.SQLRowsReturned)
	qp.counterTotals["candidates_scored"] += int64(m.CandidatesScored)
}

// Snapshot computes and returns p50/p95/p99 percentiles and cumulative counters.
func (qp *QueryProfiler) Snapshot() QueryProfileSnapshot {
	qp.mu.RLock()
	defer qp.mu.RUnlock()

	total := len(qp.samples)
	snap := QueryProfileSnapshot{
		TotalQueries:    total,
		Stages:          make(map[QueryStage]PercentileStats),
		Counters:        make(map[string]int64),
		AverageCounters: make(map[string]float64),
	}

	for k, v := range qp.counterTotals {
		snap.Counters[k] = v
		if total > 0 {
			snap.AverageCounters[k] = float64(v) / float64(total)
		}
	}

	if total == 0 {
		return snap
	}

	totals := make([]time.Duration, total)
	stageMap := make(map[QueryStage][]time.Duration)

	for i, s := range qp.samples {
		totals[i] = s.TotalDuration
		for stage, dur := range s.StageDurations {
			stageMap[stage] = append(stageMap[stage], dur)
		}
	}

	snap.Overall = calculatePercentiles(totals)
	for stage, durs := range stageMap {
		snap.Stages[stage] = calculatePercentiles(durs)
	}

	return snap
}

// Reset clears all collected telemetry samples.
func (qp *QueryProfiler) Reset() {
	qp.mu.Lock()
	defer qp.mu.Unlock()
	qp.samples = make([]QueryMetrics, 0, qp.capacity)
	qp.counterTotals = make(map[string]int64)
}

func calculatePercentiles(durs []time.Duration) PercentileStats {
	n := len(durs)
	if n == 0 {
		return PercentileStats{}
	}

	sorted := make([]time.Duration, n)
	copy(sorted, durs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	var sum time.Duration
	for _, d := range sorted {
		sum += d
	}

	return PercentileStats{
		Count: n,
		Min:   sorted[0],
		P50:   percentile(sorted, 50),
		P95:   percentile(sorted, 95),
		P99:   percentile(sorted, 99),
		Max:   sorted[n-1],
		Mean:  time.Duration(int64(sum) / int64(n)),
	}
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}
	rank := (p / 100.0) * float64(len(sorted)-1)
	idx := int(math.Round(rank))
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
