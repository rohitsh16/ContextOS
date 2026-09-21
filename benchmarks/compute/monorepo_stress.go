package compute_bench

import (
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"contextos/internal/gitidx"
	"contextos/internal/server"
	"contextos/internal/store"
)

// MonorepoQueryClass classifies stress test query scenarios.
type MonorepoQueryClass string

const (
	StressHubSymbol      MonorepoQueryClass = "hub_symbol"       // High-degree hub node (500+ edges)
	StressDeepChain      MonorepoQueryClass = "deep_chain"       // Multi-hop transitive dependency chain
	StressMultiSymbol    MonorepoQueryClass = "multi_symbol"     // Multiple candidate symbols in query
	StressColdPlan       MonorepoQueryClass = "cold_plan"        // Unique query bypassing cache
	StressAdversarial    MonorepoQueryClass = "adversarial_miss" // Non-existent symbol testing fast fallback
)

// MonorepoStressQuery defines a test query with expected properties.
type MonorepoStressQuery struct {
	Class MonorepoQueryClass
	Query string
	Scope string
}

// MonorepoStressConfig configures the synthetic monorepo scale and stress parameters.
type MonorepoStressConfig struct {
	NodeCount    int           // e.g. 10000, 50000, 100000
	EdgeCount    int           // e.g. 50000, 250000, 500000
	Workers      int           // e.g. 4, 8, 16, 32 concurrent goroutines
	TotalQueries int           // e.g. 200, 500, 1000
	TimeoutSLA   time.Duration // Claude Code hook SLA (default: 5000ms)
	Seed         int64
}

// MonorepoStressResult aggregates measured stress test metrics.
type MonorepoStressResult struct {
	NodeCount      int                                  `json:"node_count"`
	EdgeCount      int                                  `json:"edge_count"`
	Workers        int                                  `json:"workers"`
	TotalQueries   int                                  `json:"total_queries"`
	SuccessCount   int                                  `json:"success_count"`
	ErrorCount     int                                  `json:"error_count"`
	TimeoutCount   int                                  `json:"timeout_count"` // Queries > TimeoutSLA
	WarningCount   int                                  `json:"warning_count"` // Queries > 100ms
	P50Latency     time.Duration                        `json:"p50_latency"`
	P90Latency     time.Duration                        `json:"p90_latency"`
	P95Latency     time.Duration                        `json:"p95_latency"`
	P99Latency     time.Duration                        `json:"p99_latency"`
	MaxLatency     time.Duration                        `json:"max_latency"`
	ThroughputQPS  float64                              `json:"throughput_qps"`
	ClassLatencies map[MonorepoQueryClass]time.Duration `json:"class_p95_latencies"`
}

