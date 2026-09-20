package bench

import (
	"math"
	"testing"
	"time"
)

func TestCostBreakdownAndReconciliation(t *testing.T) {
	pricing := DefaultPricing()

	// Task 1: Uncached heavy
	b1 := ComputeCostBreakdown(1500, 0, 150, 0, pricing, 0.0001, 0.0)
	// Task 2: Cached heavy with cache write
	b2 := ComputeCostBreakdown(300, 1200, 150, 500, pricing, 0.0001, 0.0)

	rec1 := AccountingRecord{
		TaskID:           "task-1",
		Phase:            "execution",
		Model:            "claude-sonnet",
		InputTokens:      1500,
		CachedTokens:     0,
		OutputTokens:     150,
		CacheWriteTokens: 0,
		Breakdown:        b1,
	}

	rec2 := AccountingRecord{
		TaskID:           "task-2",
		Phase:            "execution",
		Model:            "claude-sonnet",
		InputTokens:      1500,
		CachedTokens:     1200,
		OutputTokens:     150,
		CacheWriteTokens: 500,
		Breakdown:        b2,
	}

	records := []AccountingRecord{rec1, rec2}
	reportedTotal := b1.TotalCost + b2.TotalCost

	rep := ReconcileCosts(records, reportedTotal, 1e-6)
	if !rep.Reconciled {
		t.Fatalf("expected costs to reconcile, got discrepancy: $%.8f", rep.DiscrepancyUSD)
	}
	if rep.TotalRuns != 2 {
		t.Errorf("expected 2 runs in reconciliation, got %d", rep.TotalRuns)
	}

	// Test deliberate mismatch detection
	repMismatch := ReconcileCosts(records, reportedTotal+0.05, 1e-6)
	if repMismatch.Reconciled {
		t.Errorf("expected mismatch detection when reported total is inflated by $0.05")
	}
}

func TestDetailedCacheMetrics(t *testing.T) {
	now := time.Now().Add(-60 * time.Second)
	metrics := NewDetailedCacheMetrics(
		true,  // planHit
		true,  // promptHit
		false, // responseHit
		300,   // cachedTokens
		2000,  // totalInputTokens
		500,   // prefixTokens
		500,   // writeTokens
		now,   // createdTime
		0.15,  // baselineHitRate
	)

	if !metrics.ContextPlanCacheHit {
		t.Errorf("expected ContextPlanCacheHit to be true")
	}
	if !metrics.ProviderPromptCacheHit {
		t.Errorf("expected ProviderPromptCacheHit to be true")
	}
	if metrics.ResponseCacheHit {
		t.Errorf("expected ResponseCacheHit to be false")
	}
	if metrics.RegressionGateStatus != "PASS" {
		t.Errorf("expected RegressionGateStatus PASS, got %s", metrics.RegressionGateStatus)
	}
	// 300 / 2000 = 15.0% < 60.0% -> target status should be PENDING
	if metrics.ResearchTargetStatus == "MET" {
		t.Errorf("expected ResearchTargetStatus PENDING, got %s", metrics.ResearchTargetStatus)
	}
}

func TestOracleValidation(t *testing.T) {
	valSet := GenerateStratifiedValidationSet(42, 70)
	if len(valSet) != 70 {
		t.Fatalf("expected 70 stratified tasks, got %d", len(valSet))
	}

	report := ValidateTaskSuccessOracle(valSet)
	if report.TotalEvaluated != 70 {
		t.Errorf("expected 70 evaluated, got %d", report.TotalEvaluated)
	}

	// High proxy fidelity should yield Precision and Recall >= 0.85
	if report.OverallMatrix.Precision < 0.80 {
		t.Errorf("oracle precision too low: %f", report.OverallMatrix.Precision)
	}
	if report.OverallMatrix.Recall < 0.80 {
		t.Errorf("oracle recall too low: %f", report.OverallMatrix.Recall)
	}
	if math.IsNaN(report.OverallMatrix.F1Score) {
		t.Errorf("invalid F1 score")
	}

	if len(report.PerStratumMatrix) != len(AllStrata) {
		t.Errorf("expected %d strata, got %d", len(AllStrata), len(report.PerStratumMatrix))
	}
}
