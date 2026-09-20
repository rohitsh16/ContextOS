package bench

import (
	"testing"
)

func TestStatisticalLayers(t *testing.T) {
	t.Run("LayerA_Metrics", func(t *testing.T) {
		ranked := []string{"doc1", "doc2", "doc3", "doc4"}
		relevant := map[string]bool{"doc2": true, "doc4": true}

		r1 := ComputeRecallAtK(ranked, relevant, 1)
		if r1 != 0.0 {
			t.Fatalf("expected Recall@1=0, got %f", r1)
		}
		r2 := ComputeRecallAtK(ranked, relevant, 2)
		if r2 != 0.5 {
			t.Fatalf("expected Recall@2=0.5, got %f", r2)
		}
		mrr := ComputeMRR(ranked, relevant)
		if mrr != 0.5 {
			t.Fatalf("expected MRR=0.5 (doc2 at rank 2), got %f", mrr)
		}
		ndcg := ComputeNDCG(ranked, relevant, 4)
		if ndcg <= 0.0 || ndcg > 1.0 {
			t.Fatalf("unexpected NDCG %f", ndcg)
		}
		mapVal := ComputeMAP(ranked, relevant)
		if mapVal <= 0.0 || mapVal > 1.0 {
			t.Fatalf("unexpected MAP %f", mapVal)
		}
	})

	t.Run("BootstrapCI", func(t *testing.T) {
		deltas := []float64{0.1, 0.2, 0.15, 0.25, 0.18, 0.22, 0.19, 0.21}
		ci := BootstrapCI(deltas, 1000, 0.95, 42)
		if ci.Lower > ci.Upper {
			t.Fatalf("invalid CI bounds: [%f, %f]", ci.Lower, ci.Upper)
		}
		if ci.Lower < 0.05 || ci.Upper > 0.30 {
			t.Fatalf("CI bounds unexpected for mean ~0.19: [%f, %f]", ci.Lower, ci.Upper)
		}
	})

	t.Run("PairedPermutationTest", func(t *testing.T) {
		// Clear positive difference
		deltas := []float64{0.5, 0.6, 0.4, 0.55, 0.45, 0.5, 0.6}
		p := PairedPermutationTest(deltas, 2000, 42)
		if p > 0.05 {
			t.Fatalf("expected significant p-value <= 0.05, got %f", p)
		}

		// Centered at zero
		nullDeltas := []float64{-0.1, 0.1, -0.2, 0.2, -0.05, 0.05}
		pNull := PairedPermutationTest(nullDeltas, 2000, 42)
		if pNull < 0.10 {
			t.Fatalf("expected non-significant p-value for null differences, got %f", pNull)
		}
	})

	t.Run("CliffsDelta", func(t *testing.T) {
		deltas := []float64{0.5, 0.4, 0.3, 0.6}
		d, interp := CliffsDelta(deltas)
		if d != 1.0 || interp != "large" {
			t.Fatalf("expected d=1.0 large, got %f (%s)", d, interp)
		}
	})

	t.Run("HolmBonferroni", func(t *testing.T) {
		pVals := map[string]float64{
			"H1": 0.001,
			"H2": 0.015,
			"H3": 0.040,
			"H4": 0.200,
		}
		res := HolmBonferroni(pVals, 0.05)
		if len(res) != 4 {
			t.Fatalf("expected 4 results, got %d", len(res))
		}
		if res[0].Name != "H1" || !res[0].SignificantDiff {
			t.Fatalf("expected H1 to be rank 1 and significant")
		}
		if res[3].Name != "H4" || res[3].SignificantDiff {
			t.Fatalf("expected H4 to not be significant")
		}
	})

	t.Run("WilsonScoreInterval", func(t *testing.T) {
		ci := WilsonScoreInterval(90, 100, 0.95)
		if ci.Lower < 0.80 || ci.Upper > 0.98 {
			t.Fatalf("unexpected Wilson CI for 90/100: [%f, %f]", ci.Lower, ci.Upper)
		}
	})

	t.Run("ParetoFrontier", func(t *testing.T) {
		p1 := ParetoPoint{Name: "Ideal", Success: 0.95, Cost: 0.01, Tokens: 500, LatencyMs: 1.0}
		p2 := ParetoPoint{Name: "Dominated", Success: 0.80, Cost: 0.05, Tokens: 1000, LatencyMs: 5.0}
		p3 := ParetoPoint{Name: "FastCheap", Success: 0.75, Cost: 0.005, Tokens: 300, LatencyMs: 0.5}

		if !p1.Dominates(p2) {
			t.Fatalf("p1 should dominate p2")
		}
		if p1.Dominates(p3) {
			t.Fatalf("p1 should not dominate p3 (p3 is cheaper/faster)")
		}

		frontier := ComputeParetoFrontier([]ParetoPoint{p1, p2, p3})
		for _, pt := range frontier {
			if pt.Name == "Dominated" && !pt.Dominated {
				t.Fatalf("expected Dominated point to be marked dominated")
			}
			if pt.Name == "Ideal" && pt.Dominated {
				t.Fatalf("expected Ideal point to not be dominated")
			}
		}
	})

	t.Run("PowerAnalysis", func(t *testing.T) {
		n := PowerAnalysis(0.10, 0.25, 0.05, 0.80)
		if n <= 10 || n > 500 {
			t.Fatalf("unexpected required sample size: %d", n)
		}
	})
}

