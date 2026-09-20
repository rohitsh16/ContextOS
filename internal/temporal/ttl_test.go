package temporal

import (
	"testing"
	"time"

	"contextos/internal/model"
)

func TestEstimateHazardAndAdaptiveTTL(t *testing.T) {
	// 1. Durable user decision with zero churn
	mDecision := model.Memory{
		Kind:      "decision",
		Authority: "user",
		Scope:     "internal/db",
	}
	hazardDec := EstimateHazard(mDecision, 0.0, 0, 0)
	if hazardDec.TotalHazard > 0.15 {
		t.Errorf("expected very low hazard for user architectural decision, got %f", hazardDec.TotalHazard)
	}

	ttlDec := CalculateAdaptiveTTL(mDecision, hazardDec.TotalHazard)
	if ttlDec < 30*24*time.Hour {
		t.Errorf("expected long TTL (>30 days) for architectural decision, got %v", ttlDec)
	}

	// 2. Volatile observation under high churn and 100 commits drift
	mObservation := model.Memory{
		Kind:      "fact",
		Authority: "inference",
		Scope:     "internal/ui/server.go",
	}
	hazardObs := EstimateHazard(mObservation, 0.9, 100, 5)
	if hazardObs.TotalHazard < 0.60 {
		t.Errorf("expected high hazard for volatile observation with high churn, got %f", hazardObs.TotalHazard)
	}

	ttlObs := CalculateAdaptiveTTL(mObservation, hazardObs.TotalHazard)
	if ttlObs > 48*time.Hour {
		t.Errorf("expected short TTL (<48h) for volatile high-churn observation, got %v", ttlObs)
	}

	// 3. Stale evaluation
	created := time.Now().Add(-72 * time.Hour) // 3 days ago
	if !IsStaleAdaptive(mObservation, created, time.Now(), hazardObs.TotalHazard) {
		t.Errorf("expected observation 3 days ago with short TTL to be stale")
	}
	if IsStaleAdaptive(mDecision, created, time.Now(), hazardDec.TotalHazard) {
		t.Errorf("expected decision 3 days ago with long TTL not to be stale")
	}
}
