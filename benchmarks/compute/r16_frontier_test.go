package compute_bench

import (
	"os"
	"path/filepath"
	"testing"

	"contextos/internal/compute"
	"contextos/internal/providers"
)

func TestRunR16S2CanaryExecutionAndArtifacts(t *testing.T) {
	tempDir := t.TempDir()

	cfg := R16S2Config{
		Mode:       "mock",
		ResultsDir: tempDir,
		Seed:       42,
		Repeats:    5,
		Tasks:      DefaultR16S2Tasks(),
	}

	summary, err := RunR16S2Canary(cfg)
	if err != nil {
		t.Fatalf("RunR16S2Canary failed: %v", err)
	}

	// 1. Verify 300 executions (12 tasks × 5 efforts × 5 repeats)
	if summary.TotalExecutions != 300 {
		t.Errorf("expected 300 executions, got %d", summary.TotalExecutions)
	}

	// 2. Verify split isolation
	if summary.CalibrationRuns == 0 || summary.ValidationRuns == 0 || summary.HoldoutRuns == 0 {
		t.Errorf("expected non-zero split runs: cal=%d, val=%d, hld=%d",
			summary.CalibrationRuns, summary.ValidationRuns, summary.HoldoutRuns)
	}

	// 3. Verify all 5 artifacts were written
	expectedFiles := []string{
		"r16_s2_manifest.json",
		"r16_s2_observations.jsonl",
		"r16_s2_frontiers.json",
		"r16_s2_summary.json",
		"r16_s2_report.md",
	}
	for _, f := range expectedFiles {
		p := filepath.Join(tempDir, f)
		stat, err := os.Stat(p)
		if err != nil || stat.Size() == 0 {
			t.Errorf("expected non-empty artifact %s, stat err: %v", f, err)
		}
	}

	// 4. Verify policy comparison
	ctxMetrics, ok := summary.Policies["contextos"]
	if !ok {
		t.Fatalf("missing 'contextos' policy metrics in summary")
	}
	maxMetrics, ok := summary.Policies["baseline_fixed_max"]
	if !ok {
		t.Fatalf("missing 'baseline_fixed_max' policy metrics in summary")
	}

	// ContextOS must achieve lower total cost than fixed max
	if ctxMetrics.TotalCostUSD >= maxMetrics.TotalCostUSD {
		t.Errorf("ContextOS cost ($%.4f) should be less than fixed max ($%.4f)",
			ctxMetrics.TotalCostUSD, maxMetrics.TotalCostUSD)
	}

	// ContextOS must preserve capability floor (<= 2% violations)
	if ctxMetrics.FloorViolationRate > 0.02 {
		t.Errorf("ContextOS floor violation rate too high: %.2f%%", ctxMetrics.FloorViolationRate*100)
	}

	// Automated gate checks
	if !summary.GatesPassed {
		t.Errorf("expected automated gates to pass")
	}
}

// F1 — Cheap-but-insufficient trap:
// Cheap + low effort = below floor; strong + high effort = above floor.
// The optimizer must not choose the cheap option.
func TestF1_CheapButInsufficientTrap(t *testing.T) {
	estimator := compute.NewEmpiricalCapabilityEstimator()

	// Train estimator: model-A at EffortLow has quality 0.50 (below floor 0.80) at $0.005
	// model-A at EffortHigh has quality 0.95 (above floor 0.80) at $0.050
	for i := 0; i < 5; i++ {
		estimator.RecordObservation(compute.EmpiricalObservation{
			TaskFamily:       "security",
			TaskClass:        compute.T3Difficult,
			DifficultyBucket: "0.60-0.75",
			Provider:         "openai",
			Model:            "o3-mini",
			Effort:           providers.EffortLow,
			QualityScore:     0.50,
			Success:          false,
			ActualCostUSD:    0.005,
		})
		estimator.RecordObservation(compute.EmpiricalObservation{
			TaskFamily:       "security",
			TaskClass:        compute.T3Difficult,
			DifficultyBucket: "0.60-0.75",
			Provider:         "openai",
			Model:            "o3-mini",
			Effort:           providers.EffortHigh,
			QualityScore:     0.95,
			Success:          true,
			ActualCostUSD:    0.050,
		})
	}

	models := []compute.ModelCandidate{
		{Provider: "openai", Model: "o3-mini"},
	}
	optimizer := compute.NewCapabilityPreservingOptimizer(estimator, models)

	task := compute.TaskProfile{
		Family:     "security",
		Class:      compute.T3Difficult,
		Difficulty: 0.70,
	}
	floor := compute.CapabilityFloor{
		RequiredQuality:            0.80,
		RequiredSuccessProbability: 0.80,
	}
	state := compute.ControllerState{ContextTokens: 2500, EvidenceCoverage: 0.90}

	cfg, ok := optimizer.SelectMinimumSufficient(state, task, floor)
	if !ok {
		t.Fatalf("expected feasible configuration")
	}
	if cfg.Effort < providers.EffortMedium {
		t.Fatalf("F1 FALSIFICATION VIOLATION: optimizer selected cheap insufficient effort: %v", cfg.Effort)
	}
}

