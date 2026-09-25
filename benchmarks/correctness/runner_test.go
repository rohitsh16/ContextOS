package correctness

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCorrectnessBenchmarkSuite(t *testing.T) {
	manifestPath := filepath.Join("manifests", "golden_manifests.json")

	report, err := RunCorrectnessSuite(".", manifestPath, "R18-MSE-CORRECTNESS-001")
	if err != nil {
		t.Fatalf("RunCorrectnessSuite failed: %v", err)
	}

	// 1. Gate G1: Index Integrity (Known pollution cannot enter authoritative retrieval)
	if report.Evidence.PollutionRate != 0.0 {
		t.Fatalf("Gate G1 violated: pollution rate must be 0.0, got %f", report.Evidence.PollutionRate)
	}

	// 2. Gate G2: Retrieval Recall (Required evidence can be found reliably)
	if report.Retrieval.RecallAt10 < 0.95 {
		t.Fatalf("Gate G2 violated: Recall@10 must be >= 0.95, got %f", report.Retrieval.RecallAt10)
	}

	// 3. Gate G3: Sufficiency
	if report.Evidence.Coverage < 0.80 {
		t.Fatalf("Gate G3 violated: Evidence coverage must be >= 0.80, got %f", report.Evidence.Coverage)
	}

	// 4. Gate G4: Minimality
	if report.Context.MinimalityRate < 0.70 {
		t.Fatalf("Gate G4 violated: Minimality rate must be >= 0.70, got %f", report.Context.MinimalityRate)
	}

	// 5. Gate G5: Verification & Gate G6: Safe failure
	if report.Answer.ContradictionRate < 0.05 {
		t.Fatalf("Gate G5 violated: Expected contradiction detection on adversarial test case")
	}
	if report.Selective.AbstentionRate < 0.10 {
		t.Fatalf("Gate G6 violated: Expected abstention on unsupported test case")
	}

	// 6. Gate G7: Efficiency
	if report.Context.Compression < 1.5 {
		t.Fatalf("Gate G7 violated: Expected compression >= 1.5x, got %fx", report.Context.Compression)
	}

	// 7. Overall Verdict must be GREEN
	if report.Verdict != "GREEN" {
		t.Fatalf("Expected overall verdict GREEN, got %s (failures: %+v)", report.Verdict, report.Failures)
	}

	// Test markdown formatting
	md := report.FormatMarkdownReport()
	if !strings.Contains(md, "ContextOS Correctness & Minimum Sufficient Evidence Report") {
		t.Fatalf("Markdown report missing title: %s", md)
	}
	if !strings.Contains(md, "GREEN") {
		t.Fatalf("Markdown report missing GREEN verdict: %s", md)
	}
}
