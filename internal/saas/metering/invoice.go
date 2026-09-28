package metering

import (
	"fmt"
	"math"
	"time"

	"contextos/internal/saas/auth"
)

// PricingTierDetails defines base costs and quota allowances.
type PricingTierDetails struct {
	BasePriceUSD     float64
	IncludedRequests int64
	IncludedTokens   int64
	OveragePerQuery  float64
	OveragePerMTok   float64
}

var TierPricing = map[string]PricingTierDetails{
	auth.TierFree: {
		BasePriceUSD:     0.0,
		IncludedRequests: 1000,
		IncludedTokens:   5000000,
		OveragePerQuery:  0.0,
		OveragePerMTok:   0.0,
	},
	auth.TierPro: {
		BasePriceUSD:     29.0,
		IncludedRequests: 50000,
		IncludedTokens:   250000000,
		OveragePerQuery:  0.001,
		OveragePerMTok:   0.50,
	},
	auth.TierTeam: {
		BasePriceUSD:     99.0,
		IncludedRequests: 250000,
		IncludedTokens:   1500000000,
		OveragePerQuery:  0.0008,
		OveragePerMTok:   0.40,
	},
	auth.TierEnterprise: {
		BasePriceUSD:     499.0,
		IncludedRequests: 2000000,
		IncludedTokens:   10000000000,
		OveragePerQuery:  0.0005,
		OveragePerMTok:   0.30,
	},
}

// LineItem is an individual invoice entry.
type LineItem struct {
	Description string  `json:"description"`
	Quantity    int64   `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
	AmountUSD   float64 `json:"amount_usd"`
}

// Invoice represents a generated billable statement for a customer.
type Invoice struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenant_id"`
	Tier           string     `json:"tier"`
	PeriodStart    time.Time  `json:"period_start"`
	PeriodEnd      time.Time  `json:"period_end"`
	BaseFeeUSD     float64    `json:"base_fee_usd"`
	OverageUSD     float64    `json:"overage_usd"`
	TotalUSD       float64    `json:"total_usd"`
	TotalSavings     float64          `json:"total_savings_usd"`
	SavingsBreakdown SavingsBreakdown `json:"savings_breakdown"`
	Status           string           `json:"status"` // "draft", "issued", "paid"
	LineItems        []LineItem       `json:"line_items"`
	GeneratedAt      time.Time        `json:"generated_at"`
}

// GenerateInvoice aggregates consumption and generates an invoice for the tenant.
func GenerateInvoice(tenant *auth.Tenant, agg *TenantUsageAggregate) *Invoice {
	now := time.Now().UTC()
	tier := auth.TierFree
	if tenant != nil && tenant.Tier != "" {
		tier = tenant.Tier
	}

	cfg, ok := TierPricing[tier]
	if !ok {
		cfg = TierPricing[auth.TierFree]
	}

	invoiceID := fmt.Sprintf("inv_%s_%d", tenant.ID, now.Unix())
	inv := &Invoice{
		ID:          invoiceID,
		TenantID:    tenant.ID,
		Tier:        tier,
		PeriodStart: agg.PeriodStart,
		PeriodEnd:   agg.PeriodEnd,
		BaseFeeUSD:  cfg.BasePriceUSD,
		Status:      "issued",
		GeneratedAt: now,
		LineItems:   make([]LineItem, 0),
	}

	// 1. Base Subscription Line Item
	inv.LineItems = append(inv.LineItems, LineItem{
		Description: fmt.Sprintf("%s Tier Monthly Subscription", capitalize(tier)),
		Quantity:    1,
		UnitPrice:   cfg.BasePriceUSD,
		AmountUSD:   cfg.BasePriceUSD,
	})

	var overageTotal float64

	// 2. Request overage (if applicable)
	if agg.TotalRequests > cfg.IncludedRequests && cfg.OveragePerQuery > 0 {
		overageRequests := agg.TotalRequests - cfg.IncludedRequests
		overageCost := float64(overageRequests) * cfg.OveragePerQuery
		overageTotal += overageCost
		inv.LineItems = append(inv.LineItems, LineItem{
			Description: fmt.Sprintf("Query Overage (%d excess queries)", overageRequests),
			Quantity:    overageRequests,
			UnitPrice:   cfg.OveragePerQuery,
			AmountUSD:   round2(overageCost),
		})
	}

	// 3. Token overage (if applicable)
	if agg.TotalTokens > cfg.IncludedTokens && cfg.OveragePerMTok > 0 {
		overageTokens := agg.TotalTokens - cfg.IncludedTokens
		mTokens := float64(overageTokens) / 1000000.0
		overageCost := mTokens * cfg.OveragePerMTok
		overageTotal += overageCost
		inv.LineItems = append(inv.LineItems, LineItem{
			Description: fmt.Sprintf("Token Overage (%.2fM excess tokens)", mTokens),
			Quantity:    overageTokens,
			UnitPrice:   cfg.OveragePerMTok / 1000000.0,
			AmountUSD:   round2(overageCost),
		})
	}

	inv.OverageUSD = round2(overageTotal)
	inv.TotalUSD = round2(inv.BaseFeeUSD + inv.OverageUSD)
	inv.SavingsBreakdown = agg.ComputeSavingsBreakdown()
	inv.TotalSavings = inv.SavingsBreakdown.TotalSavingsUSD

	return inv
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return string(s[0]-32) + s[1:]
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
