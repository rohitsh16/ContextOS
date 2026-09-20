package bench

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"contextos/internal/graph"
	"contextos/internal/model"
	"contextos/internal/textutil"
)

// BenchmarkConfig specifies configuration options for running scientific benchmarks.
type BenchmarkConfig struct {
	Seed       int64          `json:"seed"`
	NumTasks   int            `json:"num_tasks"`
	Budget     int            `json:"budget"`
	Baselines  []BaselineID   `json:"baselines,omitempty"`
	Ablations  AblationConfig `json:"ablations"`
	Pricing    ModelPricing   `json:"pricing"`
	Alpha      float64        `json:"alpha"` // default 0.05
	Confidence float64        `json:"confidence"` // default 0.95
}

// DefaultConfig returns standard scientific benchmark configuration.
func DefaultConfig() BenchmarkConfig {
	return BenchmarkConfig{
		Seed:       42,
		NumTasks:   100,
		Budget:     2048,
		Baselines:  AllBaselines,
		Ablations:  DefaultAblations(),
		Pricing:    DefaultPricing(),
		Alpha:      0.05,
		Confidence: 0.95,
	}
}

// BaselineReport contains the aggregated scientific layers and distributions for a single baseline.
type BaselineReport struct {
	BaselineID       BaselineID         `json:"baseline_id"`
	Metrics          LayerMetrics       `json:"metrics"`
	WilsonSuccessCI  ConfidenceInterval `json:"wilson_success_ci"`
	SuccessPerTask   []float64          `json:"-"`
	TokensPerTask    []float64          `json:"-"`
	CostPerTask      []float64          `json:"-"`
	LatencyPerTask   []float64          `json:"-"`
}

// ComparisonReport captures paired comparative statistical tests between a candidate and baseline.
type ComparisonReport struct {
	CandidateID        BaselineID         `json:"candidate_id"`
	BaselineID         BaselineID         `json:"baseline_id"`
	Metric             string             `json:"metric"`
	MeanDelta          float64            `json:"mean_delta"`
	MedianDelta        float64            `json:"median_delta"`
	BootstrapCI        ConfidenceInterval `json:"bootstrap_ci_95"`
	PermutationPValue  float64            `json:"permutation_p_value"`
	CliffsDelta        float64            `json:"cliffs_delta"`
	EffectSize         string             `json:"effect_size"`
	StatisticallyValid bool               `json:"statistically_valid"`
}

// SweepPoint captures multi-metric outcomes at a specific context token budget.
type SweepPoint struct {
	Budget         int                           `json:"budget"`
	Results        map[BaselineID]LayerMetrics   `json:"results"`
	ParetoFrontier []ParetoPoint                 `json:"pareto_frontier"`
}

// BenchmarkSuiteReport bundles the complete scientific benchmarking results.
type BenchmarkSuiteReport struct {
	Timestamp      string                 `json:"timestamp"`
	Config         BenchmarkConfig        `json:"config"`
	Baselines      map[string]BaselineReport `json:"baselines"`
	Comparisons    []ComparisonReport     `json:"comparisons"`
	Hypotheses     []HypothesisResult     `json:"hypotheses"`
	ParetoFrontier []ParetoPoint          `json:"pareto_frontier"`
	Sweep          []SweepPoint           `json:"sweep,omitempty"`
	PowerAnalysis  map[string]int         `json:"power_analysis_sample_sizes"`
}

