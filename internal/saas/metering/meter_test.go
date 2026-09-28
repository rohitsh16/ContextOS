package metering

import (
	"testing"

	"contextos/internal/saas/auth"
)

func TestUsageMeteringAggregationAndInvoicing(t *testing.T) {
	m := NewMeter()

	tenant := &auth.Tenant{
		ID:                 "tenant_pro_123",
		Name:               "Pro Dev",
		Tier:               auth.TierPro,
		MonthlyTokenBudget: 100000,
	}

	var alertFired bool
	var alertPct float64
	m.SetQuotaAlertHandler(func(tenantID string, usagePercent float64, currentTokens, budgetTokens int64) {
		alertFired = true
		alertPct = usagePercent
	})

	// Record several usage events
	events := []UsageEvent{
		{
			TenantID:      tenant.ID,
			Endpoint:      "/v1/context/plan",
			Model:         "claude-3-7-sonnet",
			InputTokens:   4000,
			CachedTokens:  2000,
			OutputTokens:  1000,
			EstimatedCost: 0.015,
			CacheHit:      true,
			LatencyMS:     120,
		},
		{
			TenantID:      tenant.ID,
			Endpoint:      "/v1/context/plan",
			Model:         "gpt-4o",
			InputTokens:   8000,
			CachedTokens:  0,
			OutputTokens:  2000,
			EstimatedCost: 0.040,
			CacheHit:      false,
			LatencyMS:     350,
		},
		{
			TenantID:      tenant.ID,
			Endpoint:      "/v1/context/search",
			Model:         "local",
			InputTokens:   1000,
			CachedTokens:  0,
			OutputTokens:  100,
			EstimatedCost: 0.0,
			CacheHit:      false,
			LatencyMS:     10,
		},
	}

	for _, ev := range events {
		m.Record(ev)
	}

	agg, ok := m.GetAggregate(tenant.ID)
	if !ok {
		t.Fatal("failed to retrieve tenant usage aggregate")
	}

	if agg.TotalRequests != 3 {
		t.Fatalf("expected 3 requests, got %d", agg.TotalRequests)
	}
	if agg.TotalInputTokens != 13000 {
		t.Fatalf("expected 13000 input tokens, got %d", agg.TotalInputTokens)
	}
	if agg.TotalCachedTokens != 2000 {
		t.Fatalf("expected 2000 cached tokens, got %d", agg.TotalCachedTokens)
	}
	if agg.CacheHits != 1 {
		t.Fatalf("expected 1 cache hit, got %d", agg.CacheHits)
	}
	if agg.ByModel["claude-3-7-sonnet"] != 1 || agg.ByModel["gpt-4o"] != 1 {
		t.Fatalf("invalid model breakdown: %+v", agg.ByModel)
	}
	if agg.ByEndpoint["/v1/context/plan"] != 2 {
		t.Fatalf("invalid endpoint breakdown: %+v", agg.ByEndpoint)
	}

	// Generate invoice
	inv := GenerateInvoice(tenant, agg)
	if inv.TenantID != tenant.ID {
		t.Fatalf("invoice tenant mismatch: %s", inv.TenantID)
	}
	if inv.BaseFeeUSD != 29.0 {
		t.Fatalf("expected $29 base fee for Pro tier, got %f", inv.BaseFeeUSD)
	}
	if inv.TotalUSD < 29.0 {
		t.Fatalf("total USD cannot be less than base fee, got %f", inv.TotalUSD)
	}
	if len(inv.LineItems) == 0 {
		t.Fatal("invoice missing line items")
	}

	// Trigger quota alert test
	// Add 90,000 tokens to exceed 80% of 100,000 budget
	m.Record(UsageEvent{
		TenantID:    tenant.ID,
		InputTokens: 85000,
	})
	m.CheckQuota(tenant)
	if !alertFired {
		t.Fatal("quota alert callback should have fired at >80% usage")
	}
	if alertPct < 80.0 {
		t.Fatalf("expected alert percentage >= 80, got %f", alertPct)
	}
}
