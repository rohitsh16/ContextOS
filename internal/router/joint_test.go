package router

import (
	"testing"

	"contextos/internal/model"
)

func TestJointRouting_CompareStrategies(t *testing.T) {
	cands := []model.Candidate{
		{ID: "c1", Content: "Distributed transaction manager", Tokens: 500, Confidence: 0.95, TaskAffinity: 0.9},
		{ID: "c2", Content: "Raft consensus election protocol", Tokens: 800, Confidence: 0.9, TaskAffinity: 0.85},
		{ID: "c3", Content: "Storage engine WAL flush", Tokens: 1200, Confidence: 0.85, TaskAffinity: 0.8},
		{ID: "c4", Content: "API endpoint handler", Tokens: 400, Confidence: 0.7, TaskAffinity: 0.5},
	}

	req := JointRouteRequest{
		Task:             "Debug distributed transaction deadlock in Raft consensus",
		TaskValue:        20.0,
		Candidates:       cands,
		AvailableBudgets: []int{1000, 2500, 5000},
		CacheHitRatio:    0.5,
	}

	strategies := CompareStrategies(req)

	if len(strategies) != 4 {
		t.Fatalf("expected 4 strategies compared, got %d", len(strategies))
	}

	fixed, ok := strategies["fixed"]
	if !ok {
		t.Fatalf("missing fixed strategy")
	}
	modelOnly, ok := strategies["model-only"]
	if !ok {
		t.Fatalf("missing model-only strategy")
	}
	contextOnly, ok := strategies["context-only"]
	if !ok {
		t.Fatalf("missing context-only strategy")
	}
	joint, ok := strategies["joint"]
	if !ok {
		t.Fatalf("missing joint strategy")
	}

	t.Logf("Fixed EV: %.4f (Model: %s, Tokens: %d)", fixed.ExpectedValue, fixed.SelectedModel.Name, fixed.SelectedTokens)
	t.Logf("Model-Only EV: %.4f (Model: %s, Tokens: %d)", modelOnly.ExpectedValue, modelOnly.SelectedModel.Name, modelOnly.SelectedTokens)
	t.Logf("Context-Only EV: %.4f (Model: %s, Tokens: %d)", contextOnly.ExpectedValue, contextOnly.SelectedModel.Name, contextOnly.SelectedTokens)
	t.Logf("Joint EV: %.4f (Model: %s, Tokens: %d)", joint.ExpectedValue, joint.SelectedModel.Name, joint.SelectedTokens)

	// Joint optimization should produce expected value at least as good as fixed
	if joint.ExpectedValue < fixed.ExpectedValue-1e-6 {
		t.Errorf("joint expected value (%.4f) should be >= fixed (%.4f)", joint.ExpectedValue, fixed.ExpectedValue)
	}
	if joint.SuccessProb <= 0 || joint.SuccessProb > 1.0 {
		t.Errorf("invalid joint success probability: %f", joint.SuccessProb)
	}
}

func TestEstimateSuccessProbabilityAndCost(t *testing.T) {
	codex := GetProfile(ModelGPT53Codex)
	local := GetProfile(ModelLocal)

	probCodex := EstimateSuccessProbability(codex, 2000, 4000, "Refactor core compiler")
	probLocal := EstimateSuccessProbability(local, 2000, 4000, "Refactor core compiler")

	if probCodex <= probLocal {
		t.Errorf("expected Codex to have higher success probability than local model for complex refactor task")
	}

	costCodex := EstimateCost(codex, 10000, 0.5)
	costLocal := EstimateCost(local, 10000, 0.5)

	if costCodex <= 0 {
		t.Errorf("expected positive cost for cloud model")
	}
	if costLocal != 0.0 {
		t.Errorf("expected zero cost for local model, got %f", costLocal)
	}
}
