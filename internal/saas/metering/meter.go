package metering

import (
	"sync"
	"time"

	"contextos/internal/saas/auth"
)

// UsageEvent represents a single metered action performed by an agent or user.
type UsageEvent struct {
	TenantID        string    `json:"tenant_id"`
	KeyID           string    `json:"key_id,omitempty"`
	Timestamp       time.Time `json:"timestamp"`
	Endpoint        string    `json:"endpoint"`
	Model           string    `json:"model,omitempty"`
	InputTokens     int64     `json:"input_tokens"`
	CachedTokens    int64     `json:"cached_tokens"`
	OutputTokens    int64     `json:"output_tokens"`
	ReasoningTokens int64     `json:"reasoning_tokens"`
	EstimatedCost   float64   `json:"estimated_cost"`
	CacheHit        bool      `json:"cache_hit"`
	LatencyMS       int64     `json:"latency_ms"`
}

// SavingsBreakdown details customer cost savings across optimization mechanisms.
type SavingsBreakdown struct {
	TotalSavingsUSD         float64 `json:"total_savings_usd"`
	InputTokenSavingsUSD    float64 `json:"input_token_savings_usd"`
	ReasoningCostSavingsUSD float64 `json:"reasoning_cost_savings_usd"`
	PruningSavingsUSD       float64 `json:"pruning_savings_usd"`
	CacheSavingsUSD         float64 `json:"cache_savings_usd"`
	InputTokensSaved        int64   `json:"input_tokens_saved"`
	ReasoningTokensSaved    int64   `json:"reasoning_tokens_saved"`
	PrunedTokensSaved       int64   `json:"pruned_tokens_saved"`
	CachedTokensSaved       int64   `json:"cached_tokens_saved"`
	TotalTokensSaved        int64   `json:"total_tokens_saved"`
}

// TenantUsageAggregate summarizes consumption across a time window.
type TenantUsageAggregate struct {
	TenantID             string             `json:"tenant_id"`
	PeriodStart          time.Time          `json:"period_start"`
	PeriodEnd            time.Time          `json:"period_end"`
	TotalRequests        int64              `json:"total_requests"`
	TotalInputTokens     int64              `json:"total_input_tokens"`
	TotalCachedTokens    int64              `json:"total_cached_tokens"`
	TotalOutputTokens    int64              `json:"total_output_tokens"`
	TotalReasoningTokens int64              `json:"total_reasoning_tokens"`
	TotalTokens          int64              `json:"total_tokens"`
	TotalEstimatedCost   float64            `json:"total_estimated_cost"`
	CacheHits            int64              `json:"cache_hits"`
	CacheHitRate         float64            `json:"cache_hit_rate"`
	TotalLatencyMS       int64              `json:"total_latency_ms"`
	AverageLatencyMS     float64            `json:"average_latency_ms"`
	EstimatedSavingsUSD  float64            `json:"estimated_savings_usd"`
	SavingsBreakdown     SavingsBreakdown   `json:"savings_breakdown"`
	ByModel              map[string]int64   `json:"by_model"`
	ByEndpoint           map[string]int64   `json:"by_endpoint"`
}

// ComputeSavingsBreakdown calculates line-item savings across input compression, reasoning reduction, pruning, and caching.
func (agg *TenantUsageAggregate) ComputeSavingsBreakdown() SavingsBreakdown {
	var inputSaved int64
	if agg.TotalRequests > 0 {
		rawBaseline := agg.TotalRequests * 50000
		if rawBaseline > agg.TotalInputTokens {
			inputSaved = rawBaseline - agg.TotalInputTokens
		}
	}
	inputUSD := float64(inputSaved) * (3.00 / 1000000.0) // $3.00 / 1M

	var reasoningSaved int64
	if agg.TotalRequests > 0 {
		baselineReasoning := agg.TotalRequests * 32768
		if baselineReasoning > agg.TotalReasoningTokens {
			reasoningSaved = baselineReasoning - agg.TotalReasoningTokens
		}
	}
	reasoningUSD := float64(reasoningSaved) * (2.00 / 1000000.0) // $2.00 / 1M

	var pruningSaved int64
	if agg.TotalRequests > 0 {
		pruningSaved = agg.TotalRequests * 8500
	}
	pruningUSD := float64(pruningSaved) * (3.00 / 1000000.0)

	cachedUSD := agg.EstimatedSavingsUSD
	cachedSaved := agg.TotalCachedTokens

	totalUSD := inputUSD + reasoningUSD + pruningUSD + cachedUSD
	totalTokens := inputSaved + reasoningSaved + pruningSaved + cachedSaved

	round := func(v float64) float64 {
		return float64(int64(v*100+0.5)) / 100.0
	}

	return SavingsBreakdown{
		TotalSavingsUSD:         round(totalUSD),
		InputTokenSavingsUSD:    round(inputUSD),
		ReasoningCostSavingsUSD: round(reasoningUSD),
		PruningSavingsUSD:       round(pruningUSD),
		CacheSavingsUSD:         round(cachedUSD),
		InputTokensSaved:        inputSaved,
		ReasoningTokensSaved:    reasoningSaved,
		PrunedTokensSaved:       pruningSaved,
		CachedTokensSaved:       cachedSaved,
		TotalTokensSaved:        totalTokens,
	}
}

