package extractor

import (
	"strings"
	"testing"
	"time"

	"contextos/internal/model"
)

func TestExtractFromTrace(t *testing.T) {
	pipeline := NewPipeline("rev-100")

	trace := `
User: We must strictly require transactional outbox pattern for retry queues.
User: We decided to use transactional outbox for event publishing in internal/outbox/publisher.go.
Tool: Test run failed: reproduced bug: connection pool exhaustion in database/pool.go:42 commit abc123def
`

	claims := pipeline.ExtractFromText(trace, "user_turn", "sess-1", "/repo")
	if len(claims) < 3 {
		t.Fatalf("expected at least 3 extracted claims, got %d", len(claims))
	}

	var foundConstraint, foundDecision, foundFailure bool
	for _, c := range claims {
		switch c.Kind {
		case "constraint":
			foundConstraint = true
			if len(c.Evidence) == 0 {
				t.Errorf("constraint should have bound evidence")
			}
		case "decision":
			foundDecision = true
			if len(c.Locations) == 0 {
				t.Errorf("decision should have bound locations, got none")
			}
		case "failure":
			foundFailure = true
			if len(c.Evidence) == 0 {
				t.Errorf("failure should have bound commit/source evidence")
			}
		}
	}

	if !foundConstraint {
		t.Error("failed to extract constraint claim")
	}
	if !foundDecision {
		t.Error("failed to extract decision claim")
	}
	if !foundFailure {
		t.Error("failed to extract failure claim")
	}
}

func TestConflictResolutionHistoricalPreservation(t *testing.T) {
	// Critical test from PR.md:
	// "Agent says A, later says B.
	// B must not silently mutate the historical record of A.
	// Instead: A valid [t0, t1], B valid [t1, ...]"

	t0 := time.Now().Add(-1 * time.Hour)
	existingA := model.Memory{
		ID:                "mem-a",
		Kind:              "decision",
		Content:           "We decided to use kafka for message queuing",
		Claim:             "We decided to use kafka for message queuing",
		Scope:             "repository",
		ValidFromRevision: "rev-100",
		Authority:         "user",
		Confidence:        0.90,
		Timestamp:         t0.Format(time.RFC3339),
	}
	existingStore := []model.Memory{existingA}

	// Later at revision rev-200, user/agent decides B (reversal/switch from kafka to redis)
	candidateB := ExtractedClaim{
		Claim:       "Switched from kafka to redis for message queuing",
		Kind:        "decision",
		Scope:       "repository",
		Authority:   "user",
		Confidence:  0.95,
		Timestamp:   time.Now().UTC(),
		IsReversal:  true,
		NegatesTerm: "kafka",
	}

	res := ResolveConflicts(existingStore, candidateB, "rev-200")
	if !res.Resolved {
		t.Fatalf("expected conflict resolution to succeed")
	}
	if res.IsDuplicate {
		t.Fatalf("reversal must not be treated as a duplicate")
	}

	// 1. Verify historical record of A is preserved and invalidated at rev-200
	if len(res.UpdatedExisting) != 1 {
		t.Fatalf("expected 1 updated existing memory, got %d", len(res.UpdatedExisting))
	}
	oldA := res.UpdatedExisting[0]
	if oldA.ID != "mem-a" {
		t.Errorf("expected updated memory ID to be mem-a, got %s", oldA.ID)
	}
	if oldA.ValidFromRevision != "rev-100" {
		t.Errorf("original ValidFromRevision rev-100 must be preserved, got %s", oldA.ValidFromRevision)
	}
	if oldA.InvalidatedAtRevision != "rev-200" {
		t.Errorf("expected InvalidatedAtRevision to be rev-200, got %s", oldA.InvalidatedAtRevision)
	}
	if oldA.SupersededBy != res.NewMemory.ID {
		t.Errorf("expected SupersededBy pointing to new memory ID %s, got %s", res.NewMemory.ID, oldA.SupersededBy)
	}

	// 2. Verify new memory B is valid starting at rev-200 and points back to superseded memory A
	newB := res.NewMemory
	if newB.ValidFromRevision != "rev-200" {
		t.Errorf("new memory must be valid from rev-200, got %s", newB.ValidFromRevision)
	}
	if newB.InvalidatedAtRevision != "" {
		t.Errorf("new memory must not be invalidated, got %s", newB.InvalidatedAtRevision)
	}
	if newB.Supersedes != "mem-a" {
		t.Errorf("new memory Supersedes must point to mem-a, got %s", newB.Supersedes)
	}
	if !strings.Contains(newB.Claim, "redis") {
		t.Errorf("new memory claim must reflect decision B")
	}
}

func TestDuplicateClaimsMerged(t *testing.T) {
	existingA := model.Memory{
		ID:         "mem-1",
		Kind:       "decision",
		Content:    "use transactional outbox for event publishing",
		Claim:      "use transactional outbox for event publishing",
		Scope:      "repository",
		Authority:  "user",
		Confidence: 0.85,
		ReuseCount: 1,
		Evidence: []model.EvidenceItem{
			{Type: "source", Path: "internal/outbox/publisher.go", Weight: 0.9},
		},
	}

	candidateDuplicate := ExtractedClaim{
		Claim:      "use transactional outbox for event publishing",
		Kind:       "decision",
		Scope:      "repository",
		Authority:  "user",
		Confidence: 0.90,
		Timestamp:  time.Now().UTC(),
		Evidence: []model.EvidenceItem{
			{Type: "commit", ID: "commit-456", Weight: 0.85},
		},
	}

	res := ResolveConflicts([]model.Memory{existingA}, candidateDuplicate, "rev-150")
	if !res.IsDuplicate {
		t.Fatalf("expected duplicate to be detected")
	}
	merged := res.NewMemory
	if merged.ReuseCount != 2 {
		t.Errorf("expected ReuseCount to increment to 2, got %d", merged.ReuseCount)
	}
	if len(merged.Evidence) != 2 {
		t.Errorf("expected combined 2 evidence items, got %d", len(merged.Evidence))
	}
	if merged.Confidence < 0.90 {
		t.Errorf("expected confidence to be at least max(0.85, 0.90), got %.2f", merged.Confidence)
	}
}

func TestConfidenceCalibration(t *testing.T) {
	// User authority + multiple strong evidences
	strongEvidence := []model.EvidenceItem{
		{Type: "user", Weight: 0.99},
		{Type: "test", Weight: 0.95},
		{Type: "source", Weight: 0.90},
	}
	confHigh := CalibrateConfidence("user", strongEvidence)
	if confHigh < 0.85 {
		t.Errorf("expected high calibrated confidence for strong evidence (> 0.85), got %.4f", confHigh)
	}

	// Weak ungrounded inference with no evidence
	confLow := CalibrateConfidence("inference", nil)
	if confLow > 0.65 {
		t.Errorf("expected moderate/low confidence for ungrounded inference (< 0.65), got %.4f", confLow)
	}

	if confHigh <= confLow {
		t.Errorf("strong evidence confidence (%.4f) must exceed ungrounded inference confidence (%.4f)", confHigh, confLow)
	}
}