// GenerateSyntheticCorpus creates realistic benchmark tasks with ground truth relevancy,
// distractors, stale memories with git commit diffs, and graph relations.
func GenerateSyntheticCorpus(seed int64, numTasks int) []TaskContext {
	r := rand.New(rand.NewSource(seed))
	tasks := make([]TaskContext, numTasks)

	serviceNames := []string{"payment", "auth", "billing", "inventory", "search", "notification", "ledger", "order"}
	frameworkTopics := []string{
		"kafka transaction retry with outbox",
		"grpc deadline propagation and context cancellation",
		"distributed lock release timeout in redis",
		"database migration lock and deadlock prevention",
		"jwt signature verification and token rotation",
		"idempotency key cache collision under concurrency",
	}

	for i := 0; i < numTasks; i++ {
		svc := serviceNames[r.Intn(len(serviceNames))]
		topic := frameworkTopics[r.Intn(len(frameworkTopics))]
		taskQuery := fmt.Sprintf("resolve %s service %s issue in worktree", svc, topic)

		// Ground-truth relevant memories (authority user/test/source)
		m1 := model.Memory{
			ID:                fmt.Sprintf("rel_%d_1", i),
			Kind:              "decision",
			Content:           fmt.Sprintf("Architectural requirement: Use transactional outbox pattern for %s service kafka events", svc),
			Authority:         "user",
			Confidence:        0.98,
			TokenCost:         140,
			Scope:             fmt.Sprintf("services/%s/kafka.go", svc),
			Locations:         []string{fmt.Sprintf("services/%s/kafka.go", svc)},
			ValidFromRevision: "rev-active",
		}
		m2 := model.Memory{
			ID:                fmt.Sprintf("rel_%d_2", i),
			Kind:              "failure",
			Content:           fmt.Sprintf("Known outage postmortem: Direct %s db commit without outbox caused duplicate events on network retry", svc),
			Authority:         "test",
			Confidence:        0.99,
			TokenCost:         160,
			Scope:             fmt.Sprintf("services/%s/store.go", svc),
			Locations:         []string{fmt.Sprintf("services/%s/store.go", svc)},
			ValidFromRevision: "rev-active",
		}
		m3 := model.Memory{
			ID:                fmt.Sprintf("rel_%d_3", i),
			Kind:              "constraint",
			Content:           fmt.Sprintf("Consistency invariant: %s service database and message broker do not share two-phase commit", svc),
			Authority:         "source",
			Confidence:        0.95,
			TokenCost:         120,
			Scope:             fmt.Sprintf("services/%s/main.go", svc),
			Locations:         []string{fmt.Sprintf("services/%s/main.go", svc)},
			ValidFromRevision: "rev-active",
		}

		// Contradictory / Stale memory that must be rejected by temporal scoping
		staleFile := fmt.Sprintf("services/%s/deprecated_lock.go", svc)
		mStale := model.Memory{
			ID:                    fmt.Sprintf("stale_%d", i),
			Kind:                  "decision",
			Content:               fmt.Sprintf("Old deprecated rule: Use redis distributed lock in %s service before publishing event", svc),
			Authority:             "inference",
			Confidence:            0.70,
			TokenCost:             130,
			Scope:                 staleFile,
			Locations:             []string{staleFile},
			ValidFromRevision:     "rev-old-1234",
			InvalidatedAtRevision: "rev-active",
		}

		// Distractors / Irrelevant noise memories
		distractorSvc := serviceNames[(r.Intn(len(serviceNames)-1)+1)%len(serviceNames)]
		mDistractor1 := model.Memory{
			ID:                fmt.Sprintf("dist_%d_1", i),
			Kind:              "fact",
			Content:           fmt.Sprintf("Frontend CSS design tokens for %s dashboard navigation bar", distractorSvc),
			Authority:         "inference",
			Confidence:        0.50,
			TokenCost:         650,
			Scope:             fmt.Sprintf("web/%s/nav.css", distractorSvc),
			Locations:         []string{fmt.Sprintf("web/%s/nav.css", distractorSvc)},
			ValidFromRevision: "rev-active",
		}
		mDistractor2 := model.Memory{
			ID:                fmt.Sprintf("dist_%d_2", i),
			Kind:              "fact",
			Content:           fmt.Sprintf("Marketing analytics cookie consent banner configuration for %s portal", distractorSvc),
			Authority:         "doc",
			Confidence:        0.60,
			TokenCost:         500,
			Scope:             fmt.Sprintf("web/%s/consent.json", distractorSvc),
			Locations:         []string{fmt.Sprintf("web/%s/consent.json", distractorSvc)},
			ValidFromRevision: "rev-active",
		}

		memories := []model.Memory{m1, m2, m3, mStale, mDistractor1, mDistractor2}

		// Shuffle memories to simulate unordered retrieval pool
		r.Shuffle(len(memories), func(a, b int) {
			memories[a], memories[b] = memories[b], memories[a]
		})

		// Corpus stats for BM25
		docs := make([]string, len(memories))
		for j, m := range memories {
			docs[j] = m.Content
		}
		stats := textutil.NewCorpusStats(docs)

		// Graph relations
		g := graph.New(graph.DefaultConfig())
		g.AddNode(&graph.Node{ID: "main"})
		g.AddNode(&graph.Node{ID: fmt.Sprintf("services/%s/main.go", svc)})
		g.AddNode(&graph.Node{ID: fmt.Sprintf("services/%s/kafka.go", svc)})
		g.AddNode(&graph.Node{ID: fmt.Sprintf("services/%s/store.go", svc)})
		g.AddEdge(fmt.Sprintf("services/%s/main.go", svc), fmt.Sprintf("services/%s/kafka.go", svc), "call")
		g.AddEdge(fmt.Sprintf("services/%s/kafka.go", svc), fmt.Sprintf("services/%s/store.go", svc), "call")
		g.AddEdge("main", fmt.Sprintf("services/%s/main.go", svc), "call")

		// Changed files diff in active revision (deprecated_lock.go was deleted "D")
		changedFiles := map[string]string{
			staleFile: "D",
			fmt.Sprintf("services/%s/kafka.go", svc): "M",
		}

		tasks[i] = TaskContext{
			ID:           fmt.Sprintf("task_%d", i),
			Task:         taskQuery,
			Query:        taskQuery,
			Memories:     memories,
			RelevantIDs:  map[string]bool{m1.ID: true, m2.ID: true, m3.ID: true},
			StaleIDs:     map[string]bool{mStale.ID: true},
			Revision:     "rev-active",
			ChangedFiles: changedFiles,
			Graph:        g,
			CorpusStats:  stats,
		}
	}
	return tasks
}