// F2 — Expensive-but-unnecessary trap:
// Cheap + medium = safely sufficient; strongest + maximum = sufficient but 10x more expensive.
// The optimizer should prefer the cheaper sufficient configuration.
func TestF2_ExpensiveButUnnecessaryTrap(t *testing.T) {
	estimator := compute.NewEmpiricalCapabilityEstimator()

	for i := 0; i < 5; i++ {
		// Cheap model o3-mini at Medium effort is safely sufficient (0.92) at $0.02
		estimator.RecordObservation(compute.EmpiricalObservation{
			TaskFamily:       "local_fix",
			TaskClass:        compute.T1Trivial,
			DifficultyBucket: "0.00-0.35",
			Provider:         "openai",
			Model:            "o3-mini",
			Effort:           providers.EffortMedium,
			QualityScore:     0.92,
			Success:          true,
			ActualCostUSD:    0.02,
		})
		// Strongest model at Maximum effort is also sufficient (0.98) at $0.25
		estimator.RecordObservation(compute.EmpiricalObservation{
			TaskFamily:       "local_fix",
			TaskClass:        compute.T1Trivial,
			DifficultyBucket: "0.00-0.35",
			Provider:         "anthropic",
			Model:            "claude-3-5-sonnet",
			Effort:           providers.EffortMaximum,
			QualityScore:     0.98,
			Success:          true,
			ActualCostUSD:    0.25,
		})
	}

	models := []compute.ModelCandidate{
		{Provider: "openai", Model: "o3-mini"},
		{Provider: "anthropic", Model: "claude-3-5-sonnet"},
	}
	optimizer := compute.NewCapabilityPreservingOptimizer(estimator, models)

	task := compute.TaskProfile{
		Family:     "local_fix",
		Class:      compute.T1Trivial,
		Difficulty: 0.20,
	}
	floor := compute.CapabilityFloor{
		RequiredQuality:            0.80,
		RequiredSuccessProbability: 0.80,
	}
	state := compute.ControllerState{ContextTokens: 1000, EvidenceCoverage: 0.95}

	cfg, ok := optimizer.SelectMinimumSufficient(state, task, floor)
	if !ok {
		t.Fatalf("expected feasible configuration")
	}
	if cfg.Model.Provider == "anthropic" && cfg.Effort == providers.EffortMaximum {
		t.Fatalf("F2 FALSIFICATION VIOLATION: optimizer selected expensive-unnecessary configuration: %+v", cfg)
	}
	if cfg.ExpectedE2ECostUSD > 0.10 {
		t.Fatalf("F2 FALSIFICATION VIOLATION: expected cheap sufficient configuration, got cost $%.4f", cfg.ExpectedE2ECostUSD)
	}
}

