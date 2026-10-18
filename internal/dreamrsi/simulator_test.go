package dreamrsi

import (
	"fmt"
	"testing"
	"contextos/internal/model"
)

func TestSimulatorReplay(t *testing.T) {
	cands := []model.Candidate{
		{
			ID:           "cand-1",
			Content:      "type PricingRegistry struct {}",
			Location:     "internal/telemetry/pricing.go",
			Tokens:       50,
			Semantic:     0.90,
			Lexical:      0.85,
			TaskAffinity: 0.95,
			Authority:    0.95,
			Confidence:   0.90,
			Evidence:     1.0,
		},
		{
			ID:           "cand-2",
			Content:      "func UnrelatedHelper() {}",
			Location:     "internal/unrelated.go",
			Tokens:       200,
			Semantic:     0.10,
			Lexical:      0.05,
			TaskAffinity: 0.10,
			Authority:    0.50, // low authority < 0.60
			Confidence:   0.30,
			Evidence:     0.0,
		},
		{
			ID:           "cand-3",
			Content:      "func LookupLatest() PricingModel {}",
			Location:     "internal/telemetry/lookup.go",
			Tokens:       80,
			Semantic:     0.88,
			Lexical:      0.80,
			TaskAffinity: 0.85,
			Authority:    0.90,
			Confidence:   0.85,
			Evidence:     0.90,
		},
	}

	sc := SimulationScenario{
		ID:               "task-pricing",
		Task:             "pricing registry lookup",
		Budget:           200,
		Candidates:       cands,
		RequiredEvidence: []string{"PricingRegistry", "LookupLatest"},
		BaselineTokens:   330,
	}

	sim := NewReplaySimulator([]SimulationScenario{sc})
	policy := DefaultBaselinePolicy()

	res := sim.Replay(sc, policy)

	if !res.PredictedSuccess {
		t.Fatalf("expected PredictedSuccess=true, got false")
	}
	if res.EvidenceRecall != 1.0 {
		t.Fatalf("expected EvidenceRecall=1.0, got %f", res.EvidenceRecall)
	}
	if res.SimulatedTokens != 130 { // cand-1 (50) + cand-3 (80)
		t.Fatalf("expected SimulatedTokens=130, got %d", res.SimulatedTokens)
	}
	// cand-2 has authority 0.50 < 0.60 -> rejected, should be excluded
	if res.TokenReductionPct <= 0 {
		t.Fatalf("expected positive token reduction, got %f", res.TokenReductionPct)
	}
}

func TestSimulatorHighThroughput(t *testing.T) {
	var scenarios []SimulationScenario
	for i := 0; i < 500; i++ {
		scenarios = append(scenarios, SimulationScenario{
			ID:     fmt.Sprintf("sc-%d", i),
			Task:   "test task query",
			Budget: 500,
			Candidates: []model.Candidate{
				{ID: "c1", Tokens: 40, Semantic: 0.9, Lexical: 0.8, Authority: 0.9},
				{ID: "c2", Tokens: 60, Semantic: 0.7, Lexical: 0.6, Authority: 0.8},
				{ID: "c3", Tokens: 80, Semantic: 0.5, Lexical: 0.4, Authority: 0.7},
			},
			RequiredEvidence: []string{"c1"},
			BaselineTokens:   180,
		})
	}

	sim := NewReplaySimulator(scenarios)
	policy := DefaultBaselinePolicy()

	report := sim.ReplayAll(policy)

	if report.TotalScenarios != 500 {
		t.Fatalf("expected 500 scenarios, got %d", report.TotalScenarios)
	}
	if report.SuccessRate != 1.0 {
		t.Fatalf("expected 100%% success rate, got %f", report.SuccessRate)
	}
	t.Logf("Replay Simulator Speed: %.1f scenarios/sec (Total: %v for 500 scenarios)",
		report.ScenariosPerSecond, report.TotalSimDuration)
}
