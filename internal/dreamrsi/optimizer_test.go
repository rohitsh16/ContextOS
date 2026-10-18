package dreamrsi

import (
	"fmt"
	"testing"
	"contextos/internal/model"
)

func TestDreamOptimizer(t *testing.T) {
	// Construct a realistic suite of replay scenarios
	var scenarios []SimulationScenario
	for i := 0; i < 20; i++ {
		scenarios = append(scenarios, SimulationScenario{
			ID:     fmt.Sprintf("task-%d", i),
			Task:   fmt.Sprintf("query for component %d", i),
			Budget: 500,
			Candidates: []model.Candidate{
				{
					ID:           fmt.Sprintf("cand-%d-core", i),
					Content:      fmt.Sprintf("CoreImplementationComponent%d", i),
					Location:     fmt.Sprintf("internal/comp%d/core.go", i),
					Tokens:       80,
					Semantic:     0.95,
					Lexical:      0.90,
					TaskAffinity: 0.90,
					Authority:    0.95,
					Evidence:     1.0,
				},
				{
					ID:           fmt.Sprintf("cand-%d-marginal", i),
					Content:      fmt.Sprintf("SecondaryHelper%d", i),
					Location:     fmt.Sprintf("internal/comp%d/helper.go", i),
					Tokens:       120,
					Semantic:     0.45,
					Lexical:      0.40,
					TaskAffinity: 0.35,
					Authority:    0.70,
					Evidence:     0.5,
				},
				{
					ID:           fmt.Sprintf("cand-%d-junk", i),
					Content:      "unrelated logging output",
					Location:     "internal/log.go",
					Tokens:       150,
					Semantic:     0.05,
					Lexical:      0.02,
					TaskAffinity: 0.05,
					Authority:    0.50, // low authority
					Evidence:     0.0,
				},
			},
			RequiredEvidence: []string{fmt.Sprintf("CoreImplementationComponent%d", i)},
			BaselineTokens:   350,
		})
	}

	sim := NewReplaySimulator(scenarios)
	cfg := OptimizerConfig{
		Generations:         10,
		PopulationSize:      5,
		PerturbationScale:   0.15,
		MinSuccessThreshold: 0.95,
		Seed:                12345,
	}

	opt := NewDreamOptimizer(sim, cfg)
	baseline := DefaultBaselinePolicy()

	report := opt.Optimize(baseline)

	if report.GenerationsRun != 10 {
		t.Fatalf("expected 10 generations run, got %d", report.GenerationsRun)
	}
	if report.TotalSimulatedEvals < 50 {
		t.Fatalf("expected at least 50 simulated evals, got %d", report.TotalSimulatedEvals)
	}
	if report.WinningSuccessRate < 0.95 {
		t.Fatalf("expected winning success rate >= 0.95, got %f", report.WinningSuccessRate)
	}
	if !report.PromotedToActive {
		t.Fatalf("expected winning policy to pass promotion gate")
	}

	t.Logf("Dream-RSI Optimization Finished in %v:", report.Duration)
	t.Logf("  Baseline Success: %.1f%% | Winning Success: %.1f%%",
		report.BaselineSuccessRate*100, report.WinningSuccessRate*100)
	t.Logf("  Baseline Tokens: %.1f | Winning Tokens: %.1f",
		report.BaselineTokensMean, report.WinningTokensMean)
	t.Logf("  Token Savings: %.2f%%", report.TokenSavingsPct)
	t.Logf("  Cliff's Delta: %.3f (%s) | Permutation P-Value: %.4f",
		report.CliffsDelta, report.CliffsInterpretation, report.PermutationPValue)
}
