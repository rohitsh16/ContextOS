package telemetry

import (
	"testing"
)

func TestUsageMetricsAddAndEffective(t *testing.T) {
	u1 := UsageMetrics{
		InputTokens:       1000,
		CachedInputTokens: 400,
		OutputTokens:      200,
		ReasoningTokens:   150,
		Turns:             1,
		EstimatedCostUSD:  0.015,
	}

	if eff := u1.EffectiveInputTokens(); eff != 600 {
		t.Fatalf("expected 600 effective input tokens, got %d", eff)
	}

	u2 := UsageMetrics{
		InputTokens:       500,
		CachedInputTokens: 200,
		OutputTokens:      100,
		ReasoningTokens:   50,
		Turns:             1,
		EstimatedCostUSD:  0.005,
	}

	u1.Add(u2)
	if u1.InputTokens != 1500 {
		t.Fatalf("expected 1500 input tokens, got %d", u1.InputTokens)
	}
	if u1.CachedInputTokens != 600 {
		t.Fatalf("expected 600 cached tokens, got %d", u1.CachedInputTokens)
	}
	if u1.Turns != 2 {
		t.Fatalf("expected 2 turns, got %d", u1.Turns)
	}
	if u1.EstimatedCostUSD < 0.0199 || u1.EstimatedCostUSD > 0.0201 {
		t.Fatalf("expected cost 0.020, got %f", u1.EstimatedCostUSD)
	}
}

func TestPricingLookupAndCalculateCost(t *testing.T) {
	pricing, ok := LookupPricing("anthropic", "claude-3-7-sonnet")
	if !ok {
		t.Fatalf("expected to find claude-3-7-sonnet pricing")
	}

	usage := UsageMetrics{
		InputTokens:       1000000,
		CachedInputTokens: 500000,
		OutputTokens:      100000,
		ReasoningTokens:   50000,
	}

	cost := CalculateUsageCost(pricing, usage)
	// uncached input: 500k * $3/M = $1.50
	// cached input: 500k * $0.30/M = $0.15
	// visible output: 50k * $15/M = $0.75
	// reasoning: 50k * $15/M = $0.75
	// Total expected = 1.50 + 0.15 + 0.75 + 0.75 = 3.15
	expected := 3.15
	if cost < expected-0.001 || cost > expected+0.001 {
		t.Fatalf("expected cost $%.2f, got $%.4f", expected, cost)
	}
}

func TestComputeSummaryKPIs(t *testing.T) {
	runs := []TaskRunTelemetry{
		{
			RunID:    "run-1",
			TaskID:   "task-1",
			Provider: "anthropic",
			Model:    "claude-3-7-sonnet",
			Success:  true,
			Usage: UsageMetrics{
				ReasoningTokens:  20000,
				Turns:            2,
				EstimatedCostUSD: 0.50,
			},
		},
		{
			RunID:    "run-2",
			TaskID:   "task-2",
			Provider: "anthropic",
			Model:    "claude-3-7-sonnet",
			Success:  false,
			Usage: UsageMetrics{
				ReasoningTokens:  10000,
				Turns:            1,
				EstimatedCostUSD: 0.25,
			},
		},
	}

	kpi := ComputeSummaryKPIs(runs, 1.00) // Baseline cost $1.00
	if kpi.TotalRuns != 2 {
		t.Fatalf("expected 2 runs, got %d", kpi.TotalRuns)
	}
	if kpi.SuccessfulTasks != 1 {
		t.Fatalf("expected 1 successful task, got %d", kpi.SuccessfulTasks)
	}
	if kpi.SuccessRate != 0.5 {
		t.Fatalf("expected 0.5 success rate, got %f", kpi.SuccessRate)
	}
	if kpi.CostPerSuccessUSD != 0.75 { // TotalCost 0.75 / 1 success
		t.Fatalf("expected CPS 0.75, got %f", kpi.CostPerSuccessUSD)
	}
	if kpi.TotalReasoningTokens != 30000 {
		t.Fatalf("expected 30000 reasoning tokens, got %d", kpi.TotalReasoningTokens)
	}
}
