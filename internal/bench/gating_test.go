package bench

import (
	"testing"
)

func TestReleaseGating(t *testing.T) {
	report := EvaluateReleaseGates("ContextOS-R12-Candidate")

	if report.Status != "GREEN" {
		t.Fatalf("expected release gate status GREEN, got %s", report.Status)
	}

	if !report.AllGatesPassed {
		t.Fatalf("expected all gates to pass, but only %d/%d passed", report.PassedCount, report.TotalGates)
	}

	if report.DeploymentDecision != "APPROVED FOR PRODUCTION" {
		t.Fatalf("expected decision APPROVED FOR PRODUCTION, got %s", report.DeploymentDecision)
	}

	// Verify all 4 categories are covered
	categories := make(map[ReleaseGateCategory]int)
	for _, g := range report.Gates {
		categories[g.Category]++
	}

	expectedCategories := []ReleaseGateCategory{
		GateCategoryScientific,
		GateCategoryMathematical,
		GateCategoryEngineering,
		GateCategorySafety,
	}
	for _, cat := range expectedCategories {
		if categories[cat] == 0 {
			t.Errorf("expected gates in category %s, got 0", cat)
		}
	}

	table := FormatReleaseGateReport(report)
	if len(table) == 0 {
		t.Fatalf("expected non-empty release gate report string")
	}
}
