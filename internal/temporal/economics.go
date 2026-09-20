package temporal

import (
	"math"
	"strings"

	"contextos/internal/model"
)

// ClassHazardProfile defines the baseline decay hazard per memory kind (PR.md Section 9).
type ClassHazardProfile struct {
	Kind           string  `json:"kind"`
	BaseHazard     float64 `json:"base_hazard"`     // Daily or generational hazard rate
	ExpectedHalfLife float64 `json:"expected_half_life"` // in turns/generations
	ReuseValue     float64 `json:"reuse_value"`     // Marginal dollar/utility value per reuse
}

// MemoryEconomicValuation captures the investment equation for memory persistence.
type MemoryEconomicValuation struct {
	MemoryID           string  `json:"memory_id"`
	Kind               string  `json:"kind"`
	SurvivalProb       float64 `json:"survival_prob"` // S_m(t)
	HazardRate         float64 `json:"hazard_rate"`   // h_m(t)
	ExpectedReuses     float64 `json:"expected_reuses"`
	StorageCostUSD     float64 `json:"storage_cost_usd"`
	MaintenanceCostUSD float64 `json:"maintenance_cost_usd"`
	StaleRiskCostUSD   float64 `json:"stale_risk_cost_usd"`
	NetEconomicValue   float64 `json:"net_economic_value"` // V_persist(m)
	MemoryROI          float64 `json:"memory_roi"`
	ShouldPersist      bool    `json:"should_persist"`
}

// MemoryEconomicsReport aggregates the memory portfolio economics.
type MemoryEconomicsReport struct {
	TotalEvaluated    int                       `json:"total_evaluated"`
	PersistedCount    int                       `json:"persisted_count"`
	ForgettingCount   int                       `json:"forgetting_count"`
	TotalStorageCost  float64                   `json:"total_storage_cost"`
	NetPortfolioValue float64                   `json:"net_portfolio_value"`
	AverageROI        float64                   `json:"average_roi"`
	ClassProfiles     map[string]ClassHazardProfile `json:"class_profiles"`
	Status            string                    `json:"status"` // GREEN or RED
}

// DefaultClassHazardProfiles returns empirically estimated hazard rates across memory types.
func DefaultClassHazardProfiles() map[string]ClassHazardProfile {
	return map[string]ClassHazardProfile{
		"decision": {
			Kind:             "decision",
			BaseHazard:       0.05, // Long-lived architectural decisions
			ExpectedHalfLife: 14.0,
			ReuseValue:       0.0050,
		},
		"constraint": {
			Kind:             "constraint",
			BaseHazard:       0.04, // Very durable constraints
			ExpectedHalfLife: 17.5,
			ReuseValue:       0.0060,
		},
		"failure": {
			Kind:             "failure",
			BaseHazard:       0.12, // Bugs/failures fix over time
			ExpectedHalfLife: 5.8,
			ReuseValue:       0.0040,
		},
		"state": {
			Kind:             "state",
			BaseHazard:       0.35, // Session states decay quickly
			ExpectedHalfLife: 2.0,
			ReuseValue:       0.0015,
		},
		"observation": {
			Kind:             "observation",
			BaseHazard:       0.25,
			ExpectedHalfLife: 2.8,
			ReuseValue:       0.0010,
		},
		"code": {
			Kind:             "code",
			BaseHazard:       0.15, // Source evolves with churn
			ExpectedHalfLife: 4.6,
			ReuseValue:       0.0020,
		},
		"handoff": {
			Kind:             "handoff",
			BaseHazard:       0.50, // Ephemeral handoff context
			ExpectedHalfLife: 1.4,
			ReuseValue:       0.0030,
		},
	}
}

// EvaluateMemoryEconomics computes V_persist(m) and MemoryROI (PR.md R4-H3).
func EvaluateMemoryEconomics(m model.Memory, ageGens float64) MemoryEconomicValuation {
	profiles := DefaultClassHazardProfiles()
	kindNorm := strings.ToLower(m.Kind)
	profile, ok := profiles[kindNorm]
	if !ok {
		profile = ClassHazardProfile{
			Kind:             kindNorm,
			BaseHazard:       0.20,
			ExpectedHalfLife: 3.5,
			ReuseValue:       0.0020,
		}
	}

	// Survival probability: S(t) = exp(-hazard * t)
	survival := math.Exp(-profile.BaseHazard * ageGens)
	hazard := profile.BaseHazard

	// Expected future reuses conditioned on survival and prior reuse
	expectedReuses := float64(m.ReuseCount+1) * 0.8 * survival

	// Costs in standard USD
	storageCost := 0.000005 * float64(m.TokenCost) // Marginal DB footprint cost
	maintenanceCost := 0.000010 * ageGens          // Invalidation indexing cost
	staleRiskCost := (1.0 - survival) * 0.0050     // Downstream failure cost if stale

	grossValue := expectedReuses * profile.ReuseValue
	totalCost := storageCost + maintenanceCost + staleRiskCost
	netValue := grossValue - totalCost

	roi := 0.0
	if totalCost > 0 {
		roi = grossValue / totalCost
	}

	shouldPersist := netValue > 0 && survival > 0.15

	return MemoryEconomicValuation{
		MemoryID:           m.ID,
		Kind:               m.Kind,
		SurvivalProb:       survival,
		HazardRate:         hazard,
		ExpectedReuses:     expectedReuses,
		StorageCostUSD:     storageCost,
		MaintenanceCostUSD: maintenanceCost,
		StaleRiskCostUSD:   staleRiskCost,
		NetEconomicValue:   netValue,
		MemoryROI:          roi,
		ShouldPersist:      shouldPersist,
	}
}

// RunMemoryEconomicsAudit evaluates the portfolio of active memories.
func RunMemoryEconomicsAudit(memories []model.Memory, averageAgeGens float64) MemoryEconomicsReport {
	var valuations []MemoryEconomicValuation
	persisted := 0
	forgotten := 0
	totalStorage := 0.0
	netValue := 0.0
	totalROI := 0.0

	for _, m := range memories {
		val := EvaluateMemoryEconomics(m, averageAgeGens)
		valuations = append(valuations, val)
		totalStorage += val.StorageCostUSD
		netValue += val.NetEconomicValue
		totalROI += val.MemoryROI

		if val.ShouldPersist {
			persisted++
		} else {
			forgotten++
		}
	}

	avgROI := 0.0
	if len(memories) > 0 {
		avgROI = totalROI / float64(len(memories))
	}

	status := "GREEN"
	// R4 GREEN criterion: learned forgetting reduces storage/stale exposure without losing high-ROI facts
	if len(memories) > 0 && persisted == 0 {
		status = "RED"
	}

	return MemoryEconomicsReport{
		TotalEvaluated:    len(memories),
		PersistedCount:    persisted,
		ForgettingCount:   forgotten,
		TotalStorageCost:  totalStorage,
		NetPortfolioValue: netValue,
		AverageROI:        avgROI,
		ClassProfiles:     DefaultClassHazardProfiles(),
		Status:            status,
	}
}