func TestBenchmarkRun(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NumTasks = 15 // fast test
	cfg.Budget = 2048
	cfg.Baselines = []BaselineID{B0FullHistory, B1Recency, B3CurrentContextOS, B9FullContextOS}

	report := Run(cfg)
	if len(report.Baselines) != 4 {
		t.Fatalf("expected 4 baselines, got %d", len(report.Baselines))
	}

	b9, ok := report.Baselines[string(B9FullContextOS)]
	if !ok {
		t.Fatalf("missing B9 in report")
	}
	if b9.Metrics.C.TaskSuccess < 0.5 {
		t.Fatalf("expected high task success for B9, got %f", b9.Metrics.C.TaskSuccess)
	}
	if len(report.Comparisons) == 0 {
		t.Fatalf("expected paired comparisons in report")
	}
}

func TestBudgetSweep(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NumTasks = 5
	cfg.Baselines = []BaselineID{B0FullHistory, B9FullContextOS}
	budgets := []int{512, 2048}

	sweep := BudgetSweep(cfg, budgets)
	if len(sweep) != 2 {
		t.Fatalf("expected 2 sweep points, got %d", len(sweep))
	}
	for _, sp := range sweep {
		if len(sp.ParetoFrontier) == 0 {
			t.Fatalf("empty pareto frontier in sweep at budget %d", sp.Budget)
		}
	}
}

func TestLongitudinalBenchmark(t *testing.T) {
	cfg := DefaultLongitudinalConfig()
	cfg.Generations = 6
	cfg.TasksPerGen = 4
	cfg.Budget = 2048
	cfg.Seed = 42

	report := RunLongitudinalBenchmark(cfg)

	if len(report.ContextOS.Generations) != 6 {
		t.Fatalf("expected 6 generations, got %d", len(report.ContextOS.Generations))
	}

	// ContextOS should have lower rediscovery rate than Stateless
	if report.ContextOS.AvgRediscovery >= report.Stateless.AvgRediscovery {
		t.Fatalf("expected ContextOS to have lower rediscovery rate than stateless (got %f vs %f)",
			report.ContextOS.AvgRediscovery, report.Stateless.AvgRediscovery)
	}

	// ContextOS should have higher handoff success than Stateless
	if report.ContextOS.AvgHandoff <= report.Stateless.AvgHandoff {
		t.Fatalf("expected ContextOS to have higher handoff success than stateless (got %f vs %f)",
			report.ContextOS.AvgHandoff, report.Stateless.AvgHandoff)
	}

	// Verify sample lifecycles
	if len(report.MemoryLifecycles) == 0 {
		t.Fatalf("expected memory lifecycles in report")
	}
	for _, lc := range report.MemoryLifecycles {
		if lc.Utility <= 0 {
			t.Fatalf("expected positive memory utility, got %f", lc.Utility)
		}
		if lc.EstimatedHalfLife <= 0 {
			t.Fatalf("expected positive estimated half-life, got %f", lc.EstimatedHalfLife)
		}
	}
}