// GenerateScaleFreeMonorepo creates a synthetic monorepo in the store with power-law degree distribution.
func GenerateScaleFreeMonorepo(st store.Store, repoPath string, nodeCount, edgeCount int, seed int64) (string, []MonorepoStressQuery, error) {
	if nodeCount <= 0 {
		nodeCount = 10000
	}
	if edgeCount <= 0 {
		edgeCount = nodeCount * 5
	}
	if seed == 0 {
		seed = 42
	}
	r := rand.New(rand.NewSource(seed))

	repoID, err := st.GetOrCreateRepo(repoPath, "synthetic-monorepo", "rev_monorepo", "main", "wt_monorepo")
	if err != nil {
		return "", nil, fmt.Errorf("get or create repo: %w", err)
	}

	packages := []string{
		"core", "api", "auth", "billing", "storage", "compute",
		"telemetry", "worker", "gateway", "router", "models",
		"consensus", "scheduler", "kvstore", "pipeline", "executor",
		"cluster", "network", "security", "search",
	}

	types := []string{
		"Client", "Manager", "Service", "Handler", "Dispatcher",
		"Validator", "Transformer", "Cache", "Coordinator", "Router",
		"Controller", "Registry", "Engine", "Session", "Connector",
	}

	verbs := []string{
		"Process", "Validate", "Dispatch", "Execute", "Verify",
		"Synchronize", "Reconcile", "Compress", "Encrypt", "Serialize",
		"Lookup", "Register", "Evict", "Subscribe", "Publish",
	}

	numFiles := nodeCount / 5
	if numFiles < 20 {
		numFiles = 20
	}

	files := make([]gitidx.SourceFile, 0, numFiles)
	syms := make([]gitidx.Symbol, 0, nodeCount)

	// Create files and symbols
	for i := 0; i < numFiles; i++ {
		pkg := packages[r.Intn(len(packages))]
		path := fmt.Sprintf("pkg/%s/component_%d.go", pkg, i)
		files = append(files, gitidx.SourceFile{
			Path:  path,
			Hash:  fmt.Sprintf("hash_file_%d", i),
			Lines: 200,
		})

		// 1 struct type per file
		tName := fmt.Sprintf("%s%d", types[r.Intn(len(types))], i)
		syms = append(syms, gitidx.Symbol{
			Path:      path,
			Name:      tName,
			Kind:      "type",
			Start:     15,
			End:       30,
			Signature: fmt.Sprintf("type %s struct", tName),
		})

		// 4 methods per type
		for j := 0; j < 4; j++ {
			fName := fmt.Sprintf("%s%s%d", verbs[r.Intn(len(verbs))], types[r.Intn(len(types))], i*10+j)
			syms = append(syms, gitidx.Symbol{
				Path:      path,
				Name:      fName,
				Kind:      "function",
				Start:     35 + j*40,
				End:       70 + j*40,
				Signature: fmt.Sprintf("func (c *%s) %s(ctx context.Context) error", tName, fName),
			})
		}
	}

	// Generate scale-free edges (Barabasi-Albert preferential attachment model)
	edges := make([]store.EdgeRecord, 0, edgeCount)
	totalNodes := len(syms)

	// Designate top 10 nodes as core super-hubs
	hubCount := 10
	if hubCount > totalNodes {
		hubCount = totalNodes
	}

	for e := 0; e < edgeCount; e++ {
		srcIdx := r.Intn(totalNodes)
		var dstIdx int

		// 60% probability of connecting to a super-hub (power-law preferential attachment)
		if r.Float64() < 0.60 {
			dstIdx = r.Intn(hubCount)
		} else {
			dstIdx = r.Intn(totalNodes)
		}

		if srcIdx == dstIdx {
			continue
		}

		edgeKinds := []string{"calls", "imports", "implements", "references"}
		edges = append(edges, store.EdgeRecord{
			SrcID: fmt.Sprintf("node_%d", srcIdx),
			DstID: fmt.Sprintf("node_%d", dstIdx),
			Kind:  edgeKinds[r.Intn(len(edgeKinds))],
		})
	}

	// Persist all nodes and edges (O3 index-time feature calculation automatically runs here)
	if err := st.SaveNodesAndEdges(repoID, files, syms, edges); err != nil {
		return "", nil, fmt.Errorf("save nodes and edges: %w", err)
	}

	// Formulate stress query corpus
	var queries []MonorepoStressQuery

	// 1. Super-hub queries (target symbols with massive in-degree)
	for h := 0; h < hubCount; h++ {
		queries = append(queries, MonorepoStressQuery{
			Class: StressHubSymbol,
			Query: fmt.Sprintf("find references and callers of %s in core", syms[h].Name),
			Scope: "pkg/core",
		})
	}

	// 2. Deep transitive chain queries
	for i := 0; i < 20; i++ {
		target := syms[r.Intn(totalNodes)].Name
		queries = append(queries, MonorepoStressQuery{
			Class: StressDeepChain,
			Query: fmt.Sprintf("trace dependency path from %s to database storage", target),
		})
	}

	// 3. Multi-symbol queries
	for i := 0; i < 20; i++ {
		s1 := syms[r.Intn(totalNodes)].Name
		s2 := syms[r.Intn(totalNodes)].Name
		queries = append(queries, MonorepoStressQuery{
			Class: StressMultiSymbol,
			Query: fmt.Sprintf("integrate %s with %s service", s1, s2),
		})
	}

	// 4. Cold plan queries (unique strings)
	for i := 0; i < 30; i++ {
		queries = append(queries, MonorepoStressQuery{
			Class: StressColdPlan,
			Query: fmt.Sprintf("refactor error handling in worker pipeline task_%d_%d", i, r.Int63()),
		})
	}

	// 5. Adversarial zero-match queries
	for i := 0; i < 15; i++ {
		queries = append(queries, MonorepoStressQuery{
			Class: StressAdversarial,
			Query: fmt.Sprintf("nonexistent_symbol_%d_xyz_not_found_anywhere", i),
		})
	}

	return repoID, queries, nil
}

