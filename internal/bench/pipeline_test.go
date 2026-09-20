package bench

import (
	"bytes"
	"strings"
	"testing"
)

func TestDataPipeline_RecordAndSummary(t *testing.T) {
	dp := NewDataPipeline()

	dp.Record(TaskExecutionRecord{
		TaskHash:        AnonymizeTask("Fix raft heartbeat election timeout"),
		Model:           "gpt-5.3-codex",
		Budget:          4000,
		InputTokens:     2500,
		CachedTokens:    1200,
		LatencyMs:       320,
		DollarCost:      0.005,
		Success:         true,
		CacheHit:        true,
		PropensityScore: 0.8,
	})

	dp.Record(TaskExecutionRecord{
		TaskHash:        AnonymizeTask("Refactor CSS grid styling"),
		Model:           "local",
		Budget:          2000,
		InputTokens:     1500,
		CachedTokens:    0,
		LatencyMs:       150,
		DollarCost:      0.0,
		Success:         true,
		CacheHit:        false,
		PropensityScore: 0.5,
	})

	dp.Record(TaskExecutionRecord{
		TaskHash:        AnonymizeTask("Investigate memory leak"),
		Model:           "claude-sonnet",
		Budget:          4000,
		InputTokens:     3500,
		CachedTokens:    0,
		LatencyMs:       850,
		DollarCost:      0.012,
		Success:         false,
		CacheHit:        false,
		PropensityScore: 0.7,
	})

	records := dp.Records()
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}

	summary := dp.Summary()
	if summary.TotalRuns != 3 {
		t.Errorf("expected 3 total runs, got %d", summary.TotalRuns)
	}
	expectedSuccessRate := 2.0 / 3.0
	if summary.SuccessRate != expectedSuccessRate {
		t.Errorf("expected success rate %f, got %f", expectedSuccessRate, summary.SuccessRate)
	}
	expectedCacheRate := 1.0 / 3.0
	if summary.CacheHitRate != expectedCacheRate {
		t.Errorf("expected cache hit rate %f, got %f", expectedCacheRate, summary.CacheHitRate)
	}

	// Test ExportJSON
	var buf bytes.Buffer
	if err := dp.ExportJSON(&buf); err != nil {
		t.Fatalf("ExportJSON failed: %v", err)
	}
	if !strings.Contains(buf.String(), "gpt-5.3-codex") {
		t.Errorf("exported JSON missing expected model")
	}

	// Test ExportJSONL
	var jsonlBuf bytes.Buffer
	if err := dp.ExportJSONL(&jsonlBuf); err != nil {
		t.Fatalf("ExportJSONL failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(jsonlBuf.String()), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 jsonl lines, got %d", len(lines))
	}
}

func TestAnonymizeTask(t *testing.T) {
	h1 := AnonymizeTask("task 1")
	h2 := AnonymizeTask("task 1")
	h3 := AnonymizeTask("task 2")

	if h1 != h2 {
		t.Errorf("expected deterministic hash for same task string")
	}
	if h1 == h3 {
		t.Errorf("expected different hash for different task strings")
	}
}