// Run executes the scientific benchmark across the configured tasks and baselines.
func Run(cfg BenchmarkConfig) BenchmarkSuiteReport {
	if cfg.NumTasks <= 0 {
		cfg.NumTasks = 100
	}
	if cfg.Budget <= 0 {
		cfg.Budget = 2048
	}
	if len(cfg.Baselines) == 0 {
		cfg.Baselines = AllBaselines
	}
	if cfg.Confidence <= 0 {
		cfg.Confidence = 0.95
	}
	if cfg.Alpha <= 0 {
		cfg.Alpha = 0.05
	}

	tasks := GenerateSyntheticCorpus(cfg.Seed, cfg.NumTasks)
	baselineReports := make(map[string]BaselineReport)

	for _, bid := range cfg.Baselines {
		report := evaluateBaselineOnTasks(bid, tasks, cfg.Budget, cfg.Ablations, cfg.Pricing, cfg.Confidence)
		baselineReports[string(bid)] = report
	}

	// Paired comparison tests between B9 (research system) and other baselines
	var comparisons []ComparisonReport
	pValues := make(map[string]float64)

	b9Report, hasB9 := baselineReports[string(B9FullContextOS)]
	if hasB9 {
		for _, bid := range cfg.Baselines {
			if bid == B9FullContextOS {
				continue
			}
			otherReport, ok := baselineReports[string(bid)]
			if !ok {
				continue
			}

			// Task success paired test
			deltas := PairedDelta(b9Report.SuccessPerTask, otherReport.SuccessPerTask)
			summary := ComputeSummary(deltas)
			bCI := BootstrapCI(deltas, 1000, cfg.Confidence, cfg.Seed)
			pVal := PairedPermutationTest(deltas, 2000, cfg.Seed)
			d, interp := CliffsDelta(deltas)

			compKey := fmt.Sprintf("%s vs %s [task_success]", B9FullContextOS, bid)
			pValues[compKey] = pVal

			comparisons = append(comparisons, ComparisonReport{
				CandidateID:        B9FullContextOS,
				BaselineID:         bid,
				Metric:             "task_success",
				MeanDelta:          summary.Mean,
				MedianDelta:        summary.Median,
				BootstrapCI:        bCI,
				PermutationPValue:  pVal,
				CliffsDelta:        d,
				EffectSize:         interp,
				StatisticallyValid: pVal <= cfg.Alpha,
			})
		}
	}

	// Holm-Bonferroni correction across multiple hypothesis tests
	hypotheses := HolmBonferroni(pValues, cfg.Alpha)

	// Pareto analysis across all evaluated baselines at the configured budget
	var paretoPoints []ParetoPoint
	for bidStr, rep := range baselineReports {
		paretoPoints = append(paretoPoints, ParetoPoint{
			Name:      bidStr,
			Budget:    cfg.Budget,
			Success:   rep.Metrics.C.TaskSuccess,
			Cost:      rep.Metrics.D.EstimatedCost,
			Tokens:    rep.Metrics.B.TokenUsage,
			LatencyMs: rep.Metrics.D.LatencyMs,
		})
	}
	paretoFrontier := ComputeParetoFrontier(paretoPoints)

	// Power analysis sample sizes for detecting various effect sizes
	powerSampleSizes := map[string]int{
		"detect_delta_0.05_power_0.80": PowerAnalysis(0.05, 0.25, cfg.Alpha, 0.80),
		"detect_delta_0.05_power_0.90": PowerAnalysis(0.05, 0.25, cfg.Alpha, 0.90),
		"detect_delta_0.10_power_0.80": PowerAnalysis(0.10, 0.25, cfg.Alpha, 0.80),
		"detect_delta_0.10_power_0.90": PowerAnalysis(0.10, 0.25, cfg.Alpha, 0.90),
	}

	return BenchmarkSuiteReport{
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Config:         cfg,
		Baselines:      baselineReports,
		Comparisons:    comparisons,
		Hypotheses:     hypotheses,
		ParetoFrontier: paretoFrontier,
		PowerAnalysis:  powerSampleSizes,
	}
}