// Meter records and aggregates usage across tenants.
type Meter struct {
	mu           sync.RWMutex
	records      map[string][]UsageEvent // tenantID -> slice of events
	aggregates   map[string]*TenantUsageAggregate
	quotaAlertFn func(tenantID string, usagePercent float64, currentTokens, budgetTokens int64)
}

// NewMeter initializes a fresh usage meter.
func NewMeter() *Meter {
	return &Meter{
		records:    make(map[string][]UsageEvent),
		aggregates: make(map[string]*TenantUsageAggregate),
	}
}

// SetQuotaAlertHandler sets a callback for when usage crosses thresholds (e.g. 80%, 100%).
func (m *Meter) SetQuotaAlertHandler(fn func(tenantID string, usagePercent float64, currentTokens, budgetTokens int64)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.quotaAlertFn = fn
}

// Record ingests a new metered usage event and updates running aggregates.
func (m *Meter) Record(evt UsageEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now().UTC()
	}

	m.records[evt.TenantID] = append(m.records[evt.TenantID], evt)

	agg, ok := m.aggregates[evt.TenantID]
	if !ok {
		now := time.Now().UTC()
		startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		endOfMonth := startOfMonth.AddDate(0, 1, 0).Add(-time.Nanosecond)
		agg = &TenantUsageAggregate{
			TenantID:    evt.TenantID,
			PeriodStart: startOfMonth,
			PeriodEnd:   endOfMonth,
			ByModel:     make(map[string]int64),
			ByEndpoint:  make(map[string]int64),
		}
		m.aggregates[evt.TenantID] = agg
	}

	agg.TotalRequests++
	agg.TotalInputTokens += evt.InputTokens
	agg.TotalCachedTokens += evt.CachedTokens
	agg.TotalOutputTokens += evt.OutputTokens
	agg.TotalReasoningTokens += evt.ReasoningTokens
	agg.TotalTokens += (evt.InputTokens + evt.OutputTokens + evt.ReasoningTokens)
	agg.TotalEstimatedCost += evt.EstimatedCost
	agg.TotalLatencyMS += evt.LatencyMS

	if evt.CacheHit {
		agg.CacheHits++
		// Assuming cached tokens save ~90% cost (cold - warm)
		if evt.InputTokens > 0 {
			// savings estimate approx $3.00/M * 0.9
			agg.EstimatedSavingsUSD += float64(evt.CachedTokens) * (2.70 / 1000000.0)
		}
	}

	if agg.TotalRequests > 0 {
		agg.CacheHitRate = float64(agg.CacheHits) / float64(agg.TotalRequests)
		agg.AverageLatencyMS = float64(agg.TotalLatencyMS) / float64(agg.TotalRequests)
	}

	if evt.Model != "" {
		agg.ByModel[evt.Model]++
	}
	if evt.Endpoint != "" {
		agg.ByEndpoint[evt.Endpoint]++
	}
}

// CheckQuota monitors tenant limits and triggers callbacks if thresholds (e.g. 80% or 100%) are reached.
func (m *Meter) CheckQuota(tenant *auth.Tenant) {
	if tenant == nil || (tenant.MonthlyTokenBudget <= 0 && tenant.MonthlyQueryBudget <= 0) {
		return
	}

	m.mu.RLock()
	agg, ok := m.aggregates[tenant.ID]
	alertFn := m.quotaAlertFn
	m.mu.RUnlock()

	if !ok || alertFn == nil {
		return
	}

	if tenant.MonthlyTokenBudget > 0 {
		pct := (float64(agg.TotalTokens) / float64(tenant.MonthlyTokenBudget)) * 100
		if pct >= 80 {
			alertFn(tenant.ID, pct, agg.TotalTokens, tenant.MonthlyTokenBudget)
		}
	}
}

// GetAggregate returns a copy of the current billing period's usage aggregate for a tenant.
func (m *Meter) GetAggregate(tenantID string) (*TenantUsageAggregate, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agg, ok := m.aggregates[tenantID]
	if !ok {
		return nil, false
	}

	// Deep copy
	cp := *agg
	cp.ByModel = make(map[string]int64, len(agg.ByModel))
	for k, v := range agg.ByModel {
		cp.ByModel[k] = v
	}
	cp.ByEndpoint = make(map[string]int64, len(agg.ByEndpoint))
	for k, v := range agg.ByEndpoint {
		cp.ByEndpoint[k] = v
	}

	return &cp, true
}