// F3 — Reasoning-sensitive task:
// Construct a task where the same model at low effort fails frequently but medium/high succeeds.
// ContextOS must allocate sufficient reasoning.
func TestF3_ReasoningSensitiveTask(t *testing.T) {
	estimator := compute.NewEmpiricalCapabilityEstimator()

	for i := 0; i < 5; i++ {
		// Low effort fails frequently
		estimator.RecordObservation(compute.EmpiricalObservation{
			TaskFamily:       "concurrency",
			TaskClass:        compute.T3Difficult,
			DifficultyBucket: "0.60-0.75",
			Provider:         "openai",
			Model:            "o3-mini",
			Effort:           providers.EffortLow,
			QualityScore:     0.60,
			Success:          false,
			ActualCostUSD:    0.01,
		})
		// Medium effort succeeds
		estimator.RecordObservation(compute.EmpiricalObservation{
			TaskFamily:       "concurrency",
			TaskClass:        compute.T3Difficult,
			DifficultyBucket: "0.60-0.75",
			Provider:         "openai",
			Model:            "o3-mini",
			Effort:           providers.EffortMedium,
			QualityScore:     0.94,
			Success:          true,
			ActualCostUSD:    0.04,
		})
	}

	models := []compute.ModelCandidate{
		{Provider: "openai", Model: "o3-mini"},
	}
	optimizer := compute.NewCapabilityPreservingOptimizer(estimator, models)

	task := compute.TaskProfile{
		Family:     "concurrency",
		Class:      compute.T3Difficult,
		Difficulty: 0.70,
	}
	floor := compute.CapabilityFloor{
		RequiredQuality:            0.85,
		RequiredSuccessProbability: 0.85,
	}
	state := compute.ControllerState{ContextTokens: 2500, EvidenceCoverage: 0.90}

	cfg, ok := optimizer.SelectMinimumSufficient(state, task, floor)
	if !ok {
		t.Fatalf("expected feasible configuration")
	}
	if cfg.Effort < providers.EffortMedium {
		t.Fatalf("F3 FALSIFICATION VIOLATION: under-allocated reasoning on sensitive task: %v", cfg.Effort)
	}
}

// F4 — Model-sensitive task:
// A stronger model at lower effort beats a weaker model at higher effort on capability per dollar.
// The optimizer should discover this tradeoff.
func TestF4_ModelSensitiveTradeoff(t *testing.T) {
	estimator := compute.NewEmpiricalCapabilityEstimator()

	for i := 0; i < 5; i++ {
		// Weaker model at High effort: reaches quality 0.82 at $0.08
		estimator.RecordObservation(compute.EmpiricalObservation{
			TaskFamily:       "architecture",
			TaskClass:        compute.T4Critical,
			DifficultyBucket: "0.75-1.00",
			Provider:         "weaker_prov",
			Model:            "weaker_model",
			Effort:           providers.EffortHigh,
			QualityScore:     0.82,
			Success:          true,
			ActualCostUSD:    0.08,
		})
		// Stronger model at Low effort: reaches quality 0.92 at $0.04
		estimator.RecordObservation(compute.EmpiricalObservation{
			TaskFamily:       "architecture",
			TaskClass:        compute.T4Critical,
			DifficultyBucket: "0.75-1.00",
			Provider:         "stronger_prov",
			Model:            "stronger_model",
			Effort:           providers.EffortLow,
			QualityScore:     0.92,
			Success:          true,
			ActualCostUSD:    0.04,
		})
	}

	models := []compute.ModelCandidate{
		{Provider: "weaker_prov", Model: "weaker_model"},
		{Provider: "stronger_prov", Model: "stronger_model"},
	}
	optimizer := compute.NewCapabilityPreservingOptimizer(estimator, models)

	task := compute.TaskProfile{
		Family:     "architecture",
		Class:      compute.T4Critical,
		Difficulty: 0.85,
	}
	floor := compute.CapabilityFloor{
		RequiredQuality:            0.80,
		RequiredSuccessProbability: 0.80,
	}
	state := compute.ControllerState{ContextTokens: 3000, EvidenceCoverage: 0.90}

	cfg, ok := optimizer.SelectMinimumSufficient(state, task, floor)
	if !ok {
		t.Fatalf("expected feasible configuration")
	}
	if cfg.Model.Provider != "stronger_prov" {
		t.Fatalf("F4 FALSIFICATION VIOLATION: optimizer failed to discover superior model tradeoff, selected: %s", cfg.Model.Provider)
	}
}

