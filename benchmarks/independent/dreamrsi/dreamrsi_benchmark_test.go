package dreamrsi_test

import (
	"fmt"
	"testing"
	"time"

	"contextos/internal/dreamrsi"
	"contextos/internal/model"
)

// BenchmarkDreamRSIOfflineReplayThroughput evaluates how fast the offline
// replay simulator can evaluate counterfactual exploration policies.
func BenchmarkDreamRSIOfflineReplayThroughput(b *testing.B) {
	var scenarios []dreamrsi.SimulationScenario
	for i := 0; i < 100; i++ {
		scenarios = append(scenarios, dreamrsi.SimulationScenario{
			ID:     fmt.Sprintf("bench-task-%d", i),
			Task:   "distributed transaction outbox pattern",
			Budget: 500,
			Candidates: []model.Candidate{
				{ID: "c1", Content: "OutboxProcessor", Tokens: 60, Semantic: 0.95, Lexical: 0.90, Authority: 0.95},
				{ID: "c2", Content: "DatabaseMigrator", Tokens: 120, Semantic: 0.40, Lexical: 0.30, Authority: 0.80},
				{ID: "c3", Content: "LegacyLogParser", Tokens: 200, Semantic: 0.10, Lexical: 0.05, Authority: 0.50},
			},
			RequiredEvidence: []string{"OutboxProcessor"},
			BaselineTokens:   380,
		})
	}

	sim := dreamrsi.NewReplaySimulator(scenarios)
	policy := dreamrsi.DefaultBaselinePolicy()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sim.ReplayAll(policy)
	}
}

// TestIndependentDreamRSIEvolutionaryCycle runs a complete offline Dream-RSI cycle
// over a realistic benchmark repository task set and measures empirical token and efficiency improvements.
func TestIndependentDreamRSIEvolutionaryCycle(t *testing.T) {
	// Construct representative task corpus with mixed relevance
	var scenarios []dreamrsi.SimulationScenario
	for i := 0; i < 30; i++ {
		scenarios = append(scenarios, dreamrsi.SimulationScenario{
			ID:     fmt.Sprintf("repo-task-%d", i),
			Task:   fmt.Sprintf("implement feature module %d", i),
			Budget: 600,
			Candidates: []model.Candidate{
				{
					ID:           fmt.Sprintf("cand-%d-authority-high", i),
					Content:      fmt.Sprintf("ArchitecturalDirective_%d", i),
					Tokens:       70,
					Semantic:     0.92,
					Lexical:      0.88,
					TaskAffinity: 0.95,
					Authority:    1.0,
					Evidence:     1.0,
				},
				{
					ID:           fmt.Sprintf("cand-%d-marginal-helper", i),
					Content:      fmt.Sprintf("SecondaryHelperUtility_%d", i),
					Tokens:       140,
					Semantic:     0.35,
					Lexical:      0.30,
					TaskAffinity: 0.25,
					Authority:    0.75,
					Evidence:     0.4,
				},
				{
					ID:           fmt.Sprintf("cand-%d-bloat", i),
					Content:      fmt.Sprintf("UnrelatedDependencySlice_%d", i),
					Tokens:       250,
					Semantic:     0.15,
					Lexical:      0.10,
					TaskAffinity: 0.05,
					Authority:    0.55, // Low authority
					Evidence:     0.0,
				},
			},
			RequiredEvidence: []string{fmt.Sprintf("ArchitecturalDirective_%d", i)},
			BaselineTokens:   460,
		})
	}

	sim := dreamrsi.NewReplaySimulator(scenarios)
	cfg := dreamrsi.OptimizerConfig{
		Generations:         15,
		PopulationSize:      8,
		PerturbationScale:   0.20,
		MinSuccessThreshold: 0.95,
		Seed:                98765,
	}

	opt := dreamrsi.NewDreamOptimizer(sim, cfg)
	baseline := dreamrsi.DefaultBaselinePolicy()

	start := time.Now()
	report := opt.Optimize(baseline)
	elapsed := time.Since(start)

	t.Log("==========================================================================")
	t.Log("            DREAM-RSI INDEPENDENT OPTIMIZATION BENCHMARK REPORT           ")
	t.Log("==========================================================================")
	t.Logf("Elapsed Wall-Clock Duration:      %v", elapsed)
	t.Logf("Total Simulated Policy Trials:    %d", report.TotalSimulatedEvals)
	t.Logf("Throughput:                       %.1f policy-scenario evaluations/sec",
		float64(report.TotalSimulatedEvals*len(scenarios))/elapsed.Seconds())
	t.Log("--------------------------------------------------------------------------")
	t.Logf("Baseline Success Rate:            %.1f%%", report.BaselineSuccessRate*100)
	t.Logf("Winning Policy Success Rate:      %.1f%%", report.WinningSuccessRate*100)
	t.Logf("Baseline Mean Tokens:             %.1f tokens", report.BaselineTokensMean)
	t.Logf("Winning Policy Mean Tokens:       %.1f tokens", report.WinningTokensMean)
	t.Logf("Empirical Token Savings:          %.2f%%", report.TokenSavingsPct)
	t.Log("--------------------------------------------------------------------------")
	t.Logf("Cliff's Delta Effect Size:        %.3f (%s)", report.CliffsDelta, report.CliffsInterpretation)
	t.Logf("Paired Permutation P-Value:       %.4f", report.PermutationPValue)
	t.Logf("Promoted to Active Runtime:       %v", report.PromotedToActive)
	t.Log("==========================================================================")

	// Statistical invariants
	if !report.PromotedToActive {
		t.Fatalf("expected winning policy to pass promotion gate")
	}
	if report.WinningSuccessRate < report.BaselineSuccessRate {
		t.Fatalf("regression detected: winning success %.2f < baseline %.2f",
			report.WinningSuccessRate, report.BaselineSuccessRate)
	}
}
