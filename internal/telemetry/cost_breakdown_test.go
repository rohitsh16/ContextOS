package telemetry

import (
	"math"
	"testing"
	"time"
)

func TestCostBreakdownExactReconciliation(t *testing.T) {
	pricing := PricingEntry{
		Provider:              "openai",
		Model:                 "o3-mini",
		InputPerMillion:       1.10,
		CachedInputPerMillion: 0.55,
		CacheWritePerMillion:  0.0,
		OutputPerMillion:      4.40,
		ReasoningPerMillion:   4.40,
	}

	usage := UsageMetrics{
		InputTokens:         10000,
		CachedInputTokens:   4000,
		ReasoningTokens:     5000,
		VisibleOutputTokens: 1000,
		OutputTokens:        6000,
	}

	toolCost := 0.005
	verifyCost := 0.012
	retryCost := 0.003
	escalateCost := 0.020
	ctrlCost := 0.001

	cb := ComputeCostBreakdown(pricing, usage, toolCost, verifyCost, retryCost, escalateCost, ctrlCost)

	// Validate components
	expectedInput := (6000.0 / 1e6) * 1.10
	expectedCached := (4000.0 / 1e6) * 0.55
	expectedReasoning := (5000.0 / 1e6) * 4.40
	expectedVisible := (1000.0 / 1e6) * 4.40

	epsilon := 1e-9
	if math.Abs(cb.InputUSD-expectedInput) > epsilon {
		t.Errorf("InputUSD expected %f, got %f", expectedInput, cb.InputUSD)
	}
	if math.Abs(cb.CachedInputUSD-expectedCached) > epsilon {
		t.Errorf("CachedInputUSD expected %f, got %f", expectedCached, cb.CachedInputUSD)
	}
	if math.Abs(cb.ReasoningUSD-expectedReasoning) > epsilon {
		t.Errorf("ReasoningUSD expected %f, got %f", expectedReasoning, cb.ReasoningUSD)
	}
	if math.Abs(cb.VisibleOutputUSD-expectedVisible) > epsilon {
		t.Errorf("VisibleOutputUSD expected %f, got %f", expectedVisible, cb.VisibleOutputUSD)
	}

	// Verify exact reconciliation (10 buckets == TotalUSD)
	if !cb.Reconcile(1e-9) {
		t.Errorf("CostBreakdown failed exact reconciliation: total=%f sum=%f", cb.TotalUSD, cb.Sum())
	}
}

func TestPricingRegistryLookup(t *testing.T) {
	reg := DefaultPricingRegistry()

	// Lookup pinned o3-mini version
	entry, err := reg.LookupVersion("openai", "o3-mini", "2025-01-31")
	if err != nil {
		t.Fatalf("LookupVersion failed: %v", err)
	}
	if entry.InputPerMillion != 1.10 {
		t.Errorf("expected 1.10 input per million, got %f", entry.InputPerMillion)
	}

	// Lookup latest
	latest, err := reg.LookupLatest("anthropic", "claude-3-7-sonnet")
	if err != nil {
		t.Fatalf("LookupLatest failed: %v", err)
	}
	if latest.Version != "2025-02-24" {
		t.Errorf("expected latest version 2025-02-24, got %s", latest.Version)
	}

	// Register custom pinned pricing
	custom := PricingEntry{
		Provider:        "test",
		Model:           "custom-model",
		Version:         "v1.0",
		EffectiveAt:     time.Now().UTC(),
		InputPerMillion: 2.0,
	}
	reg.Register(custom)

	got, err := reg.LookupVersion("test", "custom-model", "v1.0")
	if err != nil || got.InputPerMillion != 2.0 {
		t.Errorf("custom model lookup failed")
	}
}