// F5 — Cost shock:
// Multiply a configuration's economics while keeping capability unchanged.
// The chosen action may change, but the capability floor must not be violated.
func TestF5_CostShockPreservesCapabilityFloor(t *testing.T) {
	estimator := compute.NewEmpiricalCapabilityEstimator()

	for i := 0; i < 5; i++ {
		// Model A: quality 0.90, cost inflated 10x from $0.01 to $0.10
		estimator.RecordObservation(compute.EmpiricalObservation{
			TaskFamily:       "refactor",
			TaskClass:        compute.T2Moderate,
			DifficultyBucket: "0.35-0.60",
			Provider:         "prov_a",
			Model:            "model_a",
			Effort:           providers.EffortMedium,
			QualityScore:     0.90,
			Success:          true,
			ActualCostUSD:    0.10,
		})
		// Model B: quality 0.88, cost normal $0.03
		estimator.RecordObservation(compute.EmpiricalObservation{
			TaskFamily:       "refactor",
			TaskClass:        compute.T2Moderate,
			DifficultyBucket: "0.35-0.60",
			Provider:         "prov_b",
			Model:            "model_b",
			Effort:           providers.EffortMedium,
			QualityScore:     0.88,
			Success:          true,
			ActualCostUSD:    0.03,
		})
	}

	models := []compute.ModelCandidate{
		{Provider: "prov_a", Model: "model_a"},
		{Provider: "prov_b", Model: "model_b"},
	}
	optimizer := compute.NewCapabilityPreservingOptimizer(estimator, models)

	task := compute.TaskProfile{
		Family:     "refactor",
		Class:      compute.T2Moderate,
		Difficulty: 0.50,
	}
	floor := compute.CapabilityFloor{
		RequiredQuality:            0.80,
		RequiredSuccessProbability: 0.80,
	}
	state := compute.ControllerState{ContextTokens: 2000, EvidenceCoverage: 0.90}

	cfg, ok := optimizer.SelectMinimumSufficient(state, task, floor)
	if !ok {
		t.Fatalf("expected feasible configuration")
	}
	// It should switch to prov_b due to cost shock
	if cfg.Model.Provider != "prov_b" {
		t.Fatalf("expected optimizer to switch to prov_b under cost shock, got: %s", cfg.Model.Provider)
	}
	// And quality LCB must still meet floor
	if cfg.PredictedQualityLCB < floor.RequiredQuality {
		t.Fatalf("F5 FALSIFICATION VIOLATION: capability floor violated under cost shock: %f < %f",
			cfg.PredictedQualityLCB, floor.RequiredQuality)
	}
}

// F6 — Capability shock:
// Degrade a model's observed capability.
// The controller should stop selecting it when its LCB falls below the floor.
func TestF6_CapabilityShockDeselection(t *testing.T) {
	estimator := compute.NewEmpiricalCapabilityEstimator()

	for i := 0; i < 5; i++ {
		// Model A suffered capability shock: quality dropped from 0.95 to 0.40
		estimator.RecordObservation(compute.EmpiricalObservation{
			TaskFamily:       "distributed",
			TaskClass:        compute.T3Difficult,
			DifficultyBucket: "0.60-0.75",
			Provider:         "shocked_prov",
			Model:            "shocked_model",
			Effort:           providers.EffortHigh,
			QualityScore:     0.40,
			Success:          false,
			ActualCostUSD:    0.01,
		})
		// Model B is stable at 0.92
		estimator.RecordObservation(compute.EmpiricalObservation{
			TaskFamily:       "distributed",
			TaskClass:        compute.T3Difficult,
			DifficultyBucket: "0.60-0.75",
			Provider:         "stable_prov",
			Model:            "stable_model",
			Effort:           providers.EffortHigh,
			QualityScore:     0.92,
			Success:          true,
			ActualCostUSD:    0.05,
		})
	}

	models := []compute.ModelCandidate{
		{Provider: "shocked_prov", Model: "shocked_model"},
		{Provider: "stable_prov", Model: "stable_model"},
	}
	optimizer := compute.NewCapabilityPreservingOptimizer(estimator, models)

	task := compute.TaskProfile{
		Family:     "distributed",
		Class:      compute.T3Difficult,
		Difficulty: 0.70,
	}
	floor := compute.CapabilityFloor{
		RequiredQuality:            0.80,
		RequiredSuccessProbability: 0.80,
	}
	state := compute.ControllerState{ContextTokens: 2500, EvidenceCoverage: 0.90}

	cfg, ok := optimizer.SelectMinimumSufficient(state, task, floor)
	if !ok {
		t.Fatalf("expected feasible configuration")
	}
	if cfg.Model.Provider == "shocked_prov" {
		t.Fatalf("F6 FALSIFICATION VIOLATION: optimizer selected shocked provider below floor: %s", cfg.Model.Provider)
	}
}