// BudgetSweep executes all baselines across multiple budget levels to map the quality-cost Pareto frontier.
func BudgetSweep(cfg BenchmarkConfig, budgets []int) []SweepPoint {
	if len(budgets) == 0 {
		budgets = []int{512, 1024, 2048, 4096, 8192, 16384, 32768}
	}
	tasks := GenerateSyntheticCorpus(cfg.Seed, cfg.NumTasks)
	var sweepPoints []SweepPoint

	for _, b := range budgets {
		results := make(map[BaselineID]LayerMetrics)
		var points []ParetoPoint
		for _, bid := range cfg.Baselines {
			rep := evaluateBaselineOnTasks(bid, tasks, b, cfg.Ablations, cfg.Pricing, cfg.Confidence)
			results[bid] = rep.Metrics
			points = append(points, ParetoPoint{
				Name:      string(bid),
				Budget:    b,
				Success:   rep.Metrics.C.TaskSuccess,
				Cost:      rep.Metrics.D.EstimatedCost,
				Tokens:    rep.Metrics.B.TokenUsage,
				LatencyMs: rep.Metrics.D.LatencyMs,
			})
		}
		frontier := ComputeParetoFrontier(points)
		sweepPoints = append(sweepPoints, SweepPoint{
			Budget:         b,
			Results:        results,
			ParetoFrontier: frontier,
		})
	}
	return sweepPoints
}

