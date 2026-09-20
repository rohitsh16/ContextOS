package temporal

import (
	"testing"

	"contextos/internal/graph"
	"contextos/internal/model"
)

func TestUnrelatedCommitDoesNotStaleMemory(t *testing.T) {
	// PR-08 fundamental requirement:
	// A memory about README.md does not become invalid because payment_service.go changed.
	mem := model.Memory{
		ID:                "mem-readme",
		Kind:              "decision",
		Content:           "Documentation style guide and setup instructions",
		Location:          "README.md",
		Scope:             "file",
		ValidFromRevision: "rev-1",
	}

	// Git diff says payment_service.go was modified in rev-2
	changedFiles := map[string]string{
		"internal/service/payment_service.go": "M",
	}

	evaluator := NewScopedStalenessEvaluator(changedFiles, "rev-2", nil)
	freshRisk, hardStale := evaluator.EvaluateStaleness(mem)

	if hardStale {
		t.Errorf("unrelated file change must not mark memory as hard stale")
	}
	if freshRisk != 0.0 {
		t.Errorf("unrelated file change must produce freshRisk = 0.0, got %.4f", freshRisk)
	}
}

func TestScopedFileModifiedProducesFreshnessPenalty(t *testing.T) {
	mem := model.Memory{
		ID:                "mem-payment",
		Kind:              "decision",
		Content:           "Transactional outbox pattern in payment service",
		Location:          "internal/service/payment_service.go",
		Scope:             "file",
		ValidFromRevision: "rev-1",
	}

	changedFiles := map[string]string{
		"internal/service/payment_service.go": "M",
	}

	evaluator := NewScopedStalenessEvaluator(changedFiles, "rev-2", nil)
	freshRisk, hardStale := evaluator.EvaluateStaleness(mem)

	if hardStale {
		t.Errorf("modified file should not be hard stale, only freshRisk penalty")
	}
	if freshRisk < 0.70 {
		t.Errorf("modified scoped file should produce substantial freshRisk penalty (> 0.70), got %.4f", freshRisk)
	}
}

func TestScopedFileDeletedIsHardStale(t *testing.T) {
	mem := model.Memory{
		ID:                "mem-deprecated",
		Kind:              "fact",
		Content:           "Legacy authentication handler",
		Location:          "internal/auth/legacy.go",
		Scope:             "file",
		ValidFromRevision: "rev-1",
	}

	changedFiles := map[string]string{
		"internal/auth/legacy.go": "D",
	}

	evaluator := NewScopedStalenessEvaluator(changedFiles, "rev-2", nil)
	freshRisk, hardStale := evaluator.EvaluateStaleness(mem)

	if !hardStale {
		t.Errorf("deleted scoped file must mark memory as hardStale")
	}
	if freshRisk != 1.0 {
		t.Errorf("deleted scoped file must produce freshRisk = 1.0, got %.4f", freshRisk)
	}
}

func TestTransitiveDependencyChange(t *testing.T) {
	// Dep: handler.go -> service.go
	g := graph.New(graph.DefaultConfig())
	g.AddEdge("pkg/handler.go", "pkg/service.go", "import")

	mem := model.Memory{
		ID:                "mem-handler",
		Kind:              "decision",
		Content:           "Handler routing rules",
		Location:          "pkg/handler.go",
		Scope:             "file",
		ValidFromRevision: "rev-1",
	}

	// service.go modified
	changedFiles := map[string]string{
		"pkg/service.go": "M",
	}

	evaluator := NewScopedStalenessEvaluator(changedFiles, "rev-2", g)
	freshRisk, hardStale := evaluator.EvaluateStaleness(mem)

	if hardStale {
		t.Errorf("transitive dependency change should not be hard stale")
	}
	if freshRisk <= 0.0 || freshRisk >= 0.70 {
		t.Errorf("transitive dependency should yield moderate freshRisk in (0, 0.70), got %.4f", freshRisk)
	}
}

func TestExplicitInvalidationAndSuperseded(t *testing.T) {
	memInvalid := model.Memory{
		ID:                    "mem-old",
		InvalidatedAtRevision: "rev-2",
	}
	evaluator := NewScopedStalenessEvaluator(nil, "rev-3", nil)
	freshRisk, hardStale := evaluator.EvaluateStaleness(memInvalid)
	if !hardStale || freshRisk != 1.0 {
		t.Errorf("explicit invalidation must be hardStale with freshRisk=1.0")
	}

	memSuperseded := model.Memory{
		ID:           "mem-super",
		SupersededBy: "mem-new",
	}
	freshRisk2, hardStale2 := evaluator.EvaluateStaleness(memSuperseded)
	if !hardStale2 || freshRisk2 != 1.0 {
		t.Errorf("superseded memory must be hardStale with freshRisk=1.0")
	}
}
