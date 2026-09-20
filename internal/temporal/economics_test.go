package temporal

import (
	"testing"

	"contextos/internal/model"
)

func TestMemoryEconomicsAndLearnedForgetting(t *testing.T) {
	memDecision := model.Memory{
		ID:         "mem-dec",
		Kind:       "decision",
		Content:    "PostgreSQL transaction isolation serializable",
		TokenCost:  20,
		Confidence: 0.95,
		ReuseCount: 10,
	}

	memState := model.Memory{
		ID:         "mem-state",
		Kind:       "state",
		Content:    "Temporary cursor variable offset",
		TokenCost:  10,
		Confidence: 0.50,
		ReuseCount: 0,
	}

	valDec := EvaluateMemoryEconomics(memDecision, 2.0)
	valState := EvaluateMemoryEconomics(memState, 5.0)

	if !valDec.ShouldPersist {
		t.Fatalf("expected high-reuse decision memory to persist, got %+v", valDec)
	}

	if valDec.MemoryROI <= valState.MemoryROI {
		t.Fatalf("expected decision ROI > state ROI, got dec=%.2f, state=%.2f",
			valDec.MemoryROI, valState.MemoryROI)
	}

	report := RunMemoryEconomicsAudit([]model.Memory{memDecision, memState}, 3.0)
	if report.Status != "GREEN" {
		t.Fatalf("expected economics report status GREEN, got %s", report.Status)
	}
	if report.PersistedCount != 1 || report.ForgettingCount != 1 {
		t.Fatalf("expected 1 persisted and 1 forgotten, got %d and %d",
			report.PersistedCount, report.ForgettingCount)
	}
}
