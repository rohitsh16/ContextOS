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

func TestParaphraseSuite(t *testing.T) {
	manifestPath := filepath.Join("manifests", "paraphrases.json")
	rep, err := RunCorrectnessSuiteFiltered(".", manifestPath, "R18-PARAPHRASE", "paraphrase")
	if err != nil {
		t.Fatalf("RunCorrectnessSuiteFiltered paraphrase failed: %v", err)
	}

	if rep.Evidence.PollutionRate != 0.0 {
		t.Errorf("Gate G1 violated: pollution rate must be 0.0, got %f", rep.Evidence.PollutionRate)
	}
	if rep.Retrieval.RecallAt1 < 0.80 {
		t.Errorf("Recall@1 must be >= 0.80, got %f", rep.Retrieval.RecallAt1)
	}
	if rep.Retrieval.RecallAt5 < 0.90 {
		t.Errorf("Recall@5 must be >= 0.90, got %f", rep.Retrieval.RecallAt5)
	}
	if rep.Retrieval.RecallAt10 < 0.95 {
		t.Errorf("Recall@10 must be >= 0.95, got %f", rep.Retrieval.RecallAt10)
	}
	if rep.Retrieval.RecallAt20 < 0.98 {
		t.Errorf("Recall@20 must be >= 0.98, got %f", rep.Retrieval.RecallAt20)
	}
	if rep.Paraphrase.PSIAt20 < 0.80 {
		t.Errorf("PSI@20 must be >= 0.80, got %f", rep.Paraphrase.PSIAt20)
	}
	if rep.Paraphrase.RecallRange > 0.05 {
		t.Errorf("Recall range across paraphrases must be <= 0.05, got %f", rep.Paraphrase.RecallRange)
	}
	if rep.Verdict != "GREEN" {
		t.Errorf("Expected verdict GREEN, got %s", rep.Verdict)
	}
}

func TestIdentifierAblationSuite(t *testing.T) {
	manifestPath := filepath.Join("manifests", "identifier_ablation.json")
	rep, err := RunCorrectnessSuiteFiltered(".", manifestPath, "R18-ABLATION", "identifier-ablation")
	if err != nil {
		t.Fatalf("RunCorrectnessSuiteFiltered ablation failed: %v", err)
	}

	if rep.Retrieval.RecallAt10 < 0.80 {
		t.Errorf("Ablation Recall@10 must be >= 0.80, got %f", rep.Retrieval.RecallAt10)
	}
	if len(rep.Ablation.StepScores) == 0 {
		t.Errorf("Expected step scores to be recorded")
	}
	if rep.Verdict != "GREEN" {
		t.Errorf("Expected verdict GREEN, got %s", rep.Verdict)
	}
}

func TestAdversarialSuite(t *testing.T) {
	manifestPath := filepath.Join("manifests", "adversarial.json")
	rep, err := RunCorrectnessSuiteFiltered(".", manifestPath, "R18-ADVERSARIAL", "adversarial")
	if err != nil {
		t.Fatalf("RunCorrectnessSuiteFiltered adversarial failed: %v", err)
	}

	if rep.Answer.Accuracy < 0.80 {
		t.Errorf("Adversarial accuracy must be >= 0.80, got %f", rep.Answer.Accuracy)
	}
	if rep.Verdict != "GREEN" {
		t.Errorf("Expected verdict GREEN, got %s", rep.Verdict)
	}
}

func TestMSEBaselinesSuite(t *testing.T) {
	manifestPath := filepath.Join("manifests", "golden_manifests.json")
	rep, err := RunCorrectnessSuiteFiltered(".", manifestPath, "R18-MSE", "mse")
	if err != nil {
		t.Fatalf("RunCorrectnessSuiteFiltered mse failed: %v", err)
	}

	if rep.Baselines == nil {
		t.Fatalf("Expected Baselines comparison in MSE report")
	}
	if rep.Baselines.B0FullMaximal == 0 || rep.Baselines.B7ImprovedLayeredMSE == 0 {
		t.Errorf("Expected B0-B7 baselines to be evaluated")
	}
	if rep.Context.Compression < 1.5 {
		t.Errorf("Expected compression >= 1.5x, got %fx", rep.Context.Compression)
	}
	if rep.Context.MinimalityRate < 0.70 {
		t.Errorf("Expected minimality rate >= 0.70, got %f", rep.Context.MinimalityRate)
	}
	if rep.Verdict != "GREEN" {
		t.Errorf("Expected verdict GREEN, got %s", rep.Verdict)
	}
}

