package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"contextos/internal/bench"
)

func main() {
	seed := flag.Int64("seed", 42, "PRNG random seed for task generation")
	n := flag.Int("n", 100, "number of synthetic tasks to evaluate")
	budget := flag.Int("budget", 2048, "token budget for context packing")
	sweep := flag.Bool("sweep", false, "run budget sweep across [512, 1024, 2048, 4096, 8192, 16384, 32768]")
	baselinesFlag := flag.String("baselines", "", "comma-separated list of baselines (e.g. B0,B3,B7,B9 or empty for all)")
	ablations := flag.Bool("ablations", false, "run ablation experiment comparing full system against disabled subsystems")
	jsonOutput := flag.Bool("json", true, "output structured JSON report")
	flag.Parse()

	cfg := bench.DefaultConfig()
	cfg.Seed = *seed
	cfg.NumTasks = *n
	cfg.Budget = *budget

	if *baselinesFlag != "" {
		parts := strings.Split(*baselinesFlag, ",")
		var selected []bench.BaselineID
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			for _, bid := range bench.AllBaselines {
				if strings.HasPrefix(string(bid), trimmed) || string(bid) == trimmed {
					selected = append(selected, bid)
					break
				}
			}
		}
		if len(selected) > 0 {
			cfg.Baselines = selected
		}
	}

	if *sweep {
		budgets := []int{512, 1024, 2048, 4096, 8192, 16384, 32768}
		sweepResults := bench.BudgetSweep(cfg, budgets)
		if *jsonOutput {
			b, err := json.MarshalIndent(sweepResults, "", "  ")
			if err != nil {
				fmt.Fprintf(os.Stderr, "json marshal error: %v\n", err)
				os.Exit(1)
			}
			fmt.Println(string(b))
		} else {
			fmt.Printf("=== ContextOS Budget Sweep (%d tasks) ===\n", *n)
			for _, sp := range sweepResults {
				fmt.Printf("Budget: %d tokens\n", sp.Budget)
				for bid, m := range sp.Results {
					fmt.Printf("  %-32s | Success: %.2f%% | Tokens: %4.0f | Cost: $%.5f\n",
						bid, m.C.TaskSuccess*100, m.B.TokenUsage, m.D.EstimatedCost)
				}
				fmt.Println("  Pareto Frontier:")
				for _, p := range sp.ParetoFrontier {
					if !p.Dominated {
						fmt.Printf("    * %-30s Success: %.2f%% Cost: $%.5f\n", p.Name, p.Success*100, p.Cost)
					}
				}
			}
		}
		return
	}

	if *ablations {
		type AblationResult struct {
			Condition string             `json:"condition"`
			Report    bench.BaselineReport `json:"report"`
		}
		conditions := []struct {
			name      string
			ablations bench.AblationConfig
		}{
			{"Full ContextOS (All ON)", bench.DefaultAblations()},
			{"Persistent Memory OFF", bench.AblationConfig{PersistentMemory: false, Graph: true, CacheAware: true, TemporalValidity: true}},
			{"Graph Centrality OFF", bench.AblationConfig{PersistentMemory: true, Graph: false, CacheAware: true, TemporalValidity: true}},
			{"Cache-Aware Prefix OFF", bench.AblationConfig{PersistentMemory: true, Graph: true, CacheAware: false, TemporalValidity: true}},
			{"Temporal Validity Scoping OFF", bench.AblationConfig{PersistentMemory: true, Graph: true, CacheAware: true, TemporalValidity: false}},
		}

		var results []AblationResult
		tasks := bench.GenerateSyntheticCorpus(cfg.Seed, cfg.NumTasks)
		for _, c := range conditions {
			rep := bench.ExecuteBaseline(bench.B9FullContextOS, tasks[0], cfg.Budget, c.ablations)
			_ = rep
			// Run on all tasks
			suiteCfg := cfg
			suiteCfg.Ablations = c.ablations
			suiteCfg.Baselines = []bench.BaselineID{bench.B9FullContextOS}
			report := bench.Run(suiteCfg)
			if b9Rep, ok := report.Baselines[string(bench.B9FullContextOS)]; ok {
				results = append(results, AblationResult{Condition: c.name, Report: b9Rep})
			}
		}

		b, _ := json.MarshalIndent(results, "", "  ")
		fmt.Println(string(b))
		return
	}

	report := bench.Run(cfg)
	if *jsonOutput {
		b, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "json marshal error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(b))
	} else {
		fmt.Printf("=== ContextOS Scientific Benchmark Suite ===\n")
		fmt.Printf("Tasks: %d | Budget: %d tokens | Confidence: 95%%\n\n", *n, *budget)
		for name, rep := range report.Baselines {
			fmt.Printf("Baseline: %s\n", name)
			fmt.Printf("  Retrieval (Layer A) : Recall@1=%.2f Recall@5=%.2f MRR=%.3f NDCG=%.3f\n",
				rep.Metrics.A.RecallAt1, rep.Metrics.A.RecallAt5, rep.Metrics.A.MRR, rep.Metrics.A.NDCG)
			fmt.Printf("  Selection (Layer B) : Tokens=%.1f Coverage=%.2f Redundancy=%.3f Util=%.2f\n",
				rep.Metrics.B.TokenUsage, rep.Metrics.B.Coverage, rep.Metrics.B.Redundancy, rep.Metrics.B.BudgetUtilization)
			fmt.Printf("  Outcome   (Layer C) : Success=%.2f%% TestPass=%.2f%% Regressions=%.2f\n",
				rep.Metrics.C.TaskSuccess*100, rep.Metrics.C.TestPass*100, rep.Metrics.C.RegressionRate)
			fmt.Printf("  Economics (Layer D) : Cost=$%.5f Latency=%.2fms Cached=%.1f\n\n",
				rep.Metrics.D.EstimatedCost, rep.Metrics.D.LatencyMs, rep.Metrics.D.CachedTokens)
		}
	}
}
