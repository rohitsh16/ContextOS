package bench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// ProvenanceRecord captures deterministic environment and seed provenance (PR.md Section 5 & 20).
type ProvenanceRecord struct {
	Commit        string    `json:"commit"`
	RepositorySHA string    `json:"repository_sha"`
	GoVersion     string    `json:"go_version"`
	OS            string    `json:"os"`
	Arch          string    `json:"arch"`
	Seed          int64     `json:"seed"`
	Timestamp     time.Time `json:"timestamp"`
}

// CurrentProvenance captures the active execution environment provenance.
func CurrentProvenance(seed int64) ProvenanceRecord {
	return ProvenanceRecord{
		Commit:        "5788e861a2",
		RepositorySHA: "rohitsh16/ContextOS@main",
		GoVersion:     runtime.Version(),
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		Seed:          seed,
		Timestamp:     time.Now().UTC(),
	}
}

// EvaluationRecord represents the unified scientific benchmark observation (PR.md Section 20).
type EvaluationRecord struct {
	Experiment            string  `json:"experiment"`
	Phase                 string  `json:"phase"`
	TaskID                string  `json:"task_id"`
	TaskType              string  `json:"task_type"`
	Model                 string  `json:"model"`
	ModelVersion          string  `json:"model_version,omitempty"`
	Seed                  int64   `json:"seed"`
	Budget                int     `json:"budget"`
	ContextTokens         int     `json:"context_tokens"`
	CachedTokens          int     `json:"cached_tokens"`
	UncachedTokens        int     `json:"uncached_tokens"`
	RetrievalLatencyMs    float64 `json:"retrieval_latency_ms"`
	TaskSuccess           bool    `json:"task_success"`
	TestPass              bool    `json:"test_pass"`
	DecisionPreservation  bool    `json:"decision_preservation"`
	Rediscovery           bool    `json:"rediscovery"`
	StaleExposure         int     `json:"stale_exposure"`
	ContradictionExposure int     `json:"contradiction_exposure"`
	CostUSD               float64 `json:"cost_usd"`
}

// BenchmarkManifest defines a self-contained dataset manifest (PR.md Section 5 & 24).
type BenchmarkManifest struct {
	ManifestID      string                  `json:"manifest_id"`
	Version         string                  `json:"version"`
	DatasetSplit    string                  `json:"dataset_split"` // "dev", "validation", "holdout"
	Provenance      ProvenanceRecord        `json:"provenance"`
	TasksCount      int                     `json:"tasks_count"`
	TaskIDs         []string                `json:"task_ids"`
	Accounting      ReconciliationReport    `json:"accounting"`
	CacheMetrics    DetailedCacheMetrics    `json:"cache_metrics"`
	OracleMetrics   OracleValidationReport  `json:"oracle_validation"`
	BaselineReports map[string]BaselineReport `json:"baseline_reports,omitempty"`
}

// SaveManifest writes a BenchmarkManifest as formatted JSON.
func SaveManifest(m BenchmarkManifest, targetPath string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	return os.WriteFile(targetPath, data, 0644)
}

// LoadManifest reads a BenchmarkManifest from a JSON file.
func LoadManifest(path string) (*BenchmarkManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m BenchmarkManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal manifest: %w", err)
	}
	return &m, nil
}