func evaluateBaselineOnTasks(bid BaselineID, tasks []TaskContext, budget int, ablations AblationConfig, pricing ModelPricing, confidence float64) BaselineReport {
	n := len(tasks)
	var (
		sumR1, sumR3, sumR5, sumR10 float64
		sumMRR, sumNDCG, sumMAP      float64
		sumTokens, sumCoverage       float64
		sumRedundancy, sumUtility    float64
		sumSuccess, sumTestPass      float64
		sumRegressions               float64
		sumInputTok, sumCachedTok    float64
		sumUncachedTok, sumOutputTok float64
		sumLatency, sumCost          float64
	)

	successes := make([]float64, n)
	tokens := make([]float64, n)
	costs := make([]float64, n)
	latencies := make([]float64, n)

	successCount := 0

	for i, tc := range tasks {
		start := time.Now()
		exec := ExecuteBaseline(bid, tc, budget, ablations)
		elapsedMs := float64(time.Since(start).Nanoseconds()) / 1e6
		if exec.DurationMs > 0 {
			elapsedMs = exec.DurationMs
		}

		// Layer A: Retrieval
		r1 := ComputeRecallAtK(exec.RankedIDs, tc.RelevantIDs, 1)
		r3 := ComputeRecallAtK(exec.RankedIDs, tc.RelevantIDs, 3)
		r5 := ComputeRecallAtK(exec.RankedIDs, tc.RelevantIDs, 5)
		r10 := ComputeRecallAtK(exec.RankedIDs, tc.RelevantIDs, 10)
		mrr := ComputeMRR(exec.RankedIDs, tc.RelevantIDs)
		ndcg := ComputeNDCG(exec.RankedIDs, tc.RelevantIDs, 10)
		mapVal := ComputeMAP(exec.RankedIDs, tc.RelevantIDs)

		sumR1 += r1
		sumR3 += r3
		sumR5 += r5
		sumR10 += r10
		sumMRR += mrr
		sumNDCG += ndcg
		sumMAP += mapVal

		// Layer B: Selection
		toks := float64(exec.SelectedTokens)
		cov := 0.0
		if len(tc.RelevantIDs) > 0 {
			cov = float64(exec.FactsCovered) / float64(len(tc.RelevantIDs))
		}
		red := ComputeRedundancy(exec.Selected)
		util := 0.0
		for _, c := range exec.Selected {
			util += c.Score
		}

		sumTokens += toks
		sumCoverage += cov
		sumRedundancy += red
		sumUtility += util

		// Layer C: Agent Outcome
		// Task succeeds if all ground-truth facts are present and no regression was introduced
		taskSucc := 0.0
		if exec.FactsCovered == len(tc.RelevantIDs) && exec.Regressions == 0 {
			taskSucc = 1.0
			successCount++
		} else if exec.FactsCovered > 0 && exec.Regressions == 0 {
			// partial success proportional to coverage
			taskSucc = cov * 0.7
		}
		testPass := cov
		if exec.Regressions > 0 {
			testPass = math.Max(0.0, testPass-0.5)
		}

		sumSuccess += taskSucc
		sumTestPass += testPass
		if exec.Regressions > 0 {
			sumRegressions += 1.0
		}

		// Layer D: Economics
		inTok := toks
		caTok := float64(exec.CachedTokens)
		unTok := inTok - caTok
		if unTok < 0 {
			unTok = 0
		}
		outTok := 150.0 // average completion tokens for code resolution
		cost := pricing.EstimateCostUSD(unTok, caTok, outTok)

		sumInputTok += inTok
		sumCachedTok += caTok
		sumUncachedTok += unTok
		sumOutputTok += outTok
		sumLatency += elapsedMs
		sumCost += cost

		successes[i] = taskSucc
		tokens[i] = toks
		costs[i] = cost
		latencies[i] = elapsedMs
	}

	fn := float64(n)
	avgTokens := sumTokens / fn
	budgUtil := 0.0
	if budget > 0 {
		budgUtil = avgTokens / float64(budget)
	}

	metrics := LayerMetrics{
		A: LayerA{
			RecallAt1:  sumR1 / fn,
			RecallAt3:  sumR3 / fn,
			RecallAt5:  sumR5 / fn,
			RecallAt10: sumR10 / fn,
			MRR:        sumMRR / fn,
			NDCG:       sumNDCG / fn,
			MAP:        sumMAP / fn,
		},
		B: LayerB{
			TokenUsage:        avgTokens,
			Coverage:          sumCoverage / fn,
			Redundancy:        sumRedundancy / fn,
			Utility:           sumUtility / fn,
			BudgetUtilization: budgUtil,
		},
		C: LayerC{
			TaskSuccess:     sumSuccess / fn,
			TestPass:        sumTestPass / fn,
			RegressionRate:  sumRegressions / fn,
			HandoffSuccess:  math.Min(1.0, (sumSuccess/fn)*1.05),
			RediscoveryRate: math.Max(0.0, 1.0-(sumCoverage/fn)),
		},
		D: LayerD{
			InputTokens:    sumInputTok / fn,
			CachedTokens:   sumCachedTok / fn,
			UncachedTokens: sumUncachedTok / fn,
			OutputTokens:   sumOutputTok / fn,
			LatencyMs:      sumLatency / fn,
			EstimatedCost:  sumCost / fn,
		},
	}

	wilson := WilsonScoreInterval(successCount, n, confidence)

	return BaselineReport{
		BaselineID:      bid,
		Metrics:         metrics,
		WilsonSuccessCI: wilson,
		SuccessPerTask:  successes,
		TokensPerTask:   tokens,
		CostPerTask:     costs,
		LatencyPerTask:  latencies,
	}
}