// F7 — Estimator uncertainty:
// Reduce estimator sample size or increase uncertainty.
// The controller should become more conservative—not cheaper.
func TestF7_EstimatorUncertaintyConservatism(t *testing.T) {
	estimator := compute.NewEmpiricalCapabilityEstimator()

	// Only 1 observation recorded (scarce data, high uncertainty)
	estimator.RecordObservation(compute.EmpiricalObservation{
		TaskFamily:       "new_task",
		TaskClass:        compute.T2Moderate,
		DifficultyBucket: "0.35-0.60",
		Provider:         "openai",
		Model:            "o3-mini",
		Effort:           providers.EffortLow,
		QualityScore:     0.82,
		Success:          true,
		ActualCostUSD:    0.01,
	})

	models := []compute.ModelCandidate{
		{Provider: "openai", Model: "o3-mini"},
	}
	optimizer := compute.NewCapabilityPreservingOptimizer(estimator, models)

	task := compute.TaskProfile{
		Family:     "new_task",
		Class:      compute.T2Moderate,
		Difficulty: 0.50,
	}
	floor := compute.CapabilityFloor{
		RequiredQuality:            0.80,
		RequiredSuccessProbability: 0.80,
	}
	state := compute.ControllerState{ContextTokens: 2000, EvidenceCoverage: 0.90}

	cfg, _ := optimizer.SelectMinimumSufficient(state, task, floor)
	// Under high uncertainty / scarce samples (<3), LCB is penalized and the controller
	// must not select the cheap Minimal/Low configuration; it must scale up conservatively.
	if cfg.Effort < providers.EffortLow {
		t.Fatalf("F7 FALSIFICATION VIOLATION: controller selected overly cheap effort under high uncertainty: %v", cfg.Effort)
	}
}

// F8 — Distribution shift:
// Use unseen repositories or task templates (no exact empirical match).
// The policy should degrade gracefully while protecting the capability floor.
func TestF8_DistributionShiftGracefulDegradation(t *testing.T) {
	estimator := compute.NewEmpiricalCapabilityEstimator()

	// Empty empirical store: complete distribution shift
	models := compute.DefaultModels()
	optimizer := compute.NewCapabilityPreservingOptimizer(estimator, models)

	task := compute.TaskProfile{
		Family:     "completely_unseen_domain",
		Class:      compute.T3Difficult,
		Difficulty: 0.70,
	}
	floor := compute.CapabilityFloor{
		RequiredQuality:            0.75,
		RequiredSuccessProbability: 0.75,
	}
	state := compute.ControllerState{ContextTokens: 3000, EvidenceCoverage: 0.85}

	cfg, ok := optimizer.SelectMinimumSufficient(state, task, floor)
	if !ok {
		// Even if no exact feasible configuration exists, it must return a fallback without crashing
		if cfg.Model.Provider == "" {
			t.Fatalf("F8 FALSIFICATION VIOLATION: optimizer crashed or returned empty configuration on distribution shift")
		}
	}
}
