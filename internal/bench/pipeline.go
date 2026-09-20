package bench

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// TaskExecutionRecord records anonymized, privacy-preserving execution metadata (PR-14).
type TaskExecutionRecord struct {
	ID                 string    `json:"id"`
	Timestamp          time.Time `json:"timestamp"`
	TaskHash           string    `json:"task_hash"` // SHA256 of task prompt to preserve privacy
	Model              string    `json:"model"`
	Budget             int       `json:"budget"`
	InputTokens        int       `json:"input_tokens"`
	CachedTokens       int       `json:"cached_tokens"`
	LatencyMs          int64     `json:"latency_ms"`
	DollarCost         float64   `json:"dollar_cost"`
	Success            bool      `json:"success"`
	CacheHit           bool      `json:"cache_hit"`
	SelectedCount      int       `json:"selected_count"`
	PropensityScore    float64   `json:"propensity_score"` // Logging policy probability pi_0(a | x)
	AllocatedCandidateIDs []string `json:"allocated_candidate_ids,omitempty"`
}

// PipelineSummary provides statistical aggregates across logged executions.
type PipelineSummary struct {
	TotalRuns       int     `json:"total_runs"`
	SuccessRate     float64 `json:"success_rate"`
	CacheHitRate    float64 `json:"cache_hit_rate"`
	AvgInputTokens  float64 `json:"avg_input_tokens"`
	AvgLatencyMs    float64 `json:"avg_latency_ms"`
	TotalCostUSD    float64 `json:"total_cost_usd"`
}

// DataPipeline manages local-first, privacy-preserving task trace logging and export.
type DataPipeline struct {
	mu      sync.RWMutex
	records []TaskExecutionRecord
}

// NewDataPipeline initializes an in-memory or file-backed data pipeline.
func NewDataPipeline() *DataPipeline {
	return &DataPipeline{
		records: make([]TaskExecutionRecord, 0),
	}
}

// AnonymizeTask generates a stable SHA256 identifier for a task prompt.
func AnonymizeTask(task string) string {
	h := sha256.Sum256([]byte(task))
	return hex.EncodeToString(h[:16])
}

// Record appends a new execution record thread-safely.
func (dp *DataPipeline) Record(rec TaskExecutionRecord) {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	if rec.ID == "" {
		rec.ID = fmt.Sprintf("rec-%d-%s", time.Now().UnixNano(), rec.TaskHash[:8])
	}
	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now().UTC()
	}
	if rec.PropensityScore <= 0 {
		rec.PropensityScore = 1.0
	}
	dp.records = append(dp.records, rec)
}

// Records returns a copy of all logged records.
func (dp *DataPipeline) Records() []TaskExecutionRecord {
	dp.mu.RLock()
	defer dp.mu.RUnlock()
	out := make([]TaskExecutionRecord, len(dp.records))
	copy(out, dp.records)
	return out
}

// Summary calculates aggregate performance metrics.
func (dp *DataPipeline) Summary() PipelineSummary {
	dp.mu.RLock()
	defer dp.mu.RUnlock()

	n := len(dp.records)
	if n == 0 {
		return PipelineSummary{}
	}

	successCount := 0
	cacheHitCount := 0
	totalTokens := 0
	var totalLatency int64
	var totalCost float64

	for _, r := range dp.records {
		if r.Success {
			successCount++
		}
		if r.CacheHit {
			cacheHitCount++
		}
		totalTokens += r.InputTokens
		totalLatency += r.LatencyMs
		totalCost += r.DollarCost
	}

	return PipelineSummary{
		TotalRuns:      n,
		SuccessRate:    float64(successCount) / float64(n),
		CacheHitRate:   float64(cacheHitCount) / float64(n),
		AvgInputTokens: float64(totalTokens) / float64(n),
		AvgLatencyMs:   float64(totalLatency) / float64(n),
		TotalCostUSD:   totalCost,
	}
}

// ExportJSON exports all execution traces to standard JSON.
func (dp *DataPipeline) ExportJSON(w io.Writer) error {
	dp.mu.RLock()
	defer dp.mu.RUnlock()
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(dp.records)
}

// ExportJSONL exports traces one record per line for streaming ML pipelines.
func (dp *DataPipeline) ExportJSONL(w io.Writer) error {
	dp.mu.RLock()
	defer dp.mu.RUnlock()
	enc := json.NewEncoder(w)
	for _, r := range dp.records {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	return nil
}
