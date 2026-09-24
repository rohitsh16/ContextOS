package compute

import "testing"

func TestEmpiricalEstimatorSeparatesTaskFamilies(t *testing.T) {
	e := NewEmpiricalCapabilityEstimator()
	for i := 0; i < 3; i++ {
		e.RecordObservation(EmpiricalObservation{TaskFamily: "concurrency", DifficultyBucket: "0.60-0.75", Provider: "openai", Model: "o3-mini", Effort: EffortMedium, TaskClass: T3Difficult, QualityScore: .95, Success: true, ActualCostUSD: .1})
		e.RecordObservation(EmpiricalObservation{TaskFamily: "documentation", DifficultyBucket: "0.60-0.75", Provider: "openai", Model: "o3-mini", Effort: EffortMedium, TaskClass: T3Difficult, QualityScore: .20, Success: false, ActualCostUSD: .1})
	}
	p := TaskProfile{Family: "concurrency", Difficulty: .65, Class: T3Difficult}
	got := e.EstimateEnvelope(p, "openai", "o3-mini", EffortMedium)
	if got.EstimatorSource != "exact" || got.MeanQuality < .9 {
		t.Fatalf("family contamination: %+v", got)
	}
}

func TestFrontierRejectsCheapInsufficientConfiguration(t *testing.T) {
	floor := CapabilityFloor{RequiredQuality: .9, RequiredSuccessProbability: .9}
	f := Frontier{Configurations: []Configuration{
		{ExpectedE2ECostUSD: .01, Envelope: CapabilityEnvelope{QualityLCB: .4, SuccessLCB: .4}},
		{ExpectedE2ECostUSD: .05, Envelope: CapabilityEnvelope{QualityLCB: .95, SuccessLCB: .95}},
	}}
	got, ok := f.FindEmpiricalOracle(floor)
	if !ok || got.ExpectedE2ECostUSD != .05 {
		t.Fatalf("must preserve capability floor: %+v", got)
	}
}

func TestBuildAggregatedFrontierSingleObservation(t *testing.T) {
	obs := []EmpiricalObservation{
		{
			TaskID:        "task_1",
			Provider:      "gemini",
			Model:         "gemini-2.5-flash",
			Effort:        EffortLow,
			QualityScore:  0.88,
			Success:       true,
			ActualCostUSD: 0.002,
		},
	}
	frontier := BuildAggregatedFrontier("task_1", obs)
	if len(frontier.Configurations) != 1 {
		t.Fatalf("expected 1 configuration, got %d", len(frontier.Configurations))
	}
	c := frontier.Configurations[0]
	if c.PredictedQualityLCB != 0.88 {
		t.Errorf("expected quality LCB 0.88 for single observation, got %f", c.PredictedQualityLCB)
	}
	if c.Envelope.Samples != 1 {
		t.Errorf("expected 1 sample, got %d", c.Envelope.Samples)
	}
	if c.ExpectedE2ECostUSD != 0.002 {
		t.Errorf("expected cost 0.002, got %f", c.ExpectedE2ECostUSD)
	}
}

func TestBuildAggregatedFrontierMultiObservationLCB(t *testing.T) {
	// 5 repeated runs with some variance
	obs := []EmpiricalObservation{
		{TaskID: "task_1", Provider: "openai", Model: "o3-mini", Effort: EffortMedium, QualityScore: 0.90, Success: true, ActualCostUSD: 0.05, ActualLatencyMS: 1000},
		{TaskID: "task_1", Provider: "openai", Model: "o3-mini", Effort: EffortMedium, QualityScore: 0.92, Success: true, ActualCostUSD: 0.05, ActualLatencyMS: 1100},
		{TaskID: "task_1", Provider: "openai", Model: "o3-mini", Effort: EffortMedium, QualityScore: 0.88, Success: true, ActualCostUSD: 0.05, ActualLatencyMS: 1050},
		{TaskID: "task_1", Provider: "openai", Model: "o3-mini", Effort: EffortMedium, QualityScore: 0.94, Success: true, ActualCostUSD: 0.05, ActualLatencyMS: 950},
		{TaskID: "task_1", Provider: "openai", Model: "o3-mini", Effort: EffortMedium, QualityScore: 0.86, Success: true, ActualCostUSD: 0.05, ActualLatencyMS: 1200},
	}
	frontier := BuildAggregatedFrontier("task_1", obs)
	if len(frontier.Configurations) != 1 {
		t.Fatalf("expected 1 configuration, got %d", len(frontier.Configurations))
	}
	c := frontier.Configurations[0]
	// Mean quality is 0.90. Standard error should be positive, so QualityLCB < MeanQuality.
	if c.Envelope.MeanQuality != 0.90 {
		t.Errorf("expected mean quality 0.90, got %f", c.Envelope.MeanQuality)
	}
	if c.Envelope.QualityLCB >= c.Envelope.MeanQuality {
		t.Errorf("expected QualityLCB < MeanQuality due to sample variance, got LCB=%f, Mean=%f", c.Envelope.QualityLCB, c.Envelope.MeanQuality)
	}
	if c.Envelope.QualityLCB <= 0.80 {
		t.Errorf("QualityLCB unexpectedly low: %f", c.Envelope.QualityLCB)
	}
	if c.Envelope.Samples != 5 {
		t.Errorf("expected 5 samples, got %d", c.Envelope.Samples)
	}
}

func TestBuildAggregatedFrontierFiltersTaskID(t *testing.T) {
	obs := []EmpiricalObservation{
		{TaskID: "task_A", Provider: "openai", Model: "o3-mini", Effort: EffortLow, QualityScore: 0.95, Success: true, ActualCostUSD: 0.01},
		{TaskID: "task_B", Provider: "openai", Model: "o3-mini", Effort: EffortLow, QualityScore: 0.60, Success: false, ActualCostUSD: 0.01},
	}
	frontierA := BuildAggregatedFrontier("task_A", obs)
	if len(frontierA.Configurations) != 1 {
		t.Fatalf("expected 1 configuration for task_A, got %d", len(frontierA.Configurations))
	}
	if frontierA.Configurations[0].Envelope.MeanQuality != 0.95 {
		t.Errorf("task_B contaminated task_A: meanQuality=%f", frontierA.Configurations[0].Envelope.MeanQuality)
	}
}