// RunMonorepoStress executes concurrent stress queries against the ContextOS Service.
func RunMonorepoStress(svc *server.Service, queries []MonorepoStressQuery, cfg MonorepoStressConfig) (*MonorepoStressResult, error) {
	if cfg.Workers <= 0 {
		cfg.Workers = 8
	}
	if cfg.TotalQueries <= 0 {
		cfg.TotalQueries = len(queries)
	}
	if cfg.TimeoutSLA <= 0 {
		cfg.TimeoutSLA = 5 * time.Second // Claude Code 5-second hook SLA
	}

	queryChan := make(chan MonorepoStressQuery, cfg.TotalQueries)
	for i := 0; i < cfg.TotalQueries; i++ {
		queryChan <- queries[i%len(queries)]
	}
	close(queryChan)

	type queryMeasurement struct {
		class    MonorepoQueryClass
		duration time.Duration
		err      error
	}

	measurements := make([]queryMeasurement, 0, cfg.TotalQueries)
	var mu sync.Mutex

	var successCount int64
	var errorCount int64
	var timeoutCount int64
	var warningCount int64

	startAll := time.Now()
	var wg sync.WaitGroup

	for w := 0; w < cfg.Workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for q := range queryChan {
				t0 := time.Now()
				// Run actual ContextOS planning pipeline (O1-O6)
				_, err := svc.Plan(q.Query, "claude-3-5-sonnet", 2500)
				dur := time.Since(t0)

				if err != nil {
					atomic.AddInt64(&errorCount, 1)
				} else {
					atomic.AddInt64(&successCount, 1)
				}

				if dur > cfg.TimeoutSLA {
					atomic.AddInt64(&timeoutCount, 1)
				}
				if dur > 100*time.Millisecond {
					atomic.AddInt64(&warningCount, 1)
				}

				mu.Lock()
				measurements = append(measurements, queryMeasurement{
					class:    q.Class,
					duration: dur,
					err:      err,
				})
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	totalWallTime := time.Since(startAll)

	if len(measurements) == 0 {
		return nil, fmt.Errorf("no measurements recorded")
	}

	// Sort durations to compute percentiles
	durations := make([]time.Duration, len(measurements))
	classDurations := make(map[MonorepoQueryClass][]time.Duration)

	for i, m := range measurements {
		durations[i] = m.duration
		classDurations[m.class] = append(classDurations[m.class], m.duration)
	}

	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })

	p50 := durations[len(durations)*50/100]
	p90 := durations[len(durations)*90/100]
	p95 := durations[len(durations)*95/100]
	p99 := durations[len(durations)*99/100]
	maxDur := durations[len(durations)-1]

	classP95 := make(map[MonorepoQueryClass]time.Duration)
	for class, durs := range classDurations {
		sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
		idx := len(durs) * 95 / 100
		if idx >= len(durs) {
			idx = len(durs) - 1
		}
		classP95[class] = durs[idx]
	}

	qps := float64(len(measurements)) / totalWallTime.Seconds()

	return &MonorepoStressResult{
		NodeCount:      cfg.NodeCount,
		EdgeCount:      cfg.EdgeCount,
		Workers:        cfg.Workers,
		TotalQueries:   len(measurements),
		SuccessCount:   int(successCount),
		ErrorCount:     int(errorCount),
		TimeoutCount:   int(timeoutCount),
		WarningCount:   int(warningCount),
		P50Latency:     p50,
		P90Latency:     p90,
		P95Latency:     p95,
		P99Latency:     p99,
		MaxLatency:     maxDur,
		ThroughputQPS:  qps,
		ClassLatencies: classP95,
	}, nil
}
